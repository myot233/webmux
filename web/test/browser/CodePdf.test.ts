import { tick } from "svelte";
import { afterEach, expect, test, vi } from "vitest";
import { page, userEvent } from "vitest/browser";
import { cleanup, render } from "vitest-browser-svelte/pure";
import Harness from "./CodeImageHarness.svelte";
import {
  CodeFileSchema,
  type CodeDocument,
  type CodeFile,
} from "../../src/schemas";
import { brokenPdf, pdfFixture } from "../fixtures/pdf";

const now = "2026-10-24T00:00:00Z";
const realFetch = globalThis.fetch;

afterEach(async () => {
  await cleanup();
  await userEvent.cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  localStorage.removeItem("webmux.token");
});

function pdfDocument(overrides: Partial<CodeDocument> = {}): CodeDocument {
  return {
    nodeId: "ZG9jcy9kZXNpZ24ucGRm",
    path: "docs/design.pdf",
    absolutePath: "/repo/docs/design.pdf",
    source: "worktree",
    oid: "",
    contentVersion: "v1",
    kind: "pdf",
    text: "",
    lineCount: 0,
    referenceable: true,
    reason: "",
    mediaType: "application/pdf",
    size: 1234,
    downloadable: true,
    ...overrides,
  };
}
const fileOf = (document: CodeDocument, snapshotId = "snap-1"): CodeFile =>
  CodeFileSchema.parse({ snapshotId, checkedAt: now, document });

// Answers the raw snapshot endpoint; bundled pdf.js assets go to the network.
function serve(
  raw: (init: RequestInit | undefined) => Response | Promise<Response>,
) {
  const requests: { url: URL; init?: RequestInit }[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>(async (input, init) => {
      const url = new URL(
        input instanceof Request ? input.url : String(input),
        location.href,
      );
      requests.push({ url, init });
      if (!url.pathname.startsWith("/api/")) return realFetch(input, init);
      return raw(init);
    }),
  );
  return requests;
}
const pdfResponse = (bytes: Uint8Array) =>
  new Response(new Blob([bytes as BlobPart], { type: "application/pdf" }), {
    headers: { "Content-Type": "application/pdf" },
  });
const scroller = () =>
  document.querySelector<HTMLElement>('[aria-label="PDF 预览"]')!;
const pageBox = (number: number) =>
  document.querySelector<HTMLElement>(`[data-pdf-page="${number}"]`)!;
const canvases = () =>
  [...document.querySelectorAll<HTMLElement>("[data-pdf-page]")]
    .filter((box) => box.querySelector("canvas"))
    .map((box) => Number(box.dataset.pdfPage));
const percent = () =>
  document.querySelector('[aria-label="缩放比例"]')?.textContent?.trim();
const pageText = (number: number) =>
  pageBox(number).querySelector(".textLayer")?.textContent ?? "";
// A page counts as painted once pdf.js wrote non-white pixels.
function painted(number: number) {
  const canvas = pageBox(number).querySelector("canvas");
  if (!canvas || !canvas.width) return false;
  const pixels = canvas
    .getContext("2d")!
    .getImageData(0, 0, canvas.width, canvas.height).data;
  for (let i = 0; i < pixels.length; i += 4) if (pixels[i]! < 128) return true;
  return false;
}

