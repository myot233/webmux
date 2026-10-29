<script lang="ts">
  import type { SessionInfo, Worktree } from "../schemas";
  import IconDots from "~icons/tabler/dots";
  import IconFolder from "~icons/tabler/folder";
  import IconGitBranch from "~icons/tabler/git-branch";
  import IconInfoCircle from "~icons/tabler/info-circle";
  import IconPlus from "~icons/tabler/plus";
  import { Button } from "$lib/components/ui/button";
  import { Badge } from "$lib/components/ui/badge";
  import * as DropdownMenu from "$lib/components/ui/dropdown-menu";
  import * as Item from "$lib/components/ui/item";
  import SessionRow from "./SessionRow.svelte";
  import InlineNotice from "./InlineNotice.svelte";
  import { countSessions } from "../session-status";
  let {
    title,
    path,
    worktree,
    sessions,
    visibleIds,
    hidden = false,
    compact = false,
    now,
    onclose,
    onstart,
    onremove,
    pinned,
    ontogglepin,
  }: {
    title: string;
    path?: string;
    worktree?: Worktree;
    sessions: SessionInfo[];
    visibleIds?: ReadonlySet<string>;
    hidden?: boolean;
    compact?: boolean;
    now: number;
    onclose: (session: SessionInfo) => void;
    onstart?: () => void;
    onremove?: () => void;
    // Undefined outside curated repository lists.
    pinned?: boolean;
    ontogglepin?: () => void;
  } = $props();
  const detailsId = $props.id();
  let detailsOpen = $state(false);
  let menuOpen = $state(false);
  $effect(() => {
    if (hidden) menuOpen = false;
  });
  const visible = $derived(
    visibleIds
      ? sessions.filter((session) => visibleIds.has(session.id))
      : sessions,
  );
  const counts = $derived(countSessions(visible));
  const groupPath = $derived(path ?? worktree?.path);
</script>

<section
  class="min-w-0 space-y-3"
  data-worktree-id={worktree?.id}
  {hidden}
  aria-label={title}
>
  <div class="flex min-w-0 flex-wrap items-center gap-x-4 gap-y-2">
    <div
      class="flex w-full min-w-0 flex-none flex-wrap items-center gap-2 sm:w-auto sm:flex-1"
    >
      {#if worktree}
        <IconGitBranch
          class="size-4 shrink-0 text-muted-foreground"
          aria-hidden="true"
        />
      {:else}
        <IconFolder
          class="size-4 shrink-0 text-muted-foreground"
          aria-hidden="true"
        />
      {/if}
      <h3 class="min-w-0 break-all text-sm font-semibold">{title}</h3>
      {#if visible.length > 1}<span
          class="shrink-0 text-xs text-muted-foreground">{visible.length}</span
        >{/if}
      {#if worktree && (pinned === false || worktree.kind === "bare" || worktree.locked || worktree.prunable || !worktree.available || worktree.dirty !== false || worktree.hasUntracked)}
        <div class="flex max-w-full flex-wrap items-center gap-1.5">
          {#if pinned === false}<Badge variant="outline">未添加到主页</Badge
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
          {:else if worktree.dirty}<Badge variant="warning">有修改</Badge>
          {/if}
          {#if worktree.hasUntracked}<Badge variant="outline">未跟踪文件</Badge
            >{/if}
        </div>
      {/if}
    </div>
    <div
      class="ml-auto flex max-w-full flex-wrap items-center justify-end gap-1"
    >
      {#if onstart}<Button
          size="sm"
          variant="outline"
          class="min-h-9 sm:min-h-8"
          disabled={worktree &&
            (!worktree.available || worktree.kind === "bare")}
          onclick={onstart}
          ><IconPlus class="size-4" aria-hidden="true" />启动会话</Button
        >{/if}
      <Button
        size="icon-touch"
        variant="ghost"
        aria-label={`${worktree ? "工作树" : "目录"}详情 ${title}`}
        title={`${worktree ? "工作树" : "目录"}详情 ${title}`}
        aria-expanded={detailsOpen}
        aria-controls={detailsId}
        onclick={() => (detailsOpen = !detailsOpen)}
        ><IconInfoCircle aria-hidden="true" /></Button
      >
      {#if onremove || ontogglepin}
        <DropdownMenu.Root bind:open={menuOpen}>
          <DropdownMenu.Trigger>
            {#snippet child({ props })}
              <Button
                {...props}
                size="icon-sm"
                variant="ghost"
                class="size-9 sm:size-8"
                title={`工作树菜单 ${title}`}
                aria-label={`工作树菜单 ${title}`}
                ><IconDots class="size-4" aria-hidden="true" /></Button
              >
            {/snippet}
          </DropdownMenu.Trigger>
          <DropdownMenu.Content align="end">
            {#if ontogglepin}<DropdownMenu.Item onSelect={ontogglepin}
                >{pinned ? "从主页移除" : "添加到主页"}</DropdownMenu.Item
              >{/if}
            {#if onremove}<DropdownMenu.Item
                variant="destructive"
                onSelect={onremove}>删除 worktree</DropdownMenu.Item
              >{/if}
          </DropdownMenu.Content>
        </DropdownMenu.Root>
      {/if}
    </div>
    {#if worktree?.lockReason}<p
        class="w-full text-xs break-all whitespace-pre-wrap text-muted-foreground"
      >
        锁定原因：{worktree.lockReason}
      </p>{/if}
    {#if worktree?.error}<InlineNotice tone="danger" class="w-full">
        {worktree.error}
      </InlineNotice>{/if}
  </div>
  <div
    id={detailsId}
    hidden={!detailsOpen}
    class="space-y-2 rounded-lg border bg-muted/30 p-3"
  >
    <div class="flex w-full min-w-0 flex-wrap items-baseline gap-x-4 gap-y-1">
      {#if path}
        <p
          class="min-w-0 flex-1 font-mono text-xs break-all whitespace-pre-wrap text-muted-foreground"
        >
          {path}
        </p>
      {/if}
      {#if sessions.length}
        <p class="flex flex-wrap gap-x-3 gap-y-1 text-xs text-muted-foreground">
          <span>共 {visible.length} 个会话</span>
          <span
            >运行 <span class="inline-block w-[3ch] text-right tabular-nums"
              >{counts.active}</span
            ></span
          >
          <span
            >等待 <span class="inline-block w-[3ch] text-right tabular-nums"
              >{counts.waiting}</span
            ></span
          >
          <span
            >退出 <span class="inline-block w-[3ch] text-right tabular-nums"
              >{counts.exited}</span
            ></span
          >
        </p>
      {/if}
    </div>
    {#if worktree}<div class="flex flex-wrap gap-1.5">
        {#if worktree.kind === "main"}<Badge variant="secondary">主工作区</Badge
          >{/if}
        {#if worktree.available && worktree.dirty === false}<Badge
            variant="secondary">已跟踪文件干净</Badge
          >{/if}
      </div>{/if}
  </div>
  {#if sessions.length}
    <Item.Group
      variant={compact ? "panel" : "default"}
      class={compact ? undefined : "gap-3"}
      aria-label={`${title} 的会话`}
    >
      {#each sessions as session (session.id)}
        <SessionRow
          {session}
          {compact}
          {now}
          {groupPath}
          {onclose}
          hidden={hidden || Boolean(visibleIds && !visibleIds.has(session.id))}
        />
      {/each}
    </Item.Group>
  {:else}
    <p class="border-t py-4 text-xs text-muted-foreground">
      暂无会话。工作树独立保留，可以随时启动。
    </p>
  {/if}
</section>
