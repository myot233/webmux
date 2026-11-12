<script lang="ts">
  import { onMount } from "svelte";
  import IconBook2 from "~icons/tabler/book-2";
  import IconFolder from "~icons/tabler/folder";
  import IconGitBranch from "~icons/tabler/git-branch";
  import type { SessionInfo } from "../../contracts/schemas";
  import { findShell } from "../../hooks/shared/shell.svelte";

  // Where a chat's agent works, above its input as on the quick start
  // page: the repository and the worktree's branch, or the directory of a
  // session outside any repository. Fixed once the session exists, so it
  // only shows them. Entering the chat reads the repositories again, so
  // the branch is current; the last known list shows meanwhile.
  let {
    workspace,
    cwd,
    class: className = "",
  }: {
    workspace: SessionInfo["workspace"];
    cwd: string;
    class?: string;
  } = $props();
  const repositories = findShell()?.repositories;
  onMount(() => void repositories?.refresh(true));
  const repository = $derived(
    repositories?.list.find((repo) => repo.id === workspace?.repositoryId),
  );
  const worktree = $derived(
    repository?.worktrees.find((tree) => tree.id === workspace?.worktreeId),
  );
  const repositoryName = $derived(
    repository?.name ?? baseName(workspace?.worktreePath ?? ""),
  );
  const branch = $derived(
    worktree?.branch ??
      (worktree?.head ? `${worktree.head.slice(0, 7)}（分离）` : ""),
  );
  function baseName(path: string) {
    return path.split("/").filter(Boolean).at(-1) ?? path;
  }
</script>

<div
  class={[
    "flex min-w-0 items-center gap-0.5 px-1 text-xs text-muted-foreground [&_svg]:size-3 [&_svg]:shrink-0",
    className,
  ]}
  role="group"
  aria-label="会话位置"
>
  {#if workspace}
    <span
      class="flex h-6 min-w-0 items-center gap-1 px-2"
      title={`仓库：${repositoryName}（创建后不可更改）`}
      ><IconBook2 aria-hidden="true" /><span class="sr-only">仓库：</span><span
        class="truncate">{repositoryName}</span
      ></span
    >
    {#if branch}<span
        class="flex h-6 min-w-0 items-center gap-1 px-2"
        title={`分支：${branch}（${workspace.worktreePath}）`}
        ><IconGitBranch aria-hidden="true" /><span class="sr-only">分支：</span
        ><span class="truncate">{branch}</span></span
      >{/if}
  {:else}
    <span
      class="flex h-6 min-w-0 items-center gap-1 px-2"
      title={`目录：${cwd}（创建后不可更改）`}
      ><IconFolder aria-hidden="true" /><span class="sr-only">目录：</span><span
        class="truncate">{baseName(cwd)}</span
      ></span
    >
  {/if}
</div>
