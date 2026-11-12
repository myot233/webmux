<script lang="ts">
  import { onMount, untrack } from "svelte";
  import { CodeView, type CodeViewOptions } from "@pierre/diffs";
  import { captureCursor, restoreCursor } from "../../domains/code/code-search";
  import { createCodeFind } from "../../domains/code/code-find.svelte";
  import {
    settings,
    changeSettings,
  } from "../../domains/settings/settings.svelte";
  import { defaultCodeReader } from "../../domains/settings/settings-intent";
  import {
    codeTokenizeLimit,
    fileUnsafeCSS,
    pierreTheme,
  } from "../../domains/code/code-pierre";
  import { acquireCodeHighlighter } from "../../domains/code/code-highlighter";
  import { codeFont } from "../../hooks/code/code-font-preference.svelte";
  import {
    bindHorizontalScrollbar,
    readerKeydown,
    releaseCodeView,
    syncViewerOptions,
    watchReader,
    type CodeHorizontalTrack,
  } from "../../domains/code/code-reader-controls";
  import {
    measureFilePosition,
    parseFileGoto,
    resolvePierreFile,
    scrollFileToColumn,
  } from "../../domains/code/code-reader-dom";
  import type { CodeReaderState } from "../../contracts/schemas";
  import { Button } from "$lib/components/ui/button";
  import CodeHorizontalScrollbar from "./CodeHorizontalScrollbar.svelte";
  import CodeReaderPanels from "./CodeReaderPanels.svelte";
  import IconListNumbers from "~icons/tabler/list-numbers";
  import IconNumber from "~icons/tabler/number";
  import IconSearch from "~icons/tabler/search";
  import IconZoomIn from "~icons/tabler/zoom-in";

  let {
    text,
    path,
    onselect,
    active = true,
    restoreLine = 1,
    restore,
    draftEnabled = false,
    hint = $bindable(""),
  }: {
    text: string;
    path: string;
    onselect: (start: number, end: number) => void;
    // A hidden tab cannot measure itself; the position is recorded as it hides.
    active?: boolean;
    restoreLine?: number;
    // A captured reading state, applied once the reader has layout; it takes
    // precedence over `restoreLine`.
    restore?: CodeReaderState;
    draftEnabled?: boolean;
    // Selection guidance, rendered by the document's action bar.
    hint?: string;
  } = $props();

  const id = "file";
  // The document is fixed for this instance; the view remounts on a new
  // version instead of replacing the text underneath a reader. Pierre treats
  // every carriage return as a line break, so normalise them first.
  const contents = untrack(() => text.replace(/\r\n?/g, "\n"));
  const lines = contents.split("\n");
  // Each line keeps its terminator so the shared search reports line numbers.
  const searchLines = lines.map((line, index) =>
    index < lines.length - 1 ? `${line}\n` : line,
  );
  const file = untrack(() => resolvePierreFile(path, contents));
  // Until applied, `capture()` returns it unchanged.
  let pending = untrack(() => restore);
  // The top line last measured while the reader had layout; a hidden tab
  // reports this one.
  let known: Pick<CodeReaderState, "line" | "offset"> | undefined;
  let host: HTMLDivElement;
  let root: HTMLDivElement;
  let trackElement = $state<HTMLDivElement>();
  let track = $state<CodeHorizontalTrack>({ shown: false, width: 0 });
  let viewer: CodeView | undefined;
  const horizontalScrollbar = bindHorizontalScrollbar(
    () => viewer,
    () => trackElement,
    (next) => {
      if (track.shown !== next.shown || track.width !== next.width)
        track = next;
    },
  );
  let mounted = $state(false);
  let placed = false;
  // A jump made before the first render; the initial placement honors it
  // instead of scrolling back to the restore line.
  let requested: number | undefined;
  let requestedColumn: number | undefined;
  let coarse = $state(false);
  let dark = $state(false);
  let selected = $state<{ start: number; end: number } | null>(null);
  // A desktop selection extends from the last picked line; touch picks only
  // one line at a time.
  let anchor: number | undefined;
  const showGutter = $derived(
    (settings.values.codeReader ?? defaultCodeReader).lineNumbers === "show",
  );
  const find = createCodeFind(
    () => searchLines,
    () => host,
    () => root,
    (line) => {
      scrollToLine(line, "center");
      choose(line, line);
    },
  );

  function lineNumber(value: number) {
    return Math.max(1, Math.min(lines.length, value));
  }

  function paint(start: number, end: number) {
    const range = { start: Math.min(start, end), end: Math.max(start, end) };
    selected = range;
    viewer?.setSelectedLines({ id, range }, { notify: false });
  }

  function choose(start: number, end: number) {
    paint(start, end);
    onselect(selected!.start, selected!.end);
  }

  export function clearLineAction() {
    anchor = undefined;
    selected = null;
    viewer?.clearSelectedLines({ notify: false });
  }

  function select(number: number, shift: boolean) {
    const line = lineNumber(number);
    if (coarse) {
      anchor = undefined;
      choose(line, line);
      return;
    }
    if (shift && anchor !== undefined) {
      choose(anchor, line);
      anchor = undefined;
      return;
    }
    anchor = line;
    choose(line, line);
  }

  function position(): Pick<CodeReaderState, "line" | "offset"> {
    const measured = measureFilePosition(
      viewer,
      host,
      id,
      lines.length,
      () =>
        known ?? {
          line: lineNumber(untrack(() => requested ?? restoreLine)),
          offset: 0,
        },
    );
    if (viewer && host.clientHeight > 0 && viewer.getRenderedItems()[0])
      known = measured;
    return measured;
  }

  export function line() {
    return position().line;
  }

  export function capture(): CodeReaderState {
    if (pending) return pending;
    return {
      ...position(),
      selection: selected ? { start: selected.start, end: selected.end } : null,
      search: $state.snapshot(find.search),
      ...(find.panel ? { panel: find.panel } : {}),
      ...captureCursor(find.cursor, find.matches),
    };
  }

  function scrollToLine(line: number, align: "start" | "center") {
    viewer?.scrollTo({
      type: "line",
      id,
      lineNumber: lineNumber(line),
      align,
    });
  }

  // Applies `pending` once the reader has layout to scroll in.
  function applyRestore() {
    const state = pending;
    if (!state || !viewer || host.clientHeight === 0) return;
    if (!viewer.getRenderedItems()[0]) return;
    pending = undefined;
    placed = true;
    known = {
      line: lineNumber(state.line),
      offset: Math.min(1, Math.max(0, state.offset)),
    };
    find.search = state.search;
    find.panel = state.panel;
    if (state.selection) {
      const start = lineNumber(state.selection.start);
      const end = lineNumber(state.selection.end);
      paint(start, end);
      onselect(Math.min(start, end), Math.max(start, end));
    }
    find.cursor = restoreCursor(state.cursor, find.matches);
    if (requested !== undefined) {
      scrollToLine(requested, "center");
      if (requestedColumn !== undefined)
        scrollToColumn(requested, requestedColumn);
    } else
      viewer.scrollTo({
        type: "line",
        id,
        lineNumber: known.line,
        align: "start",
        // A positive offset leaves the line below the edge; the captured
        // fraction is how far the line is scrolled past it.
        offset: -known.offset * codeFont.lineHeight,
      });
  }

  function placeInitial() {
    if (placed || pending || !viewer || host.clientHeight === 0) return;
    if (!viewer.getRenderedItems()[0]) return;
    placed = true;
    if (selected)
      viewer.setSelectedLines({ id, range: selected }, { notify: false });
    if (requested !== undefined) {
      scrollToLine(requested, "center");
      if (requestedColumn !== undefined)
        scrollToColumn(requested, requestedColumn);
    } else
      scrollToLine(
        untrack(() => restoreLine),
        "start",
      );
  }

  /** Scrolls to `startLine` and selects `startLine..endLine` in the source reader. */
  export function revealLine(
    startLine: number,
    endLine = startLine,
    column?: number,
  ) {
    const a = lineNumber(Math.min(startLine, endLine));
    const z = lineNumber(Math.max(startLine, endLine));
    anchor = a;
    if (!placed) {
      requested = a;
      requestedColumn = column;
      if (pending) {
        const { panel: _, ...rest } = pending;
        pending = {
          ...rest,
          line: a,
          offset: 0,
          selection: { start: a, end: z },
        };
      }
    }
    choose(a, z);
    scrollToLine(a, "center");
    if (column !== undefined) scrollToColumn(a, column);
  }

  function scrollToColumn(line: number, column: number) {
    scrollFileToColumn(() => viewer, lineNumber(line), column);
  }

  function goto(value: string) {
    const current =
      (placed ? undefined : pending?.selection?.start) ?? selected?.start ?? 1;
    const parsed = parseFileGoto(value, current, lines.length);
    if (parsed) {
      const { target, column } = parsed;
      // Before the first placement this jump replaces the restored or initial
      // scroll position, which would otherwise win once layout arrives; the
      // rest of the restored state still applies.
      if (!placed) {
        requested = target;
        if (pending) {
          const { panel: _, ...rest } = pending;
          pending = { ...rest, line: target, offset: 0 };
        }
      }
      scrollToLine(target, "center");
      if (column !== undefined) scrollToColumn(target, column);
    }
    find.panel = undefined;
    host.focus();
  }

  function options(): CodeViewOptions<undefined, undefined> {
    return {
      ...pierreTheme(dark),
      overflow: "scroll",
      disableFileHeader: true,
      disableLineNumbers: !showGutter,
      lineHoverHighlight: "number",
      itemMetrics: codeFont.itemMetrics,
      tokenizeMaxLineLength: codeTokenizeLimit,
      unsafeCSS: fileUnsafeCSS,
      layout: { paddingTop: 0, paddingBottom: 0, gap: 0 },
      onLineNumberClick(props) {
        if (props.type !== "line") return;
        select(props.lineNumber, props.event.shiftKey);
      },
      onLineClick(props) {
        if (!coarse || props.type !== "line") return;
        const nativeSelection = document.getSelection();
        if (nativeSelection && !nativeSelection.isCollapsed) return;
        select(props.lineNumber, false);
      },
      // CodeView publishes getRenderedItems() after the item render callback.
      // Defer until then so a pending restore can scroll in.
      onPostRender: () =>
        queueMicrotask(() => {
          horizontalScrollbar.sync();
          if (pending) applyRestore();
          else placeInitial();
        }),
    };
  }

  function toggleGutter() {
    changeSettings({
      codeReader: {
        ...(settings.values.codeReader ?? defaultCodeReader),
        lineNumbers: showGutter ? "hide" : "show",
      },
    });
  }

  onMount(() => {
    const stopEnvironment = watchReader(host, {
      onPointer: (value) => {
        coarse = value;
      },
      onTheme: (value) => {
        dark = value;
      },
      // A hidden tab has no layout; a pending restore waits until it is shown.
      onResize: () => {
        horizontalScrollbar.sync();
        if (pending) applyRestore();
        else if (!placed) placeInitial();
      },
      onScroll: () => {
        if (!pending) position();
      },
      getFontSize: () => codeFont.size,
      setFontSize: (value) => codeFont.set(value),
    });
    const highlighter = acquireCodeHighlighter();
    viewer = new CodeView(options(), highlighter.pool);
    viewer.setup(host);
    mounted = true;
    viewer.setItems([{ id, type: "file", file }]);
    const keydown = (event: KeyboardEvent) =>
      readerKeydown(event, find.actions.keyHandlers);
    root.addEventListener("keydown", keydown);
    return () => {
      mounted = false;
      horizontalScrollbar.destroy();
      stopEnvironment();
      root.removeEventListener("keydown", keydown);
      releaseCodeView(viewer, highlighter);
      viewer = undefined;
    };
  });

  $effect(() => {
    const current = options();
    if (mounted) untrack(() => syncViewerOptions(viewer, current));
  });
  // A scroll event may still be pending when the tab hides. Measure before the
  // ancestor's DOM update removes the reader from the layout.
  $effect.pre(() => {
    const showing = active;
    untrack(() => {
      if (!showing) position();
    });
  });
  $effect(() => {
    hint = coarse
      ? `点击代码行，再点右侧 ＋ 发送位置到${draftEnabled ? "草稿" : "终端"}`
      : showGutter
        ? "点击行号；Shift 点击选择终点"
        : "显示行号后可点击行号选择";
    return () => {
      hint = "";
    };
  });
