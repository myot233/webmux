import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { commands, page, userEvent } from "vitest/browser";
import { cleanup, render } from "vitest-browser-svelte/pure";
import Harness from "./TerminalHarness.svelte";

declare module "vitest/internal/browser" {
  interface BrowserCommands {
    /** Real finger, iframe-local CSS points; release or cancel after the last. */
    nativeTouchGesture(
      points: { x: number; y: number }[],
      finish?: "end" | "cancel",
      holdMilliseconds?: number,
    ): Promise<void>;
  }
}
// Exercise the real transport framing, hook, xterm buffer and both renderers.
// This socket never opens a network connection or supplies a synthetic echo.
class FixtureSocket extends EventTarget {
  static readonly CONNECTING = 0;
  static readonly OPEN = 1;
  static readonly CLOSING = 2;
  static readonly CLOSED = 3;
  readonly url: string;
  binaryType = "blob";
  readyState = FixtureSocket.CONNECTING;
  onopen: ((event: Event) => void) | null = null;
  onmessage: ((event: MessageEvent) => void) | null = null;
  onclose: ((event: CloseEvent) => void) | null = null;
  onerror: ((event: Event) => void) | null = null;
  sent: Uint8Array[] = [];
  closeCount = 0;

  constructor(url: string | URL) {
    super();
    this.url = String(url);
    sockets.push(this);
  }

  open() {
    this.readyState = FixtureSocket.OPEN;
    this.onopen?.(new Event("open"));
  }

  receive(kind: number, text: string) {
    const payload = new TextEncoder().encode(text);
    const frame = new Uint8Array(payload.length + 1);
    frame[0] = kind;
    frame.set(payload, 1);
    this.onmessage?.(new MessageEvent("message", { data: frame.buffer }));
  }

  send(data: Uint8Array) {
    if (this.readyState !== FixtureSocket.OPEN) {
      throw new Error("Fixture socket is not open");
    }
    this.sent.push(new Uint8Array(data));
  }

  close() {
    this.closeCount++;
    this.readyState = FixtureSocket.CLOSED;
    this.onclose?.(new CloseEvent("close", { code: 1000 }));
  }
}

let sockets: FixtureSocket[];
let observations: ControlledObserver[];
let resolutionQueries: ResolutionQuery[];
let dpr: number;
let measurement: "consistent" | "css-pixels";
let glyphPixels: WeakMap<HTMLCanvasElement, number>;
let webglCanvases: WeakSet<HTMLCanvasElement>;
let resizeListeners: Set<EventListenerOrEventListenerObject>;
const nativeResizeObserver = window.ResizeObserver;
const nativeMatchMedia = window.matchMedia.bind(window);
let previousFont: string | null;
let touchTeardowns: (() => void)[];
let previousDark: boolean;

// Chromium's device emulator can change DPR without changing the actual
// physical-pixel observations. Only that browser API boundary is controlled;
// native ResizeObserver still determines when real layout changes are delivered.
function measuredEntry(
  target: Element,
  contentRect: DOMRectReadOnly,
  pixels?: ResizeObserverSize,
): ResizeObserverEntry {
  const size = pixels ?? {
    inlineSize: Math.round(
      contentRect.width * (measurement === "consistent" ? dpr : 1),
    ),
    blockSize: Math.round(
      contentRect.height * (measurement === "consistent" ? dpr : 1),
    ),
  };
  return {
    target,
    contentRect,
    contentBoxSize: [
      { inlineSize: contentRect.width, blockSize: contentRect.height },
    ],
    borderBoxSize: [
      { inlineSize: contentRect.width, blockSize: contentRect.height },
    ],
    devicePixelContentBoxSize: [size],
  };
}

class ControlledObserver implements ResizeObserver {
  readonly targets = new Set<Element>();
  readonly native: ResizeObserver;
  readonly callback: ResizeObserverCallback;
  disconnected = false;
  observeCount = 0;
  deliveryCount = 0;

  constructor(callback: ResizeObserverCallback) {
    this.callback = callback;
    this.native = new nativeResizeObserver((entries) => {
      this.deliveryCount++;
      callback(
        entries.map((entry) => measuredEntry(entry.target, entry.contentRect)),
        this,
      );
    });
    observations.push(this);
  }

  observe(target: Element, options?: ResizeObserverOptions) {
    this.disconnected = false;
    this.observeCount++;
    this.targets.add(target);
    this.native.observe(target, options);
  }

  unobserve(target: Element) {
    this.targets.delete(target);
    this.native.unobserve(target);
  }

  disconnect() {
    this.disconnected = true;
    this.targets.clear();
    this.native.disconnect();
  }

