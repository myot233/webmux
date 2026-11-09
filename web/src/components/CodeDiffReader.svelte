<script lang="ts">
  import { onMount, untrack } from "svelte";
  import { SvelteMap } from "svelte/reactivity";
  import {
    CodeView,
    type CodeViewOptions,
    type FileDiffMetadata,
    type SelectedLineRange,
    type SelectionSide,
  } from "@pierre/diffs";
  import { captureCursor, restoreCursor } from "../code-search";
  import { createCodeFind } from "../code-find.svelte";
  import { codeFont } from "../hooks/code-font-preference.svelte";
  import { settings, changeSettings } from "../settings.svelte";
  import { defaultCodeReader } from "../settings-intent";
  import { acquireCodeHighlighter } from "../code-highlighter";
  import {
    codeTokenizeLimit,
    codeUnsafeCSS,
    pierreTheme,
  } from "../code-pierre";
  import {
    bindHorizontalScrollbar,
    readerKeydown,
    releaseCodeView,
    syncViewerOptions,
    watchReader,
    type CodeHorizontalTrack,
  } from "../code-reader-controls";
  import type { CodeDiffReaderState } from "../schemas";
  import { Button } from "$lib/components/ui/button";
  import CodeHorizontalScrollbar from "./CodeHorizontalScrollbar.svelte";
  import CodeReaderPanels from "./CodeReaderPanels.svelte";
  import IconSearch from "~icons/tabler/search";
  import IconTextWrap from "~icons/tabler/text-wrap";
  import IconZoomIn from "~icons/tabler/zoom-in";

  let {
    fileDiff,
    side,
    expired,
    draftEnabled,
    onselect,
    restore,
    hint = $bindable(""),
  }: {
    fileDiff: FileDiffMetadata;
    side: "old" | "new";
    expired: boolean;
    draftEnabled: boolean;
    onselect: (range: SelectedLineRange | null) => void;
    // A captured reading state, applied once the diff has rendered with
    // layout.
    restore?: CodeDiffReaderState;
    // Selection guidance, rendered by the document's action bar.
    hint?: string;
  } = $props();

  const overflow = $derived(
    (settings.values.codeReader ?? defaultCodeReader).diffOverflow,
  );
  let width = $state(0);
  let dark = $state(false);
  let coarse = $state(false);
  let selected = $state<SelectedLineRange | null>(null);
  let host: HTMLDivElement;
  let root: HTMLDivElement;
  let trackElement = $state<HTMLDivElement>();
  let track = $state<CodeHorizontalTrack>({ shown: false, width: 0 });
  let viewer: CodeView | undefined;
  let mounted = $state(false);
  const horizontalScrollbar = bindHorizontalScrollbar(
    () => viewer,
    () => trackElement,
    (next) => {
      if (track.shown !== next.shown || track.width !== next.width)
        track = next;
    },
  );
  const id = "diff";
  const expansionLines = 30;
  // Until applied, `capture()` returns it unchanged.
  let pending = untrack(() => restore);
  // Unmodified lines revealed per hunk separator. The library keeps its own
  // count privately, so each rendered instance's expandHunk is wrapped to
  // record the same totals.
  const expanded = new SvelteMap<
    number,
    { fromStart: number; fromEnd: number }
  >();
  const recorded = new WeakSet<object>();
  // The top line last measured while the diff had layout; a hidden tab
  // reports this one.
  let knownTop: CodeDiffReaderState["top"];
  const diffStyle = $derived(width >= 760 ? "split" : "unified");
  const hasChanges = $derived(fileDiff.hunks.length > 0);
  const totals = $derived(
    fileDiff.hunks.reduce(
      (sum, hunk) => ({
        additions: sum.additions + hunk.additionLines,
        deletions: sum.deletions + hunk.deletionLines,
      }),
      { additions: 0, deletions: 0 },
    ),
  );
  const sourceSide = $derived<SelectionSide>(
    side === "old" ? "deletions" : "additions",
  );
  const sourceLines = $derived(
    side === "old" ? fileDiff.deletionLines : fileDiff.additionLines,
  );
  const find = createCodeFind(
    () => sourceLines,
    () => host,
    () => root,
    scrollToLine,
    () => hasChanges,
  );

  export function clearLineAction() {
    selected = null;
    viewer?.clearSelectedLines({ notify: false });
  }
  function choose(range: SelectedLineRange | null) {
    selected = range;
    onselect(range);
  }
  // The rendered diff instance, its expandHunk wrapped to record expansions
  // exactly as the library's renderer counts them.
  function diffInstance() {
    const item = viewer?.getRenderedItems()[0];
    if (item?.type !== "diff") return;
    const { instance } = item;
    if (!recorded.has(instance)) {
      recorded.add(instance);
      const expand = instance.expandHunk;
      instance.expandHunk = (index, direction, count) => {
        // The schema needs finite counts; this exceeds any region.
        const lines = Math.min(count ?? expansionLines, 1e9);
        const region = expanded.get(index) ?? { fromStart: 0, fromEnd: 0 };
        if (direction !== "down")
          region.fromStart = Math.min(region.fromStart + lines, 1e9);
        if (direction !== "up")
          region.fromEnd = Math.min(region.fromEnd + lines, 1e9);
        expanded.set(index, region);
        expand(index, direction, count);
      };
    }
    return instance;
  }
  // The first source row at least partly below the top edge.
  function topLine(): CodeDiffReaderState["top"] {
    if (host.clientHeight === 0) return knownTop;
    const item = viewer?.getRenderedItems()[0];
    const edge = host.getBoundingClientRect().top;
    for (const row of item?.element.shadowRoot?.querySelectorAll<HTMLElement>(
      "[data-line]",
    ) ?? []) {
      const line = Number(row.dataset.line);
      if (!Number.isInteger(line) || line < 1) continue;
      if (row.getBoundingClientRect().bottom <= edge + 0.5) continue;
      // Split columns name their side; unified rows only by their type.
      const deletions = row.closest("[data-deletions]")
        ? true
        : row.closest("[data-additions]")
          ? false
          : (row.dataset.lineType ?? "").includes("deletion");
      knownTop = { line, side: deletions ? "deletions" : "additions" };
      return knownTop;
    }
    return knownTop;
  }
  export function capture(): CodeDiffReaderState {
    if (pending) return pending;
    const range = viewer?.getSelectedLines()?.range ?? selected;
    return {
      selected: range && {
        start: range.start,
        end: range.end,
        ...(range.side ? { side: range.side } : {}),
        ...(range.endSide ? { endSide: range.endSide } : {}),
      },
      expanded: [...expanded].map(([index, region]) => ({
        index,
        ...region,
      })),
      search: $state.snapshot(find.search),
      ...(find.panel ? { panel: find.panel } : {}),
      ...captureCursor(find.cursor, find.matches),
      ...(viewer ? { top: topLine() } : {}),
    };
  }
  // Applies `pending` once the diff has rendered with layout.
  function applyRestore() {
    const state = pending;
    if (!state || host.clientHeight === 0) return;
    const instance = diffInstance();
    if (!instance) return;
    pending = undefined;
    for (const { index, fromStart, fromEnd } of state.expanded) {
      if (fromStart > 0) instance.expandHunk(index, "up", fromStart);
      if (fromEnd > 0) instance.expandHunk(index, "down", fromEnd);
    }
    if (state.selected) {
      viewer?.setSelectedLines(
        { id, range: state.selected },
        { notify: false },
      );
      choose(state.selected);
    }
    find.search = state.search;
    find.panel = state.panel;
    find.cursor = restoreCursor(state.cursor, find.matches);
    if (state.top) {
      knownTop = state.top;
      viewer?.scrollTo({
        type: "line",
        id,
        lineNumber: state.top.line,
        side: state.top.side,
        align: "start",
      });
    }
  }
  function scrollToLine(line: number) {
    if (!hasChanges || !sourceLines.length) return;
    const number = Math.max(1, Math.min(sourceLines.length, line));
    const startOf = (hunk: FileDiffMetadata["hunks"][number]) =>
      side === "old" ? hunk.deletionStart : hunk.additionStart;
    let region = fileDiff.hunks.findIndex((hunk) => number < startOf(hunk));
    if (region < 0) region = fileDiff.hunks.length;
    const previous = fileDiff.hunks[region - 1];
    const previousEnd = previous
      ? startOf(previous) +
        (side === "old" ? previous.deletionCount : previous.additionCount) -
        1
      : 0;
    if (number > previousEnd) {
      if (expired) return;
      diffInstance()?.expandHunk(region, "both", Number.POSITIVE_INFINITY);
    }
    viewer?.scrollTo({
      type: "line",
      id,
      lineNumber: number,
      side: sourceSide,
      align: "center",
    });
    pickLine(number, sourceSide);
  }
  function pickLine(line: number, lineSide: SelectionSide) {
    const range = { start: line, end: line, side: lineSide };
    viewer?.setSelectedLines({ id, range }, { notify: false });
    choose(range);
  }
  function goto(value: string) {
    const match = /^([+-])?(\d+)?(:\d+)?(%)?$/.exec(value.trim());
    if (match) {
      const current = selected?.start ?? 1;
      const [, sign, digits, , percent] = match;
      let target = digits ? Number(digits) : current;
      if (digits && percent) {
        target =
          Math.round((sourceLines.length * target) / 100) *
            (sign === "-" ? -1 : 1) +
          (sign ? current : 0);
      } else if (digits && sign)
        target = current + target * (sign === "-" ? -1 : 1);
      scrollToLine(target);
    }
    find.panel = undefined;
    host.focus();
  }
  function options(): CodeViewOptions<undefined, undefined> {
    return {
      ...pierreTheme(dark),
      diffStyle,
      overflow,
      diffIndicators: "bars",
      lineDiffType: "word-alt",
      maxLineDiffLength: codeTokenizeLimit,
      tokenizeMaxLineLength: codeTokenizeLimit,
      disableFileHeader: true,
      enableLineSelection: true,
      lineHoverHighlight: "number",
      expansionLineCount: expansionLines,
      hunkSeparators: "line-info",
      itemMetrics: codeFont.itemMetrics,
      layout: { paddingTop: 0, paddingBottom: 0, gap: 0 },
      onSelectedLinesChange: (selection) => choose(selection?.range ?? null),
      onLineNumberClick(props) {
        if (
          props.type !== "diff-line" ||
          props.event.shiftKey ||
          (selected && selected.start !== selected.end)
        )
          return;
        // 1.3.6 toggles a selected row by number alone, even when clicking the
        // opposite side. Explicitly retain the clicked side for references.
        pickLine(props.lineNumber, props.annotationSide);
      },
      onLineClick(props) {
        if (!coarse || props.type !== "diff-line") return;
        const nativeSelection = document.getSelection();
        if (nativeSelection && !nativeSelection.isCollapsed) return;
        pickLine(props.lineNumber, props.annotationSide);
      },
      // CodeView publishes getRenderedItems() after the item render callback.
      // Defer until then so plain-text files get named, focusable controls on
      // the first render, even when highlighting needs no second render.
      onPostRender: () =>
        queueMicrotask(() => {
          horizontalScrollbar.sync();
          enhanceControls();
          applyRestore();
        }),
      // Source text is rendered by the library's escaped token renderer.
      unsafeCSS: `${codeUnsafeCSS}
        [data-expand-button][aria-disabled="true"] { opacity: .45; cursor: not-allowed; }
        [data-expand-button]:focus-visible { outline: 2px solid var(--primary); outline-offset: -2px; }
      `,
    };
  }
  // The library's expand controls are role=button divs. Supply names and
  // keyboard handling here, and enforce the frozen-snapshot policy.
  function enhanceControls() {
    diffInstance();
    for (const item of viewer?.getRenderedItems() ?? []) {
      for (const button of item.element.shadowRoot?.querySelectorAll<HTMLElement>(
        "[data-expand-button]",
      ) ?? []) {
        const all =
          button.hasAttribute("data-expand-all-button") ||
          button.hasAttribute("data-expand-both");
        button.setAttribute(
          "aria-label",
          all
            ? "展开全部上下文"
            : button.hasAttribute("data-expand-up")
              ? `向上展开 ${expansionLines} 行`
              : `向下展开 ${expansionLines} 行`,
        );
        button.setAttribute("aria-disabled", String(expired));
        button.tabIndex = expired ? -1 : 0;
        if (button.hasAttribute("data-expand-all-button"))
          button.textContent = "展开全部";
      }
      for (const label of item.element.shadowRoot?.querySelectorAll<HTMLElement>(
        "[data-unmodified-lines]",
      ) ?? []) {
        const count = label.textContent?.match(/\d+/)?.[0];
        if (count) label.textContent = `${count} 行未修改`;
      }
    }
  }
  function expandTarget(event: Event) {
    return event
      .composedPath()
      .find(
        (node): node is HTMLElement =>
          node instanceof HTMLElement &&
          (node.hasAttribute("data-expand-button") ||
            node.hasAttribute("data-unmodified-lines")),
      );
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
        width = host.clientWidth;
        horizontalScrollbar.sync();
        applyRestore();
      },
      onScroll: () => {
        if (!pending) topLine();
      },
      getFontSize: () => codeFont.size,
      setFontSize: (value) => codeFont.set(value),
    });
    width = host.clientWidth;
    const highlighter = acquireCodeHighlighter();
    viewer = new CodeView(options(), highlighter.pool);
    viewer.setup(host);
    viewer.setItems([{ id, type: "diff", fileDiff }]);
    mounted = true;
    const guardExpansion = (event: Event) => {
      if (expired && expandTarget(event)) {
        event.preventDefault();
        event.stopImmediatePropagation();
      }
    };
    const keydown = (event: KeyboardEvent) => {
      const button = expandTarget(event);
      if (button && (event.key === "Enter" || event.key === " ")) {
        event.preventDefault();
        if (!expired) {
          const separator = button.closest<HTMLElement>("[data-expand-index]");
          const index = Number(separator?.dataset.expandIndex);
          const instance = diffInstance();
          if (instance && Number.isInteger(index)) {
            const all =
              button.hasAttribute("data-expand-all-button") ||
              button.hasAttribute("data-expand-both");
            instance.expandHunk(
              index,
              all
                ? "both"
                : button.hasAttribute("data-expand-up")
                  ? "up"
                  : "down",
              all ? Number.POSITIVE_INFINITY : expansionLines,
            );
          }
        }
      } else readerKeydown(event, find.actions.keyHandlers);
    };
    const selectionChanged = () => {
      if (!coarse) return;
      const nativeSelection = document.getSelection();
      if (nativeSelection && !nativeSelection.isCollapsed) selected = null;
    };
    host.addEventListener("pointerdown", guardExpansion, true);
    host.addEventListener("click", guardExpansion, true);
    root.addEventListener("keydown", keydown);
    document.addEventListener("selectionchange", selectionChanged);
    return () => {
      mounted = false;
      horizontalScrollbar.destroy();
      stopEnvironment();
      host.removeEventListener("pointerdown", guardExpansion, true);
      host.removeEventListener("click", guardExpansion, true);
      root.removeEventListener("keydown", keydown);
      document.removeEventListener("selectionchange", selectionChanged);
      releaseCodeView(viewer, highlighter);
      viewer = undefined;
    };
  });
  $effect(() => {
    const current = options();
    if (mounted)
      untrack(() =>
        syncViewerOptions(viewer, current, (range) => choose(range)),
      );
  });
  $effect(() => {
    void expired;
    if (mounted) enhanceControls();
  });
  $effect(() => {
    hint = !hasChanges
      ? "当前可引用整个文件"
      : coarse
        ? `点击代码行，再发送到${draftEnabled ? "草稿" : "终端"}`
        : "点击行号选择；Shift 点击选择范围";
    return () => {
      hint = "";
    };
  });
