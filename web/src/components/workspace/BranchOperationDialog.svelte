<script lang="ts">
  import {
    operationTitle,
    type BranchOperation,
  } from "../../domains/workspace/branch-actions";
  import type { BranchOperations } from "../../domains/workspace/branch-operations";
  import BranchOperationFlow from "./BranchOperationFlow.svelte";
  import ResponsiveDialog from "../shared/ResponsiveDialog.svelte";

  // A branch operation started outside the repository panel (home page,
  // sidebar, worktree details): a dialog on desktop, a bottom drawer on
  // phones, kept open while its request runs.
  let {
    repositoryId,
    operation,
    source,
    onchange,
    onclose,
  }: {
    repositoryId: string;
    operation: BranchOperation;
    source?: BranchOperations;
    onchange: () => void;
    onclose: () => void;
  } = $props();
  let busy = $state(false);
  const descriptions: Record<BranchOperation["kind"], string> = {
    "fast-forward": "按本机的远程跟踪分支快进，不访问网络，不产生合并提交。",
    push: "推送预览中的提交，不强制推送，不推送标签。",
    delete: "只删除本地分支，不删除远端分支。",
  };
</script>

<ResponsiveDialog
  title={operationTitle(operation)}
  description={descriptions[operation.kind]}
  {busy}
  {onclose}
>
  <BranchOperationFlow
    {repositoryId}
    {operation}
    bind:busy
    {source}
    {onchange}
    {onclose}
  />
</ResponsiveDialog>