  deliver(entry: ResizeObserverEntry) {
    // Deliberately usable after disconnect: a queued browser callback must be safe.
    this.callback([entry], this);
  }
}

class ResolutionQuery extends EventTarget {
  readonly listeners = new Set<EventListenerOrEventListenerObject>();
  readonly legacyListeners = new Set<EventListener>();
  readonly media: string;
  readonly ratio: number;
  onchange: ((event: MediaQueryListEvent) => void) | null = null;

  constructor(media: string, ratio: number) {
    super();
    this.media = media;
    this.ratio = ratio;
  }

  get matches() {
    return this.ratio === dpr;
  }

  override addEventListener(
    type: string,
    callback: EventListenerOrEventListenerObject | null,
    options?: AddEventListenerOptions | boolean,
  ) {
    if (type === "change" && callback) this.listeners.add(callback);
    super.addEventListener(type, callback, options);
  }

  override removeEventListener(
    type: string,
    callback: EventListenerOrEventListenerObject | null,
    options?: EventListenerOptions | boolean,
  ) {
    if (type === "change" && callback) this.listeners.delete(callback);
    super.removeEventListener(type, callback, options);
  }

  addListener(callback: ((event: MediaQueryListEvent) => void) | null) {
    if (!callback) return;
    this.legacyListeners.add(callback as EventListener);
    super.addEventListener("change", callback as EventListener);
  }

  removeListener(callback: ((event: MediaQueryListEvent) => void) | null) {
    if (!callback) return;
    this.legacyListeners.delete(callback as EventListener);
    super.removeEventListener("change", callback as EventListener);
  }
}

function setDpr(value: number) {
  const changed = resolutionQueries.filter(
    (query) => query.matches !== (query.ratio === value),
  );
  dpr = value;
  vi.stubGlobal("devicePixelRatio", value);
  for (const query of changed) {
    const event = new MediaQueryListEvent("change", {
      media: query.media,
      matches: query.matches,
    });
    query.dispatchEvent(event);
    query.onchange?.(event);
  }
}

// Read actual glyph pixels immediately after the real GL draw, before Chromium
// discards a non-preserved drawing buffer. Rendering/context creation are not mocked.
function observeGlyphPixels(unavailable = false) {
  const getContext = HTMLCanvasElement.prototype.getContext;
  const seen = new WeakSet<WebGL2RenderingContext>();
  vi.spyOn(HTMLCanvasElement.prototype, "getContext").mockImplementation(
    function (this: HTMLCanvasElement, contextId: string, options?: unknown) {
      if (unavailable && contextId === "webgl2") return null;
      const context = getContext.call(this, contextId, options);
      if (contextId === "webgl2" && context) {
        const gl = context as WebGL2RenderingContext;
        webglCanvases.add(this);
        if (!seen.has(gl)) {
          seen.add(gl);
          const canvas = this;
          const draw = gl.drawElementsInstanced.bind(gl);
          vi.spyOn(gl, "drawElementsInstanced").mockImplementation(
            (...args) => {
              draw(...args);
              if (!canvas.isConnected || gl.isContextLost()) return;
              const width = gl.drawingBufferWidth;
              const height = Math.min(
                gl.drawingBufferHeight,
                Math.ceil(100 * dpr),
              );
              if (!width || !height) return;
              const pixels = new Uint8Array(width * height * 4);
              gl.readPixels(
                0,
                gl.drawingBufferHeight - height,
                width,
                height,
                gl.RGBA,
                gl.UNSIGNED_BYTE,
                pixels,
              );
              let red = 0;
              for (let index = 0; index < pixels.length; index += 4) {
                if (
                  pixels[index] > 180 &&
                  pixels[index + 1] < 80 &&
                  pixels[index + 2] < 80
                )
                  red++;
              }
              glyphPixels.set(canvas, red);
            },
          );
        }
      }
      return context;
    } as typeof HTMLCanvasElement.prototype.getContext,
  );
}

function host() {
  const element = document.querySelector<HTMLDivElement>(
    '[data-testid="terminal-host"]',
  );
  if (!element) throw new Error("Missing terminal host");
  return element;
}

function webglCanvas() {
  // Link overlays also use canvases; identify the one with a real GL context.
  return (
    Array.from(
      host().querySelectorAll<HTMLCanvasElement>(".xterm-screen > canvas"),
    ).find((canvas) => webglCanvases.has(canvas)) ?? null
  );
}

function hostObserver() {
  const observer = observations.find((observer) =>
    observer.targets.has(host()),
  );
  if (!observer) throw new Error("Missing terminal host observer");
  return observer;
}

function domText() {
  return host().querySelector(".xterm-rows")?.textContent ?? "";
}

