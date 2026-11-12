import {
  P2PControlMessageSchema,
  P2PPingResponseSchema,
  P2PSignalResponseSchema,
  type P2PICECandidate,
} from "../contracts/schemas";
import { stored } from "./storage";
import { defaultBrowserTransport } from "./transport";

const P2P_PROTO_VERSION = 1;
const CONTROL_LABEL = "webmux:control";
const PING_TIMEOUT_MS = 4_000;
const NEGOTIATION_TIMEOUT_MS = 30_000;
const CANDIDATE_INTERVAL_MS = 1_000;

export interface NegotiatedSession {
  peerId: string;
  pc: RTCPeerConnection;
  control: RTCDataChannel;
  candidateType: string;
  ipFamily: string;
  sendPing: () => Promise<number>;
  close: (notifyServer?: boolean) => void;
}

let pingSeq = 0;

function delay(ms: number, signal: AbortSignal): Promise<void> {
  signal.throwIfAborted();
  return new Promise<void>((resolve, reject) => {
    const timer = setTimeout(() => {
      signal.removeEventListener("abort", abort);
      resolve();
    }, ms);
    const abort = () => {
      clearTimeout(timer);
      reject(signal.reason);
    };
    signal.addEventListener("abort", abort, { once: true });
  });
}

async function withRequestTimeout<T>(
  signal: AbortSignal | undefined,
  timeoutMs: number,
  request: (signal: AbortSignal) => Promise<T>,
): Promise<T> {
  const controller = new AbortController();
  const timer = setTimeout(
    () => controller.abort(new Error("P2P request timeout")),
    timeoutMs,
  );
  const abort = () => {
    clearTimeout(timer);
    controller.abort(signal?.reason);
  };
  if (signal?.aborted) abort();
  else signal?.addEventListener("abort", abort, { once: true });
  try {
    controller.signal.throwIfAborted();
    return await request(controller.signal);
  } finally {
    clearTimeout(timer);
    signal?.removeEventListener("abort", abort);
  }
}

async function sendSignalHttp(
  payload: {
    action: "offer" | "candidate" | "close";
    peerId?: string;
    sdp?: string;
    candidates?: P2PICECandidate[];
  },
  signal?: AbortSignal,
) {
  return withRequestTimeout(
    signal,
    payload.action === "close" ? PING_TIMEOUT_MS : NEGOTIATION_TIMEOUT_MS,
    async (requestSignal) => {
      const res = await defaultBrowserTransport().fetch("/p2p/signal", {
        method: "POST",
        headers: {
          Authorization: "Bearer " + stored("token"),
          "Content-Type": "application/json",
        },
        body: JSON.stringify(payload),
        signal: requestSignal,
      });
      if (!res.ok) {
        throw new Error(`P2P signal failed (${res.status})`);
      }
      return P2PSignalResponseSchema.parse(await res.json());
    },
  );
}

export async function measureBaselineHttpRtt(
  count = 3,
  signal?: AbortSignal,
): Promise<number[]> {
  const samples: number[] = [];
  for (let i = 0; i < count; i++) {
    const started = performance.now();
    await withRequestTimeout(signal, PING_TIMEOUT_MS, async (requestSignal) => {
      const res = await defaultBrowserTransport().fetch(
        `/p2p/ping?seq=${++pingSeq}`,
        {
          method: "GET",
          headers: { Authorization: "Bearer " + stored("token") },
          signal: requestSignal,
        },
      );
      if (!res.ok) throw new Error(`HTTP ping failed (${res.status})`);
      P2PPingResponseSchema.parse(await res.json());
      requestSignal.throwIfAborted();
    });
    samples.push(Math.max(0, Math.round(performance.now() - started)));
  }
  return samples;
}

function asString(value: unknown): string {
  return typeof value === "string" ? value : "";
}

