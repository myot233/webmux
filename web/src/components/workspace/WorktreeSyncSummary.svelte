<script lang="ts">
  import type { Snippet } from "svelte";
  import IconCopy from "~icons/tabler/copy";
  import { Badge } from "$lib/components/ui/badge";
  import { Button } from "$lib/components/ui/button";
  import { copyText } from "../../platform/clipboard";
  import { relativeTime } from "../../domains/workspace/commit-choices";
  import type { Worktree } from "../../contracts/schemas";
  import {
    changeCounts,
    fetchedText,
    operationLabels,
    worktreeUpstream,
  } from "../../domains/workspace/worktree-sync";
  import InlineNotice from "../shared/InlineNotice.svelte";
  import UpstreamStatus from "./UpstreamStatus.svelte";

  // A worktree's Git sync: its upstream and the commits ahead or behind it,
  // the change counts, the last commit and an operation in progress.
  // `indicator` is the home title's: only what needs attention, laid out by
  // the parent's flex row. `compact` is one wrapping line of a list row;
  // `full` the details' three sections.
  let {
    worktree,
    variant = "compact",
    lastFetchedAt,
    actions,
  }: {
    worktree: Worktree;
    variant?: "indicator" | "compact" | "full";
    /** The repository's last fetch; shown beside the upstream in `full`. */
    lastFetchedAt?: string | null;
    /** The branch's sync actions, at the end of `full`'s branch section. */
    actions?: Snippet;
  } = $props();
  const upstream = $derived(worktreeUpstream(worktree));
  const operation = $derived(
    worktree.operation ? operationLabels[worktree.operation] : "",
  );
  const commit = $derived(worktree.lastCommit);
  // A readable worktree without commits says so instead of "未知".
  const empty = $derived(
    worktree.available && worktree.changes !== null && worktree.head === null,
  );
  const changed = $derived(
    worktree.changes
      ? changeCounts(worktree.changes).filter((item) => item.count > 0)
      : [],
  );
</script>

{#snippet operationBadge()}
  {#if operation}<Badge variant="warning">{operation}</Badge>{/if}
{/snippet}

{#if variant === "indicator"}
  {@render operationBadge()}
  {#if upstream}<UpstreamStatus state={upstream} indicator />{/if}
{:else if worktree.kind === "bare"}
  <!-- A bare repository has no working tree to sync. -->
{:else if variant === "compact"}
  <div
    class="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground"
  >
    {#if operation || upstream}
      <span class="flex min-w-0 flex-wrap items-center gap-1.5">
        {@render operationBadge()}
        {#if upstream}<UpstreamStatus state={upstream} />{/if}
      </span>
    {/if}
    {#if changed.length}
      <span class="tabular-nums"
        >{changed
          .map((item) => `${item.label} ${item.count}`)
          .join(" · ")}</span
      >
    {/if}
    {#if commit}
      <span class="min-w-0 break-all" title={commit.hash}
        >{commit.subject || "无标题提交"} · {relativeTime(
          commit.committedAt,
        )}</span
      >
    {:else if empty}<span>尚无提交</span>{/if}
  </div>
{:else}
  <div class="space-y-4">
    {#if operation}
      <InlineNotice tone="warning"
        >{operation}：Git 操作尚未完成，完成或中止后状态才会稳定。</InlineNotice
      >
    {/if}
    <section class="space-y-2" aria-label="分支与上游">
      <h3 class="text-sm font-medium">分支与上游</h3>
      <dl class="grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 gap-y-2 text-xs">
        <dt class="text-muted-foreground">分支</dt>
        <dd class="font-mono break-all">
          {worktree.branch ?? "（分离 HEAD）"}{worktree.head
            ? ` · ${worktree.head.slice(0, 8)}`
            : ""}
        </dd>
        {#if upstream}
          <dt class="text-muted-foreground">上游</dt>
          <dd class="flex min-w-0 flex-wrap items-center gap-1.5">
            <UpstreamStatus state={upstream} />
          </dd>
          {#if lastFetchedAt !== undefined && upstream.kind !== "none" && upstream.kind !== "unknown"}
            <dt class="text-muted-foreground">上次获取</dt>
            <dd
              title={lastFetchedAt
                ? new Date(lastFetchedAt).toLocaleString()
                : undefined}
            >
              {fetchedText(lastFetchedAt)}（近似，领先/落后以此为准）
            </dd>
          {/if}
        {/if}
      </dl>
      {@render actions?.()}
    </section>
    <section class="space-y-2" aria-label="最近提交">
      <h3 class="text-sm font-medium">最近提交</h3>
      {#if commit}
        <dl class="grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 gap-y-2 text-xs">
          <dt class="text-muted-foreground">标题</dt>
          <dd class="break-all">{commit.subject || "无标题提交"}</dd>
          <dt class="text-muted-foreground">提交</dt>
          <dd class="flex min-w-0 items-start gap-1">
            <span class="min-w-0 flex-1 font-mono break-all">{commit.hash}</span
            >
            <Button
              size="icon-xs"
              variant="ghost"
              aria-label="复制提交哈希"
              title="复制提交哈希"
              onclick={() => void copyText(commit.hash)}
              ><IconCopy aria-hidden="true" /></Button
            >
          </dd>
          <dt class="text-muted-foreground">作者</dt>
          <dd class="break-all">{commit.author}</dd>
          <dt class="text-muted-foreground">时间</dt>
          <dd title={new Date(commit.committedAt).toLocaleString()}>
            {relativeTime(commit.committedAt)}
          </dd>
        </dl>
      {:else}
        <p class="text-xs text-muted-foreground">
          {empty ? "尚无提交" : "未知"}
        </p>
      {/if}
    </section>
    <section class="space-y-2" aria-label="修改统计">
      <h3 class="text-sm font-medium">修改统计</h3>
      {#if worktree.changes}
        <dl class="grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 gap-y-2 text-xs">
          {#each changeCounts(worktree.changes) as item (item.label)}
            <dt class="text-muted-foreground">{item.label}</dt>
            <dd class="tabular-nums">{item.count}</dd>
          {/each}
        </dl>
      {:else}
        <p class="text-xs text-muted-foreground">未知</p>
      {/if}
    </section>
  </div>
{/if}
