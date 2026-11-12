<script lang="ts">
  import SessionListRemovalDialog from "../components/sessions/SessionListRemovalDialog.svelte";
  import { onMount } from "svelte";
  import SessionSwitcherTrigger from "../components/shell/SessionSwitcherTrigger.svelte";
  import WorkspaceActions from "../components/workspace/WorkspaceActions.svelte";
  import WorkspaceToolbar from "../components/workspace/WorkspaceToolbar.svelte";
  import { store, stored } from "../platform/api";
  import type { ArchivedSession, Worktree } from "../contracts/schemas";
  import { errorMessage } from "../platform/error";
  import PendingUpdateNotice from "../components/notifications/PendingUpdateNotice.svelte";
  import {
    dismissUpdate,
    dismissedUpdate,
    homeUpdate,
  } from "../platform/pending-update";
  import { Skeleton } from "$lib/components/ui/skeleton";
  import SessionFilters from "../components/sessions/SessionFilters.svelte";
  import SessionListNotices from "../components/sessions/SessionListNotices.svelte";
  import RepositoryGroup from "../components/workspace/RepositoryGroup.svelte";
  import DirectSessions from "../components/sessions/DirectSessions.svelte";
  import SidebarButton from "../components/shell/SidebarButton.svelte";
  import WorktreeBadges from "../components/workspace/WorktreeBadges.svelte";
  import WorkspaceMenu from "../components/workspace/WorkspaceMenu.svelte";
  import { removeRecord, resumeSession } from "../domains/sessions/recoverable";
  import {
    placeArchived,
    type ArchiveActions,
  } from "../domains/sessions/archived";
  import SessionListEmpty from "../components/sessions/SessionListEmpty.svelte";
  import { homeWorktrees } from "../domains/workspace/home-worktrees.svelte";
  import {
    setPinned,
    worktreeTitle,
  } from "../domains/workspace/home-worktrees";
  import {
    deriveRepositoryExpanded,
    filterDirectSessions,
    filterVisibleArchiveIds,
    filterVisibleSessions,
    hasVisibleArchivedInRepository,
    isRepositoryGroupOpen,
    isSelectedWorktreePinned,
    isSessionListEmpty,
    mountSessionListPolling,
    reconcileSelectedRepository,
    removeRecordAfterWorktree,
    resolveScopeLabel,
    resolveScopedArchive,
    resolveScopedSessions,
    resolveSelectedRepository,
  } from "../hooks/sessions/session-list";
  import { type StatusFilter } from "../domains/sessions/session-status";
  import {
    resetBrowserChrome,
    setBrowserChrome,
  } from "../platform/browser-chrome";
  import { getShell } from "../hooks/shared/shell.svelte";
  import { switchableWorktree } from "../domains/workspace/worktree-sync";
  import { z } from "zod";

  // The workbench home: every live session grouped by repository and
  // worktree, beside the shell's tree. The shell owns the shared data.
  const shell = getShell();
  const { drafts } = shell;
  const sessions = $derived(shell.sessions.sessions);
  const recoverable = $derived(shell.archived.records);
  const repositories = $derived(shell.repositories.list);
  const repositoriesSettled = $derived(shell.repositories.settled);
  const refreshing = $derived(shell.repositories.refreshing);
  const repositoryError = $derived(shell.repositories.error);
  const setup = $derived(shell.setup.value);
  const capabilityError = $derived(setup.capabilityError);
  const loading = $derived(shell.sessions.loading);
  const initializing = $derived(!shell.setup.settled);
  const error = $derived(shell.sessions.error);
  let dismissedVersion = $state(dismissedUpdate());
  const updateNotice = $derived(
    homeUpdate(setup.pendingUpdate, dismissedVersion),
  );
  let removing = $state<Worktree | null>(null);
  let removingRecord = $state<ArchivedSession | null>(null);
  let recordError = $state("");
  let now = $state(Date.now());
  let focusedExpanded = $state<Record<string, boolean>>({});
  let expansionOverrides = $state<Record<string, boolean>>({});
  let query = $state("");
  let statusFilter = $state<StatusFilter>("all");
  let filterExpanded = $state<Record<string, boolean>>({});
  const repositoryChoice = $derived(shell.repositories.choice);
  // Output previews are the default; only an explicit choice keeps rows compact.
  let compact = $state(
    z.enum(["compact", "preview"]).safeParse(stored("homeSessionView")).data ===
      "compact",
  );
  const list = $derived(
    sessions.filter((session) => session.state === "active"),
  );
  const direct = $derived(filterDirectSessions(list, repositories));
  const placement = $derived(placeArchived(recoverable, repositories));
  const selectedRepository = $derived(
    resolveSelectedRepository(repositoryChoice, repositories),
  );
  // A worktree picked in the tree narrows the repository view to it.
  const selectedWorktree = $derived(
    repositories
      .find((repo) => repo.id === selectedRepository)
      ?.worktrees.find((tree) => tree.id === shell.repositories.worktree),
  );
  const scopeLabel = $derived(
    resolveScopeLabel(selectedRepository, selectedWorktree, repositories),
  );
  const scopedSessions = $derived(
    resolveScopedSessions(selectedRepository, selectedWorktree, list, direct),
  );
  const scopedArchive = $derived(
    resolveScopedArchive(
      selectedRepository,
      selectedWorktree,
      recoverable,
      placement,
    ),
  );
  const scopedArchiveCount = $derived(scopedArchive.length);
  $effect(() => {
    reconcileSelectedRepository({
      loading,
      initializing,
      repositoriesSettled,
      repositoryError,
      repositoryChoice,
      selectedRepository,
      selectedWorktreeId: shell.repositories.worktree,
      repositories,
      list,
      select: (id) => shell.repositories.select(id),
    });
  });
  function setCompact(value: boolean) {
    compact = value;
    store("homeSessionView", value ? "compact" : "preview");
  }
  const search = $derived(query.trim().toLocaleLowerCase());
  const filtering = $derived(Boolean(search) || statusFilter !== "all");
  const visible = $derived(
    filterVisibleSessions(scopedSessions, repositories, statusFilter, search),
  );
  const visibleIds = $derived(
    filtering || selectedRepository !== "all"
      ? new Set(visible.map((session) => session.id))
      : undefined,
  );
  const visibleArchiveIds = $derived(
    filterVisibleArchiveIds(
      recoverable,
      repositories,
      filtering,
      statusFilter,
      search,
    ),
  );
  const archivedMatches = $derived(
    visibleArchiveIds === undefined
      ? 0
      : scopedArchive.filter((record) => visibleArchiveIds.has(record.id))
          .length,
  );
  function archivedInRepository(id: string) {
    return hasVisibleArchivedInRepository(placement, id, visibleArchiveIds);
  }
  let rebuildFailure = $state("");
  const archive: ArchiveActions = $derived({
    now,
    filtering,
    resume: resumeSession,
    remove: async (id) => {
      recordError = "";
      await removeRecord(id);
      drafts.remove(id);
      await shell.archived.refresh();
    },
    removeRecordAndWorktree: (record, tree) => {
      recordError = "";
      removingRecord = record;
      removing = tree;
    },
    release: shell.releaseWorktree,
    onresumed: () => ((rebuildFailure = ""), void shell.archived.refresh()),
    onrebuilt: (failure) => ((rebuildFailure = failure), refreshWorktrees()),
  });
  const running = $derived(
    sessions.some((session) => session.status === "running"),
  );
  $effect(() => setBrowserChrome("WebMux · 工作区", running));
  $effect(() => {
    void query;
    void statusFilter;
    void selectedRepository;
    void selectedWorktree;
    filterExpanded = {};
  });
  function clearFilters() {
    query = "";
    statusFilter = "all";
  }
  const expanded = $derived(
    deriveRepositoryExpanded(
      repositories,
      expansionOverrides,
      sessions,
      placement,
    ),
  );
  function pin(repositoryId: string, ids: string[], pinned: boolean) {
    const repo = repositories.find((r) => r.id === repositoryId);
    if (repo)
      homeWorktrees.save(setPinned(homeWorktrees.value, repo, ids, pinned));
  }
  let worktreeDetailsOpen = $state(false);
  $effect(() => {
    void selectedWorktree?.id;
    worktreeDetailsOpen = false;
  });
  const selectedPinned = $derived(
    isSelectedWorktreePinned(
      homeWorktrees.value,
      repositories,
      selectedRepository,
      selectedWorktree,
    ),
  );
  async function removeAfterWorktree(record: ArchivedSession | null) {
    await removeRecordAfterWorktree(
      record,
      removeRecord,
      (id) => drafts.remove(id),
      () => shell.archived.refresh(),
      (msg) => (recordError = msg),
      () => (removingRecord = null),
      errorMessage,
    );
  }
  function refreshWorktrees() {
    void shell.repositories.refresh();
    void shell.archived.refresh().then((records) => {
      if (records)
        drafts.reconcile([
          ...sessions.map((session) => session.id),
          ...records,
        ]);
    });
  }
  const closeRemoveWorktree = () => {
    removing = null;
    removingRecord = null;
  };
  onMount(() =>
    mountSessionListPolling(
      () => void shell.repositories.refresh(),
      (nextNow) => (now = nextNow),
      resetBrowserChrome,
    ),
  );
