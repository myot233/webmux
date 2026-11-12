<script lang="ts">
  import { tick, untrack } from "svelte";
  import { SvelteMap, SvelteSet } from "svelte/reactivity";
  import { apiBlob } from "../../platform/api";
  import type { CodeDocument, CodePdfState } from "../../contracts/schemas";
  import {
    basename,
    codeRawPath,
    formatBytes,
    rawError,
  } from "../../domains/code/code-image";
  import type { PDFDocumentProxy, PdfLink } from "../../domains/code/code-pdf";
  import CodeDownloadButton from "./CodeDownloadButton.svelte";
  import { Button } from "$lib/components/ui/button";
  import { Input } from "$lib/components/ui/input";
  import IconArrowAutofitWidth from "~icons/tabler/arrow-autofit-width";
  import IconZoomIn from "~icons/tabler/zoom-in";
  import IconZoomOut from "~icons/tabler/zoom-out";
  let {
    id,
    snapshotId,
    document: doc,
    active = true,
    ondownloaderror,
    restore,
  }: {
    id: string;
    snapshotId: string;
    document: CodeDocument;
    /** Inactive views cancel the read and parse, and start none. */
    active?: boolean;
    ondownloaderror: (message: string, status: number) => void;
    /** A captured zoom and page position, applied once page sizes are known. */
    restore?: CodePdfState;
  } = $props();
  type PdfModule = typeof import("../../domains/code/code-pdf");
  const minZoom = 0.25,
    maxZoom = 5,
    step = 1.25,
    // Pages kept rendered beyond the visible ones, and released past this.
    renderAhead = 1,
    releaseBeyond = 3,
    // Matches the page list's padding (p-2).
    padding = 8;
  const clamp = (value: number) => Math.min(maxZoom, Math.max(minZoom, value));
  let pdf = $state.raw<PdfModule>();
  let proxy = $state.raw<PDFDocumentProxy>();
  let phase = $state<"loading" | "ready" | "failed">("loading");
  let message = $state("");
  let renderFailure = $state("");
  // Page boxes in CSS pixels at 100 %; unloaded pages assume the first size.
  let sizes = $state<{ width: number; height: number }[]>([]);
  let zoom = $state(1);
  // While fitted, panel resizes refit the width instead of keeping zoom.
  let fitted = $state(true);
  // Live two-finger pinch factor, committed to `zoom` when the pinch ends.
  let pinch = $state<{ scale: number; x: number; y: number }>();
  let current = $state(1);
  let pageInput = $state("1");
  let editing = $state(false);
  let scroller = $state<HTMLDivElement>();
  const visible = new SvelteSet<number>();
  const kept = new SvelteSet<number>();
  const links = new SvelteMap<number, PdfLink[]>();
  const path = $derived(codeRawPath(id, snapshotId));
  // Only an unfinished load is cancelled by leaving the tab; a loaded or
  // failed document stays as it is for the reader's return.
  const wanted = $derived(active || phase !== "loading");

  $effect(() => {
    const source = path;
    if (!wanted) return;
    const controller = new AbortController();
    const { signal } = controller;
    let opened: ReturnType<PdfModule["openPdf"]> | undefined;
    untrack(() => {
      phase = "loading";
      message = "";
      renderFailure = "";
      pdf = undefined;
      proxy = undefined;
      sizes = [];
      links.clear();
      visible.clear();
      kept.clear();
    });
    void (async () => {
      let module: PdfModule;
      let data: ArrayBuffer;
      try {
        const [loaded, blob] = await Promise.all([
          import("../../domains/code/code-pdf"),
          apiBlob(source, signal),
        ]);
        module = loaded;
        data = await blob.arrayBuffer();
      } catch (error) {
        if (!signal.aborted) fail(rawError(error, "预览"));
        return;
      }
      if (signal.aborted) return;
      try {
        opened = module.openPdf(data, signal);
      } catch {
        // pdf.js throws synchronously when the browser lacks a built-in it
        // needs; the bytes may be fine, so the reader is sent to download.
        fail("当前浏览器无法预览 PDF，请下载后查看");
        return;
      }
      try {
        const document = await opened.promise;
        const first = await document.getPage(1);
        if (signal.aborted) return;
        const { width, height } = first.getViewport({
          scale: module.cssUnits,
        });
        sizes = Array.from({ length: document.numPages }, () => ({
          width,
          height,
        }));
        pdf = module;
        proxy = document;
        phase = "ready";
        fit();
        void applyRestore();
      } catch (error) {
        if (!signal.aborted) fail(module.pdfLoadError(error));
      }
    })();
    return () => {
      controller.abort();
      void opened?.destroy();
    };
  });

  function fail(text: string) {
    phase = "failed";
    message = text;
  }
  // pdf.js caches page proxies per document.
  function page(number: number) {
    return proxy?.getPage(number);
  }
  function fit() {
    fitted = true;
    const first = sizes[0];
    if (!scroller || !first) return;
    zoom = clamp((scroller.clientWidth - padding * 2) / first.width);
  }
  // Until applied, `capture()` returns it unchanged.
  let pending = untrack(() => restore);
  let applying = false;
  // The position last seen with layout; a hidden tab reports this one.
  let known: CodePdfState = { fitted: true, zoom: 1, page: 1, offset: 0 };
  function reading(): CodePdfState {
    if (!pdf) return known;
    return pdf.measurePdfReading(scroller, phase, fitted, zoom, known);
  }
  export function capture(): CodePdfState {
    return pending ?? reading();
  }
  $effect(() => {
    void zoom;
    void fitted;
    if (!pending) known = reading();
  });
  async function applyRestore() {
    const state = pending,
      element = scroller;
    if (!state || applying || phase !== "ready" || !sizes.length) return;
    if (!element || element.clientWidth === 0) return;
    applying = true;
    if (state.fitted) fit();
    else {
      fitted = false;
      zoom = clamp(state.zoom);
    }
    await tick();
    applying = false;
    if (pending !== state || phase !== "ready") return;
    pending = undefined;
    const target = element.querySelector<HTMLElement>(
      `[data-pdf-page="${Math.min(state.page, sizes.length)}"]`,
    );
    if (target)
      element.scrollTop =
        target.offsetTop - padding + state.offset * target.offsetHeight;
    known = reading();
  }
  // Zooms keeping the content point under (x, y) of the scroller fixed.
  function zoomTo(next: number, x?: number, y?: number) {
    if (phase !== "ready" || !scroller || !pdf) return;
    zoom = pdf.zoomPdfScroller(scroller, zoom, next, x, y);
    fitted = false;
  }
  // Near the end the scroll position is clamped, so a jump target that cannot
  // reach the top stays current until the reader scrolls away from it.
  let jumped: { page: number; top: number } | undefined;
  function jump(number: number) {
    if (!scroller || !Number.isInteger(number)) return;
    const target = Math.min(sizes.length, Math.max(1, number));
    const element = scroller.querySelector<HTMLElement>(
      `[data-pdf-page="${target}"]`,
    );
    if (!element) return;
    scroller.scrollTop = element.offsetTop - padding;
    jumped = { page: target, top: scroller.scrollTop };
    current = target;
    pageInput = String(target);
  }
  function submitPage() {
    editing = false;
    const number = Number(pageInput);
    if (Number.isInteger(number) && number >= 1) jump(number);
    else pageInput = String(current);
  }

  // Visible pages decide which pages hold a canvas.
  $effect(() => pdf?.observePdfPages(scroller, phase, sizes.length, visible));
  // The current page is the one crossing the top edge of the viewport; the
  // first intersecting page may be the previous page's edge after a jump.
  function track() {
    if (!scroller || !visible.size || !pdf) return;
    if (jumped && Math.abs(scroller.scrollTop - jumped.top) < 1) {
      current = jumped.page;
      if (!editing) pageInput = String(jumped.page);
      return;
    }
    jumped = undefined;
    const page = pdf.findVisiblePdfPage(scroller, visible);
    current = page;
    if (!editing) pageInput = String(page);
  }
  $effect(() => {
    if (!visible.size) return;
    const first = Math.min(...visible),
      last = Math.max(...visible);
    untrack(() => {
      for (
        let number = Math.max(1, first - renderAhead);
        number <= Math.min(sizes.length, last + renderAhead);
        number++
      )
        kept.add(number);
      for (const number of kept)
        if (number < first - releaseBeyond || number > last + releaseBeyond)
          kept.delete(number);
      track();
    });
  });

  // Draws the page at the current zoom; detaching cancels and frees pixels.
  function canvas(number: number) {
    if (!pdf) return () => {};
    return pdf.attachPdfCanvas(
      pdf,
      zoom,
      page(number),
      sizes,
      number,
      (msg) => (renderFailure = msg),
    );
  }
  // Selectable text and link annotations; both follow zoom through CSS.
  function textLayer(number: number) {
    if (!pdf) return () => {};
    return pdf.attachPdfTextLayer(pdf, proxy, page(number), links, number);
  }

  // Native one-finger scrolling stays; two fingers zoom the pages, previewed
  // with a transform and committed (re-rendered) when the pinch ends.
  $effect(() => {
    const element = scroller;
    const module = pdf;
    if (!element || !module) return;
    return module.attachPdfPinchZoom(element, {
      isReady: () => phase === "ready",
      getZoom: () => zoom,
      getPinch: () => pinch,
      setPinch: (next) => (pinch = next),
      zoomTo,
      onResize: () => {
        if (pending) void applyRestore();
        else if (fitted && phase === "ready") fit();
      },
    });
  });
