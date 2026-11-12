// pdf.js integration for the code panel's PDF preview. Only CodePdfView
// imports this module, dynamically, so pdf.js, its worker and data files stay
// out of the initial bundle and load the first time a PDF is opened.
//
// The legacy build carries polyfills for the browser baseline in
// vite.config.ts; the modern build needs very recent built-ins.
import {
  getDocument,
  GlobalWorkerOptions,
  PixelsPerInch,
  TextLayer,
  type PDFDocumentLoadingTask,
  type PDFDocumentProxy,
  type PDFPageProxy,
  type PageViewport,
} from "pdfjs-dist/legacy/build/pdf.mjs";
import workerUrl from "pdfjs-dist/legacy/build/pdf.worker.min.mjs?url";

export type { PDFDocumentProxy };
import type { CodePdfState } from "../../contracts/schemas";

GlobalWorkerOptions.workerSrc = workerUrl;

// CMaps, standard fonts and image decoders are emitted as hashed build assets
// (never inlined, never fetched from a CDN); the map turns pdf.js's fixed
// file names into those URLs.
const bundled = import.meta.glob<string>(
  [
    "/node_modules/pdfjs-dist/cmaps/*.bcmap",
    "/node_modules/pdfjs-dist/standard_fonts/*.{pfb,ttf}",
    "/node_modules/pdfjs-dist/wasm/{openjpeg,jbig2}.wasm",
  ],
  { query: "?url&no-inline", import: "default", eager: true },
);
const folders: Record<string, string> = {
  cMapUrl: "cmaps",
  standardFontDataUrl: "standard_fonts",
  wasmUrl: "wasm",
};

// pdf.js instantiates this with its URL options; the worker asks the main
// thread for data files because useWorkerFetch is off.
class BundledDataFactory {
  async fetch({ kind, filename }: { kind: string; filename: string }) {
    const url =
      bundled[`/node_modules/pdfjs-dist/${folders[kind]}/${filename}`];
    if (!url) throw new Error(`PDF 资源不存在：${filename}`);
    const response = await fetch(url);
    if (!response.ok) throw new Error(`PDF 资源读取失败：${filename}`);
    return new Uint8Array(await response.arrayBuffer());
  }
}

// CSS pixels per PDF point at 100 %.
export const cssUnits = PixelsPerInch.PDF_TO_CSS_UNITS;

// Opens bytes already read from the snapshot. Aborting the signal destroys
// the loading task, which also terminates parsing in the worker.
export function openPdf(data: ArrayBuffer, signal: AbortSignal) {
  const task: PDFDocumentLoadingTask = getDocument({
    data: new Uint8Array(data),
    enableXfa: false,
    useWorkerFetch: false,
    BinaryDataFactory: BundledDataFactory,
  });
  const abort = () => void task.destroy();
  signal.addEventListener("abort", abort, { once: true });
  return {
    promise: task.promise,
    destroy: () => {
      signal.removeEventListener("abort", abort);
      return task.destroy();
    },
  };
}

// Reader-facing explanation of a failed load.
export function pdfLoadError(error: unknown) {
  return error instanceof Error && error.name === "PasswordException"
    ? "PDF 受密码保护，请下载后查看"
    : "PDF 无法解析";
}

export const pageViewport = (page: PDFPageProxy, zoom: number) =>
  page.getViewport({ scale: zoom * cssUnits });

// Renders a page into `canvas` at the device pixel ratio capped at 2 (and at
// a total pixel budget), returning a cancel function.
export function renderCanvas(
  page: PDFPageProxy,
  canvas: HTMLCanvasElement,
  viewport: PageViewport,
) {
  const maxPixels = 16_777_216;
  const ratio = Math.min(
    globalThis.devicePixelRatio || 1,
    2,
    Math.sqrt(maxPixels / (viewport.width * viewport.height)),
  );
  canvas.width = Math.floor(viewport.width * ratio);
  canvas.height = Math.floor(viewport.height * ratio);
  const task = page.render({
    canvas,
    viewport,
    transform: ratio === 1 ? undefined : [ratio, 0, 0, ratio, 0, 0],
  });
  return { promise: task.promise, cancel: () => task.cancel() };
}

// Builds the selectable text layer inside `container`.
export function renderText(
  page: PDFPageProxy,
  container: HTMLElement,
  viewport: PageViewport,
) {
  const layer = new TextLayer({
    textContentSource: page.streamTextContent(),
    container,
    viewport,
  });
  return { promise: layer.render(), cancel: () => layer.cancel() };
}

export type PdfLink = {
  // Percentages of the page box, so links follow zoom without recomputing.
  left: number;
  top: number;
  width: number;
  height: number;
} & ({ page: number } | { url: string });

async function destinationPage(doc: PDFDocumentProxy, dest: unknown) {
  const explicit =
    typeof dest === "string" ? await doc.getDestination(dest) : dest;
  if (!Array.isArray(explicit)) return;
  const [target] = explicit as unknown[];
  if (typeof target === "number") return target + 1;
  if (target && typeof target === "object" && "num" in target)
    return (await doc.getPageIndex(target as { num: number; gen: number })) + 1;
}

