<script lang="ts">
  import IconLayoutSidebarLeftCollapse from "~icons/tabler/layout-sidebar-left-collapse";
  import IconMessagePlus from "~icons/tabler/message-plus";
  import IconPlus from "~icons/tabler/plus";
  import { Button } from "$lib/components/ui/button";
  import { ScrollArea } from "$lib/components/ui/scroll-area";
  import { errorMessage } from "../../platform/error";
  import { recordTitle } from "../../domains/sessions/archived";
  import { homeWorktrees } from "../../domains/workspace/home-worktrees.svelte";
  import { connectorMissingText } from "../../domains/sessions/hybrid";
  import {
    removeRecord,
    resumeSession,
  } from "../../domains/sessions/recoverable";
  import { sessionPins } from "../../domains/sessions/session-pins.svelte";
  import type { Route } from "../../hooks/shared/route";
  import type {
    ArchivedSession,
    HybridRelease,
    Worktree,
  } from "../../contracts/schemas";
  import {
    getShell,
    openPage,
    openSession,
    shellUi,
    type ShellMode,
  } from "../../hooks/shared/shell.svelte";
  import Brand from "./Brand.svelte";
  import ConfirmDialog from "../shared/ConfirmDialog.svelte";
  import ChatHistory from "../chat/ChatHistory.svelte";
  import RebuildWorktreeDialog from "../workspace/RebuildWorktreeDialog.svelte";
  import SidebarLiveSessionRow from "./SidebarLiveSessionRow.svelte";
  import ModeSwitch from "./ModeSwitch.svelte";
  import SidebarFooter from "./SidebarFooter.svelte";
  import WorkbenchTree from "./WorkbenchTree.svelte";

  // The sidebar body, the same on desktop and in the phone drawer: brand and
  // mode switch, the mode's primary action, its navigation, then the global
  // footer that replaced the home page header.
  let {
    route,
    collapsible = false,
    onmode,
    onprimary,
  }: {
    route: Extract<Route, { name: "sessions" | "quick-start" | "terminal" }>;
    /** Desktop only; the drawer closes on its own. */
    collapsible?: boolean;
    onmode: (mode: ShellMode) => void;
    onprimary: () => void;
  } = $props();
  const shell = getShell();
  const current = $derived(route.name === "terminal" ? route.id : "");
  const live = $derived(
    shell.sessions.sessions.filter((session) => session.state === "active"),
  );
  const pinned = $derived(sessionPins.ordered(live));
  const pinnedIDs = $derived(new Set(pinned.map((session) => session.id)));
  const unpinned = $derived(
    live.filter((session) => !pinnedIDs.has(session.id)),
  );
  const configs = $derived(
    shell.launchConfigs.state.status === "ready"
      ? shell.launchConfigs.state.configs
      : [],
  );
  const configIcons = $derived(
    new Map(configs.map((config) => [config.id, config.icon ?? ""])),
  );
  const setup = $derived(shell.setup.value);

  let resuming = $state("");
  // The last failed sidebar action (resume or record deletion), shown above
  // the list.
  let resumeError = $state("");
  let removing = $state<ArchivedSession | null>(null);
  let removeOpen = $state(false);
  function askRemove(record: ArchivedSession) {
    removing = record;
    removeOpen = true;
  }
  function askRemoveWithWorktree(record: ArchivedSession, worktree: Worktree) {
    shell.removeWorktree(worktree, {
      onremoved: () => void forgetAfterWorktree(record),
      forgetRecords: [record],
    });
  }
  // Runs a worktree action of the tree, by the IDs its rows carry.
  function withTree(action: (tree: Worktree) => void) {
    return (repositoryId: string, worktreeId: string) => {
      const tree = shell.repositories.list
        .find((r) => r.id === repositoryId)
        ?.worktrees.find((w) => w.id === worktreeId);
      if (tree) action(tree);
    };
  }
  // The worktree is already gone when this runs; a failed record deletion
  // leaves a released record behind, which the user can delete again.
  async function forgetAfterWorktree(record: ArchivedSession) {
    resumeError = "";
    try {
      await removeRecord(record.id);
      sessionPins.refreshEnded(record.id);
      shell.drafts.remove(record.id);
    } catch (err) {
      resumeError = `工作树已删除，但无法删除“${recordTitle(record)}”的记录：${errorMessage(err)}`;
    } finally {
      await shell.archived.refresh();
    }
  }
  async function confirmRemove() {
    const record = removing;
    removeOpen = false;
    if (!record) return;
    resumeError = "";
    try {
      await removeRecord(record.id);
      sessionPins.refreshEnded(record.id);
      shell.drafts.remove(record.id);
      await shell.archived.refresh();
    } catch (err) {
      resumeError = `无法删除“${recordTitle(record)}”的记录：${errorMessage(err)}`;
    }
  }
  // Resume on the recorded view, independent of the sidebar's mode. Choices
  // that need the user are made on the record's own page. A released record's
  // worktree is rebuilt first, once the user confirms how.
  let rebuilding = $state<{
    record: ArchivedSession;
    released: HybridRelease;
  } | null>(null);
  function open(record: ArchivedSession) {
    if (record.released) rebuilding = { record, released: record.released };
    else void resume(record);
  }
  function rebuilt() {
    const target = rebuilding;
    rebuilding = null;
    if (!target) return;
    // The worktree is back and the release cleared, whatever the resume does.
    void shell.repositories.refresh();
    void shell.archived.refresh();
    void resume(target.record);
  }
  async function resume(record: ArchivedSession) {
    resuming = record.id;
    resumeError = "";
    try {
      const outcome = await resumeSession(record.id, record.view);
      if (outcome.status === "resumed") {
        void shell.archived.refresh();
        openSession(outcome.session);
      } else if (outcome.status === "connector_missing")
        resumeError = `${outcome.message}。${connectorMissingText(outcome.connector)}`;
      else openPage(`/s/${record.id}`);
    } catch (err) {
      resumeError = `无法恢复“${record.title}”：${errorMessage(err)}`;
    } finally {
      resuming = "";
    }
  }
  function scope(next: string, worktreeId?: string) {
    shellUi.setMode("workbench");
    shell.repositories.select(next, worktreeId);
    openPage("/");
  }
