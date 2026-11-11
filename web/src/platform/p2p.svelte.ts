import { api } from "./api";
import {
  isLocalOrOverlayHost,
  medianSample,
  shouldUpgradeToP2P,
} from "./p2p-framing";
import {
  measureBaselineHttpRtt,
  negotiateDirectPeer,
  type NegotiatedSession,
} from "./p2p-session";
import { createP2PTransport } from "./p2p-transport";
import { P2PStatusSchema, type P2PStatus } from "../contracts/schemas";
import { onActiveHostChange } from "./storage";
import { defaultBrowserTransport, setClientTransport } from "./transport";
import { handoffLiveSockets } from "./ws";

const KEEPALIVE_INTERVAL_MS = 15_000;
const BACKOFF_STEPS_MS = [5_000, 15_000, 60_000];

export type P2PPhase =
  | "idle"
  | "disabled"
  | "unsupported"
  | "skipped_local"
  | "skipped_low_rtt"
  | "probing"
  | "negotiating"
  | "connected"
  | "rejected_relay"
  | "standby_hysteresis"
  | "fallback";

export interface P2PABTestSummary {
  running: boolean;
  error: string;
  baselineSamples: number[];
  baselineMinMs: number | null;
  baselineMedianMs: number | null;
  p2pSamples: number[];
  p2pMinMs: number | null;
  p2pMedianMs: number | null;
  directSuccess: boolean;
  candidateType: string;
  ipFamily: string;
}

export const p2pState = $state({
  mode: "ws" as "ws" | "p2p",
  phase: "idle" as P2PPhase,
  candidateType: "",
  ipFamily: "",
  portMapState: "off" as "off" | "probing" | "mapped" | "unavailable",
  portMapProtocol: "",
  portMapExternalPort: 0,
  baselineRttMs: null as number | null,
  p2pRttMs: null as number | null,
  upgradeDurationMs: null as number | null,
  switchCount: 0,
  firstReadyMs: null as number | null,
  abTest: {
    running: false,
    error: "",
    baselineSamples: [],
    baselineMinMs: null,
    baselineMedianMs: null,
    p2pSamples: [],
    p2pMinMs: null,
    p2pMedianMs: null,
    directSuccess: false,
    candidateType: "",
    ipFamily: "",
  } as P2PABTestSummary,
});

const bootTime = typeof performance !== "undefined" ? performance.now() : 0;

export function markFirstScreenReady() {
  if (p2pState.firstReadyMs === null && typeof performance !== "undefined") {
    p2pState.firstReadyMs = Math.max(
      0,
      Math.round(performance.now() - bootTime),
    );
  }
}

let activeEpoch = 0;
let activeSession: NegotiatedSession | null = null;
let keepaliveTimer: ReturnType<typeof setInterval> | undefined;
let retryTimer: ReturnType<typeof setTimeout> | undefined;
let retryIndex = 0;
let forceMode = false;

function hasForceFlag(): boolean {
  if (
    typeof location !== "undefined" &&
    new URLSearchParams(location.search).get("p2p") === "force"
  ) {
    forceMode = true;
  }
  return forceMode;
}

function teardownActiveSession(notifyServer: boolean) {
  clearInterval(keepaliveTimer);
  keepaliveTimer = undefined;
  const current = activeSession;
  activeSession = null;
  current?.close(notifyServer);
}

function fallbackToBaseline(nextPhase: P2PPhase, scheduleRetry: boolean) {
  const wasP2P = p2pState.mode === "p2p";
  teardownActiveSession(true);
  setClientTransport(null);
  p2pState.mode = "ws";
  p2pState.phase = nextPhase;
  if (wasP2P) {
    p2pState.switchCount++;
    handoffLiveSockets();
  }
  if (scheduleRetry) {
    clearTimeout(retryTimer);
    const delay =
      BACKOFF_STEPS_MS[Math.min(retryIndex++, BACKOFF_STEPS_MS.length - 1)]!;
    const epoch = activeEpoch;
    retryTimer = setTimeout(() => {
      if (epoch === activeEpoch) void probeAndMaybeUpgrade(epoch);
    }, delay);
  }
}