// Link annotations only: internal destinations jump to a page; http(s) and
// mailto open a new tab; anything else (scripts, launch, forms) is dropped.
export async function pageLinks(
  doc: PDFDocumentProxy,
  page: PDFPageProxy,
): Promise<PdfLink[]> {
  const viewport = page.getViewport({ scale: 1 });
  const links: PdfLink[] = [];
  for (const annotation of await page.getAnnotations({ intent: "display" })) {
    if (annotation.subtype !== "Link") continue;
    const [rx1, ry1, rx2, ry2] = annotation.rect as number[];
    const [x1 = 0, y1 = 0] = viewport.convertToViewportPoint(rx1!, ry1!);
    const [x2 = 0, y2 = 0] = viewport.convertToViewportPoint(rx2!, ry2!);
    const box = {
      left: (Math.min(x1, x2) / viewport.width) * 100,
      top: (Math.min(y1, y2) / viewport.height) * 100,
      width: (Math.abs(x2 - x1) / viewport.width) * 100,
      height: (Math.abs(y2 - y1) / viewport.height) * 100,
    };
    const url = typeof annotation.url === "string" ? annotation.url : "";
    if (/^(?:https?|mailto):/i.test(url)) links.push({ ...box, url });
    else if (annotation.dest) {
      const target = await destinationPage(doc, annotation.dest).catch(
        () => undefined,
      );
      if (target && target <= doc.numPages)
        links.push({ ...box, page: target });
    }
  }
  return links;
}

const minPdfZoom = 0.25;
const maxPdfZoom = 5;
const pdfZoomStep = 1.25;
const pdfPadding = 8;
const clampPdfZoom = (value: number) =>
  Math.min(maxPdfZoom, Math.max(minPdfZoom, value));

export function measurePdfReading(
  element: HTMLElement | undefined,
  phase: "loading" | "ready" | "failed",
  fitted: boolean,
  zoom: number,
  known: CodePdfState,
): CodePdfState {
  if (phase !== "ready" || !element || element.clientHeight === 0) return known;
  const pages = element.querySelectorAll<HTMLElement>("[data-pdf-page]");
  if (!pages.length) return known;
  // The page crossing the top edge, found by its offset like `track`.
  const line = element.scrollTop + pdfPadding;
  let low = 0,
    high = pages.length - 1;
  while (low < high) {
    const middle = Math.ceil((low + high) / 2);
    if (pages[middle]!.offsetTop <= line + 1) low = middle;
    else high = middle - 1;
  }
  const box = pages[low]!;
  return {
    fitted,
    zoom,
    page: low + 1,
    offset:
      box.offsetHeight > 0
        ? Math.min(1, Math.max(0, (line - box.offsetTop) / box.offsetHeight))
        : 0,
  };
}

export function attachPdfCanvas(
  module: {
    pageViewport: typeof pageViewport;
    renderCanvas: typeof renderCanvas;
  },
  scale: number,
  loading: Promise<PDFPageProxy> | undefined,
  sizes: { width: number; height: number }[],
  number: number,
  onFailure: (message: string) => void,
) {
  return (element: HTMLCanvasElement) => {
    if (!loading) return;
    let stopped = false;
    let cancel = () => {};
    void loading
      .then((loaded) => {
        if (stopped) return;
        const viewport = module.pageViewport(loaded, scale);
        const size = sizes[number - 1];
        if (
          size &&
          (Math.abs(size.width - viewport.width / scale) > 0.5 ||
            Math.abs(size.height - viewport.height / scale) > 0.5)
        )
          sizes[number - 1] = {
            width: viewport.width / scale,
            height: viewport.height / scale,
          };
        const task = module.renderCanvas(loaded, element, viewport);
        cancel = task.cancel;
        return task.promise;
      })
      .catch((error: unknown) => {
        if (
          !stopped &&
          !(
            error instanceof Error &&
            error.name === "RenderingCancelledException"
          )
        )
          onFailure(`PDF 渲染失败：第 ${number} 页无法显示`);
      });
    return () => {
      stopped = true;
      cancel();
      element.width = 0;
      element.height = 0;
    };
  };
}

export function attachPdfTextLayer(
  module: {
    pageLinks: typeof pageLinks;
    renderText: typeof renderText;
    pageViewport: typeof pageViewport;
  },
  document: PDFDocumentProxy | undefined,
  loading: Promise<PDFPageProxy> | undefined,
  links: Map<number, PdfLink[]>,
  number: number,
) {
  return (element: HTMLElement) => {
    if (!document || !loading) return;
    let stopped = false;
    let cancel = () => {};
    void loading
      .then((loaded) => {
        if (stopped) return;
        if (!links.has(number))
          void module
            .pageLinks(document, loaded)
            .then((found) => links.set(number, found))
            .catch(() => links.set(number, []));
        const task = module.renderText(
          loaded,
          element,
          module.pageViewport(loaded, 1),
        );
        cancel = task.cancel;
        return task.promise;
      })
      .catch(() => {
        /* the canvas reports page failures; text is best effort */
      });
    return () => {
      stopped = true;
      cancel();
      element.replaceChildren();
    };
  };
}