test("a multi-page Chinese PDF renders near the viewport, releases far pages and jumps", async () => {
  localStorage.setItem("webmux.token", "secret");
  const requests = serve(() => pdfResponse(pdfFixture({ pages: 30 })));
  await render(Harness, { file: fileOf(pdfDocument()) });
  const raw = requests.find((r) => r.url.pathname.startsWith("/api/"))!;
  expect(raw.url.pathname).toBe(
    "/api/sessions/session/code/snapshots/snap-1/raw",
  );
  expect(new Headers(raw.init?.headers).get("Authorization")).toBe(
    "Bearer secret",
  );
  // The first test in a run pays for the cold renderer and worker import.
  await expect
    .poll(() => document.querySelectorAll("[data-pdf-page]").length, {
      timeout: 10_000,
    })
    .toBe(30);
  await expect.poll(() => painted(1), { timeout: 10_000 }).toBe(true);
  await expect
    .poll(() => pageText(1), { timeout: 10_000 })
    .toContain("中文测试");
  // The CJK CMap came from the bundled assets, not a CDN.
  const cmap = requests.find((r) => r.url.pathname.endsWith(".bcmap"));
  expect(cmap?.url.origin).toBe(location.origin);
  expect(Math.max(...canvases())).toBeLessThan(10);
  // Fit width: the page fills the panel minus its padding.
  expect(pageBox(1).getBoundingClientRect().width).toBeCloseTo(
    scroller().clientWidth - 16,
    0,
  );

  scroller().scrollTop = pageBox(20).offsetTop;
  await expect.poll(() => canvases().includes(20)).toBe(true);
  await expect.poll(() => canvases().includes(1)).toBe(false);
  expect(canvases().every((number) => number >= 16 && number <= 24)).toBe(true);
  await expect.element(page.getByLabelText("页码")).toHaveValue("20");

  await page.getByLabelText("页码").fill("5");
  await userEvent.keyboard("{Enter}");
  await expect
    .poll(() => Math.abs(scroller().scrollTop - (pageBox(5).offsetTop - 8)))
    .toBeLessThan(2);
  await expect.poll(() => painted(5), { timeout: 10_000 }).toBe(true);
  await expect.poll(() => canvases().includes(20)).toBe(false);
  // The text layer is built after the canvas paints.
  await expect
    .poll(() => pageText(5), { timeout: 10_000 })
    .toContain("第 5 页");

  // The last page cannot scroll to the top edge when the panel is taller than
  // a fitted page (Pixel 7); it still counts as current after its own scroll
  // event, and scrolling away hands the number back to the page at the top.
  scroller().style.flex = "none";
  scroller().style.height = `${pageBox(30).offsetHeight * 2}px`;
  // The page box keeps focus after the previous Enter; a scroll while the
  // reader types must not overwrite the typed number.
  await page.getByLabelText("页码").fill("30");
  scroller().dispatchEvent(new Event("scroll"));
  await tick();
  await userEvent.keyboard("{Enter}");
  await expect.poll(() => canvases().includes(30)).toBe(true);
  expect(scroller().scrollTop).toBeLessThan(pageBox(30).offsetTop - 8);
  scroller().dispatchEvent(new Event("scroll"));
  await tick();
  expect(
    page.getByLabelText("页码").element() as HTMLInputElement,
  ).toHaveProperty("value", "30");
  scroller().scrollTop = pageBox(20).offsetTop;
  await expect.element(page.getByLabelText("页码")).toHaveValue("20");
});

test("zoom controls, ctrl+wheel and two-finger pinch rescale the pages", async () => {
  serve(() => pdfResponse(pdfFixture({ pages: 2 })));
  await render(Harness, { file: fileOf(pdfDocument()) });
  await expect.poll(() => painted(1), { timeout: 10_000 }).toBe(true);
  const fit = percent();
  const width = () => pageBox(1).getBoundingClientRect().width;
  const fitted = width();
  await page.getByRole("button", { name: "放大" }).click();
  await expect.poll(width).toBeCloseTo(fitted * 1.25, 0);
  expect(percent()).not.toBe(fit);
  await page.getByRole("button", { name: "缩小" }).click();
  await page.getByRole("button", { name: "缩小" }).click();
  await expect.poll(width).toBeCloseTo(fitted / 1.25, 0);
  await page.getByRole("button", { name: "适应宽度" }).click();
  await expect.poll(percent).toBe(fit);

  const event = new WheelEvent("wheel", {
    deltaY: -100,
    ctrlKey: true,
    bubbles: true,
    cancelable: true,
  });
  scroller().dispatchEvent(event);
  expect(event.defaultPrevented).toBe(true);
  await expect.poll(width).toBeCloseTo(fitted * 1.25, 0);
  await page.getByRole("button", { name: "适应宽度" }).click();

  const rect = scroller().getBoundingClientRect();
  const touches = (spread: number) =>
    [-1, 1].map(
      (side, identifier) =>
        new Touch({
          identifier,
          target: scroller(),
          clientX: rect.left + rect.width / 2 + side * spread,
          clientY: rect.top + 100,
        }),
    );
  const touch = (type: string, list: Touch[]) => {
    const touchEvent = new TouchEvent(type, {
      touches: list,
      changedTouches: list,
      bubbles: true,
      cancelable: true,
    });
    scroller().dispatchEvent(touchEvent);
    return touchEvent;
  };
  touch("touchstart", touches(50));
  expect(touch("touchmove", touches(100)).defaultPrevented).toBe(true);
  touch("touchend", []);
  await expect.poll(width).toBeCloseTo(fitted * 2, 0);
  await expect.poll(() => painted(1), { timeout: 10_000 }).toBe(true);
});