</script>

<nav class="flex h-full min-h-0 flex-col" aria-label="导航">
  <div class="flex items-center gap-1 px-2 pt-2 pb-1">
    {#if collapsible}
      <Button
        class="shrink-0"
        variant="ghost"
        size="icon-touch"
        aria-label="收起侧栏"
        title="收起侧栏"
        onclick={() => shellUi.setCollapsed(true)}
        ><IconLayoutSidebarLeftCollapse aria-hidden="true" /></Button
      >
    {/if}
    <span class="brand mr-auto min-w-0 overflow-hidden px-1 whitespace-nowrap"
      ><Brand /></span
    >
    <ModeSwitch mode={shellUi.mode} onchange={onmode} />
  </div>
  <div class="px-2 pt-1">
    <Button
      variant="outline"
      size="sm-touch"
      class="w-full"
      onclick={onprimary}
    >
      {#if shellUi.mode === "chat"}<IconMessagePlus
          aria-hidden="true"
        />新聊天{:else}<IconPlus aria-hidden="true" />新建会话{/if}
    </Button>
  </div>
  {#if pinned.length}
    <section
      class="max-h-36 shrink-0 overflow-y-auto px-2 pt-2"
      aria-label="已置顶会话"
    >
      <h2 class="px-3 pb-1 text-xs text-muted-foreground">置顶</h2>
      <ul class="space-y-0.5">
        {#each pinned as session (session.id)}
          <SidebarLiveSessionRow
            {session}
            {current}
            icon={session.agent?.icon ??
              configIcons.get(session.configId ?? "") ??
              ""}
            pinned
            onopen={openSession}
            onrename={shell.rename}
            onclose={shell.close}
          />
        {/each}
      </ul>
    </section>
  {/if}
  <ScrollArea
    type="hover"
    class="min-h-0 flex-1 [&_[data-slot=scroll-area-viewport]]:overscroll-contain [&_[data-slot=scroll-area-viewport]>div]:!block"
  >
    <div class="px-2 py-2">
      {#if resumeError || sessionPins.error}<p
          role="alert"
          class="mb-2 rounded-md border border-destructive/40 px-3 py-2 text-xs text-destructive"
        >
          {resumeError || sessionPins.error}
        </p>{/if}
      {#if shellUi.mode === "chat"}
        <ChatHistory
          sessions={unpinned}
          records={shell.archived.records}
          {configs}
          repositories={shell.repositories.list}
          {current}
          {resuming}
          onopen={openSession}
          onresume={open}
          onrename={shell.rename}
          onclose={shell.close}
          onrelease={shell.releaseWorktree}
          onremove={askRemove}
          onremoveworktree={askRemoveWithWorktree}
        />
      {:else if !shell.sessions.loading && (shell.repositories.settled || shell.repositories.list.length)}
        <!-- Drawn once sessions and repositories are known, so rows never
           appear between rows already on screen during startup. -->
        <WorkbenchTree
          repositories={shell.repositories.list}
          sessions={unpinned}
          home={homeWorktrees.value}
          scope={route.name === "sessions"
            ? (shell.repositories.choice ?? "all")
            : ""}
          worktree={route.name === "sessions"
            ? (shell.repositories.worktree ?? "")
            : ""}
          {current}
          canCreate={setup.gitReady && setup.registryReady}
          {configIcons}
          onscope={scope}
          onopen={openSession}
          onnewworktree={(repositoryId) =>
            shell.start({ kind: "new-worktree", repositoryId })}
          onhomeworktrees={(repositoryId) => {
            const repo = shell.repositories.list.find(
              (r) => r.id === repositoryId,
            );
            if (repo) shell.chooseHomeWorktrees(repo);
          }}
          onmanageworktrees={(repositoryId) => {
            const repo = shell.repositories.list.find(
              (r) => r.id === repositoryId,
            );
            if (repo) shell.repositoryDetails(repo, "worktrees");
          }}
          onrepositorydetails={(repositoryId) => {
            const repo = shell.repositories.list.find(
              (r) => r.id === repositoryId,
            );
            if (repo) shell.repositoryDetails(repo);
          }}
          onworktreedetails={withTree((tree) => shell.worktreeDetails(tree))}
          onremoveworktree={withTree((tree) => shell.removeWorktree(tree))}
          onswitchbranch={withTree((tree) => shell.switchBranch(tree))}
          onbranchoperation={shell.branchOperation}
          onrename={shell.rename}
          onclose={shell.close}
          records={shell.archived.records}
          {resuming}
          onresume={open}
          onremove={askRemove}
        />
      {/if}
    </div>
  </ScrollArea>
  <SidebarFooter
    connection={shell.sessions.connection}
    hostname={setup.hostname}
    setupSettled={shell.setup.settled}
    onforward={shell.forward}
    onadd={shell.addRepository}
    canAdd={setup.gitReady && setup.registryReady}
  />
</nav>

<ConfirmDialog
  bind:open={removeOpen}
  title={`删除“${removing ? recordTitle(removing) : ""}”的记录？`}
  onconfirm={() => void confirmRemove()}
>
  只删除 WebMux 的记录与本机草稿；工作树与 agent 自己保存的会话不受影响。
</ConfirmDialog>
{#if rebuilding}
  {@const { record, released } = rebuilding}
  <RebuildWorktreeDialog
    id={record.id}
    title={recordTitle(record)}
    {released}
    onclose={() => (rebuilding = null)}
    onrebuilt={rebuilt}
  />
{/if}
