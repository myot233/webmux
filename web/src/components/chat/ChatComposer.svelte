<script lang="ts">
  import { horizontalWheel } from "../../hooks/shared/horizontal-wheel";
  import type { Snippet } from "svelte";
  import {
    Button,
    type ButtonSize,
    type ButtonVariant,
  } from "$lib/components/ui/button";
  import { ListButton } from "$lib/components/ui/list-button";
  import {
    requestNextPrompt,
    type ChatActions,
    type ConfigValue,
  } from "../../platform/chat-api";
  import type { TimelineEntry } from "../../domains/chat/chat-model";
  import { errorMessage } from "../../platform/error";
  import { useTextareaDraft } from "../../hooks/terminal/textarea-draft.svelte";
  import type { ChatUploads } from "../../hooks/shared/upload.svelte";
  import type {
    ChatCommand,
    ChatPermissionMode as PermissionMode,
    ChatState,
    SessionInfo,
  } from "../../contracts/schemas";
  import type {
    TerminalDraft,
    TerminalDrafts,
  } from "../../domains/terminal/terminal-draft.svelte";
  import {
    canSuggestNextPrompt,
    nextPromptPreference,
  } from "../../domains/chat/next-prompt.svelte";
  import { enterSendIntent } from "../../hooks/shared/enter-send";
  import ChatConfig from "./ChatConfig.svelte";
  import ChatLocation from "./ChatLocation.svelte";
  import ChatPermissionMode from "./ChatPermissionMode.svelte";
  import ChatSessionOptions from "./ChatSessionOptions.svelte";
  import ChatUploadButton from "./ChatUploadButton.svelte";
  import ComposerCard from "./ComposerCard.svelte";
  import UsageRing from "./UsageRing.svelte";

  // The chat input. Its text is the session's draft, saved on this device
  // like the terminal draft. Sending while a turn runs queues on the server;
  // Ctrl/Cmd+Enter injects into the running turn when the agent takes it, and
  // the send button stops the turn while there is nothing to send.
  // "/" opens the agent's commands.
  let {
    draft,
    drafts,
    chat,
    entries,
    unavailable,
    uploads,
    uploadUnavailable,
    location: location_,
    actions,
    onerror,
    quickMenu,
  }: {
    draft: TerminalDraft;
    drafts: TerminalDrafts;
    chat: ChatState;
    entries: readonly TimelineEntry[];
    /** Why input is disabled now (switching, offline); "" when enabled. */
    unavailable: string;
    uploads: ChatUploads;
    /** Why uploads are refused (session gone or exited); "" otherwise. */
    uploadUnavailable: string;
    /** Where the session runs, shown read-only above the input. */
    location?: Pick<SessionInfo, "workspace" | "cwd">;
    actions: ChatActions;
    onerror: (message: string) => void;
    quickMenu?: Snippet<["top" | "bottom", ButtonSize?, ButtonVariant?]>;
  } = $props();

  let textarea = $state<HTMLTextAreaElement | null>(null);
  let card = $state<ComposerCard>();
  let sending = $state(false);
  const selection = useTextareaDraft(
    () => textarea,
    () => draft,
    () => drafts,
  );
  // A permission mode change is on its way; the select waits for it.
  let savingMode = $state(false);
  let optionError = $state("");
  let highlighted = $state(0);
  let recalledId = "";
  // Escape hides the menu for the text it was open on.
  let menuDismissed = $state<string | null>(null);
  let observedTurn = $state<{
    session: string;
    turn: string;
    startedAt?: string;
  }>();
  let unavailableChat: ChatState | undefined;
  $effect(() => {
    const session = draft.sessionId;
    const { running, turnId, startedAt } = chat.turn;
    if (unavailable) {
      // A reopened socket can be connected before its fresh snapshot arrives.
      // Do not mistake the retained pre-disconnect state for a live turn.
      unavailableChat = chat;
      observedTurn = undefined;
      return;
    }
    if (observedTurn && observedTurn.session !== session)
      observedTurn = undefined;
    if (
      chat !== unavailableChat &&
      running &&
      turnId &&
      (observedTurn?.session !== session ||
        observedTurn.turn !== turnId ||
        observedTurn.startedAt !== startedAt)
    ) {
      observedTurn = { session, turn: turnId, startedAt };
    }
  });
  const running = $derived(chat.turn.running);
  const typed = $derived(draft.text.trim() !== "");
  const ready = $derived(typed || uploads.attachments.length > 0);
  const imageDegraded = $derived(
    !chat.promptCapabilities.image &&
      uploads.attachments.some((item) => item.kind === "image"),
  );
  const locked = $derived(!!draft.submission);
  const disabled = $derived(!!unavailable || locked);
  const uploading = $derived(uploads.progress !== null);
  const steerable = $derived(running && chat.steering);
  // With nothing to send the send button stops the running turn.
  const stopping = $derived(running && !ready && !uploading && !sending);
  const queueing = $derived(running || chat.queue.paused);
  const query = $derived(
    /^\/\S*$/.test(draft.text) ? draft.text.slice(1).toLowerCase() : null,
  );
  const matches = $derived(
    query === null
      ? []
      : chat.commands.filter((command) =>
          command.name.toLowerCase().startsWith(query),
        ),
  );
  const menuOpen = $derived(
    !disabled && matches.length > 0 && menuDismissed !== draft.text,
  );
  const active = $derived(Math.min(highlighted, matches.length - 1));

  const suggestionKey = $derived(
    chat.turn.turnId && chat.turn.endedAt
      ? `${chat.turn.turnId}:${chat.turn.endedAt}`
      : undefined,
  );
  const canSuggest = $derived(
    nextPromptPreference.enabled &&
      observedTurn?.session === draft.sessionId &&
      observedTurn.turn === chat.turn.turnId &&
      observedTurn.startedAt === chat.turn.startedAt &&
      suggestionKey &&
      !nextPromptPreference.dismissed(draft.sessionId, suggestionKey) &&
      !draft.text &&
      !ready &&
      !running &&
      !uploading &&
      !sending &&
      !disabled &&
      !chat.queue.items.length &&
      canSuggestNextPrompt(chat),
  );
  let generated = $state<{ session: string; key: string; text: string }>();
  $effect(() => {
    const { turnId, endedAt } = chat.turn;
    const session = draft.sessionId;
    const key = suggestionKey;
    if (!canSuggest || !key || !turnId || !endedAt) return;
    if (generated?.session === session && generated.key === key) return;

    const controller = new AbortController();
    void requestNextPrompt(session, turnId, endedAt, controller.signal).then(
      ({ text }) => {
        if (!controller.signal.aborted) generated = { session, key, text };
      },
      () => {
        if (!controller.signal.aborted) generated = { session, key, text: "" };
      },
    );
    return () => controller.abort();
  });
  const suggestion = $derived(
    canSuggest &&
      generated?.session === draft.sessionId &&
      generated.key === suggestionKey
      ? generated.text
      : "",
  );
  function dismissSuggestion() {
    if (suggestionKey)
      nextPromptPreference.dismiss(draft.sessionId, suggestionKey);
  }
  function acceptSuggestion() {
    if (!suggestion) return;
    drafts.edit(draft, suggestion, suggestion.length);
    dismissSuggestion();
    textarea?.focus();
  }

  async function submit(mode: "auto" | "steer") {
    const text = draft.text.trim();
    if (!ready || sending || disabled || uploading) return;
    recalledId = "";
    sending = true;
    drafts.edit(draft, "", 0);
    const sent = uploads.take();
    try {
      await actions.send(text, mode, sent);
      uploads.commit(sent);
      onerror("");
    } catch (err) {
      // Keep both the failed message and anything typed while it was sent.
      const current = draft.text;
      const restored =
        text && current ? `${text}\n${current}` : text || current;
      drafts.edit(draft, restored, text.length);
      uploads.restore(sent);
      onerror(errorMessage(err));
    } finally {
      sending = false;
    }
  }
  function pick(command: ChatCommand) {
    recalledId = "";
    const text = `/${command.name} `;
    drafts.edit(draft, text, text.length);
    textarea?.focus();
  }
  function recall(event: KeyboardEvent) {
    const older = event.key === "ArrowUp";
    const input = textarea;
    if (
      (!older && event.key !== "ArrowDown") ||
      !input ||
      (draft.text ? !recalledId : !older) ||
      disabled ||
      uploading ||
      sending ||
      uploads.attachments.length > 0 ||
      event.altKey ||
      event.ctrlKey ||
      event.metaKey ||
      event.shiftKey ||
      input.selectionStart !== input.selectionEnd
    )
      return;
    // At an interior caret position the browser owns visual line navigation.
    if (input.selectionStart !== (older ? 0 : draft.text.length)) return;
    const history = entries.flatMap((entry) =>
      entry.type === "message" &&
      entry.role === "user" &&
      entry.text &&
      !entry.truncated
        ? [entry]
        : [],
    );
    const current = draft.text
      ? history.findIndex(
          (entry) => entry.id === recalledId && entry.text === draft.text,
        )
      : history.length;
    if (current < 0) return;
    const next = current + (older ? -1 : 1);
    if (next < 0 || next > history.length) return;
    const entry = history[next];
    const text = entry?.text ?? "";
    recalledId = entry?.id ?? "";
    menuDismissed = text;
    event.preventDefault();
    drafts.edit(draft, text, older ? 0 : text.length);
  }
  function keydown(event: KeyboardEvent) {
    if (event.isComposing || event.keyCode === 229 || selection.composing)
      return;
    if (menuOpen) {
      const command = matches[active];
      if (event.key === "ArrowDown" || event.key === "ArrowUp") {
        event.preventDefault();
        const step = event.key === "ArrowDown" ? 1 : -1;
        highlighted = (active + step + matches.length) % matches.length;
        return;
      }
      if (event.key === "Tab" && command) {
        event.preventDefault();
        pick(command);
        return;
      }
      if (event.key === "Escape") {
        event.preventDefault();
        menuDismissed = draft.text;
        return;
      }
    }
    const intent = enterSendIntent(event);
    if (!intent) {
      recall(event);
      return;
    }
    event.preventDefault();
    void submit(intent === "steer" && steerable ? "steer" : "auto");
  }
  async function stop() {
    try {
      await actions.cancel();
    } catch (err) {
      onerror(errorMessage(err));
    }
  }
  async function setPermissionMode(mode: PermissionMode) {
    savingMode = true;
    optionError = "";
    try {
      await actions.setPermissionMode(mode);
      reportOptionError("");
    } catch (err) {
      reportOptionError(errorMessage(err));
    } finally {
      savingMode = false;
    }
  }
  function setConfig(configId: string, value: ConfigValue) {
    optionError = "";
    actions.setConfig(configId, value).then(
      () => reportOptionError(""),
      (err: unknown) => reportOptionError(errorMessage(err)),
    );
  }
  function reportOptionError(message: string) {
    optionError = message;
    onerror(message);
  }
  export function focus() {
    card?.focus();
  }
