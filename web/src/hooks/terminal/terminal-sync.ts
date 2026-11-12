import { osc52CopyText } from "../../domains/terminal/terminal-clipboard";
import type { TerminalGraphics } from "./terminal-graphics";
import type { Terminal } from "@xterm/xterm";

interface RendererInternals {
  renderRows?: (start: number, end: number) => void;
  clear?: () => void;
  // Only xterm's DOM renderer keeps one element per row.
  _rowElements?: unknown;
}

interface CoreInternals {
  _renderService?: {
    setRenderer?: (renderer: RendererInternals | undefined) => void;
    _renderer?: { value?: RendererInternals };
  };
  coreService?: { decPrivateModes?: { synchronizedOutput?: unknown } };
}

function core(term: Terminal) {
  return (term as unknown as { _core?: CoreInternals })._core;
}

// xterm 6.0 gates its repaints on DEC 2026 synchronized output, but its DOM
// renderer also repaints rows on its own: for the selection refresh that every
// scroll queues, for a resize, focus or cursor change, and it empties every row
// when a buffer is reset or switched. A synchronized redraw that scrolls, such
// as a TUI reprinting its transcript, or a replay after reset() then showed its
// partial or blank frames anyway. While an update is synchronized, the DOM
// renderer's own row repaints and clears become a buffered refresh, which
// xterm paints once when the update ends. The WebGL renderer already routes
// every repaint through the gated refresh and keeps its own glyph model, so it
// is left untouched. Internals are feature-detected; without them xterm keeps
// its own behavior.
export function holdRepaintsDuringSync(term: Terminal): () => void {
  const service = core(term)?._renderService;
  const setRenderer = service?.setRenderer;
  if (!service || typeof setRenderer !== "function") return () => {};
  const synchronized = () => term.modes.synchronizedOutputMode;
  const restores: (() => void)[] = [];
  const refresh = (start: number, end: number) =>
    term.refresh(Math.max(0, start), Math.min(term.rows - 1, end));

  const hold = (renderer: RendererInternals | undefined) => {
    if (!renderer || !Array.isArray(renderer._rowElements)) return;
    const { renderRows, clear } = renderer;
    if (typeof renderRows !== "function" || typeof clear !== "function") return;
    const heldRows = (start: number, end: number) => {
      if (synchronized()) refresh(start, end);
      else renderRows.call(renderer, start, end);
    };
    const heldClear = () => {
      if (synchronized()) refresh(0, term.rows - 1);
      else clear.call(renderer);
    };
    renderer.renderRows = heldRows;
    renderer.clear = heldClear;
    restores.push(() => {
      if (renderer.renderRows === heldRows) renderer.renderRows = renderRows;
      if (renderer.clear === heldClear) renderer.clear = clear;
    });
  };
  hold(service._renderer?.value);
  // The DOM renderer is replaced when WebGL loads or falls back.
  const heldSetRenderer: typeof setRenderer = (renderer) => {
    hold(renderer);
    setRenderer.call(service, renderer);
  };
  service.setRenderer = heldSetRenderer;

  return () => {
    if (service.setRenderer === heldSetRenderer)
      service.setRenderer = setRenderer;
    for (const restore of restores.splice(0)) restore();
  };
}

// Ends a synchronized update without writing bytes into the output stream:
// a replayed snapshot may stop inside an unfinished APC, OSC or DCS sequence,
// which would swallow a written `CSI ? 2026 l` as data. The modes object is
// replaced by reset(), so it is looked up when the update ends. Returns false
// when the internals are unavailable.
function endSynchronizedOutput(term: Terminal): boolean {
  const modes = core(term)?.coreService?.decPrivateModes;
  if (!modes || typeof modes.synchronizedOutput !== "boolean") return false;
  if (modes.synchronizedOutput) {
    modes.synchronizedOutput = false;
    // Paints the rows buffered while synchronized and clears xterm's timeout.
    term.refresh(0, term.rows - 1);
  }
  return true;
}

// Whether endSynchronizedOutput() can end an update this terminal starts.
function canEndSynchronizedOutput(term: Terminal): boolean {
  const modes = core(term)?.coreService?.decPrivateModes;
  return !!modes && typeof modes.synchronizedOutput === "boolean";
}

// CAN aborts an unfinished control sequence without executing it. Send it as
// bytes so xterm also abandons any incomplete UTF-8 prefix before the reset.
const resetBoundary = new Uint8Array([0x18]);
// DEC 2026 synchronized output around a replay: xterm keeps the old screen
// until the restored one is complete and paints it once, instead of a blank
// screen and every intermediate frame. It starts right after CAN, so the screen
// reset() empties is never painted, and again after reset(), which clears it.
const syncStart = new TextEncoder().encode("\x1b[?2026h");
const syncedResetBoundary = new Uint8Array([...resetBoundary, ...syncStart]);

export interface SnapshotRestoreContext {
  term: Terminal;
  graphics: TerminalGraphics | undefined;
  writeOutput(data: Uint8Array, parsed: () => void): void;
  resetFind(): void;
  scrollToBottom(): void;
  isDisposed(): boolean;
  isCurrent(version: number): boolean;
  setReplaying(value: boolean): void;
  setRestoring(value: boolean): void;
  setDrawn(run: string): void;
  isRecovering(): boolean;
}

export function restoreTerminalSnapshot(
  ctx: SnapshotRestoreContext,
  payload: Uint8Array,
  run: string | undefined,
  version: number,
  releaseGraphics: (() => void) | undefined,
  setRestoreFrame: (id: number) => void,
) {
  ctx.setRestoring(true);
  const synchronize = canEndSynchronizedOutput(ctx.term);
  ctx.writeOutput(synchronize ? syncedResetBoundary : resetBoundary, () => {
    if (ctx.isDisposed()) return;
    ctx.term.reset();
    releaseGraphics?.();
    ctx.resetFind();
    if (ctx.isCurrent(version)) ctx.setReplaying(true);
  });
  let replay = payload;
  if (synchronize) {
    replay = new Uint8Array(syncStart.length + payload.length);
    replay.set(syncStart);
    replay.set(payload, syncStart.length);
  }
  ctx.writeOutput(replay, () => {
    if (synchronize && !ctx.isDisposed() && ctx.isCurrent(version))
      endSynchronizedOutput(ctx.term);
    if (ctx.isCurrent(version)) ctx.setReplaying(false);
    if (payload.length && run) ctx.setDrawn(run);
    if (ctx.isDisposed() || !ctx.isCurrent(version)) return;
    setRestoreFrame(
      requestAnimationFrame(() => {
        if (ctx.isDisposed() || !ctx.isCurrent(version)) return;
        ctx.scrollToBottom();
        ctx.setRestoring(ctx.isRecovering());
      }),
    );
  });
}

export function registerTerminalProtocolHandlers(
  term: Terminal,
  isReplaying: () => boolean,
  copy: (text: string) => void,
  reportScheme: () => void,
) {
  const oscClipboard = term.parser.registerOscHandler(52, (data) => {
    const text = osc52CopyText(data);
    if (text === undefined) return false;
    if (!isReplaying()) copy(text);
    return true;
  });
  const schemeQuery = term.parser.registerCsiHandler(
    { prefix: "?", final: "n" },
    (params) => {
      if (params.length !== 1 || params[0] !== 996) return false;
      if (!isReplaying()) reportScheme();
      return true;
    },
  );
  return () => {
    oscClipboard.dispose();
    schemeQuery.dispose();
  };
}
