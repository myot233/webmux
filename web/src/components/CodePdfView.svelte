<script lang="ts">
  import { untrack } from "svelte";
  import { SvelteMap, SvelteSet } from "svelte/reactivity";
  import { apiBlob } from "../api";
  import type { CodeDocument } from "../schemas";
  import { basename, codeRawPath, formatBytes, rawError } from "../code-image";
  import type { PDFDocumentProxy, PdfLink } from "../code-pdf";
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
  }: {
    id: string;
    snapshotId: string;
    document: CodeDocument;
    /** Inactive views cancel the read and parse, and start none. */
    active?: boolean;
    ondownloaderror: (message: string, status: number) => void;
  } = $props();
  type PdfModule = typeof import("../code-pdf");
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
          import("../code-pdf"),
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
  // Zooms keeping the content point under (x, y) of the scroller fixed.
  function zoomTo(next: number, x?: number, y?: number) {
    if (phase !== "ready" || !scroller) return;
    const target = clamp(next);
    const px = x ?? scroller.clientWidth / 2,
      py = y ?? scroller.clientHeight / 2;
    const ratio = target / zoom;
    const left = (scroller.scrollLeft + px) * ratio - px,
      top = (scroller.scrollTop + py) * ratio - py;
    zoom = target;
    fitted = false;
    const element = scroller;
    requestAnimationFrame(() => {
      element.scrollLeft = left;
      element.scrollTop = top;
    });
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
  $effect(() => {
    const element = scroller;
    const count = sizes.length;
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
  });
  // The current page is the one crossing the top edge of the viewport; the
  // first intersecting page may be the previous page's edge after a jump.
  function track() {
    if (!scroller || !visible.size) return;
    if (jumped && Math.abs(scroller.scrollTop - jumped.top) < 1) {
      current = jumped.page;
      if (!editing) pageInput = String(jumped.page);
      return;
    }
    jumped = undefined;
    const line = scroller.scrollTop + padding + 1;
    let page = Math.min(...visible);
    for (const number of visible) {
      const box = scroller.querySelector<HTMLElement>(
        `[data-pdf-page="${number}"]`,
      );
      if (box && box.offsetTop <= line && number > page) page = number;
    }
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
    return (element: HTMLCanvasElement) => {
      const module = pdf,
        scale = zoom,
        loading = page(number);
      if (!module || !loading) return;
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
            renderFailure = `PDF 渲染失败：第 ${number} 页无法显示`;
        });
      return () => {
        stopped = true;
        cancel();
        element.width = 0;
        element.height = 0;
      };
    };
  }
  // Selectable text and link annotations; both follow zoom through CSS.
  function textLayer(number: number) {
    return (element: HTMLElement) => {
      const module = pdf,
        document = proxy,
        loading = page(number);
      if (!module || !document || !loading) return;
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

  // Native one-finger scrolling stays; two fingers zoom the pages, previewed
  // with a transform and committed (re-rendered) when the pinch ends.
  $effect(() => {
    const element = scroller;
    if (!element) return;
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
      if (phase !== "ready" || event.touches.length !== 2) return;
      start = measure(event.touches);
      if (start) pinch = { scale: 1, x: start.x, y: start.y };
    };
    const touchmove = (event: TouchEvent) => {
      event.stopPropagation();
      if (!start || event.touches.length !== 2) return;
      event.preventDefault();
      const now = measure(event.touches);
      if (now)
        pinch = {
          scale: clamp((zoom * now.distance) / start.distance) / zoom,
          x: start.x,
          y: start.y,
        };
    };
    const touchend = (event: TouchEvent) => {
      event.stopPropagation();
      if (!start || event.touches.length >= 2) return;
      const done = pinch;
      start = undefined;
      pinch = undefined;
      if (done && done.scale !== 1) zoomTo(zoom * done.scale, done.x, done.y);
    };
    const wheel = (event: WheelEvent) => {
      // Browsers report trackpad pinches as ctrl+wheel.
      if (!event.ctrlKey) return;
      event.preventDefault();
      event.stopPropagation();
      const rect = element.getBoundingClientRect();
      zoomTo(
        zoom * step ** (-event.deltaY / 100),
        event.clientX - rect.left,
        event.clientY - rect.top,
      );
    };
    const resize = () => {
      if (fitted && phase === "ready") fit();
    };
    const observer = new ResizeObserver(resize);
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
    onscroll={track}
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
