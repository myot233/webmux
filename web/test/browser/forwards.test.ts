import { afterEach, expect, test, vi } from "vitest";
import {
  connectSessions,
  forwardLink,
  forwards,
  ignorePort,
  isIgnored,
  pruneIgnored,
} from "../../src/domains/workspace/forwards.svelte";
import {
  setClientTransport,
  setHostBaseUrl,
} from "../../src/platform/transport";
import { forward } from "./forward-fixture";

class MockWebSocket {
  static OPEN = 1;
  static instances: MockWebSocket[] = [];
  readyState = MockWebSocket.OPEN;
  bufferedAmount = 0;
  binaryType = "";
  onopen: (() => void) | null = null;
  onmessage: ((event: { data: string }) => void) | null = null;
  onclose: ((event: { code: number }) => void) | null = null;
  onerror: (() => void) | null = null;
  constructor() {
    MockWebSocket.instances.push(this);
  }
  send() {}
  close() {
    this.readyState = 3;
  }
}

afterEach(() => {
  setClientTransport(null);
  setHostBaseUrl(null);
  vi.unstubAllGlobals();
  MockWebSocket.instances = [];
});

function open() {
  vi.stubGlobal("WebSocket", MockWebSocket);
  const onEvent = vi.fn();
  const onError = vi.fn();
  const connection = connectSessions(onEvent, vi.fn(), onError);
  const socket = MockWebSocket.instances.at(-1)!;
  socket.onopen?.();
  const send = (value: unknown) =>
    socket.onmessage?.({ data: JSON.stringify(value) });
  return { connection, send, onEvent, onError };
}

test("snapshot and forwards events replace the list; invalid ones change nothing", () => {
  const { connection, send, onEvent, onError } = open();
  expect(forwards.ready).toBe(false);
  send({
    type: "snapshot",
    sessions: [],
    forwards: [forward(8000), forward(5173)],
  });
  expect(forwards.ready).toBe(true);
  expect(forwards.list.map((entry) => entry.port)).toEqual([5173, 8000]);
  expect(onEvent).toHaveBeenCalledOnce();

  send({ type: "forwards", forwards: [{ ...forward(5173), status: "bogus" }] });
  expect(onError).toHaveBeenCalledOnce();
  expect(forwards.list.map((entry) => entry.port)).toEqual([5173, 8000]);
  expect(onEvent).toHaveBeenCalledOnce();

  connection.close();
  expect(forwards.ready).toBe(false);
});

test("a later connection's valid forwards event replaces the list", () => {
  const { connection, send } = open();
  send({ type: "snapshot", sessions: [], forwards: [forward(5173)] });
  send({ type: "forwards", forwards: [] });
  expect(forwards.list).toEqual([]);
  // A snapshot without forwards is invalid as a whole.
  send({ type: "snapshot", sessions: [] });
  expect(forwards.list).toEqual([]);
  connection.close();
});

test("an ignored port is forgotten once its session stops listening on it", () => {
  ignorePort("aaaaaaaa", 5173);
  ignorePort("aaaaaaaa", 3000);
  ignorePort("bbbbbbbb", 5173);
  pruneIgnored("aaaaaaaa", [
    { port: 3000, process: "node" },
    { port: 5173, process: "node" },
  ]);
  expect(isIgnored("aaaaaaaa", 5173)).toBe(true);

  pruneIgnored("aaaaaaaa", [{ port: 3000, process: "node" }]);
  expect(isIgnored("aaaaaaaa", 5173)).toBe(false);
  expect(isIgnored("aaaaaaaa", 3000)).toBe(true);
  // Other sessions keep their own ignored ports.
  expect(isIgnored("bbbbbbbb", 5173)).toBe(true);

  pruneIgnored("aaaaaaaa", []);
  expect(isIgnored("aaaaaaaa", 3000)).toBe(false);
});

test("forwardLink follows same-origin, configured host base URL, and custom transport origin", () => {
  expect(forwardLink("/forward/5173/")).toBe(
    `${location.origin}/forward/5173/`,
  );

  setHostBaseUrl("https://host-a.example.ts.net");
  expect(forwardLink("/forward/5173/")).toBe(
    "https://host-a.example.ts.net/forward/5173/",
  );

  setClientTransport({
    origin: "https://p2p-peer.webmux.internal",
    fetch: () => Promise.reject(new Error("unused")),
    upload: () => Promise.reject(new Error("unused")),
    openStream: () => {
      throw new Error("unused");
    },
  });
  expect(forwardLink("/forward/8080/")).toBe(
    "https://p2p-peer.webmux.internal/forward/8080/",
  );
});
