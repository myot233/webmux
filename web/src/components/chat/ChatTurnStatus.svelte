<script lang="ts">
  import IconCircleCheck from "~icons/tabler/circle-check";
  import IconHandStop from "~icons/tabler/hand-stop";
  import IconLoader2 from "~icons/tabler/loader-2";
  import IconX from "~icons/tabler/x";
  import { Button } from "$lib/components/ui/button";
  import ConnectorInstallHint from "../sessions/ConnectorInstallHint.svelte";
  import InlineNotice from "../shared/InlineNotice.svelte";
  import { outdatedText } from "../../domains/sessions/hybrid";
  import type { ChatModel } from "../../domains/chat/chat-model";
  import type { ChatRequest, ChatState } from "../../contracts/schemas";

  let {
    mode = "turn",
    truncated = false,
    error = "",
    unavailable = "",
    onterminal,
    chat,
    entries = [],
    request,
  }: {
    mode?: "notices" | "dock" | "turn";
    truncated?: boolean;
    error?: string;
    unavailable?: string;
    onterminal?: () => void;
    chat: ChatState | undefined;
    entries?: ChatModel["entries"];
    request?: ChatRequest | undefined;
  } = $props();

  let now = $state(Date.now());
  const turnStartedAt = $derived(
    chat?.turn.startedAt ? Date.parse(chat.turn.startedAt) : undefined,
  );
  const turnEndedAt = $derived(
    chat?.turn.endedAt ? Date.parse(chat.turn.endedAt) : undefined,
  );
  const turnElapsed = $derived.by(() => {
    if (turnStartedAt === undefined) return undefined;
    const end = chat?.turn.running ? now : turnEndedAt;
    return end === undefined ? undefined : Math.max(0, end - turnStartedAt);
  });

  function formatDuration(milliseconds: number) {
    const totalSeconds = Math.floor(milliseconds / 1000);
    const seconds = totalSeconds % 60;
    const totalMinutes = Math.floor(totalSeconds / 60);
    const minutes = totalMinutes % 60;
    const hours = Math.floor(totalMinutes / 60);
    if (hours)
      return `${hours} 小时 ${String(minutes).padStart(2, "0")} 分 ${String(seconds).padStart(2, "0")} 秒`;
    if (totalMinutes)
      return `${totalMinutes} 分 ${String(seconds).padStart(2, "0")} 秒`;
    return `${seconds} 秒`;
  }

  // Some agents (agy) end the turn while a backgrounded command still runs and
  // never report it finished over ACP; "已完成" would then be wrong. The server
  // records `turn.unfinished` on the state so a truncated or reset timeline
  // still keeps the label.
  const turnHasOpenTools = $derived.by(() => {
    if (chat?.turn.unfinished) return true;
    for (let i = entries.length - 1; i >= 0; i--) {
      const entry = entries[i];
      if (entry?.type === "message" && entry.role === "user") return false;
      if (entry?.type === "tool" && entry.status === "in_progress") return true;
    }
    return false;
  });
  const turnCompletionLabel = $derived(
    chat?.turn.stopReason === "end_turn"
      ? turnHasOpenTools
        ? "本轮已结束，仍有任务未完成"
        : "已完成"
      : chat?.turn.stopReason === "cancelled"
        ? "已中断"
        : "已结束",
  );
  const turnFailed = $derived(
    chat?.turn.stopReason === "error" ||
      chat?.turn.stopReason === "refusal" ||
      chat?.turn.stopReason === "auth_required",
  );
  const turnCompletionAnnouncement = $derived.by(() => {
    if (
      !chat ||
      chat.turn.running ||
      turnElapsed === undefined ||
      turnEndedAt === undefined
    )
      return "";
    return `${turnCompletionLabel} · 用时 ${formatDuration(turnElapsed)}`;
  });

  // Server timestamps keep the duration correct across reconnects and devices.
  // Only the live display ticks; the total is derived from the final timestamp.
  $effect(() => {
    if (mode !== "turn" || !chat?.turn.running || turnStartedAt === undefined)
      return;
    now = Date.now();
    const timer = window.setInterval(() => (now = Date.now()), 1000);
    return () => window.clearInterval(timer);
  });
</script>

{#if mode === "notices"}
  {#if truncated}
    <InlineNotice>更早的历史未缓存，可切到终端查看</InlineNotice>
  {/if}
  {#each chat?.outdated ?? [] as item (item.name)}
    <InlineNotice tone="warning">{outdatedText(item)}</InlineNotice>
  {/each}
  {#if chat?.unsupported.length}
    <InlineNotice tone="warning"
      >聊天视图不支持该参数：{chat.unsupported.join(" ")}</InlineNotice
    >
  {/if}
{:else if mode === "dock"}
  {#if error}<InlineNotice tone="danger">{error}</InlineNotice>{/if}
  {#if chat?.connectorMissing}
    <ConnectorInstallHint connector={chat.connectorMissing}>
      {#snippet actions()}<Button
          size="sm"
          variant="outline"
          disabled={!!unavailable}
          onclick={onterminal}>切到终端</Button
        >{/snippet}
    </ConnectorInstallHint>
  {/if}
  {#if chat?.authRequired}
    <InlineNotice tone="danger">
      <span class="flex flex-wrap items-center gap-2"
        >Agent 需要登录，请在终端中完成登录。<Button
          size="sm"
          variant="outline"
          disabled={!!unavailable}
          onclick={onterminal}>切到终端登录</Button
        ></span
      >
    </InlineNotice>
  {/if}
{:else}
  <!-- A persistent live region announces only the final duration. The
       visible per-second ticker stays out of every live region. -->
  <p class="sr-only" role="status">{turnCompletionAnnouncement}</p>
  {#if !chat}
    <p
      class="flex items-center gap-2 text-sm text-muted-foreground"
      role="status"
    >
      <IconLoader2 class="size-3.5 animate-spin" aria-hidden="true" />
      正在连接聊天…
    </p>
  {:else if chat.turn.running && !request}
    <p
      class="flex items-center gap-2 text-sm text-muted-foreground"
      role="status"
    >
      <IconLoader2 class="size-3.5 animate-spin" aria-hidden="true" />
      正在工作…
      {#if turnElapsed !== undefined}
        <span class="tabular-nums" aria-hidden="true"
          >· {formatDuration(turnElapsed)}</span
        >
      {/if}
    </p>
  {:else if turnElapsed !== undefined && turnEndedAt !== undefined}
    <p
      class="flex items-center gap-2 text-sm text-muted-foreground"
      aria-hidden="true"
      data-turn-result={chat.turn.stopReason}
    >
      {#if chat.turn.stopReason === "end_turn" && turnHasOpenTools}
        <IconLoader2 class="size-3.5 text-info" aria-hidden="true" />
      {:else if chat.turn.stopReason === "end_turn"}
        <IconCircleCheck class="size-3.5 text-success" aria-hidden="true" />
      {:else if chat.turn.stopReason === "cancelled"}
        <IconHandStop class="size-3.5 text-warning" aria-hidden="true" />
      {:else if turnFailed}
        <IconX class="size-3.5 text-destructive" aria-hidden="true" />
      {:else}
        <IconCircleCheck class="size-3.5" aria-hidden="true" />
      {/if}
      {turnCompletionLabel}
      <span class="tabular-nums">· 用时 {formatDuration(turnElapsed)}</span>
    </p>
  {/if}
{/if}
