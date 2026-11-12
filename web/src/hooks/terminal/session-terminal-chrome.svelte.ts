import { tick, untrack } from "svelte";
import { store, stored } from "../../platform/api";
import type { ChatCodeLink } from "../../domains/code/markdown-link";
import type { CodeReveal, SessionInfo } from "../../contracts/schemas";
import {
  captureFindReturnFocus,
  clearFindFocus,
  copySessionBuffer,
  copySessionText,
  isEditableActiveElement,
  pasteFromSessionClipboard,
  type SessionClipboardContext,
  type SessionHybridView,
} from "../../domains/terminal/session-terminal-io";
import type {
  DraftOperation,
  TerminalDraft,
  TerminalDrafts,
  TextInsertion,
} from "../../domains/terminal/terminal-draft.svelte";
import { hasTouchInput } from "../../platform/touch-input";
import { codePanelState } from "../code/code-panel-state";
import { createKeyboardSizeHold } from "./keyboard-size-hold";
import { keyboardSizeHold } from "./keyboard-size-hold-preference.svelte";
import type { KeyboardVisibility } from "../shared/viewport.svelte";

export function useSessionTerminalChrome(options: {
  id: () => string;
  draft: TerminalDraft;
  drafts: TerminalDrafts;
  isMounted: () => boolean;
  isActiveAndVisible: () => boolean;
  isTerminalView: () => boolean;
  isTerminalVisible: () => boolean;
  isDesktop: () => boolean;
  focusFindBar: () => void;
  focusComposer: () => void;
  clearTerminalFind: () => void;
  focusFallback: () => void;
  fitTerminal: () => void;
  viewportHeight: () => number;
  keyboardVisibility: () => KeyboardVisibility;
  keyboardDismissed: () => boolean;
  releaseChatEditing: () => void;
  isCodePanelPoppedOut: () => boolean;
  insertText: (text: string, operation?: DraftOperation) => TextInsertion;
  uploadFiles: (
    files: FileList | File[],
    operation?: DraftOperation,
  ) => Promise<void>;
  setError: (message: string) => void;
  oninfo: (info: SessionInfo, connection: string) => void;
  onscrollstate?: (state: {
    chatView: boolean;
    desktop: boolean;
    codeShown: boolean;
    editing: boolean;
  }) => void;
}) {
  let findOpen = $state(false);
  let findReturnFocus: HTMLElement | null = null;
  let sizeLocked = $state(stored("sizeLocked") === "1");
  let keyboardHeld = $state(false);
  let pasteOperation = $state<DraftOperation | undefined>();
  let pasteOpen = $state(false);
  let pasteText = $state("");
  let copyOpen = $state(false);
  let copyText = $state("");
  let pendingCopy = $state<string | null>(null);
  let codeShown = $state(codePanelState.shown());
  $effect(() => codePanelState.setShown(codeShown));
  let codeTarget = $state<CodeReveal>();
  let chatEditing = $state(false);
  let choiceClosed = $state(false);

  const hold = createKeyboardSizeHold({
    canHold: () => keyboardSizeHold.enabled && hasTouchInput(),
    keyboardStays: isEditableActiveElement,
    onActive: (value) => (keyboardHeld = value),
    release: () => void tick().then(() => options.fitTerminal()),
  });

  $effect(() => {
    options.viewportHeight();
    hold.update(options.keyboardVisibility());
  });
  $effect(() => {
    if (!keyboardSizeHold.enabled) hold.cancel();
  });
  $effect(() => () => hold.dispose());

  function showFind() {
    if (!options.isTerminalView()) return;
    if (!findOpen) findReturnFocus = captureFindReturnFocus();
    if (!options.isDesktop()) codeShown = false;
    findOpen = true;
    void tick().then(() => {
      if (options.isActiveAndVisible()) options.focusFindBar();
    });
  }

  function closeFind() {
    findOpen = false;
    clearFindFocus(
      options.clearTerminalFind,
      hasTouchInput(),
      options.isActiveAndVisible,
      findReturnFocus,
      options.focusFallback,
      (fn) => void tick().then(fn),
    );
  }

  function setSizeLocked(value: boolean) {
    sizeLocked = value;
    store("sizeLocked", value ? "1" : "0");
    void tick().then(() => options.fitTerminal());
  }

  function review(path: string) {
    if (!options.isCodePanelPoppedOut()) codeShown = true;
    codeTarget = { path, mode: "diff", key: (codeTarget?.key ?? 0) + 1 };
  }

  function openCodeLocation(target: ChatCodeLink) {
    if (!options.isCodePanelPoppedOut()) codeShown = true;
    codeTarget = { ...target, mode: "file", key: (codeTarget?.key ?? 0) + 1 };
  }

  const clipboardCtx: SessionClipboardContext = {
    get id() {
      return options.id();
    },
    isMounted: options.isMounted,
    isActiveAndVisible: options.isActiveAndVisible,
    isTerminalVisible: options.isTerminalVisible,
    draft: options.draft,
    drafts: options.drafts,
    insertText: options.insertText,
    uploadFiles: options.uploadFiles,
    focusComposer: options.focusComposer,
    setPendingCopy: (text) => (pendingCopy = text),
    openCopyDialog: (text) => {
      copyText = text;
      copyOpen = true;
    },
    openPasteDialog: (operation) => {
      pasteOperation = operation;
      pasteText = "";
      pasteOpen = true;
    },
    setError: options.setError,
  };

  function copyToClipboard(text: string, userGesture = false) {
    return copySessionText(text, userGesture, clipboardCtx);
  }

  function copyPending() {
    const text = pendingCopy;
    pendingCopy = null;
    if (text !== null) void copyToClipboard(text, true);
  }

  function syncSessionEffects(
    hybrid: SessionHybridView & {
      info?: SessionInfo | null;
      missing: boolean;
      awaitingChoice: boolean;
    },
    terminalConnection: () => string,
    terminalGeneration: () => string | undefined,
  ) {
    $effect(() => {
      const current = hybrid.info;
      const connection =
        hybrid.view === "chat" ? hybrid.chat.connection : terminalConnection();
      if (current) untrack(() => options.oninfo(current, connection));
    });
    $effect(() => {
      if (hybrid.view !== "chat") chatEditing = false;
    });
    $effect(() => {
      if (options.keyboardDismissed() && chatEditing) {
        chatEditing = false;
        options.releaseChatEditing();
      }
    });
    $effect(() => {
      const state = {
        chatView: hybrid.view === "chat",
        desktop: options.isDesktop(),
        codeShown,
        editing: chatEditing || options.keyboardVisibility() === "visible",
      };
      untrack(() => options.onscrollstate?.(state));
    });
    $effect(() => {
      if (hybrid.missing) options.drafts.remove(options.id());
    });
    $effect(() => {
      if (!hybrid.awaitingChoice) choiceClosed = false;
    });
    $effect(() => {
      const generation = terminalGeneration();
      if (generation) options.drafts.bindGeneration(options.draft, generation);
    });
  }

  return {
    get findOpen() {
      return findOpen;
    },
    get sizeLocked() {
      return sizeLocked;
    },
    get keyboardHeld() {
      return keyboardHeld;
    },
    get pasteOperation() {
      return pasteOperation;
    },
    get pasteOpen() {
      return pasteOpen;
    },
    set pasteOpen(value: boolean) {
      pasteOpen = value;
    },
    get pasteText() {
      return pasteText;
    },
    set pasteText(value: string) {
      pasteText = value;
    },
    get copyOpen() {
      return copyOpen;
    },
    set copyOpen(value: boolean) {
      copyOpen = value;
    },
    get copyText() {
      return copyText;
    },
    get pendingCopy() {
      return pendingCopy;
    },
    set pendingCopy(value: string | null) {
      pendingCopy = value;
    },
    get codeShown() {
      return codeShown;
    },
    set codeShown(value: boolean) {
      codeShown = value;
    },
    get codeTarget() {
      return codeTarget;
    },
    get chatEditing() {
      return chatEditing;
    },
    set chatEditing(value: boolean) {
      chatEditing = value;
    },
    get choiceClosed() {
      return choiceClosed;
    },
    set choiceClosed(value: boolean) {
      choiceClosed = value;
    },
    beginHold: (target: EventTarget) => hold.begin(target),
    showFind,
    closeFind,
    setSizeLocked,
    review,
    openCodeLocation,
    copyToClipboard,
    copyPending,
    copy: () => copySessionBuffer(clipboardCtx),
    paste: () => pasteFromSessionClipboard(clipboardCtx),
    syncSessionEffects,
  };
}
