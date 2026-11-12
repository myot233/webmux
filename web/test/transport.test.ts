import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { z } from "zod";
import {
  APIError,
  api,
  apiBlob,
  apiBuffer,
  store,
  uploadFile,
  uploadForm,
} from "../src/platform/api";
import { setActiveHostId } from "../src/platform/storage";
import {
  SOCKET_CLOSED,
  SOCKET_CONNECTING,
  SOCKET_OPEN,
  TransportUploadError,
  createHttpTransport,
  currentTransport,
  hostOrigin,
  setClientTransport,
  setHostBaseUrl,
  withCompressionQuery,
  type ClientTransport,
  type StreamOptions,
  type TransportSocket,
  type UploadRequest,
  type UploadResult,
} from "../src/platform/transport";
import { connect, openSocket, setWsCompression } from "../src/platform/ws";

class FakeStream implements TransportSocket {
  readyState = SOCKET_CONNECTING;
  bufferedAmount = 0;
  onopen: (() => void) | null = null;
  onmessage: ((event: { data: string | ArrayBuffer }) => void) | null = null;
  onclose: ((event: { code: number; reason?: string }) => void) | null = null;
  onerror: (() => void) | null = null;
  readonly sent: (string | Uint8Array)[] = [];
  closedWith: { code?: number; reason?: string } | null = null;

  constructor(
    readonly path: string,
    readonly options: StreamOptions,
  ) {}

  open() {
    this.readyState = SOCKET_OPEN;
    this.onopen?.();
  }

  send(data: string | Uint8Array) {
    if (typeof data === "string") {
      this.sent.push(data);
      this.bufferedAmount += data.length;
      return;
    }
    const copy = data.slice();
    this.sent.push(copy);
    this.bufferedAmount += copy.byteLength;
  }

  close(code?: number, reason?: string) {
    this.readyState = SOCKET_CLOSED;
    this.closedWith = { code, reason };
  }

  serverClose(code: number, reason?: string) {
    this.readyState = SOCKET_CLOSED;
    this.onclose?.({ code, reason });
  }
}

class MockXHR {
  static instances: MockXHR[] = [];
  method = "";
  url = "";
  headers = new Map<string, string>();
  responseType = "";
  timeout = 0;
  status = 200;
  statusText = "OK";
  response: unknown = null;
  aborted = false;
  upload: {
    onprogress:
      | ((event: {
          lengthComputable: boolean;
          loaded: number;
          total: number;
        }) => void)
      | null;
  } = { onprogress: null };
  onload: (() => void) | null = null;
  onerror: (() => void) | null = null;
  ontimeout: (() => void) | null = null;
  onabort: (() => void) | null = null;
  sentForm: FormData | null = null;

  constructor() {
    MockXHR.instances.push(this);
  }
  open(method: string, url: string) {
    this.method = method;
    this.url = url;
  }
  setRequestHeader(name: string, value: string) {
    this.headers.set(name, value);
  }
  send(form: FormData) {
    this.sentForm = form;
  }
  abort() {
    this.aborted = true;
    this.onabort?.();
  }
}

class MockWebSocket {
  static instances: MockWebSocket[] = [];
  readyState = SOCKET_OPEN;
  bufferedAmount = 0;
  binaryType = "";
  onopen: (() => void) | null = null;
  onmessage: ((event: { data: string | ArrayBuffer }) => void) | null = null;
  onclose: ((event: { code: number; reason?: string }) => void) | null = null;
  onerror: (() => void) | null = null;

  constructor(
    readonly url: string,
    readonly protocols: string[],
  ) {
    MockWebSocket.instances.push(this);
  }
  send(_data: string | Uint8Array) {}
  close() {
    this.readyState = SOCKET_CLOSED;
  }
}