</script>

{#snippet menu()}
  {@const repo = shell.settingsRepository}
  {#if repo}
    {@const tree = selectedWorktree}
    <WorkspaceMenu
      repository={repo.name}
      unavailable={repo.availability !== "available"}
      onnewworktree={() =>
        shell.start({ kind: "new-worktree", repositoryId: repo.id })}
      onhome={() => shell.chooseHomeWorktrees(repo)}
      onmanage={() => shell.repositoryDetails(repo, "worktrees")}
      onsettings={shell.openRepositorySettings}
      ondetails={() => shell.repositoryDetails(repo)}
      worktree={tree
        ? {
            title: worktreeTitle(tree),
            ondetails: () => (worktreeDetailsOpen = true),
            onswitch: switchableWorktree(tree)
              ? () => shell.switchBranch(tree)
              : undefined,
            pinned: selectedPinned,
            ontogglepin: () => pin(repo.id, [tree.id], !selectedPinned),
            onremove:
              tree.kind === "linked" ? () => (removing = tree) : undefined,
            tree,
            onoperation: (operation) =>
              shell.branchOperation(repo.id, operation),
          }
        : undefined}
    />
  {/if}
{/snippet}

{#snippet actions()}
  <SessionSwitcherTrigger size="icon-sm" />
  <WorkspaceActions
    {compact}
    {refreshing}
    onchange={setCompact}
    onrefresh={() => void shell.repositories.refresh(true)}
    menu={shell.settingsRepository ? menu : undefined}
  />
{/snippet}

<div class="workspace-page safe-area-page min-h-dvh">
  <!-- Phones reach the sidebar from this bar; wider screens show it. -->
  <header
    class="app-header flex min-h-(--app-header-height) flex-wrap items-center gap-1 border-b bg-card px-1 sm:hidden"
  >
    <SidebarButton />
    <h1 class="min-w-0 truncate text-sm font-medium">
      {selectedRepository === "all" ? "全部仓库" : scopeLabel}
    </h1>
    <!-- The sidebar footer is out of sight here, so phones keep this notice. -->
    {#if shell.sessions.connection === "reconnecting"}<span
        role="status"
        class="shrink-0 text-xs text-warning">连接已断开，重连中…</span
      >{/if}
    <div
      class="ml-auto flex max-w-full flex-wrap items-center justify-end gap-1"
    >
      {@render actions()}
    </div>
  </header>
  {#if updateNotice}
    {@const version = updateNotice.installedVersion}
    <div class="mx-auto max-w-7xl px-4 pt-5 sm:px-8 sm:pt-6">
      <PendingUpdateNotice
        update={updateNotice}
        ondismiss={() => {
          dismissUpdate(version);
          dismissedVersion = version;
        }}
      />
    </div>
  {/if}
  {#if loading || initializing}
    <div
      class="mx-auto max-w-7xl space-y-4 px-4 py-5 sm:px-8 sm:py-6"
      aria-label="正在加载会话"
    >
      {#each [0, 1, 2] as item (item)}<Skeleton class="h-40 w-full" />{/each}
    </div>
  {:else}
    <div class="mx-auto max-w-6xl px-4 py-5 sm:px-8 sm:py-6">
      <main class="min-w-0">
        <WorkspaceToolbar
          title={scopeLabel}
          {loading}
          {filtering}
          visibleCount={visible.length}
          sessionCount={scopedSessions.length}
          repositoryCount={selectedRepository === "all"
            ? repositories.length
            : selectedRepository === "direct"
              ? 0
              : 1}
          {refreshing}
          {actions}
          onstart={() =>
            selectedWorktree
              ? shell.start({
                  kind: "worktree",
                  repositoryId: selectedRepository,
                  worktreeId: selectedWorktree.id,
                })
              : shell.start()}
          startDisabled={Boolean(
            selectedWorktree &&
            (!selectedWorktree.available || selectedWorktree.kind === "bare"),
          )}
        >
          {#snippet meta()}
            {#if selectedWorktree}<WorktreeBadges
                worktree={selectedWorktree}
                pinned={selectedPinned}
              />{/if}
          {/snippet}
          <SessionFilters
            sessions={scopedSessions}
            archivedCount={scopedArchiveCount}
            bind:query
            bind:statusFilter
            filtered={filtering && (visible.length > 0 || archivedMatches > 0)}
            shortcutDisabled={Boolean(removing)}
            onclear={clearFilters}
          />
        </WorkspaceToolbar>
        <SessionListNotices
          setupError={shell.setup.error}
          setupBusy={shell.setup.busy}
          onretrysetup={() => void shell.setup.load()}
          {error}
          {rebuildFailure}
          {recordError}
          {capabilityError}
          {repositoryError}
          hasRepositories={repositories.length > 0}
        />
        <div class="space-y-8">
          {#each repositories as repo (repo.id)}
            <RepositoryGroup
              {repo}
              sessions={list.filter(
                (s) => s.workspace?.repositoryId === repo.id,
              )}
              {filtering}
              {compact}
              focused={selectedRepository === repo.id}
              worktree={selectedRepository === repo.id
                ? selectedWorktree?.id
                : undefined}
              hidden={selectedRepository !== "all" &&
                selectedRepository !== repo.id}
              {visibleIds}
              open={isRepositoryGroupOpen({
                repoId: repo.id,
                filtering,
                visible,
                hasArchived: archivedInRepository(repo.id),
                filterExpanded,
                selectedRepository,
                focusedExpanded,
                expanded,
              })}
              onopenchange={(open) => {
                if (filtering) filterExpanded[repo.id] = open;
                else if (selectedRepository === repo.id)
                  focusedExpanded[repo.id] = open;
                else expansionOverrides[repo.id] = open;
              }}
              {now}
              onstart={(worktreeId) =>
                worktreeId === undefined
                  ? shell.start({
                      kind: "new-worktree",
                      repositoryId: repo.id,
                    })
                  : shell.start({
                      kind: "worktree",
                      repositoryId: repo.id,
                      worktreeId,
                    })}
              ontogglepin={(worktreeId, pinned) =>
                pin(repo.id, [worktreeId], pinned)}
              onremove={(tree) => (removing = tree)}
              onswitch={(tree) => shell.switchBranch(tree)}
              onclose={shell.close}
              archived={placement.repositories.get(repo.id)}
              {visibleArchiveIds}
              {archive}
              detailsOpen={worktreeDetailsOpen}
              ondetailschange={(open) => (worktreeDetailsOpen = open)}
              onoperation={(operation) =>
                shell.branchOperation(repo.id, operation)}
            />
          {/each}
          {#if direct.length || placement.direct.length}<DirectSessions
              sessions={direct}
              {compact}
              focused={selectedRepository === "direct"}
              {visibleIds}
              {now}
              onclose={shell.close}
              archived={placement.direct}
              visibleArchiveIds={selectedRepository === "all" ||
              selectedRepository === "direct"
                ? visibleArchiveIds
                : new Set()}
              {archive}
            />{/if}
          {#if isSessionListEmpty( { filtering, visibleCount: visible.length, archivedMatches, repositoriesCount: repositories.length, listCount: list.length, recoverableCount: recoverable.length, selectedRepository, directCount: direct.length, directArchiveCount: placement.direct.length } )}<SessionListEmpty
              filtered={filtering && !visible.length && !archivedMatches}
              canStart
              onclear={clearFilters}
              onstart={() => shell.start()}
            />{/if}
        </div>
      </main>
    </div>
  {/if}
</div>
{#if removing}
  {@const target = removing}
  {@const targetRecord = removingRecord}
  <SessionListRemovalDialog
    worktree={target}
    record={targetRecord}
    onremoved={() => void removeAfterWorktree(targetRecord)}
    onchange={refreshWorktrees}
    onclose={closeRemoveWorktree}
  />
{/if}