async function expectDomText(text: string) {
  await expect.poll(domText).toContain(text);
  const rows = host().querySelector<HTMLElement>(".xterm-rows")!;
  await expect.element(page.elementLocator(rows)).toBeVisible();
  expect(rows.getBoundingClientRect().width).toBeGreaterThan(0);
  expect(rows.getBoundingClientRect().height).toBeGreaterThan(0);
  expect(webglCanvas()).toBeNull();
}

async function expectGlyphs() {
  await expect
    .poll(() => {
      const canvas = webglCanvas();
      return canvas ? (glyphPixels.get(canvas) ?? 0) : 0;
    })
    .toBeGreaterThan(100);
  const canvas = webglCanvas()!;
  await expect.element(page.elementLocator(canvas)).toBeVisible();
  const bounds = canvas.getBoundingClientRect();
  expect(Math.abs(canvas.width - bounds.width * dpr)).toBeLessThanOrEqual(1);
  expect(Math.abs(canvas.height - bounds.height * dpr)).toBeLessThanOrEqual(1);
  return canvas;
}

const prompt = "fixture@mobile:~/project$ omp";
const snapshot = `\x1b[38;2;255;0;0m${prompt}\x1b[0m\r\n`;

async function openTerminal() {
  const rendered = await render(Harness);
  // The terminal waits for the bundled monospace face before opening a socket.
  await expect.poll(() => sockets.length, { timeout: 5000 }).toBe(1);
  const socket = sockets[0];
  socket.open();
  socket.receive(1, snapshot);
  await expect
    .element(page.getByRole("status", { name: "Terminal connection" }))
    .toHaveTextContent("connected");
  await expect
    .element(page.getByRole("status", { name: "Terminal restoration" }))
    .toHaveTextContent("ready");
  return { socket, rendered };
}

function expectNoSessionMutation(socket: FixtureSocket) {
  expect(sockets).toEqual([socket]);
  expect(socket.closeCount).toBe(0);
  // Resize frames are legitimate; input and resync frames are not.
  expect(socket.sent.length).toBeGreaterThan(0);
  expect(socket.sent.every((frame) => frame[0] === 1)).toBe(true);
}

function inputText(socket: FixtureSocket) {
  return socket.sent
    .filter((frame) => frame[0] === 0)
    .map((frame) => new TextDecoder().decode(frame.subarray(1)))
    .join("");
}

function geometry(socket: FixtureSocket) {
  const resize = socket.sent.filter((frame) => frame[0] === 1).at(-1)!;
  const { cols, rows } = JSON.parse(
    new TextDecoder().decode(resize.subarray(1)),
  ) as { cols: number; rows: number };
  const bounds = host().querySelector(".xterm-screen")!.getBoundingClientRect();
  return {
    row: bounds.height / rows,
    cell: bounds.width / cols,
    bounds,
  };
}

function gestureStart(socket: FixtureSocket, column = 8, row = 12) {
  const { bounds, cell, row: height } = geometry(socket);
  return {
    x: bounds.left + (column - 0.5) * cell,
    y: bounds.top + (row - 0.5) * height,
  };
}

async function swipe(
  socket: FixtureSocket,
  lines: number,
  start = gestureStart(socket),
  horizontal = 0,
) {
  const height = geometry(socket).row;
  // Every step must clear the 8 px long-press jitter tolerance. Smaller steps
  // let a slow runner reach the 500 ms long press mid-swipe and select instead.
  const travel = Math.hypot(horizontal, lines * height);
  const steps = Math.max(1, Math.min(8, Math.floor(travel / 8)));
  await commands.nativeTouchGesture(
    Array.from({ length: steps + 1 }, (_, index) => ({
      x: start.x + (horizontal * index) / steps,
      y: start.y + (lines * height * index) / steps,
    })),
  );
}

function historyLine(line: number) {
  return `history-${String(line).padStart(4, "0")}`;
}

function firstHistoryLine() {
  const text = host().querySelector(".xterm-rows > div")?.textContent ?? "";
  return Number(/history-(\d+)/.exec(text)?.[1] ?? NaN);
}

async function populateHistory(socket: FixtureSocket) {
  // More than three viewports: assertions read the actual visible DOM renderer.
  socket.receive(
    1,
    Array.from({ length: 160 }, (_, index) => historyLine(index + 1)).join(
      "\r\n",
    ),
  );
  await expectDomText(historyLine(160));
  expect(firstHistoryLine()).toBeGreaterThan(1);
  return firstHistoryLine();
}

