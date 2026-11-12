<script lang="ts">
  import { errorMessage } from "../../platform/error";
  import {
    remoteBranchBases,
    suggestWorktreeBranch,
  } from "../../domains/workspace/branch-name";
  import {
    branchTips,
    commitChoices,
    fetchRepositoryBranches,
    fetchRepositoryCommits,
    formatCommitLoadError,
    isAbortError,
    refreshRepositoryBranchBase,
    resolveInitialStartBranch,
    resolveRefreshedLocalHead,
    resolveSelectedStartBranchState,
    resolveWorktreePresetSelection,
    type CommitTip,
  } from "../../domains/workspace/commit-choices";
  import { LatestRequest } from "../../hooks/shared/remote-request.svelte";
  import type { WorktreePreset } from "../../domains/sessions/worktree-preset";
  import {
    BranchesSchema,
    CommitsSchema,
    type Repository,
  } from "../../contracts/schemas";
  import { Button } from "$lib/components/ui/button";
  import { Input } from "$lib/components/ui/input";
  import { Label } from "$lib/components/ui/label";
  import IconRefresh from "~icons/tabler/refresh";
  import BranchCommandList from "./BranchCommandList.svelte";
  import IconGitBranch from "~icons/tabler/git-branch";
  import PickerField from "../shared/PickerField.svelte";
  import WorktreeAdvancedFields from "./WorktreeAdvancedFields.svelte";
  import WorktreeBaseStatus from "./WorktreeBaseStatus.svelte";
  import * as SegmentedControl from "$lib/components/ui/segmented-control";
  import { untrack } from "svelte";
  import type { z } from "zod";
  let {
    repository,
    deferred = false,
    path = $bindable(""),
    mode = $bindable<"new" | "existing">("new"),
    branch = $bindable(""),
    base = $bindable(""),
    startBranch = $bindable(""),
    disabled = false,
    preferredStartBranch,
    preset,
  }: {
    repository: Repository;
    /** Handoff reserves a fresh branch and assigns its directory on send. */
    deferred?: boolean;
    path: string;
    mode: "new" | "existing";
    branch: string;
    base: string;
    /** The local branch a new branch starts from. */
    startBranch?: string;
    disabled?: boolean;
    /** The start branch to use when it is a local branch, else the default. */
    preferredStartBranch?: string;
    /** Initial values, applied once when the branches are first read. */
    preset?: WorktreePreset;
  } = $props();
  // The preset sets the mode at once so the form never shows the other one
  // first; its branches wait for the list.
  let pendingPreset = untrack(() => preset);
  if (pendingPreset) mode = pendingPreset.mode;
  let data = $state<z.infer<typeof BranchesSchema> | null>(null);
  let error = $state("");
  let loading = $state(false);
  let refreshing = $state(false);
  let refreshWarning = $state("");
  let upstream = $state("");
  let baseSource = $state<"local" | "remote" | "">("");
  let baseSubject = $state("");
  let lastRefreshedAt = $state<Date | null>(null);
  const refreshRequests = new LatestRequest();
  let baseEditVersion = 0;
  let pathEdited = $state(false);
  let startBranchPickerOpen = $state(false);
  let startBranchSearch = $state("");
  const setStartBranchPickerOpen = (next: boolean) => {
    startBranchPickerOpen = next;
    if (!next) startBranchSearch = "";
  };
  let branchPickerOpen = $state(false);
  let branchSearch = $state("");
  const setBranchPickerOpen = (next: boolean) => {
    branchPickerOpen = next;
    if (!next) branchSearch = "";
  };
  let branchEdited = $state(false);
  let commits = $state<z.infer<typeof CommitsSchema>["commits"]>([]);
  let commitsLoading = $state(false);
  let commitsError = $state("");
  const commitRequests = new LatestRequest();
  // Tips the history was read from; their rows carry these tags.
  let commitTips = $state<CommitTip[]>([]);
  const remoteBranches = $derived(
    data
      ? remoteBranchBases(
          data.bases,
          data.branches.map((item) => item.name),
        )
      : [],
  );
  const commitRows = $derived(
    commitChoices(commits, { tips: commitTips, base, baseSubject }),
  );
  const selectedCommit = $derived(
    commitRows.find((item) => item.value === base),
  );

  function clearCommits() {
    commitRequests.cancel();
    commitsLoading = false;
    commitsError = "";
  }

  // Lists the history reachable from the tips, merged newest first. Later
  // calls supersede earlier ones.
  function loadCommits(tips: CommitTip[]) {
    const request = commitRequests.begin();
    commitTips = tips;
    commitsLoading = true;
    commitsError = "";
    void fetchRepositoryCommits(repository.id, tips, request.signal)
      .then((value) => {
        if (request.current()) commits = value.commits;
      })
      .catch((err: unknown) => {
        if (!request.reportable()) return;
        commits = [];
        commitsError = formatCommitLoadError(err);
      })
      .finally(() => {
        if (request.current()) commitsLoading = false;
      });
  }

  const suggestedBranch = (avoid?: string) =>
    data
      ? suggestWorktreeBranch({
          branchNames: data.branches.map((item) => item.name),
          worktreePaths: repository.worktrees.map((tree) => tree.path),
          avoid,
        })
      : "";
  function fillSuggestedBranch() {
    if (mode === "new" && !branchEdited && !branch && data)
      branch = suggestedBranch();
  }

  function clearRefresh() {
    refreshRequests.cancel();
    refreshing = false;
    refreshWarning = "";
    upstream = "";
    lastRefreshedAt = null;
  }

  function refreshUpstream(branchName: string) {
    const request = refreshRequests.begin();
    const editVersion = baseEditVersion;
    const repositoryID = repository.id;
    refreshing = true;
    refreshWarning = "";
    void refreshRepositoryBranchBase(repositoryID, branchName, request.signal)
      .then((value) => {
        if (
          !request.current() ||
          mode !== "new" ||
          repository.id !== repositoryID ||
          startBranch !== branchName
        )
          return;
        upstream = value.upstream;
        refreshWarning = value.fetchWarning;
        lastRefreshedAt = value.baseSource === "remote" ? new Date() : null;
        const localHead = resolveRefreshedLocalHead(value, data, branchName);
        if (editVersion === baseEditVersion) {
          base = value.base;
          baseSource = value.baseSource;
          baseSubject = value.baseSubject;
        }
        loadCommits(branchTips(localHead, value.upstreamHead));
        if (!value.fetchWarning)
          void fetchRepositoryBranches(repositoryID, request.signal)
            .then((next) => {
              if (request.current() && repository.id === repositoryID)
                data = next;
            })
            .catch(() => {});
      })
      .catch((err: unknown) => {
        if (!request.reportable() || isAbortError(err)) return;
        refreshWarning = "上游刷新失败，保留当前基线；请重试";
        baseSource = "";
        upstream = "";
        lastRefreshedAt = null;
      })
      .finally(() => {
        if (request.current()) refreshing = false;
      });
  }

  function chooseStartBranch(name: string) {
    startBranch = name;
    clearRefresh();
    const resolved = resolveSelectedStartBranchState(
      name,
      data,
      remoteBranches,
    );
    base = resolved.base;
    baseSubject = resolved.baseSubject;
    if (resolved.kind === "remote") {
      baseSource = "remote";
      upstream = resolved.upstream;
      loadCommits(resolved.tips);
      refreshUpstream(name);
      return;
    }
    if (resolved.kind === "local") {
      baseSource = "local";
      loadCommits(resolved.tips);
      refreshUpstream(name);
      return;
    }
    baseSource = "";
    if (resolved.tips.length) loadCommits(resolved.tips);
    else clearCommits();
  }

  $effect(() => {
    const id = repository.id;
    let active = true;
    clearRefresh();
    clearCommits();
    commits = [];
    data = null;
    startBranch = "";
    baseSource = "";
    baseSubject = "";
    error = "";
    loading = true;
    pathEdited = false;
    branchEdited = false;
    branch = pendingPreset?.mode === "new" ? (pendingPreset.name ?? "") : "";
    base = pendingPreset?.base ?? "";
    void fetchRepositoryBranches(id)
      .then((value) => {
        if (!active) return;
        data = value;
        base = value.defaultBase || "";
        baseSubject = value.defaultBaseSubject;
        const initial = pendingPreset;
        pendingPreset = undefined;
        const applied = initial
          ? resolveWorktreePresetSelection(initial, value)
          : null;
        if (applied) {
          branch = applied.branch;
          branchEdited = applied.branchEdited;
        }
        fillSuggestedBranch();
        startBranch = resolveInitialStartBranch(
          value,
          remoteBranches,
          initial,
          preferredStartBranch,
        );
        if (mode === "new" && initial?.base) {
          base = initial.base;
          baseSubject = "已保存的起始提交";
          loadCommits([{ hash: base, tag: "已保存的起始提交" }]);
        } else if (mode === "new") chooseStartBranch(startBranch);
      })
      .catch((err) => active && (error = errorMessage(err)))
      .finally(() => active && (loading = false));
    return () => {
      active = false;
      clearRefresh();
      clearCommits();
    };
  });
  $effect(() => {
    if (!pathEdited)
      path = data
        ? `${data.defaultParent}/${branch.split("/").at(-1) || ""}`
        : "";
  });
  const mappedLocalBranches = $derived(
    (data?.branches ?? []).map((item) => ({ name: item.name })),
  );
  const mappedExistingBranches = $derived(
    (data?.branches ?? []).map((item) => ({
      name: item.name,
      reason: item.worktreePaths.length ? "已被工作树占用" : undefined,
    })),
  );
  function handleModeChange(next: string | undefined) {
    if (next === "new" && mode !== "new") {
      mode = "new";
      setBranchPickerOpen(false);
      setStartBranchPickerOpen(false);
      branchEdited = false;
      branch = "";
      fillSuggestedBranch();
      chooseStartBranch(startBranch);
    } else if (next === "existing" && mode !== "existing") {
      mode = "existing";
      setStartBranchPickerOpen(false);
      clearRefresh();
      clearCommits();
      baseSource = "";
      branch = "";
    }
  }
  const ready = $derived(
    !!data && !loading && !!branch && !!path && (mode === "existing" || !!base),
  );
  export const isReady = () => ready;
