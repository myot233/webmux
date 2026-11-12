<script lang="ts">
  import RepositoryPanel from "../src/components/workspace/RepositoryPanel.svelte";
  import WorktreeSyncSummary from "../src/components/workspace/WorktreeSyncSummary.svelte";
  import { Button } from "../src/lib/components/ui/button";
  import type {
    Branches,
    FetchRemotesResponse,
    Repository,
    SessionInfo,
    Worktree,
  } from "../src/contracts/schemas";
  import type { RepositoryTab } from "../src/hooks/shared/shell.svelte";
  import { longPath, longToken, session, worktree } from "./fixtures";

  // The repository panel at each section, and the worktree sync summary's
  // forms and states, with long names that must wrap instead of overflowing.
  // `fetchReply` picks how "获取远程" answers: never ("pending") or with one
  // remote fetched and one failed ("partial").
  let {
    tab = "overview",
    sync = false,
    fetchReply = "partial",
  }: {
    tab?: RepositoryTab;
    sync?: boolean;
    fetchReply?: "pending" | "partial";
  } = $props();
  let open = $state(true);

  const at = "2026-01-01T09:00:00Z";
  const tree = (id: string, patch: Partial<Worktree>): Worktree => ({
    ...worktree,
    id,
    repositoryId: "storybook-repository",
    kind: "linked",
    path: `${longPath}/${id}`,
    branch: `feature/${id}-${longToken}`,
    head: "1234567890abcdef1234567890abcdef12345678",
    locked: false,
    lockReason: "",
    prunable: false,
    available: true,
    dirty: false,
    hasUntracked: false,
    hasIgnored: false,
    error: "",
    operation: null,
    ...patch,
  });
  const trees = [
    tree("main", {
      kind: "main",
      branch: "main",
      path: longPath,
      upstream: "origin/main",
      ahead: 0,
      behind: 0,
    }),
    tree("ahead", {
      dirty: true,
      hasUntracked: true,
      operation: "rebase",
    }),
    tree("gone", { upstream: `origin/gone-${longToken}`, upstreamGone: true }),
    tree("uncounted", {
      upstream: `origin/uncounted-${longToken}`,
      ahead: null,
      behind: null,
    }),
    tree("untracked", { upstream: null, ahead: null, behind: null }),
    tree("unknown", { dirty: null, changes: null, error: "git status 超时" }),
    // Directories deleted outside WebMux; the locked one stays registered.
    tree("stale", {
      prunable: true,
      available: false,
      dirty: null,
      changes: null,
    }),
    tree("locked-stale", {
      prunable: true,
      available: false,
      locked: true,
      lockReason: `移动硬盘 ${longToken}`,
      dirty: null,
      changes: null,
    }),
  ];
  const repository: Repository = {
    id: "storybook-repository",
    name: `仓库 ${longToken}`,
    commonDir: `${longPath}/.git`,
    anchorPath: longPath,
    availability: "available",
    error: "",
    checkedAt: at,
    lastFetchedAt: at,
    initInstruction: "",
    worktrees: trees,
  };
  const sessions: SessionInfo[] = [
    {
      ...session,
      workspace: {
        repositoryId: repository.id,
        worktreeId: "ahead",
        worktreePath: trees[1]!.path,
      },
    },
  ];
  const branches: Branches = {
    branches: trees.map((item) => ({
      name: item.branch ?? "",
      head: item.head ?? "",
      worktreePaths: [item.path],
      subject: item.lastCommit?.subject ?? "",
      committedAt: at,
      upstream: item.upstream,
      ahead: item.ahead,
      behind: item.behind,
      upstreamGone: item.upstreamGone,
      mergedIntoDefault: false,
    })),
    bases: [
      {
        name: `refs/remotes/origin/${longToken}`,
        head: "abcdef1234567890abcdef1234567890abcdef12",
        subject: `远程提交 ${longToken}`,
        kind: "remote",
        committedAt: at,
      },
    ],
    defaultBase: null,
    defaultBaseSubject: "",
    defaultBranch: "main",
    defaultParent: longPath,
  };
  let shown = $state.raw(repository);
  const forkName = `fork-${longToken}`;
  const source = {
    branches: async () => branches,
    remotes: async () => ({
      remotes: [
        {
          name: "origin",
          fetchUrl: `https://user:***@github.com/${longToken}/repo.git`,
          pushUrl: `https://user:***@github.com/${longToken}/repo.git`,
        },
        {
          name: forkName,
          fetchUrl: `https://***@github.com/${longToken}/fork.git`,
          pushUrl: `git@github.com:${longToken}/fork.git`,
        },
      ],
    }),
    fetch: (): Promise<FetchRemotesResponse> =>
      fetchReply === "pending"
        ? new Promise(() => {})
        : Promise.resolve({
            results: [
              {
                remote: forkName,
                status: "auth",
                message: `认证失败，请在终端完成认证后重试：fatal: could not read Username for 'https://github.com/${longToken}/fork.git': terminal prompts disabled`,
              },
              { remote: "origin", status: "ok", message: "" },
            ],
            repository: {
              ...repository,
              worktrees: repository.worktrees.map((item) =>
                item.kind === "main" ? { ...item, behind: 3 } : item,
              ),
            },
          }),
  };
</script>

<main class="mx-auto w-full max-w-5xl p-4" data-testid="repository-story">
  {#if sync}
    <div class="space-y-6">
      {#each trees as item (item.id)}
        <section class="min-w-0 space-y-2 border-b pb-4" aria-label={item.id}>
          <div class="flex min-w-0 flex-wrap items-center gap-1.5">
            <WorktreeSyncSummary worktree={item} variant="indicator" />
          </div>
          <WorktreeSyncSummary worktree={item} />
        </section>
      {/each}
      <WorktreeSyncSummary
        worktree={trees[1]!}
        variant="full"
        lastFetchedAt={at}
      />
    </div>
  {:else}
    <Button onclick={() => (open = true)}>打开仓库面板</Button>
    {#if open}
      <RepositoryPanel
        repository={shown}
        onrepository={(next) => (shown = next)}
        {sessions}
        records={[]}
        home={{}}
        refreshing={false}
        refreshError=""
        registryReady
        initialTab={tab}
        {source}
        onunregister={() => {}}
        onchange={() => {}}
        onrefresh={() => {}}
        onnewworktree={() => {}}
        onclose={() => (open = false)}
      />
    {/if}
  {/if}
</main>
