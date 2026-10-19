import { onMount } from "svelte";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { WebglAddon } from "@xterm/addon-webgl";
import { apiBuffer, stored, store } from "../api";
import {
  ExitSchema,
  FontSizeSchema,
  SessionInfoSchema,
  type SessionInfo,
} from "../schemas";
import { connect, type Connection, type SocketConnection } from "../ws";
import { modifiedInput, type Modifiers } from "../input";
import {
  loadTerminalFont,
  terminalFontFamilyAlt,
  terminalFontFamilyCss,
  terminalLineHeight,
} from "../fonts";
import { osc52CopyText } from "../terminalClipboard";
import { createOutputBudget } from "../terminal-output";
import { attachTouchGestures } from "./touch";
import { hasTouchInput, isIOS } from "../touchInput";
import "@xterm/xterm/css/xterm.css";
import "../fonts-terminal.css";

// Read the same theme tokens used by the shadcn surfaces, including live theme changes.
function terminalTheme() {
  const css = getComputedStyle(document.documentElement);
  return {
    background: css.getPropertyValue("--terminal-background").trim(),
    foreground: css.getPropertyValue("--terminal-foreground").trim(),
    cursor: css.getPropertyValue("--terminal-blue").trim(),
    selectionBackground: css.getPropertyValue("--terminal-selection").trim(),
    black: css.getPropertyValue("--terminal-black").trim(),
    red: css.getPropertyValue("--terminal-red").trim(),
    green: css.getPropertyValue("--terminal-green").trim(),
    yellow: css.getPropertyValue("--terminal-yellow").trim(),
    blue: css.getPropertyValue("--terminal-blue").trim(),
    magenta: css.getPropertyValue("--terminal-magenta").trim(),
    cyan: css.getPropertyValue("--terminal-cyan").trim(),
    white: css.getPropertyValue("--terminal-white").trim(),
  };
}