export async function inspectSelectedCandidatePair(
  pc: RTCPeerConnection,
): Promise<{
  isRelay: boolean;
  candidateType: string;
  ipFamily: string;
}> {
  if (typeof pc.getStats !== "function") {
    return { isRelay: false, candidateType: "direct", ipFamily: "" };
  }
  const stats = await pc.getStats();
  let selectedLocalId = "";
  let selectedRemoteId = "";

  stats.forEach((report: Record<string, unknown>) => {
    if (
      report.type === "transport" &&
      typeof report.selectedCandidatePairId === "string"
    ) {
      const pair = stats.get(report.selectedCandidatePairId) as
        | Record<string, unknown>
        | undefined;
      if (pair) {
        selectedLocalId = asString(pair.localCandidateId);
        selectedRemoteId = asString(pair.remoteCandidateId);
      }
    }
  });

  if (!selectedLocalId) {
    stats.forEach((report: Record<string, unknown>) => {
      if (
        report.type === "candidate-pair" &&
        (report.selected === true || report.nominated === true) &&
        report.state === "succeeded"
      ) {
        selectedLocalId = asString(report.localCandidateId);
        selectedRemoteId = asString(report.remoteCandidateId);
      }
    });
  }

  const localCand = selectedLocalId
    ? (stats.get(selectedLocalId) as Record<string, unknown> | undefined)
    : undefined;
  const remoteCand = selectedRemoteId
    ? (stats.get(selectedRemoteId) as Record<string, unknown> | undefined)
    : undefined;

  const localType = asString(localCand?.candidateType);
  const remoteType = asString(remoteCand?.candidateType);
  const isRelay = localType === "relay" || remoteType === "relay";
  const addr =
    asString(remoteCand?.address) ||
    asString(remoteCand?.ip) ||
    asString(localCand?.address);
  const ipFamily = addr.includes(":") ? "ipv6" : addr ? "ipv4" : "";
  const candidateType =
    localType && remoteType
      ? `${localType}/${remoteType}`
      : localType || remoteType || "direct";

  return { isRelay, candidateType, ipFamily };
}

