<script lang="ts">
  import { worktreeTitle } from "../../domains/workspace/home-worktrees";
  import type { WorktreePreset } from "../../domains/sessions/new-session-defaults";
  import type { Repository, Worktree } from "../../contracts/schemas";
  import ResponsiveDialog from "../shared/ResponsiveDialog.svelte";
  import SwitchBranch, { type SwitchSource } from "./SwitchBranch.svelte";

  // The switch of a worktree's branch outside the repository panel (home
  // page, sidebar): a dialog on desktop, a bottom drawer on phones. Every
  // way out also closes it, so only one dialog is ever on screen; the host
  // reads its own state in the handler before the close clears it.
  let {
    repository,
    worktree,
    source,
    onchange,
    onnewworktree,
    ongoto,
    onclose,
  }: {
    repository: Repository;
    worktree: Worktree;
    source?: SwitchSource;
    /** A switch ran; the repositories are read again. */
    onchange: () => void;
    /** Opens the new-worktree dialog with the chosen target. */
    onnewworktree: (preset: WorktreePreset) => void;
    /** Opens the details of the worktree at a path. */
    ongoto: (path: string) => void;
    onclose: () => void;
  } = $props();
  let busy = $state(false);
</script>

<ResponsiveDialog
  title={`切换分支：${worktreeTitle(worktree)}`}
  description="在原位切换这个工作树的分支。有未提交的修改时不能切换；运行中的会话不会被关闭。"
  drawerDescription="原位切换分支；会话不会被关闭。"
  truncateDrawerTitle
  {busy}
  {onclose}
>
  <SwitchBranch
    {repository}
    {worktree}
    {source}
    bind:busy
    {onchange}
    ondone={onclose}
    onnewworktree={(preset) => {
      onnewworktree(preset);
      onclose();
    }}
    ongoto={(path) => {
      ongoto(path);
      onclose();
    }}
    oncancel={onclose}
  />
</ResponsiveDialog>
