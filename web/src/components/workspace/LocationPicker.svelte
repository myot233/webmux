<script lang="ts">
  import { tick } from "svelte";
  import IconCirclePlus from "~icons/tabler/circle-plus";
  import IconFolder from "~icons/tabler/folder";
  import IconGitBranch from "~icons/tabler/git-branch";
  import IconPencil from "~icons/tabler/pencil";
  import IconTrash from "~icons/tabler/trash";
  import { Button } from "$lib/components/ui/button";
  import * as Command from "$lib/components/ui/command";
  import { Input } from "$lib/components/ui/input";
  import { Label } from "$lib/components/ui/label";
  import ChoiceDetails from "../shared/ChoiceDetails.svelte";
  import DirectoryPicker from "./DirectoryPicker.svelte";
  import PickerField from "../shared/PickerField.svelte";
  import {
    directoryName,
    selectableWorktree,
    worktreeLabel,
    type LaunchScope,
    type TargetKind,
  } from "../../domains/sessions/new-session-defaults";
  import type { Repository, Worktree } from "../../contracts/schemas";

  // The "where" row of the new-session dialog: recent directories, worktrees
  // of every repository, a typed or browsed directory, and "new worktree".
  let {
    repositories,
    recent,
    preferredRepositoryId,
    createdTree,
    newWorktreeSummary,
    onremove,
    scope,
    kind = $bindable(),
    cwd = $bindable(),
    repositoryId = $bindable(),
    worktreeId = $bindable(),
    disabled = false,
  }: {
    repositories: readonly Repository[];
    recent: readonly string[];
    /** Listed first: the home page's or the context entry's repository. */
    preferredRepositoryId: string;
    /** A tree created by this dialog that the repository list may lack. */
    createdTree: Worktree | null;
    newWorktreeSummary: string;
    /** Asks to delete a linked worktree; the picker never deletes itself. */
    onremove?: (tree: Worktree) => void;
    /**
     * Narrows the list to the current repository, or to the current
     * worktree and a new worktree from its branch; no other places.
     */
    scope?: LaunchScope;
    kind: TargetKind;
    cwd: string;
    repositoryId: string;
    worktreeId: string;
    disabled?: boolean;
  } = $props();
  let open = $state(false);
  let search = $state("");
  // The desktop list stays mounted while closed; each opening starts unfiltered.
  function setOpen(next: boolean) {
    open = next;
    if (!next) search = "";
  }
  let cwdInput = $state<HTMLInputElement | null>(null);

  function treesOf(repo: Repository) {
    return createdTree?.repositoryId === repo.id &&
      !repo.worktrees.some((tree) => tree.id === createdTree?.id)
      ? [...repo.worktrees, createdTree]
      : repo.worktrees;
  }
  function scopedTrees(repo: Repository) {
    const trees = treesOf(repo);
    return scope?.worktreeId
      ? trees.filter((tree) => tree.id === scope.worktreeId)
      : trees;
  }
  const ordered = $derived(
    scope
      ? repositories.filter((repo) => repo.id === scope.repositoryId)
      : [
          ...repositories.filter((repo) => repo.id === preferredRepositoryId),
          ...repositories.filter((repo) => repo.id !== preferredRepositoryId),
        ],
  );
  const repository = $derived(
    repositories.find((repo) => repo.id === repositoryId),
  );
  const worktree = $derived(
    repository && treesOf(repository).find((tree) => tree.id === worktreeId),
  );

  function chooseTree(repo: Repository, tree: Worktree) {
    kind = "worktree";
    repositoryId = repo.id;
    worktreeId = tree.id;
    setOpen(false);
  }
  function chooseDirectory(dir: string) {
    kind = "directory";
    cwd = dir;
    setOpen(false);
  }
  async function typeDirectory() {
    kind = "directory";
    setOpen(false);
    await tick();
    cwdInput?.focus();
    cwdInput?.select();
  }
  function chooseNewWorktree(repo: Repository) {
    kind = "new-worktree";
    repositoryId = repo.id;
    setOpen(false);
  }
</script>

