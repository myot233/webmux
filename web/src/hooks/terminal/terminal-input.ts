import type { ControlState } from "../../contracts/schemas";
import type { TerminalRenderer } from "./terminal-renderer";
import type { IDisposable, Terminal } from "@xterm/xterm";
import { isIOS } from "../../platform/touch-input";

// Draft-mode routing supplied by the page: while `draftEnabled()` holds, user
// keys, text and IME commits go to the draft instead of the PTY.
export interface TerminalUserInput {
  draftEnabled: () => boolean;
  focusDraft: () => void;
  key: (event: KeyboardEvent) => void;
  insertText: (text: string) => void;
}

export interface TerminalClipboard {
  paste: () => void;
  // `userGesture` lets the page use the legacy execCommand copy fallback that
  // plain-HTTP origins need; an OSC 52 request has no gesture.
  copySelection: (text: string, userGesture?: boolean) => void;
}

export interface TerminalInputOptions {
  userInput?: TerminalUserInput;
  clipboard: TerminalClipboard;
  // Bytes xterm emits; `user` is false for its automatic device replies.
  onData: (data: string, user: boolean) => void;
}

// Windows Terminal copies a selection on Ctrl+C and leaves an empty
// selection to xterm as SIGINT. macOS keeps the native Cmd+C convention and
// always sends Ctrl+C to the program.
export function isTerminalCopyShortcut(
  event: Pick<
    KeyboardEvent,
    | "key"
    | "ctrlKey"
    | "shiftKey"
    | "altKey"
    | "metaKey"
    | "isComposing"
    | "keyCode"
    | "code"
  >,
  platform: string,
): boolean {
  if (
    event.isComposing ||
    event.altKey ||
    event.shiftKey ||
    event.keyCode === 229 ||
    (event.key.toLowerCase() !== "c" && event.code !== "KeyC")
  )
    return false;
  return /Mac/.test(platform)
    ? event.metaKey && !event.ctrlKey
    : event.ctrlKey && !event.metaKey;
}

