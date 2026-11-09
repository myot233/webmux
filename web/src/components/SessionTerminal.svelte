<script lang="ts">
  import { paneEnter } from "../hooks/pane-enter";
  import { settings, changeSettings } from "../settings.svelte";
  import { errorMessage } from "../error";
  import { onMount, tick, untrack, type Snippet } from "svelte";
  import {
    apiBuffer,
    agentGuidePrompt,
    ensureArtifactsFolder,
    navigate,
    store,
    stored,
    uploadFile,
  } from "../api";
  import {
    renderArtifactDir,
    renderWebmuxBrief,
    usesArtifactDir,
    usesWebmuxBrief,
  } from "../quick-commands";
  import { clipboardFiles, copyText as writeClipboard } from "../clipboard";
  import { useTerminal } from "../hooks/terminal.svelte";
  import { createKeyboardSizeHold } from "../hooks/keyboard-size-hold";
  import { keyboardSizeHold } from "../hooks/keyboard-size-hold-preference.svelte";
  import { touchGamepad } from "../hooks/touch-gamepad-preference.svelte";
  import { codePanelState } from "../hooks/code-panel-state";
  import type { KeyboardVisibility } from "../hooks/viewport.svelte";
  import { useHybrid } from "../hooks/hybrid.svelte";
  import { useChatUpload, useUpload } from "../hooks/upload.svelte";
  import { useFullscreen } from "../hooks/fullscreen.svelte";
  import { hasTouchInput, isIOS } from "../touch-input";
  import { findShell, shellUi } from "../shell.svelte";
  import type { ButtonSize, ButtonVariant } from "$lib/components/ui/button";
  import QuickCommandMenu from "../components/QuickCommandMenu.svelte";
  import VoiceButton from "../components/VoiceButton.svelte";
  import { voicePreference } from "../hooks/voice-preference.svelte";
  import { quickCommands } from "../quick-commands.svelte";
  import type { ChatCodeLink } from "../markdown-link";
  import type {
    CodeReveal,
    QuickCommand,
    SubmissionProgress,
  } from "../schemas";
  import { newSubmissionId } from "../terminal-protocol";
  import { browserDraftStorage } from "../draft-storage";
  import CloseSessionDialog from "../components/CloseSessionDialog.svelte";
  import ConfirmDialog from "../components/ConfirmDialog.svelte";
  import ConnectorInstallHint from "../components/ConnectorInstallHint.svelte";
  import HybridCandidatePicker from "../components/HybridCandidatePicker.svelte";
  import HybridRecovery from "../components/HybridRecovery.svelte";
  import ChatView from "../components/chat/ChatView.svelte";
  import SwitchChoiceDialog from "../components/chat/SwitchChoiceDialog.svelte";
  import SwitchProgress from "../components/chat/SwitchProgress.svelte";
  import type CodePanelComponent from "../components/CodePanel.svelte";
  import ScratchPanel from "../components/ScratchPanel.svelte";
  import TerminalCopyDialog from "../components/TerminalCopyDialog.svelte";
  import DraftToggle from "../components/DraftToggle.svelte";
  import TerminalFontDialog from "../components/TerminalFontDialog.svelte";
  import TerminalHeader from "../components/TerminalHeader.svelte";
  import TerminalInputComposer from "../components/TerminalInputComposer.svelte";
  import TerminalNotices from "../components/TerminalNotices.svelte";
  import TerminalPasteDialog from "../components/TerminalPasteDialog.svelte";
  import TerminalViewport from "../components/TerminalViewport.svelte";
  import TerminalWorkspace from "../components/TerminalWorkspace.svelte";
  import TerminalFindBar from "../components/TerminalFindBar.svelte";
  import type {
    DraftOperation,
    TerminalDrafts,
    TextInsertion,
  } from "../terminal-draft.svelte";

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
  }: {
    id: string;
    drafts: TerminalDrafts;
    onUploadNotice: (message: string) => void;
    /** Mounts the page afresh, for a session that came back under its ID. */
    reopen: () => void;
    visible: boolean;
    active: boolean;
    paneActions: Snippet<[boolean]>;
    keyboardDismissed: () => boolean;
    keyboardVisibility: () => KeyboardVisibility;
    viewportHeight: () => number;
    oninfo: (
      info: import("../schemas").SessionInfo,
      connection: string,
    ) => void;
    onremoved: () => void;
    /** Closes this pane when others share the page; absent, a back button goes home. */
    onleave?: () => void;
    // A live hybrid session was archived from this pane.
    onarchived: () => void;
    /** Opens the port-forwarding dialog from a port prompt. */
    showForwards: () => void;
    /** The session as reported by the `/ws/sessions` feed, if connected. */
    feedSession?: import("../schemas").SessionInfo;
  } = $props();
  const draft = untrack(() => drafts.get(id));
  const shell = findShell();
  const leave = () => (onleave ? onleave() : navigate(shellUi.home));
  const voiceEnabled = $derived(
    !!shell?.setup.value.voice && voicePreference.enabled,
  );
  let composer = $state<TerminalInputComposer>();
  let chatView = $state<ChatView>();
  let sending = false;
  let pasteOperation: DraftOperation | undefined;
  let container = $state<HTMLDivElement>();
  let error = $state("");
  let notice = $state("");
  let closing = $state(false);
  let pasteOpen = $state(false);
  let pasteText = $state("");
  let copyOpen = $state(false);
  let copyText = $state("");
  let pendingCopy = $state<string | null>(null);
  let settingsOpen = $state(false);
  let scratchOpen = $state(false);
  const forceSoftKeys = $derived(settings.values.forceSoftKeys ?? false);
  let mounted = false;
  let codeShown = $state(codePanelState.shown());
  $effect(() => codePanelState.setShown(codeShown));
  let codePanel = $state<CodePanelComponent>();
  // A code location the code panel should open (from chat links or tool diffs).
  let codeTarget = $state<CodeReveal>();
  let paneWidth = $state(0);
  const desktop = $derived(paneWidth >= 1024);
  let choiceClosed = $state(false);
  let findOpen = $state(false);
  let findPanel = $state<TerminalFindBar>();
  let findReturnFocus: HTMLElement | null = null;
  function showFind() {
    if (hybrid.view !== "terminal") return;
    if (!findOpen) {
      const active = document.activeElement;
      findReturnFocus =
        active instanceof HTMLElement &&
        (active.matches(".xterm-helper-textarea") ||
          active.closest(".draft-pane"))
          ? active
          : null;
    }
    if (!desktop) codeShown = false;
    findOpen = true;
    void tick().then(() => {
      if (active && visible) findPanel?.focus();
    });
  }
  function closeFind() {
    t.clearFind();
    findOpen = false;
    if (document.activeElement instanceof HTMLElement)
      document.activeElement.blur();
    if (!hasTouchInput())
      void tick().then(() => {
        if (!active || !visible) return;
        if (findReturnFocus?.isConnected) findReturnFocus.focus();
        else focusInput();
      });
  }
  // Loaded on first use; the type-only import keeps it out of this chunk.
  let CodePanel = $state<typeof CodePanelComponent>();
  let loadingCode = false;
  // A hybrid session off its terminal side keeps xterm mounted but hidden.
  const terminalShown = () =>
    hybrid.view === "terminal" || hybrid.view === "recovery";
  const terminalVisible = () =>
    visible && (desktop || !codeShown) && terminalShown();
  $effect(() => {
    if (codeShown && !CodePanel && !loadingCode) {
      loadingCode = true;
      void import("../components/CodePanel.svelte")
        .then((m) => {
          if (mounted) CodePanel = m.default;
        })
        .catch((e) => {
          error = errorMessage(e);
        })
        .finally(() => (loadingCode = false));
    }
    const showing = terminalVisible();
    if (showing) void tick().then(() => t.fit());
  });
  // The chat input is the session's draft text: input begun on the chat side
  // goes there whether or not the terminal draft pane is open.
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
      codeShown = false;
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
    if (draft.enabled) composer?.editKey(input);
    else t.send(input);
  }
  function focusInput() {
    if (!active || !visible) return;
    if (hybrid.view === "chat") chatView?.focus();
    else if (draft.enabled) composer?.focus();
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
        if (active && visible) composer?.focus();
      });
    }
  }
  const draftMessages: Partial<Record<SubmissionProgress, string>> = {
    failed: "草稿未写入终端，可修改后重新发送。",
    missing:
      "服务端没有这次提交的记录：可能未送达，或记录已过期。请检查终端后决定是否重新发送。",
    partial: "草稿只有部分写入终端，提交键未写入；请检查终端后再处理草稿。",
  };
  // Draft text stays locked until the server reports the outcome.
  function reportDraft(id: string, announce: boolean) {
    return (state: SubmissionProgress) => {
      drafts.updateSubmission(draft, id, state);
      if (!mounted) return;
      if (state === "written") {
        error = "";
        if (announce) notice = "上次提交的草稿已写入终端。";
      } else if (draftMessages[state]) error = draftMessages[state];
    };
  }
  // `resend` repeats an unconfirmed submission under the same ID, which the
  // server writes at most once.
  function sendDraft(resend = false) {
    if (sending || !draft.enabled || !draft.text || !drafts.valid(draft))
      return;
    const pending = draft.submission;
    if (pending && (!resend || pending.state !== "unknown")) return;
    sending = true;
    try {
      const id = pending?.id ?? newSubmissionId();
      if (!pending) drafts.beginSubmission(draft, id);
      notice = "";
      if (t.submit(id, draft.text, true, reportDraft(id, false))) error = "";
      else {
        if (!pending) drafts.releaseSubmission(draft);
        error = "终端未就绪，草稿未发送；请恢复连接后手动重试。";
      }
    } finally {
      sending = false;
    }
  }
  function releaseDraftSubmission() {
    if (draft.submission) t.untrack(draft.submission.id);
    drafts.releaseSubmission(draft);
    error = "";
  }
  const quickMessages: Partial<Record<SubmissionProgress, string>> = {
    failed: "快捷指令未写入终端；请检查终端后重试。",
    missing: "快捷指令未送达服务端；请检查终端后重试。",
    partial: "快捷指令正文已写入，但回车未写入；请检查终端后手动提交。",
    unknown: "连接中断，快捷指令结果未知；重连后自动查询。",
  };
  // The operation binds this session's draft and target at selection. A body
  // with `{{dir}}` or `{{webmux}}` first waits for the artifacts folder or
  // the WebMux brief, then runs as written; the brief goes in last, so
  // nothing in it is replaced.
  async function runQuickCommand(cmd: QuickCommand) {
    t.resetModifiers();
    const chat = hybrid.view === "chat";
    let text = cmd.command;
    let operation: DraftOperation | undefined;
    if (usesArtifactDir(text)) {
      operation = beginInput();
      let dir: string;
      try {
        dir = await ensureArtifactsFolder(id);
      } catch (err) {
        if (mounted) error = `临时文件夹不可用：${errorMessage(err)}`;
        return;
      }
      if (!mounted) return;
      text = renderArtifactDir(text, dir);
    }
    if (usesWebmuxBrief(text)) {
      operation ??= beginInput();
      let brief: string;
      try {
        brief = await agentGuidePrompt(id);
      } catch (err) {
        if (mounted) error = `WebMux 工具说明不可用：${errorMessage(err)}`;
        return;
      }
      if (!mounted) return;
      text = renderWebmuxBrief(text, brief);
    }
    if (!chat && cmd.enter && drafts.destination(draft) === "terminal") {
      // Text and Enter form one acknowledged submission.
      const sent = t.submit(newSubmissionId(), text, true, (state) => {
        if (!mounted) return;
        if (quickMessages[state]) error = quickMessages[state];
        else if (state === "written" && error === quickMessages.unknown)
          error = "";
      });
      error = sent ? "" : "终端未就绪，快捷指令未发送；请恢复连接后重试。";
      return;
    }
    // The chat side only inserts: sending stays with the message box.
    const result = insertText(text, operation ?? beginInput());
    if (result === "failed")
      error =
        chat || drafts.destination(draft, operation) === "draft"
          ? "草稿正在提交到终端，快捷指令未插入；请确认结果后重试。"
          : "终端未就绪，快捷指令未发送；请恢复连接后重试。";
    else error = "";
  }
  let sizeLocked = $state(stored("sizeLocked") === "1");
  let keyboardHeld = $state(false);
  const hold = createKeyboardSizeHold({
    canHold: () => keyboardSizeHold.enabled && hasTouchInput(),
    keyboardStays: () => {
      const active = document.activeElement;
      return (
        active instanceof HTMLInputElement ||
        active instanceof HTMLTextAreaElement ||
        (active instanceof HTMLElement && active.isContentEditable)
      );
    },
    onActive: (value) => {
      keyboardHeld = value;
    },
    release: () => {
      void tick().then(() => t.fit());
    },
  });
  $effect(() => {
    viewportHeight();
    hold.update(keyboardVisibility());
  });
  // The preference is device-wide: turning it off in any pane ends its holds.
  $effect(() => {
    if (!keyboardSizeHold.enabled) hold.cancel();
  });
  $effect(() => () => hold.dispose());
  const t = useTerminal(
    () => container!,
    untrack(() => id),
    {
      paste: () => {
        void paste();
      },
      copySelection: (text, userGesture) => {
        void copyToClipboard(text, userGesture ?? false);
      },
    },
    terminalVisible,
    {
      draftEnabled: () => draft.enabled,
      focusDraft: () => {
        if (active && visible) composer?.focus();
      },
      key: (event) => composer?.terminalKey(event),
      insertText: (text) => {
        insertText(text);
      },
    },
    () => keyboardDismissed(),
    () => sizeLocked,
    () => active,
    false,
    () => keyboardHeld,
  );
  function setSizeLocked(value: boolean) {
    sizeLocked = value;
    store("sizeLocked", value ? "1" : "0");
    void tick().then(() => t.fit());
  }
  function setKeyboardSizeHold(value: boolean) {
    keyboardSizeHold.enabled = value;
  }
  function setTouchGamepad(value: boolean) {
    touchGamepad.enabled = value;
  }
  const sendUnavailable = $derived(
    t.connection === "missing"
      ? "会话不存在，无法发送"
      : t.exitCode !== null || t.connection === "exited"
        ? "会话已退出，可编辑或复制草稿"
        : !t.ready
          ? "终端未就绪，草稿不会自动发送"
          : "",
  );
  const quickUnavailable = $derived(
    t.exitCode !== null || t.connection === "exited"
      ? "会话已退出，无法发送"
      : !t.ready
        ? "终端未就绪，暂不能发送"
        : "",
  );
  const hybrid = useHybrid(
    untrack(() => id),
    t,
    untrack(() => drafts),
    draft,
    false,
  );
  $effect(() => {
    const current = hybrid.info;
    const connection =
      hybrid.view === "chat" ? hybrid.chat.connection : t.connection;
    if (current) untrack(() => oninfo(current, connection));
  });
  // Chat uploads live on this page (not in ChatView) so switching sides does
  // not abort an upload; pending attachments live on the session draft so
  // switching pages preserves them.
  const sessionId = untrack(() => id);
  const chatUploads = useChatUpload(
    (file, progress, signal) => uploadFile(sessionId, file, progress, signal),
    (message) => (error = message),
    {
      get: () => draft.attachments,
      set: (next) => drafts.setAttachments(draft, next),
    },
  );
  const chatUploadUnavailable = $derived(
    hybrid.missing
      ? "会话不存在，无法上传文件"
      : hybrid.info?.status === "exited"
        ? "会话已退出，无法上传文件"
        : "",
  );
  // Only a session without a recoverable record is gone for good.
  $effect(() => {
    if (hybrid.missing) drafts.remove(id);
  });
  $effect(() => {
    if (!hybrid.awaitingChoice) choiceClosed = false;
  });
  // The agent session picked when a switch to chat found several.
  let ambiguousPick = $derived(
    hybrid.ambiguous?.candidates[0]?.agentSessionId ?? "",
  );
  // The header reads the chat channel while the terminal is not shown.
  const headerTerminal = {
    get info() {
      return hybrid.info;
    },
    get connection() {
      return terminalShown() ? t.connection : hybrid.chat.connection;
    },
    get exitCode() {
      return terminalShown() ? t.exitCode : null;
    },
    get restoring() {
      return t.restoring;
    },
    get historyTruncated() {
      return t.historyTruncated;
    },
    get canFind() {
      return t.canFind;
    },
    scrollToBottom: () => t.scrollToBottom(),
    clear: () => t.clear(),
    resync: () => t.resync(),
  };
  function review(path: string) {
    if (!codePanel?.isPoppedOut()) codeShown = true;
    codeTarget = { path, mode: "diff", key: (codeTarget?.key ?? 0) + 1 };
  }
  function openCodeLocation(target: ChatCodeLink) {
    if (!codePanel?.isPoppedOut()) codeShown = true;
    codeTarget = { ...target, mode: "file", key: (codeTarget?.key ?? 0) + 1 };
  }
  $effect(() => {
    const generation = t.info?.generation;
    if (generation) drafts.bindGeneration(draft, generation);
  });
  const draftStorage = browserDraftStorage();
  const savedLabel = (persisted: boolean, oversized: string) =>
    persisted && draftStorage.enabled
      ? "已保存在本机"
      : draftStorage.enabled
        ? oversized
        : "仅当前页面，刷新后丢失";
  const draftSaved = $derived(
    savedLabel(draft.persisted, "草稿过大，仅保存在当前页面"),
  );
  const scratchSaved = $derived(
    savedLabel(draft.scratchPersisted, "暂存超出本机保存限制，仅当前页面"),
  );
  function closeSession() {
    closing = true;
  }
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
  const fullscreen = useFullscreen((message) => {
    error = message;
  });

  onMount(() => {
    mounted = true;
    // A submission restored from storage may have been written before the
    // page went away; ask the server instead of guessing.
    if (draft.submission)
      t.track(draft.submission.id, reportDraft(draft.submission.id, true));
    return () => {
      mounted = false;
    };
  });

  async function copyToClipboard(text: string, userGesture = false) {
    if (!mounted) return;
    // Keep only the latest background request. It must neither replace the
    // clipboard nor open a focus-trapped dialog in another pane.
    if (!active || !visible || (!userGesture && !terminalVisible())) {
      pendingCopy = text;
      return;
    }
    if (window.isSecureContext && navigator.clipboard?.writeText) {
      try {
        await navigator.clipboard.writeText(text);
        return;
      } catch {
        // Permission denied or a missing transient activation.
      }
    }
    // A toolbar click or Ctrl/Cmd+C is a gesture. The legacy execCommand path
    // only fills the gap on plain-HTTP LAN/VPN origins; a secure origin that
    // denied the Clipboard API keeps the manual text box fallback.
    if (userGesture && !window.isSecureContext && (await writeClipboard(text)))
      return;
    if (!mounted) return;
    if (!active || !visible || (!userGesture && !terminalVisible())) {
      pendingCopy = text;
      return;
    }
    copyText = text;
    copyOpen = true;
  }
  function copyPending() {
    const text = pendingCopy;
    pendingCopy = null;
    if (text !== null) void copyToClipboard(text, true);
  }
  async function copy() {
    try {
      await copyToClipboard(
        new TextDecoder().decode(await apiBuffer(id)),
        true,
      );
    } catch (err) {
      error = errorMessage(err);
    }
  }

  async function paste() {
    const operation = drafts.begin(draft);
    try {
      if (
        matchMedia("(pointer: fine)").matches &&
        typeof navigator.clipboard?.read === "function"
      ) {
        const items = await navigator.clipboard.read();
        if (!mounted) return;
        const files = await clipboardFiles(items);
        if (files.length) {
          if (mounted) await uploads.uploadFiles(files, operation);
          return;
        }
        const textItem = items.find((item) =>
          item.types.includes("text/plain"),
        );
        if (textItem) {
          const text = await (await textItem.getType("text/plain")).text();
          if (mounted) insertText(text, operation);
          return;
        }
      }
      const text = await navigator.clipboard.readText();
      if (mounted) insertText(text, operation);
    } catch {
      if (mounted) {
        if (drafts.destination(draft, operation) === "draft") {
          error = draft.enabled
            ? "无法读取剪贴板，请在现有草稿框内手动粘贴。"
            : "无法读取剪贴板，请重新开启草稿模式后在草稿框内手动粘贴。";
          if (draft.enabled && active && visible && !hasTouchInput())
            composer?.focus();
        } else if (drafts.destination(draft, operation) !== "failed") {
          pasteOperation = operation;
          pasteText = "";
          pasteOpen = true;
        }
      }
    }
  }
  function insertVoiceResult(text: string) {
    if (!text || !mounted) return;
    const chat = hybrid.view === "chat";
    const operation = beginInput();
    const result = insertText(text, operation);
    if (result === "failed") {
      error =
        chat || drafts.destination(draft, operation) === "draft"
          ? "草稿正在提交到终端，语音识别内容未插入；请确认结果后重试。"
          : "终端未就绪，语音识别内容未发送；请恢复连接后重试。";
    } else {
      error = "";
      if (!hasTouchInput()) {
        void tick().then(() => {
          if (active && visible) focusInput();
        });
      }
    }
  }
  async function focusTerminal(event: Event) {
    event.preventDefault();
    if (hasTouchInput()) return;
    await tick();
    if (!active || !visible) return;
    if (findOpen) {
      findPanel?.focus();
      return;
    }
    focusInput();
  }
