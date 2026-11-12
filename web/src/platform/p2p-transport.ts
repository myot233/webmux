import {
  P2P_CHUNK_BYTES,
  StreamFrameReassembler,
  encodeStreamCloseFrame,
  encodeStreamFrames,
} from "./p2p-framing";
import {
  P2PRPCRequestHeaderSchema,
  P2PRPCResponseHeaderSchema,
} from "../contracts/schemas";
import {
  SOCKET_CLOSED,
  SOCKET_CONNECTING,
  SOCKET_OPEN,
  TransportUploadError,
  type ClientTransport,
  type StreamOptions,
  type TransportSocket,
  type UploadRequest,
  type UploadResult,
} from "./transport";

const BUFFER_LOW_THRESHOLD = 256 * 1024;
const BUFFER_HIGH_THRESHOLD = 2 * 1024 * 1024;

export interface P2PTransportOptions {
  pc: RTCPeerConnection;
  fallback: ClientTransport;
  onChannelFailure?: () => void;
}

function stripCompressionQuery(path: string): string {
  const qIndex = path.indexOf("?");
  if (qIndex < 0) return path;
  const base = path.slice(0, qIndex);
  const params = new URLSearchParams(path.slice(qIndex + 1));
  if (params.has("compress")) {
    params.delete("compress");
    // Ensure zreplay=1 is enabled when WebSocket permessage-deflate is omitted.
    if (base.startsWith("/ws/s/") && !base.endsWith("/chat")) {
      params.set("zreplay", "1");
    }
  }
  const search = params.toString();
  return search ? `${base}?${search}` : base;
}

function normalizeHeaders(headers?: HeadersInit): Record<string, string> {
  const out: Record<string, string> = {};
  if (!headers) return out;
  const h = new Headers(headers);
  h.forEach((value, key) => {
    out[key] = value;
  });
  return out;
}

async function serializeBody(
  body: BodyInit | null | undefined,
  headers: Record<string, string>,
): Promise<Uint8Array<ArrayBuffer>> {
  if (body === undefined || body === null) {
    return new Uint8Array(0);
  }
  if (typeof body === "string") {
    return new TextEncoder().encode(body);
  }
  if (body instanceof Uint8Array) {
    const copy = new Uint8Array(body.byteLength);
    copy.set(body);
    return copy;
  }
  if (body instanceof ArrayBuffer) {
    return new Uint8Array(body.slice(0));
  }
  const resp = new Response(body);
  const ct = resp.headers.get("Content-Type");
  if (
    ct &&
    !Object.keys(headers).some((k) => k.toLowerCase() === "content-type")
  ) {
    headers["Content-Type"] = ct;
  }
  const buf = await resp.arrayBuffer();
  return new Uint8Array(buf);
}

async function waitBufferedLow(
  dc: RTCDataChannel,
  signal?: AbortSignal | null,
): Promise<void> {
  while (
    dc.readyState === "open" &&
    dc.bufferedAmount > BUFFER_HIGH_THRESHOLD
  ) {
    if (signal?.aborted) {
      throw new DOMException("Aborted", "AbortError");
    }
    await new Promise<void>((resolve, reject) => {
      const done = () => {
        dc.removeEventListener("bufferedamountlow", onLow);
        dc.removeEventListener("close", onClose);
        dc.removeEventListener("error", onClose);
        signal?.removeEventListener("abort", onAbort);
      };
      const onLow = () => {
        done();
        resolve();
      };
      const onClose = () => {
        done();
        reject(new Error("P2P DataChannel closed"));
      };
      const onAbort = () => {
        done();
        reject(new DOMException("Aborted", "AbortError"));
      };
      dc.addEventListener("bufferedamountlow", onLow, { once: true });
      dc.addEventListener("close", onClose, { once: true });
      dc.addEventListener("error", onClose, { once: true });
      signal?.addEventListener("abort", onAbort, { once: true });
    });
  }
}

