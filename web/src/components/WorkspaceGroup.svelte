<script lang="ts">
  import { IconGitBranch, IconFolder } from "@tabler/icons-svelte";
  import type { SessionInfo, Worktree } from "../schemas";
  import { Button } from "$lib/components/ui/button";
  import { Badge } from "$lib/components/ui/badge";
  import * as DropdownMenu from "$lib/components/ui/dropdown-menu";
  import * as Card from "$lib/components/ui/card";
  import * as Item from "$lib/components/ui/item";
  import SessionRow from "./SessionRow.svelte";
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
  const counts = $derived({
    running: sessions.filter(
      (s) => s.status === "running" || s.status === "idle",
    ).length,
    waiting: sessions.filter((s) => s.status === "waiting").length,
    exited: sessions.filter((s) => s.status === "exited").length,
  });
</script>

<Card.Root
  class="workspace-group min-w-0"
  role="region"
  data-worktree-id={worktree?.id}
  aria-label={title}
>
  <Card.Header>
    <Card.Title
      class="col-span-full col-start-1 row-start-1 min-w-0 break-all sm:col-span-1"
    >
      <h3 class="flex min-w-0 items-start gap-2 text-sm font-semibold">
        {#if worktree}<IconGitBranch
            class="mt-0.5 size-4 shrink-0 text-muted-foreground"
            aria-hidden="true"
          />
        {:else}<IconFolder
            class="mt-0.5 size-4 shrink-0 text-muted-foreground"
            aria-hidden="true"
          />{/if}
        <span class="min-w-0">{title}</span>
      </h3>
    </Card.Title>
    <Card.Description
      class="col-span-full col-start-1 row-start-2 min-w-0 sm:col-span-1"
    >
      {#if path}<span class="block font-mono break-all whitespace-pre-wrap"
          >{path}</span
        >{/if}
      <span class="block"
        >运行 {counts.running} · 等待 {counts.waiting} · 退出 {counts.exited}</span
      >
    </Card.Description>
    {#if onstart || onremove}
      <Card.Action
        class="col-start-1 row-start-3 row-span-1 flex items-center gap-2 justify-self-start sm:col-start-2 sm:row-start-1 sm:row-span-2 sm:justify-self-end"
      >
        {#if onstart}<Button
            size="sm"
            variant="outline"
            disabled={worktree &&
              (!worktree.available || worktree.kind === "bare")}
            onclick={onstart}>启动会话</Button
          >{/if}
        {#if onremove}
          <DropdownMenu.Root>
            <DropdownMenu.Trigger>
              {#snippet child({ props })}
                <Button
                  {...props}
                  size="icon-sm"
                  variant="ghost"
                  aria-label={`工作树菜单 ${title}`}>⋯</Button
                >
              {/snippet}
            </DropdownMenu.Trigger>
            <DropdownMenu.Content align="end">
              <DropdownMenu.Item variant="destructive" onSelect={onremove}
                >删除 worktree</DropdownMenu.Item
              >
            </DropdownMenu.Content>
          </DropdownMenu.Root>
        {/if}
      </Card.Action>
    {/if}
    {#if worktree}
      <div class="col-span-full flex flex-wrap items-center gap-1.5">
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
      </div>
    {/if}
    {#if worktree?.lockReason}<p
        class="col-span-full break-all whitespace-pre-wrap text-muted-foreground"
      >
        锁定原因：{worktree.lockReason}
      </p>{/if}
    {#if worktree?.error}<p
        role="alert"
        class="col-span-full break-all whitespace-pre-wrap text-destructive"
      >
        {worktree.error}
      </p>{/if}
  </Card.Header>
  <Card.Content>
    {#if sessions.length}
      <Item.Group class="gap-2" aria-label={`${title} 的会话`}>
        {#each sessions as session (session.id)}
          <SessionRow {session} {now} {onclose} />
        {/each}
      </Item.Group>
    {:else}
      <p class="text-muted-foreground">
        暂无会话。工作树独立保留，可以随时启动。
      </p>
    {/if}
  </Card.Content>
</Card.Root>