</script>

<div class="flex min-h-0 flex-1 flex-col">
  <!-- One row on every width; narrow touch screens zoom by pinching. -->
  <div class="flex shrink-0 items-center gap-1 border-b px-2 py-1 text-xs">
    <Input
      size="sm"
      class="w-12"
      inputmode="numeric"
      aria-label="页码"
      title="页码，输入后按回车跳转"
      disabled={phase !== "ready"}
      bind:value={pageInput}
      onfocus={() => (editing = true)}
      oninput={() => (editing = true)}
      onblur={submitPage}
      onkeydown={(event) => {
        if (event.key === "Enter") submitPage();
      }}
    />
    <span class="shrink-0 text-muted-foreground tabular-nums"
      >/ {sizes.length || "–"}</span
    >
    <p class="min-w-0 flex-1 truncate text-muted-foreground max-sm:hidden">
      PDF · {formatBytes(doc.size)}
    </p>
    <span class="flex-1 sm:hidden"></span>
    <output class="min-w-10 text-right tabular-nums" aria-label="缩放比例"
      >{phase === "ready" ? `${Math.round(zoom * 100)}%` : ""}</output
    >
    <Button
      variant="outline"
      size="icon-sm"
      aria-label="适应宽度"
      title="适应宽度"
      aria-pressed={phase === "ready" && fitted}
      disabled={phase !== "ready"}
      onclick={fit}><IconArrowAutofitWidth aria-hidden="true" /></Button
    >
    <Button
      variant="outline"
      size="icon-sm"
      class="max-sm:any-pointer-coarse:hidden"
      aria-label="缩小"
      title="缩小"
      disabled={phase !== "ready" || zoom <= minZoom}
      onclick={() => zoomTo(zoom / step)}
      ><IconZoomOut aria-hidden="true" /></Button
    >
    <Button
      variant="outline"
      size="icon-sm"
      class="max-sm:any-pointer-coarse:hidden"
      aria-label="放大"
      title="放大"
      disabled={phase !== "ready" || zoom >= maxZoom}
      onclick={() => zoomTo(zoom * step)}
      ><IconZoomIn aria-hidden="true" /></Button
    >
    <CodeDownloadButton
      {path}
      filename={basename(doc.path)}
      disabled={!doc.downloadable}
      onerror={ondownloaderror}
    />
  </div>
  {#if renderFailure}<p
      class="shrink-0 border-b px-2 py-1 text-sm text-destructive"
      role="alert"
    >
      {renderFailure}
    </p>{/if}
  <div
    bind:this={scroller}
    class="code-pdf-scroller relative min-h-0 flex-1 overflow-auto bg-muted/40"
    role="group"
    aria-label="PDF 预览"
    onscroll={() => {
      track();
      if (!pending) known = reading();
    }}
  >
    {#if phase === "loading"}<p
        class="absolute inset-0 grid place-items-center text-sm text-muted-foreground"
        role="status"
      >
        正在加载 PDF…
      </p>{:else if phase === "failed"}<p
        class="absolute inset-0 grid place-items-center p-4 text-center text-sm text-destructive"
        role="alert"
      >
        {message}
      </p>{:else if pdf}
      <div
        class="flex w-max min-w-full origin-(--pinch-origin) transform-(--pinch-transform) flex-col items-center gap-2 p-2"
        style:--pinch-origin={pinch ? `${pinch.x}px ${pinch.y}px` : "0 0"}
        style:--pinch-transform={pinch ? `scale(${pinch.scale})` : "none"}
      >
        {#each sizes as size, index (index)}
          {@const number = index + 1}
          <div
            data-pdf-page={number}
            class="code-pdf-page relative h-(--page-height) w-(--page-width) shrink-0 shadow-sm"
            style:--page-width={`${size.width * zoom}px`}
            style:--page-height={`${size.height * zoom}px`}
            style:--total-scale-factor={zoom * pdf.cssUnits}
            role="region"
            aria-label={`第 ${number} 页`}
          >
            {#if kept.has(number)}
              <canvas
                class="absolute inset-0 size-full"
                aria-hidden="true"
                {@attach canvas(number)}
              ></canvas>
              <div class="textLayer" {@attach textLayer(number)}></div>
            {/if}
            {#each links.get(number) ?? [] as link, linkIndex (linkIndex)}
              {@const label =
                "url" in link
                  ? `打开链接 ${link.url}`
                  : `跳到第 ${link.page} 页`}
              <a
                class="code-pdf-link absolute top-(--link-top) left-(--link-left) h-(--link-height) w-(--link-width)"
                style:--link-left={`${link.left}%`}
                style:--link-top={`${link.top}%`}
                style:--link-width={`${link.width}%`}
                style:--link-height={`${link.height}%`}
                href={"url" in link ? link.url : `#pdf-page-${link.page}`}
                target={"url" in link ? "_blank" : undefined}
                rel={"url" in link ? "noopener noreferrer" : undefined}
                title={label}
                aria-label={label}
                onclick={(event) => {
                  if ("url" in link) return;
                  event.preventDefault();
                  jump(link.page);
                }}
              ></a>
            {/each}
          </div>
        {/each}
      </div>
    {/if}
  </div>
</div>

<style>
  .code-pdf-scroller {
    overscroll-behavior: contain;
  }
  /* Paper is white like the PDF's own page, whatever the app theme. */
  .code-pdf-page {
    background: white;
    --scale-round-x: 1px;
    --scale-round-y: 1px;
  }
  .code-pdf-link {
    z-index: 2;
  }
  .code-pdf-link:focus-visible {
    outline: 2px solid var(--ring);
  }
  /* The subset of pdf.js's text layer styles needed for selection. */
  .code-pdf-page :global(.textLayer) {
    position: absolute;
    inset: 0;
    overflow: clip;
    line-height: 1;
    text-align: initial;
    text-size-adjust: none;
    forced-color-adjust: none;
    transform-origin: 0 0;
    z-index: 1;
    --min-font-size: 1;
    --text-scale-factor: calc(var(--total-scale-factor) * var(--min-font-size));
    --min-font-size-inv: calc(1 / var(--min-font-size));
  }
  .code-pdf-page :global(.textLayer :is(span, br)) {
    color: transparent;
    position: absolute;
    white-space: pre;
    cursor: text;
    transform-origin: 0% 0%;
    user-select: text;
  }
  .code-pdf-page :global(.textLayer > :not(.markedContent)),
  .code-pdf-page :global(.textLayer .markedContent span:not(.markedContent)) {
    --font-height: 0;
    --scale-x: 1;
    --rotate: 0deg;
    font-size: calc(var(--text-scale-factor) * var(--font-height));
    transform: rotate(var(--rotate)) scaleX(var(--scale-x))
      scale(var(--min-font-size-inv));
  }
  .code-pdf-page :global(.textLayer .markedContent) {
    display: contents;
  }
  .code-pdf-page :global(.textLayer ::selection) {
    background: color-mix(in srgb, AccentColor, transparent 50%);
    color: transparent;
  }
  .code-pdf-page :global(.textLayer .endOfContent) {
    display: block;
    position: absolute;
    inset: 100% 0 0;
    user-select: none;
  }
</style>