function blurAndTrackFocus() {
  const textarea = host().querySelector<HTMLTextAreaElement>(
    ".xterm-helper-textarea",
  )!;
  textarea.blur();
  const focused = vi.fn();
  textarea.addEventListener("focus", focused);
  touchTeardowns.push(() => textarea.removeEventListener("focus", focused));
  return { textarea, focused };
}

function pointer(
  target: HTMLElement,
  type: string,
  id: number,
  point: { x: number; y: number },
) {
  target.dispatchEvent(
    new PointerEvent(type, {
      bubbles: true,
      cancelable: true,
      pointerType: "touch",
      pointerId: id,
      clientX: point.x,
      clientY: point.y,
      buttons: type === "pointerup" ? 0 : 1,
    }),
  );
}

function syntheticPointerCapture(target: HTMLElement) {
  // Synthetic pointer ids cannot be captured by Chromium. Only that browser
  // boundary is replaced; the real hook, fit, rendering and xterm protocol run.
  return {
    set: vi.spyOn(target, "setPointerCapture").mockImplementation(() => {}),
    release: vi
      .spyOn(target, "releasePointerCapture")
      .mockImplementation(() => {}),
  };
}

beforeEach(async () => {
  sockets = [];
  previousDark = document.documentElement.classList.contains("dark");
  previousFont = localStorage.getItem("webmux.fontSize");
  localStorage.setItem("webmux.fontSize", "14");
  touchTeardowns = [];
  observations = [];
  resolutionQueries = [];
  resizeListeners = new Set();
  glyphPixels = new WeakMap();
  webglCanvases = new WeakSet();
  dpr = 1;
  measurement = "consistent";
  await page.viewport(412, 924);
  const addWindowListener = window.addEventListener.bind(window);
  vi.spyOn(window, "addEventListener").mockImplementation(
    (type, listener, options) => {
      if (type === "resize" && listener) resizeListeners.add(listener);
      addWindowListener(type, listener, options);
    },
  );
  vi.stubGlobal("WebSocket", FixtureSocket);
  vi.stubGlobal("ResizeObserver", ControlledObserver);
  vi.stubGlobal("devicePixelRatio", dpr);
  vi.stubGlobal("matchMedia", (query: string) => {
    const resolution = /\(resolution:\s*([\d.]+)dppx\)/.exec(query);
    if (!resolution) return nativeMatchMedia(query);
    const result = new ResolutionQuery(query, Number(resolution[1]));
    resolutionQueries.push(result);
    return result as unknown as MediaQueryList;
  });
});

afterEach(async () => {
  try {
    await cleanup();
    await userEvent.cleanup();
    expect(
      observations
        .filter((observer) => observer.targets.size > 0)
        .map((observer) =>
          [...observer.targets].map((target) => target.className),
        ),
    ).toEqual([]);
    // Assert the hook's modern listeners. xterm 5.5's legacy ScreenDprMonitor
    // is not registered for disposal; isolate that dependency in teardown.
    expect(
      resolutionQueries
        .filter((query) => query.listeners.size > 0)
        .map((query) => ({
          media: query.media,
          listeners: query.listeners.size,
        })),
    ).toEqual([]);
    expect(sockets.map((socket) => socket.closeCount)).toEqual(
      sockets.map(() => 1),
    );
  } finally {
    for (const teardown of touchTeardowns) teardown();
    document.documentElement.classList.toggle("dark", previousDark);
    if (previousFont === null) localStorage.removeItem("webmux.fontSize");
    else localStorage.setItem("webmux.fontSize", previousFont);
    for (const observer of observations) observer.native.disconnect();
    for (const query of resolutionQueries) {
      for (const listener of [...query.listeners])
        query.removeEventListener("change", listener);
      for (const listener of [...query.legacyListeners])
        query.removeListener(listener);
    }
    for (const listener of resizeListeners)
      window.removeEventListener("resize", listener);
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  }
});

test("mobile high-DPR CSS-pixel observations render the restored prompt and retain working input", async () => {
  setDpr(2.625);
  measurement = "css-pixels";
  observeGlyphPixels();
  const { socket } = await openTerminal();
  await expectDomText(prompt);
  expectNoSessionMutation(socket);

  await page.getByRole("button", { name: "Focus terminal input" }).click();
  const textarea = host().querySelector<HTMLTextAreaElement>(
    ".xterm-helper-textarea",
  )!;
  expect(document.activeElement).toBe(textarea);
  expect(textarea.disabled).toBe(false);
  await userEvent.keyboard("x");
  await expect
    .poll(() => socket.sent.filter((frame) => frame[0] === 0).length)
    .toBe(1);
  const input = socket.sent.find((frame) => frame[0] === 0)!;
  expect(new TextDecoder().decode(input.subarray(1))).toBe("x");
  expect(sockets).toEqual([socket]);
  expect(socket.closeCount).toBe(0);
});

