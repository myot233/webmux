<script lang="ts">
  import { errorMessage } from "../../platform/error";
  import { onMount, tick, untrack, type Snippet } from "svelte";
  import { navigate, uploadFile } from "../../platform/api";
  import { useTerminal } from "../../hooks/terminal/terminal.svelte";
  import { keyboardSizeHold } from "../../hooks/terminal/keyboard-size-hold-preference.svelte";
  import { touchGamepad } from "../../hooks/terminal/touch-gamepad-preference.svelte";
  import { useSessionTerminalChrome } from "../../hooks/terminal/session-terminal-chrome.svelte";
  import type { KeyboardVisibility } from "../../hooks/shared/viewport.svelte";
  import { useHybrid } from "../../hooks/sessions/hybrid.svelte";
  import { useChatUpload, useUpload } from "../../hooks/shared/upload.svelte";
  import { useFullscreen } from "../../hooks/shared/fullscreen.svelte";
  import { hasTouchInput, isIOS } from "../../platform/touch-input";
  import { findShell, shellUi } from "../../hooks/shared/shell.svelte";
  import { voicePreference } from "../../hooks/shared/voice-preference.svelte";
  import type { QuickCommand, SessionInfo } from "../../contracts/schemas";
  import type CodePanelComponent from "../code/CodePanel.svelte";
  import SessionTerminalDialogs from "./SessionTerminalDialogs.svelte";
  import SessionTerminalStage from "./SessionTerminalStage.svelte";
  import {
    settings,
    changeSettings,
  } from "../../domains/settings/settings.svelte";
  import {
    applySessionVoiceResult,
    createDraftReporter,
    createHeaderTerminalView,
    deriveChatUploadUnavailable,
    deriveQuickUnavailable,
    deriveSendUnavailable,
    executeSessionQuickCommand,
    savedStorageLabel,
    submitSessionDraft,
  } from "../../domains/terminal/session-terminal-io";
  import type {
    DraftOperation,
    TerminalDrafts,
    TextInsertion,
  } from "../../domains/terminal/terminal-draft.svelte";

  let {
    id,
    drafts,
    onUploadNotice,
    reopen,
    visible,
    active,
    paneActions,
    keyboardDismissed,
    keyboardVisibility,
    viewportHeight,
    oninfo,
    onremoved,
    onarchived,
    onleave,
    showForwards,
    feedSession,
    documentScroll = false,
    onscrollstate,
  }: {
    id: string;
    drafts: TerminalDrafts;
    onUploadNotice: (message: string) => void;
    reopen: () => void;
    visible: boolean;
    active: boolean;
    paneActions: Snippet<[boolean]>;
    keyboardDismissed: () => boolean;
    keyboardVisibility: () => KeyboardVisibility;
    viewportHeight: () => number;
    oninfo: (info: SessionInfo, connection: string) => void;
    onremoved: () => void;
    onleave?: () => void;
    onarchived: () => void;
    showForwards: () => void;
    feedSession?: SessionInfo;
    documentScroll?: boolean;
    onscrollstate?: (state: {
      chatView: boolean;
      desktop: boolean;
      codeShown: boolean;
      editing: boolean;
    }) => void;
  } = $props();
  const draft = untrack(() => drafts.get(id));
  const shell = findShell();
  const leave = () => (onleave ? onleave() : navigate(shellUi.home));
  const voiceEnabled = $derived(
    !!shell?.setup.value.voice && voicePreference.enabled,
  );
  let stage = $state<SessionTerminalStage>();
  let sending = false;
  let container = $state<HTMLDivElement>();
  let error = $state("");
  let notice = $state("");
  let closing = $state(false);
  let settingsOpen = $state(false);
  let scratchOpen = $state(false);
  const forceSoftKeys = $derived(settings.values.forceSoftKeys ?? false);
  let mounted = false;
  let codePanel = $state<CodePanelComponent>();
  let paneWidth = $state(0);
  const desktop = $derived(paneWidth >= 1024);
  let CodePanel = $state<typeof CodePanelComponent>();
  let loadingCode = false;
  const terminalShown = () =>
    hybrid.view === "terminal" || hybrid.view === "recovery";
  const terminalVisible = () =>
    visible && (desktop || !chrome.codeShown) && terminalShown();
  const chrome = useSessionTerminalChrome({
    id: () => id,
    draft,
    drafts: untrack(() => drafts),
    isMounted: () => mounted,
    isActiveAndVisible: () => active && visible,
    isTerminalView: () => hybrid.view === "terminal",
    isTerminalVisible: terminalVisible,
    isDesktop: () => desktop,
    focusFindBar: () => stage?.focusFind(),
    focusComposer: () => stage?.focusComposer(),
    clearTerminalFind: () => t.clearFind(),
    focusFallback: focusInput,
    fitTerminal: () => t.fit(),
    viewportHeight: () => viewportHeight(),
    keyboardVisibility: () => keyboardVisibility(),
    keyboardDismissed: () => keyboardDismissed(),
    releaseChatEditing: () => stage?.releaseChatEditing(),
    isCodePanelPoppedOut: () => !!codePanel?.isPoppedOut(),
    insertText,
    uploadFiles: (files, operation) =>
      uploads.uploadFiles(Array.from(files), operation),
    setError: (msg) => (error = msg),
    oninfo: (info, conn) => oninfo(info, conn),
    onscrollstate: (state) => onscrollstate?.(state),
  });
  $effect(() => {
    if (chrome.codeShown && !CodePanel && !loadingCode) {
      loadingCode = true;
      void import("../code/CodePanel.svelte")
        .then((m) => {
          if (mounted) CodePanel = m.default;
        })
        .catch((e) => (error = errorMessage(e)))
        .finally(() => (loadingCode = false));
    }
    if (terminalVisible()) {
      if (window.scrollX || window.scrollY) window.scrollTo(0, 0);
      void tick().then(() => t.fit());
    }
  });
  function beginInput(): DraftOperation {
    const operation = drafts.begin(draft);
    return hybrid.view === "chat"
      ? { ...operation, startedInDraft: true }
      : operation;
  }
  function insertReference(text: string) {
    if (hybrid.view === "chat") return insertText(text, beginInput());
    const result = insertText(text);
    if (result === "terminal" && !desktop) {
      chrome.codeShown = false;
      void tick().then(() => t.fit());
    }
    return result;
  }
  function insertText(
    text: string,
    operation = drafts.begin(draft),
  ): TextInsertion {
    if (!mounted) return "failed";
    return drafts.insert(draft, text, operation, t.paste);
  }
  function controlNotice() {
    error = "终端控制需先退出草稿模式；草稿框内可正常复制、选择和编辑。";
  }
  function softInput(input: string) {
    if (draft.enabled) stage?.editComposerKey(input);
    else t.send(input);
  }
  function focusInput() {
    if (!active || !visible) return;
    if (hybrid.view === "chat") stage?.focusChat();
    else if (draft.enabled) stage?.focusComposer();
    else t.focus();
  }
  export function focus() {
    focusInput();
  }
  export function send(input: string) {
    t.send(input);
  }
  function toggleDraft() {
    drafts.setEnabled(draft, !draft.enabled);
    t.resetModifiers();
    if (draft.enabled && !hasTouchInput()) {
      void tick().then(() => {
        if (active && visible) stage?.focusComposer();
      });
    }
  }
  const reportDraft = createDraftReporter(
    draft,
    untrack(() => drafts),
    () => mounted,
    (msg) => (error = msg),
    (msg) => (notice = msg),
  );
  function sendDraft(resend = false) {
    if (sending) return;
    sending = true;
    try {
      submitSessionDraft(
        draft,
        drafts,
        t,
        resend,
        reportDraft,
        (msg) => (error = msg),
        (msg) => (notice = msg),
      );
    } finally {
      sending = false;
    }
  }
  function releaseDraftSubmission() {
    if (draft.submission) t.untrack(draft.submission.id);
    drafts.releaseSubmission(draft);
    error = "";
  }
  function runQuickCommand(cmd: QuickCommand) {
    return executeSessionQuickCommand(cmd, {
      id,
      isMounted: () => mounted,
      isChatView: () => hybrid.view === "chat",
      draft,
      drafts,
      beginInput,
      insertText,
      resetModifiers: () => t.resetModifiers(),
      submitTerminal: (subId, text, enter, onProgress) =>
        t.submit(subId, text, enter, onProgress),
      getError: () => error,
      setError: (msg) => (error = msg),
    });
  }
  const t = useTerminal(
    () => container!,
    untrack(() => id),
    {
      paste: () => void chrome.paste(),
      copySelection: (text, userGesture) =>
        void chrome.copyToClipboard(text, userGesture ?? false),
    },
    terminalVisible,
    {
      draftEnabled: () => draft.enabled,
      focusDraft: () => {
        if (active && visible) stage?.focusComposer();
      },
      key: (event) => stage?.handleTerminalKey(event),
      insertText: (text) => {
        insertText(text);
      },
    },
    () => keyboardDismissed(),
    () => chrome.sizeLocked,
    () => active,
    false,
    () => chrome.keyboardHeld,
  );
  const sendUnavailable = $derived(deriveSendUnavailable(t));
  const quickUnavailable = $derived(deriveQuickUnavailable(t));
  const hybrid = useHybrid(
    untrack(() => id),
    t,
    untrack(() => drafts),
    draft,
    false,
  );
  chrome.syncSessionEffects(
    hybrid,
    () => t.connection,
    () => t.info?.generation,
  );
  const sessionId = untrack(() => id);
  const chatUploads = useChatUpload(
    (file, progress, signal) => uploadFile(sessionId, file, progress, signal),
    (message) => (error = message),
    {
      get: () => draft.attachments,
      set: (next) => drafts.setAttachments(draft, next),
    },
  );
  const chatUploadUnavailable = $derived(deriveChatUploadUnavailable(hybrid));
  const headerTerminal = createHeaderTerminalView(hybrid, t, terminalShown);
  const draftSaved = $derived(
    savedStorageLabel(draft.persisted, "草稿过大，仅保存在当前页面"),
  );
  const scratchSaved = $derived(
    savedStorageLabel(
      draft.scratchPersisted,
      "暂存超出本机保存限制，仅当前页面",
    ),
  );
  function showScratch() {
    draft.scratchUpdatedWhileHidden = false;
    scratchOpen = true;
  }
  const uploads = useUpload(
    () => container!,
    untrack(() => id),
    t,
    (message) => {
      if (mounted) error = message;
      else if (message) onUploadNotice(`会话 ${id}：${message}`);
    },
    {
      begin: () => drafts.begin(draft),
      destination: (operation) => drafts.destination(draft, operation),
      insert: insertText,
    },
  );
  const fullscreen = useFullscreen((message) => (error = message));

  onMount(() => {
    mounted = true;
    if (draft.submission)
      t.track(draft.submission.id, reportDraft(draft.submission.id, true));
    return () => {
      mounted = false;
    };
  });

  function insertVoiceResult(text: string) {
    applySessionVoiceResult(text, {
      isMounted: () => mounted,
      isChatView: () => hybrid.view === "chat",
      draft,
      drafts,
      beginInput,
      insertText,
      setError: (msg) => (error = msg),
      onInserted: () => {
        if (!hasTouchInput()) {
          void tick().then(() => {
            if (active && visible) focusInput();
          });
        }
      },
    });
  }
  async function focusTerminal(event: Event) {
    event.preventDefault();
    if (hasTouchInput()) return;
    await tick();
    if (!active || !visible) return;
    if (chrome.findOpen) {
      stage?.focusFind();
      return;
    }
    focusInput();
  }