</script>

<ComposerCard
  bind:this={card}
  bind:textarea
  value={draft.text}
  oninput={(event) => {
    if (recalledId) menuDismissed = null;
    recalledId = "";
    const input = event.currentTarget;
    drafts.edit(draft, input.value, input.selectionStart, input.selectionEnd);
    highlighted = 0;
  }}
  placeholder={unavailable ||
    (locked
      ? "草稿正在提交到终端，确认结果前不能编辑"
      : queueing
        ? "继续输入，发送后加入排队"
        : "给 Agent 发消息，输入 / 查看命令")}
  label="消息"
  {disabled}
  {uploads}
  uploadsDisabled={sending}
  busy={Boolean(
    draft.text || uploads.attachments.length > 0 || uploading || sending,
  )}
  action={stopping
    ? {
        kind: "stop",
        label: "停止",
        title: "停止当前回合",
        disabled: !!unavailable,
        onclick: stop,
      }
    : {
        kind: "send",
        label: queueing ? "加入排队" : "发送",
        disabled: disabled || !ready || sending || uploading,
        onclick: () => submit("auto"),
      }}
  textareaProps={{
    "aria-controls": menuOpen ? "chat-commands" : undefined,
    "aria-expanded": menuOpen,
    ...selection.selectionProps,
    onkeydown: keydown,
  }}
