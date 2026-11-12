import { tick } from "svelte";
import type {
  CodeView,
  CodeViewOptions,
  SelectedLineRange,
} from "@pierre/diffs";
import { observeDarkTheme } from "./code-pierre";
import {
  stepMatchLine,
  type MatchCursor,
  type SearchSpec,
} from "./code-search";

export type CodeReaderPanel = "search" | "goto" | "zoom";

/** Focuses the open search, go-to-line or zoom control after it renders. */
async function focusCodePanel(root: ParentNode) {
  await tick();
  root
    .querySelector<HTMLElement>(
      "[data-code-search] input, [data-code-goto-line] input, [data-code-zoom] [role='slider']",
    )
    ?.focus();
}

/** Toggles the named panel, or closes it when it is already open. */
function togglePanel<Panel extends string>(
  current: Panel | undefined,
  value: Panel,
  toggle: boolean,
): Panel | undefined {
  return toggle && current === value ? undefined : value;
}

/** Opens or closes a reader panel and focuses its input. */
async function openCodePanel(
  root: ParentNode,
  current: CodeReaderPanel | undefined,
  value: CodeReaderPanel,
  toggle: boolean,
  setPanel: (panel: CodeReaderPanel | undefined) => void,
) {
  setPanel(togglePanel(current, value, toggle));
  await focusCodePanel(root);
}

/** Closes a reader panel and returns focus to its scroll surface. */
function closeCodePanel(
  host: HTMLElement,
  setPanel: (panel: undefined) => void,
) {
  setPanel(undefined);
  host.focus();
}

/** Panel toggling, search edits and match stepping for a code reader. */
export function createReaderPanelActions(options: {
  root: () => ParentNode;
  host: () => HTMLElement;
  getPanel: () => CodeReaderPanel | undefined;
  setPanel: (panel: CodeReaderPanel | undefined) => void;
  getCursor: () => MatchCursor | undefined;
  setCursor: (cursor: MatchCursor | undefined) => void;
  setSearch: (search: SearchSpec) => void;
  getMatches: () => readonly number[];
  reveal: (line: number) => void;
  // Diff readers have no searchable lines until their diff is ready.
  enabled?: () => boolean;
}) {
  const actions = {
    async openPanel(value: CodeReaderPanel, toggle = true) {
      if (value !== "zoom" && options.enabled && !options.enabled()) return;
      await openCodePanel(
        options.root(),
        options.getPanel(),
        value,
        toggle,
        options.setPanel,
      );
    },
    query(value: SearchSpec) {
      options.setSearch(value);
      options.setCursor(undefined);
    },
    closePanel() {
      closeCodePanel(options.host(), options.setPanel);
    },
    nextMatch(direction: 1 | -1) {
      options.setCursor(
        advanceMatch(
          options.getCursor(),
          options.getMatches(),
          direction,
          options.reveal,
        ),
      );
    },
  };
  return {
    ...actions,
    keyHandlers: readerKeyHandlers(
      (value, toggle) => actions.openPanel(value, toggle),
      (direction) => actions.nextMatch(direction),
      () => actions.closePanel(),
    ),
  };
}

/** Key handlers shared by both readers; `openPanel` opens without toggling. */
function readerKeyHandlers(
  openPanel: (value: "search" | "goto", toggle?: boolean) => unknown,
  nextMatch: (direction: 1 | -1) => void,
  close: () => void,
) {
  return {
    openSearch: () => void openPanel("search", false),
    openGoto: () => void openPanel("goto", false),
    nextMatch,
    close,
  };
}

/** Shared search and navigation keys for both code readers. */
export function readerKeydown(
  event: KeyboardEvent,
  handlers: {
    openSearch: () => void;
    openGoto: () => void;
    nextMatch: (direction: 1 | -1) => void;
    close: () => void;
  },
) {
  const mod = event.metaKey || event.ctrlKey;
  const key = event.key.toLowerCase();
  if (mod && key === "f") {
    event.preventDefault();
    handlers.openSearch();
  } else if (mod && event.altKey && key === "g") {
    event.preventDefault();
    handlers.openGoto();
  } else if (event.key === "F3" || (mod && key === "g")) {
    event.preventDefault();
    handlers.nextMatch(event.shiftKey ? -1 : 1);
  } else if (event.key === "Escape") {
    handlers.close();
  }
}

/**
 * Measures on every scroll event, not on a later frame: a tab can be hidden
 * before the next frame, and its last position must already be captured.
 */
function watchScroll(host: HTMLElement, measure: () => void) {
  host.addEventListener("scroll", measure, { passive: true });
  return () => host.removeEventListener("scroll", measure);
}