test("desktop to incompatible high-DPR fallback preserves rendered history and the same socket", async () => {
  observeGlyphPixels();
  const { socket, rendered } = await openTerminal();
  const canvas = await expectGlyphs();
  const snapshotPixels = glyphPixels.get(canvas)!;
  socket.receive(0, "output before DPR transition\r\n");
  socket.receive(0, "\x1b[38;2;255;0;0mvisible history\x1b[0m\r\n");
  // Additional red glyph pixels prove new output was actually drawn, not just queued.
  await expect
    .poll(() => glyphPixels.get(canvas) ?? 0)
    .toBeGreaterThan(snapshotPixels + 100);

  const observer = hostObserver();
  const observeCount = observer.observeCount;
  measurement = "css-pixels";
  setDpr(2.625);
  expect(observer.observeCount).toBeGreaterThan(observeCount);
  await expectDomText(prompt);
  await expectDomText("output before DPR transition");
  await expectDomText("visible history");
  socket.receive(0, "output after DPR transition\r\n");
  await expectDomText("output after DPR transition");
  expectNoSessionMutation(socket);

  const oldHost = host();
  const lateEntry = measuredEntry(oldHost, oldHost.getBoundingClientRect());
  const oldQueries = [...resolutionQueries];
  await rendered.unmount();
  expect(observer.disconnected).toBe(true);
  expect(socket.closeCount).toBe(1);
  const sentCount = socket.sent.length;
  const unmountedObserveCount = observer.observeCount;
  expect(() => observer.deliver(lateEntry)).not.toThrow();
  for (const query of oldQueries) query.dispatchEvent(new Event("change"));
  expect(socket.sent).toHaveLength(sentCount);
  expect(sockets).toEqual([socket]);
  expect(observer.observeCount).toBe(unmountedObserveCount);
  expect(oldQueries.every((query) => query.listeners.size === 0)).toBe(true);
});

test("fractional DPR, one-pixel rounding, hidden boxes and absent physical measurements keep real WebGL glyphs", async () => {
  setDpr(1.25);
  observeGlyphPixels();
  const { socket } = await openTerminal();
  const canvas = await expectGlyphs();
  const observer = hostObserver();
  const bounds = host().getBoundingClientRect();
  observer.deliver(
    measuredEntry(host(), bounds, {
      inlineSize: bounds.width * dpr + 1,
      blockSize: Math.round(bounds.height * dpr),
    }),
  );
  observer.deliver(
    measuredEntry(host(), new DOMRect(0, 0, 0, 0), {
      inlineSize: 0,
      blockSize: 0,
    }),
  );
  const { devicePixelContentBoxSize: _physical, ...unsupported } =
    measuredEntry(host(), bounds);
  observer.deliver(unsupported as ResizeObserverEntry);
  glyphPixels.delete(canvas);
  socket.receive(
    0,
    "\x1b[38;2;255;0;0mstill rendered at fractional DPR\x1b[0m\r\n",
  );
  expect(await expectGlyphs()).toBe(canvas);

  const deliveries = observer.deliveryCount;
  setDpr(2.625);
  await expect.poll(() => observer.deliveryCount).toBeGreaterThan(deliveries);
  glyphPixels.delete(canvas);
  socket.receive(
    0,
    "\x1b[38;2;255;0;0mconsistent fresh physical pixels\x1b[0m\r\n",
  );
  expect(await expectGlyphs()).toBe(canvas);
  expectNoSessionMutation(socket);
});

test("unavailable WebGL renders snapshot and later output visibly with the DOM renderer", async () => {
  setDpr(2.625);
  observeGlyphPixels(true);
  const { socket } = await openTerminal();
  await expectDomText(prompt);
  socket.receive(0, "output without WebGL\r\n");
  await expectDomText("output without WebGL");
  expectNoSessionMutation(socket);
});

test("real WebGL context loss falls back visibly without discarding existing history", async () => {
  observeGlyphPixels();
  const { socket } = await openTerminal();
  const canvas = await expectGlyphs();
  const gl = canvas.getContext("webgl2")!;
  const extension = gl.getExtension("WEBGL_lose_context");
  expect(extension).not.toBeNull();
  extension!.loseContext();
  // addon-webgl allows three seconds for context restoration before fallback.
  await expect.poll(domText, { timeout: 10000 }).toContain(prompt);
  await expectDomText(prompt);
  socket.receive(0, "output after real context loss\r\n");
  await expectDomText("output after real context loss");
  expectNoSessionMutation(socket);
}, 15000);

