<script lang="ts">
  import { settings, changeSettings } from "../settings.svelte";
  import { errorMessage } from "../error";
  import { onMount, tick, untrack } from "svelte";
  import { MediaQuery } from "svelte/reactivity";
  import { apiBuffer, navigate, store, stored } from "../api";
  import { clipboardFiles, copyText as writeClipboard } from "../clipboard";
  import { useTerminal } from "../hooks/terminal.svelte";
  import { useHybrid } from "../hooks/hybrid.svelte";
  import { useViewport } from "../hooks/viewport.svelte";
  import { useChatUpload, useUpload } from "../hooks/upload.svelte";
  import { useFullscreen } from "../hooks/fullscreen.svelte";
  import { hasTouchInput, isIOS } from "../touchInput";
  import QuickCommandMenu from "../components/QuickCommandMenu.svelte";
  import { quickCommands } from "../quickCommands.svelte";
  import type { QuickCommand, SubmissionProgress } from "../schemas";
  import { newSubmissionId } from "../terminalProtocol";
  import { browserDraftStorage } from "../draftStorage";
  import CloseSessionDialog from "../components/CloseSessionDialog.svelte";
  import ConfirmDialog from "../components/ConfirmDialog.svelte";
  import HybridRecovery from "../components/HybridRecovery.svelte";
  import ChatView from "../components/chat/ChatView.svelte";
  import SwitchChoiceDialog from "../components/chat/SwitchChoiceDialog.svelte";
  import SwitchProgress from "../components/chat/SwitchProgress.svelte";
  import type CodePanelComponent from "../components/CodePanel.svelte";
  import ScratchPanel from "../components/ScratchPanel.svelte";
  import TerminalCopyDialog from "../components/TerminalCopyDialog.svelte";
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
  }: {
    id: string;
    drafts: TerminalDrafts;
    onUploadNotice: (message: string) => void;
    /** Mounts the page afresh, for a session that came back under its ID. */
    reopen: () => void;
  } = $props();
  const draft = untrack(() => drafts.get(id));
  let composer = $state<TerminalInputComposer>();
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
  let settingsOpen = $state(false);
  let scratchOpen = $state(false);
  const forceSoftKeys = $derived(settings.values.forceSoftKeys ?? false);
  let mounted = false;
  let codeShown = $state(false);
  // A path the code panel should open (a chat tool call's diff).
  let codeTarget = $state<{ path: string; key: number }>();
  const desktopMedia = new MediaQuery("(min-width: 1024px)");
  const desktop = $derived(desktopMedia.current);
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
    void tick().then(() => findPanel?.focus());
  }
  function closeFind() {
    t.clearFind();
    findOpen = false;
    if (document.activeElement instanceof HTMLElement)
      document.activeElement.blur();
    if (!hasTouchInput())
      void tick().then(() => {
        if (findReturnFocus?.isConnected) findReturnFocus.focus();
        else focusInput();
      });
  }
  // Loaded on first use; the type-only import keeps it out of this chunk.
  let CodePanel = $state<typeof CodePanelComponent>();
  let loadingCode = false;
  // Fade a pane in as it is shown; hidden panes restart the animation.
  const paneEnter =
    "motion-safe:animate-in motion-safe:fade-in-0 motion-safe:duration-300";
  // A hybrid session off its terminal side keeps xterm mounted but hidden.
  const terminalShown = () =>
    hybrid.view === "terminal" || hybrid.view === "recovery";
  const terminalVisible = () => (desktop || !codeShown) && terminalShown();
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
  function insertReference(text: string) {
    // The chat input is the session's draft text.
    if (hybrid.view === "chat")
      return drafts.insert(draft, text, {
        ...drafts.begin(draft),
        startedInDraft: true,
      });
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
    if (draft.enabled) composer?.focus();
    else t.focus();
  }
  function toggleDraft() {
    drafts.setEnabled(draft, !draft.enabled);
    t.resetModifiers();
    if (draft.enabled && !hasTouchInput()) {
      void tick().then(() => composer?.focus());
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
  function runQuickCommand(cmd: QuickCommand) {
    t.resetModifiers();
    if (cmd.enter && drafts.destination(draft) === "terminal") {
      // Text and Enter form one acknowledged submission.
      const sent = t.submit(newSubmissionId(), cmd.command, true, (state) => {
        if (!mounted) return;
        if (quickMessages[state]) error = quickMessages[state];
        else if (state === "written" && error === quickMessages.unknown)
          error = "";
      });
      error = sent ? "" : "终端未就绪，快捷指令未发送；请恢复连接后重试。";
      return;
    }
    const result = insertText(cmd.command);
    if (result === "failed")
      error = "终端未就绪，快捷指令未发送；请恢复连接后重试。";
    else error = "";
  }
  const viewport = useViewport();
  const keyboardDismissed = () => hasTouchInput() && viewport.keyboardDismissed;
  let sizeLocked = $state(stored("sizeLocked") === "1");
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
      focusDraft: () => composer?.focus(),
      key: (event) => composer?.terminalKey(event),
      insertText: (text) => {
        insertText(text);
      },
    },
    keyboardDismissed,
    () => sizeLocked,
  );
  function setSizeLocked(value: boolean) {
    sizeLocked = value;
    store("sizeLocked", value ? "1" : "0");
    void tick().then(() => t.fit());
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
  );
  // Chat uploads live on this page, not in ChatView, so switching sides does
  // not abort an upload; the finished path lands in the shared draft.
  const chatUploads = useChatUpload(
    untrack(() => id),
    untrack(() => drafts),
    draft,
    (message) => {
      if (mounted) error = message;
      else if (message) onUploadNotice(`会话 ${id}：${message}`);
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
    codeShown = true;
    codeTarget = { path, key: (codeTarget?.key ?? 0) + 1 };
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
    // Application OSC 52 copies have no user gesture. writeText still succeeds
    // in a secure context when clipboard-write is allowed; otherwise the
    // existing text box is the manual copy fallback.
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
    copyText = text;
    copyOpen = true;
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
          if (draft.enabled && !hasTouchInput()) composer?.focus();
        } else if (drafts.destination(draft, operation) !== "failed") {
          pasteOperation = operation;
          pasteText = "";
          pasteOpen = true;
        }
      }
    }
  }
  async function focusTerminal(event: Event) {
    event.preventDefault();
    if (hasTouchInput()) return;
    await tick();
    if (findOpen) {
      findPanel?.focus();
      return;
    }
    focusInput();
  }