export function createP2PTransport(
  options: P2PTransportOptions,
): ClientTransport {
  const { pc, fallback, onChannelFailure } = options;
  let rpcCounter = 0;

  async function executeRPC(
    apiPath: string,
    method: string,
    headers: Record<string, string>,
    body: Uint8Array<ArrayBuffer>,
    signal?: AbortSignal | null,
    onProgress?: (percent: number) => void,
  ): Promise<{
    status: number;
    statusText: string;
    headers: Record<string, string>;
    body: Uint8Array<ArrayBuffer>;
  }> {
    if (signal?.aborted) {
      throw new DOMException("Aborted", "AbortError");
    }
    if (
      pc.connectionState !== "connected" &&
      pc.connectionState !== "connecting"
    ) {
      throw new Error("PeerConnection not active");
    }

    const dc = pc.createDataChannel(`rpc:${++rpcCounter}`, { ordered: true });
    dc.binaryType = "arraybuffer";
    dc.bufferedAmountLowThreshold = BUFFER_LOW_THRESHOLD;

    return await new Promise((resolve, reject) => {
      let settled = false;
      let respHeader: {
        status: number;
        statusText: string;
        headers: Record<string, string>;
      } | null = null;
      const respChunks: Uint8Array[] = [];
      let respBytes = 0;

      const finish = (fn: () => void) => {
        if (settled) return;
        settled = true;
        signal?.removeEventListener("abort", onAbort);
        try {
          dc.close();
        } catch {
          // ignore close errors
        }
        fn();
      };

      const onAbort = () => {
        finish(() => reject(new DOMException("Aborted", "AbortError")));
      };
      signal?.addEventListener("abort", onAbort, { once: true });

      dc.onerror = () => {
        onChannelFailure?.();
        finish(() => reject(new Error("P2P RPC DataChannel error")));
      };
      dc.onclose = () => {
        if (!settled) {
          onChannelFailure?.();
          finish(() => reject(new Error("P2P RPC DataChannel closed early")));
        }
      };

      dc.onmessage = (event) => {
        if (settled) return;
        if (respHeader === null) {
          if (typeof event.data !== "string") {
            finish(() => reject(new Error("Invalid P2P RPC response header")));
            return;
          }
          try {
            respHeader = P2PRPCResponseHeaderSchema.parse(
              JSON.parse(event.data),
            );
          } catch (err) {
            finish(() => reject(err));
          }
          return;
        }
        const chunk =
          event.data instanceof ArrayBuffer
            ? new Uint8Array(event.data)
            : new TextEncoder().encode(String(event.data));
        if (chunk.byteLength === 0) {
          const merged = new Uint8Array(respBytes);
          let off = 0;
          for (const part of respChunks) {
            merged.set(part, off);
            off += part.byteLength;
          }
          const hdr = respHeader;
          finish(() =>
            resolve({
              status: hdr.status,
              statusText: hdr.statusText,
              headers: hdr.headers,
              body: merged,
            }),
          );
          return;
        }
        respChunks.push(chunk);
        respBytes += chunk.byteLength;
      };

      dc.onopen = () => {
        void (async () => {
          try {
            const reqHdr = P2PRPCRequestHeaderSchema.parse({
              method,
              path: `/api${apiPath}`,
              headers,
              bodyLength: body.byteLength,
            });
            dc.send(JSON.stringify(reqHdr));

            for (let off = 0; off < body.byteLength; off += P2P_CHUNK_BYTES) {
              await waitBufferedLow(dc, signal);
              const end = Math.min(off + P2P_CHUNK_BYTES, body.byteLength);
              dc.send(body.subarray(off, end));
              if (onProgress && body.byteLength > 0) {
                const sent = Math.max(0, end - dc.bufferedAmount);
                onProgress(
                  Math.min(99, Math.round((sent / body.byteLength) * 100)),
                );
              }
            }
            await waitBufferedLow(dc, signal);
            dc.send(new Uint8Array(0));
            onProgress?.(100);
          } catch (err) {
            finish(() => reject(err));
          }
        })();
      };
    });
  }

  return {
    get origin() {
      return fallback.origin;
    },

    async fetch(path: string, init: RequestInit): Promise<Response> {
      // Signaling, P2P settings, and baseline HTTP ping always stay on HTTP.
      if (path === "/p2p" || path.startsWith("/p2p/")) {
        return fallback.fetch(path, init);
      }
      const method = (init.method ?? "GET").toUpperCase();
      const headers = normalizeHeaders(init.headers);
      const body = await serializeBody(init.body, headers);
      try {
        const res = await executeRPC(path, method, headers, body, init.signal);
        // Null-body statuses (204, 205, 304) must have a null Response body.
        const nullBody =
          res.status === 204 || res.status === 205 || res.status === 304;
        return new Response(nullBody ? null : res.body, {
          status: res.status,
          statusText: res.statusText,
          headers: res.headers,
        });
      } catch (err) {
        if (err instanceof DOMException && err.name === "AbortError") {
          throw err;
        }
        // Safe idempotent reads retry transparently on baseline HTTP once.
        if (method === "GET" || method === "HEAD") {
          return fallback.fetch(path, init);
        }
        throw err;
      }
    },

    async upload(req: UploadRequest): Promise<UploadResult> {
      if (req.signal.aborted) {
        throw new DOMException("上传已取消", "AbortError");
      }
      const headers: Record<string, string> = {
        Authorization: "Bearer " + req.token,
      };
      const body = await serializeBody(req.form, headers);
      const timeoutCtrl = new AbortController();
      let timedOut = false;
      const timer = setTimeout(() => {
        timedOut = true;
        timeoutCtrl.abort();
      }, req.timeoutMs);
      const onCallerAbort = () => timeoutCtrl.abort();
      req.signal.addEventListener("abort", onCallerAbort, { once: true });

      try {
        const res = await executeRPC(
          req.path,
          "POST",
          headers,
          body,
          timeoutCtrl.signal,
          req.onProgress,
        );
        let parsed: unknown = null;
        if (res.body.byteLength > 0) {
          const text = new TextDecoder().decode(res.body);
          try {
            parsed = JSON.parse(text);
          } catch {
            parsed = text;
          }
        }
        return {
          status: res.status,
          statusText: res.statusText,
          body: parsed,
        };
      } catch (err) {
        if (req.signal.aborted) {
          throw new DOMException("上传已取消", "AbortError");
        }
        if (timedOut) {
          throw new TransportUploadError("timeout");
        }
        if (err instanceof Error) {
          throw new TransportUploadError("network");
        }
        throw new TransportUploadError("network");
      } finally {
        clearTimeout(timer);
        req.signal.removeEventListener("abort", onCallerAbort);
      }
    },

    openStream(path: string, _options: StreamOptions): TransportSocket {
      if (
        pc.connectionState !== "connected" &&
        pc.connectionState !== "connecting"
      ) {
        onChannelFailure?.();
        return fallback.openStream(path, _options);
      }
      const target = stripCompressionQuery(path);
      const dc = pc.createDataChannel(`stream:${target}`, { ordered: true });
      dc.binaryType = "arraybuffer";
      return adaptStreamDataChannel(dc, onChannelFailure);
    },
  };
}

