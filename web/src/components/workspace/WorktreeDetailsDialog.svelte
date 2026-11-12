<script lang="ts">
  import type { BranchOperation } from "../../domains/workspace/branch-actions";
  import type { SessionInfo, Worktree } from "../../contracts/schemas";
  import ResponsiveDialog from "../shared/ResponsiveDialog.svelte";
  import WorktreeDetails from "./WorktreeDetails.svelte";

  // The dialog holding a worktree's (or directory's) details outside the
  // repository panel: the home page's groups and the sidebar. A branch
  // action closes it and opens the operation's own dialog.
  let {
    title,
    path,
    worktree,
    sessions,
    lastFetchedAt,
    onoperation,
    onclose,
  }: {
    title: string;
    path?: string;
    worktree?: Worktree;
    sessions: readonly SessionInfo[];
    lastFetchedAt?: string | null;
    onoperation?: (operation: BranchOperation) => void;
    onclose: () => void;
  } = $props();
</script>

<ResponsiveDialog
  {title}
  description={worktree
    ? "工作树路径、Git 同步状态与占用空间。"
    : "目录与会话。"}
  {onclose}
>
  <WorktreeDetails
    {path}
    {worktree}
    {sessions}
    {lastFetchedAt}
    onoperation={onoperation &&
      ((operation) => {
        onclose();
        onoperation(operation);
      })}
  />
</ResponsiveDialog>