describe("ClientTransport & host base URL abstraction", () => {
  const storageItems = new Map<string, string>();
  let windowTarget: EventTarget;

  beforeEach(() => {
    storageItems.clear();
    MockXHR.instances = [];
    MockWebSocket.instances = [];
    setClientTransport(null);
    setHostBaseUrl(null);
    setActiveHostId(null);
    vi.stubGlobal("localStorage", {
      getItem: (key: string) => storageItems.get(key) ?? null,
      setItem: (key: string, value: string) => storageItems.set(key, value),
      removeItem: (key: string) => storageItems.delete(key),
      get length() {
        return storageItems.size;
      },
      key: (index: number) => [...storageItems.keys()][index] ?? null,
    });
    vi.stubGlobal("location", {
      protocol: "http:",
      host: "127.0.0.1:7681",
      origin: "http://127.0.0.1:7681",
    });
    vi.stubGlobal("document", new EventTarget());
    windowTarget = new EventTarget();
    vi.stubGlobal(
      "window",
      Object.assign(windowTarget, {
        setTimeout: (...args: Parameters<typeof setTimeout>) =>
          globalThis.setTimeout(...args),
      }),
    );
  });

  afterEach(() => {
    setClientTransport(null);
    setHostBaseUrl(null);
    setActiveHostId(null);
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it("formats compression query parameters cleanly", () => {
    expect(withCompressionQuery("/ws/sessions", false)).toBe("/ws/sessions");
    expect(withCompressionQuery("/ws/sessions", true)).toBe(
      "/ws/sessions?compress=1",
    );
    expect(withCompressionQuery("/ws/s/aaaaaaaa?client=c1", true)).toBe(
      "/ws/s/aaaaaaaa?client=c1&compress=1",
    );
  });

  it("resolves same-origin URLs by default and switches cleanly with setHostBaseUrl", async () => {
    const fetchMock = vi.fn<typeof fetch>().mockImplementation(
      async () =>
        new Response(JSON.stringify({ ok: true }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
    );
    vi.stubGlobal("fetch", fetchMock);
    vi.stubGlobal("XMLHttpRequest", MockXHR);
    vi.stubGlobal("WebSocket", MockWebSocket);
    store("token", "local-tok");

    expect(hostOrigin()).toBe("http://127.0.0.1:7681");

    await api("/ping", z.object({ ok: z.boolean() }));
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/ping",
      expect.objectContaining({
        headers: {
          Authorization: "Bearer local-tok",
          "Content-Type": "application/json",
        },
      }),
    );

    openSocket("/ws/voice", false);
    expect(MockWebSocket.instances[0]?.url).toBe(
      "ws://127.0.0.1:7681/ws/voice",
    );

    // Switch to cross-origin HTTPS host base URL.
    setHostBaseUrl("https://host-a.example.ts.net");
    expect(hostOrigin()).toBe("https://host-a.example.ts.net");

    await api("/ping", z.object({ ok: z.boolean() }));
    expect(fetchMock).toHaveBeenLastCalledWith(
      "https://host-a.example.ts.net/api/ping",
      expect.anything(),
    );

    const uploadController = new AbortController();
    const progress = vi.fn();
    const uploadPromise = uploadForm(
      "/uploads",
      new File(["abc"], "a.txt"),
      z.object({ path: z.string() }),
      progress,
      uploadController.signal,
    );
    const xhr = MockXHR.instances[0]!;
    expect(xhr.url).toBe("https://host-a.example.ts.net/api/uploads");
    expect(xhr.headers.get("Authorization")).toBe("Bearer local-tok");
    xhr.upload.onprogress?.({ lengthComputable: true, loaded: 1, total: 2 });
    expect(progress).toHaveBeenCalledWith(50);
    xhr.response = { path: "/tmp/a.txt" };
    xhr.onload?.();
    await expect(uploadPromise).resolves.toEqual({ path: "/tmp/a.txt" });

    openSocket("/ws/sessions", false);
    expect(MockWebSocket.instances[1]?.url).toBe(
      "wss://host-a.example.ts.net/ws/sessions",
    );

    // Explicit standalone createHttpTransport with base path strips trailing slashes.
    const subpathTransport = createHttpTransport("http://100.64.0.2:7681/sub/");
    expect(subpathTransport.origin).toBe("http://100.64.0.2:7681/sub");
    subpathTransport.openStream("/ws/sessions", {
      token: "t",
      compress: true,
    });
    expect(MockWebSocket.instances[2]?.url).toBe(
      "ws://100.64.0.2:7681/sub/ws/sessions?compress=1",
    );
  });

  it("rejects invalid host base URLs", () => {
    expect(() => setHostBaseUrl("ftp://example.com")).toThrow(
      /Unsupported host base URL protocol/,
    );
    expect(() => setHostBaseUrl("https://example.com?token=1")).toThrow(
      /query or hash/,
    );
    expect(() => setHostBaseUrl("https://example.com#frag")).toThrow(
      /query or hash/,
    );
  });

  it("handles HTTP XHR network, timeout, and abort errors uniformly", async () => {
    vi.stubGlobal("XMLHttpRequest", MockXHR);
    const schema = z.object({ path: z.string() });
    const file = new File(["x"], "x.txt");

    // Pre-aborted signal rejects before creating XHR.
    const preAborted = new AbortController();
    preAborted.abort();
    await expect(
      uploadForm("/uploads", file, schema, vi.fn(), preAborted.signal),
    ).rejects.toMatchObject({ name: "AbortError" });
    expect(MockXHR.instances).toHaveLength(0);

    // Network error -> APIError(0, ...)
    const pNet = uploadForm(
      "/uploads",
      file,
      schema,
      vi.fn(),
      new AbortController().signal,
    );
    MockXHR.instances[0]!.onerror?.();
    await expect(pNet).rejects.toEqual(
      new APIError(0, "上传失败：网络连接异常"),
    );

    // Timeout -> APIError(0, ...)
    const pTimeout = uploadForm(
      "/uploads",
      file,
      schema,
      vi.fn(),
      new AbortController().signal,
    );
    MockXHR.instances[1]!.ontimeout?.();
    await expect(pTimeout).rejects.toEqual(new APIError(0, "上传超时"));

    // In-flight abort -> AbortError
    const inFlight = new AbortController();
    const pAbort = uploadForm(
      "/uploads",
      file,
      schema,
      vi.fn(),
      inFlight.signal,
    );
    inFlight.abort();
    await expect(pAbort).rejects.toMatchObject({ name: "AbortError" });
    expect(MockXHR.instances[2]!.aborted).toBe(true);
  });

  it("routes api(), apiBuffer(), apiBlob(), uploadFile(), and forwardLink() through a custom ClientTransport", async () => {
    // Ensure globals are never called when a custom transport is active.
    const forbiddenFetch = vi.fn();
    vi.stubGlobal("fetch", forbiddenFetch);

    setActiveHostId("host-p2p");
    store("token", "p2p-secret");

    const fetchCalls: { path: string; init: RequestInit }[] = [];
    const uploadCalls: UploadRequest[] = [];
    let nextUploadImpl: (req: UploadRequest) => Promise<UploadResult> = async (
      req,
    ) => {
      req.onProgress(100);
      return {
        status: 200,
        statusText: "OK",
        body: { path: "/remote/file.png", name: "file.png", size: 3 },
      };
    };

    const custom: ClientTransport = {
      origin: "https://p2p-peer.webmux.internal",
      async fetch(path, init) {
        fetchCalls.push({ path, init });
        if (path === "/unauthorized") {
          return new Response(JSON.stringify({ error: "认证失败" }), {
            status: 401,
            headers: { "Content-Type": "application/json" },
          });
        }
        if (path === "/sessions/aaaaaaaa/buffer") {
          return new Response(new Uint8Array([1, 2, 3, 4]), { status: 200 });
        }
        if (path === "/blob") {
          const stream = new ReadableStream<Uint8Array>({
            start(controller) {
              controller.enqueue(new Uint8Array([10, 20]));
              controller.enqueue(new Uint8Array([30]));
              controller.close();
            },
          });
          return new Response(stream, {
            status: 200,
            headers: { "Content-Type": "application/octet-stream" },
          });
        }
        return new Response(JSON.stringify({ ok: true }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      },
      upload(req) {
        uploadCalls.push(req);
        return nextUploadImpl(req);
      },
      openStream(path, options) {
        return new FakeStream(path, options);
      },
    };

    setClientTransport(custom);
    expect(currentTransport()).toBe(custom);
    expect(hostOrigin()).toBe("https://p2p-peer.webmux.internal");

    await expect(api("/check", z.object({ ok: z.boolean() }))).resolves.toEqual(
      { ok: true },
    );
    expect(fetchCalls[0]?.path).toBe("/check");
    expect(fetchCalls[0]?.init.headers).toEqual({
      Authorization: "Bearer p2p-secret",
      "Content-Type": "application/json",
    });

    const buf = await apiBuffer("aaaaaaaa");
    expect([...buf]).toEqual([1, 2, 3, 4]);

    const bytesProgress: number[] = [];
    const blob = await apiBlob("/blob", undefined, (n) =>
      bytesProgress.push(n),
    );
    expect(bytesProgress).toEqual([2, 3]);
    expect(blob.size).toBe(3);

    // 401 on custom transport dispatches webmux:auth and throws APIError(401).
    let authEvents = 0;
    windowTarget.addEventListener("webmux:auth", () => authEvents++);
    await expect(api("/unauthorized", z.unknown())).rejects.toMatchObject({
      status: 401,
      message: "认证失败",
    });
    expect(authEvents).toBe(1);

    // Upload success + progress over custom transport.
    const progress: number[] = [];
    const uploaded = await uploadFile(
      "aaaaaaaa",
      new File(["png"], "file.png"),
      (p) => progress.push(p),
      new AbortController().signal,
    );
    expect(uploaded.path).toBe("/remote/file.png");
    expect(progress).toEqual([100]);
    expect(uploadCalls[0]?.token).toBe("p2p-secret");

    // Upload 401 over custom transport also dispatches webmux:auth.
    nextUploadImpl = async () => ({
      status: 401,
      statusText: "Unauthorized",
      body: { error: "上传未授权" },
    });
    await expect(
      uploadFile(
        "aaaaaaaa",
        new File(["png"], "file.png"),
        vi.fn(),
        new AbortController().signal,
      ),
    ).rejects.toMatchObject({ status: 401, message: "上传未授权" });
    expect(authEvents).toBe(2);

    // TransportUploadError maps to APIError(0).
    nextUploadImpl = async () => {
      throw new TransportUploadError("timeout");
    };
    await expect(
      uploadFile(
        "aaaaaaaa",
        new File(["png"], "file.png"),
        vi.fn(),
        new AbortController().signal,
      ),
    ).rejects.toEqual(new APIError(0, "上传超时"));

    expect(forbiddenFetch).not.toHaveBeenCalled();
  });

  it("preserves ws.ts send budget, control coalescing, close codes, and reconnects over custom TransportSocket", () => {
    vi.useFakeTimers();
    vi.spyOn(Math, "random").mockReturnValue(0);
    store("token", "stream-tok");

    const streams: FakeStream[] = [];
    setClientTransport({
      origin: "https://p2p.internal",
      fetch: () => Promise.reject(new Error("unused")),
      upload: () => Promise.reject(new Error("unused")),
      openStream(path, options) {
        const s = new FakeStream(path, options);
        streams.push(s);
        return s;
      },
    });

    const states: string[] = [];
    const messages: (string | ArrayBuffer)[] = [];
    const conn = connect(
      "/ws/s/aaaaaaaa",
      (data) => messages.push(data),
      (state) => states.push(state),
      vi.fn(),
    );

    expect(streams).toHaveLength(1);
    expect(streams[0]!.path).toBe("/ws/s/aaaaaaaa");
    expect(streams[0]!.options).toEqual({
      token: "stream-tok",
      compress: false,
    });
    expect(states).toEqual(["connecting"]);

    streams[0]!.open();
    expect(states).toEqual(["connecting", "connected"]);

    // Send budget & control coalescing over FakeStream.
    streams[0]!.bufferedAmount = 1024 * 1024;
    expect(conn.send(0, new Uint8Array([65]))).toBe(false);
    expect(conn.send(1, new Uint8Array([80, 24]))).toBe(true);
    expect(conn.send(1, new Uint8Array([120, 40]))).toBe(true);
    streams[0]!.bufferedAmount = 0;
    vi.advanceTimersByTime(50);
    expect(streams[0]!.sent).toEqual([new Uint8Array([1, 120, 40])]);

    // Normal/slow close triggers reconnecting backoff.
    streams[0]!.serverClose(4408);
    expect(states.at(-1)).toBe("reconnecting");
    vi.advanceTimersByTime(250);
    expect(streams).toHaveLength(2);
    streams[1]!.open();

    // 4409 -> moved (stops automatic retry until explicit reconnect).
    streams[1]!.serverClose(4409);
    expect(states.at(-1)).toBe("moved");
    vi.advanceTimersByTime(10000);
    expect(streams).toHaveLength(2);

    conn.reconnect();
    expect(streams).toHaveLength(3);
    streams[2]!.open();

    // setWsCompression reattaches live streams with StreamOptions.compress = true.
    setWsCompression(true);
    expect(streams).toHaveLength(4);
    expect(streams[3]!.options.compress).toBe(true);
    streams[3]!.open();

    // 4410 -> exited; 4404 -> missing (permanent).
    streams[3]!.serverClose(4410);
    expect(states.at(-1)).toBe("exited");
    conn.reconnect();
    expect(streams).toHaveLength(5);
    streams[4]!.serverClose(4404);
    expect(states.at(-1)).toBe("missing");
    conn.reconnect();
    expect(streams).toHaveLength(5);

    conn.close();
  });
});