</script>

<div
  class="session-terminal flex h-full min-h-0 min-w-0 flex-col overflow-hidden"
  class:force-soft-keys={forceSoftKeys}
  class:ios={isIOS()}
  bind:clientWidth={paneWidth}
  data-terminal-compact={!desktop}
>
  <SessionTerminalStage
    bind:this={stage}
    bind:container
    bind:notice
    bind:codeShown={
      () => chrome.codeShown, (value) => (chrome.codeShown = value)
    }
    bind:forceSoftKeys={
      () => forceSoftKeys, (value) => changeSettings({ forceSoftKeys: value })
    }
    {id}
    {paneActions}
    {headerTerminal}
    {fullscreen}
    {desktop}
    {documentScroll}
    {hybrid}
    terminal={t}
    {uploads}
    {chatUploads}
    {chatUploadUnavailable}
    {sendUnavailable}
    {quickUnavailable}
    {voiceEnabled}
    {draft}
    {drafts}
    {draftSaved}
    {scratchSaved}
    {error}
    pendingCopy={chrome.pendingCopy !== null}
    copyPending={chrome.copyPending}
    dismissCopy={() => (chrome.pendingCopy = null)}
    {showScratch}
    showFontSize={() => (settingsOpen = true)}
    closeSession={() => (closing = true)}
    {closing}
    {reopen}
    {leave}
    {feedSession}
    {showForwards}
    terminalShown={terminalShown()}
    terminalVisible={terminalVisible()}
    sizeLocked={chrome.sizeLocked}
    setSizeLocked={chrome.setSizeLocked}
    keyboardSizeHold={keyboardSizeHold.enabled}
    setKeyboardSizeHold={(value) => (keyboardSizeHold.enabled = value)}
    keyboardHeld={chrome.keyboardHeld}
    touchGamepad={touchGamepad.enabled}
    setTouchGamepad={(value) => (touchGamepad.enabled = value)}
    findOpen={chrome.findOpen}
    showFind={chrome.showFind}
    closeFind={chrome.closeFind}
    {keyboardDismissed}
    {toggleDraft}
    {sendDraft}
    {releaseDraftSubmission}
    runQuickCommand={(cmd) => void runQuickCommand(cmd)}
    manageQuickCommands={() =>
      navigate(`/settings?session=${id}#quick-commands`)}
    {focusTerminal}
    {insertVoiceResult}
    onVoiceError={(message) => {
      if (!mounted) return;
      if (hybrid.view !== "chat" || !stage?.setChatVoiceError(message))
        error = message;
    }}
    {softInput}
    {controlNotice}
    copy={chrome.copy}
    paste={chrome.paste}
    holdBegin={() => {
      if (draft.enabled) stage?.openComposerKeyboard(chrome.beginHold);
      else t.keyboard(chrome.beginHold);
    }}
    review={chrome.review}
    openCodeLocation={chrome.openCodeLocation}
    onChatEditChange={(value) => (chrome.chatEditing = value)}
    onChooseSwitch={() => (chrome.choiceClosed = false)}
  >
    {#snippet codePane()}
      {#if CodePanel}<CodePanel
          bind:this={codePanel}
          {id}
          visible={visible && chrome.codeShown}
          connection={hybrid.view === "chat"
            ? hybrid.chat.connection
            : t.connection}
          paste={insertReference}
          draftEnabled={draft.enabled || hybrid.view === "chat"}
          reveal={chrome.codeTarget}
          sessionCwd={hybrid.info?.cwd ?? ""}
          reportedCwd={hybrid.info?.reportedCwd ?? null}
        />{/if}
    {/snippet}
  </SessionTerminalStage>
</div>

{#if active && visible}
  <SessionTerminalDialogs
    {id}
    {draft}
    {drafts}
    {hybrid}
    fontSize={t.fontSize}
    setFontSize={t.setFontSize}
    filePaste={uploads.filePaste}
    pasteOperation={chrome.pasteOperation}
    {insertText}
    {focusTerminal}
    {scratchSaved}
    {onremoved}
    {onarchived}
    bind:pasteOpen={() => chrome.pasteOpen, (v) => (chrome.pasteOpen = v)}
    bind:pasteText={() => chrome.pasteText, (v) => (chrome.pasteText = v)}
    bind:copyOpen={() => chrome.copyOpen, (v) => (chrome.copyOpen = v)}
    copyText={chrome.copyText}
    bind:settingsOpen
    bind:closing
    bind:choiceClosed={
      () => chrome.choiceClosed, (v) => (chrome.choiceClosed = v)
    }
    bind:scratchOpen
  />
{/if}
