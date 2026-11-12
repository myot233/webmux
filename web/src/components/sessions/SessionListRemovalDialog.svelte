<script lang="ts">
  import type { ArchivedSession, Worktree } from "../../contracts/schemas";
  import LazyDialog from "../shared/LazyDialog.svelte";

  let {
    worktree,
    record,
    onremoved,
    onchange,
    onclose,
  }: {
    worktree: Worktree;
    record: ArchivedSession | null;
    onremoved: () => void;
    onchange: () => void;
    onclose: () => void;
  } = $props();
</script>

<LazyDialog
  load={() => import("../workspace/RemoveWorktreeDialog.svelte")}
  label="删除确认"
  {onclose}
>
  {#snippet children(page)}<page.default
      {worktree}
      forgetRecords={record ? [record] : []}
      {onremoved}
      {onchange}
      {onclose}
    />{/snippet}
</LazyDialog>