// Keyboard, IME, clipboard-shortcut and draft routing for xterm in `parent`.
// Returns the cleanup that removes every listener it added.
export function attachTerminalInput(
  term: Terminal,
  parent: HTMLElement,
  { userInput, clipboard, onData }: TerminalInputOptions,
) {
  term.textarea?.setAttribute("autocapitalize", "off");
  term.textarea?.setAttribute("autocorrect", "off");
  term.textarea?.setAttribute("spellcheck", "false");
  if (isIOS()) parent.classList.add("ios-ime");
  // Divert user events, never onData: device replies and explicit paste
  // must retain their normal PTY path while a draft is being edited.
  const focusDraft = () => {
    if (userInput?.draftEnabled()) userInput.focusDraft();
  };
  const draftPaste = (event: ClipboardEvent) => {
    if (!userInput?.draftEnabled() || event.clipboardData?.files.length) return;
    event.preventDefault();
    event.stopImmediatePropagation();
    userInput.insertText(event.clipboardData?.getData("text/plain") ?? "");
    userInput.focusDraft();
  };
  const draftInput = (event: InputEvent) => {
    if (!userInput?.draftEnabled()) return;
    // If beforeinput cannot be cancelled, consume its subsequent input
    // instead. Composition is committed only at compositionend.
    if (event.type === "beforeinput" && !event.cancelable) return;
    event.preventDefault();
    event.stopImmediatePropagation();
    userInput.focusDraft();
    if (
      event.data &&
      !event.isComposing &&
      event.inputType !== "insertCompositionText" &&
      event.inputType !== "insertFromComposition"
    )
      userInput.insertText(event.data);
  };
  const draftComposition = (event: CompositionEvent) => {
    if (!userInput?.draftEnabled()) return;
    event.stopImmediatePropagation();
    userInput.focusDraft();
    if (event.type === "compositionend" && event.data)
      userInput.insertText(event.data);
  };
  parent.addEventListener("focusin", focusDraft, true);
  parent.addEventListener("paste", draftPaste, true);
  // beforeinput covers browsers dispatching insertion to the previously
  // focused xterm textarea after focus has already moved.
  parent.addEventListener("beforeinput", draftInput as EventListener, true);
  parent.addEventListener("input", draftInput as EventListener, true);
  parent.addEventListener("compositionstart", draftComposition, true);
  parent.addEventListener("compositionupdate", draftComposition, true);
  parent.addEventListener("compositionend", draftComposition, true);
  // xterm accepts a composed insertText only while no keydown is
  // pending, otherwise relying on a textarea diff scheduled by a keyCode
  // 229 keydown. iOS CJK keyboards commit space, digits and punctuation as
  // insertText outside that window (keyups of IME-handled keys go missing),
  // so xterm drops them. Cancel the edit so that diff sees no change and
  // forward the text once. xterm already sent keys it cancelled at keydown
  // and printable keys it handled at keypress, which it leaves uncancelled;
  // compositions remain xterm's, including Safari's trailing insertText.
  let keypressed = false;
  let composing = false;
  let compositionTimer: number | undefined;
  const trackKey = (event: KeyboardEvent) => {
    keypressed = event.type === "keypress";
  };
  const trackComposition = (event: CompositionEvent) => {
    clearTimeout(compositionTimer);
    composing = true;
    if (event.type === "compositionend")
      compositionTimer = window.setTimeout(() => (composing = false));
  };
  const resetComposition = () => {
    clearTimeout(compositionTimer);
    keypressed = false;
    const textarea = term.textarea;
    if (!composing) return;
    if (textarea) {
      textarea.value = "";
      textarea.dispatchEvent(
        new CompositionEvent("compositionend", {
          data: "",
          bubbles: true,
          composed: true,
        }),
      );
    }
    clearTimeout(compositionTimer);
    composing = false;
  };
  const visible = () => {
    if (document.hidden) resetComposition();
  };
  const directInput = (event: InputEvent) => {
    const lineBreak =
      event.inputType === "insertLineBreak" ||
      event.inputType === "insertParagraph";
    const text = lineBreak
      ? "\r"
      : event.inputType === "insertText" ||
          event.inputType === "insertFromComposition"
        ? event.data
        : null;
    if (
      userInput?.draftEnabled() ||
      event.target !== term.textarea ||
      !text ||
      event.isComposing ||
      composing ||
      keypressed ||
      !event.cancelable
    ) {
      if (event.target === term.textarea) keypressed = false;
      return;
    }
    event.preventDefault();
    event.stopImmediatePropagation();
    term.input(text, true);
  };
  const clearKeypressed = (event: InputEvent) => {
    if (event.target === term.textarea) keypressed = false;
  };
  // A keyboard extension (Typeless, other dictation keyboards) commits text
  // with one insertText call, which WebKit reports as one key: keydown and
  // keypress carry the whole string as `key`, the keypress `charCode` names
  // only its first code point, then insertText follows. xterm sends that
  // code point and ignores the insertion as already handled. Send the string
  // once and cancel the keypress so the insertion does not happen; key names
  // never match (Enter's charCode is 13, not "E").
  const wholeStringKey = (event: KeyboardEvent) => {
    const { key, charCode } = event;
    if (
      userInput?.draftEnabled() ||
      event.target !== term.textarea ||
      event.isComposing ||
      composing ||
      event.ctrlKey ||
      event.metaKey ||
      event.altKey ||
      !charCode ||
      key.codePointAt(0) !== charCode ||
      key.length <= String.fromCodePoint(charCode).length
    )
      return;
    keypressed = false;
    event.preventDefault();
    event.stopImmediatePropagation();
    term.input(key, true);
  };
  for (const type of ["keydown", "keypress", "keyup"] as const)
    parent.addEventListener(type, trackKey, true);
  parent.addEventListener("keypress", wholeStringKey, true);
  parent.addEventListener("beforeinput", directInput as EventListener, true);
  parent.addEventListener("input", clearKeypressed as EventListener, true);
  parent.addEventListener("compositionstart", trackComposition, true);
  parent.addEventListener("compositionend", trackComposition, true);
  term.textarea?.addEventListener("blur", resetComposition);
  document.addEventListener("visibilitychange", visible);

  // xterm emits keypresses and its own answers to device queries through
  // the same event, sometimes as identical bytes (Shift+F3 and a cursor
  // report are both "ESC [ 1 ; 2 R"). Its core fires onUserInput right
  // before onData only for the former; the server needs that provenance
  // to write one answer per query and to let only real input take
  // control. Without the hook, everything is sent as user input.
  let typed = false;
  const core = (
    term as unknown as {
      _core?: {
        coreService?: {
          onUserInput?: (listener: () => void) => IDisposable;
        };
      };
    }
  )._core;
  const provenance = core?.coreService?.onUserInput?.(() => {
    typed = true;
  });
  const input = term.onData((data) => {
    const user = typed || !provenance;
    typed = false;
    onData(data, user);
  });

  term.attachCustomKeyEventHandler((event) => {
    if (userInput?.draftEnabled()) {
      if (
        event.type === "keydown" &&
        (event.ctrlKey || event.metaKey) &&
        event.key.toLowerCase() === "v"
      ) {
        if (event.shiftKey) {
          event.preventDefault();
          clipboard.paste();
        } else userInput.focusDraft();
      } else {
        event.preventDefault();
        userInput.key(event);
      }
      return false;
    }
    if (
      event.type === "keydown" &&
      event.altKey &&
      !event.ctrlKey &&
      !event.metaKey &&
      !event.shiftKey &&
      !event.isComposing &&
      event.keyCode !== 229
    ) {
      // xterm 6 changed Alt-arrow sequences. Keep shell word navigation
      // and classify these bytes as user input, just like native keys.
      const sequences: Record<string, string> = /Mac/.test(navigator.platform)
        ? { ArrowLeft: "\x1bb", ArrowRight: "\x1bf" }
        : {
            ArrowLeft: "\x1b[1;5D",
            ArrowRight: "\x1b[1;5C",
            ArrowUp: "\x1b[1;5A",
            ArrowDown: "\x1b[1;5B",
          };
      const sequence = sequences[event.key];
      if (sequence) {
        event.preventDefault();
        term.input(sequence, true);
        return false;
      }
    }
    if (
      event.type === "keydown" &&
      !event.altKey &&
      (event.ctrlKey || event.metaKey) &&
      event.key.toLowerCase() === "v"
    ) {
      // Native Ctrl/Cmd+V works without Clipboard API permissions, including
      // HTTP deployments. Shift+V retains the explicit clipboard action.
      if (event.shiftKey) {
        event.preventDefault();
        clipboard.paste();
      }
      return false;
    }
    if (
      event.type === "keydown" &&
      event.ctrlKey &&
      event.shiftKey &&
      event.key.toLowerCase() === "c"
    ) {
      // Ctrl+Shift+C belongs to the browser (DevTools inspect); never forward
      // it to the PTY as ^C or treat it as a copy shortcut.
      return false;
    }
    if (
      event.type === "keydown" &&
      isTerminalCopyShortcut(event, navigator.platform)
    ) {
      const selected = term.getSelection();
      if (selected) {
        event.preventDefault();
        clipboard.copySelection(selected, true);
        term.clearSelection();
        return false;
      }
    }
    return true;
  });

  return () => {
    parent.removeEventListener("focusin", focusDraft, true);
    parent.removeEventListener("paste", draftPaste, true);
    parent.removeEventListener(
      "beforeinput",
      draftInput as EventListener,
      true,
    );
    parent.removeEventListener("input", draftInput as EventListener, true);
    parent.removeEventListener("compositionstart", draftComposition, true);
    parent.removeEventListener("compositionupdate", draftComposition, true);
    parent.removeEventListener("compositionend", draftComposition, true);
    parent.removeEventListener(
      "beforeinput",
      directInput as EventListener,
      true,
    );
    parent.removeEventListener("input", clearKeypressed as EventListener, true);
    for (const type of ["keydown", "keypress", "keyup"] as const)
      parent.removeEventListener(type, trackKey, true);
    parent.removeEventListener("keypress", wholeStringKey, true);
    parent.removeEventListener("compositionstart", trackComposition, true);
    parent.removeEventListener("compositionend", trackComposition, true);
    term.textarea?.removeEventListener("blur", resetComposition);
    document.removeEventListener("visibilitychange", visible);
    clearTimeout(compositionTimer);
    parent.classList.remove("ios-ime");
    input.dispose();
    provenance?.dispose();
  };
}

