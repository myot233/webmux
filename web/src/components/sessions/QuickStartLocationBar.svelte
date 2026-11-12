<script lang="ts">
  import { untrack } from "svelte";
  import IconBook2 from "~icons/tabler/book-2";
  import IconCirclePlus from "~icons/tabler/circle-plus";
  import IconGitBranch from "~icons/tabler/git-branch";
  import * as Command from "$lib/components/ui/command";
  import * as Popover from "$lib/components/ui/popover";
  import * as SegmentedControl from "$lib/components/ui/segmented-control";
  import { api } from "../../platform/api";
  import ChoiceDetails from "../shared/ChoiceDetails.svelte";
  import ChatSelect from "../chat/ChatSelect.svelte";
  import QuietTrigger from "../chat/QuietTrigger.svelte";
  import BranchCommandList from "../workspace/BranchCommandList.svelte";
  import { errorMessage } from "../../platform/error";
  import {
    selectableWorktree,
    worktreeLabel,
  } from "../../domains/sessions/new-session-defaults";
  import {
    resolveQuickStartBranches,
    resolveQuickStartWorktree,
    type QuickStartBranches,
    type QuickStartTargetMode,
  } from "../../hooks/sessions/quick-start";
  import {
    BranchesSchema,
    type Repository,
    type Worktree,
  } from "../../contracts/schemas";

  let {
    compact = false,
    repositories,
    repository,
    sending,
    targetModeChoice,
    worktreeChoice,
    baseChoice,
    uploadError,
    targetMode = $bindable<QuickStartTargetMode>("new"),
    selectedWorktree = $bindable<Worktree | undefined>(),
    branchData = $bindable<QuickStartBranches | undefined>(),
    branchError = $bindable<string | undefined>(),
    base = $bindable(""),
    onrepositorychange,
    ontargetmodechange,
    onselectworktree,
    onselectbranch,
  }: {
    compact?: boolean;
    repositories: readonly Repository[];
    repository?: Repository;
    sending: boolean;
    targetModeChoice?: QuickStartTargetMode;
    worktreeChoice?: { repositoryId: string; worktreeId: string };
    baseChoice?: { repositoryId: string; name: string };
    uploadError: string;
    targetMode?: QuickStartTargetMode;
    selectedWorktree?: Worktree;
    branchData?: QuickStartBranches;
    branchError?: string;
    base?: string;
    onrepositorychange: (id: string) => void;
    ontargetmodechange: (mode: QuickStartTargetMode) => void;
    onselectworktree: (worktreeId: string) => void;
    onselectbranch: (name: string) => void;
  } = $props();

  let branches = $state<{
    id: string;
    data?: QuickStartBranches;
    error?: string;
  }>();
  const repositoryId = $derived(repository?.id);
  $effect(() => {
    const id = repositoryId;
    if (!id || untrack(() => branches?.id) === id) return;
    branches = { id };
    const controller = new AbortController();
    api(
      `/repositories/${id}/branches`,
      BranchesSchema,
      "GET",
      undefined,
      controller.signal,
    )
      .then((data) => {
        if (branches?.id === id) branches = { id, data };
      })
      .catch((err: unknown) => {
        if (branches?.id === id && !controller.signal.aborted)
          branches = { id, error: errorMessage(err) };
      });
    return () => controller.abort();
  });

  const resolvedBranchData = $derived(
    branches?.id === repository?.id ? branches?.data : undefined,
  );
  const resolvedBranches = $derived(
    resolveQuickStartBranches(resolvedBranchData, repository?.id, baseChoice),
  );
  const localBranches = $derived(resolvedBranches.localBranches);
  const remoteBranches = $derived(resolvedBranches.remoteBranches);
  const baseName = $derived(resolvedBranches.baseName);
  const resolvedWorktree = $derived(
    resolveQuickStartWorktree(repository, worktreeChoice),
  );
  const worktrees = $derived(resolvedWorktree.worktrees);
  const defaultWorktree = $derived(resolvedWorktree.defaultWorktree);
  const resolvedSelectedWorktree = $derived(resolvedWorktree.selectedWorktree);
  const resolvedTargetMode = $derived<QuickStartTargetMode>(
    targetModeChoice === "existing" && resolvedSelectedWorktree
      ? "existing"
      : "new",
  );
  const branchLoading = $derived(!resolvedBranchData && !branches?.error);
  const branchFailed = $derived(!!branches?.error);

  $effect.pre(() => {
    targetMode = resolvedTargetMode;
    selectedWorktree = resolvedSelectedWorktree;
    branchData = resolvedBranchData;
    branchError = branches?.id === repository?.id ? branches?.error : undefined;
    base = resolvedBranches.base;
  });

  let basePickerOpen = $state(false);
  let baseSearch = $state("");
  let worktreeSearch = $state("");
  const baseBranchesCount = $derived(
    localBranches.length + remoteBranches.length,
  );
  const usableRepository = (repo: Repository) =>
    repo.availability === "available";

  function setBasePickerOpen(next: boolean) {
    basePickerOpen = next;
    if (!next) {
      baseSearch = "";
      worktreeSearch = "";
    }
  }