>
  {#snippet location({ compact })}
    {#if location_}
      <ChatLocation
        workspace={location_.workspace}
        cwd={location_.cwd}
        class={compact ? "composer-mobile:hidden" : ""}
      />
    {/if}
  {/snippet}
  {#snippet menu()}
    {#if menuOpen}
      <div
        id="chat-commands"
        class="max-h-52 overflow-y-auto border-b p-1"
        role="listbox"
        aria-label="命令"
      >
        {#each matches as command, i (command.name)}
          <ListButton
            variant="suggestion"
            role="option"
            aria-selected={i === active}
            class="flex w-full min-w-0 flex-wrap items-baseline"
            onmousedown={(event) => event.preventDefault()}
            onclick={() => pick(command)}
          >
            <span class="shrink-0 font-mono text-sm">/{command.name}</span>
            {#if command.hint}<span
                class="min-w-0 break-words text-muted-foreground"
                >&lt;{command.hint}&gt;</span
              >{/if}
            <span class="ml-auto min-w-0 truncate text-muted-foreground"
              >{command.description}</span
            >
          </ListButton>
        {/each}
      </div>
    {/if}
  {/snippet}
  {#snippet hints()}
    {#if suggestion}
      <div class="flex flex-wrap items-center gap-2 px-2 py-1">
        <Button
          variant="ghost"
          size="xs"
          class="max-w-full"
          title={suggestion}
          onclick={acceptSuggestion}
        >
          <span class="truncate text-left">{suggestion}</span>
        </Button>
        <Button variant="ghost" size="xs" onclick={dismissSuggestion}
          >忽略</Button
        >
      </div>
    {/if}
  {/snippet}
  {#snippet notices()}
    {#if imageDegraded}
      <p class="border-b px-3 py-1.5 text-xs text-warning" role="status">
        该 agent 不支持图片，图片将按服务器路径发送。
      </p>
    {/if}
  {/snippet}
  {#snippet leading()}
    <ChatUploadButton {uploads} {disabled} unavailable={uploadUnavailable} />
    <span class="hidden composer-mobile:contents">
      <ChatSessionOptions
        mode={chat.permissionMode}
        config={chat.config}
        error={optionError}
        {disabled}
        modeDisabled={!!unavailable || savingMode}
        onmodechange={(mode) => void setPermissionMode(mode)}
        onconfigchange={setConfig}
      />
    </span>
  {/snippet}
  {#snippet accessories({ size, variant })}
    {#if chat.usage}<UsageRing usage={chat.usage} />{/if}
    {@render quickMenu?.("top", size, variant)}
  {/snippet}
  {#snippet below()}
    <!-- Scroll the desktop row when space runs out so the option labels
         keep their width. On mobile the sheet lists the options. -->
    <div
      class="-my-1 flex min-w-0 flex-nowrap items-center gap-0.5 overflow-x-auto px-1 py-1 [scrollbar-width:none] *:shrink-0 composer-mobile:hidden"
      use:horizontalWheel
      role="toolbar"
      aria-label="会话选项"
    >
      <ChatPermissionMode
        mode={chat.permissionMode}
        disabled={!!unavailable || savingMode}
        onchange={(mode) => void setPermissionMode(mode)}
      />
      <ChatConfig config={chat.config} {disabled} onchange={setConfig} />
    </div>
  {/snippet}
</ComposerCard>
