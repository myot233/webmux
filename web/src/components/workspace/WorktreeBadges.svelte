<script lang="ts">
  import type { Worktree } from "../../contracts/schemas";
  import { Badge } from "$lib/components/ui/badge";
  import { syncNotable } from "../../domains/workspace/worktree-sync";
  import WorktreeSyncSummary from "./WorktreeSyncSummary.svelte";

  // Undefined `pinned` means the list is not curated (no pin badge).
  let { worktree, pinned }: { worktree: Worktree; pinned?: boolean } = $props();
  const shown = $derived(
    pinned === false ||
      worktree.kind === "bare" ||
      worktree.locked ||
      worktree.prunable ||
      !worktree.available ||
      worktree.dirty !== false ||
      worktree.hasUntracked ||
      syncNotable(worktree),
  );
</script>

{#if shown}
  <div class="flex max-w-full flex-wrap items-center gap-1.5">
    {#if pinned === false}<Badge variant="outline">未添加到主页</Badge>{/if}
    {#if worktree.kind === "bare"}<Badge variant="secondary"
        >bare · 不可启动</Badge
      >{/if}
    {#if worktree.locked}<Badge variant="outline">已锁定</Badge>{/if}
    {#if worktree.prunable}<Badge variant="outline">Git 登记待清理</Badge>{/if}
    {#if !worktree.available}<Badge variant="destructive">不可用</Badge>
    {:else if worktree.dirty === null}<Badge variant="outline">状态未知</Badge>
    {:else if worktree.dirty}<Badge variant="warning">有修改</Badge>
    {/if}
    {#if worktree.hasUntracked}<Badge variant="outline">未跟踪文件</Badge>{/if}
    <WorktreeSyncSummary {worktree} variant="indicator" />
  </div>
{/if}