{#snippet treeGroup(repo: Repository)}
  <Command.Group heading={repo.name} value="repo:{repo.id}">
    {#each scopedTrees(repo) as tree (tree.id)}
      <Command.Item
        value="tree:{repo.id}:{tree.id}"
        keywords={[repo.name, worktreeLabel(tree), tree.path]}
        disabled={!selectableWorktree(tree) ||
          repo.availability !== "available"}
        data-checked={kind === "worktree" &&
          repositoryId === repo.id &&
          worktreeId === tree.id}
        onSelect={() => chooseTree(repo, tree)}
      >
        <IconGitBranch class="text-muted-foreground" aria-hidden="true" />
        <ChoiceDetails primary={worktreeLabel(tree)} secondary={tree.path} />
        {#if onremove && tree.kind === "linked"}
          <Button
            type="button"
            variant="quiet"
            size="icon-sm"
            class="pointer-events-auto"
            title={`删除工作树 ${worktreeLabel(tree)}`}
            aria-label={`删除工作树 ${worktreeLabel(tree)}`}
            onkeydown={(event) => {
              // The command root rewrites Enter into selecting the row.
              if (event.key === "Enter") event.stopPropagation();
            }}
            onclick={(event) => {
              event.stopPropagation();
              setOpen(false);
              onremove(tree);
            }}><IconTrash class="size-4" aria-hidden="true" /></Button
          >
        {/if}
      </Command.Item>
    {/each}
    <Command.Item
      value="new:{repo.id}"
      keywords={[repo.name, "新建工作树", "worktree"]}
      disabled={repo.availability !== "available"}
      data-checked={kind === "new-worktree" && repositoryId === repo.id}
      onSelect={() => chooseNewWorktree(repo)}
    >
      <IconCirclePlus class="text-muted-foreground" aria-hidden="true" />
      <span class="text-sm"
        >{scope?.worktreeId ? "从当前工作树分支新建…" : "新建工作树…"}</span
      >
    </Command.Item>
  </Command.Group>
{/snippet}

<div class="space-y-2">
  <Label for="session-location">位置</Label>
  <PickerField id="session-location" bind:open={() => open, setOpen} {disabled}>
    {#snippet trigger()}
      {#if kind === "directory"}
        <IconFolder
          class="size-4 shrink-0 text-muted-foreground"
          aria-hidden="true"
        />
        <ChoiceDetails primary="直接目录" />
      {:else if kind === "new-worktree"}
        <IconCirclePlus
          class="size-4 shrink-0 text-muted-foreground"
          aria-hidden="true"
        />
        <ChoiceDetails
          primary={repository
            ? `新建工作树 · ${repository.name}`
            : "新建工作树"}
          secondary={newWorktreeSummary}
        />
      {:else}
        <IconGitBranch
          class="size-4 shrink-0 text-muted-foreground"
          aria-hidden="true"
        />
        {#if worktree}
          <ChoiceDetails
            primary={`${repository?.name ?? ""} · ${worktreeLabel(worktree)}`}
            secondary={worktree.path}
          />
        {:else}
          <span class="grid min-w-0 flex-1 gap-0.5">
            <span class="text-sm text-muted-foreground">选择工作树</span>
            {#if repository}<span class="truncate text-xs text-muted-foreground"
                >{repository.name} 没有可用的工作树</span
              >{/if}
          </span>
        {/if}
      {/if}
    {/snippet}
    <Command.Root label="位置" class="h-auto min-w-0">
      <Command.Input
        bind:value={search}
        placeholder="搜索目录或工作树…"
        aria-label="搜索位置"
        autocomplete="off"
        autocapitalize="off"
        autocorrect="off"
        spellcheck={false}
      />
      <Command.List aria-label="位置列表" class="max-h-[min(20rem,50dvh)] p-1">
        <Command.Empty>没有匹配的位置</Command.Empty>
        {#if ordered[0]?.id === preferredRepositoryId}
          {@render treeGroup(ordered[0])}
        {/if}
        {#if recent.length && !scope}
          <Command.Group heading="最近目录" value="recent">
            {#each recent as dir (dir)}
              <Command.Item
                value="dir:{dir}"
                keywords={[dir]}
                data-checked={kind === "directory" && cwd === dir}
                onSelect={() => chooseDirectory(dir)}
              >
                <IconFolder class="text-muted-foreground" aria-hidden="true" />
                <ChoiceDetails primary={directoryName(dir)} secondary={dir} />
              </Command.Item>
            {/each}
          </Command.Group>
        {/if}
        {#each ordered.filter((repo) => repo.id !== preferredRepositoryId) as repo (repo.id)}
          {@render treeGroup(repo)}
        {/each}
        {#if !scope}
          <Command.Group heading="其他" value="other">
            <Command.Item
              value="action:directory"
              keywords={["目录", "directory"]}
              onSelect={typeDirectory}
            >
              <IconPencil class="text-muted-foreground" aria-hidden="true" />
              <span class="text-sm">输入或浏览目录…</span>
            </Command.Item>
          </Command.Group>
        {/if}
      </Command.List>
    </Command.Root>
  </PickerField>
  {#if kind === "directory"}
    <div class="flex gap-2">
      <Input
        id="cwd"
        aria-label="工作目录"
        bind:ref={cwdInput}
        bind:value={cwd}
        mono
        placeholder="~/projects"
        required
        autocomplete="off"
        autocapitalize="off"
        autocorrect="off"
        spellcheck={false}
      />
      <DirectoryPicker bind:value={cwd} />
    </div>
  {/if}
</div>