</script>

<div
  class={[
    "-my-1 flex min-w-0 flex-nowrap items-center gap-0.5 overflow-x-auto px-1 py-1 [scrollbar-width:none]",
    compact && "composer-mobile:hidden",
  ]}
  role="toolbar"
  aria-label="仓库与分支"
>
  <ChatSelect
    labeled
    name="仓库"
    value={repository?.id ?? ""}
    choices={repositories.map((repo) => ({
      value: repo.id,
      name: repo.name,
      reason: usableRepository(repo)
        ? undefined
        : repo.error || "仓库当前不可用",
    }))}
    disabled={sending}
    onchange={onrepositorychange}
  >
    {#snippet icon()}<IconBook2 aria-hidden="true" />{/snippet}
  </ChatSelect>
  <Popover.Root bind:open={() => basePickerOpen, setBasePickerOpen}>
    <Popover.Trigger
      disabled={sending || (!baseBranchesCount && !defaultWorktree)}
    >
      {#snippet child({ props })}
        {@const treeLabel = selectedWorktree
          ? worktreeLabel(selectedWorktree)
          : ""}
        <QuietTrigger
          {props}
          label={targetMode === "existing"
            ? `工作树：${treeLabel}`
            : `基准分支：${baseName}`}
          title={targetMode === "existing"
            ? selectedWorktree
              ? `工作树：${treeLabel}（${selectedWorktree.path}）`
              : "工作树"
            : baseName
              ? `基准分支：${baseName}（发送后新建 worktree）`
              : "基准分支"}
          text={targetMode === "existing" ? treeLabel : baseName}
        >
          {#snippet icon()}{#if targetMode === "existing"}<IconGitBranch
                aria-hidden="true"
              />{:else}<IconCirclePlus aria-hidden="true" />{/if}{/snippet}
        </QuietTrigger>
      {/snippet}
    </Popover.Trigger>
    <Popover.Content align="start" class="w-72 gap-0 overflow-hidden p-0">
      <div class="border-b px-2 py-1.5">
        <SegmentedControl.Root
          value={targetMode}
          onValueChange={(next) => {
            if (next === "new") ontargetmodechange("new");
            else if (next === "existing" && defaultWorktree)
              ontargetmodechange("existing");
          }}
          aria-label="工作树方式"
          class="w-full"
        >
          <SegmentedControl.Item value="new" disabled={sending}
            >新建 worktree</SegmentedControl.Item
          >
          <SegmentedControl.Item
            value="existing"
            disabled={sending || !defaultWorktree}
            >已有工作树</SegmentedControl.Item
          >
        </SegmentedControl.Root>
      </div>
      {#if targetMode === "existing"}
        <Command.Root
          label="工作树"
          class="h-auto min-w-0"
          filter={(value, query, keywords) =>
            [value, ...(keywords ?? [])].some((item) =>
              item.toLowerCase().includes(query.trim().toLowerCase()),
            )
              ? 1
              : 0}
          disableInitialScroll
        >
          <Command.Input
            bind:value={worktreeSearch}
            placeholder="搜索工作树…"
            aria-label="搜索工作树"
            autocomplete="off"
            autocapitalize="off"
            autocorrect="off"
            spellcheck={false}
          />
          <Command.List
            aria-label="工作树列表"
            class="max-h-[min(14rem,50dvh)] p-1"
          >
            <Command.Empty>没有匹配的工作树</Command.Empty>
            {#each worktrees as tree (tree.id)}
              {@const label = worktreeLabel(tree)}
              {@const usable = selectableWorktree(tree)}
              <Command.Item
                value={tree.id}
                keywords={[label, tree.path]}
                disabled={sending || !usable}
                data-checked={selectedWorktree?.id === tree.id}
                onSelect={() => {
                  onselectworktree(tree.id);
                  setBasePickerOpen(false);
                }}
              >
                <IconGitBranch
                  class="text-muted-foreground"
                  aria-hidden="true"
                />
                <ChoiceDetails
                  primary={label}
                  secondary={usable ? tree.path : tree.error || "工作树不可用"}
                />
              </Command.Item>
            {/each}
          </Command.List>
        </Command.Root>
      {:else}
        <BranchCommandList
          label="基准分支"
          branches={localBranches}
          {remoteBranches}
          selected={baseName}
          loading={branchLoading}
          failed={branchFailed}
          disabled={sending}
          bind:search={baseSearch}
          onselect={(name) => {
            onselectbranch(name);
            setBasePickerOpen(false);
          }}
        />
      {/if}
    </Popover.Content>
  </Popover.Root>
</div>

{#if uploadError}<p role="alert" class="px-1 text-sm text-destructive">
    {uploadError}
  </p>{/if}
