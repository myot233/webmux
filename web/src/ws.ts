import { stored } from "./api";

const maxBufferedAmount = 1024 * 1024;

export type Connection =
  "connecting" | "connected" | "reconnecting" | "exited" | "missing";
export interface SocketConnection {
  send(kind: number, payload: Uint8Array): boolean;
  close(): void;
  reconnect(): void;
}
export function connect(
  // Evaluated on every attempt, so a reconnect can carry fresh query state.
  path: string | (() => string),
  onMessage: (data: string | ArrayBuffer) => void,
  onState: (state: Connection) => void,
  onError: (message: string) => void,
): SocketConnection {
  let socket: WebSocket | undefined,
    timer: number | undefined,
    stableTimer: number | undefined,
    attempt = 0,
    disposed = false,
    finished = false;
  function open() {
    if (disposed || finished) return;
    clearTimeout(timer);
    clearTimeout(stableTimer);
    const previous = socket;
    socket = undefined;
    previous?.close();
    onState(attempt ? "reconnecting" : "connecting");
    const target = typeof path === "string" ? path : path();
    const next = new WebSocket(
      `${location.protocol === "https:" ? "wss" : "ws"}://${location.host}${target}${target.includes("?") ? "&" : "?"}token=${encodeURIComponent(stored("token"))}`,
    );
    socket = next;
    next.binaryType = "arraybuffer";
    next.onopen = () => {
      if (socket !== next) return;
      stableTimer = window.setTimeout(() => {
        if (socket === next) attempt = 0;
      }, 10000);
      onState("connected");
    };
    next.onmessage = (event) => {
      if (socket !== next) return;
      try {
        onMessage(event.data);
      } catch (error) {
        onError(error instanceof Error ? error.message : "无效的服务器消息");
        next.close(1002, "Invalid server message");
      }
    };
    next.onclose = (event) => {
      if (socket !== next || disposed) return;
      clearTimeout(stableTimer);
      if (event.code === 4410 || event.code === 4404) {
        finished = true;
        onState(event.code === 4410 ? "exited" : "missing");
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
    // A suspended mobile socket may still claim OPEN after the network died.
    // Always reattach on foregrounding; the server replay is the source of truth.
    if (!document.hidden && !finished) open();
  };
  document.addEventListener("visibilitychange", visible);
  const online = () => {
    if (socket?.readyState !== WebSocket.OPEN) open();
  };
  window.addEventListener("online", online);
  window.addEventListener("pageshow", visible);
  open();
  return {
    send(kind: number, payload: Uint8Array) {
      if (socket?.readyState !== WebSocket.OPEN) return false;
      // Leave rejected input with the caller instead of growing the browser queue.
      if (socket.bufferedAmount + payload.byteLength + 1 > maxBufferedAmount)
        return false;
      const frame = new Uint8Array(payload.length + 1);
      frame[0] = kind;
      frame.set(payload, 1);
      socket.send(frame);
      return true;
    },
    close() {
      disposed = true;
      clearTimeout(timer);
      clearTimeout(stableTimer);
      socket?.close();
      document.removeEventListener("visibilitychange", visible);
      window.removeEventListener("online", online);
      window.removeEventListener("pageshow", visible);
    },
    reconnect: open,
  };
}
