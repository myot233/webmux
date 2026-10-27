import { stored } from "./api";

const maxBufferedAmount = 1024 * 1024;

// Browsers cannot set Authorization on a WebSocket handshake. The token rides
// in an offered subprotocol instead of the URL (proxy and edge request logs)
// or a cookie (shared by every port of the host). The server selects only
// "webmux" and never echoes the token protocol.
export function protocols(token: string): string[] {
  if (!token) return ["webmux"];
  let binary = "";
  for (const byte of new TextEncoder().encode(token))
    binary += String.fromCharCode(byte);
  const encoded = btoa(binary)
    .replaceAll("+", "-")
    .replaceAll("/", "_")
    .replace(/=+$/, "");
  return ["webmux", "webmux.token." + encoded];
}

export type Connection =
  | "connecting"
  | "connected"
  | "reconnecting"
  | "exited"
  | "missing";
export interface SocketConnection {
  // True means sent or coalesced for delivery on this socket (controls only).
  send(kind: number, payload: Uint8Array): boolean;
  close(): void;
  // Explicit recovery may reopen an exited session for its final replay, not a missing one.
  reconnect(): void;
}
// How long a foregrounded OPEN socket may stay silent after `resume` before
// it is treated as a dead mobile connection and replaced.
export const resumeTimeout = 3000;
export interface ConnectOptions {
  // Asks the server for a reply on a socket that still claims OPEN when the
  // page returns to the foreground, and returns true once it sent one. Any
  // message within resumeTimeout proves the connection healthy; silence
  // replaces it. Returning false, or omitting it, reattaches immediately and
  // takes the server snapshot as the source of truth.
  resume?: (send: SocketConnection["send"]) => boolean;
}
export function connect(
  // Evaluated on every attempt, so a reconnect can carry fresh query state.
  path: string | (() => string),
  onMessage: (data: string | ArrayBuffer) => void,
  onState: (state: Connection) => void,
  onError: (message: string) => void,
  options: ConnectOptions = {},
): SocketConnection {
  let socket: WebSocket | undefined,
    timer: number | undefined,
    stableTimer: number | undefined,
    controlTimer: number | undefined,
    resumeTimer: number | undefined,
    attempt = 0,
    disposed = false,
    finished: "exited" | "missing" | undefined;
  const controls = new Map<number, Uint8Array>();
  function clearControls() {
    clearTimeout(controlTimer);
    controlTimer = undefined;
    controls.clear();
  }
  function transmit(kind: number, payload: Uint8Array) {
    if (
      socket?.readyState !== WebSocket.OPEN ||
      socket.bufferedAmount + payload.byteLength + 1 > maxBufferedAmount
    )
      return false;
    const frame = new Uint8Array(payload.length + 1);
    frame[0] = kind;
    frame.set(payload, 1);
    socket.send(frame);
    return true;
  }
  function flushControls() {
    clearTimeout(controlTimer);
    controlTimer = undefined;
    if (socket?.readyState !== WebSocket.OPEN) return;
    for (const [kind, payload] of controls) {
      if (transmit(kind, payload)) controls.delete(kind);
    }
    if (controls.size) controlTimer = window.setTimeout(flushControls, 50);
  }
  function open() {
    if (disposed || finished) return;
    clearTimeout(timer);
    clearTimeout(stableTimer);
    clearTimeout(resumeTimer);
    clearControls();
    const previous = socket;
    socket = undefined;
    previous?.close();
    onState(attempt ? "reconnecting" : "connecting");
    const target = typeof path === "string" ? path : path();
    const next = new WebSocket(
      `${location.protocol === "https:" ? "wss" : "ws"}://${location.host}${target}`,
      protocols(stored("token")),
    );
    socket = next;
    let closed = false;
    next.binaryType = "arraybuffer";
    next.onopen = () => {
      if (socket !== next || closed) return;
      stableTimer = window.setTimeout(() => {
        if (socket === next) attempt = 0;
      }, 10000);
      onState("connected");
    };
    next.onmessage = (event) => {
      if (socket !== next || closed) return;
      clearTimeout(resumeTimer);
      try {
        onMessage(event.data);
      } catch (error) {
        onError(error instanceof Error ? error.message : "无效的服务器消息");
        next.close(1002, "Invalid server message");
      }
    };
    next.onclose = (event) => {
      if (socket !== next || disposed || closed) return;
      closed = true;
      clearTimeout(stableTimer);
      clearTimeout(resumeTimer);
      clearControls();
      if (event.code === 4410 || event.code === 4404) {
        finished = event.code === 4410 ? "exited" : "missing";
        onState(finished);
        return;
      }
      onState("reconnecting");
      const ceiling = Math.min(8000, 500 * 2 ** Math.min(attempt++, 4));
      const delay = ceiling * (0.5 + Math.random() * 0.5);
      timer = window.setTimeout(open, delay);
    };
    next.onerror = () => next.close();
  }
  const visible = () => {
    // A probe left by a foreground that was hidden again must not replace
    // the socket in the background.
    clearTimeout(resumeTimer);
    if (document.hidden || finished) return;
    // A suspended mobile socket may still claim OPEN after the network died.
    // Callers that can probe it keep a healthy connection.
    const current = socket;
    const probed =
      current?.readyState === WebSocket.OPEN &&
      !!options.resume?.((kind, payload) => api.send(kind, payload));
    if (!probed) {
      open();
      return;
    }
    resumeTimer = window.setTimeout(() => {
      if (socket === current) open();
    }, resumeTimeout);
  };
  document.addEventListener("visibilitychange", visible);
  const online = () => {
    if (socket?.readyState !== WebSocket.OPEN) open();
  };
  window.addEventListener("online", online);
  const restored = (event: PageTransitionEvent) => {
    // Initial pageshow may follow an already healthy connection when fonts
    // load late. Only bfcache restores require a fresh attachment here.
    if (event.persisted) visible();
  };
  window.addEventListener("pageshow", restored);
  const api: SocketConnection = {
    send(kind: number, payload: Uint8Array) {
      if (socket?.readyState !== WebSocket.OPEN) return false;
      if (payload.byteLength + 1 > maxBufferedAmount) return false;
      // Only idempotent controls are retained: one latest resize and one resync.
      if (kind === 1 || kind === 2) {
        controls.set(kind, payload.slice());
        flushControls();
        return true;
      }
      flushControls();
      // Leave rejected terminal input with the caller; never replay it later.
      return transmit(kind, payload);
    },
    close() {
      disposed = true;
      clearTimeout(timer);
      clearTimeout(stableTimer);
      clearTimeout(resumeTimer);
      clearControls();
      const previous = socket;
      socket = undefined;
      previous?.close();
      document.removeEventListener("visibilitychange", visible);
      window.removeEventListener("online", online);
      window.removeEventListener("pageshow", restored);
    },
    reconnect() {
      if (disposed || finished === "missing") return;
      finished = undefined;
      open();
    },
  };
  open();
  return api;
}
