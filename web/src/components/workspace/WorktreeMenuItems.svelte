<script lang="ts">
  import * as DropdownMenu from "$lib/components/ui/dropdown-menu";
  import type { BranchOperation } from "../../domains/workspace/branch-actions";
  import type { Worktree } from "../../contracts/schemas";
  import BranchMenuItems from "./BranchMenuItems.svelte";

  // The worktree-level entries of a "⋯" menu, the same on the home page, in
  // the sidebar and in the repository panel: details, switching its branch,
  // home-page pinning and removal. Each place passes the actions it offers;
  // switching is left out for bare, missing and prunable worktrees. With
  // `worktree` and `onoperation`, its branch's sync actions follow the
  // switch (by `branchActions`).
  let {
    ondetails,
    onswitch,
    pinned,
    ontogglepin,
    onremove,
    worktree,
    onoperation,
  }: {
    ondetails?: () => void;
    onswitch?: () => void;
    pinned?: boolean;
    ontogglepin?: () => void;
    onremove?: () => void;
    worktree?: Worktree;
    onoperation?: (operation: BranchOperation) => void;
  } = $props();
</script>

{#if ondetails}<DropdownMenu.Item onSelect={ondetails}
    >工作树详情…</DropdownMenu.Item
  >{/if}
{#if onswitch}<DropdownMenu.Item onSelect={onswitch}
    >切换分支…</DropdownMenu.Item
  >{/if}
{#if worktree && onoperation}<BranchMenuItems
    target={worktree}
    {onoperation}
  />{/if}
{#if ontogglepin}<DropdownMenu.Item onSelect={ontogglepin}
    >{pinned ? "从主页移除" : "添加到主页"}</DropdownMenu.Item
  >{/if}
{#if onremove}<DropdownMenu.Item variant="destructive" onSelect={onremove}
    >删除 worktree…</DropdownMenu.Item
  >{/if}