test("native Chrome swipes keep scrolling visibly through row redraws without keyboard, duplicate scroll or outer-page pan", async () => {
  observeGlyphPixels(true);
  const { socket } = await openTerminal();
  const bottom = await populateHistory(socket);
  const { textarea, focused } = blurAndTrackFocus();
  const spacer = document.createElement("div");
  spacer.style.height = "2000px";
  document.body.append(spacer);
  touchTeardowns.push(() => {
    spacer.remove();
    window.scrollTo(0, 0);
  });
  expect(document.documentElement.scrollHeight).toBeGreaterThan(innerHeight);
  const outerY = scrollY;
  const events: string[] = [];
  const record = (event: Event) => {
    if (event.isTrusted) events.push(event.type);
  };
  const types = ["pointermove", "pointerup", "pointercancel", "touchcancel"];
  for (const type of types) document.addEventListener(type, record, true);
  touchTeardowns.push(() => {
    for (const type of types) document.removeEventListener(type, record, true);
  });

  const originalRow = host().querySelector(".xterm-rows > div > span")!;
  await swipe(socket, 4.25);
  await expect.poll(firstHistoryLine).toBe(bottom - 4);
  await expectDomText(historyLine(bottom - 4));
  // xterm replaces the hit-tested span during scroll. Pointer capture must keep
  // receiving the rest of the real finger, including its final release.
  expect(originalRow.isConnected).toBe(false);
  expect(
    events.filter((type) => type === "pointermove").length,
  ).toBeGreaterThanOrEqual(7);
  expect(events.filter((type) => type === "pointerup")).toHaveLength(1);
  expect(events.filter((type) => type.endsWith("cancel"))).toEqual([]);

  await swipe(socket, -4.25, gestureStart(socket, 8, 30));
  await expect.poll(firstHistoryLine).toBe(bottom);
  // A fresh gesture must discard the previous quarter-row remainder.
  await swipe(socket, 0.8);
  expect(firstHistoryLine()).toBe(bottom);
  for (let index = 0; index < Math.ceil(bottom / 20); index++) {
    await swipe(socket, 20.25, gestureStart(socket, 8, 6));
  }
  await expect.poll(firstHistoryLine).toBe(1);
  await expectDomText(historyLine(1));
  await swipe(socket, 8.25);
  expect(firstHistoryLine()).toBe(1);
  for (let index = 0; index < Math.ceil(bottom / 20); index++) {
    await swipe(socket, -20.25, gestureStart(socket, 8, 30));
  }
  await expect.poll(firstHistoryLine).toBe(bottom);
  await expectDomText(historyLine(160));
  await swipe(socket, -8.25, gestureStart(socket, 8, 30));
  expect(firstHistoryLine()).toBe(bottom);
  expect(scrollY).toBe(outerY);
  expect(events.filter((type) => type.endsWith("cancel"))).toEqual([]);
  expect(document.activeElement).not.toBe(textarea);
  expect(focused).not.toHaveBeenCalled();
  expectNoSessionMutation(socket);
  // About 20 real swipes, each waiting two animation frames per touch step:
  // ~10 s on Linux CI, over 20 s on hosted macOS runners.
}, 60000);

test("native fullscreen gestures report row-scaled SGR wheel input at stable starting cells", async () => {
  observeGlyphPixels(true);
  const { socket } = await openTerminal();
  socket.receive(0, "\x1b[?1049h\x1b[?1000h\x1b[?1006hmouse-ready");
  await expectDomText("mouse-ready");
  const { textarea, focused } = blurAndTrackFocus();
  const start = gestureStart(socket, 8, 12);

  // Horizontal finger drift and vertical row changes must not move the
  // application's wheel anchor. Exact protocol equality excludes clicks,
  // cursor arrows, malformed reports and duplicate xterm touch scrolling.
  await swipe(socket, 3.25, start, geometry(socket).cell * 4);
  expect(inputText(socket)).toBe("\x1b[<64;8;12M".repeat(3));
  await swipe(socket, -6.25, start, -geometry(socket).cell * 4);
  expect(inputText(socket)).toBe(
    "\x1b[<64;8;12M".repeat(3) + "\x1b[<65;8;12M".repeat(6),
  );
  expect(document.activeElement).not.toBe(textarea);
  expect(focused).not.toHaveBeenCalled();
});

