<script lang="ts" module>
  import type {
    Branches,
    FetchRemotesResponse,
    Remotes,
  } from "../../contracts/schemas";

  /** Where the panel reads and fetches; stories pass fixtures. */
  export interface RepositorySource {
    branches(id: string, signal: AbortSignal): Promise<Branches>;
    remotes(id: string, signal: AbortSignal): Promise<Remotes>;
    fetch(id: string): Promise<FetchRemotesResponse>;
  }
</script>

<script lang="ts">
  import { onDestroy, tick, untrack } from "svelte";
  import IconArrowLeft from "~icons/tabler/arrow-left";
  import IconCloudDownload from "~icons/tabler/cloud-download";
  import IconLoader2 from "~icons/tabler/loader-2";
  import { Button } from "$lib/components/ui/button";
  import * as Tabs from "$lib/components/ui/tabs";
  import {
    fetchRepositoryRemotes,
    repositoryBranches,
    repositoryRemotes,
  } from "../../platform/api";
  import {
    operationTitle,
    type BranchOperation,
  } from "../../domains/workspace/branch-actions";
  import { errorMessage } from "../../platform/error";
  import { LatestRequest } from "../../hooks/shared/remote-request.svelte";
  import { worktreeTitle } from "../../domains/workspace/home-worktrees";
  import type { WorktreePreset } from "../../domains/sessions/worktree-preset";
  import type {
    ArchivedSession,
    FetchResult,
    HomeWorktrees,
    Remote,
    Repository,
    SessionInfo,
  } from "../../contracts/schemas";
  import type { RepositoryTab } from "../../hooks/shared/shell.svelte";
  import BranchOperationFlow from "./BranchOperationFlow.svelte";
  import InlineNotice from "../shared/InlineNotice.svelte";
  import RepositoryBranches from "./RepositoryBranches.svelte";
  import RepositoryOverview from "./RepositoryOverview.svelte";
  import RepositoryRemotes from "./RepositoryRemotes.svelte";
  import RepositoryWorktrees from "./RepositoryWorktrees.svelte";
  import ResponsiveDialog from "../shared/ResponsiveDialog.svelte";
  import SwitchBranch, { type SwitchSource } from "./SwitchBranch.svelte";
  import WorktreeDetails from "./WorktreeDetails.svelte";
  import { switchableWorktree } from "../../domains/workspace/worktree-sync";

  // The repository panel: overview, worktrees, branches and remotes as line
  // tabs in one dialog (a bottom drawer on phones). A worktree's details and
  // the switch of its branch open inside it as a stack of subviews with
  // "返回" instead of a second dialog; the tabs stay mounted underneath, so
  // their search, filters and scroll survive. While a section's request or
  // a switch is in flight (`busy`), going back, switching sections and
  // closing are all blocked. A branch operation (fast-forward, push, delete)
  // is a subview too, over the tabs or a worktree's details. "获取远程" in the title bar acts on the whole
  // repository from every section; its per-remote results stay below the
  // title until the panel closes or the next fetch.
  let {
    repository,
    sessions,
    records,
    home,
    refreshing,
    refreshError,
    registryReady,
    initialTab = "overview",
    source = {
      branches: repositoryBranches,
      remotes: repositoryRemotes,
      fetch: fetchRepositoryRemotes,
    },
    switchSource,
    onunregister,
    onchange,
    onrefresh,
    onnewworktree,
    onrepository,
    onremoved,
    onclose,
  }: {
    repository: Repository;
    sessions: readonly SessionInfo[];
    records: readonly ArchivedSession[];
    home: HomeWorktrees;
    /** The repositories are being read again; this is the last snapshot. */
    refreshing: boolean;
    /** The last read's failure; "" after a successful one. */
    refreshError: string;
    registryReady: boolean;
    initialTab?: RepositoryTab;
    source?: RepositorySource;
    /** Where the switch reads and writes; stories pass fixtures. */
    switchSource?: SwitchSource;
    onunregister: () => void;
    onchange: () => void;
    /** A switch ran: the repositories are read again at once. */
    onrefresh: () => void;
    /** Closes the panel and opens the new-worktree dialog with a preset. */
    onnewworktree: (preset: WorktreePreset) => void;
    /** A fetch read the repository again; replaces the listed snapshot. */
    onrepository?: (repository: Repository) => void;
    onremoved?: (removedIds: string[]) => void;
    onclose: () => void;
  } = $props();

  const available = $derived(repository.availability === "available");
  let tab = $state<RepositoryTab>(untrack(() => initialTab));
  let busy = $state(false);
  // The subviews over the tabs, the last one shown; empty for the tabs.
  type Subview =
    | { kind: "detail" | "switch"; id: string }
    | { kind: "operation"; operation: BranchOperation };
  let views = $state<Subview[]>([]);
  const top = $derived(views.at(-1));
  const topTree = $derived(
    top && top.kind !== "operation"
      ? repository.worktrees.find((tree) => tree.id === top.id)
      : undefined,
  );
  // Bumped after an operation so the branches section reads them again.
  let branchesVersion = $state(0);
  const tabs: { id: RepositoryTab; label: string }[] = [
    { id: "overview", label: "概览" },
    { id: "worktrees", label: "工作树" },
    { id: "branches", label: "分支" },
    { id: "remotes", label: "远程" },
  ];
  const descriptions: Record<RepositoryTab, string> = {
    overview: "仓库路径、默认分支、远程与 Git 状态。",
    worktrees:
      "该仓库的全部工作树（含未添加到主页的项），可查看详情或批量清理。删除工作树不会删除本地分支。",
    branches: "本地分支与远程跟踪分支，按本机记录读取，不访问网络。",
    remotes: "远程名称与地址；地址中的密码与令牌已隐藏。",
  };

  // Remotes are read once when the panel opens: the overview shows their
  // count and "获取远程" needs at least one.
  let remotes = $state<Remote[] | undefined>();
  let remotesError = $state("");
  const remoteRequests = new LatestRequest();
  onDestroy(() => remoteRequests.dispose());
  async function readRemotes() {
    const request = remoteRequests.begin();
    remotesError = "";
    try {
      const result = await source.remotes(repository.id, request.signal);
      if (request.current()) remotes = result.remotes;
    } catch (err) {
      if (request.reportable()) remotesError = errorMessage(err);
    }
  }
  let remotesAsked = false;
  $effect(() => {
    if (!available || remotesAsked) return;
    remotesAsked = true;
    untrack(() => void readRemotes());
  });

  // The server finishes a fetch even if this panel closes, so its reply
  // still replaces the repository snapshot then.
  let fetching = $state(false);
  let fetchResults = $state.raw<FetchResult[] | null>(null);
  let fetchError = $state("");
  // Bumped after a fetch so the branches section reads its upstreams again.
  let branchesRead = $state(0);
  const fetchBlocked = $derived(
    remotes === undefined && !remotesError
      ? "正在读取远程…"
      : remotes?.length === 0
        ? "这个仓库没有配置远程，无法获取"
        : "",
  );
  const failedFetches = $derived(
    fetchResults?.filter((result) => result.status !== "ok").length ?? 0,
  );
  async function fetchRemotes() {
    if (fetching || fetchBlocked) return;
    fetching = true;
    fetchResults = null;
    fetchError = "";
    try {
      const response = await source.fetch(repository.id);
      fetchResults = response.results;
      onrepository?.(response.repository);
      branchesRead++;
    } catch (err) {
      fetchError = errorMessage(err);
      // A conflict may mean the remotes changed since they were read.
      void readRemotes();
    } finally {
      fetching = false;
    }
  }

  // Scroll offsets of the tabs and the dialog body, restored on "返回".
  let body = $state<HTMLElement>();
  let tabsView = $state<HTMLElement>();
  let saved: [HTMLElement, number][] = [];
  function dialogScrollers(): HTMLElement[] {
    const out: HTMLElement[] = [];
    for (let el = body?.parentElement; el; el = el.parentElement) {
      out.push(el);
      if (el.getAttribute("role") === "dialog") break;
    }
    return out;
  }
  // Opens a subview; leaving the tabs saves their scroll for "返回".
  async function push(view: Subview) {
    if (busy) return;
    const outer = dialogScrollers();
    if (!views.length)
      saved = [
        ...(tabsView?.querySelectorAll<HTMLElement>("*") ?? []),
        ...outer,
      ]
        .filter((el) => el.scrollTop > 0)
        .map((el) => [el, el.scrollTop]);
    views = [...views, view];
    await tick();
    for (const el of outer) el.scrollTop = 0;
  }
  const openWorktree = (id: string) => void push({ kind: "detail", id });
  const openOperation = (operation: BranchOperation) =>
    void push({ kind: "operation", operation });
  function operated() {
    branchesVersion++;
    onrefresh();
  }
  function openPath(path: string) {
    const tree = repository.worktrees.find((w) => w.path === path);
    if (!tree) return;
    if (views.length) replaceTop({ kind: "detail", id: tree.id });
    else openWorktree(tree.id);
  }
  async function back() {
    if (busy) return;
    views = views.slice(0, -1);
    if (views.length) return;
    await tick();
    for (const [el, offset] of saved) el.scrollTop = offset;
    saved = [];
  }
  const replaceTop = (view: Subview) => (views = [...views.slice(0, -1), view]);
  function switched(id: string) {
    const below = views.at(-2);
    if (below?.kind === "detail" && below.id === id) views = views.slice(0, -1);
    else replaceTop({ kind: "detail", id });
  }
  const worktreePaths = $derived(
    new Set(repository.worktrees.map((t) => t.path)),
  );
  const fetchReasonId = $props.id();
  const fetchLabel = $derived(fetching ? "正在获取远程" : "获取远程");