function adaptStreamDataChannel(
  dc: RTCDataChannel,
  onChannelFailure?: () => void,
): TransportSocket {
  const reassembler = new StreamFrameReassembler();
  let closed = false;
  let closeCode = 1006;
  let closeReason = "";
  const socket: TransportSocket = {
    get readyState() {
      if (closed) return SOCKET_CLOSED;
      switch (dc.readyState) {
        case "connecting":
          return SOCKET_CONNECTING;
        case "open":
          return SOCKET_OPEN;
        default:
          return SOCKET_CLOSED;
      }
    },
    get bufferedAmount() {
      return dc.bufferedAmount;
    },
    onopen: null,
    onmessage: null,
    onclose: null,
    onerror: null,
    send(data) {
      if (closed || dc.readyState !== "open") return;
      for (const frame of encodeStreamFrames(data)) {
        dc.send(frame);
      }
    },
    close(code = 1000, reason = "") {
      if (closed) return;
      if (dc.readyState === "open") {
        try {
          dc.send(encodeStreamCloseFrame(code, reason));
        } catch {
          // ignore send errors during close
        }
      }
      emitClose(code, reason);
    },
  };

  function emitClose(code: number, reason: string) {
    if (closed) return;
    closed = true;
    closeCode = code;
    closeReason = reason;
    try {
      dc.close();
    } catch {
      // ignore
    }
    socket.onclose?.({ code: closeCode, reason: closeReason });
  }

  dc.onopen = () => {
    if (!closed) socket.onopen?.();
  };
  dc.onmessage = (event) => {
    if (closed) return;
    const raw =
      event.data instanceof ArrayBuffer
        ? event.data
        : new TextEncoder().encode(String(event.data)).buffer;
    const decoded = reassembler.feed(raw);
    if (!decoded) return;
    if (decoded.kind === "message") {
      socket.onmessage?.({ data: decoded.data });
    } else if (decoded.kind === "close") {
      emitClose(decoded.code, decoded.reason);
    } else {
      socket.onerror?.();
      emitClose(1002, decoded.message);
    }
  };
  dc.onerror = () => {
    onChannelFailure?.();
    if (!closed) socket.onerror?.();
  };
  dc.onclose = () => {
    emitClose(closeCode, closeReason);
  };

  return socket;
}
