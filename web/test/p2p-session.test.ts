import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  measureBaselineHttpRtt,
  negotiateDirectPeer,
} from "../src/platform/p2p-session";
import type { P2PICECandidate } from "../src/contracts/schemas";

const { http } = vi.hoisted(() => ({ http: vi.fn<typeof fetch>() }));
vi.mock("../src/platform/transport", () => ({
  defaultBrowserTransport: () => ({ fetch: http }),
}));
vi.mock("../src/platform/storage", () => ({ stored: () => "test-token" }));

interface SignalRequest {
  action: "offer" | "candidate" | "close";
  peerId?: string;
  candidates?: P2PICECandidate[];
  signal: AbortSignal;
  at: number;
}

function candidate(value: string): P2PICECandidate {
  return { candidate: value, sdpMid: "0", sdpMLineIndex: 0 };
}

function response(candidates: P2PICECandidate[] = []): Response {
  return Response.json({
    peerId: "peer-1",
    sdp: "answer-sdp",
    candidates,
    stunServers: [],
  });
}

class MockControl {
  readyState = "connecting";
  onopen: (() => void) | null = null;
  onmessage: ((event: { data: string }) => void) | null = null;
  onclose: (() => void) | null = null;
  onerror: (() => void) | null = null;
  send = vi.fn<(data: string) => void>();
  close = vi.fn(() => {
    this.readyState = "closed";
    this.onclose?.();
  });

  hello() {
    this.readyState = "open";
    this.onopen?.();
    this.onmessage?.({
      data: JSON.stringify({
        type: "hello",
        proto: 1,
        appVersion: "1",
        features: ["stream-v1", "rpc-v1"],
      }),
    });
  }
}

class MockPeer {
  static instances: MockPeer[] = [];
  static initialCandidates: string[] = [];
  readonly control = new MockControl();
  connectionState = "new";
  onicecandidate: ((event: { candidate: P2PICECandidate }) => void) | null =
    null;
  onconnectionstatechange: (() => void) | null = null;
  createOffer = vi.fn(async () => ({ type: "offer", sdp: "offer-sdp" }));
  setLocalDescription = vi.fn(async () => {
    for (const value of MockPeer.initialCandidates) this.emitCandidate(value);
  });
  setRemoteDescription = vi.fn(async () => undefined);
  addIceCandidate = vi
    .fn<(item: P2PICECandidate) => Promise<void>>()
    .mockResolvedValue(undefined);
  getStats = vi.fn(async () => new Map<string, Record<string, unknown>>());
  close = vi.fn(() => {
    this.connectionState = "closed";
    this.onconnectionstatechange?.();
  });

  constructor() {
    MockPeer.instances.push(this);
  }

  createDataChannel() {
    return this.control;
  }

  emitCandidate(value: string) {
    this.onicecandidate?.({ candidate: candidate(value) });
  }
}