test("native alternate-buffer wheel fallback honors cursor-key mode and normal-buffer restoration", async () => {
  observeGlyphPixels(true);
  const { socket } = await openTerminal();
  const bottom = await populateHistory(socket);
  const { focused } = blurAndTrackFocus();
  socket.receive(0, "\x1b[?1049h\x1b[?1lalt-normal");
  await expectDomText("alt-normal");
  await swipe(socket, 2.25);
  await swipe(socket, -3.25);
  expect(inputText(socket)).toBe("\x1b[A".repeat(2) + "\x1b[B".repeat(3));

  socket.receive(0, "\x1b[?1h\r\nalt-application");
  await expectDomText("alt-application");
  await swipe(socket, 4.25);
  await swipe(socket, -2.25);
  expect(inputText(socket)).toBe(
    "\x1b[A".repeat(2) +
      "\x1b[B".repeat(3) +
      "\x1bOA".repeat(4) +
      "\x1bOB".repeat(2),
  );
  const beforeRestore = inputText(socket);
  socket.receive(0, "\x1b[?1049l");
  await expectDomText(historyLine(160));
  await swipe(socket, 5.25);
  await expect.poll(firstHistoryLine).toBe(bottom - 5);
  await expectDomText(historyLine(bottom - 5));
  await swipe(socket, -5.25);
  await expect.poll(firstHistoryLine).toBe(bottom);
  expect(inputText(socket)).toBe(beforeRestore);
  expect(focused).not.toHaveBeenCalled();
});

