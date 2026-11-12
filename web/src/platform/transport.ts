import { encodeBase64Url } from "./base64url";

export const SOCKET_CONNECTING = 0;
export const SOCKET_OPEN = 1;
export const SOCKET_CLOSED = 3;

export interface TransportSocket {
  readonly readyState: number;
  readonly bufferedAmount: number;
  send(data: string | Uint8Array<ArrayBuffer>): void;
  close(code?: number, reason?: string): void;
  onopen: (() => void) | null;
  onmessage: ((event: { data: string | ArrayBuffer }) => void) | null;
  onclose: ((event: { code: number; reason?: string }) => void) | null;
  onerror: (() => void) | null;
}

export interface UploadRequest {
  path: string;
  form: FormData;
  token: string;
  timeoutMs: number;
  signal: AbortSignal;
  onProgress: (percent: number) => void;
}

export interface UploadResult {
  status: number;
  statusText: string;
  body: unknown;
}

export class TransportUploadError extends Error {
  constructor(public readonly kind: "network" | "timeout") {
    super(kind === "timeout" ? "上传超时" : "上传失败：网络连接异常");
  }
}

export interface StreamOptions {
  token: string;
  compress: boolean;
}

export interface ClientTransport {
  readonly origin: string;
  fetch(path: string, init: RequestInit): Promise<Response>;
  upload(request: UploadRequest): Promise<UploadResult>;
  openStream(path: string, options: StreamOptions): TransportSocket;
}

// Browsers cannot set Authorization on a WebSocket handshake. The token rides
// in an offered subprotocol instead of the URL (proxy and edge request logs)
// or a cookie (shared by every port of the host). The server selects only
// "webmux" and never echoes the token protocol.
export function protocols(token: string): string[] {
  if (!token) return ["webmux"];
  return ["webmux", "webmux.token." + encodeBase64Url(token)];
}

export function withCompressionQuery(path: string, compress: boolean): string {
  if (!compress) return path;
  return `${path}${path.includes("?") ? "&" : "?"}compress=1`;
}

interface ParsedBaseUrl {
  origin: string;
  wsOrigin: string;
  basePath: string;
}

function parseBaseUrl(raw: string): ParsedBaseUrl {
  const url = new URL(raw);
  if (url.protocol !== "http:" && url.protocol !== "https:") {
    throw new Error(`Unsupported host base URL protocol "${url.protocol}"`);
  }
  if (url.search || url.hash) {
    throw new Error("Host base URL must not include query or hash");
  }
  const basePath = url.pathname === "/" ? "" : url.pathname.replace(/\/+$/, "");
  const wsScheme = url.protocol === "https:" ? "wss:" : "ws:";
  return {
    origin: `${url.origin}${basePath}`,
    wsOrigin: `${wsScheme}//${url.host}${basePath}`,
    basePath,
  };
}

let configuredBase: ParsedBaseUrl | null = null;

/** Creates a browser HTTP + XHR + WebSocket transport for a given base URL (or same-origin when null). */
export function createHttpTransport(
  baseUrl: string | null = null,
): ClientTransport {
  const fixed = baseUrl === null ? null : parseBaseUrl(baseUrl);
  const resolveBase = () => fixed ?? configuredBase;
  const apiUrl = (path: string) => {
    const base = resolveBase();
    return base ? `${base.origin}/api${path}` : `/api${path}`;
  };
  const streamUrl = (path: string) => {
    const base = resolveBase();
    if (base) return `${base.wsOrigin}${path}`;
    return `${location.protocol === "https:" ? "wss" : "ws"}://${location.host}${path}`;
  };

  return {
    get origin() {
      const base = resolveBase();
      if (base) return base.origin;
      return location.origin ?? `${location.protocol}//${location.host}`;
    },
    fetch(path, init) {
      return fetch(apiUrl(path), init);
    },
    upload(req) {
      const { promise, resolve, reject } =
        Promise.withResolvers<UploadResult>();
      if (req.signal.aborted) {
        reject(new DOMException("上传已取消", "AbortError"));
        return promise;
      }
      const xhr = new XMLHttpRequest();
      const abort = () => xhr.abort();
      const cleanup = () => req.signal.removeEventListener("abort", abort);
      xhr.open("POST", apiUrl(req.path));
      xhr.setRequestHeader("Authorization", "Bearer " + req.token);
      xhr.responseType = "json";
      xhr.timeout = req.timeoutMs;
      xhr.upload.onprogress = (event) => {
        if (event.lengthComputable)
          req.onProgress(Math.round((event.loaded / event.total) * 100));
      };
      xhr.onload = () => {
        cleanup();
        resolve({
          status: xhr.status,
          statusText: xhr.statusText,
          body: xhr.response,
        });
      };
      xhr.onerror = () => {
        cleanup();
        reject(new TransportUploadError("network"));
      };
      xhr.ontimeout = () => {
        cleanup();
        reject(new TransportUploadError("timeout"));
      };
      xhr.onabort = () => {
        cleanup();
        reject(new DOMException("上传已取消", "AbortError"));
      };
      req.signal.addEventListener("abort", abort, { once: true });
      xhr.send(req.form);
      return promise;
    },
    openStream(path, options) {
      const target = withCompressionQuery(path, options.compress);
      const socket = new WebSocket(streamUrl(target), protocols(options.token));
      socket.binaryType = "arraybuffer";
      return adaptWebSocket(socket);
    },
  };
}

function adaptWebSocket(socket: WebSocket): TransportSocket {
  let onopen: TransportSocket["onopen"] = null;
  let onmessage: TransportSocket["onmessage"] = null;
  let onclose: TransportSocket["onclose"] = null;
  let onerror: TransportSocket["onerror"] = null;
  socket.onopen = () => onopen?.();
  socket.onmessage = (event) => {
    if (typeof event.data === "string" || event.data instanceof ArrayBuffer)
      onmessage?.({ data: event.data });
  };
  socket.onclose = (event) =>
    onclose?.({ code: event.code, reason: event.reason });
  socket.onerror = () => onerror?.();
  return {
    get readyState() {
      return socket.readyState;
    },
    get bufferedAmount() {
      return socket.bufferedAmount;
    },
    send(data) {
      socket.send(data);
    },
    close(code, reason) {
      if (code === undefined) socket.close();
      else socket.close(code, reason);
    },
    get onopen() {
      return onopen;
    },
    set onopen(fn) {
      onopen = fn;
    },
    get onmessage() {
      return onmessage;
    },
    set onmessage(fn) {
      onmessage = fn;
    },
    get onclose() {
      return onclose;
    },
    set onclose(fn) {
      onclose = fn;
    },
    get onerror() {
      return onerror;
    },
    set onerror(fn) {
      onerror = fn;
    },
  };
}

const defaultTransport = createHttpTransport();
let activeTransport: ClientTransport = defaultTransport;

/** Returns the default browser HTTP + WebSocket transport. */
export function defaultBrowserTransport(): ClientTransport {
  return defaultTransport;
}

/** Returns the active client transport. */
export function currentTransport(): ClientTransport {
  return activeTransport;
}

/** Replaces the active client transport (or restores the default HTTP/WebSocket transport when null). */
export function setClientTransport(transport: ClientTransport | null): void {
  activeTransport = transport ?? defaultTransport;
}

/** Configures the default HTTP/WebSocket transport's target host base URL (or null for same-origin). */
export function setHostBaseUrl(baseUrl: string | null): void {
  configuredBase = baseUrl === null ? null : parseBaseUrl(baseUrl);
}

/** Returns the active host's origin URL for shareable host links. */
export function hostOrigin(): string {
  return activeTransport.origin;
}
