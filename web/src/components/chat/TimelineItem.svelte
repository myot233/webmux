<script lang="ts">
  import IconCheck from "~icons/tabler/check";
  import IconFile from "~icons/tabler/file";
  import IconHandStop from "~icons/tabler/hand-stop";
  import IconLoader2 from "~icons/tabler/loader-2";
  import IconPhoto from "~icons/tabler/photo";
  import IconX from "~icons/tabler/x";
  import CodeMarkdownPreview from "../CodeMarkdownPreview.svelte";
  import InlineNotice from "../InlineNotice.svelte";
  import type { DisplayEntry } from "../../chat-model";
  import type { StopReason } from "../../schemas";
  import ChatDisclosure from "./ChatDisclosure.svelte";
  import ToolRow from "./ToolRow.svelte";

  // One transcript entry: the user's turn is a bubble, the agent's reply is
  // prose, and its work is a column of quiet one-line rows.
  let {
    entry,
    waiting,
    onreview,
  }: {
    entry: DisplayEntry;
    /** Tool calls whose permission request waits for an answer. */
    waiting: readonly string[];
    onreview: (path: string) => void;
  } = $props();

  const turnEnds: Partial<
    Record<StopReason, { text: string; error: boolean }>
  > = {
    cancelled: { text: "已中断 · 已输出的内容保留", error: false },
    max_tokens: { text: "输出达到上限，回合结束", error: false },
    max_turn_requests: { text: "请求次数达到上限，回合结束", error: false },
    refusal: { text: "Agent 拒绝了这次请求", error: true },
    error: { text: "回合出错", error: true },
    auth_required: { text: "Agent 需要登录", error: true },
  };
  const markers = {
    loaded: "已加载对话历史",
    truncated: "更早的历史未缓存",
    reconnected: "Agent 已重新连接",
    resumed: "Agent 已唤醒",
  };
</script>

{#snippet separator(text: string)}
  <div class="flex items-center gap-3 py-1 text-xs text-muted-foreground">
    <span class="h-px flex-1 bg-border"></span>
    <span class="min-w-0 text-center break-words">{text}</span>
    <span class="h-px flex-1 bg-border"></span>
  </div>
{/snippet}

{#if entry.type === "message" && entry.role === "user"}
  <div class="flex justify-end pt-2">
    <div
      class="max-w-[85%] min-w-0 space-y-1.5 rounded-xl bg-muted px-4 py-2.5 text-sm leading-relaxed break-words whitespace-pre-wrap"
    >
      {#if entry.text}<p><span class="sr-only">你：</span>{entry.text}</p>{/if}
      {#if entry.attachments?.length}
        <ul class="flex flex-wrap gap-1" aria-label="附件">
          {#each entry.attachments as attachment (attachment.path)}
            <li
              class="flex max-w-full min-w-0 items-center gap-1 rounded border bg-background/60 px-1.5 py-0.5 text-xs"
            >
              {#if attachment.kind === "image"}<IconPhoto
                  class="size-3.5 shrink-0"
                  aria-hidden="true"
                />{:else}<IconFile
                  class="size-3.5 shrink-0"
                  aria-hidden="true"
                />{/if}
              <span class="min-w-0 truncate"
                >{attachment.name ?? attachment.path}</span
              >
            </li>
          {/each}
        </ul>
      {/if}
    </div>
  </div>
{:else if entry.type === "message" && entry.role === "agent"}
  <div class="min-w-0">
    <CodeMarkdownPreview text={entry.text} variant="message" />
    {#if entry.truncated}<p class="text-xs text-muted-foreground">
        （内容过长，已截断）
      </p>{/if}
  </div>
{:else if entry.type === "message"}
  <ChatDisclosure>
    {#snippet summary()}已思考{/snippet}
    <p class="break-words whitespace-pre-wrap">{entry.text}</p>
  </ChatDisclosure>
{:else if entry.type === "reads"}
  <ChatDisclosure>
    {#snippet summary()}已浏览 {entry.calls.length} 项{/snippet}
    {#each entry.calls as call (call.id)}
      <p class="flex min-w-0 items-center gap-1.5">
        {#if call.status === "in_progress"}<IconLoader2
            class="size-3.5 shrink-0 animate-spin text-info"
            aria-label="进行中"
          />{:else if call.status === "failed"}<IconX
            class="size-3.5 shrink-0 text-destructive"
            aria-label="失败"
          />{:else}<IconCheck
            class="size-3.5 shrink-0 text-success"
            aria-hidden="true"
          />{/if}
        <span class="min-w-0 break-all">{call.title || "工具调用"}</span>
      </p>
    {/each}
  </ChatDisclosure>
{:else if entry.type === "tool"}
  <ToolRow tool={entry} waiting={waiting.includes(entry.id)} {onreview} />
{:else if entry.type === "compaction"}
  {#if entry.summary}
    <ChatDisclosure>
      {#snippet summary()}
        <span class="h-px flex-1 bg-border"></span>
        <span class="shrink-0 text-xs"
          >{entry.status === "failed" ? "上下文压缩失败" : "上下文已压缩"}</span
        >
        <span class="h-px flex-1 bg-border"></span>
      {/snippet}
      <p class="break-words whitespace-pre-wrap">{entry.summary}</p>
    </ChatDisclosure>
  {:else}
    {@render separator(
      entry.status === "running"
        ? "正在压缩上下文…"
        : entry.status === "failed"
          ? "上下文压缩失败"
          : "上下文已压缩",
    )}
  {/if}
{:else if entry.type === "notice"}
  <InlineNotice
    tone={entry.level === "error"
      ? "danger"
      : entry.level === "warning"
        ? "warning"
        : "info"}>{entry.text}</InlineNotice
  >
{:else if entry.type === "turnEnd"}
  {@const end = turnEnds[entry.stopReason]}
  {#if end?.error}
    <InlineNotice tone="danger"
      >{end.text}{entry.error ? `：${entry.error}` : ""}</InlineNotice
    >
  {:else if end}
    <p class="flex items-center gap-1.5 text-xs text-warning">
      <IconHandStop class="size-3.5 shrink-0" aria-hidden="true" />{end.text}
    </p>
  {/if}
{:else if entry.type === "marker"}
  {@render separator(
    entry.kind === "switched"
      ? entry.side === "tui"
        ? "已切到终端"
        : "已切到聊天"
      : markers[entry.kind],
  )}
{/if}
