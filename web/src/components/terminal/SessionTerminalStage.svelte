<script lang="ts">
  import type { Snippet } from "svelte";
  import type { ButtonSize, ButtonVariant } from "$lib/components/ui/button";
  import { paneEnter } from "../../hooks/shared/pane-enter";
  import type { HybridSession } from "../../hooks/sessions/hybrid.svelte";
  import type { TerminalController } from "../../hooks/terminal/terminal.svelte";
  import type {
    ChatUploads,
    useUpload,
  } from "../../hooks/shared/upload.svelte";
  import type { ChatCodeLink } from "../../domains/code/markdown-link";
  import { quickCommands } from "../../domains/settings/quick-commands.svelte";
  import type { QuickCommand, SessionInfo } from "../../contracts/schemas";
  import { isVoiceDisabled } from "../../domains/terminal/session-terminal-io";
  import type {
    TerminalDraft,
    TerminalDrafts,
  } from "../../domains/terminal/terminal-draft.svelte";
  import ConnectorInstallHint from "../sessions/ConnectorInstallHint.svelte";
  import DraftToggle from "./DraftToggle.svelte";
  import HybridRecovery from "../sessions/HybridRecovery.svelte";
  import QuickCommandMenu from "../settings/QuickCommandMenu.svelte";
  import TerminalFindBar from "./TerminalFindBar.svelte";
  import TerminalHeader from "./TerminalHeader.svelte";
  import TerminalInputComposer from "./TerminalInputComposer.svelte";
  import TerminalNotices from "./TerminalNotices.svelte";
  import TerminalViewport from "./TerminalViewport.svelte";
  import TerminalWorkspace from "./TerminalWorkspace.svelte";
  import VoiceButton from "../shared/VoiceButton.svelte";
  import ChatView from "../chat/ChatView.svelte";
  import SwitchProgress from "../chat/SwitchProgress.svelte";
  import type { createHeaderTerminalView } from "../../domains/terminal/session-terminal-io";
  import type { useFullscreen } from "../../hooks/shared/fullscreen.svelte";

  let {
    id,
    paneActions,
    headerTerminal,
    fullscreen,
    codeShown = $bindable(),
    forceSoftKeys = $bindable(),
    desktop,
    documentScroll,
    hybrid,
    terminal,
    uploads,
    chatUploads,
    chatUploadUnavailable,
    sendUnavailable,
    quickUnavailable,
    voiceEnabled,
    draft,
    drafts,
    draftSaved,
    scratchSaved,
    error,
    pendingCopy,
    copyPending,
    dismissCopy,
    notice = $bindable(),
    showScratch,
    showFontSize,
    closeSession,
    closing,
    reopen,
    leave,
    feedSession,
    showForwards,
    terminalShown,
    terminalVisible,
    sizeLocked,
    setSizeLocked,
    keyboardSizeHold,
    setKeyboardSizeHold,
    keyboardHeld,
    touchGamepad,
    setTouchGamepad,
    findOpen,
    showFind,
    closeFind,
    keyboardDismissed,
    toggleDraft,
    sendDraft,
    releaseDraftSubmission,
    runQuickCommand,
    manageQuickCommands,
    focusTerminal,
    insertVoiceResult,
    onVoiceError,
    softInput,
    controlNotice,
    copy,
    paste,
    holdBegin,
    codePane,
    review,
    openCodeLocation,
    onChatEditChange,
    onChooseSwitch,
    container = $bindable(),
  }: {
    id: string;
    paneActions: Snippet<[boolean]>;
    headerTerminal: ReturnType<typeof createHeaderTerminalView>;
    fullscreen: ReturnType<typeof useFullscreen>;
    codeShown: boolean;
    forceSoftKeys: boolean;
    desktop: boolean;
    documentScroll: boolean;
    hybrid: HybridSession;
    terminal: TerminalController;
    uploads: ReturnType<typeof useUpload>;
    chatUploads: ChatUploads;
    chatUploadUnavailable: string;
    sendUnavailable: string;
    quickUnavailable: string;
    voiceEnabled: boolean;
    draft: TerminalDraft;
    drafts: TerminalDrafts;
    draftSaved: string;
    scratchSaved: string;
    error: string;
    pendingCopy: boolean;
    copyPending: () => void;
    dismissCopy: () => void;
    notice: string;
    showScratch: () => void;
    showFontSize: () => void;
    closeSession: () => void;
    closing: boolean;
    reopen: () => void;
    leave: () => void;
    feedSession?: SessionInfo;
    showForwards: () => void;
    terminalShown: boolean;
    terminalVisible: boolean;
    sizeLocked: boolean;
    setSizeLocked: (value: boolean) => void;
    keyboardSizeHold: boolean;
    setKeyboardSizeHold: (value: boolean) => void;
    keyboardHeld: boolean;
    touchGamepad: boolean;
    setTouchGamepad: (value: boolean) => void;
    findOpen: boolean;
    showFind: () => void;
    closeFind: () => void;
    keyboardDismissed: () => boolean;
    toggleDraft: () => void;
    sendDraft: (resend?: boolean) => void;
    releaseDraftSubmission: () => void;
    runQuickCommand: (cmd: QuickCommand) => void;
    manageQuickCommands: () => void;
    focusTerminal: (event: Event) => void;
    insertVoiceResult: (text: string) => void;
    onVoiceError: (message: string) => void;
    softInput: (input: string) => void;
    controlNotice: () => void;
    copy: () => Promise<void>;
    paste: () => Promise<void>;
    holdBegin: () => void;
    codePane: Snippet;
    review: (path: string) => void;
    openCodeLocation: (target: ChatCodeLink) => void;
    onChatEditChange: (value: boolean) => void;
    onChooseSwitch: () => void;
    container: HTMLDivElement | undefined;
  } = $props();

  let findPanel = $state<TerminalFindBar>();
  let chatView = $state<ChatView>();
  let composer = $state<TerminalInputComposer>();

  export function focusFind() {
    findPanel?.focus();
  }
  export function focusChat() {
    chatView?.focus();
  }
  export function focusComposer() {
    composer?.focus();
  }
  export function editComposerKey(input: string) {
    composer?.editKey(input);
  }
  export function handleTerminalKey(event: KeyboardEvent) {
    return composer?.terminalKey(event);
  }
  export function openComposerKeyboard(
    beginHold: (target: EventTarget) => void,
  ) {
    composer?.keyboard(beginHold);
  }
  export function releaseChatEditing() {
    chatView?.releaseEditing();
  }
  export function setChatVoiceError(message: string): boolean {
    if (!chatView) return false;
    chatView.setVoiceError(message);
    return true;
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
    disabled={isVoiceDisabled(hybrid, terminal, draft)}
    onmousedown={(event) => {
      if (side === "top" && hybrid.view !== "chat") event.preventDefault();
    }}
    onresult={insertVoiceResult}
    onerror={onVoiceError}
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
      : terminal.connection === "missing"}
    {side}
    {size}
    {variant}
    run={runQuickCommand}
    manage={manageQuickCommands}
    onCloseAutoFocus={focusTerminal}
  />
{/snippet}
{#snippet draftToggle()}
  <DraftToggle
    class="sm:hidden"
    {draft}
    {draftSaved}
    {scratchSaved}
    disabled={terminal.connection === "missing"}
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
  {keyboardSizeHold}
  {setKeyboardSizeHold}
  bind:codeShown
  bind:forceSoftKeys
  {quickMenu}
  upload={uploads.uploadFiles}
  {toggleDraft}
  {copy}
  {paste}
  {showScratch}
  {showFontSize}
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
  {codePane}
>
  {#snippet terminalPane()}
    <div
      class={[
        "relative flex flex-col",
        documentScroll ? "min-h-0 flex-1" : "h-full",
      ]}
    >
      <div
        class={hybrid.view === "chat"
          ? "flex shrink-0 flex-col bg-background"
          : "absolute inset-x-0 top-0 z-30 flex flex-col bg-background"}
      >
        <TerminalNotices
          {leave}
          {terminal}
          {uploads}
          {draft}
          {draftSaved}
          {error}
          {pendingCopy}
          {copyPending}
          {dismissCopy}
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
      <div
        class="min-h-0 flex-1"
        use:paneEnter={hybrid.known && terminalShown}
        hidden={!terminalShown}
      >
        <TerminalViewport
          bind:container
          {terminal}
          visible={terminalVisible}
          sizeLocked={sizeLocked || keyboardHeld}
          {touchGamepad}
          {setTouchGamepad}
          {findOpen}
          {uploads}
          {quickMenu}
          {draftToggle}
          send={softInput}
          toggle={(key) => {
            if (draft.enabled) controlNotice();
            else terminal.toggleModifier(key);
          }}
          lock={(key) => {
            if (draft.enabled) controlNotice();
            else terminal.lockModifier(key);
          }}
          {paste}
          keyboard={holdBegin}
        >
          {#snippet findBar()}
            <TerminalFindBar
              bind:this={findPanel}
              open={findOpen}
              {terminal}
              onclose={closeFind}
            />
          {/snippet}
        </TerminalViewport>
      </div>
      {#if hybrid.view === "chat"}
        <div
          class={[
            "flex min-h-0 flex-1 flex-col",
            !documentScroll && "h-full overflow-hidden",
          ]}
          use:paneEnter={true}
        >
          <ChatView
            bind:this={chatView}
            sessionId={id}
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
            {documentScroll}
            oneditchange={onChatEditChange}
          />
        </div>
      {:else if hybrid.view === "switching"}
        <SwitchProgress
          target={hybrid.target}
          steps={hybrid.steps}
          awaitingChoice={hybrid.awaitingChoice}
          onchoose={onChooseSwitch}
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
      target={terminal.info?.title || id}
      openScratch={showScratch}
      send={() => sendDraft()}
      resend={() => sendDraft(true)}
      release={releaseDraftSubmission}
      filePaste={uploads.filePaste}
      {controlNotice}
      voice={voiceEnabled ? draftVoice : undefined}
    />
  {/snippet}
</TerminalWorkspace>
