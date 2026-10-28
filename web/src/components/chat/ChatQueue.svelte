<script lang="ts">
  import { MediaQuery } from "svelte/reactivity";
  import IconArrowBarUp from "~icons/tabler/arrow-bar-up";
  import IconArrowNarrowUp from "~icons/tabler/arrow-narrow-up";
  import IconChevronDown from "~icons/tabler/chevron-down";
  import IconCornerDownRight from "~icons/tabler/corner-down-right";
  import IconPlayerPause from "~icons/tabler/player-pause";
  import IconTrash from "~icons/tabler/trash";
  import { Button } from "$lib/components/ui/button";
  import { Textarea } from "$lib/components/ui/textarea";
  import ConfirmDialog from "../ConfirmDialog.svelte";
  import type { ChatActions } from "../../chat-api";
  import { errorMessage } from "../../error";
  import type { ChatState } from "../../schemas";

  // The server's prompt queue, shared by every device. Phones fold it to one
  // line with the count and the first prompt. After an interrupted turn it
  // pauses until the user continues or clears it.
  let {
    queue,
    running,
    disabled,
    actions,
    onerror,
  }: {
    queue: ChatState["queue"];
    running: boolean;
    disabled: boolean;
    actions: ChatActions;
    onerror: (message: string) => void;
  } = $props();

  let open = $state(new MediaQuery("(min-width: 640px)").current);
  let sendNow = $state<string | null>(null);
  let sendNowOpen = $state(false);

  function run(action: () => Promise<unknown>) {
    action().then(
      () => onerror(""),
      (err: unknown) => onerror(errorMessage(err)),
    );
  }
  function edit(itemId: string, before: string, text: string) {
    const next = text.trim();
    if (next && next !== before) run(() => actions.editQueued(itemId, next));
  }
</script>

<section class="rounded-xl border bg-card p-1 text-sm" aria-label="排队的消息">
  <div class="flex flex-wrap items-center gap-2 px-2 py-0.5">
    <Button
      variant="quiet"
      size="xs"
      class="max-w-full min-w-0"
      aria-expanded={open}
      onclick={() => (open = !open)}
      >排队 {queue.items.length}{#if !open}<span class="min-w-0 truncate"
          >· {queue.items[0]?.text}</span
        >{/if}<IconChevronDown
        class={open ? "rotate-180" : undefined}
        aria-hidden="true"
      /></Button
    >
    {#if queue.paused}
      <span class="flex items-center gap-1 text-xs text-warning" role="status">
        <IconPlayerPause
          class="size-3.5"
          aria-hidden="true"
        />回合已中断，队列暂停
      </span>
      <span class="ml-auto flex gap-1">
        <Button
          variant="outline"
          size="xs"
          {disabled}
          onclick={() => run(actions.resumeQueue)}>继续</Button
        >
        <Button
          variant="quiet"
          size="xs"
          {disabled}
          onclick={() => run(actions.clearQueue)}>清空</Button
        >
      </span>
    {/if}
  </div>
  <ol hidden={!open}>
    {#each queue.items as item, i (item.id)}
      <li class="flex flex-wrap items-center justify-end gap-x-1 pr-1 pl-2">
        <IconCornerDownRight
          class="size-3.5 shrink-0 text-muted-foreground"
          aria-hidden="true"
        />
        <Textarea
          variant="ghost"
          rows={1}
          class="min-h-0 min-w-0 flex-1 basis-40"
          aria-label={`编辑排队消息 ${i + 1}`}
          {disabled}
          value={item.text}
          onkeydown={(event) => {
            if (event.key !== "Enter" || event.shiftKey || event.isComposing)
              return;
            event.preventDefault();
            event.currentTarget.blur();
          }}
          onchange={(event) =>
            edit(item.id, item.text, event.currentTarget.value)}
        />
        {#if i > 0}
          <Button
            variant="quiet"
            size="icon-sm"
            aria-label="上移"
            title="上移"
            {disabled}
            onclick={() => run(() => actions.moveQueuedUp(item.id))}
            ><IconArrowNarrowUp aria-hidden="true" /></Button
          >
        {/if}
        <Button
          variant="quiet"
          size="icon-sm"
          aria-label="立即发送"
          title={running ? "立即发送（中断当前回合）" : "立即发送"}
          {disabled}
          onclick={() => {
            if (!running) {
              run(() => actions.sendQueuedNow(item.id));
              return;
            }
            sendNow = item.id;
            sendNowOpen = true;
          }}><IconArrowBarUp aria-hidden="true" /></Button
        >
        <Button
          variant="quiet"
          size="icon-sm"
          aria-label="删除"
          title="删除"
          {disabled}
          onclick={() => run(() => actions.removeQueued(item.id))}
          ><IconTrash aria-hidden="true" /></Button
        >
      </li>
    {/each}
  </ol>
</section>

<ConfirmDialog
  bind:open={sendNowOpen}
  title="立即发送？"
  onconfirm={() => {
    const itemId = sendNow;
    sendNowOpen = false;
    if (itemId) run(() => actions.sendQueuedNow(itemId));
  }}>当前回合会被中断，然后发送这条消息。</ConfirmDialog
>
