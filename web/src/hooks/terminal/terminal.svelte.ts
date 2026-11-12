import { onMount, untrack } from "svelte";
import type { Terminal } from "@xterm/xterm";
import { SearchAddon } from "@xterm/addon-search";
import {
  executeTerminalFind,
  terminalFindError,
  terminalFindLimit,
  type TerminalFindOptions,
  type TerminalFindResults,
} from "../../domains/terminal/terminal-search";
import { terminalFont } from "./terminal-font-preference.svelte";
import { modifiedInput, type Modifiers } from "../../platform/input";
import { loadTerminalFont } from "../../platform/fonts";
import { hasTouchInput } from "../../platform/touch-input";
import { attachTouchGestures } from "./touch";
import {
  createTerminalRenderer,
  type TerminalRenderer,
} from "./terminal-renderer";
import {
  attachTerminalInput,
  bindTerminalViewport,
  fitTerminalGrid,
  toggleTerminalKeyboard,
  type TerminalClipboard,
  type TerminalUserInput,
} from "./terminal-input";
import { createTerminalSocket } from "./terminal-socket.svelte";
export type { TerminalController } from "../../domains/terminal/terminal-search";
import type { TerminalController } from "../../domains/terminal/terminal-search";

// One session's terminal: composes the xterm renderer, the session socket,
// keyboard/IME/draft input and touch gestures on the host from
// `getContainer`, and releases all of them when the component unmounts.
export function useTerminal(
  getContainer: () => HTMLDivElement,
  id: string,
  clipboard: TerminalClipboard,
  isVisible: () => boolean = () => true,
  userInput?: TerminalUserInput,
  isKeyboardDismissed: () => boolean = () => false,
  isSizeLocked: () => boolean = () => false,
  isActive: () => boolean = () => true,
  manageChrome = true,
  // A soft keyboard opened from the keyboard button: keep the grid while
  // only the host height changes. Width and font changes still refit.
  isHeightHeld: () => boolean = () => false,
): TerminalController {
  let terminal: Terminal | undefined;
  let renderer: TerminalRenderer | undefined;
  let scrollOnFit = false;
  // A locked grid still needs its first fit; afterwards host changes keep it
  // until the user unlocks. A hidden host drops the state so showing it again
  // still adopts the available box.
  let sized = false;
  // Whether output should keep the newest rows in view. Kept apart from
  // `atBottom` because a host resize moves the grid under a stationary
  // scrollTop; only a user scroll may stop following the tail.
  let follow = true;
  // Host width and font the grid was last fitted at, for the height hold.
  // offsetWidth includes a scrollbar the held grid itself may cause.
  let fitWidth = 0;
  let fitFont = 0;
  // Locked and held grids may exceed the host; both scroll it to the tail.
  const gridFixed = () => isSizeLocked() || isHeightHeld();
  const stream = createTerminalSocket(id, manageChrome);
  const fontSize = $derived(terminalFont.size);
  const setFontSize = terminalFont.set;
  let modifiers = $state<Modifiers>({
    ctrl: false,
    alt: false,
    shift: false,
  });
  let shiftLocked = $state(false);
  let selection = $state("");
  let search: SearchAddon | undefined;
  let canFind = $state(false);
  let findResults = $state<TerminalFindResults | null>(null);
  let findReplayed = $state(false);
  let bufferType = $state<"normal" | "alternate">("normal");
  let scrollback = $state(10000);
  let findActive = false;
  function clearFind() {
    findActive = false;
    search?.clearDecorations();
    terminal?.clearSelection();
    // clearDecorations does not emit a result-reset event.
    findResults = null;
    findReplayed = false;
  }
  function find(
    query: string,
    options: TerminalFindOptions,
    previous: boolean,
    incremental = false,
  ) {
    if (!query) {
      clearFind();
      return;
    }
    if (
      !terminal ||
      !search ||
      stream.restoring ||
      terminalFindError(query, options.regex)
    )
      return;
    const wasActive = findActive;
    findActive = true;
    findReplayed = false;
    executeTerminalFind(
      terminal,
      search,
      query,
      options,
      previous,
      incremental,
      wasActive,
    );
  }
  // Whether the newest output is in view: the xterm viewport sits at the
  // buffer end and, for an observer panning a larger grid, the host does too.
  let atBottom = $state(true);
  let pasteResult = false;
  // xterm reports `terminal.paste()` payload through onData, where it must stay
  // literal even while a soft modifier is armed.
  let pasting = false;
  let keyboardReopenAttempted = false;
  function anchorBottom(parent: HTMLElement) {
    terminal?.scrollToBottom();
    parent.scrollTop = parent.scrollHeight;
    // Anchoring is an explicit request to follow the tail, not a user scroll.
    follow = true;
  }
  function safeFit() {
    const parent = getContainer();
    if (isVisible() && parent?.clientWidth > 0 && parent?.clientHeight > 0) {
      const held =
        isHeightHeld() &&
        sized &&
        parent.offsetWidth === fitWidth &&
        terminal?.options.fontSize === fitFont;
      ({ sized, fitWidth, fitFont } = fitTerminalGrid(
        parent,
        terminal,
        renderer,
        stream.control,
        stream.observing,
        isSizeLocked(),
        held,
        sized,
        fitWidth,
        fitFont,
      ));
      if ((scrollOnFit || (gridFixed() && follow)) && terminal) {
        anchorBottom(parent);
        scrollOnFit = false;
      }
      updateBottom();
    } else {
      sized = false;
    }
  }
  function updateBottom(userScrolled = false) {
    const parent = getContainer();
    if (!terminal || !parent) return;
    const buffer = terminal.buffer.active;
    // Observers and locked grids scroll their host past the xterm viewport.
    const scrollsHost = parent.classList.contains("observing") || gridFixed();
    atBottom =
      buffer.viewportY >= buffer.baseY &&
      (!scrollsHost ||
        parent.scrollTop + parent.clientHeight >= parent.scrollHeight - 1);
    // Output and host resizes can change the geometry without moving the
    // scroll position; only an actual user scroll changes the intent.
    if (userScrolled) follow = atBottom;
  }
  function scrollToBottom() {
    scrollOnFit = true;
    follow = true;
    safeFit();
  }
  // One-shot modifiers clear after use; a locked Shift is a mode that stays
  // on until it is tapped again.
  function releaseModifiers() {
    modifiers = { ctrl: false, alt: false, shift: shiftLocked };
  }
  function clearModifiers() {
    shiftLocked = false;
    modifiers = { ctrl: false, alt: false, shift: false };
  }
  function send(input: string) {
    const value = pasting ? input : modifiedInput(input, modifiers);
    releaseModifiers();
    pasteResult = stream.sendInput(value);
    return pasteResult;
  }

  $effect(() => {
    const value = fontSize;
    untrack(() => {
      if (!terminal) return;
      terminal.options.fontSize = value;
      // A locked grid keeps its PTY rows and columns; the font only rescales
      // the pixels, so re-anchor instead of fitting back to the host.
      const lockedGrid = isSizeLocked() && sized;
      safeFit();
      if (lockedGrid && follow) {
        requestAnimationFrame(() => {
          if (terminal && isSizeLocked() && follow) {
            anchorBottom(getContainer());
            updateBottom();
          }
        });
      }
    });
  });

  onMount(() => {
    const parent = getContainer();
    let disposed = false;
    let release = () => {};
    void loadTerminalFont(fontSize).then(() => {
      if (!disposed) release = mountTerminal();
    });

    function mountTerminal() {
      const view = createTerminalRenderer(parent, fontSize, {
        layout: safeFit,
        // Called only by the theme observer, once `attachment` exists.
        themeChanged: () => attachment.themeChanged(),
      });
      const term = view.term;
      renderer = view;
      terminal = term;
      search = new SearchAddon({ highlightLimit: terminalFindLimit });
      term.loadAddon(search);
      canFind = true;
      scrollback = term.options.scrollback ?? 10000;
      bufferType = term.buffer.active.type;
      const resultsChanged = search.onDidChangeResults((results) => {
        if (findActive) {
          // The addon clears then reselects on its delayed output refresh.
          // xterm omits the second selection event for unchanged coordinates,
          // so read the completed search's selection rather than its clear.
          selection = term.getSelection();
          findResults = results;
        }
      });
      const bufferChanged = term.buffer.onBufferChange((buffer) => {
        clearFind();
        bufferType = buffer.type;
        // Replayed TUI entry/exit must not erase the full-replay notice.
        findReplayed = stream.restoring;
      });
      const attachment = stream.attach(term, {
        fit: safeFit,
        scrollToBottom,
        isVisible,
        copy: (text) => clipboard.copySelection(text),
        resetFind: () => {
          clearFind();
          findReplayed = true;
        },
        preserveViewport: () => findActive,
      });
      const detachInput = attachTerminalInput(term, parent, {
        userInput,
        clipboard,
        onData: (data, user) => {
          if (user) send(data);
          else stream.sendReply(data);
        },
      });
      const detachViewport = bindTerminalViewport(
        term,
        parent,
        () => (selection = term.getSelection()),
        () => updateBottom(true),
        () => updateBottom(),
        () => {
          pasting = true;
          queueMicrotask(() => (pasting = false));
        },
      );
      const detachTouch = attachTouchGestures(term, parent, {
        hasSelection: () => Boolean(selection),
        keyboardDismissed: isKeyboardDismissed,
        hostScrolled: () => updateBottom(true),
      });
      safeFit();
      if (!hasTouchInput() && isVisible() && isActive()) term.focus();
      return () => {
        disposed = true;
        // Observers go first; the terminal itself is disposed two frames
        // later, after the socket below has closed.
        view.dispose();
        detachTouch();
        detachInput();
        detachViewport();
        resultsChanged.dispose();
        bufferChanged.dispose();
        clearFind();
        search?.dispose();
        search = undefined;
        canFind = false;
        attachment.dispose();
        terminal = undefined;
        renderer = undefined;
        parent.classList.remove("observing");
        atBottom = true;
        follow = true;
        scrollOnFit = false;
      };
    }

    return () => {
      disposed = true;
      release();
    };
  });

  return {
    get ready() {
      return !!terminal && stream.accepting;
    },
    get info() {
      return stream.info;
    },
    get connection() {
      return stream.connection;
    },
    get restoring() {
      return stream.restoring;
    },
    get drawn() {
      return stream.drawn;
    },
    get exitCode() {
      return stream.exitCode;
    },
    get fontSize() {
      return fontSize;
    },
    get modifiers() {
      return modifiers;
    },
    get shiftLocked() {
      return shiftLocked;
    },
    get error() {
      return stream.error;
    },
    get selection() {
      return selection;
    },
    get atBottom() {
      return atBottom;
    },
    get historyTruncated() {
      return stream.historyTruncated;
    },
    get control() {
      return stream.control;
    },
    get controlling() {
      return stream.controlling;
    },
    get clientId() {
      return stream.clientId;
    },
    get canFind() {
      return canFind;
    },
    get findResults() {
      return findResults;
    },
    get findReplayed() {
      return findReplayed;
    },
    get bufferType() {
      return bufferType;
    },
    get scrollback() {
      return scrollback;
    },
    findNext(query, options, incremental = false) {
      find(query, options, false, incremental);
    },
    findPrevious(query, options) {
      find(query, options, true);
    },
    clearFind,
    claim: stream.claim,
    submit(submissionId, text, enter, report) {
      if (!stream.submittable) return false;
      releaseModifiers();
      return stream.submit(submissionId, text, enter, report);
    },
    track: stream.track,
    untrack: stream.untrack,
    copySelection() {
      if (selection) clipboard.copySelection(selection, true);
    },
    clearSelection() {
      terminal?.clearSelection();
    },
    send,
    setFontSize,
    resetModifiers: clearModifiers,
    toggleModifier(key: keyof Modifiers) {
      if (key === "shift" && shiftLocked) {
        shiftLocked = false;
        modifiers = { ...modifiers, shift: false };
        return;
      }
      modifiers = { ...modifiers, [key]: !modifiers[key] };
    },
    lockModifier(key: "shift") {
      shiftLocked = true;
      modifiers = { ...modifiers, [key]: true };
    },
    paste(text: string) {
      if (!terminal || !stream.accepting) return false;
      pasteResult = false;
      releaseModifiers();
      pasting = true;
      try {
        terminal.paste(text);
      } finally {
        pasting = false;
      }
      return pasteResult;
    },
    keyboard(beforeOpen) {
      keyboardReopenAttempted = toggleTerminalKeyboard(
        terminal,
        isKeyboardDismissed,
        keyboardReopenAttempted,
        stream.claim,
        beforeOpen,
      );
    },
    clear() {
      clearFind();
      terminal?.clear();
    },
    resync: stream.resync,
    reconnect: stream.reconnect,
    fit: safeFit,
    scrollToBottom,
    focus() {
      terminal?.focus();
    },
  };
}