</script>

<div class="space-y-3 rounded-lg border p-3">
  {#if !deferred}
    <SegmentedControl.Root
      value={mode}
      onValueChange={handleModeChange}
      aria-label="工作树分支"
    >
      <SegmentedControl.Item value="new">创建新分支</SegmentedControl.Item>
      <SegmentedControl.Item value="existing"
        >使用已有分支</SegmentedControl.Item
      >
    </SegmentedControl.Root>
  {/if}
  {#if loading}<p role="status" class="text-sm">正在读取本地分支…</p>{/if}
  {#if error}<p role="alert" class="break-all text-sm text-destructive">
      {error}
    </p>{/if}
  {#if mode === "new"}
    <div class="grid gap-3 sm:grid-cols-2">
      <div class="min-w-0 space-y-2">
        <div class="flex items-center justify-between gap-2">
          <Label for="worktree-start-branch">起始分支</Label>
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={!startBranch || refreshing || disabled}
            aria-busy={refreshing}
            onclick={() => refreshUpstream(startBranch)}
            ><IconRefresh
              class={refreshing
                ? "animate-spin motion-reduce:animate-none"
                : ""}
              aria-hidden="true"
            />{refreshing ? "正在刷新" : "刷新上游"}</Button
          >
        </div>
        <PickerField
          id="worktree-start-branch"
          bind:open={() => startBranchPickerOpen, setStartBranchPickerOpen}
          disabled={disabled || loading || !data}
        >
          {#snippet trigger()}
            {#if startBranch}
              <IconGitBranch
                class="size-4 shrink-0 text-muted-foreground"
                aria-hidden="true"
              />
              <span class="min-w-0 flex-1 truncate" title={startBranch}
                >{startBranch}</span
              >
            {:else}
              <span class="text-sm text-muted-foreground">
                {loading ? "正在读取本地分支…" : "选择起始分支"}
              </span>
            {/if}
          {/snippet}
          <BranchCommandList
            label="起始分支"
            branches={mappedLocalBranches}
            remoteBranches={remoteBranches.map((item) => ({ name: item.name }))}
            selected={startBranch}
            {loading}
            failed={!!error}
            {disabled}
            bind:search={startBranchSearch}
            onselect={(name) => {
              chooseStartBranch(name);
              setStartBranchPickerOpen(false);
            }}
          />
        </PickerField>
        <WorktreeBaseStatus
          {refreshing}
          {startBranch}
          {baseSource}
          {upstream}
          {baseSubject}
          {lastRefreshedAt}
          {refreshWarning}
        />
      </div>
      <div class="min-w-0 space-y-2">
        <div class="flex items-center justify-between gap-2">
          <Label for="worktree-branch">新分支名称</Label>
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={disabled || !data || loading}
            onclick={() => (branch = suggestedBranch(branch || undefined))}
            >换一个</Button
          >
        </div>
        <Input
          id="worktree-branch"
          disabled={disabled || loading || !data}
          bind:value={branch}
          oninput={() => (branchEdited = true)}
          required
          placeholder="feature/my-task"
          autocomplete="off"
          autocapitalize="off"
          autocorrect="off"
          spellcheck={false}
        />
      </div>
    </div>
  {:else}
    <div class="space-y-2">
      <Label for="existing-branch">已有本地分支</Label>
      <PickerField
        id="existing-branch"
        bind:open={() => branchPickerOpen, setBranchPickerOpen}
        disabled={disabled || loading || !data}
      >
        {#snippet trigger()}
          {#if branch}
            <IconGitBranch
              class="size-4 shrink-0 text-muted-foreground"
              aria-hidden="true"
            />
            <span class="min-w-0 flex-1 truncate" title={branch}>{branch}</span>
          {:else}
            <span class="text-sm text-muted-foreground">
              {loading ? "正在读取本地分支…" : "选择本地分支"}
            </span>
          {/if}
        {/snippet}
        <BranchCommandList
          label="已有本地分支"
          branches={mappedExistingBranches}
          selected={branch}
          {loading}
          failed={!!error}
          {disabled}
          bind:search={branchSearch}
          onselect={(name) => ((branch = name), setBranchPickerOpen(false))}
        />
      </PickerField>
    </div>
  {/if}
  <WorktreeAdvancedFields
    {mode}
    {deferred}
    bind:path
    bind:base
    {disabled}
    {loading}
    hasData={!!data}
    {commitRows}
    {selectedCommit}
    commitsCount={commits.length}
    {commitsLoading}
    {commitsError}
    onpathedited={() => (pathEdited = true)}
    onbaseedited={() => {
      baseEditVersion += 1;
      baseSource = "";
      baseSubject = "";
      upstream = "";
      lastRefreshedAt = null;
    }}
  />
</div>
