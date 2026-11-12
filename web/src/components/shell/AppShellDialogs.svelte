<script lang="ts">
  import type { BranchOperation } from "../../domains/workspace/branch-actions";
  import { homeWorktrees } from "../../domains/workspace/home-worktrees.svelte";
  import {
    homeLayout,
    setPinned,
    worktreeTitle,
  } from "../../domains/workspace/home-worktrees";
  import type {
    LaunchEntry,
    LaunchScope,
  } from "../../domains/sessions/new-session-defaults";
  import type {
    ArchivedSession,
    Repository,
    SessionInfo,
    Worktree,
  } from "../../contracts/schemas";
  import type { RepositoryTab, Shell } from "../../hooks/shared/shell.svelte";
  import CloseSessionDialog from "../sessions/CloseSessionDialog.svelte";
  import LazyDialog from "../shared/LazyDialog.svelte";
  import RenameSessionDialog from "../sessions/RenameSessionDialog.svelte";
  import UnregisterRepositoryDialog from "../workspace/UnregisterRepositoryDialog.svelte";

  let {
    shell,
    creating = $bindable(null),
    renaming = $bindable(null),
    closing = $bindable(null),
    removingWorktree = $bindable(null),
    choosingHome = $bindable(null),
    viewingRepository = $bindable(null),
    viewingWorktree = $bindable(null),
    switchingBranch = $bindable(null),
    releasingWorktree = $bindable(null),
    operating = $bindable(null),
    forwarding = $bindable(false),
    adding = $bindable(false),
    configuring = $bindable(null),
    onrefreshworktrees,
  }: {
    shell: Shell;
    creating: { entry?: LaunchEntry; scope?: LaunchScope } | null;
    renaming: SessionInfo | null;
    closing: SessionInfo | null;
    removingWorktree: {
      worktree: Worktree;
      onremoved?: () => void;
      forgetRecords?: readonly ArchivedSession[];
    } | null;
    choosingHome: Repository | null;
    viewingRepository: { repository: Repository; tab: RepositoryTab } | null;
    viewingWorktree: Worktree | null;
    switchingBranch: Worktree | null;
    releasingWorktree: Worktree | null;
    operating: { repositoryId: string; operation: BranchOperation } | null;
    forwarding: boolean;
    adding: boolean;
    configuring: Repository | null;
    onrefreshworktrees: () => void;
  } = $props();

  // Kept after closing so the confirmation still names the repository.
  let unregistering = $state<Repository | null>(null);
  let unregisterOpen = $state(false);

  // Unpins a worktree from the home page; absent when its repository is gone.
  function hideFromHome(tree: Worktree) {
    const repo = shell.repositories.list.find(
      (r) => r.id === tree.repositoryId,
    );
    // Already off the home page: nothing to hide.
    const pinned = repo
      ? homeLayout(homeWorktrees.value, repo, [], new Map()).shown.some(
          (w) => w.tree.id === tree.id && w.pinned,
        )
      : false;
    return repo && pinned
      ? () =>
          homeWorktrees.save(
            setPinned(homeWorktrees.value, repo, [tree.id], false),
          )
      : undefined;
  }

  function resolveWorktreeDialogTarget(target: Worktree) {
    const repo = shell.repositories.list.find(
      (r) => r.id === target.repositoryId,
    );
    const current = repo?.worktrees.find((w) => w.id === target.id) ?? target;
    return { repo, current };
  }
</script>

