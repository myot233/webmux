<script lang="ts">
  import { Button } from "$lib/components/ui/button";
  import { ListButton } from "$lib/components/ui/list-button";
  import { diffStats, type ToolEntry } from "../../domains/chat/chat-model";
  import {
    isExternalLink,
    parseChatCodeLink,
    type ChatCodeLink,
  } from "../../domains/code/markdown-link";
  import DiffCount from "./DiffCount.svelte";

  // What a tool call produced: output text, diffs, resource links and the
  // locations it touched. Diffs open in the code panel.
  let {
    tool,
    onreview,
    onopenlocation,
  }: {
    tool: ToolEntry;
    /** Opens a changed file in the code panel. */
    onreview: (path: string) => void;
    /** Opens a referenced code file or line in the code panel. */
    onopenlocation?: (target: ChatCodeLink) => void;
  } = $props();

  const blocks = $derived(
    tool.content.map((block) => ({
      block,
      stats: block.type === "diff" ? diffStats(block) : null,
    })),
  );
</script>

{#each blocks as { block, stats }, i (i)}
  {#if block.type === "text"}
    <pre
      class="overflow-x-auto rounded-md bg-muted px-3 py-2 font-mono text-xs break-all whitespace-pre-wrap">{block.text}</pre>
  {:else if block.type === "diff"}
    <div class="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
      <span class="min-w-0 font-mono text-xs break-all text-foreground"
        >{block.path}</span
      >
      {#if stats}<DiffCount added={stats.added} removed={stats.removed} />{/if}
      {#if block.path}
        {@const path = block.path}
        <Button variant="outline" size="xs" onclick={() => onreview(path)}
          >在代码面板中查看</Button
        >
      {/if}
    </div>
  {:else if block.type === "resource_link"}
    {@const label = block.name || block.uri}
    {@const target =
      block.uri && onopenlocation ? parseChatCodeLink(block.uri) : null}
    {#if block.uri && isExternalLink(block.uri)}
      <p>
        <ListButton
          variant="code-link"
          href={block.uri}
          target="_blank"
          rel="noopener noreferrer">{label}</ListButton
        >
      </p>
    {:else if target}
      <p>
        <ListButton variant="code-link" onclick={() => onopenlocation?.(target)}
          >{label}</ListButton
        >
      </p>
    {:else}
      <p class="font-mono text-xs break-all">{label}</p>
    {/if}
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
  {@const label = `${location.path}${location.line === undefined ? "" : `:${location.line}`}`}
  {#if onopenlocation && location.path}
    <p>
      <ListButton
        variant="code-link"
        onclick={() =>
          onopenlocation({
            path: location.path,
            ...(location.line !== undefined && location.line >= 1
              ? { line: location.line }
              : {}),
          })}>{label}</ListButton
      >
    </p>
  {:else}
    <p class="font-mono text-xs break-all">{label}</p>
  {/if}
{/each}