/** Pointer, theme, resize, scroll and zoom wiring shared by the code readers. */
export function watchReader(
  host: HTMLElement,
  handlers: {
    onPointer: (coarse: boolean) => void;
    onTheme: (dark: boolean) => void;
    onResize: () => void;
    onScroll: () => void;
    getFontSize?: () => number;
    setFontSize?: (size: number) => void;
  },
) {
  const pointer = matchMedia("(pointer: coarse)");
  const updatePointer = () => handlers.onPointer(pointer.matches);
  updatePointer();
  pointer.addEventListener("change", updatePointer);
  const stopTheme = observeDarkTheme(handlers.onTheme);
  const resize = new ResizeObserver(handlers.onResize);
  resize.observe(host);
  const stopScroll = watchScroll(host, handlers.onScroll);
  const stopZoom =
    handlers.getFontSize && handlers.setFontSize
      ? watchCodeZoom(host, handlers.getFontSize, handlers.setFontSize)
      : () => {};
  return () => {
    resize.disconnect();
    stopTheme();
    pointer.removeEventListener("change", updatePointer);
    stopScroll();
    stopZoom();
  };
}

/**
 * Ctrl+wheel and two-finger pinch zoom on a code reader's scroll surface.
 * One-finger scrolling stays native; two fingers scale the code font size.
 */
function watchCodeZoom(
  host: HTMLElement,
  getFontSize: () => number,
  setFontSize: (size: number) => void,
) {
  let pinch: { distance: number; fontSize: number } | undefined;
  let wheelRemainder = 0;
  const distanceOf = (touches: TouchList) => {
    const [a, b] = [touches[0], touches[1]];
    if (!a || !b) return;
    return Math.hypot(a.clientX - b.clientX, a.clientY - b.clientY) || 1;
  };
  const touchstart = (event: TouchEvent) => {
    if (event.touches.length !== 2) return;
    event.stopPropagation();
    const distance = distanceOf(event.touches);
    if (distance) pinch = { distance, fontSize: getFontSize() };
  };
  const touchmove = (event: TouchEvent) => {
    if (!pinch || event.touches.length !== 2) return;
    event.preventDefault();
    event.stopPropagation();
    const distance = distanceOf(event.touches);
    if (distance)
      setFontSize(Math.round((pinch.fontSize * distance) / pinch.distance));
  };
  const touchend = (event: TouchEvent) => {
    if (!pinch || event.touches.length >= 2) return;
    event.stopPropagation();
    pinch = undefined;
  };
  const wheel = (event: WheelEvent) => {
    if (!event.ctrlKey) return;
    event.preventDefault();
    event.stopPropagation();
    const unit =
      event.deltaMode === WheelEvent.DOM_DELTA_LINE
        ? 16
        : event.deltaMode === WheelEvent.DOM_DELTA_PAGE
          ? 100
          : 1;
    const delta = -event.deltaY * unit;
    if (delta === 0) return;
    if (Math.sign(delta) !== Math.sign(wheelRemainder)) wheelRemainder = 0;
    wheelRemainder += delta / 50;
    const steps =
      Math.abs(wheelRemainder) >= 1
        ? Math.trunc(wheelRemainder)
        : Math.abs(delta) >= 15
          ? Math.sign(delta)
          : 0;
    if (steps !== 0) {
      wheelRemainder -= steps;
      setFontSize(getFontSize() + steps);
    }
  };
  host.addEventListener("touchstart", touchstart, { passive: true });
  host.addEventListener("touchmove", touchmove, { passive: false });
  host.addEventListener("touchend", touchend);
  host.addEventListener("touchcancel", touchend);
  host.addEventListener("wheel", wheel, { passive: false });
  return () => {
    host.removeEventListener("touchstart", touchstart);
    host.removeEventListener("touchmove", touchmove);
    host.removeEventListener("touchend", touchend);
    host.removeEventListener("touchcancel", touchend);
    host.removeEventListener("wheel", wheel);
  };
}

/** Moves to the next match, reveals its line and returns the new cursor. */
function advanceMatch(
  cursor: MatchCursor | undefined,
  matches: readonly number[],
  direction: 1 | -1,
  reveal: (line: number) => void,
) {
  const next = stepMatchLine(cursor, matches, direction);
  if (!next) return cursor;
  reveal(next.line);
  return next.cursor;
}

/** Releases a reader's rendered DOM and its shared highlighter. */
export function releaseCodeView(
  viewer: CodeView | undefined,
  highlighter: { release: () => void },
) {
  viewer?.cleanUp();
  highlighter.release();
}

/** Re-renders a reader with new options and keeps its line selection. */
export function syncViewerOptions(
  viewer: CodeView | undefined,
  options: CodeViewOptions<undefined, undefined>,
  onSelection?: (range: SelectedLineRange) => void,
) {
  if (!viewer) return;
  const selection = viewer.getSelectedLines();
  viewer.setOptions(options);
  viewer.render();
  if (selection) {
    viewer.setSelectedLines(selection, { notify: false });
    onSelection?.(selection.range);
  }
}

