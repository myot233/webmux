<script lang="ts">
  import { SvelteSet } from "svelte/reactivity";
  import { api } from "../../platform/api";
  import type { BranchOperation } from "../../domains/workspace/branch-actions";
  import { errorMessage } from "../../platform/error";
  import {
    BatchRemovalPreviewRequestSchema,
    BatchRemovalPreviewSchema,
    BatchRemoveResultSchema,
    BatchRemoveWorktreesRequestSchema,
    MAX_BATCH_WORKTREES,
    type ArchivedSession,
    type BatchRemovalPreview,
    type BatchRemoveResult,
    type HomeWorktrees,
    type Repository,
    type SessionInfo,
  } from "../../contracts/schemas";
  import {
    buildWorktreeBatchEntries,
    countWorktreeBatchCategories,
    filterWorktreeBatchEntries,
    groupBatchPreviewItems,
    staleRegistrations,
    type WorktreeBatchFilter,
  } from "../../domains/workspace/worktree-batch";
  import { Button } from "$lib/components/ui/button";
  import * as SegmentedControl from "$lib/components/ui/segmented-control";
  import InlineNotice from "../shared/InlineNotice.svelte";
  import ListSearchInput from "../shared/ListSearchInput.svelte";
  import ManageWorktreesPreview from "./ManageWorktreesPreview.svelte";
  import ManageWorktreesSelectList from "./ManageWorktreesSelectList.svelte";

  // The repository panel's worktrees: every worktree of the repository,
  // searchable and filtered by how safely it can be removed. Browsing opens
  // a worktree's details; "批量清理" turns the list into a selection, then
  // a server preview, then per-item results. "清理失效登记" preselects the
  // stale registrations and goes straight to that preview. Removal happens
  // only here in the panel, so the rows' menus carry no delete entry.
  let {
    repository,
    sessions,
    archived,
    home,
    busy = $bindable(false),
    onopen,
    onswitch,
    onoperation,
    onchange,
    onremoved,
  }: {
    repository: Repository;
    sessions: readonly SessionInfo[];
    archived: readonly ArchivedSession[];
    home: HomeWorktrees;
    /** A preview or removal request is in flight. */
    busy?: boolean;
    /** Opens a worktree's details inside the panel. */
    onopen: (worktreeId: string) => void;
    /** Opens the switch of a worktree's branch inside the panel. */
    onswitch: (worktreeId: string) => void;
    /** Opens a branch operation inside the panel. */
    onoperation: (operation: BranchOperation) => void;
    onchange: () => void;
    onremoved?: (removedIds: string[]) => void;
  } = $props();

  type Stage = "browse" | "select" | "preview" | "result";
  let stage = $state<Stage>("browse");
  let query = $state("");
  let statusFilter = $state<WorktreeBatchFilter>("all");
  const selected = new SvelteSet<string>();
  let error = $state("");
  let done = $state("");
  let preview = $state<BatchRemovalPreview | null>(null);
  let includeForce = $state(false);
  let lastResult = $state<BatchRemoveResult | null>(null);

  const entries = $derived(
    buildWorktreeBatchEntries(repository, home, sessions, archived),
  );
  const counts = $derived(countWorktreeBatchCategories(entries));
  const stale = $derived(
    staleRegistrations(entries)
      .slice(0, MAX_BATCH_WORKTREES)
      .map((entry) => entry.tree.id),
  );
  const filteredEntries = $derived(
    filterWorktreeBatchEntries(entries, statusFilter, query),
  );
  const filters = $derived([
    { id: "all", label: "全部", count: counts.all },
    { id: "safe", label: "可安全清理", count: counts.safe },
    { id: "blocked", label: "有阻塞", count: counts.blocked },
    { id: "protected", label: "受保护", count: counts.protected },
  ] as const);

  // Selected IDs kept in the stable repository order; protected or vanished
  // entries are ignored if a background refresh changed the snapshot.
  const selectedIds = $derived(
    entries
      .filter(
        (entry) =>
          entry.category !== "protected" && selected.has(entry.tree.id),
      )
      .map((entry) => entry.tree.id),
  );

  const previewGroups = $derived(
    preview
      ? groupBatchPreviewItems(preview.items)
      : { safe: [], blocked: [], unsupported: [] },
  );
  const actionCount = $derived(
    previewGroups.safe.length +
      (includeForce ? previewGroups.blocked.length : 0),
  );

  const resultSummary = $derived.by(() => {
    const results = lastResult?.results ?? [];
    return {
      removed: results.filter((r) => r.status === "removed").length,
      skipped: results.filter((r) => r.status === "skipped").length,
      failed: results.filter((r) => r.status === "failed"),
    };
  });

  const heading = $derived(
    stage === "preview"
      ? includeForce
        ? "批量强制删除"
        : "确认批量清理"
      : stage === "result"
        ? "批量清理部分完成"
        : "",
  );
  const description = $derived(
    stage === "preview"
      ? includeForce
        ? "危险操作：将关闭受阻工作树中的活动会话，并永久丢弃未提交修改、未跟踪及忽略文件；已归档聊天记录保留。"
        : "默认仅删除无阻塞的工作树并跳过受阻项；本地分支与已归档混合会话记录均会保留。"
      : "部分工作树已完成清理；失败项已保留选中，可重新读取预览后重试。",
  );

  function toggleWorktree(id: string, checked: boolean) {
    error = "";
    if (checked) {
      if (selectedIds.length >= MAX_BATCH_WORKTREES && !selected.has(id)) {
        error = `单次批量管理最多选择 ${MAX_BATCH_WORKTREES} 个工作树。`;
        return;
      }
      selected.add(id);
    } else {
      selected.delete(id);
    }
  }

  function selectAllSafe() {
    error = "";
    selected.clear();
    let count = 0;
    for (const entry of entries) {
      if (entry.category !== "safe") continue;
      if (count >= MAX_BATCH_WORKTREES) {
        error = `已选中前 ${MAX_BATCH_WORKTREES} 个可安全清理工作树（已达单次上限）。`;
        break;
      }
      selected.add(entry.tree.id);
      count++;
    }
  }

  function startSelecting() {
    error = "";
    done = "";
    stage = "select";
  }

  function stopSelecting() {
    error = "";
    selected.clear();
    stage = "browse";
  }

  // Selects exactly the stale registrations and previews them; a failed
  // preview leaves the selection to review.
  async function pruneStale() {
    done = "";
    selected.clear();
    for (const id of stale) selected.add(id);
    await loadPreview(stale);
    if (stage !== "preview") stage = "select";
  }

  async function loadPreview(ids: readonly string[] = selectedIds) {
    if (ids.length === 0) return;
    busy = true;
    error = "";
    includeForce = false;
    try {
      preview = await api(
        `/repositories/${repository.id}/worktrees/batch-removal-preview`,
        BatchRemovalPreviewSchema,
        "POST",
        { worktreeIds: [...ids] },
        undefined,
        BatchRemovalPreviewRequestSchema,
      );
      lastResult = null;
      stage = "preview";
    } catch (err) {
      error = errorMessage(err);
      onchange();
    } finally {
      busy = false;
    }
  }

  async function executeRemove() {
    if (!preview) return;
    busy = true;
    error = "";
    try {
      const res = await api(
        `/repositories/${repository.id}/worktrees/batch-remove`,
        BatchRemoveResultSchema,
        "POST",
        {
          confirmationId: preview.confirmationId,
          includeForce,
          closeSessions: includeForce,
        },
        undefined,
        BatchRemoveWorktreesRequestSchema,
      );
      const removed = res.results
        .filter((item) => item.status === "removed")
        .map((item) => item.worktreeId);
      const failed = res.results.filter((item) => item.status === "failed");
      if (removed.length > 0) onremoved?.(removed);
      onchange();
      preview = null;
      includeForce = false;
      selected.clear();
      if (failed.length === 0) {
        // Everything asked for is done: back to the refreshed list.
        done = `已删除 ${removed.length} 个工作树。`;
        stage = "browse";
        return;
      }
      for (const item of failed) selected.add(item.worktreeId);
      lastResult = res;
      stage = "result";
    } catch (err) {
      error = `${errorMessage(err)}。请重新读取预览后确认。`;
      preview = null;
      includeForce = false;
      stage = "select";
      onchange();
    } finally {
      busy = false;
    }
  }
