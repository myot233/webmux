<script lang="ts" module>
  import {
    previewSwitch,
    repositoryBranches,
    repositoryCommits,
    switchWorktree,
  } from "../../platform/api";
  import type {
    Branches,
    Commit,
    SwitchPreview,
    SwitchRequest,
    SwitchTarget,
    Worktree,
  } from "../../contracts/schemas";

  /** Where the switch reads and writes; stories pass fixtures. */
  export interface SwitchSource {
    branches(id: string, signal: AbortSignal): Promise<Branches>;
    commits(
      id: string,
      from: readonly string[],
      signal: AbortSignal,
    ): Promise<{ commits: Commit[] }>;
    preview(
      tree: Worktree,
      target: SwitchTarget,
      signal: AbortSignal,
    ): Promise<SwitchPreview>;
    switch(tree: Worktree, request: SwitchRequest): Promise<Worktree>;
  }
  export const apiSwitchSource: SwitchSource = {
    branches: repositoryBranches,
    commits: repositoryCommits,
    preview: previewSwitch,
    switch: switchWorktree,
  };
</script>

<script lang="ts">
  import { onDestroy, untrack } from "svelte";
  import IconGitBranch from "~icons/tabler/git-branch";
  import { Button } from "$lib/components/ui/button";
  import { Input } from "$lib/components/ui/input";
  import { Label } from "$lib/components/ui/label";
  import * as SegmentedControl from "$lib/components/ui/segmented-control";
  import { APIError } from "../../platform/api";
  import { commitChoices } from "../../domains/workspace/commit-choices";
  import { errorMessage } from "../../platform/error";
  import { LatestRequest } from "../../hooks/shared/remote-request.svelte";
  import type { WorktreePreset } from "../../domains/sessions/worktree-preset";
  import type { Repository } from "../../contracts/schemas";
  import {
    buildSwitchTarget,
    canConfirmSwitch,
    deriveSwitchBranchItems,
    resolveSwitchNewWorktreePreset,
    resolveSwitchOccupantPath,
    suggestSwitchBranchName,
  } from "../../domains/workspace/worktree-sync";
  import BranchCommandList from "./BranchCommandList.svelte";
  import PickerField from "../shared/PickerField.svelte";
  import PreviewForm from "./PreviewForm.svelte";
  import SwitchBranchCommitSelect from "./SwitchBranchCommitSelect.svelte";
  import SwitchBranchPreview from "./SwitchBranchPreview.svelte";

  // Switching a worktree's branch in place: choose an existing branch, a new
  // one or a commit to detach to; every choice is previewed by the server
  // (blockers, warnings, the sessions that keep running) and only a clean
  // preview can be confirmed. The same content sits in a dialog (home page,
  // sidebar) or in a repository panel subview; the host closes or goes back.
  let {
    repository,
    worktree,
    source = apiSwitchSource,
    busy = $bindable(false),
    ondone,
    onchange,
    onnewworktree,
    ongoto,
    oncancel,
  }: {
    repository: Repository;
    worktree: Worktree;
    source?: SwitchSource;
    /** The switch runs; the host blocks closing and going back. */
    busy?: boolean;
    /** The switch finished; the worktree as it is now. */
    ondone: (tree: Worktree) => void;
    /** A switch ran, whatever its outcome; the repositories are read again. */
    onchange: () => void;
    /** Opens the chosen target in a new worktree instead. */
    onnewworktree: (preset: WorktreePreset) => void;
    /** Shows the worktree at a path: the one that has the target checked out. */
    ongoto: (path: string) => void;
    oncancel: () => void;
  } = $props();

  type Mode = "existing" | "new" | "detach";
  let mode = $state<Mode>("existing");

  let data = $state<Branches | null>(null);
  let loadError = $state("");
  const branchRequests = new LatestRequest();
  async function readBranches() {
    const request = branchRequests.begin();
    loadError = "";
    try {
      const result = await source.branches(repository.id, request.signal);
      if (!request.current()) return;
      data = result;
      if (mode === "new" && !newName) newName = suggestion();
    } catch (err) {
      if (request.reportable()) loadError = errorMessage(err);
    }
  }

  const branchItems = $derived(
    deriveSwitchBranchItems(data, worktree.path, worktree.branch),
  );
  const localNames = $derived(branchItems.localNames);
  const remotes = $derived(branchItems.remotes);
  const localItems = $derived(branchItems.localItems);
  const remoteItems = $derived(branchItems.remoteItems);

  const localBranchList = $derived(
    (data?.branches ?? []).map((item) => ({ name: item.name })),
  );
  const remoteBranchList = $derived(
    remotes.map((item) => ({ name: item.name })),
  );
  let chosen = $state<{ kind: "local" | "remote"; name: string } | null>(null);
  let branchOpen = $state(false);
  let branchSearch = $state("");
  const setBranchOpen = (next: boolean) => (
    (branchOpen = next),
    !next && (branchSearch = "")
  );
  const selectBranch = (name: string) => (
    (chosen = { kind: localNames.has(name) ? "local" : "remote", name }),
    setBranchOpen(false)
  );
  let newName = $state("");
  let startPoint = $state(
    untrack(() => worktree.branch ?? worktree.head ?? ""),
  );
  let startOpen = $state(false);
  let startSearch = $state("");
  const setStartOpen = (next: boolean) => (
    (startOpen = next),
    !next && (startSearch = "")
  );
  const suggestion = (avoid?: string) =>
    suggestSwitchBranchName(localNames, repository, avoid);
  const nameTaken = $derived(localNames.has(newName.trim()));
  const startIsBranch = $derived(
    localNames.has(startPoint) || remotes.some((r) => r.name === startPoint),
  );
  let commits = $state<Commit[]>([]);
  let commitsRead = $state(false);
  let commitsError = $state("");
  let commit = $state("");
  const commitRequests = new LatestRequest();
  async function readCommits() {
    const request = commitRequests.begin();
    commitsError = "";
    try {
      const result = worktree.head
        ? await source.commits(repository.id, [worktree.head], request.signal)
        : { commits: [] };
      if (request.current()) commits = result.commits;
    } catch (err) {
      if (request.reportable()) commitsError = errorMessage(err);
    } finally {
      if (request.current()) commitsRead = true;
    }
  }
  const commitRows = $derived(
    commitChoices(commits, {
      tips: worktree.head ? [{ hash: worktree.head, tag: "当前" }] : [],
      base: "",
      baseSubject: "",
    }),
  );

  function changeMode(next: string) {
    if (next !== "existing" && next !== "new" && next !== "detach") return;
    if (next === mode) return;
    mode = next;
    setBranchOpen(false);
    setStartOpen(false);
    if (next === "new" && !newName && data) newName = suggestion();
    if (next === "detach" && !commitsRead) void readCommits();
  }

  const target = $derived(
    buildSwitchTarget({
      mode,
      chosen,
      newName,
      nameTaken,
      startPoint,
      commit,
    }),
  );
  const targetKey = $derived(target ? JSON.stringify(target) : "");
  let preview = $state<SwitchPreview | null>(null);
  let previewing = $state(false);
  let previewError = $state("");
  let acknowledged = $state(false);
  let conflict = $state("");
  let error = $state("");
  const previews = new LatestRequest();
  onDestroy(() => {
    previews.dispose();
    branchRequests.dispose();
    commitRequests.dispose();
  });
  void readBranches();
  async function readPreview(next: SwitchTarget) {
    const request = previews.begin();
    previewing = true;
    previewError = "";
    try {
      const result = await source.preview(worktree, next, request.signal);
      if (!request.current()) return;
      preview = result;
      acknowledged = false;
    } catch (err) {
      if (request.reportable()) {
        preview = null;
        previewError = errorMessage(err);
      }
    } finally {
      if (request.current()) previewing = false;
    }
  }
  function resetPreview() {
    previews.cancel();
    preview = null;
    previewing = false;
    previewError = "";
  }
  $effect(() => {
    if (!targetKey) {
      untrack(resetPreview);
      return;
    }
    const next = untrack(() => target);
    if (!next) return;
    untrack(() => {
      previews.cancel();
      preview = null;
      conflict = "";
      error = "";
      previewing = true;
    });
    const timer = setTimeout(
      () => void readPreview(next),
      next.kind === "new" ? 300 : 0,
    );
    return () => clearTimeout(timer);
  });

  const occupant = $derived(resolveSwitchOccupantPath(preview));
  const canSwitch = $derived(
    canConfirmSwitch(preview, previewing, busy, acknowledged),
  );
  const newWorktreePreset = $derived(
    resolveSwitchNewWorktreePreset(preview, target, startIsBranch),
  );

  const short = (hash: string | null | undefined) => hash?.slice(0, 8) ?? "";

  async function submit() {
    if (!canSwitch || !preview || !target) return;
    const confirmed = preview;
    const again = target;
    previews.cancel();
    busy = true;
    error = "";
    conflict = "";
    try {
      const tree = await source.switch(worktree, {
        confirmationId: confirmed.confirmationId,
        acknowledgeSessions: acknowledged,
      });
      onchange();
      ondone(tree);
      return;
    } catch (err) {
      if (err instanceof APIError && err.status === 409)
        conflict = errorMessage(err);
      else error = errorMessage(err);
      onchange();
    } finally {
      busy = false;
    }
    preview = null;
    acknowledged = false;
    void readPreview(again);
  }
