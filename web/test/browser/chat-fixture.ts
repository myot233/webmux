import { afterEach, beforeEach, vi } from "vitest";
import { userEvent } from "vitest/browser";
import { cleanup } from "vitest-browser-svelte/pure";
import type { ChatEvent, ChatFrame, ChatState } from "../../src/schemas";

// A hybrid session's streams and commands without a backend: every
// WebSocket the page opens is a FakeSocket the test drives, and every API
// request lands in `requests` and is answered by `respond`.

export class FakeSocket {
  static readonly CONNECTING = 0;
  static readonly OPEN = 1;
  static readonly CLOSING = 2;
  static readonly CLOSED = 3;
  readonly url: string;
  binaryType = "blob";
  readyState = FakeSocket.CONNECTING;
  bufferedAmount = 0;
  onopen: ((event: Event) => void) | null = null;
  onmessage: ((event: MessageEvent) => void) | null = null;
  onclose: ((event: CloseEvent) => void) | null = null;
  onerror: ((event: Event) => void) | null = null;

  constructor(url: string | URL) {
    this.url = String(url);
    sockets.push(this);
  }
  get path() {
    return new URL(this.url).pathname;
  }
  open() {
    this.readyState = FakeSocket.OPEN;
    this.onopen?.(new Event("open"));
  }
  send() {}
  close() {
    if (this.readyState === FakeSocket.CLOSED) return;
    this.readyState = FakeSocket.CLOSED;
    this.onclose?.(new CloseEvent("close", { code: 1000 }));
  }
  /** A close by the server, such as 4409 when the session moved. */
  drop(code: number) {
    this.readyState = FakeSocket.CLOSED;
    this.onclose?.(new CloseEvent("close", { code }));
  }
  /** A chat channel frame (JSON text). */
  frame(frame: ChatFrame) {
    this.onmessage?.(
      new MessageEvent("message", { data: JSON.stringify(frame) }),
    );
  }
  /** A terminal stream frame: one kind byte, then a JSON payload. */
  binary(kind: number, payload: unknown) {
    const body = new TextEncoder().encode(JSON.stringify(payload));
    const data = new Uint8Array(body.length + 1);
    data[0] = kind;
    data.set(body, 1);
    this.onmessage?.(new MessageEvent("message", { data: data.buffer }));
  }
}

type Recorded = { method: string; path: string; body: unknown };
type Responder = (request: Recorded) => Response | Promise<Response>;

export let sockets: FakeSocket[] = [];
let requests: Recorded[] = [];
let responders: [RegExp, Responder][] = [];

/** Answers requests whose "METHOD /path" matches `pattern`; later wins. */
export function respond(pattern: RegExp, responder: Responder) {
  responders.unshift([pattern, responder]);
}

export function installChatFixture() {
  beforeEach(() => {
    sockets = [];
    requests = [];
    responders = [];
    vi.stubGlobal("WebSocket", FakeSocket);
    vi.stubGlobal(
      "fetch",
      vi.fn<typeof fetch>(async (input, init) => {
        const url = new URL(String(input), location.href);
        const request: Recorded = {
          method: init?.method ?? "GET",
          path: url.pathname,
          body: typeof init?.body === "string" ? JSON.parse(init.body) : null,
        };
        requests.push(request);
        const key = `${request.method} ${request.path}`;
        const match = responders.find(([pattern]) => pattern.test(key));
        if (match) return match[1](request);
        if (request.method !== "GET")
          return new Response(null, { status: 204 });
        return Response.json({ error: "not found" }, { status: 404 });
      }),
    );
  });
  afterEach(async () => {
    try {
      await cleanup();
      await userEvent.cleanup();
    } finally {
      vi.unstubAllGlobals();
      vi.restoreAllMocks();
    }
  });
}

/** The open socket whose path is `path`, once the page created it. */
export async function socketAt(path: string, nth = 0) {
  let found: FakeSocket | undefined;
  await vi.waitFor(() => {
    found = sockets.filter((socket) => socket.path === path)[nth];
    if (!found) throw new Error(`no socket at ${path} (#${nth})`);
  });
  if (found!.readyState === FakeSocket.CONNECTING) found!.open();
  return found!;
}

/** Requests other than reads, in order. */
export function commands() {
  return requests.filter((request) => request.method !== "GET");
}

export function chatState(patch: Partial<ChatState> = {}): ChatState {
  return {
    side: "acp",
    dormant: false,
    agent: "omp",
    agentSessionId: "01a17e26c4b84f0e9d2a",
    eligibility: { available: true },
    turn: { running: false },
    requests: [],
    config: [],
    commands: [],
    plan: null,
    usage: null,
    queue: { paused: false, items: [] },
    steering: false,
    authRequired: false,
    promptCapabilities: { image: false, embeddedContext: false },
    permissionMode: "auto",
    unsupported: [],
    idChange: null,
    recovery: null,
    ...patch,
  };
}

const at = "2026-10-28T00:00:00Z";
let seq = 0;

/** Events with consecutive sequence numbers from 0 for each new snapshot. */
export function events(...items: DistributiveOmit<ChatEvent, "seq" | "at">[]) {
  return items.map((item) => ({ ...item, seq: seq++, at }) as ChatEvent);
}

/** A fresh chat channel: a snapshot from sequence 0. */
export function snapshot(
  state: ChatState,
  extra: { truncated?: boolean } = {},
): ChatFrame {
  seq = 0;
  return { type: "snapshot", generation: "g1", from: 0, state, ...extra };
}

type DistributiveOmit<T, K extends PropertyKey> = T extends unknown
  ? Omit<T, K>
  : never;