</script>

<div class="space-y-4">
  {#if stage === "browse" || stage === "select"}
    <div class="space-y-3">
      <ListSearchInput
        bind:value={query}
        label="搜索工作树"
        placeholder="按分支、提交哈希或路径搜索…"
      />

      <SegmentedControl.Root
        value={statusFilter}
        onValueChange={(value) => {
          const next = filters.find((item) => item.id === value);
          if (next) statusFilter = next.id;
        }}
        size="form"
        aria-label="按工作树状态筛选"
        wrap
      >
        {#each filters as filter (filter.id)}
          <SegmentedControl.Item value={filter.id} class="min-h-9">
            {filter.label}
            <span
              class="inline-block w-[3ch] text-right font-mono text-xs tabular-nums opacity-60"
              >{filter.count}</span
            >
          </SegmentedControl.Item>
        {/each}
      </SegmentedControl.Root>

      <div class="flex flex-wrap items-center justify-between gap-2 text-xs">
        <span class="text-muted-foreground tabular-nums">
          {stage === "select"
            ? `已选 ${selectedIds.length} 项（共 ${entries.length} 个工作树）`
            : `共 ${entries.length} 个工作树`}
        </span>
        <div class="flex flex-wrap items-center gap-1.5">
          {#if stage === "select"}
            <Button
              type="button"
              variant="outline"
              size="sm"
              class="min-h-9 sm:min-h-8"
              disabled={busy || counts.safe === 0}
              onclick={selectAllSafe}
            >
              选中全部可安全清理（{counts.safe}）
            </Button>
            <Button
              type="button"
              variant="ghost"
              size="sm"
              class="min-h-9 sm:min-h-8"
              disabled={busy || selectedIds.length === 0}
              onclick={() => {
                error = "";
                selected.clear();
              }}
            >
              清空选择
            </Button>
            <Button
              type="button"
              variant="ghost"
              size="sm"
              class="min-h-9 sm:min-h-8"
              disabled={busy}
              onclick={stopSelecting}
            >
              退出批量清理
            </Button>
          {:else}
            {#if stale.length > 0}
              <Button
                type="button"
                variant="outline"
                size="sm"
                class="min-h-9 sm:min-h-8"
                disabled={busy}
                onclick={() => void pruneStale()}
              >
                {busy ? "正在读取预览…" : `清理失效登记（${stale.length}）`}
              </Button>
            {/if}
            <Button
              type="button"
              variant="outline"
              size="sm"
              class="min-h-9 sm:min-h-8"
              disabled={busy || entries.length === 0}
              onclick={startSelecting}
            >
              批量清理
            </Button>
          {/if}
        </div>
      </div>
    </div>

    {#if done}<InlineNotice tone="success">{done}</InlineNotice>{/if}

    <ManageWorktreesSelectList
      {stage}
      {entries}
      {filteredEntries}
      {selectedIds}
      {busy}
      ontoggle={toggleWorktree}
      {onopen}
      {onswitch}
      {onoperation}
    />
  {:else}
    <div class="space-y-1">
      <h3 class="text-sm font-medium">{heading}</h3>
      <p class="text-xs text-muted-foreground">{description}</p>
    </div>
    {#if stage === "preview" && preview}
      {#if busy}
        <p role="status" class="text-sm">正在处理，请勿重复提交…</p>
      {/if}
      <ManageWorktreesPreview groups={previewGroups} bind:includeForce {busy} />
    {:else if stage === "result" && lastResult}
      <div class="rounded-lg border bg-muted/40 p-3 text-sm">
        <p class="font-medium">
          已删除 {resultSummary.removed} 个
          {#if resultSummary.skipped > 0}，已跳过 {resultSummary.skipped} 个{/if}
          ，失败 {resultSummary.failed.length} 个
        </p>
        <p class="mt-1 text-xs text-muted-foreground">
          失败项的文件与未授权会话均未删除，已保留勾选以便重新读取最新状态后重试。
        </p>
      </div>

      <InlineNotice tone="danger">
        <ul class="space-y-1.5">
          {#each resultSummary.failed as item (item.worktreeId)}
            <li class="break-all">
              <span class="font-medium">{item.branch || item.path}</span>
              {#if item.branch}
                <span class="font-mono text-xs">（{item.path}）</span>
              {/if}
              ：{item.error || "删除未完成"}
            </li>
          {/each}
        </ul>
      </InlineNotice>
    {/if}
  {/if}

  {#if error}
    <p role="alert" class="text-sm break-all text-destructive">{error}</p>
  {/if}

  {#if stage !== "browse"}
    <div class="flex flex-wrap justify-end gap-2">
      {#if stage === "select"}
        <Button
          type="button"
          disabled={busy ||
            selectedIds.length === 0 ||
            selectedIds.length > MAX_BATCH_WORKTREES}
          onclick={() => void loadPreview()}
        >
          {busy ? "正在读取预览…" : `下一步：预览删除 (${selectedIds.length})`}
        </Button>
      {:else if stage === "preview" && preview}
        <Button
          type="button"
          variant="outline"
          disabled={busy}
          onclick={() => {
            preview = null;
            includeForce = false;
            error = "";
            stage = "select";
          }}
        >
          返回重选
        </Button>
        {#if includeForce && previewGroups.blocked.length > 0}
          <Button
            type="button"
            variant="destructive"
            disabled={busy || actionCount === 0}
            onclick={() => void executeRemove()}
          >
            {busy ? "正在关闭会话并删除…" : `关闭会话并删除 (${actionCount})`}
          </Button>
        {:else}
          <Button
            type="button"
            disabled={busy || previewGroups.safe.length === 0}
            onclick={() => void executeRemove()}
          >
            {busy ? "正在清理…" : `确认安全删除 (${previewGroups.safe.length})`}
          </Button>
        {/if}
      {:else if stage === "result" && lastResult}
        <Button
          type="button"
          variant="outline"
          disabled={busy}
          onclick={() => {
            lastResult = null;
            error = "";
            stage = "select";
          }}
        >
          返回列表
        </Button>
        <Button
          type="button"
          disabled={busy || selected.size === 0}
          onclick={() => void loadPreview([...selected])}
        >
          {busy ? "正在读取预览…" : `重新预览重试 (${selected.size})`}
        </Button>
      {/if}
    </div>
  {/if}
</div>