export type CodeHorizontalTrack = {
  shown: boolean;
  width: number;
};

/**
 * Keeps a viewport-pinned horizontal scrollbar synchronized with the
 * `<code data-code>` scrollers inside a virtualized `CodeView`.
 *
 * `CodeView` renders an overscrolled vertical window inside its sticky
 * container, so `[data-code]`'s bottom edge lies below the visible viewport
 * until the end of the file. A dedicated bottom track stays visible at the
 * bottom of the reader whenever rendered lines overflow horizontally.
 */
export function bindHorizontalScrollbar(
  viewer: () => CodeView | undefined,
  track: () => HTMLElement | undefined,
  onChange: (state: CodeHorizontalTrack) => void,
) {
  let left = 0;
  let frame = 0;
  const scrollers = new Set<HTMLElement>();
  const observed = new Set<Element>();

  function currentScrollers(): HTMLElement[] {
    const item = viewer()?.getRenderedItems()[0];
    if (!item) return [];
    return [
      ...(item.element.shadowRoot?.querySelectorAll<HTMLElement>(
        "[data-code]",
      ) ?? []),
    ];
  }

  function applyLeft(targetLeft: number, source?: HTMLElement) {
    left = Math.max(0, targetLeft);
    for (const scroller of currentScrollers()) {
      if (scroller !== source && Math.abs(scroller.scrollLeft - left) > 0.5)
        scroller.scrollLeft = left;
    }
    const bar = track();
    if (bar && bar !== source && Math.abs(bar.scrollLeft - left) > 0.5)
      bar.scrollLeft = left;
  }

  const onCodeScroll = (event: Event) => {
    const target = event.currentTarget;
    if (!(target instanceof HTMLElement)) return;
    if (Math.abs(target.scrollLeft - left) <= 0.5) return;
    applyLeft(target.scrollLeft, target);
  };

  const onTrackScroll = () => {
    const bar = track();
    if (!bar || Math.abs(bar.scrollLeft - left) <= 0.5) return;
    applyLeft(bar.scrollLeft, bar);
  };

  function measure() {
    const active = currentScrollers();
    const bar = track();
    for (const old of scrollers) {
      if (!active.includes(old)) {
        old.removeEventListener("scroll", onCodeScroll);
        scrollers.delete(old);
      }
    }
    const nextObserved = new Set<Element>();
    if (bar) nextObserved.add(bar);
    let maxOverflow = 0;
    for (const scroller of active) {
      if (!scrollers.has(scroller)) {
        scrollers.add(scroller);
        scroller.addEventListener("scroll", onCodeScroll, { passive: true });
      }
      nextObserved.add(scroller);
      for (const child of scroller.children) nextObserved.add(child);
      const overflow = scroller.scrollWidth - scroller.clientWidth;
      if (overflow > maxOverflow) maxOverflow = overflow;
    }
    for (const old of observed) {
      if (!nextObserved.has(old)) {
        resize.unobserve(old);
        observed.delete(old);
      }
    }
    for (const el of nextObserved) {
      if (!observed.has(el)) {
        observed.add(el);
        resize.observe(el);
      }
    }
    if (maxOverflow <= 1) {
      left = 0;
      onChange({ shown: false, width: 0 });
      return;
    }
    const baseWidth =
      bar && bar.clientWidth > 0
        ? bar.clientWidth
        : (active[0]?.clientWidth ?? 0);
    const width = Math.ceil(baseWidth + maxOverflow);
    // In a single-scroller reader, scrollToColumn sets scroller.scrollLeft
    // directly before its async scroll event; in split diff, any difference
    // between scrollers means a scroll event is in flight and left will update.
    const sourceLeft =
      active.length === 1 && active[0]!.scrollLeft > 0
        ? active[0]!.scrollLeft
        : left;
    const targetLeft = Math.min(sourceLeft, maxOverflow);
    onChange({ shown: true, width });
    applyLeft(targetLeft);
    requestAnimationFrame(() => {
      const currentBar = track();
      if (currentBar && Math.abs(currentBar.scrollLeft - left) > 0.5)
        currentBar.scrollLeft = left;
    });
  }

  function scheduleMeasure() {
    if (frame) return;
    frame = requestAnimationFrame(() => {
      frame = 0;
      measure();
    });
  }

  const resize = new ResizeObserver(scheduleMeasure);

  return {
    sync: measure,
    onTrackScroll,
    destroy() {
      if (frame) cancelAnimationFrame(frame);
      frame = 0;
      resize.disconnect();
      observed.clear();
      for (const scroller of scrollers)
        scroller.removeEventListener("scroll", onCodeScroll);
      scrollers.clear();
    },
  };
}
