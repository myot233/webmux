import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  resolveAppendOverlap,
  trimOverlapChunk,
} from "../src/domains/terminal/terminal-protocol";
import {
  SOCKET_CLOSED,
  SOCKET_CONNECTING,
  SOCKET_OPEN,
  setClientTransport,
  type ClientTransport,
  type StreamOptions,
  type TransportSocket,
} from "../src/platform/transport";
import { connect, handoffLiveSockets } from "../src/platform/ws";

class MockSocket implements TransportSocket {
  readyState = SOCKET_CONNECTING;
  bufferedAmount = 0;
  onopen: (() => void) | null = null;
  onmessage: ((event: { data: string | ArrayBuffer }) => void) | null = null;
  onclose: ((event: { code: number; reason?: string }) => void) | null = null;
  onerror: (() => void) | null = null;
  readonly sent: (string | Uint8Array)[] = [];
  closed = false;

  constructor(
    readonly label: string,
    readonly path: string,
    readonly options: StreamOptions,
  ) {}

  open() {
    this.readyState = SOCKET_OPEN;
    this.onopen?.();
  }

  fail(code = 1006) {
    this.readyState = SOCKET_CLOSED;
    this.onerror?.();
    this.onclose?.({ code });
  }

  deliverText(text: string) {
    this.onmessage?.({ data: text });
  }

  send(data: string | Uint8Array) {
    this.sent.push(typeof data === "string" ? data : data.slice());
  }

  close() {
    this.readyState = SOCKET_CLOSED;
    this.closed = true;
  }
}

function makeTransport(name: string, sink: MockSocket[]): ClientTransport {
  return {
    origin: "https://term.example.com",
    fetch: vi.fn(),
    upload: vi.fn(),
    openStream(path, options) {
      const sock = new MockSocket(name, path, options);
      sink.push(sock);
      return sock;
    },
  };
}

describe("Make-before-break transport handoff", () => {
  const cleanups: (() => void)[] = [];

  beforeEach(() => {
    vi.stubGlobal("localStorage", {
      getItem: () => "token-1",
      setItem: () => undefined,
      removeItem: () => undefined,
    });
    vi.stubGlobal("document", new EventTarget());
    vi.stubGlobal(
      "window",
      Object.assign(new EventTarget(), { setTimeout: globalThis.setTimeout }),
    );
  });

  afterEach(() => {
    while (cleanups.length > 0) cleanups.pop()?.();
    setClientTransport(null);
    vi.unstubAllGlobals();
  });

  it("keeps the baseline socket receiving until the P2P candidate opens, without state flicker", () => {
    const wsSockets: MockSocket[] = [];
    const p2pSockets: MockSocket[] = [];
    setClientTransport(makeTransport("ws", wsSockets));

    const states: string[] = [];
    const messages: string[] = [];
    const conn = connect(
      "/ws/sessions",
      (data) => messages.push(String(data)),
      (st) => states.push(st),
      vi.fn(),
    );
    cleanups.push(() => conn.close());

    expect(wsSockets).toHaveLength(1);
    wsSockets[0]!.open();
    wsSockets[0]!.deliverText("from-ws-1");
    expect(states).toEqual(["connecting", "connected"]);

    // Switch active transport to P2P and trigger make-before-break handoff.
    setClientTransport(makeTransport("p2p", p2pSockets));
    handoffLiveSockets();
    expect(p2pSockets).toHaveLength(1);
    expect(wsSockets[0]!.closed).toBe(false);
    wsSockets[0]!.deliverText("from-ws-during-handoff");

    // Candidate P2P DataChannel opens -> old WS closes cleanly with zero "reconnecting" state!
    p2pSockets[0]!.open();
    expect(wsSockets[0]!.closed).toBe(true);
    expect(states).toEqual(["connecting", "connected", "connected"]);
    expect(states).not.toContain("reconnecting");

    p2pSockets[0]!.deliverText("from-p2p-1");
    expect(messages).toEqual([
      "from-ws-1",
      "from-ws-during-handoff",
      "from-p2p-1",
    ]);

    conn.close();
  });

  it("retains the healthy baseline socket when a candidate P2P stream fails before opening", () => {
    const wsSockets: MockSocket[] = [];
    const p2pSockets: MockSocket[] = [];
    setClientTransport(makeTransport("ws", wsSockets));

    const states: string[] = [];
    const messages: string[] = [];
    const conn = connect(
      "/ws/sessions",
      (data) => messages.push(String(data)),
      (st) => states.push(st),
      vi.fn(),
    );
    cleanups.push(() => conn.close());

    wsSockets[0]!.open();
    setClientTransport(makeTransport("p2p", p2pSockets));
    handoffLiveSockets();

    expect(p2pSockets).toHaveLength(1);
    p2pSockets[0]!.fail(1006);

    expect(wsSockets[0]!.closed).toBe(false);
    expect(states).toEqual(["connecting", "connected"]);
    wsSockets[0]!.deliverText("still-alive-on-ws");
    expect(messages).toEqual(["still-alive-on-ws"]);

    conn.close();
  });

  it("computes overlap skip and trims duplicate bytes across multi-chunk append replay", () => {
    const generation = "0123456789abcdef";

    // Exact match: no bytes to skip, tail exists.
    expect(
      resolveAppendOverlap(true, generation, 5, {
        generation,
        start: 5,
        end: 11,
      }),
    ).toEqual({ accept: true, skipBytes: 0, hasTail: true });

    // Partial overlap during make-before-break: baseline advanced to 8 while candidate started at 5.
    const decision = resolveAppendOverlap(true, generation, 8, {
      generation,
      start: 5,
      end: 11,
    });
    expect(decision).toEqual({ accept: true, skipBytes: 3, hasTail: true });

    const encoder = new TextEncoder();
    const decoder = new TextDecoder();
    // Replay arrives in two chunks: "_W" (2 bytes, fully skipped) then "ORLD" (1 byte skipped -> "RLD")
    const c1 = trimOverlapChunk(encoder.encode("_W"), 3);
    expect(c1).toEqual({ remainingSkip: 1, slice: null });
    const c2 = trimOverlapChunk(encoder.encode("ORLD"), c1.remainingSkip);
    expect(c2.remainingSkip).toBe(0);
    expect(decoder.decode(c2.slice!)).toBe("RLD");

    // Full overlap (baseline caught up to candidate's end=11): accept, skip 6, no tail.
    expect(
      resolveAppendOverlap(true, generation, 11, {
        generation,
        start: 5,
        end: 11,
      }),
    ).toEqual({ accept: true, skipBytes: 6, hasTail: false });

    // Rejects mismatched generation, unsynced state, or gap (start > currentOffset).
    expect(
      resolveAppendOverlap(false, generation, 5, {
        generation,
        start: 5,
        end: 11,
      }),
    ).toEqual({ accept: false });
    expect(
      resolveAppendOverlap(true, "other-generation", 5, {
        generation,
        start: 5,
        end: 11,
      }),
    ).toEqual({ accept: false });
    expect(
      resolveAppendOverlap(true, generation, 4, {
        generation,
        start: 5,
        end: 11,
      }),
    ).toEqual({ accept: false });
  });
});
