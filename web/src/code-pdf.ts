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
