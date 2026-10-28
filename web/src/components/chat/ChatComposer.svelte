<script lang="ts">
  import IconArrowUp from "~icons/tabler/arrow-up";
  import IconMessageBolt from "~icons/tabler/message-bolt";
  import IconPlayerStopFilled from "~icons/tabler/player-stop-filled";
  import { Button } from "$lib/components/ui/button";
  import { ListButton } from "$lib/components/ui/list-button";
  import { Textarea } from "$lib/components/ui/textarea";
  import type { ChatActions } from "../../chat-api";
  import { errorMessage } from "../../error";
  import type {
    ChatCommand,
    ChatPermissionMode as PermissionMode,
    ChatState,
  } from "../../schemas";
  import type {
    TerminalDraft,
    TerminalDrafts,
  } from "../../terminal-draft.svelte";
  import { hasTouchInput } from "../../touchInput";
  import ChatConfig from "./ChatConfig.svelte";
  import ChatPermissionMode from "./ChatPermissionMode.svelte";
  import UsageRing from "./UsageRing.svelte";

  // The chat input. Its text is the session's draft, saved on this device
  // like the terminal draft. Sending while a turn runs queues on the server;
  // "插话" appears only when the agent can take text into the running turn.
  // "/" opens the agent's commands.
  let {
    draft,
    drafts,
    chat,
    unavailable,
    actions,
    onerror,
  }: {
    draft: TerminalDraft;
    drafts: TerminalDrafts;
    chat: ChatState;
    /** Why input is disabled now (switching, offline); "" when enabled. */
    unavailable: string;
    actions: ChatActions;
    onerror: (message: string) => void;
  } = $props();

  let textarea = $state<HTMLTextAreaElement | null>(null);
  let sending = $state(false);
  // A permission mode change is on its way; the select waits for it.
  let savingMode = $state(false);
  let highlighted = $state(0);
  // Escape hides the menu for the text it was open on.
  let menuDismissed = $state<string | null>(null);
  const running = $derived(chat.turn.running);
  const typed = $derived(draft.text.trim() !== "");
  const locked = $derived(!!draft.submission);
  const disabled = $derived(!!unavailable || locked);
  const steerable = $derived(running && chat.steering);
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
    if (!text || sending || disabled) return;
    sending = true;
    drafts.edit(draft, "", 0);
    try {
      await actions.send(text, mode);
      onerror("");
    } catch (err) {
      // Give the text back unless the user already typed something new.
      if (!draft.text) drafts.edit(draft, text, text.length);
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
      if ((event.key === "Enter" || event.key === "Tab") && command) {
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
    if (event.key !== "Enter" || event.shiftKey) return;
    const steer = event.ctrlKey || event.metaKey;
    // On touch keyboards Enter starts a new line; the button sends.
    if (!steer && hasTouchInput()) return;
    event.preventDefault();
    void submit(steer && steerable ? "steer" : "auto");
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
</script>

<div class="space-y-1.5" data-chat-composer>
  <!-- The card shows the message box's focus once; the box itself has none. -->
  <div
    class="rounded-xl border bg-card shadow-sm transition has-[textarea:focus]:border-ring has-[textarea:focus]:ring-3 has-[textarea:focus]:ring-ring/20"
  >
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
    <div class="px-2 pt-1">
      <Textarea
        bind:ref={textarea}
        variant="bare"
        rows={2}
        class="max-h-48 min-h-14 resize-none"
        placeholder={unavailable ||
          (locked
            ? "草稿正在提交到终端，确认结果前不能编辑"
            : queueing
              ? "继续输入，发送后加入排队"
              : "给 Agent 发消息，输入 / 查看命令")}
        aria-label="消息"
        aria-controls={menuOpen ? "chat-commands" : undefined}
        aria-expanded={menuOpen}
        {disabled}
        value={draft.text}
        oninput={(event) => {
          const input = event.currentTarget;
          drafts.edit(draft, input.value, input.selectionStart);
          highlighted = 0;
        }}
        onkeydown={keydown}
      />
    </div>
    <!-- Only what acts on the message stays in the card: context use and the
       send controls (they wrap only at very large text). -->
    <div class="flex flex-wrap items-center justify-end gap-1 px-2 pb-2">
      {#if chat.usage}<UsageRing usage={chat.usage} />{/if}
      {#if steerable}
        <Button
          variant="outline"
          size="icon-touch"
          aria-label="插话"
          title="插话：注入当前回合（Ctrl+Enter）"
          disabled={disabled || !typed || sending}
          onclick={() => submit("steer")}
          ><IconMessageBolt aria-hidden="true" /></Button
        >
      {/if}
      {#if running}
        <Button
          variant="secondary"
          size="icon-touch"
          aria-label="停止"
          title="停止当前回合"
          disabled={!!unavailable}
          onclick={stop}><IconPlayerStopFilled aria-hidden="true" /></Button
        >
      {/if}
      <Button
        size="icon-touch"
        aria-label={queueing ? "加入排队" : "发送"}
        title={queueing ? "加入排队" : "发送"}
        disabled={disabled || !typed || sending}
        onclick={() => submit("auto")}
        ><IconArrowUp aria-hidden="true" /></Button
      >
    </div>
  </div>
  <!-- Session options sit under the card; on a narrow screen they wrap
       there instead of crowding the message box. -->
  <div
    class="flex min-w-0 flex-wrap items-center gap-x-0.5 px-1"
    role="toolbar"
    aria-label="会话选项"
  >
    <ChatPermissionMode
      mode={chat.permissionMode}
      disabled={!!unavailable || savingMode}
      onchange={(mode) => void setPermissionMode(mode)}
    />
    <ChatConfig
      config={chat.config}
      {disabled}
      onchange={(configId, value) => {
        actions.setConfig(configId, value).then(
          () => onerror(""),
          (err: unknown) => onerror(errorMessage(err)),
        );
      }}
    />
  </div>
</div>