test("pinch resizes real cells and rebases one-finger scrolling, while cancel and capture loss discard ghost gestures", async () => {
  observeGlyphPixels(true);
  const { socket, rendered } = await openTerminal();
  socket.receive(0, "\x1b[?1049h\x1b[?1000h\x1b[?1006hedge-ready");
  await expectDomText("edge-ready");
  const target = host();
  const { textarea, focused } = blurAndTrackFocus();
  const start = gestureStart(socket);
  const originalHeight = geometry(socket).row;
  await commands.nativeTouchGesture(
    [start, { x: start.x, y: start.y + originalHeight * 0.8 }],
    "cancel",
  );
  expect(inputText(socket)).toBe("");
  expect(focused).not.toHaveBeenCalled();
  const capture = syntheticPointerCapture(target);
  pointer(target, "pointerdown", 1, start);
  const first = { x: start.x, y: start.y + originalHeight * 0.8 };
  pointer(target, "pointermove", 1, first);
  pointer(target, "pointerdown", 2, { x: first.x + 100, y: first.y });
  pointer(target, "pointermove", 2, { x: first.x + 130, y: first.y });
  await expect.poll(() => geometry(socket).row).toBeGreaterThan(originalHeight);
  await expectDomText("edge-ready");
  expect(Number(localStorage.getItem("webmux.fontSize"))).toBeGreaterThan(14);
  pointer(target, "pointerup", 2, { x: first.x + 130, y: first.y });
  expect(inputText(socket)).toBe("");
  const height = geometry(socket).row;
  pointer(target, "pointermove", 1, { x: first.x, y: first.y + height * 0.6 });
  expect(inputText(socket)).toBe("");
  pointer(target, "pointermove", 1, { x: first.x, y: first.y + height * 1.2 });
  expect(inputText(socket)).toMatch(/^\x1b\[<64;\d+;\d+M$/);
  pointer(target, "pointerup", 1, { x: first.x, y: first.y + height * 1.2 });
  expect(focused).not.toHaveBeenCalled();

  for (const cancellation of ["pointercancel", "lostpointercapture"]) {
    const before = inputText(socket);
    const fresh = gestureStart(socket);
    pointer(target, "pointerdown", 1, fresh);
    pointer(target, "pointermove", 1, {
      x: fresh.x,
      y: fresh.y + height * 0.8,
    });
    pointer(target, cancellation, 1, fresh);
    pointer(target, "pointermove", 1, { x: fresh.x, y: fresh.y + height * 12 });
    pointer(target, "pointerup", 1, fresh);
    expect(inputText(socket)).toBe(before);
    expect(focused).not.toHaveBeenCalled();

    pointer(target, "pointerdown", 1, fresh);
    pointer(target, "pointermove", 1, {
      x: fresh.x,
      y: fresh.y + height * 0.4,
    });
    expect(inputText(socket)).toBe(before);
    pointer(target, "pointermove", 1, {
      x: fresh.x,
      y: fresh.y + height * 1.2,
    });
    pointer(target, "pointerup", 1, fresh);
    expect(inputText(socket).slice(before.length)).toMatch(
      /^\x1b\[<64;\d+;\d+M$/,
    );
  }

  expect(focused).not.toHaveBeenCalled();
  expect(document.activeElement).not.toBe(textarea);
  capture.set.mockRestore();
  capture.release.mockRestore();
  socket.receive(0, "\x1b[?1000l\x1b[?1006l\x1b[?1049l");
  await expectDomText(prompt);
  const beforeTap = inputText(socket);
  await commands.nativeTouchGesture([gestureStart(socket)]);
  expect(document.activeElement).toBe(textarea);
  expect(focused).toHaveBeenCalledTimes(1);
  expect(inputText(socket)).toBe(beforeTap);
  syntheticPointerCapture(target);

  // A live gesture disposed before its release must not keep any socket or
  // focus effects attached to the old host, even with queued late events.
  pointer(target, "pointerdown", 1, start);
  await rendered.unmount();
  const sent = socket.sent.length;
  for (const type of [
    "pointermove",
    "pointercancel",
    "pointerup",
    "lostpointercapture",
  ]) {
    pointer(target, type, 1, { x: start.x, y: start.y + height * 12 });
  }
  pointer(target, "pointerdown", 2, start);
  pointer(target, "pointerup", 2, start);
  expect(socket.sent).toHaveLength(sent);
  expect(socket.closeCount).toBe(1);
  expect(focused).toHaveBeenCalledTimes(1);
});

test("terminal measures the bundled monospace face before painting", async () => {
  observeGlyphPixels(true);
  await openTerminal();
  const rows = host().querySelector<HTMLElement>(".xterm-rows");
  expect(rows).toBeTruthy();
  expect(getComputedStyle(rows!).fontFamily).toContain("Maple Mono NF CN");
  expect(document.fonts.check('14px "Maple Mono NF CN"')).toBe(true);
});

test("explicit black ANSI input remains readable in dark theme and updates on theme changes", async () => {
  observeGlyphPixels(true);
  document.documentElement.classList.add("dark");
  const { socket } = await openTerminal();
  socket.receive(1, "\x1b[38;2;0;0;0m中文 input\x1b[0m");
  await expectDomText("中文 input");
  const ratio = () => {
    const span = [
      ...host().querySelectorAll<HTMLElement>(".xterm-rows span"),
    ].find((element) => element.textContent?.includes("input"))!;
    const luminance = (color: string) => {
      const rgb = color
        .match(/\d+/g)!
        .slice(0, 3)
        .map((value) => {
          const channel = Number(value) / 255;
          return channel <= 0.04045
            ? channel / 12.92
            : ((channel + 0.055) / 1.055) ** 2.4;
        });
      return rgb[0] * 0.2126 + rgb[1] * 0.7152 + rgb[2] * 0.0722;
    };
    const foreground = luminance(getComputedStyle(span).color);
    const background = luminance(
      getComputedStyle(host().querySelector(".xterm")!).backgroundColor,
    );
    return (
      (Math.max(foreground, background) + 0.05) /
      (Math.min(foreground, background) + 0.05)
    );
  };
  await expect.poll(ratio).toBeGreaterThanOrEqual(4.49);
  document.documentElement.classList.remove("dark");
  await expect.poll(ratio).toBeGreaterThanOrEqual(4.49);
  document.documentElement.classList.add("dark");
  await expect.poll(ratio).toBeGreaterThanOrEqual(4.49);
});

test("native long press selects wide text and dragging copies multiple lines without terminal input or keyboard", async () => {
  observeGlyphPixels(true);
  const { socket } = await openTerminal();
  socket.receive(1, "prefix 中文 tail\r\nsecond line");
  await expectDomText("second line");
  const { textarea, focused } = blurAndTrackFocus();
  // Column 9 is the trailing half of 中, not a separate blank cell.
  const start = gestureStart(socket, 9, 1);
  await commands.nativeTouchGesture([start], "end", 650);
  await expect
    .element(page.getByRole("status", { name: "Terminal selection" }))
    .toHaveTextContent("中文");
  await page.getByRole("button", { name: "Copy selection" }).click();
  await expect
    .element(page.getByRole("status", { name: "Copied terminal text" }))
    .toHaveTextContent("中文");
  await commands.nativeTouchGesture(
    [start, gestureStart(socket, 6, 2)],
    "end",
    650,
  );
  await expect
    .poll(
      () =>
        document.querySelector('[aria-label="Terminal selection"]')
          ?.textContent,
    )
    .toBe("中文 tail\nsecond");
  await page.getByRole("button", { name: "Copy selection" }).click();
  await expect
    .poll(
      () =>
        document.querySelector('[aria-label="Copied terminal text"]')
          ?.textContent,
    )
    .toBe("中文 tail\nsecond");
  expect(inputText(socket)).toBe("");
  expect(document.activeElement).not.toBe(textarea);
  expect(focused).not.toHaveBeenCalled();
  await page.getByRole("button", { name: "Clear selection" }).click();
  await expect
    .element(page.getByRole("status", { name: "Terminal selection" }))
    .toHaveTextContent("");
});