export function attachPdfPinchZoom(
  element: HTMLElement,
  options: {
    isReady: () => boolean;
    getZoom: () => number;
    getPinch: () => { scale: number; x: number; y: number } | undefined;
    setPinch: (
      pinch: { scale: number; x: number; y: number } | undefined,
    ) => void;
    zoomTo: (next: number, x?: number, y?: number) => void;
    onResize: () => void;
  },
) {
  let start: { distance: number; x: number; y: number } | undefined;
  const measure = (touches: TouchList) => {
    const [a, b] = [touches[0], touches[1]];
    if (!a || !b) return;
    const rect = element.getBoundingClientRect();
    return {
      distance: Math.hypot(a.clientX - b.clientX, a.clientY - b.clientY) || 1,
      x: (a.clientX + b.clientX) / 2 - rect.left,
      y: (a.clientY + b.clientY) / 2 - rect.top,
    };
  };
  const touchstart = (event: TouchEvent) => {
    event.stopPropagation();
    if (!options.isReady() || event.touches.length !== 2) return;
    start = measure(event.touches);
    if (start) options.setPinch({ scale: 1, x: start.x, y: start.y });
  };
  const touchmove = (event: TouchEvent) => {
    event.stopPropagation();
    if (!start || event.touches.length !== 2) return;
    event.preventDefault();
    const now = measure(event.touches);
    const zoom = options.getZoom();
    if (now)
      options.setPinch({
        scale: clampPdfZoom((zoom * now.distance) / start.distance) / zoom,
        x: start.x,
        y: start.y,
      });
  };
  const touchend = (event: TouchEvent) => {
    event.stopPropagation();
    if (!start || event.touches.length >= 2) return;
    const done = options.getPinch();
    start = undefined;
    options.setPinch(undefined);
    if (done && done.scale !== 1)
      options.zoomTo(options.getZoom() * done.scale, done.x, done.y);
  };
  const wheel = (event: WheelEvent) => {
    // Browsers report trackpad pinches as ctrl+wheel.
    if (!event.ctrlKey) return;
    event.preventDefault();
    event.stopPropagation();
    const rect = element.getBoundingClientRect();
    options.zoomTo(
      options.getZoom() * pdfZoomStep ** (-event.deltaY / 100),
      event.clientX - rect.left,
      event.clientY - rect.top,
    );
  };
  const observer = new ResizeObserver(options.onResize);
  observer.observe(element);
  element.addEventListener("touchstart", touchstart, { passive: true });
  element.addEventListener("touchmove", touchmove, { passive: false });
  element.addEventListener("touchend", touchend);
  element.addEventListener("touchcancel", touchend);
  element.addEventListener("wheel", wheel, { passive: false });
  return () => {
    observer.disconnect();
    element.removeEventListener("touchstart", touchstart);
    element.removeEventListener("touchmove", touchmove);
    element.removeEventListener("touchend", touchend);
    element.removeEventListener("touchcancel", touchend);
    element.removeEventListener("wheel", wheel);
  };
}

export function observePdfPages(
  element: HTMLElement | undefined,
  phase: "loading" | "ready" | "failed",
  count: number,
  visible: Set<number>,
) {
  if (!element || phase !== "ready" || !count) return;
  const observer = new IntersectionObserver(
    (entries) => {
      for (const entry of entries) {
        const number = Number((entry.target as HTMLElement).dataset.pdfPage);
        if (entry.isIntersecting) visible.add(number);
        else visible.delete(number);
      }
    },
    { root: element },
  );
  for (const target of element.querySelectorAll("[data-pdf-page]"))
    observer.observe(target);
  return () => observer.disconnect();
}

export function findVisiblePdfPage(
  scroller: HTMLElement,
  visible: Set<number>,
): number {
  const line = scroller.scrollTop + pdfPadding + 1;
  let page = Math.min(...visible);
  for (const number of visible) {
    const box = scroller.querySelector<HTMLElement>(
      `[data-pdf-page="${number}"]`,
    );
    if (box && box.offsetTop <= line && number > page) page = number;
  }
  return page;
}

export function zoomPdfScroller(
  scroller: HTMLElement,
  zoom: number,
  next: number,
  x?: number,
  y?: number,
): number {
  const target = clampPdfZoom(next);
  const px = x ?? scroller.clientWidth / 2,
    py = y ?? scroller.clientHeight / 2;
  const ratio = target / zoom;
  const left = (scroller.scrollLeft + px) * ratio - px,
    top = (scroller.scrollTop + py) * ratio - py;
  requestAnimationFrame(() => {
    scroller.scrollLeft = left;
    scroller.scrollTop = top;
  });
  return target;
}