</script>

{#snippet voiceButton(
  side: "top" | "bottom",
  size?: ButtonSize,
  variant?: ButtonVariant,
)}
  <VoiceButton
    {size}
    {variant}
    disabled={hybrid.view === "chat"
      ? hybrid.missing || !!draft.submission
      : t.connection === "missing" ||
        (draft.enabled
          ? !!draft.submission
          : t.exitCode !== null || t.connection === "exited" || !t.ready)}
    onmousedown={(event) => {
      if (side === "top" && hybrid.view !== "chat") event.preventDefault();
    }}
    onresult={insertVoiceResult}
    onerror={(message) => {
      if (!mounted) return;
      if (hybrid.view === "chat" && chatView) chatView.setVoiceError(message);
      else error = message;
    }}
  />
{/snippet}
{#snippet draftVoice()}
  {@render voiceButton("top", "icon-sm", "ghost")}
{/snippet}
{#snippet quickMenu(
  side: "top" | "bottom",
  size?: ButtonSize,
  variant?: ButtonVariant,
)}
  <!-- In the terminal view the microphone lives in the draft box only. -->
  {#if voiceEnabled && hybrid.view === "chat"}
    {@render voiceButton(side, size, variant)}
  {/if}
  <QuickCommandMenu
    commands={quickCommands.list}
    target={hybrid.view === "chat"
      ? "chat"
      : draft.enabled
        ? "draft"
        : "terminal"}
    unavailable={quickUnavailable}
    disabled={hybrid.view === "chat"
      ? hybrid.missing
      : t.connection === "missing"}
    {side}
    {size}
    {variant}
    run={(cmd) => void runQuickCommand(cmd)}
    manage={() => navigate(`/settings?session=${id}#quick-commands`)}
    onCloseAutoFocus={focusTerminal}
  />
{/snippet}
{#snippet draftToggle()}
  <DraftToggle
    class="sm:hidden"
    {draft}
    {draftSaved}
    {scratchSaved}
    disabled={t.connection === "missing"}
    toggle={toggleDraft}
  />
{/snippet}

{#snippet recovery()}
  {#if hybrid.recoverable}<HybridRecovery
      record={hybrid.recoverable}
      onresumed={reopen}
      {leave}
    />{/if}
{/snippet}

<div
  class="session-terminal flex h-full min-h-0 min-w-0 flex-col overflow-hidden"
  class:force-soft-keys={forceSoftKeys}
  class:ios={isIOS()}
  bind:clientWidth={paneWidth}
  data-terminal-compact={!desktop}
>
  <TerminalHeader
    {paneActions}
    terminal={headerTerminal}
    {fullscreen}
    {draft}
    {draftSaved}
    {scratchSaved}
    uploading={uploads.progress !== null}
    {desktop}
    {sizeLocked}
    {setSizeLocked}
    keyboardSizeHold={keyboardSizeHold.enabled}
    {setKeyboardSizeHold}
    bind:codeShown
    bind:forceSoftKeys={
      () => forceSoftKeys, (value) => changeSettings({ forceSoftKeys: value })
    }
    {quickMenu}
    upload={uploads.uploadFiles}
    {toggleDraft}
    {copy}
    {paste}
    {showScratch}
    showFontSize={() => (settingsOpen = true)}
    {showFind}
    close={closeSession}
    {leave}
    hybrid={hybrid.known ? hybrid : undefined}
  />
  <TerminalWorkspace
    {codeShown}
    {desktop}
    draftOpen={draft.enabled && hybrid.view === "terminal"}
    onfind={showFind}
  >
    {#snippet terminalPane()}
      <div class="relative flex h-full flex-col">
        <!-- Notices float over the terminal view (not the chat view or the code
           pane beside it): showing or dismissing one must not resize the
           terminal, which would redraw the PTY. On the chat view, notices stay
           in the flex flow above the scrollable transcript so they never cover
           messages or banners. z-30 keeps them above the find bar (z-20). -->
        <div
          class={hybrid.view === "chat"
            ? "flex shrink-0 flex-col bg-background"
            : "absolute inset-x-0 top-0 z-30 flex flex-col bg-background"}
        >
          <TerminalNotices
            {leave}
            terminal={t}
            {uploads}
            {draft}
            {draftSaved}
            {error}
            pendingCopy={pendingCopy !== null}
            {copyPending}
            dismissCopy={() => (pendingCopy = null)}
            bind:notice
            {showScratch}
            dismissScratch={() => (draft.scratchUpdatedWhileHidden = false)}
            remove={closeSession}
            notices={hybrid.notices}
            dismiss={(key) => hybrid.dismiss(key)}
            recovery={hybrid.recoverable && !closing ? recovery : undefined}
            session={id}
            listening={hybrid.view === "chat"
              ? (feedSession?.listeningPorts ?? hybrid.info?.listeningPorts)
              : hybrid.view === "terminal"
                ? hybrid.info?.listeningPorts
                : undefined}
            {showForwards}
          />
          {#if hybrid.connector && hybrid.view === "terminal"}<div
              class="shrink-0 border-b px-4 py-2"
            >
              <ConnectorInstallHint
                connector={hybrid.connector}
                detail="安装后会自动重新检测"
                ondismiss={() => hybrid.dismissConnector()}
              />
            </div>{/if}
        </div>
        <!-- A hybrid session's panes fade in as a switch lands on them. -->
        <div
          class="min-h-0 flex-1"
          use:paneEnter={hybrid.known && terminalShown()}
          hidden={!terminalShown()}
        >
          <TerminalViewport
            bind:container
            terminal={t}
            visible={terminalVisible()}
            sizeLocked={sizeLocked || keyboardHeld}
            touchGamepad={touchGamepad.enabled}
            {setTouchGamepad}
            {findOpen}
            {uploads}
            {quickMenu}
            {draftToggle}
            send={softInput}
            toggle={(key) => {
              if (draft.enabled) controlNotice();
              else t.toggleModifier(key);
            }}
            lock={(key) => {
              if (draft.enabled) controlNotice();
              else t.lockModifier(key);
            }}
            {paste}
            keyboard={() => {
              if (draft.enabled) composer?.keyboard(hold.begin);
              else t.keyboard(hold.begin);
            }}
          >
            {#snippet findBar()}
              <TerminalFindBar
                bind:this={findPanel}
                open={findOpen}
                terminal={t}
                onclose={closeFind}
              />
            {/snippet}
          </TerminalViewport>
        </div>
        {#if hybrid.view === "chat"}
          <div class="min-h-0 flex-1" use:paneEnter={true}>
            <ChatView
              bind:this={chatView}
              {sessionId}
              model={hybrid.chat.model}
              chat={hybrid.chat.state}
              connection={hybrid.chat.connection}
              {draft}
              {drafts}
              uploads={chatUploads}
              uploadUnavailable={chatUploadUnavailable}
              location={hybrid.info}
              actions={hybrid.actions}
              onreview={review}
              onopenlocation={openCodeLocation}
              onterminal={() => hybrid.request("terminal")}
              {quickMenu}
            />
          </div>
        {:else if hybrid.view === "switching"}
          <SwitchProgress
            target={hybrid.target}
            steps={hybrid.steps}
            awaitingChoice={hybrid.awaitingChoice}
            onchoose={() => (choiceClosed = false)}
          />
        {/if}
      </div>
    {/snippet}
    {#snippet draftPane()}
      <TerminalInputComposer
        bind:this={composer}
        {draft}
        {drafts}
        {keyboardDismissed}
        unavailable={sendUnavailable}
        target={t.info?.title || id}
        openScratch={showScratch}
        send={() => sendDraft()}
        resend={() => sendDraft(true)}
        release={releaseDraftSubmission}
        filePaste={uploads.filePaste}
        {controlNotice}
        voice={voiceEnabled ? draftVoice : undefined}
      />
    {/snippet}
    {#snippet codePane()}
      {#if CodePanel}<CodePanel
          bind:this={codePanel}
          {id}
          visible={visible && codeShown}
          connection={hybrid.view === "chat"
            ? hybrid.chat.connection
            : t.connection}
          paste={insertReference}
          draftEnabled={draft.enabled || hybrid.view === "chat"}
          reveal={codeTarget}
          sessionCwd={hybrid.info?.cwd ?? ""}
          reportedCwd={hybrid.info?.reportedCwd ?? null}
        />{/if}
    {/snippet}
  </TerminalWorkspace>
</div>

{#if active && visible}
  <TerminalPasteDialog
    bind:open={pasteOpen}
    bind:text={pasteText}
    filePaste={uploads.filePaste}
    send={(text) => insertText(text, pasteOperation)}
    onCloseAutoFocus={focusTerminal}
  />
  <TerminalCopyDialog
    bind:open={copyOpen}
    text={copyText}
    onCloseAutoFocus={focusTerminal}
  />
  <TerminalFontDialog
    bind:open={settingsOpen}
    fontSize={t.fontSize}
    setFontSize={t.setFontSize}
    onCloseAutoFocus={focusTerminal}
  />
  {#if closing && hybrid.info}<CloseSessionDialog
      session={hybrid.info}
      onclose={() => {
        closing = false;
      }}
      onremoved={() => {
        // A hybrid session's record keeps its draft until it is removed.
        if (!hybrid.known) drafts.remove(id);
        onremoved();
      }}
      {onarchived}
    />{/if}
  <ConfirmDialog
    bind:open={
      () => !!hybrid.confirming,
      (open) => {
        if (!open) hybrid.cancelConfirm();
      }
    }
    title={hybrid.confirming?.title ?? ""}
    onconfirm={() => hybrid.confirm()}>{hybrid.confirming?.body}</ConfirmDialog
  >
  <ConfirmDialog
    bind:open={
      () => !!hybrid.ambiguous,
      (open) => {
        if (!open) hybrid.cancelPick();
      }
    }
    title="选择要切到聊天的会话"
    onconfirm={() => hybrid.pick(ambiguousPick)}
    >{hybrid.ambiguous?.message}<HybridCandidatePicker
      candidates={hybrid.ambiguous?.candidates ?? []}
      bind:picked={ambiguousPick}
      label="选择要打开的会话"
    /></ConfirmDialog
  >
  <SwitchChoiceDialog
    bind:open={
      () => hybrid.awaitingChoice && !choiceClosed,
      (open) => (choiceClosed = !open)
    }
    note={hybrid.chat.state?.forceSwitchNote ?? ""}
    busy={hybrid.answering}
    onchoose={(choice) => void hybrid.choose(choice)}
  />
  <ScratchPanel bind:open={scratchOpen} {draft} {drafts} saved={scratchSaved} />
{/if}