</script>

{#snippet pickerFace(value: string, placeholder: string)}
  {#if value}
    <IconGitBranch
      class="size-4 shrink-0 text-muted-foreground"
      aria-hidden="true"
    />
    <span class="min-w-0 flex-1 truncate" title={value}>{value}</span>
  {:else}
    <span class="text-sm text-muted-foreground">
      {data || loadError ? placeholder : "正在读取分支…"}
    </span>
  {/if}
{/snippet}

<PreviewForm {busy} {error} onsubmit={() => void submit()} onclose={oncancel}>
  {#snippet summary()}
    <p
      class="rounded-lg border bg-muted/40 p-3 font-mono text-xs break-all text-muted-foreground"
    >
      {worktree.path}
    </p>
  {/snippet}
  <SegmentedControl.Root
    size="form"
    wrap
    value={mode}
    onValueChange={changeMode}
    disabled={busy}
    aria-label="切换目标"
  >
    <SegmentedControl.Item value="existing">已有分支</SegmentedControl.Item>
    <SegmentedControl.Item value="new">新建分支</SegmentedControl.Item>
    <SegmentedControl.Item value="detach">分离到提交</SegmentedControl.Item>
  </SegmentedControl.Root>
  {#if loadError}<p role="alert" class="text-sm break-all text-destructive">
      无法读取分支：{loadError}
    </p>{/if}

  {#if mode === "existing"}
    <div class="space-y-2">
      <Label for="switch-branch">目标分支</Label>
      <PickerField
        id="switch-branch"
        bind:open={() => branchOpen, setBranchOpen}
        disabled={busy || !data}
      >
        {#snippet trigger()}{@render pickerFace(
            chosen?.name ?? "",
            "选择本地或远程分支",
          )}{/snippet}
        <BranchCommandList
          label="目标分支"
          branches={localItems}
          remoteBranches={remoteItems}
          selected={chosen?.name ?? ""}
          loading={!data && !loadError}
          failed={!!loadError}
          disabled={busy}
          bind:search={branchSearch}
          onselect={selectBranch}
        />
      </PickerField>
      <p class="text-xs text-muted-foreground">
        远程分支会新建同名本地分支并设为其上游。
      </p>
    </div>
  {:else if mode === "new"}
    <div class="space-y-3">
      <div class="space-y-2">
        <div class="flex items-center justify-between gap-2">
          <Label for="switch-new-branch">新分支名称</Label>
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={busy || !data}
            onclick={() => (newName = suggestion(newName || undefined))}
            >换一个</Button
          >
        </div>
        <Input
          id="switch-new-branch"
          bind:value={newName}
          disabled={busy}
          placeholder="feature/my-task"
          autocomplete="off"
          autocapitalize="off"
          autocorrect="off"
          spellcheck={false}
        />
        {#if nameTaken}<p role="alert" class="text-xs text-destructive">
            已有同名本地分支；请换一个名称，或在“已有分支”中选择它。
          </p>{/if}
      </div>
      <div class="space-y-2">
        <Label for="switch-start-point">起点</Label>
        <PickerField
          id="switch-start-point"
          bind:open={() => startOpen, setStartOpen}
          disabled={busy || !data}
        >
          {#snippet trigger()}{@render pickerFace(
              startIsBranch || !startPoint
                ? startPoint
                : `当前提交 ${short(startPoint)}`,
              "选择起点",
            )}{/snippet}
          <BranchCommandList
            label="起点"
            branches={localBranchList}
            remoteBranches={remoteBranchList}
            selected={startPoint}
            loading={!data && !loadError}
            failed={!!loadError}
            disabled={busy}
            bind:search={startSearch}
            onselect={(name) => ((startPoint = name), setStartOpen(false))}
          />
        </PickerField>
        <p class="text-xs text-muted-foreground">新分支不设置上游。</p>
      </div>
    </div>
  {:else}
    <SwitchBranchCommitSelect
      bind:commit
      {commitRows}
      {commitsRead}
      {commitsError}
      {busy}
    />
  {/if}

  <SwitchBranchPreview
    {preview}
    {previewing}
    {previewError}
    {conflict}
    {worktree}
    bind:acknowledged
    {busy}
  />

  {#snippet actions()}
    {#if occupant}
      <Button
        type="button"
        variant="outline"
        wrap
        disabled={busy}
        title={occupant}
        onclick={() => ongoto(occupant)}>转到该工作树</Button
      >
    {/if}
    <Button type="submit" disabled={!canSwitch}
      >{busy ? "正在切换…" : "切换"}</Button
    >
    {#if newWorktreePreset}
      <Button
        type="button"
        variant="success"
        wrap
        disabled={busy}
        title="不改动当前工作树与其中的会话，在新工作树中打开所选分支"
        onclick={() => onnewworktree(newWorktreePreset)}
        >改为在新工作树打开</Button
      >
    {/if}
  {/snippet}
</PreviewForm>
