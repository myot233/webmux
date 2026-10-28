<script lang="ts">
  import type { Component } from "svelte";
  import IconArrowsMove from "~icons/tabler/arrows-move";
  import IconBulb from "~icons/tabler/bulb";
  import IconCheck from "~icons/tabler/check";
  import IconCircleDashed from "~icons/tabler/circle-dashed";
  import IconFileText from "~icons/tabler/file-text";
  import IconLoader2 from "~icons/tabler/loader-2";
  import IconPencil from "~icons/tabler/pencil";
  import IconSearch from "~icons/tabler/search";
  import IconShieldQuestion from "~icons/tabler/shield-question";
  import IconSwitchHorizontal from "~icons/tabler/switch-horizontal";
  import IconTerminal2 from "~icons/tabler/terminal-2";
  import IconTool from "~icons/tabler/tool";
  import IconTrash from "~icons/tabler/trash";
  import IconWorld from "~icons/tabler/world";
  import IconX from "~icons/tabler/x";
  import { Button } from "$lib/components/ui/button";
  import { diffStats, type ToolEntry } from "../../chat-model";
  import ChatDisclosure from "./ChatDisclosure.svelte";
  import DiffCount from "./DiffCount.svelte";

  // One tool call as a quiet row: kind, title, diff size and status. It is
  // keyed by toolCallId, so later updates change this row in place; the
  // details show its output and open diffs in the code panel.
  let {
    tool,
    waiting = false,
    onreview,
  }: {
    tool: ToolEntry;
    /** A permission request on this call waits for an answer. */
    waiting?: boolean;
    /** Opens a changed file in the code panel. */
    onreview: (path: string) => void;
  } = $props();

  const kinds: Record<string, Component> = {
    read: IconFileText,
    edit: IconPencil,
    delete: IconTrash,
    move: IconArrowsMove,
    search: IconSearch,
    execute: IconTerminal2,
    think: IconBulb,
    fetch: IconWorld,
    switch_mode: IconSwitchHorizontal,
  };
  const KindIcon = $derived(kinds[tool.kind] ?? IconTool);
  const blocks = $derived(
    tool.content.map((block) => ({
      block,
      stats: block.type === "diff" ? diffStats(block) : null,
    })),
  );
  const added = $derived(
    blocks.reduce((sum, { stats }) => sum + (stats?.added ?? 0), 0),
  );
  const removed = $derived(
    blocks.reduce((sum, { stats }) => sum + (stats?.removed ?? 0), 0),
  );
  const rejected = $derived(
    tool.permission?.outcome === "cancelled" ||
      !!tool.permission?.kind?.startsWith("reject"),
  );
  const status = $derived(
    waiting
      ? "等待授权"
      : tool.permission?.outcome === "cancelled"
        ? "已取消"
        : rejected
          ? "已拒绝"
          : tool.status === "in_progress"
            ? "进行中"
            : tool.status === "pending"
              ? "等待中"
              : tool.status === "failed"
                ? "失败"
                : "完成",
  );
</script>

<div class="min-w-0" data-tool-call={tool.id}>
  <ChatDisclosure>
    {#snippet summary()}
      <KindIcon class="size-3.5 shrink-0" aria-hidden="true" />
      <span
        class={tool.kind === "execute"
          ? "min-w-0 truncate font-mono text-xs text-foreground"
          : "min-w-0 truncate text-foreground"}
        >{tool.title || (tool.partial ? "早前的工具调用" : "工具调用")}</span
      >
      {#if added || removed}<DiffCount {added} {removed} />{/if}
      <span
        class={waiting || tool.status === "pending"
          ? "flex shrink-0 items-center gap-1 text-xs text-warning"
          : tool.status === "failed" || rejected
            ? "flex shrink-0 items-center gap-1 text-xs text-destructive"
            : "flex shrink-0 items-center gap-1 text-xs"}
      >
        {#if waiting}<IconShieldQuestion
            class="size-3.5"
            aria-hidden="true"
          />{:else if tool.status === "in_progress"}<IconLoader2
            class="size-3.5 animate-spin text-info"
            aria-hidden="true"
          />{:else if tool.status === "pending"}<IconCircleDashed
            class="size-3.5"
            aria-hidden="true"
          />{:else if tool.status === "failed" || rejected}<IconX
            class="size-3.5"
            aria-hidden="true"
          />{:else}<IconCheck
            class="size-3.5 text-success"
            aria-hidden="true"
          />{/if}<span class={status === "完成" ? "sr-only" : undefined}
          >{status}</span
        >
      </span>
    {/snippet}
    {#each blocks as { block, stats }, i (i)}
      {#if block.type === "text"}
        <pre
          class="overflow-x-auto rounded-md bg-muted px-3 py-2 font-mono text-xs break-all whitespace-pre-wrap">{block.text}</pre>
      {:else if block.type === "diff"}
        <div class="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
          <span class="min-w-0 font-mono text-xs break-all text-foreground"
            >{block.path}</span
          >
          {#if stats}<DiffCount
              added={stats.added}
              removed={stats.removed}
            />{/if}
          {#if block.path}
            {@const path = block.path}
            <Button variant="outline" size="xs" onclick={() => onreview(path)}
              >在代码面板中查看</Button
            >
          {/if}
        </div>
      {:else if block.type === "resource_link"}
        <p class="font-mono text-xs break-all">{block.name || block.uri}</p>
      {:else}
        <p>不支持的内容：{block.unsupported || "未知类型"}</p>
      {/if}
      {#if block.truncated}<p class="text-xs">（内容过长，已截断）</p>{/if}
    {:else}
      <p>
        {tool.partial ? "这次调用开始得更早，详情已不在缓存中。" : "没有输出。"}
      </p>
    {/each}
    {#each tool.locations as location, i (i)}
      <p class="font-mono text-xs break-all">
        {location.path}{location.line === undefined ? "" : `:${location.line}`}
      </p>
    {/each}
  </ChatDisclosure>
</div>