export function useTerminal(
  getContainer: () => HTMLDivElement,
  id: string,
  clipboard: { paste: () => void; copySelection: (text: string) => void },
  isVisible: () => boolean = () => true,
  userInput?: {
    draftEnabled: () => boolean;
    focusDraft: () => void;
    key: (event: KeyboardEvent) => void;
    insertText: (text: string) => void;
  },
) {
  let terminal: Terminal | undefined;
  let socket: SocketConnection | undefined;
  let fit: FitAddon | undefined;
  let restoreBuffer: ((payload: Uint8Array) => void) | undefined;
  let scrollOnFit = false;
  let info = $state<SessionInfo>();
  let connection = $state<Connection>("connecting");
  let restoring = $state(true);
  let exitCode = $state<number | null>(null);
  const defaultFont = matchMedia("(pointer: coarse)").matches ? 13 : 14;
  let fontSize = $state(
    FontSizeSchema.catch(defaultFont).parse(
      stored("fontSize", String(defaultFont)),
    ),
  );
  let modifiers = $state<Modifiers>({ ctrl: false, alt: false });
  let error = $state("");
  let selection = $state("");
  const encoder = new TextEncoder();
  const decoder = new TextDecoder();

  let pasteResult = false;
  function safeFit() {
    const parent = getContainer();
    if (isVisible() && parent?.clientWidth > 0 && parent?.clientHeight > 0) {
      fit?.fit();
      if (scrollOnFit && terminal) {
        terminal.scrollToBottom();
        scrollOnFit = false;
      }
    }
  }
  function scrollToBottom() {
    scrollOnFit = true;
    safeFit();
  }
  function send(input: string) {
    const value = modifiedInput(input, modifiers);
    modifiers = { ctrl: false, alt: false };
    pasteResult = socket?.send(0, encoder.encode(value)) ?? false;
    return pasteResult;
  }

  function setFontSize(value: number) {
    fontSize = Math.max(12, Math.min(20, Math.round(value)));
    if (terminal) {
      terminal.options.fontSize = fontSize;
      safeFit();
    }
    store("fontSize", String(fontSize));
  }

  onMount(() => {
    const parent = getContainer();
    let disposed = false;
    let release = () => {};
    void loadTerminalFont(fontSize).then(() => {
      if (!disposed) release = mountTerminal();
    });

    function mountTerminal() {
      const term = new Terminal({
        fontSize,
        fontFamily: terminalFontFamilyCss,
        fontWeight: 400,
        fontWeightBold: 700,
        lineHeight: terminalLineHeight,
        scrollback: 10000,
        cursorBlink: true,
        minimumContrastRatio: 4.5,
        allowProposedApi: true,
        macOptionIsMeta: true,
        theme: terminalTheme(),
      });
      terminal = term;
      const fitAddon = new FitAddon();
      fit = fitAddon;
      term.loadAddon(fitAddon);
      term.open(parent);
      let webgl: WebglAddon | undefined;
      const useDomRenderer = () => {
        if (!webgl) return;
        const addon = webgl;
        webgl = undefined;
        addon.dispose();
        term.refresh(0, term.rows - 1);
      };
      try {
        webgl = new WebglAddon();
        term.loadAddon(webgl);
        webgl.onContextLoss(useDomRenderer);
      } catch {
        useDomRenderer();
        // xterm's built-in DOM renderer remains available without WebGL.
      }
      const theme = () => {
        if (disposed) return;
        term.options.theme = terminalTheme();
      };
      const colors = new MutationObserver(theme);
      colors.observe(document.documentElement, {
        attributes: true,
        attributeFilter: ["class"],
      });
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
        if (!userInput?.draftEnabled() || event.clipboardData?.files.length)
          return;
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
      // xterm 5.5 accepts a composed insertText only while no keydown is
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
      const directInput = (event: InputEvent) => {
        if (
          userInput?.draftEnabled() ||
          event.target !== term.textarea ||
          event.inputType !== "insertText" ||
          !event.data ||
          event.isComposing ||
          composing ||
          keypressed ||
          !event.cancelable
        )
          return;
        event.preventDefault();
        event.stopImmediatePropagation();
        term.input(event.data, true);
      };
      for (const type of ["keydown", "keypress", "keyup"] as const)
        parent.addEventListener(type, trackKey, true);
      parent.addEventListener(
        "beforeinput",
        directInput as EventListener,
        true,
      );
      parent.addEventListener("compositionstart", trackComposition, true);
      parent.addEventListener("compositionend", trackComposition, true);
      let stale = false;
      let exited = false;
      let restoreFrame = 0;
      let restoreVersion = 0;
      // Snapshots replay output whose OSC 52 copies already ran when it was
      // live. xterm parses writes and runs their callbacks in order, so an
      // empty write's callback opens the replay window exactly where the
      // snapshot starts and the snapshot's callback closes it.
      let replaying = false;
      const output = createOutputBudget(
        (data, parsed) => term.write(data, parsed),
        () => {
          // A scheduled reconnect already brings a fresh snapshot.
          if (connection !== "connected") return;
          if (!ws.send(2, new Uint8Array())) ws.reconnect();
        },
      );
      const restore = (payload: Uint8Array) => {
        output.replay();
        const version = ++restoreVersion;
        cancelAnimationFrame(restoreFrame);
        restoring = true;
        term.write("", () => {
          if (disposed) return;
          term.reset();
          replaying = true;
        });
        term.write(payload, () => {
          replaying = false;
          if (disposed || version !== restoreVersion) return;
          restoreFrame = requestAnimationFrame(() => {
            if (disposed || version !== restoreVersion) return;
            scrollToBottom();
            restoring = output.recovering;
          });
        });
      };
      restoreBuffer = restore;
      const ws = connect(
        "/ws/s/" + id,
        (data) => {
          if (typeof data === "string") return;
          const frame = new Uint8Array(data);
          const payload = frame.subarray(1);
          if (frame[0] === 0 || frame[0] === 1) {
            if (document.hidden) stale = true;
            else if (frame[0] === 1) {
              stale = false;
              restore(payload);
            } else if (!output.write(payload)) {
              stale = true;
              restoring = true;
            }
          } else if (frame[0] === 2) {
            info = SessionInfoSchema.parse(JSON.parse(decoder.decode(payload)));
            document.title = info.title + " · WebMux";
          } else if (frame[0] === 3) {
            exited = true;
            exitCode = ExitSchema.parse(
              JSON.parse(decoder.decode(payload)),
            ).exitCode;
            if (stale && !document.hidden) restoreExited();
          }
        },
        (state) => {
          connection = state;
          if (state === "connected") {
            safeFit();
            if (isVisible() && term.cols > 0 && term.rows > 0)
              ws.send(
                1,
                encoder.encode(
                  JSON.stringify({ cols: term.cols, rows: term.rows }),
                ),
              );
          }
          if (state === "connecting" || state === "reconnecting")
            restoring = true;
          if (state === "missing") restoring = false;
        },
        (message) => {
          error = message;
        },
      );
      socket = ws;
      const input = term.onData(send);
      const selected = term.onSelectionChange(() => {
        selection = term.getSelection();
      });
      // OSC 52 is the application copy request handled by xterm's clipboard
      // addon. Selection copy (Ctrl+Shift+C and the selection toolbar) already
      // uses this same callback, which writes the browser clipboard.
      const oscClipboard = term.parser.registerOscHandler(52, (data) => {
        const text = osc52CopyText(data);
        if (text === undefined) return false;
        if (!replaying) clipboard.copySelection(text);
        return true;
      });
      const resize = term.onResize((size) => {
        if (isVisible() && size.cols > 0 && size.rows > 0)
          ws.send(1, encoder.encode(JSON.stringify(size)));
      });
      let timer: number | undefined;
      const observer = new ResizeObserver(([entry]) => {
        if (disposed) return;
        if (webgl) {
          const pixels = entry?.devicePixelContentBoxSize?.[0];
          const css = entry?.contentRect;
          const dpr = window.devicePixelRatio || 1;
          // Chromium emulation can report physical pixels at the host DPR while
          // xterm renders at the emulated DPR. The WebGL addon then shrinks its
          // backing canvas without updating its render dimensions. DOM rendering
          // avoids that incompatible measurement; allow normal pixel rounding.
          if (
            pixels &&
            css.width > 0 &&
            css.height > 0 &&
            pixels.inlineSize > 0 &&
            pixels.blockSize > 0 &&
            (Math.abs(pixels.inlineSize - css.width * dpr) > 1 ||
              Math.abs(pixels.blockSize - css.height * dpr) > 1)
          ) {
            useDomRenderer();
          }
        }
        clearTimeout(timer);
        timer = window.setTimeout(() => safeFit(), 100);
      });
      const observeSize = () => {
        try {
          observer.observe(parent, { box: "device-pixel-content-box" });
        } catch {
          observer.observe(parent);
        }
      };
      let resolution = matchMedia(
        `(resolution: ${window.devicePixelRatio || 1}dppx)`,
      );
      const resolutionChanged = () => {
        if (disposed) return;
        resolution.removeEventListener("change", resolutionChanged);
        resolution = matchMedia(
          `(resolution: ${window.devicePixelRatio || 1}dppx)`,
        );
        resolution.addEventListener("change", resolutionChanged);
        // A DPR change need not change the CSS box. Request a fresh observation
        // rather than comparing the new DPR against stale physical dimensions.
        observer.unobserve(parent);
        observeSize();
      };
      resolution.addEventListener("change", resolutionChanged);
      observeSize();
      function restoreExited() {
        void apiBuffer(id)
          .then((payload) => {
            if (!disposed) {
              stale = false;
              restore(payload);
            }
          })
          .catch((err) => {
            if (!disposed) error = (err as Error).message;
          });
      }
      const visible = () => {
        // Live sessions reattach in connect(); exited sessions cannot reopen that socket.
        if (!document.hidden && stale && exited) {
          restoreExited();
        }
      };
      document.addEventListener("visibilitychange", visible);

      const detachTouch = attachTouchGestures(term, parent, {
        fontSize: () => fontSize,
        setFontSize,
        hasSelection: () => Boolean(selection),
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
        if (event.type === "keydown" && event.ctrlKey && event.shiftKey) {
          if (event.key.toLowerCase() === "c") {
            clipboard.copySelection(term.getSelection());
            return false;
          }
        }
        return true;
      });
      let remeasureFrame = 0;
      const remeasure = () => {
        cancelAnimationFrame(remeasureFrame);
        remeasureFrame = requestAnimationFrame(() => {
          if (disposed) return;
          // Later unicode-range slices must be rasterized into the cell grid
          // that was measured from the Latin face.
          webgl?.clearTextureAtlas();
          const current = term.options.fontFamily;
          term.options.fontFamily =
            current === terminalFontFamilyCss
              ? terminalFontFamilyAlt
              : terminalFontFamilyCss;
          term.options.fontFamily = current ?? terminalFontFamilyCss;
          safeFit();
        });
      };
      document.fonts.addEventListener("loadingdone", remeasure);
      safeFit();
      if (!hasTouchInput()) term.focus();
      return () => {
        disposed = true;
        cancelAnimationFrame(remeasureFrame);
        cancelAnimationFrame(restoreFrame);
        document.fonts.removeEventListener("loadingdone", remeasure);
        clearTimeout(timer);
        observer.disconnect();
        resolution.removeEventListener("change", resolutionChanged);
        document.removeEventListener("visibilitychange", visible);
        detachTouch();
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
        for (const type of ["keydown", "keypress", "keyup"] as const)
          parent.removeEventListener(type, trackKey, true);
        parent.removeEventListener("compositionstart", trackComposition, true);
        parent.removeEventListener("compositionend", trackComposition, true);
        clearTimeout(compositionTimer);
        parent.classList.remove("ios-ime");
        input.dispose();
        resize.dispose();
        selected.dispose();
        oscClipboard.dispose();
        colors.disconnect();
        output.dispose();
        ws.close();
        const dying = term;
        // Viewport.reset() queues syncScrollArea for the next frame, and that
        // frame reads RenderService.dimensions. Disposing in this turn removes
        // the renderer first, so the callback throws after a restore or theme
        // change. Let that frame, and the scroll refresh it may schedule, run
        // before the terminal is disposed.
        requestAnimationFrame(() => {
          requestAnimationFrame(() => {
            dying.dispose();
          });
        });
        terminal = undefined;
        socket = undefined;
        fit = undefined;
        restoreBuffer = undefined;
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
      return (
        !!terminal &&
        connection === "connected" &&
        !restoring &&
        exitCode === null
      );
    },
    get info() {
      return info;
    },
    get connection() {
      return connection;
    },
    get restoring() {
      return restoring;
    },
    get exitCode() {
      return exitCode;
    },
    get fontSize() {
      return fontSize;
    },
    get modifiers() {
      return modifiers;
    },
    get error() {
      return error;
    },
    get selection() {
      return selection;
    },
    copySelection() {
      if (selection) clipboard.copySelection(selection);
    },
    clearSelection() {
      terminal?.clearSelection();
    },
    send,
    setFontSize,
    resetModifiers() {
      modifiers = { ctrl: false, alt: false };
    },
    toggleModifier(key: keyof Modifiers) {
      modifiers = { ...modifiers, [key]: !modifiers[key] };
    },
    paste(text: string) {
      if (
        !terminal ||
        connection !== "connected" ||
        restoring ||
        exitCode !== null
      )
        return false;
      pasteResult = false;
      modifiers = { ctrl: false, alt: false };
      terminal.paste(text);
      return pasteResult;
    },
    keyboard() {
      if (document.activeElement === terminal?.textarea) terminal?.blur();
      else terminal?.focus();
    },
    clear() {
      terminal?.clear();
    },
    resync() {
      if (exitCode !== null) {
        void apiBuffer(id)
          .then((payload) => restoreBuffer?.(payload))
          .catch((err) => {
            error = (err as Error).message;
          });
      } else socket?.send(2, new Uint8Array());
    },
    fit: safeFit,
    scrollToBottom,
    focus() {
      terminal?.focus();
    },
  };
}
