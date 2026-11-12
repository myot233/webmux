import { describe, expect, it, vi } from "vitest";
import {
  StreamFrameReassembler,
  encodeStreamCloseFrame,
  encodeStreamFrames,
  isLocalOrOverlayHost,
  medianSample,
  shouldUpgradeToP2P,
} from "../src/platform/p2p-framing";
import { inspectSelectedCandidatePair } from "../src/platform/p2p-session";
import { createP2PTransport } from "../src/platform/p2p-transport";
import type { ClientTransport } from "../src/platform/transport";

class MockDataChannel extends EventTarget {
  readyState: RTCDataChannelState = "connecting";
  binaryType: BinaryType = "arraybuffer";
  bufferedAmount = 0;
  bufferedAmountLowThreshold = 0;
  onopen: (() => void) | null = null;
  onmessage: ((event: { data: string | ArrayBuffer }) => void) | null = null;
  onclose: (() => void) | null = null;
  onerror: (() => void) | null = null;
  readonly sent: (string | Uint8Array)[] = [];

  constructor(readonly label: string) {
    super();
  }

  open() {
    this.readyState = "open";
    this.onopen?.();
  }

  send(data: string | Uint8Array | ArrayBuffer) {
    if (typeof data === "string") {
      this.sent.push(data);
    } else if (data instanceof Uint8Array) {
      const copy = new Uint8Array(data.byteLength);
      copy.set(data);
      this.sent.push(copy);
    } else {
      this.sent.push(new Uint8Array(data.slice(0)));
    }
  }

  receive(data: string | ArrayBuffer | Uint8Array) {
    if (typeof data === "string") {
      this.onmessage?.({ data });
    } else if (data instanceof Uint8Array) {
      const copy = new Uint8Array(data.byteLength);
      copy.set(data);
      this.onmessage?.({ data: copy.buffer });
    } else {
      this.onmessage?.({ data });
    }
  }

  close() {
    if (this.readyState === "closed") return;
    this.readyState = "closed";
    this.onclose?.();
  }
}

class MockPeerConnection {
  connectionState: RTCPeerConnectionState = "connected";
  readonly channels: MockDataChannel[] = [];

  createDataChannel(label: string) {
    const ch = new MockDataChannel(label);
    this.channels.push(ch);
    return ch as unknown as RTCDataChannel;
  }
}

describe("P2P host locality and hysteresis predicates", () => {
  it("identifies loopback, RFC1918 LAN, IPv6 ULA/link-local, and Tailscale hosts", () => {
    expect(isLocalOrOverlayHost("localhost")).toBe(true);
    expect(isLocalOrOverlayHost("127.0.0.1")).toBe(true);
    expect(isLocalOrOverlayHost("[::1]")).toBe(true);
    expect(isLocalOrOverlayHost("192.168.1.50")).toBe(true);
    expect(isLocalOrOverlayHost("10.0.4.12")).toBe(true);
    expect(isLocalOrOverlayHost("172.20.0.1")).toBe(true);
    expect(isLocalOrOverlayHost("100.101.102.103")).toBe(true);
    expect(isLocalOrOverlayHost("devbox.tail1234.ts.net")).toBe(true);
    expect(isLocalOrOverlayHost("fd7a:115c:a1e0::1")).toBe(true);

    expect(isLocalOrOverlayHost("term.example.com")).toBe(false);
    expect(isLocalOrOverlayHost("203.0.113.10")).toBe(false);
    expect(isLocalOrOverlayHost("172.32.0.1")).toBe(false);
    expect(isLocalOrOverlayHost("100.63.255.1")).toBe(false);
  });

  it("applies strict 15ms floor and 10ms / 15% hysteresis before upgrading", () => {
    expect(shouldUpgradeToP2P(12, 1)).toBe(false);
    expect(shouldUpgradeToP2P(80, 72)).toBe(false);
    expect(shouldUpgradeToP2P(80, 65)).toBe(true);
    expect(shouldUpgradeToP2P(40, 28)).toBe(true);
    expect(medianSample([90, 20, 50])).toBe(50);
  });
});

describe("P2P stream framing & close codes", () => {
  it("fragments and reassembles >64 KiB binary payloads and decodes 4410 close frame", () => {
    const large = new Uint8Array(100 * 1024);
    for (let i = 0; i < large.length; i++) large[i] = (i * 13) & 0xff;

    const frames = encodeStreamFrames(large);
    expect(frames.length).toBe(4); // 32 + 32 + 32 + 4 KiB

    const reassembler = new StreamFrameReassembler();
    expect(reassembler.feed(frames[0]!)).toBeNull();
    expect(reassembler.feed(frames[1]!)).toBeNull();
    expect(reassembler.feed(frames[2]!)).toBeNull();
    const done = reassembler.feed(frames[3]!);
    expect(done?.kind).toBe("message");
    if (done?.kind === "message") {
      expect(new Uint8Array(done.data as ArrayBuffer)).toEqual(large);
    }

    const closeFrame = encodeStreamCloseFrame(4410, "session exited");
    const closed = reassembler.feed(closeFrame);
    expect(closed).toEqual({
      kind: "close",
      code: 4410,
      reason: "session exited",
    });
  });

  it("rejects relay ICE candidates via RTCStatsReport inspection", async () => {
    const fakeStats = new Map<string, Record<string, unknown>>([
      ["t1", { type: "transport", selectedCandidatePairId: "pair1" }],
      [
        "pair1",
        {
          type: "candidate-pair",
          localCandidateId: "loc1",
          remoteCandidateId: "rem1",
          selected: true,
          state: "succeeded",
        },
      ],
      ["loc1", { candidateType: "srflx", address: "198.51.100.5" }],
      ["rem1", { candidateType: "relay", address: "203.0.113.9" }],
    ]);
    const pc = {
      getStats: async () => fakeStats,
    } as unknown as RTCPeerConnection;

    const res = await inspectSelectedCandidatePair(pc);
    expect(res.isRelay).toBe(true);
    expect(res.candidateType).toBe("srflx/relay");
    expect(res.ipFamily).toBe("ipv4");
  });
});

