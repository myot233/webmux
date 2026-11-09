<script lang="ts">
  import { onDestroy, untrack, type Snippet } from "svelte";
  import { SvelteSet } from "svelte/reactivity";
  import IconLayoutSidebarLeftExpand from "~icons/tabler/layout-sidebar-left-expand";
  import IconMessagePlus from "~icons/tabler/message-plus";
  import IconPlus from "~icons/tabler/plus";
  import { Button } from "$lib/components/ui/button";
  import * as Drawer from "$lib/components/ui/drawer";
  import { navigate } from "../api";
  import { createArchiveSignal } from "../archive-signal";
  import { createArchived } from "../hooks/archived.svelte";
  import { createLaunchConfigs } from "../hooks/launch-configs.svelte";
  import { createRepositories } from "../hooks/repositories.svelte";
  import { createServerSetup } from "../hooks/server-setup.svelte";
  import { createSessionList } from "../hooks/session-list.svelte";
  import { homeWorktrees } from "../home-worktrees.svelte";
  import { homeLayout, setPinned } from "../home-worktrees";
  import {
    resolveLaunchScope,
    type LaunchEntry,
    type LaunchScope,
  } from "../new-session-defaults";
  import type { Route } from "../route";
  import type {
    ArchivedSession,
    Repository,
    SessionInfo,
    Worktree,
  } from "../schemas";
  import { unknownWorkspaces } from "../session-list";
  import {
    openPage,
    openSession,
    setShell,
    shellUi,
    sidebarWidthMax,
    sidebarWidthMin,
    type Shell,
    type ShellMode,
  } from "../shell.svelte";
  import { dragWidth, keyWidth } from "../splitter";
  import type { TerminalDrafts } from "../terminal-draft.svelte";
  import AppSidebar from "./AppSidebar.svelte";
  import AttentionToasts from "./AttentionToasts.svelte";
  import LazyDialog from "./LazyDialog.svelte";
  import CloseSessionDialog from "./CloseSessionDialog.svelte";
  import RenameSessionDialog from "./RenameSessionDialog.svelte";
  import UnregisterRepositoryDialog from "./UnregisterRepositoryDialog.svelte";
  import SidebarFooter from "./SidebarFooter.svelte";
  import SidebarRailSessions from "./SidebarRailSessions.svelte";

  // The layout around the home page, quick start and session pages: the
  // sidebar (a rail when collapsed on desktop, a left drawer on phones) and
  // the page. It owns the data those pages share — one session-list
  // connection, the repositories, archived records, server setup and launch
  // configs — and the dialogs any of them can open. Navigating between its
  // pages keeps this instance, so the sidebar's scroll and folds survive.
  let {
    route,
    drafts,
    children,
  }: {
    route: Extract<Route, { name: "sessions" | "quick-start" | "terminal" }>;
    drafts: TerminalDrafts;
    children: Snippet;
  } = $props();

  // The drafts store lives as long as the app; the shell never swaps it.
  const ownDrafts = untrack(() => drafts);
  const archived = createArchived();
  const archiveSignal = createArchiveSignal();
  const repositories = createRepositories();
  const setup = createServerSetup();
  const launchConfigs = createLaunchConfigs();
  const sessions = createSessionList({
    drafts: ownDrafts,
    archived,
    onreconnect: () => void repositories.refresh(),
  });
  export function sessionSwitcherSource() {
    return { sessions, repositories };
  }

  // Optional Git discovery and the setup read never hold the page; each
  // fills in when it arrives.
  void setup.load();
  void launchConfigs.load(true);
  void repositories.refresh();

  let creating = $state<{
    entry?: LaunchEntry;
    scope?: LaunchScope;
  } | null>(null);
  let renaming = $state<SessionInfo | null>(null);
  let closing = $state<SessionInfo | null>(null);
  let removingWorktree = $state<{
    worktree: Worktree;
    onremoved?: () => void;
    forgetRecords?: readonly ArchivedSession[];
  } | null>(null);
  // Unpins a worktree from the home page; absent when its repository is gone.
  function hideFromHome(tree: Worktree) {
    const repo = repositories.list.find((r) => r.id === tree.repositoryId);
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
  let managingWorktrees = $state<Repository | null>(null);
  let choosingHome = $state<Repository | null>(null);
  let viewingRepository = $state<Repository | null>(null);
  // Kept after closing so the confirmation still names the repository.
  let unregistering = $state<Repository | null>(null);
  let unregisterOpen = $state(false);
  let releasingWorktree = $state<Worktree | null>(null);
  let forwarding = $state(false);
  let adding = $state(false);
  let configuring = $state<Repository | null>(null);
  // The home view's repository; elsewhere the toolbar has no settings button.
  const settingsRepository = $derived(
    route.name === "sessions" && setup.value.registryReady
      ? (repositories.list.find((repo) => repo.id === repositories.choice) ??
          ((repositories.choice ?? "all") === "all" &&
          repositories.list.length === 1
            ? repositories.list[0]
            : undefined))
      : undefined,
  );

  const shell: Shell = {
    drafts: ownDrafts,
    sessions,
    repositories,
    archived,
    archiveSignal,
    setup,
    launchConfigs,
    start(entry) {
      shellUi.drawerOpen = false;
      creating = {
        entry,
        scope: resolveLaunchScope({
          entry,
          home: route.name === "sessions",
          choice: repositories.choice,
          worktreeId: repositories.worktree,
          repositories: repositories.list,
        }),
      };
      // Pick up configs changed on other devices since the page loaded.
      void launchConfigs.load();
    },
    rename(session) {
      shellUi.drawerOpen = false;
      renaming = session;
    },
    close(session) {
      shellUi.drawerOpen = false;
      closing = session;
    },
    removeWorktree(worktree, options) {
      shellUi.drawerOpen = false;
      removingWorktree = { worktree, ...options };
    },
    manageWorktrees(repository) {
      shellUi.drawerOpen = false;
      managingWorktrees = repository;
    },
    chooseHomeWorktrees(repository) {
      shellUi.drawerOpen = false;
      choosingHome = repository;
    },
    repositoryDetails(repository) {
      shellUi.drawerOpen = false;
      viewingRepository = repository;
    },
    releaseWorktree(worktree) {
      shellUi.drawerOpen = false;
      releasingWorktree = worktree;
    },
    forward() {
      shellUi.drawerOpen = false;
      forwarding = true;
    },
    addRepository() {
      shellUi.drawerOpen = false;
      adding = true;
    },
    get settingsRepository() {
      return settingsRepository;
    },
    openRepositorySettings() {
      shellUi.drawerOpen = false;
      configuring = settingsRepository ?? null;
    },
  };
  setShell(shell);
  onDestroy(() => {
    sessions.close();
    repositories.dispose();
    launchConfigs.dispose();
    // Leaving the shell (settings) must not reopen the drawer on return.
    shellUi.drawerOpen = false;
  });

  // The new-session dialog belongs to the page it was opened on: a created
  // session navigates away, and the dialog must not follow it there.
  const page = $derived(route.name === "terminal" ? route.id : route.name);
  $effect(() => {
    void page;
    untrack(() => (creating = null));
  });

  // A session started elsewhere names a worktree this page has not read yet.
  // Read the repositories once per unknown worktree: if it is still missing
  // afterwards (for example a released one), "已不可用工作区" is correct and
  // asking again would only repeat the Git discovery.
  const askedFor = new SvelteSet<string>();
  $effect(() => {
    if (!repositories.settled) return;
    const unknown = unknownWorkspaces(
      sessions.sessions,
      repositories.list,
    ).filter((key) => !askedFor.has(key));
    if (!unknown.length) return;
    for (const key of unknown) askedFor.add(key);
    untrack(() => void repositories.refresh(true));
  });

  // The drawer takes focus when it opens and gives it back to the button
  // that opened it when it closes in place.
  $effect(() => {
    if (!shellUi.drawerOpen) return;
    const opener =
      document.activeElement instanceof HTMLElement
        ? document.activeElement
        : null;
    const timer = setTimeout(() =>
      document
        .querySelector<HTMLElement>('[data-slot="drawer-content"] button')
        ?.focus(),
    );
    return () => {
      clearTimeout(timer);
      if (!shellUi.drawerOpen && opener?.isConnected) opener.focus();
    };
  });

  // A home page decides the mode; a session page keeps the remembered one.
  $effect(() => {
    if (route.name === "sessions" && shellUi.mode !== "workbench")
      shellUi.setMode("workbench");
    else if (route.name === "quick-start" && shellUi.mode !== "chat")
      shellUi.setMode("chat");
  });

  // Switching by hand only swaps the sidebar on a session page; on a home
  // page it also shows the other home. The drawer stays open either way.
  function changeMode(next: ShellMode) {
    if (next === shellUi.mode) return;
    shellUi.setMode(next);
    if (route.name !== "terminal") navigate(shellUi.home);
  }
  function primary() {
    if (shellUi.mode === "chat") openPage("/new");
    else shell.start();
  }
  function refreshWorktrees() {
    void repositories.refresh();
    void archived.refresh().then((records) => {
      if (records)
        ownDrafts.reconcile([
          ...sessions.sessions.map((session) => session.id),
          ...records,
        ]);
    });
  }
  // Dragging or arrow keys on the sidebar's edge set its width.
  const setWidth = (width: number) => shellUi.setWidth(width);
  function resizeStart(event: PointerEvent) {
    dragWidth(event, shellUi.width, setWidth);
  }
  function resizeKey(event: KeyboardEvent) {
    keyWidth(
      event,
      shellUi.width,
      { min: sidebarWidthMin, max: sidebarWidthMax },
      setWidth,
    );
  }
</script>

<div
  class="app-shell"
  data-collapsed={shellUi.collapsed}
  data-mode={shellUi.mode}
  style:--app-sidebar-full-width={`${shellUi.width}px`}
>
  <nav class="app-sidebar app-sidebar-rail" aria-label="窄栏导航">
    <Button
      variant="ghost"
      size="icon-touch"
      aria-label="展开侧栏"
      title="展开侧栏"
      onclick={() => {
        // Tablets keep the rail and show the full sidebar as a drawer.
        if (matchMedia("(max-width: 1023px)").matches)
          shellUi.drawerOpen = true;
        else shellUi.setCollapsed(false);
      }}><IconLayoutSidebarLeftExpand aria-hidden="true" /></Button
    >
    <Button
      variant="ghost"
      size="icon-touch"
      aria-label={shellUi.mode === "chat" ? "新聊天" : "新建会话"}
      title={shellUi.mode === "chat" ? "新聊天" : "新建会话"}
      onclick={primary}
      >{#if shellUi.mode === "chat"}<IconMessagePlus
          aria-hidden="true"
        />{:else}<IconPlus aria-hidden="true" />{/if}</Button
    >
    <SidebarRailSessions
      sessions={sessions.sessions.filter(
        (item) => item.state === "active" && item.status !== "exited",
      )}
      current={route.name === "terminal" ? route.id : ""}
      onopen={openSession}
    />
    <div class="mt-auto w-full">
      <SidebarFooter
        rail
        connection={sessions.connection}
        hostname={setup.value.hostname}
        setupSettled={setup.settled}
        onforward={shell.forward}
        onadd={shell.addRepository}
        canAdd={setup.value.gitReady && setup.value.registryReady}
      />
    </div>
  </nav>
  <aside class="app-sidebar app-sidebar-full" aria-label="侧栏">
    <AppSidebar {route} collapsible onmode={changeMode} onprimary={primary} />
    <!-- A focusable separator with aria-valuenow is an interactive widget (WAI-ARIA window splitter). -->
    <!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions -->
    <div
      class="app-sidebar-divider"
      role="separator"
      aria-orientation="vertical"
      aria-label="调整侧栏宽度"
      title="拖动调整侧栏宽度"
      aria-valuemin={sidebarWidthMin}
      aria-valuemax={sidebarWidthMax}
      aria-valuenow={shellUi.width}
      tabindex="0"
      onpointerdown={resizeStart}
      onkeydown={resizeKey}
    ></div>
  </aside>
  <div class="app-shell-main">
    {@render children()}
  </div>
</div>
<AttentionToasts />

<!-- noBodyStyles: the page under it keeps its scroll position and layout. -->
<Drawer.Root direction="left" noBodyStyles bind:open={shellUi.drawerOpen}>
  <Drawer.Content class="w-80 max-w-[85vw] lg:hidden">
    <Drawer.Title class="sr-only">导航</Drawer.Title>
    <AppSidebar {route} onmode={changeMode} onprimary={primary} />
  </Drawer.Content>
</Drawer.Root>

{#if creating}
  {@const target = creating}
  <LazyDialog
    load={() => import("./NewSessionDialog.svelte")}
    label="创建表单"
    onclose={() => (creating = null)}
  >
    {#snippet children(page)}<page.default
        launchConfigs={launchConfigs.state}
        repositories={[...repositories.list]}
        defaultCwd={setup.value.cwd}
        entry={target.entry}
        scope={target.scope}
        selectedRepositoryId={repositories.choice ?? "all"}
        intent={target.entry?.kind === "new-worktree" ? "worktree" : "session"}
        onchange={refreshWorktrees}
        onworktreecreated={(tree) => {
          const repo = repositories.list.find(
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
    load={() => import("./RepositorySettingsDialog.svelte")}
    label="仓库设置"
    onclose={() => (configuring = null)}
  >
    {#snippet children(page)}<page.default
        repository={target}
        onchange={() => void repositories.refresh(true)}
        onclose={() => (configuring = null)}
      />{/snippet}
  </LazyDialog>
{/if}
{#if forwarding}
  <LazyDialog
    load={() => import("./ForwardDialog.svelte")}
    label="端口转发"
    onclose={() => (forwarding = false)}
  >
    {#snippet children(page)}<page.default
        sessions={[...sessions.sessions]}
        detection={setup.value.portDetection}
        onclose={() => (forwarding = false)}
      />{/snippet}
  </LazyDialog>
{/if}
{#if adding}
  <LazyDialog
    load={() => import("./AddRepositoryDialog.svelte")}
    label="仓库表单"
    onclose={() => (adding = false)}
  >
    {#snippet children(page)}<page.default
        defaultCwd={setup.value.cwd}
        onchange={() => void repositories.refresh(true)}
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
      ownDrafts.remove(closing.id);
      archiveSignal.emit(closing.id);
    }}
    onarchived={(session) => {
      void archived.refresh();
      archiveSignal.emit(session.id);
    }}
    onclose={() => (closing = null)}
  />{/if}
{#if removingWorktree}
  {@const target = removingWorktree}
  <LazyDialog
    load={() => import("./RemoveWorktreeDialog.svelte")}
    label="删除确认"
    onclose={() => (removingWorktree = null)}
  >
    {#snippet children(page)}<page.default
        worktree={target.worktree}
        forgetRecords={target.forgetRecords ?? []}
        onremoved={target.onremoved}
        onhide={hideFromHome(target.worktree)}
        onchange={refreshWorktrees}
        onclose={() => (removingWorktree = null)}
      />{/snippet}
  </LazyDialog>
{/if}
{#if managingWorktrees}
  {@const target = managingWorktrees}
  {@const currentRepo =
    repositories.list.find((r) => r.id === target.id) ?? target}
  <LazyDialog
    load={() => import("./ManageWorktreesDialog.svelte")}
    label="管理工作树"
    onclose={() => (managingWorktrees = null)}
  >
    {#snippet children(page)}<page.default
        repository={currentRepo}
        sessions={sessions.sessions}
        archived={archived.records}
        home={homeWorktrees.value}
        onremoved={(removedIds) => {
          homeWorktrees.save(
            setPinned(homeWorktrees.value, currentRepo, removedIds, false),
          );
        }}
        onchange={refreshWorktrees}
        onclose={() => (managingWorktrees = null)}
      />{/snippet}
  </LazyDialog>
{/if}
{#if choosingHome}
  {@const target = choosingHome}
  {@const currentRepo =
    repositories.list.find((r) => r.id === target.id) ?? target}
  <LazyDialog
    load={() => import("./HomeWorktreeDialog.svelte")}
    label="主页显示"
    onclose={() => (choosingHome = null)}
  >
    {#snippet children(page)}<page.default
        repository={currentRepo}
        sessions={sessions.sessions}
        records={archived.records}
        home={homeWorktrees.value}
        ontoggle={(worktreeId, pinned) =>
          homeWorktrees.save(
            setPinned(homeWorktrees.value, currentRepo, [worktreeId], pinned),
          )}
        onmanage={() => {
          choosingHome = null;
          shell.manageWorktrees(currentRepo);
        }}
        onclose={() => (choosingHome = null)}
      />{/snippet}
  </LazyDialog>
{/if}
{#if viewingRepository}
  {@const target = viewingRepository}
  {@const currentRepo =
    repositories.list.find((r) => r.id === target.id) ?? target}
  <LazyDialog
    load={() => import("./RepositoryDetailsDialog.svelte")}
    label="仓库详情"
    onclose={() => (viewingRepository = null)}
  >
    {#snippet children(page)}<page.default
        repository={currentRepo}
        sessions={sessions.sessions}
        records={archived.records}
        home={homeWorktrees.value}
        refreshing={repositories.refreshing}
        registryReady={setup.value.registryReady}
        onunregister={() => {
          unregistering = currentRepo;
          unregisterOpen = true;
        }}
        onclose={() => (viewingRepository = null)}
      />{/snippet}
  </LazyDialog>
{/if}
<UnregisterRepositoryDialog
  repository={unregistering}
  bind:open={unregisterOpen}
  onchange={() => repositories.refresh(true)}
/>
{#if releasingWorktree}
  {@const target = releasingWorktree}
  <LazyDialog
    load={() => import("./ReleaseWorktreeDialog.svelte")}
    label="释放空间预览"
    onclose={() => (releasingWorktree = null)}
  >
    {#snippet children(page)}<page.default
        worktree={target}
        onchange={refreshWorktrees}
        onclose={() => (releasingWorktree = null)}
      />{/snippet}
  </LazyDialog>
{/if}
