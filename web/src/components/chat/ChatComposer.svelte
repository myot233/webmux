<script lang="ts">
  import type { Snippet } from "svelte";
  import type { ButtonSize, ButtonVariant } from "$lib/components/ui/button";
  import { ListButton } from "$lib/components/ui/list-button";
  import type { ChatActions, ConfigValue } from "../../chat-api";
  import { errorMessage } from "../../error";
  import { useTextareaDraft } from "../../hooks/textarea-draft.svelte";
  import type { ChatUploads } from "../../hooks/upload.svelte";
  import type {
    ChatCommand,
    ChatPermissionMode as PermissionMode,
    ChatState,
    SessionInfo,
  } from "../../schemas";
  import type {
    TerminalDraft,
    TerminalDrafts,
  } from "../../terminal-draft.svelte";
  import { enterSendIntent } from "../../enter-send";
  import ChatConfig from "./ChatConfig.svelte";
  import ChatLocation from "./ChatLocation.svelte";
  import ChatOptionsBar from "./ChatOptionsBar.svelte";
  import ChatPermissionMode from "./ChatPermissionMode.svelte";
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
  let highlighted = $state(0);
  // Escape hides the menu for the text it was open on.
  let menuDismissed = $state<string | null>(null);
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

  async function submit(mode: "auto" | "steer") {
    const text = draft.text.trim();
    if (!ready || sending || disabled || uploading) return;
    sending = true;
    drafts.edit(draft, "", 0);
    const sent = uploads.take();
    try {
      await actions.send(text, mode, sent);
      onerror("");
    } catch (err) {
      // Give the text and attachments back; keep anything typed since.
      if (!draft.text) drafts.edit(draft, text, text.length);
      uploads.restore(sent);
      onerror(errorMessage(err));
    } finally {
      sending = false;
    }
  }
  function pick(command: ChatCommand) {
    const text = `/${command.name} `;
    drafts.edit(draft, text, text.length);
    textarea?.focus();
  }
  function keydown(event: KeyboardEvent) {
    if (event.isComposing) return;
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
    if (!intent) return;
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
    try {
      await actions.setPermissionMode(mode);
      onerror("");
    } catch (err) {
      onerror(errorMessage(err));
    } finally {
      savingMode = false;
    }
  }
  function setConfig(configId: string, value: ConfigValue) {
    actions.setConfig(configId, value).then(
      () => onerror(""),
      (err: unknown) => onerror(errorMessage(err)),
    );
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
        class={compact ? "max-sm:hidden" : ""}
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
  {#snippet notices()}
    {#if imageDegraded}
      <p class="border-b px-3 py-1.5 text-xs text-warning" role="status">
        该 agent 不支持图片，图片将按服务器路径发送。
      </p>
    {/if}
  {/snippet}
  {#snippet leading()}
    <ChatUploadButton {uploads} {disabled} unavailable={uploadUnavailable} />
  {/snippet}
  {#snippet accessories({ size, variant })}
    {#if chat.usage}<UsageRing usage={chat.usage} />{/if}
    {@render quickMenu?.("top", size, variant)}
  {/snippet}
  {#snippet below()}
    <ChatOptionsBar label="会话选项">
      <ChatPermissionMode
        mode={chat.permissionMode}
        disabled={!!unavailable || savingMode}
        onchange={(mode) => void setPermissionMode(mode)}
      />
      <ChatConfig config={chat.config} {disabled} onchange={setConfig} />
    </ChatOptionsBar>
  {/snippet}
</ComposerCard>