describe("P2P ClientTransport streams, RPC fetch, and multipart upload", () => {
  it("strips compress=1 and adds zreplay=1 on terminal streams", () => {
    const mockPC = new MockPeerConnection();
    const fallback: ClientTransport = {
      origin: "https://term.example.com",
      fetch: vi.fn(),
      upload: vi.fn(),
      openStream: vi.fn(),
    };
    const transport = createP2PTransport({
      pc: mockPC as unknown as RTCPeerConnection,
      fallback,
    });

    const sock = transport.openStream("/ws/s/abc?client=c-1&compress=1", {
      token: "secret",
      compress: true,
    });
    expect(mockPC.channels).toHaveLength(1);
    expect(mockPC.channels[0]!.label).toBe(
      "stream:/ws/s/abc?client=c-1&zreplay=1",
    );

    const opened = vi.fn();
    const received: (string | ArrayBuffer)[] = [];
    const closed = vi.fn();
    sock.onopen = opened;
    sock.onmessage = (ev) => received.push(ev.data);
    sock.onclose = closed;

    const dc = mockPC.channels[0]!;
    dc.open();
    expect(opened).toHaveBeenCalledOnce();

    for (const chunk of encodeStreamFrames("hello p2p")) {
      dc.receive(chunk);
    }
    expect(received).toEqual(["hello p2p"]);

    dc.receive(encodeStreamCloseFrame(4404, "not found"));
    expect(closed).toHaveBeenCalledWith({ code: 4404, reason: "not found" });
  });

  it("carries /api/* JSON requests and FormData uploads over rpc:<id> DataChannels", async () => {
    const mockPC = new MockPeerConnection();
    const fallbackFetch = vi.fn(
      async () => new Response('{"seq":1,"serverTs":100}'),
    );
    const fallback: ClientTransport = {
      origin: "https://term.example.com",
      fetch: fallbackFetch,
      upload: vi.fn(),
      openStream: vi.fn(),
    };
    const transport = createP2PTransport({
      pc: mockPC as unknown as RTCPeerConnection,
      fallback,
    });

    // /p2p/* bypasses DataChannel and uses baseline HTTP
    await transport.fetch("/p2p/ping?seq=1", { method: "GET" });
    expect(fallbackFetch).toHaveBeenCalledOnce();
    expect(mockPC.channels).toHaveLength(0);

    // /sessions goes over rpc:1 DataChannel
    const fetchPromise = transport.fetch("/sessions", {
      method: "POST",
      headers: {
        Authorization: "Bearer tok",
        "Content-Type": "application/json",
      },
      body: JSON.stringify({ name: "dev" }),
    });

    await vi.waitFor(() => expect(mockPC.channels).toHaveLength(1));
    const rpcCh = mockPC.channels[0]!;
    expect(rpcCh.label).toBe("rpc:1");
    rpcCh.open();

    await vi.waitFor(() => expect(rpcCh.sent.length).toBeGreaterThanOrEqual(3));
    const reqHdr = JSON.parse(rpcCh.sent[0] as string);
    expect(reqHdr.method).toBe("POST");
    expect(reqHdr.path).toBe("/api/sessions");
    expect(reqHdr.headers.authorization).toBe("Bearer tok");

    rpcCh.receive(
      JSON.stringify({
        status: 201,
        statusText: "Created",
        headers: { "content-type": "application/json" },
      }),
    );
    rpcCh.receive(new TextEncoder().encode('{"id":"s1"}'));
    rpcCh.receive(new Uint8Array(0));

    const resp = await fetchPromise;
    expect(resp.status).toBe(201);
    expect(await resp.json()).toEqual({ id: "s1" });

    // Upload with progress
    const form = new FormData();
    form.append(
      "file",
      new Blob(["hello upload"], { type: "text/plain" }),
      "note.txt",
    );
    const progresses: number[] = [];
    const uploadPromise = transport.upload({
      path: "/uploads",
      form,
      token: "tok",
      timeoutMs: 5000,
      signal: new AbortController().signal,
      onProgress: (p) => progresses.push(p),
    });

    await vi.waitFor(() => expect(mockPC.channels).toHaveLength(2));
    const upCh = mockPC.channels[1]!;
    upCh.open();
    await vi.waitFor(() => expect(upCh.sent.length).toBeGreaterThanOrEqual(3));
    const upHdr = JSON.parse(upCh.sent[0] as string);
    expect(upHdr.headers["Content-Type"]).toContain(
      "multipart/form-data; boundary=",
    );

    upCh.receive(
      JSON.stringify({
        status: 200,
        statusText: "OK",
        headers: { "content-type": "application/json" },
      }),
    );
    upCh.receive(new TextEncoder().encode('{"path":"/tmp/note.txt"}'));
    upCh.receive(new Uint8Array(0));

    const upResult = await uploadPromise;
    expect(upResult.status).toBe(200);
    expect(upResult.body).toEqual({ path: "/tmp/note.txt" });
    expect(progresses).toContain(100);
  });
});