describe("P2P session negotiation", () => {
  let requests: SignalRequest[];
  let handle: (request: SignalRequest) => Response | Promise<Response>;

  beforeEach(() => {
    vi.useFakeTimers({
      toFake: ["Date", "setTimeout", "clearTimeout", "performance"],
    });
    MockPeer.instances = [];
    MockPeer.initialCandidates = [];
    vi.stubGlobal("RTCPeerConnection", MockPeer);
    requests = [];
    handle = () => response();
    http.mockReset();
    http.mockImplementation(async (_input, init) => {
      if (!init?.signal) throw new Error("Missing request signal");
      const request: SignalRequest = {
        ...JSON.parse(String(init.body)),
        signal: init.signal,
        at: Date.now(),
      };
      requests.push(request);
      return handle(request);
    });
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it("serializes candidate responses and batches candidates gathered during requests", async () => {
    MockPeer.initialCandidates = ["local-early-1", "local-early-2"];
    const firstReply = Promise.withResolvers<Response>();
    const applied = Promise.withResolvers<void>();
    let candidateRequests = 0;
    handle = (request) => {
      if (request.action === "offer") return response([candidate("answer")]);
      if (request.action === "candidate") {
        candidateRequests++;
        return candidateRequests === 1
          ? firstReply.promise
          : response([candidate("second-reply")]);
      }
      return response();
    };
    const pending = negotiateDirectPeer([]);
    const pc = MockPeer.instances[0]!;
    pc.addIceCandidate.mockImplementation(async (item) => {
      if (item.candidate === "first-reply-1") await applied.promise;
    });
    await vi.advanceTimersByTimeAsync(0);
    expect(
      pc.addIceCandidate.mock.calls.map(([item]) => item.candidate),
    ).toEqual(["answer"]);
    await vi.advanceTimersByTimeAsync(1_000);
    expect(requests[1]?.candidates).toEqual([
      candidate("local-early-1"),
      candidate("local-early-2"),
    ]);
    pc.emitCandidate("local-during-request-1");
    pc.emitCandidate("local-during-request-2");
    await vi.advanceTimersByTimeAsync(2_000);
    expect(candidateRequests).toBe(1);
    firstReply.resolve(
      response([candidate("first-reply-1"), candidate("first-reply-2")]),
    );
    await vi.advanceTimersByTimeAsync(0);
    await vi.advanceTimersByTimeAsync(2_000);
    expect(candidateRequests).toBe(1);
    expect(
      pc.addIceCandidate.mock.calls.map(([item]) => item.candidate),
    ).toEqual(["answer", "first-reply-1"]);
    applied.resolve();
    await vi.advanceTimersByTimeAsync(0);
    await vi.advanceTimersByTimeAsync(1_000);
    expect(requests[2]?.candidates).toEqual([
      candidate("local-during-request-1"),
      candidate("local-during-request-2"),
    ]);
    expect(
      pc.addIceCandidate.mock.calls.map(([item]) => item.candidate),
    ).toEqual(["answer", "first-reply-1", "first-reply-2", "second-reply"]);
    pc.control.hello();
    const session = await pending;
    expect(pc.getStats).toHaveBeenCalledOnce();
    expect(vi.getTimerCount()).toBe(0);
    await vi.advanceTimersByTimeAsync(10_000);
    expect(candidateRequests).toBe(2);
    session.close(false);
  });

  it("pulls late server candidates without any new browser candidates", async () => {
    let polls = 0;
    handle = (request) => {
      if (request.action === "candidate" && ++polls === 2) {
        return response([candidate("late-server")]);
      }
      return response();
    };
    const pending = negotiateDirectPeer([]);
    await vi.advanceTimersByTimeAsync(2_000);
    expect(polls).toBe(2);
    const pulls = requests.filter((request) => request.action === "candidate");
    expect(pulls.every((request) => !("candidates" in request))).toBe(true);
    expect(pulls[1]!.at - pulls[0]!.at).toBeGreaterThanOrEqual(1_000);
    const pc = MockPeer.instances[0]!;
    expect(pc.addIceCandidate).toHaveBeenCalledWith(candidate("late-server"));
    pc.control.hello();
    const session = await pending;
    expect(vi.getTimerCount()).toBe(0);
    session.close(false);
  });

  it("accepts a valid direct handshake taking longer than eight seconds", async () => {
    const pending = negotiateDirectPeer(["stun:example.test"]);
    await vi.advanceTimersByTimeAsync(12_000);
    const pc = MockPeer.instances[0]!;
    expect(pc.close).not.toHaveBeenCalled();
    pc.control.hello();
    const session = await pending;
    expect(session.peerId).toBe("peer-1");
    expect(vi.getTimerCount()).toBe(0);
    session.close(false);
  });

  it("times out at thirty seconds, closes both sides, and stops polling", async () => {
    const rejected = expect(negotiateDirectPeer([])).rejects.toThrow(
      "P2P handshake timeout",
    );
    await vi.advanceTimersByTimeAsync(30_000);
    await rejected;
    const pc = MockPeer.instances[0]!;
    expect(pc.close).toHaveBeenCalledOnce();
    expect(pc.control.close).toHaveBeenCalledOnce();
    expect(
      requests.filter((request) => request.action === "close"),
    ).toHaveLength(1);
    const count = requests.length;
    expect(vi.getTimerCount()).toBe(0);
    await vi.advanceTimersByTimeAsync(60_000);
    expect(requests).toHaveLength(count);
    expect(pc.onicecandidate).toBeNull();
    expect(pc.control.onopen).toBeNull();
  });

  it("aborts an in-flight candidate request and does not keep signaling", async () => {
    handle = (request) => {
      if (request.action !== "candidate") return response();
      const { promise, reject } = Promise.withResolvers<Response>();
      request.signal.addEventListener(
        "abort",
        () => reject(request.signal.reason),
        {
          once: true,
        },
      );
      return promise;
    };
    const abort = new AbortController();
    const pending = negotiateDirectPeer([], abort.signal);
    const rejected = expect(pending).rejects.toThrow("network changed");
    await vi.advanceTimersByTimeAsync(1_000);
    const request = requests.find((item) => item.action === "candidate")!;
    abort.abort(new Error("network changed"));
    await rejected;
    await vi.advanceTimersByTimeAsync(0);
    expect(request.signal.aborted).toBe(true);
    expect(MockPeer.instances[0]!.close).toHaveBeenCalledOnce();
    expect(requests.filter((item) => item.action === "close")).toHaveLength(1);
    const count = requests.length;
    expect(vi.getTimerCount()).toBe(0);
    await vi.advanceTimersByTimeAsync(60_000);
    expect(requests).toHaveLength(count);
  });

  it("does not construct a peer for an already aborted negotiation", async () => {
    const abort = new AbortController();
    abort.abort();
    await expect(negotiateDirectPeer([], abort.signal)).rejects.toMatchObject({
      name: "AbortError",
    });
    expect(MockPeer.instances).toHaveLength(0);
    expect(http).not.toHaveBeenCalled();
    expect(vi.getTimerCount()).toBe(0);
  });

  it("closes a late server peer when the offer transport ignores cancellation", async () => {
    const offer = Promise.withResolvers<Response>();
    handle = (request) =>
      request.action === "offer" ? offer.promise : response();
    const abort = new AbortController();
    const pending = negotiateDirectPeer([], abort.signal);
    const rejected = expect(pending).rejects.toThrow("cancelled");
    await vi.advanceTimersByTimeAsync(0);
    abort.abort(new Error("cancelled"));
    await rejected;
    const pc = MockPeer.instances[0]!;
    expect(pc.close).toHaveBeenCalledOnce();
    expect(vi.getTimerCount()).toBe(0);
    offer.resolve(response());
    await vi.advanceTimersByTimeAsync(0);
    expect(requests.map((request) => request.action)).toEqual([
      "offer",
      "close",
    ]);
    expect(pc.setRemoteDescription).not.toHaveBeenCalled();
    expect(vi.getTimerCount()).toBe(0);
  });

  it("aborts pending polling on success and ignores its late response", async () => {
    const poll = Promise.withResolvers<Response>();
    handle = (request) =>
      request.action === "candidate" ? poll.promise : response();
    const pending = negotiateDirectPeer([]);
    await vi.advanceTimersByTimeAsync(1_000);
    const request = requests.find((item) => item.action === "candidate")!;
    const pc = MockPeer.instances[0]!;
    pc.control.hello();
    const session = await pending;
    expect(request.signal.aborted).toBe(true);
    expect(vi.getTimerCount()).toBe(0);
    poll.resolve(response([candidate("too-late")]));
    await vi.advanceTimersByTimeAsync(0);
    expect(pc.addIceCandidate).not.toHaveBeenCalled();
    expect(pc.close).not.toHaveBeenCalled();
    session.close(false);
  });

  it("cancels while selected-pair inspection is pending without publishing a session", async () => {
    const stats = Promise.withResolvers<Map<string, Record<string, unknown>>>();
    const abort = new AbortController();
    const pending = negotiateDirectPeer([], abort.signal);
    const rejected = expect(pending).rejects.toThrow("cancel stats");
    await vi.advanceTimersByTimeAsync(0);
    const pc = MockPeer.instances[0]!;
    pc.getStats.mockReturnValue(stats.promise);
    pc.control.hello();
    await vi.advanceTimersByTimeAsync(0);
    expect(pc.getStats).toHaveBeenCalledOnce();
    abort.abort(new Error("cancel stats"));
    await rejected;
    stats.resolve(new Map());
    await vi.advanceTimersByTimeAsync(0);
    expect(pc.close).toHaveBeenCalledOnce();
    expect(vi.getTimerCount()).toBe(0);
  });

  it("observes control errors promptly while an offer is still pending", async () => {
    const offer = Promise.withResolvers<Response>();
    handle = (request) =>
      request.action === "offer" ? offer.promise : response();
    const pending = negotiateDirectPeer([]);
    const rejected = expect(pending).rejects.toThrow(
      "P2P control channel error",
    );
    await vi.advanceTimersByTimeAsync(0);
    MockPeer.instances[0]!.control.onerror?.();
    await rejected;
    offer.resolve(response());
    await vi.advanceTimersByTimeAsync(0);
    expect(MockPeer.instances[0]!.close).toHaveBeenCalledOnce();
    expect(requests.map((request) => request.action)).toEqual([
      "offer",
      "close",
    ]);
    expect(vi.getTimerCount()).toBe(0);
  });

  it("fails immediately on candidate signaling errors without leaving a hello timer", async () => {
    handle = (request) =>
      request.action === "candidate"
        ? new Response(null, { status: 500 })
        : response();
    const rejected = expect(negotiateDirectPeer([])).rejects.toThrow(
      "P2P signal failed (500)",
    );
    await vi.advanceTimersByTimeAsync(1_000);
    await rejected;
    expect(MockPeer.instances[0]!.close).toHaveBeenCalledOnce();
    expect(vi.getTimerCount()).toBe(0);
  });

  it("retains ping, close(false), and selected-pair relay rejection semantics", async () => {
    const pending = negotiateDirectPeer([]);
    await vi.advanceTimersByTimeAsync(0);
    const pc = MockPeer.instances[0]!;
    pc.control.hello();
    const session = await pending;
    const pong = session.sendPing();
    const ping = JSON.parse(pc.control.send.mock.lastCall![0]);
    await vi.advanceTimersByTimeAsync(25);
    pc.control.onmessage?.({
      data: JSON.stringify({
        type: "pong",
        seq: ping.seq,
        clientTs: ping.clientTs,
      }),
    });
    await expect(pong).resolves.toBe(25);
    session.close(false);
    await expect(session.sendPing()).rejects.toThrow(
      "Control channel not open",
    );
    expect(requests.some((request) => request.action === "close")).toBe(false);

    const relay = negotiateDirectPeer([]);
    const rejected = expect(relay).rejects.toMatchObject({
      name: "RelayRejectedError",
    });
    await vi.advanceTimersByTimeAsync(0);
    const relayPC = MockPeer.instances[1]!;
    relayPC.getStats.mockResolvedValue(
      new Map<string, Record<string, unknown>>([
        [
          "pair",
          {
            type: "candidate-pair",
            selected: true,
            state: "succeeded",
            localCandidateId: "local",
          },
        ],
        ["local", { candidateType: "relay" }],
      ]),
    );
    relayPC.control.hello();
    await rejected;
    await vi.advanceTimersByTimeAsync(0);
    expect(relayPC.close).toHaveBeenCalledOnce();
    expect(vi.getTimerCount()).toBe(0);
  });

  it("cancels baseline HTTP sampling and never starts another sample", async () => {
    const requestSignals: AbortSignal[] = [];
    http.mockImplementation((_input, init) => {
      const requestSignal = init?.signal;
      if (!requestSignal) throw new Error("Missing baseline signal");
      requestSignals.push(requestSignal);
      const { promise, reject } = Promise.withResolvers<Response>();
      requestSignal.addEventListener(
        "abort",
        () => reject(requestSignal.reason),
        {
          once: true,
        },
      );
      return promise;
    });
    const abort = new AbortController();
    const pending = measureBaselineHttpRtt(3, abort.signal);
    const rejected = expect(pending).rejects.toThrow("cancel baseline");
    abort.abort(new Error("cancel baseline"));
    await rejected;
    expect(requestSignals[0]!.aborted).toBe(true);
    expect(http).toHaveBeenCalledOnce();
    expect(vi.getTimerCount()).toBe(0);
  });
});