</script>

{#snippet subviewHeader(heading: string)}
  <div class="flex min-w-0 items-center gap-2">
    <Button
      variant="ghost"
      size="sm"
      class="-ml-2"
      disabled={busy}
      onclick={() => void back()}
      ><IconArrowLeft aria-hidden="true" />返回</Button
    >
    <h3 class="min-w-0 text-sm font-medium break-all">{heading}</h3>
  </div>
{/snippet}

{#snippet fetchAction()}
  <!-- A disabled button shows no hint, so the wrapper carries the reason. -->
  <span class="inline-flex" title={fetchBlocked || fetchLabel}>
    <Button
      variant="outline"
      size="sm"
      class="max-sm:size-(--control-default)"
      aria-label={fetchLabel}
      aria-describedby={fetchBlocked ? fetchReasonId : undefined}
      aria-busy={fetching}
      disabled={fetching || !!fetchBlocked}
      onclick={() => void fetchRemotes()}
    >
      {#if fetching}
        <IconLoader2
          aria-hidden="true"
          class="animate-spin motion-reduce:animate-none"
        />
      {:else}
        <IconCloudDownload aria-hidden="true" />
      {/if}
      <span class="hidden sm:inline">{fetching ? "正在获取…" : "获取远程"}</span
      >
    </Button>
    {#if fetchBlocked}
      <span id={fetchReasonId} class="sr-only">{fetchBlocked}</span>
    {/if}
  </span>
{/snippet}

<ResponsiveDialog
  title={repository.name}
  description={available ? descriptions[tab] : descriptions.overview}
  class="sm:max-w-2xl"
  drawerDescription="概览、工作树、分支与远程"
  pinDrawerHeader
  truncateDrawerTitle
  actions={available ? fetchAction : undefined}
  {busy}
  {onclose}
>
  <div bind:this={body} class="min-w-0 space-y-3">
    {#if refreshing}
      <p role="status" class="text-xs text-muted-foreground">
        正在重新读取状态…（先显示上次读取的结果）
      </p>
    {:else if refreshError}
      <InlineNotice tone="danger"
        >重新读取失败：{refreshError}。以下为 {new Date(
          repository.checkedAt,
        ).toLocaleString()} 读取的状态，可能已过时。</InlineNotice
      >
    {/if}
    {#if fetchError}
      <InlineNotice tone="danger">获取远程失败：{fetchError}</InlineNotice>
    {:else if fetchResults}
      {@const tone =
        failedFetches === 0
          ? "success"
          : failedFetches === fetchResults.length
            ? "danger"
            : "warning"}
      <InlineNotice {tone}>
        <ul class="space-y-1" aria-label="获取结果">
          {#each fetchResults as result (result.remote)}
            <li class="wrap-anywhere">
              <span class="font-medium">{result.remote}</span
              >：{result.status === "ok"
                ? "已获取"
                : result.message || "获取失败"}
            </li>
          {/each}
        </ul>
      </InlineNotice>
    {/if}
    <div bind:this={tabsView} hidden={!!top}>
      {#if available}
        <Tabs.Root
          value={tab}
          onValueChange={(value) => {
            const next = tabs.find((item) => item.id === value);
            if (next && !busy) tab = next.id;
          }}
        >
          <!-- Wraps instead of overflowing with large text on a narrow phone. -->
          <Tabs.List
            variant="line"
            class="max-w-full flex-wrap group-data-horizontal/tabs:h-auto"
            aria-label="仓库面板分区"
          >
            {#each tabs as item (item.id)}
              <Tabs.Trigger value={item.id} disabled={busy && tab !== item.id}
                >{item.label}</Tabs.Trigger
              >
            {/each}
          </Tabs.List>
          <Tabs.Content value="overview" class="pt-2">
            <RepositoryOverview
              {repository}
              {sessions}
              {records}
              {home}
              remoteCount={remotes?.length}
              {registryReady}
              {onunregister}
            />
          </Tabs.Content>
          <Tabs.Content value="worktrees" class="pt-2">
            <RepositoryWorktrees
              {repository}
              {sessions}
              archived={records}
              {home}
              bind:busy
              onopen={openWorktree}
              onswitch={(id) => void push({ kind: "switch", id })}
              onoperation={openOperation}
              {onchange}
              {onremoved}
            />
          </Tabs.Content>
          <Tabs.Content value="branches" class="pt-2">
            {#key branchesRead}
              <RepositoryBranches
                active={tab === "branches"}
                load={(signal) => source.branches(repository.id, signal)}
                {worktreePaths}
                onopen={openPath}
                {onnewworktree}
                onoperation={openOperation}
                version={branchesVersion}
              />
            {/key}
          </Tabs.Content>
          <Tabs.Content value="remotes" class="pt-2">
            <RepositoryRemotes
              {remotes}
              error={remotesError}
              onretry={() => void readRemotes()}
            />
          </Tabs.Content>
        </Tabs.Root>
      {:else}
        <RepositoryOverview
          {repository}
          {sessions}
          {records}
          {home}
          remoteCount={undefined}
          {registryReady}
          {onunregister}
        />
      {/if}
    </div>
    {#if top?.kind === "operation"}
      {@const heading = operationTitle(top.operation)}
      <section class="min-w-0 space-y-3" aria-label={heading}>
        {@render subviewHeader(heading)}
        {#key top}
          <BranchOperationFlow
            repositoryId={repository.id}
            operation={top.operation}
            bind:busy
            onchange={operated}
            onclose={() => void back()}
          />
        {/key}
      </section>
    {:else if top}
      {@const title = topTree ? worktreeTitle(topTree) : "工作树"}
      <section
        class="min-w-0 space-y-3"
        aria-label={top.kind === "switch"
          ? `切换分支：${title}`
          : topTree
            ? `工作树详情：${title}`
            : "工作树详情"}
      >
        {@render subviewHeader(
          top.kind === "switch" ? `切换分支：${title}` : title,
        )}
        {#if top.kind === "detail" && topTree && switchableWorktree(topTree)}
          {@const id = topTree.id}
          <Button
            variant="outline"
            size="sm"
            disabled={busy}
            onclick={() => void push({ kind: "switch", id })}>切换分支…</Button
          >
        {/if}
        {#if !topTree}
          <p class="text-xs text-muted-foreground">
            这个工作树已不在仓库中，可能已被删除。
          </p>
        {:else if top.kind === "switch"}
          {@const id = topTree.id}
          {#key id}<SwitchBranch
              {repository}
              worktree={topTree}
              source={switchSource}
              bind:busy
              onchange={onrefresh}
              ondone={() => switched(id)}
              {onnewworktree}
              ongoto={openPath}
              oncancel={() => void back()}
            />{/key}
        {:else}
          <WorktreeDetails
            path={topTree.path}
            worktree={topTree}
            sessions={sessions.filter(
              (s) => s.workspace?.worktreeId === topTree.id,
            )}
            lastFetchedAt={repository.lastFetchedAt}
            onoperation={openOperation}
          />
        {/if}
      </section>
    {/if}
  </div>
</ResponsiveDialog>
