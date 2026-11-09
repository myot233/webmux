<script lang="ts">
  import { errorMessage } from "../error";
  import { api } from "../api";
  import { remoteBranchBases, suggestWorktreeBranch } from "../branch-name";
  import {
    commitChoiceLabel,
    commitChoices,
    type CommitChoice,
    type CommitTip,
  } from "../commit-choices";
  import { LatestRequest } from "../hooks/remote-request.svelte";
  import { Badge } from "$lib/components/ui/badge";
  import {
    BranchesSchema,
    CommitsSchema,
    RefreshBranchBaseRequestSchema,
    RefreshBranchBaseResponseSchema,
    type Repository,
  } from "../schemas";
  import { Button } from "$lib/components/ui/button";
  import { Input } from "$lib/components/ui/input";
  import { Label } from "$lib/components/ui/label";
  import * as Select from "$lib/components/ui/select";
  import IconRefresh from "~icons/tabler/refresh";
  import AdvancedSection from "./AdvancedSection.svelte";
  import BranchCommandList from "./BranchCommandList.svelte";
  import IconGitBranch from "~icons/tabler/git-branch";
  import PickerField from "./PickerField.svelte";
  import * as SegmentedControl from "$lib/components/ui/segmented-control";
  import type { z } from "zod";
  let {
    repository,
    path = $bindable(""),
    mode = $bindable<"new" | "existing">("new"),
    branch = $bindable(""),
    base = $bindable(""),
    startBranch = $bindable(""),
    disabled = false,
    preferredStartBranch,
  }: {
    repository: Repository;
    path: string;
    mode: "new" | "existing";
    branch: string;
    base: string;
    /** The local branch a new branch starts from. */
    startBranch?: string;
    disabled?: boolean;
    /** The start branch to use when it is a local branch, else the default. */
    preferredStartBranch?: string;
  } = $props();
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
  function setStartBranchPickerOpen(next: boolean) {
    startBranchPickerOpen = next;
    if (!next) startBranchSearch = "";
  }
  let branchPickerOpen = $state(false);
  let branchSearch = $state("");
  // The desktop list stays mounted while closed; each opening starts unfiltered.
  function setBranchPickerOpen(next: boolean) {
    branchPickerOpen = next;
    if (!next) branchSearch = "";
  }
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

  // Tags for a branch's local tip and, after a fetch, its upstream tip.
  function branchTips(local: string, upstreamHead: string): CommitTip[] {
    if (!local && upstreamHead)
      return [{ hash: upstreamHead, tag: "远程最新" }];
    if (!upstreamHead) return [{ hash: local, tag: "本地最新" }];
    if (upstreamHead === local) return [{ hash: local, tag: "本地与远程最新" }];
    return [
      { hash: upstreamHead, tag: "远程最新" },
      { hash: local, tag: "本地最新" },
    ];
  }

  // Lists the history reachable from the tips, merged newest first. Later
  // calls supersede earlier ones.
  function loadCommits(tips: CommitTip[]) {
    const request = commitRequests.begin();
    const repositoryID = repository.id;
    commitTips = tips;
    commitsLoading = true;
    commitsError = "";
    const query = new URLSearchParams(
      [...new Set(tips.map((tip) => tip.hash))].map((hash) => ["from", hash]),
    );
    void api(
      `/repositories/${repositoryID}/commits?${query}`,
      CommitsSchema,
      "GET",
      undefined,
      request.signal,
    )
      .then((value) => {
        if (request.current()) commits = value.commits;
      })
      .catch((err: unknown) => {
        if (!request.reportable()) return;
        commits = [];
        commitsError = `无法读取最近提交：${errorMessage(err)}`;
      })
      .finally(() => {
        if (request.current()) commitsLoading = false;
      });
  }

  function suggestedBranch(avoid?: string) {
    if (!data) return "";
    return suggestWorktreeBranch({
      branchNames: data.branches.map((item) => item.name),
      worktreePaths: repository.worktrees.map((tree) => tree.path),
      avoid,
    });
  }
  function fillSuggestedBranch() {
    if (mode !== "new" || branchEdited || branch || !data) return;
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
    void api(
      `/repositories/${repositoryID}/branches/refresh`,
      RefreshBranchBaseResponseSchema,
      "POST",
      { branch: branchName },
      request.signal,
      RefreshBranchBaseRequestSchema,
    )
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
        // A remote base descends from the local tip; a local base is the
        // freshly read tip. Either way the fetched upstream tip is listed, so
        // a diverged upstream stays selectable without becoming the default.
        const localBranch = data?.branches.find(
          (item) => item.name === branchName,
        );
        const localHead =
          value.baseSource === "local" ? value.base : (localBranch?.head ?? "");
        if (editVersion === baseEditVersion) {
          base = value.base;
          baseSource = value.baseSource;
          baseSubject = value.baseSubject;
        }
        loadCommits(branchTips(localHead, value.upstreamHead));
        if (!value.fetchWarning) {
          void api(
            `/repositories/${repositoryID}/branches`,
            BranchesSchema,
            "GET",
            undefined,
            request.signal,
          )
            .then((next) => {
              if (request.current() && repository.id === repositoryID)
                data = next;
            })
            .catch(() => {});
        }
      })
      .catch((err: unknown) => {
        if (!request.reportable()) return;
        if (err instanceof DOMException && err.name === "AbortError") return;
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
    const selected = data?.branches.find((item) => item.name === name);
    if (!selected) {
      const remote = remoteBranches.find((item) => item.name === name);
      if (remote) {
        base = remote.head;
        baseSource = "remote";
        upstream = remote.name;
        baseSubject = remote.subject;
        loadCommits(branchTips("", remote.head));
        refreshUpstream(name);
        return;
      }
      base = data?.defaultBase || "";
      baseSource = "";
      baseSubject = data?.defaultBaseSubject || "";
      if (base) loadCommits([{ hash: base, tag: "当前主工作区" }]);
      else clearCommits();
      return;
    }
    base = selected.head;
    baseSource = "local";
    baseSubject =
      data?.bases.find((item) => item.name === `refs/heads/${name}`)?.subject ||
      "";
    loadCommits(branchTips(selected.head, ""));
    refreshUpstream(name);
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
    branch = "";
    base = "";
    void api(`/repositories/${id}/branches`, BranchesSchema)
      .then((value) => {
        if (!active) return;
        data = value;
        base = value.defaultBase || "";
        baseSubject = value.defaultBaseSubject;
        fillSuggestedBranch();
        startBranch =
          preferredStartBranch &&
          value.branches.some((item) => item.name === preferredStartBranch)
            ? preferredStartBranch
            : value.defaultBranch || "";
        if (mode === "new") chooseStartBranch(startBranch);
      })
      .catch((err) => {
        if (active) error = errorMessage(err);
      })
      .finally(() => {
        if (active) loading = false;
      });
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
  const ready = $derived(
    !!data && !loading && !!branch && !!path && (mode === "existing" || !!base),
  );
  export function isReady() {
    return ready;
  }
</script>

{#snippet commitDetails(item: CommitChoice)}
  <span class="grid min-w-0 flex-1 gap-0.5">
    <span class="truncate text-sm font-medium" title={item.subject}
      >{item.subject}</span
    >
    <span
      class="flex min-w-0 flex-wrap items-center gap-1.5 text-xs text-muted-foreground"
    >
      <span class="font-mono">{item.shortHash}</span>
      {#each item.tags as tag (tag)}
        <Badge variant={tag === "远程最新" ? "info" : "secondary"}>{tag}</Badge>
      {/each}
      {#if item.age}<span>{item.age}</span>{/if}
    </span>
  </span>
{/snippet}

<div class="space-y-3 rounded-lg border p-3">
  <SegmentedControl.Root
    value={mode}
    onValueChange={(next) => {
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
    }}
    aria-label="工作树分支"
  >
    <SegmentedControl.Item value="new">创建新分支</SegmentedControl.Item>
    <SegmentedControl.Item value="existing">使用已有分支</SegmentedControl.Item>
  </SegmentedControl.Root>
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
            branches={(data?.branches ?? []).map((item) => ({
              name: item.name,
            }))}
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
        {#if refreshing}
          <p
            role="status"
            aria-label="上游刷新状态"
            class="text-xs text-muted-foreground"
          >
            正在获取 {startBranch} 对应的上游最新提交；可继续编辑或创建。
          </p>
        {:else if baseSource === "remote" && upstream}
          <p
            role="status"
            aria-label="上游刷新状态"
            class="break-all text-xs text-muted-foreground"
          >
            远程最新 · {upstream} · {baseSubject || "无标题提交"}
            {#if lastRefreshedAt}
              · 最近刷新 {lastRefreshedAt.toLocaleTimeString()}{/if}
          </p>
        {:else if baseSource === "local" && startBranch}
          <p
            role="status"
            aria-label="上游刷新状态"
            class="break-all text-xs text-muted-foreground"
          >
            本地：{startBranch}
          </p>
        {/if}
        {#if refreshWarning}
          <p
            role="status"
            aria-label="上游刷新警告"
            class="text-xs text-warning"
          >
            {refreshWarning}
          </p>
        {/if}
      </div>
      <div class="min-w-0 space-y-2">
        <div class="flex items-center justify-between gap-2">
          <Label for="worktree-branch">新分支名称</Label>
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={!data || loading}
            onclick={() => {
              branch = suggestedBranch(branch || undefined);
            }}>换一个</Button
          >
        </div>
        <Input
          id="worktree-branch"
          bind:value={branch}
          oninput={() => {
            branchEdited = true;
          }}
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
          branches={(data?.branches ?? []).map((item) => ({
            name: item.name,
            reason: item.worktreePaths.length ? "已被工作树占用" : undefined,
          }))}
          selected={branch}
          {loading}
          failed={!!error}
          {disabled}
          bind:search={branchSearch}
          onselect={(name) => {
            branch = name;
            setBranchPickerOpen(false);
          }}
        />
      </PickerField>
    </div>
  {/if}
  {#if mode === "new" && data && !commitRows.length && !commitsLoading}<p
      class="text-sm text-muted-foreground"
    >
      尚无可用提交。请先在终端创建提交；仍可在主工作区启动会话。
    </p>{/if}
  <AdvancedSection
    title={mode === "new" ? "高级：起始提交与存放目录" : "高级：存放目录"}
    summary={[mode === "new" ? selectedCommit?.shortHash : "", path]
      .filter(Boolean)
      .join(" · ")}
  >
    {#if mode === "new"}
      <div class="space-y-2">
        <Label for="worktree-base">起始提交</Label>
        <Select.Root
          type="single"
          bind:value={base}
          disabled={disabled || loading || !data || !commitRows.length}
          onValueChange={() => {
            baseEditVersion += 1;
            baseSource = "";
            baseSubject = "";
            upstream = "";
            lastRefreshedAt = null;
          }}
        >
          <Select.Trigger
            id="worktree-base"
            aria-describedby="worktree-base-help"
          >
            {#if selectedCommit}
              {@render commitDetails(selectedCommit)}
            {:else}
              <span class="text-muted-foreground">
                {commitsLoading ? "正在读取最近提交…" : "选择提交"}
              </span>
            {/if}
          </Select.Trigger>
          <Select.Content
            class="max-h-[min(24rem,var(--bits-select-content-available-height))]"
          >
            {#each commitRows as item (item.value)}
              <Select.Item
                value={item.value}
                label={item.subject}
                aria-label={commitChoiceLabel(item)}
              >
                {@render commitDetails(item)}
              </Select.Item>
            {/each}
          </Select.Content>
        </Select.Root>
        <p id="worktree-base-help" class="text-xs text-muted-foreground">
          {#if commitsLoading}正在读取最近提交…{:else}所选分支最近 {commits.length}
            条提交；“刷新上游”会加入远程最新提交。{/if}
        </p>
        {#if commitsError}<p role="status" class="text-xs text-warning">
            {commitsError}
          </p>{/if}
      </div>
    {/if}
    <div class="space-y-2">
      <Label for="worktree-path">新工作树完整路径</Label>
      <Input
        id="worktree-path"
        bind:value={path}
        oninput={() => {
          pathEdited = true;
        }}
        mono
        required
        autocomplete="off"
        autocapitalize="off"
        autocorrect="off"
        spellcheck={false}
      />
      <p class="text-xs text-muted-foreground">
        目录名默认取分支最后一段，不会覆盖已有目录。
      </p>
    </div>
  </AdvancedSection>
</div>