{#if creating}
  {@const target = creating}
  <LazyDialog
    load={() => import("../sessions/NewSessionDialog.svelte")}
    label="创建表单"
    onclose={() => (creating = null)}
  >
    {#snippet children(page)}<page.default
        launchConfigs={shell.launchConfigs.state}
        repositories={[...shell.repositories.list]}
        defaultCwd={shell.setup.value.cwd}
        entry={target.entry}
        scope={target.scope}
        selectedRepositoryId={shell.repositories.choice ?? "all"}
        intent={target.entry?.kind === "new-worktree" ? "worktree" : "session"}
        onchange={onrefreshworktrees}
        onworktreecreated={(tree) => {
          const repo = shell.repositories.list.find(
            (r) => r.id === tree.repositoryId,
          );
          if (repo)
            homeWorktrees.save(
              setPinned(homeWorktrees.value, repo, [tree.id], true),
            );
        }}
        onclose={() => (creating = null)}
      />{/snippet}
  </LazyDialog>
{/if}
{#if configuring}
  {@const target = configuring}
  <LazyDialog
    load={() => import("../workspace/RepositorySettingsDialog.svelte")}
    label="仓库设置"
    onclose={() => (configuring = null)}
  >
    {#snippet children(page)}<page.default
        repository={target}
        onchange={() => void shell.repositories.refresh(true)}
        onclose={() => (configuring = null)}
      />{/snippet}
  </LazyDialog>
{/if}
{#if forwarding}
  <LazyDialog
    load={() => import("../workspace/ForwardDialog.svelte")}
    label="端口转发"
    onclose={() => (forwarding = false)}
  >
    {#snippet children(page)}<page.default
        sessions={[...shell.sessions.sessions]}
        detection={shell.setup.value.portDetection}
        onclose={() => (forwarding = false)}
      />{/snippet}
  </LazyDialog>
{/if}
{#if adding}
  <LazyDialog
    load={() => import("../workspace/AddRepositoryDialog.svelte")}
    label="仓库表单"
    onclose={() => (adding = false)}
  >
    {#snippet children(page)}<page.default
        defaultCwd={shell.setup.value.cwd}
        onchange={() => void shell.repositories.refresh(true)}
        onclose={() => (adding = false)}
      />{/snippet}
  </LazyDialog>
{/if}
{#if renaming}<RenameSessionDialog
    session={renaming}
    onclose={() => (renaming = null)}
  />{/if}
{#if closing}<CloseSessionDialog
    session={closing}
    onremoved={() => {
      if (!closing) return;
      shell.drafts.remove(closing.id);
      shell.archiveSignal.emit(closing.id);
    }}
    onarchived={(session) => {
      void shell.archived.refresh();
      shell.archiveSignal.emit(session.id);
    }}
    onclose={() => (closing = null)}
  />{/if}
{#if removingWorktree}
  {@const target = removingWorktree}
  <LazyDialog
    load={() => import("../workspace/RemoveWorktreeDialog.svelte")}
    label="删除确认"
    onclose={() => (removingWorktree = null)}
  >
    {#snippet children(page)}<page.default
        worktree={target.worktree}
        forgetRecords={target.forgetRecords ?? []}
        onremoved={target.onremoved}
        onhide={hideFromHome(target.worktree)}
        onchange={onrefreshworktrees}
        onclose={() => (removingWorktree = null)}
      />{/snippet}
  </LazyDialog>
{/if}
{#if choosingHome}
  {@const target = choosingHome}
  {@const currentRepo =
    shell.repositories.list.find((r) => r.id === target.id) ?? target}
  <LazyDialog
    load={() => import("../workspace/HomeWorktreeDialog.svelte")}
    label="主页显示"
    onclose={() => (choosingHome = null)}
  >
    {#snippet children(page)}<page.default
        repository={currentRepo}
        sessions={shell.sessions.sessions}
        records={shell.archived.records}
        home={homeWorktrees.value}
        ontoggle={(worktreeId, pinned) =>
          homeWorktrees.save(
            setPinned(homeWorktrees.value, currentRepo, [worktreeId], pinned),
          )}
        onmanage={() => {
          choosingHome = null;
          shell.repositoryDetails(currentRepo, "worktrees");
        }}
        onclose={() => (choosingHome = null)}
      />{/snippet}
  </LazyDialog>
{/if}
{#if viewingRepository}
  {@const target = viewingRepository}
  {@const currentRepo =
    shell.repositories.list.find((r) => r.id === target.repository.id) ??
    target.repository}
  <LazyDialog
    load={() => import("../workspace/RepositoryPanel.svelte")}
    label="仓库详情"
    onclose={() => (viewingRepository = null)}
  >
    {#snippet children(page)}<page.default
        repository={currentRepo}
        initialTab={target.tab}
        sessions={shell.sessions.sessions}
        records={shell.archived.records}
        home={homeWorktrees.value}
        refreshing={shell.repositories.refreshing}
        refreshError={shell.repositories.error}
        registryReady={shell.setup.value.registryReady}
        onunregister={() => {
          unregistering = currentRepo;
          unregisterOpen = true;
          viewingRepository = null;
        }}
        onremoved={(removedIds) => {
          homeWorktrees.save(
            setPinned(homeWorktrees.value, currentRepo, removedIds, false),
          );
        }}
        onchange={onrefreshworktrees}
        onrefresh={() => void shell.repositories.refresh(true)}
        onnewworktree={(preset) => {
          // Read the panel's repository before closing the panel clears it.
          shell.start({
            kind: "new-worktree",
            repositoryId: currentRepo.id,
            preset,
          });
          viewingRepository = null;
        }}
        onrepository={(repo) => shell.repositories.replace(repo)}
        onclose={() => (viewingRepository = null)}
      />{/snippet}
  </LazyDialog>
{/if}
{#if viewingWorktree}
  {@const { repo, current } = resolveWorktreeDialogTarget(viewingWorktree)}
  <LazyDialog
    load={() => import("../workspace/WorktreeDetailsDialog.svelte")}
    label="工作树详情"
    onclose={() => (viewingWorktree = null)}
  >
    {#snippet children(page)}<page.default
        title={worktreeTitle(current)}
        path={current.path}
        worktree={current}
        sessions={shell.sessions.sessions.filter(
          (s) => s.workspace?.worktreeId === current.id,
        )}
        lastFetchedAt={repo?.lastFetchedAt}
        onoperation={(operation) =>
          shell.branchOperation(current.repositoryId, operation)}
        onclose={() => (viewingWorktree = null)}
      />{/snippet}
  </LazyDialog>
{/if}
{#if switchingBranch}
  {@const { repo, current } = resolveWorktreeDialogTarget(switchingBranch)}
  {#if repo}
    <LazyDialog
      load={() => import("../workspace/SwitchBranchDialog.svelte")}
      label="切换分支"
      onclose={() => (switchingBranch = null)}
    >
      {#snippet children(page)}<page.default
          repository={repo}
          worktree={current}
          onchange={() => void shell.repositories.refresh(true)}
          onnewworktree={(preset) =>
            shell.start({
              kind: "new-worktree",
              repositoryId: repo.id,
              preset,
            })}
          ongoto={(path) => {
            const tree = repo.worktrees.find((w) => w.path === path);
            if (tree) shell.worktreeDetails(tree);
          }}
          onclose={() => (switchingBranch = null)}
        />{/snippet}
    </LazyDialog>
  {/if}
{/if}
<UnregisterRepositoryDialog
  repository={unregistering}
  bind:open={unregisterOpen}
  onchange={() => shell.repositories.refresh(true)}
/>
{#if releasingWorktree}
  {@const target = releasingWorktree}
  <LazyDialog
    load={() => import("../workspace/ReleaseWorktreeDialog.svelte")}
    label="释放空间预览"
    onclose={() => (releasingWorktree = null)}
  >
    {#snippet children(page)}<page.default
        worktree={target}
        onchange={onrefreshworktrees}
        onclose={() => (releasingWorktree = null)}
      />{/snippet}
  </LazyDialog>
{/if}
{#if operating}
  {@const target = operating}
  <LazyDialog
    load={() => import("../workspace/BranchOperationDialog.svelte")}
    label="分支操作"
    onclose={() => (operating = null)}
  >
    {#snippet children(page)}<page.default
        repositoryId={target.repositoryId}
        operation={target.operation}
        onchange={onrefreshworktrees}
        onclose={() => (operating = null)}
      />{/snippet}
  </LazyDialog>
{/if}
