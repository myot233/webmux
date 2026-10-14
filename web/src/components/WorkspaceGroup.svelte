<script lang="ts">
  import { untrack } from "svelte";
  import type { SessionInfo, Worktree } from "../schemas";
  import { Button } from "$lib/components/ui/button";
  import { Badge } from "$lib/components/ui/badge";
  import * as DropdownMenu from "$lib/components/ui/dropdown-menu";
  import SessionCard from "./SessionCard.svelte";
  let {
    title,
    path,
    worktree,
    sessions,
    now,
    onclose,
    onstart,
    onremove,
  }: {
    title: string;
    path?: string;
    worktree?: Worktree;
    sessions: SessionInfo[];
    now: number;
    onclose: (session: SessionInfo) => void;
    onstart?: () => void;
    onremove?: () => void;
  } = $props();
  let expanded = $state(untrack(() => sessions.length > 0));
  const counts = $derived({
    running: sessions.filter(
      (s) => s.status === "running" || s.status === "idle",
    ).length,
    waiting: sessions.filter((s) => s.status === "waiting").length,
    exited: sessions.filter((s) => s.status === "exited").length,
  });
</script>

<section
  class="min-w-0 rounded-lg border bg-background/50"
  data-worktree-id={worktree?.id}
  aria-label={title}
>
  <div class="flex flex-wrap items-center gap-2 p-3 sm:p-4">
    <button
      type="button"
      class="flex min-w-0 flex-1 items-start gap-2 text-left focus-visible:outline-2 focus-visible:outline-ring"
      aria-expanded={expanded}
      onclick={() => {
        expanded = !expanded;
      }}
    >
      <span class="mt-0.5 shrink-0 text-muted-foreground" aria-hidden="true"
        >{expanded ? "▾" : "▸"}</span
      >
      <span class="min-w-0 space-y-1">
        <span class="block text-sm font-medium break-all">{title}</span>
        <span class="block text-xs text-muted-foreground"
          >运行 {counts.running} · 等待 {counts.waiting} · 退出 {counts.exited}</span
        >
      </span>
    </button>
    {#if onstart}<Button
        size="sm"
        variant="outline"
        disabled={worktree && (!worktree.available || worktree.kind === "bare")}
        onclick={onstart}>启动会话</Button
      >{/if}
    {#if onremove}
      <DropdownMenu.Root>
        <DropdownMenu.Trigger
          >{#snippet child({ props })}<Button
              {...props}
              size="icon-sm"
              variant="ghost"
              aria-label={`工作树菜单 ${title}`}>⋯</Button
            >{/snippet}</DropdownMenu.Trigger
        >
        <DropdownMenu.Content align="end"
          ><DropdownMenu.Item variant="destructive" onSelect={onremove}
            >删除 worktree</DropdownMenu.Item
          ></DropdownMenu.Content
        >
      </DropdownMenu.Root>
    {/if}
    <div class="flex w-full flex-wrap gap-1.5 pl-5 text-xs">
      {#if worktree}
        {#if worktree.kind === "main"}<Badge variant="secondary">主工作区</Badge
          >{/if}
        {#if worktree.kind === "bare"}<Badge variant="secondary"
            >bare · 不可启动</Badge
          >{/if}
        {#if worktree.locked}<Badge variant="outline">已锁定</Badge>{/if}
        {#if worktree.prunable}<Badge variant="outline">Git 登记待清理</Badge
          >{/if}
        {#if !worktree.available}<Badge variant="destructive">不可用</Badge>
        {:else if worktree.dirty === null}<Badge variant="outline"
            >状态未知</Badge
          >
        {:else if worktree.dirty}<Badge variant="outline">有修改</Badge>
        {:else}<Badge variant="secondary">已跟踪文件干净</Badge>{/if}
        {#if worktree.hasUntracked}<Badge variant="outline">未跟踪文件</Badge
          >{/if}
        {#if worktree.hasIgnored}<Badge variant="outline">忽略文件</Badge>{/if}
      {/if}
      {#if sessions.length > 1}<Badge variant="outline"
          >共享目录 · {sessions.length} 个独立终端</Badge
        >{/if}
    </div>
    {#if path}<p
        class="w-full pl-5 font-mono text-xs break-all whitespace-pre-wrap text-muted-foreground"
      >
        {path}
      </p>{/if}
    {#if worktree?.lockReason}<p
        class="w-full pl-5 text-xs break-all whitespace-pre-wrap text-muted-foreground"
      >
        锁定原因：{worktree.lockReason}
      </p>{/if}
    {#if worktree?.error}<p
        role="alert"
        class="w-full pl-5 text-xs break-all text-destructive"
      >
        {worktree.error}
      </p>{/if}
  </div>
  {#if expanded}
    <div class="border-t p-3 sm:p-4">
      {#if sessions.length}
        <div class="grid min-w-0 gap-3 md:grid-cols-2 xl:grid-cols-3">
          {#each sessions as session (session.id)}<SessionCard
              {session}
              {now}
              {onclose}
            />{/each}
        </div>
      {:else}<p class="text-sm text-muted-foreground">
          暂无会话。工作树独立保留，可以随时启动。
        </p>{/if}
    </div>
  {/if}
</section>
