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
const NEGOTIATION_TIMEOUT_MS = 8_000;

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

async function sendSignalHttp(payload: {
  action: "offer" | "candidate" | "close";
  peerId?: string;
  sdp?: string;
  candidates?: P2PICECandidate[];
}) {
  const res = await defaultBrowserTransport().fetch("/p2p/signal", {
    method: "POST",
    headers: {
      Authorization: "Bearer " + stored("token"),
      "Content-Type": "application/json",
    },
    body: JSON.stringify(payload),
    signal: AbortSignal.timeout(NEGOTIATION_TIMEOUT_MS),
  });
  if (!res.ok) {
    throw new Error(`P2P signal failed (${res.status})`);
  }
  return P2PSignalResponseSchema.parse(await res.json());
}

export async function measureBaselineHttpRtt(count = 3): Promise<number[]> {
  const samples: number[] = [];
  for (let i = 0; i < count; i++) {
    const started = performance.now();
    const res = await defaultBrowserTransport().fetch(
      `/p2p/ping?seq=${++pingSeq}`,
      {
        method: "GET",
        headers: { Authorization: "Bearer " + stored("token") },
        signal: AbortSignal.timeout(PING_TIMEOUT_MS),
      },
    );
    if (!res.ok) throw new Error(`HTTP ping failed (${res.status})`);
    P2PPingResponseSchema.parse(await res.json());
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
): Promise<NegotiatedSession> {
  if (typeof RTCPeerConnection === "undefined") {
    throw new Error("RTCPeerConnection unavailable");
  }
  const iceServers: RTCIceServer[] =
    stunServers.length > 0 ? [{ urls: stunServers }] : [];
  const pc = new RTCPeerConnection({ iceServers });
  const earlyCandidates: P2PICECandidate[] = [];
  let peerId = "";
  let closed = false;

  const pendingPongs = new Map<
    number,
    {
      resolve: (rttMs: number) => void;
      reject: (err: Error) => void;
      sentAt: number;
      timer: ReturnType<typeof setTimeout>;
    }
  >();

  const clearPongs = () => {
    for (const p of pendingPongs.values()) {
      clearTimeout(p.timer);
      p.reject(new Error("Control channel closed"));
    }
    pendingPongs.clear();
  };

  pc.onicecandidate = (ev) => {
    if (!ev.candidate || !ev.candidate.candidate || closed) return;
    const item: P2PICECandidate = {
      candidate: ev.candidate.candidate,
      sdpMid: ev.candidate.sdpMid,
      sdpMLineIndex: ev.candidate.sdpMLineIndex,
    };
    if (!peerId) {
      earlyCandidates.push(item);
    } else {
      void sendSignalHttp({
        action: "candidate",
        peerId,
        candidates: [item],
      }).catch(() => undefined);
    }
  };

  const control = pc.createDataChannel(CONTROL_LABEL, { ordered: true });

  const closeSession = (notifyServer = true) => {
    if (closed) return;
    closed = true;
    clearPongs();
    control.onmessage = null;
    control.onclose = null;
    control.onerror = null;
    pc.onconnectionstatechange = null;
    pc.onicecandidate = null;
    try {
      control.close();
    } catch {
      // ignore
    }
    try {
      pc.close();
    } catch {
      // ignore
    }
    if (notifyServer && peerId) {
      void sendSignalHttp({ action: "close", peerId }).catch(() => undefined);
    }
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
      control.send(
        JSON.stringify({
          type: "ping",
          seq,
          clientTs: Math.round(Date.now()),
        }),
      );
    });
  };

  const helloDone = new Promise<{ candidateType: string; ipFamily: string }>(
    (resolve, reject) => {
      const timer = setTimeout(() => {
        reject(new Error("P2P handshake timeout"));
      }, NEGOTIATION_TIMEOUT_MS);

      control.onerror = () => {
        clearTimeout(timer);
        reject(new Error("P2P control channel error"));
      };
      control.onopen = () => {
        control.send(
          JSON.stringify({
            type: "hello",
            proto: P2P_PROTO_VERSION,
            appVersion: "1",
            features: ["stream-v1", "rpc-v1"],
          }),
        );
      };
      control.onmessage = (ev) => {
        if (typeof ev.data !== "string") return;
        let parsed;
        try {
          parsed = P2PControlMessageSchema.parse(JSON.parse(ev.data));
        } catch {
          return;
        }
        if (parsed.type === "hello") {
          clearTimeout(timer);
          if (parsed.proto !== P2P_PROTO_VERSION) {
            reject(new Error(`Incompatible P2P proto ${parsed.proto}`));
            return;
          }
          resolve({
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
    },
  );

  try {
    const offer = await pc.createOffer();
    await pc.setLocalDescription(offer);
    const sig = await sendSignalHttp({
      action: "offer",
      sdp: offer.sdp ?? "",
    });
    peerId = sig.peerId;

    await pc.setRemoteDescription({
      type: "answer",
      sdp: sig.sdp ?? "",
    });
    for (const c of sig.candidates) {
      if (c.candidate) {
        await pc.addIceCandidate({
          candidate: c.candidate,
          sdpMid: c.sdpMid ?? null,
          sdpMLineIndex: c.sdpMLineIndex ?? null,
        });
      }
    }
    if (earlyCandidates.length > 0) {
      const batch = earlyCandidates.splice(0, earlyCandidates.length);
      const more = await sendSignalHttp({
        action: "candidate",
        peerId,
        candidates: batch,
      });
      for (const c of more.candidates) {
        if (c.candidate) {
          await pc.addIceCandidate({
            candidate: c.candidate,
            sdpMid: c.sdpMid ?? null,
            sdpMLineIndex: c.sdpMLineIndex ?? null,
          });
        }
      }
    }

    const helloMeta = await helloDone;
    const statsMeta = await inspectSelectedCandidatePair(pc);
    if (statsMeta.isRelay) {
      closeSession(true);
      const relayErr = new Error("Relay candidate rejected");
      relayErr.name = "RelayRejectedError";
      throw relayErr;
    }

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
  } catch (err) {
    closeSession(true);
    throw err;
  }
}