export async function negotiateDirectPeer(
  stunServers: string[],
  signal?: AbortSignal,
): Promise<NegotiatedSession> {
  signal?.throwIfAborted();
  if (typeof RTCPeerConnection === "undefined") {
    throw new Error("RTCPeerConnection unavailable");
  }
  const iceServers: RTCIceServer[] =
    stunServers.length > 0 ? [{ urls: stunServers }] : [];
  const pc = new RTCPeerConnection({ iceServers });
  let control: RTCDataChannel;
  try {
    control = pc.createDataChannel(CONTROL_LABEL, { ordered: true });
  } catch (err) {
    pc.close();
    throw err;
  }
  const localCandidates: P2PICECandidate[] = [];
  const work = new AbortController();
  let peerId = "";
  let closed = false;
  let negotiating = true;
  let resolveHello!: (meta: {
    candidateType: string;
    ipFamily: string;
  }) => void;
  let rejectHello!: (err: unknown) => void;
  const helloDone = new Promise<{ candidateType: string; ipFamily: string }>(
    (resolve, reject) => {
      resolveHello = resolve;
      rejectHello = reject;
    },
  );
  // A control error can arrive while offer/answer HTTP is still pending.
  void helloDone.catch(() => undefined);
  let rejectFailure!: (err: unknown) => void;
  const failure = new Promise<never>((_, reject) => {
    rejectFailure = reject;
  });
  const timer = setTimeout(
    () => fail(new Error("P2P handshake timeout")),
    NEGOTIATION_TIMEOUT_MS,
  );
  const abort = () => fail(signal?.reason);
  const stopNegotiating = () => {
    negotiating = false;
    clearTimeout(timer);
    signal?.removeEventListener("abort", abort);
    work.abort();
    pc.onicecandidate = null;
    control.onopen = null;
    localCandidates.length = 0;
  };
  const pendingPongs = new Map<
    number,
    {
      resolve: (rttMs: number) => void;
      reject: (err: Error) => void;
      sentAt: number;
      timer: ReturnType<typeof setTimeout>;
    }
  >();
  const notifyClose = () => {
    if (peerId) {
      void sendSignalHttp({ action: "close", peerId }).catch(() => undefined);
    }
  };
  const closeSession = (notifyServer = true) => {
    if (closed) return;
    closed = true;
    stopNegotiating();
    for (const pending of pendingPongs.values()) {
      clearTimeout(pending.timer);
      pending.reject(new Error("Control channel closed"));
    }
    pendingPongs.clear();
    control.onmessage = null;
    control.onclose = null;
    control.onerror = null;
    pc.onconnectionstatechange = null;
    try {
      control.close();
    } catch {
      // Already closed.
    }
    try {
      pc.close();
    } catch {
      // Already closed.
    }
    if (notifyServer) notifyClose();
  };
  function fail(err: unknown) {
    if (!negotiating) return;
    rejectHello(err);
    rejectFailure(err);
    closeSession();
  }
  const ensureNegotiating = () => {
    work.signal.throwIfAborted();
  };
  const sendPing = (): Promise<number> => {
    if (closed || control.readyState !== "open") {
      return Promise.reject(new Error("Control channel not open"));
    }
    const seq = ++pingSeq;
    const sentAt = performance.now();
    return new Promise<number>((resolve, reject) => {
      const timer = setTimeout(() => {
        pendingPongs.delete(seq);
        reject(new Error("P2P ping timeout"));
      }, PING_TIMEOUT_MS);
      pendingPongs.set(seq, { resolve, reject, sentAt, timer });
      try {
        control.send(
          JSON.stringify({
            type: "ping",
            seq,
            clientTs: Math.round(Date.now()),
          }),
        );
      } catch (err) {
        clearTimeout(timer);
        pendingPongs.delete(seq);
        reject(err);
      }
    });
  };
  pc.onicecandidate = (ev) => {
    if (!ev.candidate?.candidate || !negotiating) return;
    localCandidates.push({
      candidate: ev.candidate.candidate,
      sdpMid: ev.candidate.sdpMid,
      sdpMLineIndex: ev.candidate.sdpMLineIndex,
    });
  };
  pc.onconnectionstatechange = () => {
    if (pc.connectionState === "failed" || pc.connectionState === "closed") {
      if (negotiating) fail(new Error("P2P connection closed"));
      else closeSession();
    }
  };
  control.onclose = () => {
    if (negotiating) fail(new Error("P2P control channel closed"));
    else closeSession();
  };
  control.onerror = () => {
    if (negotiating) fail(new Error("P2P control channel error"));
    else closeSession();
  };
  control.onopen = () => {
    try {
      control.send(
        JSON.stringify({
          type: "hello",
          proto: P2P_PROTO_VERSION,
          appVersion: "1",
          features: ["stream-v1", "rpc-v1"],
        }),
      );
    } catch (err) {
      fail(err);
    }
  };
  control.onmessage = (ev) => {
    if (typeof ev.data !== "string") return;
    let parsed;
    try {
      parsed = P2PControlMessageSchema.parse(JSON.parse(ev.data));
    } catch {
      return;
    }
    if (parsed.type === "hello" && negotiating) {
      if (parsed.proto !== P2P_PROTO_VERSION) {
        fail(new Error(`Incompatible P2P proto ${parsed.proto}`));
        return;
      }
      resolveHello({
        candidateType: parsed.candidateType ?? "",
        ipFamily: parsed.ipFamily ?? "",
      });
    } else if (parsed.type === "pong") {
      const pending = pendingPongs.get(parsed.seq);
      if (pending) {
        clearTimeout(pending.timer);
        pendingPongs.delete(parsed.seq);
        pending.resolve(
          Math.max(0, Math.round(performance.now() - pending.sentAt)),
        );
      }
    }
  };
  signal?.addEventListener("abort", abort, { once: true });
  const applyCandidates = async (candidates: P2PICECandidate[]) => {
    for (const candidate of candidates) {
      ensureNegotiating();
      if (candidate.candidate) {
        await pc.addIceCandidate({
          candidate: candidate.candidate,
          sdpMid: candidate.sdpMid ?? null,
          sdpMLineIndex: candidate.sdpMLineIndex ?? null,
        });
      }
    }
    ensureNegotiating();
  };
  const exchangeCandidates = async () => {
    while (negotiating) {
      await delay(CANDIDATE_INTERVAL_MS, work.signal);
      const batch = localCandidates.splice(0);
      const response = await sendSignalHttp(
        {
          action: "candidate",
          peerId,
          ...(batch.length > 0 ? { candidates: batch } : {}),
        },
        work.signal,
      );
      await applyCandidates(response.candidates);
    }
  };
  const negotiate = async (): Promise<NegotiatedSession> => {
    ensureNegotiating();
    const offer = await pc.createOffer();
    ensureNegotiating();
    await pc.setLocalDescription(offer);
    ensureNegotiating();
    const response = await sendSignalHttp(
      { action: "offer", sdp: offer.sdp ?? "" },
      work.signal,
    );
    peerId = response.peerId;
    if (!negotiating) {
      // A transport may finish an offer despite aborting its request.
      notifyClose();
      ensureNegotiating();
    }
    await pc.setRemoteDescription({
      type: "answer",
      sdp: response.sdp ?? "",
    });
    await applyCandidates(response.candidates);
    // One serial pump owns both upload batches and every response's candidates.
    // Empty uploads keep fetching server candidates even after local gathering ends.
    void exchangeCandidates().catch((err: unknown) => {
      if (negotiating) fail(err);
    });
    const helloMeta = await helloDone;
    ensureNegotiating();
    const statsMeta = await inspectSelectedCandidatePair(pc);
    ensureNegotiating();
    if (statsMeta.isRelay) {
      const err = new Error("Relay candidate rejected");
      err.name = "RelayRejectedError";
      throw err;
    }
    stopNegotiating();
    return {
      peerId,
      pc,
      control,
      candidateType:
        statsMeta.candidateType !== "direct"
          ? statsMeta.candidateType
          : helloMeta.candidateType || "direct",
      ipFamily: statsMeta.ipFamily || helloMeta.ipFamily || "ipv4",
      sendPing,
      close: closeSession,
    };
  };
  try {
    if (signal?.aborted) abort();
    return await Promise.race([negotiate(), failure]);
  } catch (err) {
    closeSession();
    throw err;
  }
}