function applyServerP2PStatus(status: P2PStatus) {
  p2pState.portMapState = status.portMap.state;
  p2pState.portMapProtocol = status.portMap.protocol ?? "";
  p2pState.portMapExternalPort = status.portMap.externalPort ?? 0;
  if (!status.supported) {
    fallbackToBaseline("unsupported", false);
    return;
  }
  if (!status.enabled) {
    fallbackToBaseline("disabled", false);
  }
}

async function probeAndMaybeUpgrade(epoch: number) {
  if (epoch !== activeEpoch) return;
  if (typeof RTCPeerConnection === "undefined") {
    p2pState.phase = "unsupported";
    return;
  }

  let status: P2PStatus;
  try {
    status = await api("/p2p", P2PStatusSchema);
  } catch {
    return;
  }
  if (epoch !== activeEpoch) return;
  applyServerP2PStatus(status);
  if (!status.supported || !status.enabled) return;

  const checkForce = hasForceFlag();

  if (
    !checkForce &&
    typeof location !== "undefined" &&
    isLocalOrOverlayHost(location.hostname)
  ) {
    p2pState.phase = "skipped_local";
    return;
  }

  p2pState.phase = "probing";
  let baseMedian: number | null = null;
  try {
    const baseSamples = await measureBaselineHttpRtt(3);
    baseMedian = medianSample(baseSamples);
    p2pState.baselineRttMs = baseMedian;
  } catch {
    fallbackToBaseline("fallback", true);
    return;
  }
  if (epoch !== activeEpoch || baseMedian === null) return;

  if (!checkForce && baseMedian < 15) {
    p2pState.phase = "skipped_low_rtt";
    return;
  }

  p2pState.phase = "negotiating";
  const upgradeStart = performance.now();
  let session: NegotiatedSession;
  try {
    session = await negotiateDirectPeer(status.stunServers);
  } catch (err) {
    if (epoch !== activeEpoch) return;
    if (err instanceof Error && err.name === "RelayRejectedError") {
      p2pState.phase = "rejected_relay";
      return;
    }
    fallbackToBaseline("fallback", true);
    return;
  }
  if (epoch !== activeEpoch) {
    session.close(true);
    return;
  }

  const p2pSamples: number[] = [];
  try {
    for (let i = 0; i < 3; i++) {
      p2pSamples.push(await session.sendPing());
    }
  } catch {
    session.close(true);
    fallbackToBaseline("fallback", true);
    return;
  }

  const p2pMed = medianSample(p2pSamples) ?? 0;
  p2pState.p2pRttMs = p2pMed;
  p2pState.candidateType = session.candidateType;
  p2pState.ipFamily = session.ipFamily;

  session.control.send(
    JSON.stringify({
      type: "peer_meta",
      candidateType: session.candidateType,
      ipFamily: session.ipFamily,
      rttMs: p2pMed,
    }),
  );

  if (!checkForce && !shouldUpgradeToP2P(baseMedian, p2pMed)) {
    session.close(true);
    p2pState.phase = "standby_hysteresis";
    return;
  }

  teardownActiveSession(true);
  activeSession = session;
  retryIndex = 0;

  session.pc.onconnectionstatechange = () => {
    if (activeSession !== session) return;
    const st = session.pc.connectionState;
    if (st === "disconnected" || st === "failed" || st === "closed") {
      fallbackToBaseline("fallback", true);
    }
  };
  session.control.onclose = () => {
    if (activeSession === session) {
      fallbackToBaseline("fallback", true);
    }
  };

  const transport = createP2PTransport({
    pc: session.pc,
    fallback: defaultBrowserTransport(),
    onChannelFailure: () => {
      if (
        activeSession === session &&
        (session.pc.connectionState === "disconnected" ||
          session.pc.connectionState === "failed" ||
          session.pc.connectionState === "closed")
      ) {
        fallbackToBaseline("fallback", true);
      }
    },
  });

  setClientTransport(transport);
  p2pState.mode = "p2p";
  p2pState.phase = "connected";
  p2pState.upgradeDurationMs = Math.max(
    1,
    Math.round(performance.now() - upgradeStart),
  );
  p2pState.switchCount++;
  handoffLiveSockets();

  keepaliveTimer = setInterval(() => {
    if (activeSession !== session) return;
    void (async () => {
      try {
        const rtt = await session.sendPing();
        p2pState.p2pRttMs = rtt;
        session.control.send(
          JSON.stringify({
            type: "peer_meta",
            candidateType: p2pState.candidateType,
            ipFamily: p2pState.ipFamily,
            rttMs: rtt,
          }),
        );
      } catch {
        if (activeSession === session) {
          fallbackToBaseline("fallback", true);
        }
      }
    })();
  }, KEEPALIVE_INTERVAL_MS);
}

