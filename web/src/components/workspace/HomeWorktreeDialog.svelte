<script lang="ts">
  import { Badge } from "$lib/components/ui/badge";
  import { Button } from "$lib/components/ui/button";
  import * as Command from "$lib/components/ui/command";
  import { placeArchived } from "../../domains/sessions/archived";
  import {
    homeLayout,
    worktreeTitle,
  } from "../../domains/workspace/home-worktrees";
  import type {
    ArchivedSession,
    HomeWorktrees,
    Repository,
    SessionInfo,
  } from "../../contracts/schemas";
  import ResponsiveDialog from "../shared/ResponsiveDialog.svelte";

  // Which of a repository's worktrees the home page lists. Every worktree is
  // one checkable row; a worktree with sessions stays listed whatever its
  // check, so the row says so instead of hiding it.
  let {
    repository,
    sessions,
    records,
    home,
    ontoggle,
    onmanage,
    onclose,
  }: {
    repository: Repository;
    sessions: readonly SessionInfo[];
    records: readonly ArchivedSession[];
    home: HomeWorktrees;
    ontoggle: (worktreeId: string, pinned: boolean) => void;
    onmanage?: () => void;
    onclose: () => void;
  } = $props();
  let search = $state("");
  const layout = $derived(
    homeLayout(
      home,
      repository,
      sessions,
      placeArchived(
        records.filter((record) => !sessions.some((s) => s.id === record.id)),
        [repository],
      ).repositories.get(repository.id)?.worktrees,
    ),
  );
  // Every worktree in a stable order, so a row does not jump when its check
  // changes; a worktree with sessions is listed even when not pinned.
  const worktrees = $derived(layout.trees);
  const pinned = $derived(
    new Set(layout.shown.filter((w) => w.pinned).map((w) => w.tree.id)),
  );
  const active = $derived(
    new Set(layout.shown.filter((w) => !w.pinned).map((w) => w.tree.id)),
  );
</script>

<ResponsiveDialog
  title={`主页显示 · ${repository.name}`}
  description="勾选的工作树显示在主页；有会话的工作树始终显示。"
  {onclose}
>
  <Command.Root
    label="主页工作树"
    class="h-auto min-h-0 min-w-0 flex-1"
    filter={(value, term, keywords) =>
      [value, ...(keywords ?? [])].some((text) =>
        text.toLowerCase().includes(term.trim().toLowerCase()),
      )
        ? 1
        : 0}
    disableInitialScroll
  >
    <Command.Input
      bind:value={search}
      placeholder="按分支或路径搜索…"
      autocomplete="off"
      autocapitalize="off"
      autocorrect="off"
      spellcheck={false}
    />
    <Command.List
      aria-label="工作树列表"
      aria-multiselectable="true"
      class="max-h-72 min-h-0 flex-1"
    >
      <Command.Empty>没有匹配的 worktree</Command.Empty>
      {#each worktrees as tree (tree.id)}
        <Command.Item
          value={tree.id}
          keywords={[worktreeTitle(tree), tree.path]}
          data-checked={pinned.has(tree.id)}
          aria-label={worktreeTitle(tree)}
          onSelect={() => ontoggle(tree.id, !pinned.has(tree.id))}
        >
          <span class="min-w-0 flex-1 space-y-0.5">
            <span class="flex flex-wrap items-center gap-1.5">
              <span class="font-medium break-all">{worktreeTitle(tree)}</span>
              {#if tree.kind === "main"}<Badge variant="secondary"
                  >主工作区</Badge
                >{/if}
              {#if active.has(tree.id) && !pinned.has(tree.id)}<Badge
                  variant="outline">有会话，始终显示</Badge
                >{/if}
              {#if !tree.available}<Badge variant="destructive">不可用</Badge
                >{/if}
            </span>
            <span
              class="block font-mono text-xs break-all text-muted-foreground"
              >{tree.path}</span
            >
          </span>
        </Command.Item>
      {/each}
    </Command.List>
  </Command.Root>
  <div
    class="flex flex-wrap items-center justify-between gap-2 border-t px-3 py-2 text-xs text-muted-foreground"
  >
    <span>取消勾选只把它移出主页，不会删除 worktree。</span>
    {#if onmanage}
      <Button
        size="xs"
        variant="ghost"
        onclick={() => {
          onclose();
          onmanage();
        }}>批量管理…</Button
      >
    {/if}
  </div>
</ResponsiveDialog>
