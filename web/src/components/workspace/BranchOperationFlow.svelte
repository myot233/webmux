<script lang="ts">
  import type { BranchOperation } from "../../domains/workspace/branch-actions";
  import type { BranchOperations } from "../../domains/workspace/branch-operations";
  import DeleteBranches from "./DeleteBranches.svelte";
  import FastForwardBranch from "./FastForwardBranch.svelte";
  import PushBranch from "./PushBranch.svelte";

  // The flow of one branch operation, the same in its dialog (home page,
  // sidebar) and in a repository panel subview.
  let {
    repositoryId,
    operation,
    busy = $bindable(false),
    source,
    onchange,
    onclose,
  }: {
    repositoryId: string;
    operation: BranchOperation;
    busy?: boolean;
    source?: BranchOperations;
    onchange: () => void;
    onclose: () => void;
  } = $props();
</script>

{#if operation.kind === "fast-forward"}
  <FastForwardBranch
    {repositoryId}
    branch={operation.branch}
    bind:busy
    {source}
    {onchange}
    {onclose}
  />
{:else if operation.kind === "push"}
  <PushBranch
    {repositoryId}
    branch={operation.branch}
    bind:busy
    {source}
    {onchange}
    {onclose}
  />
{:else}
  <DeleteBranches
    {repositoryId}
    branches={operation.branches}
    bind:busy
    {source}
    {onchange}
    {onclose}
  />
{/if}