test("internal links jump, web links open a new tab and scripts never run", async () => {
  serve(() =>
    pdfResponse(
      pdfFixture({
        pages: 3,
        internalTarget: 3,
        externalUrl: "https://example.com/",
        script: true,
      }),
    ),
  );
  await render(Harness, { file: fileOf(pdfDocument()) });
  const external = page.getByRole("link", {
    name: "打开链接 https://example.com/",
  });
  await expect.element(external).toBeInTheDocument();
  expect(external.element().getAttribute("target")).toBe("_blank");
  expect(external.element().getAttribute("rel")).toBe("noopener noreferrer");
  // Only the two supported link kinds become links; the script action does not.
  expect(pageBox(1).querySelectorAll("a")).toHaveLength(2);
  await page.getByRole("link", { name: "跳到第 3 页" }).click();
  await expect.element(page.getByLabelText("页码")).toHaveValue("3");
  expect(scroller().scrollTop).toBeCloseTo(pageBox(3).offsetTop - 8, 0);
  expect("pdfScript" in window).toBe(false);
});

test("encrypted and damaged PDFs explain the failure and keep the download", async () => {
  serve(() => pdfResponse(pdfFixture({ encrypted: true })));
  const rendered = await render(Harness, { file: fileOf(pdfDocument()) });
  await expect
    .element(page.getByRole("alert"))
    .toHaveTextContent("PDF 受密码保护，请下载后查看");
  await expect
    .element(page.getByRole("button", { name: "下载" }))
    .toBeEnabled();
  await rendered.unmount();

  serve(() => pdfResponse(brokenPdf()));
  await render(Harness, {
    file: fileOf(pdfDocument({ path: "broken.pdf" }), "snap-2"),
  });
  await expect
    .element(page.getByRole("alert"))
    .toHaveTextContent("PDF 无法解析");
  await expect
    .element(page.getByRole("button", { name: "下载" }))
    .toBeEnabled();
});

test("over-limit PDFs show the reason and download without loading the renderer", async () => {
  const requests = serve(() => pdfResponse(pdfFixture()));
  await render(Harness, {
    file: fileOf(
      pdfDocument({
        kind: "too-large",
        reason: "超过 20 MiB 预览上限",
        size: 40 * 1024 * 1024,
      }),
    ),
  });
  await expect.element(page.getByText("超过 20 MiB 预览上限")).toBeVisible();
  await expect
    .element(page.getByLabelText("PDF 信息"))
    .toHaveTextContent("PDF · 40 MiB");
  await expect
    .element(page.getByRole("button", { name: "下载" }))
    .toBeEnabled();
  expect(document.querySelector('[aria-label="PDF 预览"]')).toBeNull();
  expect(requests).toHaveLength(0);
});

test("unmounting cancels the PDF read", async () => {
  let signal: AbortSignal | undefined;
  serve((init) => {
    const { promise, reject } = Promise.withResolvers<Response>();
    signal = init?.signal ?? undefined;
    signal?.addEventListener("abort", () =>
      reject(new DOMException("aborted", "AbortError")),
    );
    return promise;
  });
  const rendered = await render(Harness, { file: fileOf(pdfDocument()) });
  await expect.element(page.getByText("正在加载 PDF…")).toBeVisible();
  await expect.poll(() => signal).toBeDefined();
  expect(signal?.aborted).toBe(false);
  await rendered.unmount();
  expect(signal?.aborted).toBe(true);
});

test("leaving the tab cancels an unfinished load and keeps a loaded PDF", async () => {
  const signals: AbortSignal[] = [];
  let hold = true;
  serve((init) => {
    if (init?.signal) signals.push(init.signal);
    if (!hold) return pdfResponse(pdfFixture({ pages: 2 }));
    const { promise, reject } = Promise.withResolvers<Response>();
    init?.signal?.addEventListener("abort", () =>
      reject(new DOMException("aborted", "AbortError")),
    );
    return promise;
  });
  const view = await render(Harness, { file: fileOf(pdfDocument()) });
  await expect.poll(() => signals.length).toBe(1);
  await view.rerender({ active: false });
  expect(signals[0]?.aborted).toBe(true);
  hold = false;
  await view.rerender({ active: true });
  await expect.poll(() => painted(1), { timeout: 10_000 }).toBe(true);
  expect(signals).toHaveLength(2);
  await view.rerender({ active: false });
  await view.rerender({ active: true });
  expect(painted(1)).toBe(true);
  expect(signals).toHaveLength(2);
  expect(signals[1]?.aborted).toBe(false);
});