export function toggleTerminalKeyboard(
  terminal: Terminal | undefined,
  isKeyboardDismissed: () => boolean,
  reopenAttempted: boolean,
  claim: () => void,
  beforeOpen?: (target: HTMLElement) => void,
): boolean {
  const textarea = terminal?.textarea;
  if (!terminal || !textarea) return reopenAttempted;
  const focused = document.activeElement === textarea;
  if (focused && isKeyboardDismissed() && !reopenAttempted) {
    // Android can hide the IME with Back while the helper keeps focus;
    // focus() alone is then a no-op, so restart focus once to reopen it.
    // If the browser still declines, the next press toggles normally.
    claim();
    textarea.blur();
    beforeOpen?.(textarea);
    terminal.focus();
    return true;
  }
  if (focused) {
    terminal.blur();
    return false;
  }
  claim();
  beforeOpen?.(textarea);
  terminal.focus();
  return false;
}

export function fitTerminalGrid(
  parent: HTMLElement,
  terminal: Terminal | undefined,
  renderer: TerminalRenderer | undefined,
  control: ControlState | undefined,
  observing: boolean,
  locked: boolean,
  held: boolean,
  sized: boolean,
  fitWidth: number,
  fitFont: number,
) {
  parent.classList.toggle("observing", observing);
  if (control && observing) {
    if (terminal?.cols !== control.cols || terminal?.rows !== control.rows) {
      terminal?.resize(control.cols, control.rows);
      parent.scrollTop = parent.scrollHeight;
    }
    return { sized: false, fitWidth, fitFont };
  }
  if (!sized || (!locked && !held)) {
    renderer?.fit();
    if (renderer) {
      return {
        sized: true,
        fitWidth: parent.offsetWidth,
        fitFont: terminal?.options.fontSize ?? 0,
      };
    }
  }
  return { sized, fitWidth, fitFont };
}

export function bindTerminalViewport(
  term: Terminal,
  parent: HTMLElement,
  onSelect: () => void,
  onUserScroll: () => void,
  onWriteParsed: () => void,
  onNativePaste: () => void,
) {
  parent.addEventListener("paste", onNativePaste, true);
  const selected = term.onSelectionChange(onSelect);
  const scrolled = term.onScroll(onUserScroll);
  const parsed = term.onWriteParsed(onWriteParsed);
  parent.addEventListener("scroll", onUserScroll, { passive: true });
  return () => {
    scrolled.dispose();
    parsed.dispose();
    parent.removeEventListener("scroll", onUserScroll);
    parent.removeEventListener("paste", onNativePaste, true);
    selected.dispose();
  };
}
