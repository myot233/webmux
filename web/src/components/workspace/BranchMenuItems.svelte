<script lang="ts">
  import * as DropdownMenu from "$lib/components/ui/dropdown-menu";
  import {
    branchActionLabels,
    branchActions,
    branchOperation,
    type BranchOperation,
  } from "../../domains/workspace/branch-actions";
  import type { Branch, Worktree } from "../../contracts/schemas";

  // The branch sync entries of a "⋯" menu, by `branchActions`: a worktree's
  // menu (home page, sidebar, repository panel) and a branch row's.
  let {
    target,
    onoperation,
  }: {
    target: Branch | Worktree;
    onoperation: (operation: BranchOperation) => void;
  } = $props();
  const offered = $derived(branchActions(target));
</script>

{#if offered.branch !== null}
  {@const branch = offered.branch}
  {#each offered.actions as action (action)}
    <DropdownMenu.Item
      variant={action === "delete" ? "destructive" : "default"}
      onSelect={() => onoperation(branchOperation(action, branch))}
      >{branchActionLabels[action]}</DropdownMenu.Item
    >
  {/each}
{/if}