</script>

<div
  bind:this={root}
  class="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden"
>
  <div
    class="flex min-w-0 shrink-0 flex-wrap items-center gap-1 border-b px-2 py-1"
  >
    <span
      class="mr-auto flex gap-2 font-mono text-xs tabular-nums"
      role="img"
      aria-label={`新增 ${totals.additions} 行，删除 ${totals.deletions} 行`}
      ><span class="text-success">+{totals.additions}</span><span
        class="text-destructive">−{totals.deletions}</span
      ></span
    >
    <Button
      variant="ghost"
      size="icon-sm"
      aria-label="文件内查找"
      aria-expanded={find.panel === "search"}
      aria-keyshortcuts="Control+f Meta+f"
      title="文件内查找（Ctrl / ⌘ F）"
      disabled={!hasChanges}
      onclick={() => void find.actions.openPanel("search")}
      ><IconSearch aria-hidden="true" /></Button
    >
    <Button
      variant="ghost"
      size="icon-sm"
      aria-label="自动换行"
      title="自动换行"
      aria-pressed={overflow === "wrap"}
      onclick={() => {
        changeSettings({
          codeReader: {
            ...(settings.values.codeReader ?? defaultCodeReader),
            diffOverflow: overflow === "wrap" ? "scroll" : "wrap",
          },
        });
      }}><IconTextWrap aria-hidden="true" /></Button
    >
    <Button
      variant="ghost"
      size="icon-sm"
      title="缩放"
      aria-label="缩放"
      aria-expanded={find.panel === "zoom"}
      onclick={() => void find.actions.openPanel("zoom")}
      ><IconZoomIn aria-hidden="true" /></Button
    >
  </div>
  <CodeReaderPanels
    panel={find.panel}
    query={find.search}
    line={selected?.start ?? 1}
    status={`${side === "old" ? "修改前" : "修改后"} · ${find.status}`}
    {...find.panelProps}
    ongoto={goto}
  />
  {#if diffStyle === "split"}
    <div
      class="grid shrink-0 grid-cols-2 border-b bg-muted/40 font-mono text-xs text-muted-foreground"
    >
      <span class="truncate border-r px-3 py-1">修改前</span><span
        class="truncate px-3 py-1">修改后</span
      >
    </div>
  {/if}
  <div class="relative min-h-0 min-w-0 flex-1">
    <div
      bind:this={host}
      class="code-diff-reader h-full overflow-x-hidden overflow-y-auto overscroll-contain outline-none contain-strict [overflow-anchor:none]"
      style:--diffs-font-size={`${codeFont.size}px`}
      style:--diffs-line-height={`${codeFont.lineHeight}px`}
      tabindex="-1"
      aria-label="只读代码"
    ></div>
    {#if !hasChanges}
      <p
        class="absolute inset-x-0 top-8 text-center text-sm text-muted-foreground"
        role="status"
      >
        文件内容没有变化；可通过“完整文件”查看内容
      </p>
    {/if}
  </div>
  <CodeHorizontalScrollbar
    {track}
    bind:trackElement
    onscroll={horizontalScrollbar.onTrackScroll}
  />
</div>

<style>
  .code-diff-reader {
    --diffs-font-family: var(--font-mono, monospace);
  }
</style>