</script>

<div
  bind:this={root}
  class="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden"
>
  <div class="flex shrink-0 items-center gap-1 border-b px-1 py-0.5">
    <Button
      variant="ghost"
      size="icon-sm"
      aria-label="文件内查找"
      aria-expanded={find.panel === "search"}
      aria-keyshortcuts="Control+f Meta+f"
      title="文件内查找（Ctrl / ⌘ F）"
      onclick={() => void find.actions.openPanel("search")}
      ><IconSearch aria-hidden="true" /></Button
    >
    <Button
      variant="ghost"
      size="icon-sm"
      aria-label="跳转行"
      aria-expanded={find.panel === "goto"}
      aria-keyshortcuts="Control+Alt+g Meta+Alt+g"
      title="跳转行（Ctrl / ⌘ + Alt + G）"
      onclick={() => void find.actions.openPanel("goto")}
      ><IconListNumbers aria-hidden="true" /></Button
    >
    <Button
      variant="ghost"
      size="icon-sm"
      aria-label="缩放"
      aria-expanded={find.panel === "zoom"}
      title="缩放"
      onclick={() => void find.actions.openPanel("zoom")}
      ><IconZoomIn aria-hidden="true" /></Button
    >
    <Button
      variant="ghost"
      size="icon-sm"
      aria-label="显示行号"
      title={showGutter ? "隐藏行号" : "显示行号"}
      aria-pressed={showGutter}
      onclick={toggleGutter}><IconNumber aria-hidden="true" /></Button
    >
  </div>
  <CodeReaderPanels
    panel={find.panel}
    query={find.search}
    line={selected?.start ?? line()}
    status={find.status}
    {...find.panelProps}
    ongoto={goto}
  />
  <div class="relative min-h-0 min-w-0 flex-1">
    <div
      bind:this={host}
      data-code-reader
      tabindex="-1"
      aria-label="只读代码"
      style:--diffs-font-size={`${codeFont.size}px`}
      style:--diffs-line-height={`${codeFont.lineHeight}px`}
      class="code-reader h-full overflow-x-hidden overflow-y-auto overscroll-contain outline-none contain-strict [overflow-anchor:none]"
    ></div>
  </div>
  <CodeHorizontalScrollbar
    {track}
    bind:trackElement
    onscroll={horizontalScrollbar.onTrackScroll}
  />
</div>

<style>
  .code-reader {
    --diffs-font-family: var(--font-mono, monospace);
  }
</style>