</script>

{#snippet quickMenu(side: "top" | "bottom")}
  <QuickCommandMenu
    commands={quickCommands.list}
    target={draft.enabled ? "draft" : "terminal"}
    unavailable={quickUnavailable}
    disabled={t.connection === "missing"}
    {side}
    run={runQuickCommand}
    manage={() => navigate(`/settings?session=${id}#quick-commands`)}
    onCloseAutoFocus={focusTerminal}
  />
{/snippet}

{#snippet recovery()}
  {#if hybrid.recoverable}<HybridRecovery
      record={hybrid.recoverable}
      onresumed={reopen}
    />{/if}
{/snippet}

<main
  class="terminal-page"
  class:force-soft-keys={forceSoftKeys}
  class:ios={isIOS()}
  style:--viewport-height={`${viewport.height}px`}
>
  <TerminalHeader
    terminal={headerTerminal}
    {fullscreen}
    {draft}
    {draftSaved}
    {scratchSaved}
    uploading={uploads.progress !== null}
    {desktop}
    {sizeLocked}
    {setSizeLocked}
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
    hybrid={hybrid.known ? hybrid : undefined}
  />
  <TerminalNotices
    terminal={t}
    {uploads}
    {draft}
    {draftSaved}
    {error}
    bind:notice
    {showScratch}
    dismissScratch={() => (draft.scratchUpdatedWhileHidden = false)}
    remove={closeSession}
    notices={hybrid.notices}
    dismiss={(key) => hybrid.dismiss(key)}
    recovery={hybrid.recoverable ? recovery : undefined}
  />
  <TerminalWorkspace
    {codeShown}
    {desktop}
    draftOpen={draft.enabled && hybrid.view === "terminal"}
    onfind={showFind}
  >
    {#snippet terminalPane()}
      <!-- A hybrid session's panes fade in as a switch lands on them. -->
      <div
        class={["h-full", hybrid.known && paneEnter]}
        hidden={!terminalShown()}
      >
        <TerminalViewport
          bind:container
          terminal={t}
          visible={terminalVisible()}
          {sizeLocked}
          {findOpen}
          {uploads}
          {quickMenu}
          send={softInput}
          toggle={(key) => {
            if (draft.enabled) controlNotice();
            else t.toggleModifier(key);
          }}
          {paste}
          keyboard={() => {
            if (draft.enabled) composer?.keyboard();
            else t.keyboard();
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
        <div class={["h-full", paneEnter]}>
          <ChatView
            model={hybrid.chat.model}
            chat={hybrid.chat.state}
            connection={hybrid.chat.connection}
            {draft}
            {drafts}
            uploads={chatUploads}
            uploadUnavailable={chatUploadUnavailable}
            actions={hybrid.actions}
            onreview={review}
            onlogin={() => hybrid.request("tui")}
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
    {/snippet}
    {#snippet draftPane()}
      <TerminalInputComposer
        bind:this={composer}
        {draft}
        {drafts}
        {keyboardDismissed}
        unavailable={sendUnavailable}
        saved={draftSaved}
        target={t.info?.title || id}
        openScratch={showScratch}
        send={() => sendDraft()}
        resend={() => sendDraft(true)}
        release={releaseDraftSubmission}
        filePaste={uploads.filePaste}
        {controlNotice}
      />
    {/snippet}
    {#snippet codePane()}
      {#if CodePanel}<CodePanel
          {id}
          visible={codeShown}
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
</main>

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
      navigate("/");
    }}
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
<SwitchChoiceDialog
  bind:open={
    () => hybrid.awaitingChoice && !choiceClosed,
    (open) => (choiceClosed = !open)
  }
  agent={hybrid.chat.state?.agent ?? ""}
  busy={hybrid.answering}
  onchoose={(choice) => void hybrid.choose(choice)}
/>
<ScratchPanel bind:open={scratchOpen} {draft} {drafts} saved={scratchSaved} />