function handleNetworkOrForegroundEvent() {
  if (!activeEpoch) return;
  if (p2pState.mode === "p2p" && activeSession) {
    void activeSession.sendPing().catch(() => {
      fallbackToBaseline("fallback", true);
    });
  } else if (p2pState.phase === "fallback") {
    clearTimeout(retryTimer);
    retryIndex = 0;
    void probeAndMaybeUpgrade(activeEpoch);
  }
}

export function startP2P() {
  stopP2P();
  hasForceFlag();
  const epoch = ++activeEpoch;
  if (typeof window !== "undefined") {
    window.addEventListener("online", handleNetworkOrForegroundEvent);
    document.addEventListener(
      "visibilitychange",
      handleNetworkOrForegroundEvent,
    );
  }
  void probeAndMaybeUpgrade(epoch);
}

export function stopP2P() {
  activeEpoch++;
  clearTimeout(retryTimer);
  retryTimer = undefined;
  retryIndex = 0;
  if (typeof window !== "undefined") {
    window.removeEventListener("online", handleNetworkOrForegroundEvent);
    document.removeEventListener(
      "visibilitychange",
      handleNetworkOrForegroundEvent,
    );
  }
  const wasP2P = p2pState.mode === "p2p";
  teardownActiveSession(true);
  setClientTransport(null);
  p2pState.mode = "ws";
  p2pState.phase = "idle";
  if (wasP2P) {
    handoffLiveSockets();
  }
}

export function refreshP2PAfterSettingsChange(status: P2PStatus) {
  applyServerP2PStatus(status);
  if (status.supported && status.enabled && p2pState.mode === "ws") {
    clearTimeout(retryTimer);
    retryIndex = 0;
    void probeAndMaybeUpgrade(activeEpoch);
  }
}

export async function runP2PEchoABTest(stunServers: string[]): Promise<void> {
  if (p2pState.abTest.running) return;
  p2pState.abTest = {
    running: true,
    error: "",
    baselineSamples: [],
    baselineMinMs: null,
    baselineMedianMs: null,
    p2pSamples: [],
    p2pMinMs: null,
    p2pMedianMs: null,
    directSuccess: false,
    candidateType: "",
    ipFamily: "",
  };

  let tempSession: NegotiatedSession | null = null;
  try {
    const baseSamples = await measureBaselineHttpRtt(8);
    p2pState.abTest.baselineSamples = baseSamples;
    p2pState.abTest.baselineMinMs = Math.min(...baseSamples);
    p2pState.abTest.baselineMedianMs = medianSample(baseSamples);

    let session = activeSession;
    if (!session || session.control.readyState !== "open") {
      tempSession = await negotiateDirectPeer(stunServers);
      session = tempSession;
    }

    const p2pSamples: number[] = [];
    for (let i = 0; i < 8; i++) {
      p2pSamples.push(await session.sendPing());
    }
    p2pState.abTest.p2pSamples = p2pSamples;
    p2pState.abTest.p2pMinMs = Math.min(...p2pSamples);
    p2pState.abTest.p2pMedianMs = medianSample(p2pSamples);
    p2pState.abTest.directSuccess = true;
    p2pState.abTest.candidateType = session.candidateType;
    p2pState.abTest.ipFamily = session.ipFamily;
  } catch (err) {
    p2pState.abTest.error =
      err instanceof Error ? err.message : "双路径测速失败";
  } finally {
    tempSession?.close(true);
    p2pState.abTest.running = false;
  }
}

onActiveHostChange(() => {
  forceMode = false;
  stopP2P();
});
