<script lang="ts">
  import { onMount } from "svelte";
  import {
    IconSun,
    IconMoon,
    IconChevronDown,
    IconSettings,
    IconFolderPlus,
    IconRefresh,
    IconTerminal2,
    IconSearch,
  } from "@tabler/icons-svelte";
  import { toggleTheme } from "../theme";
  import { api } from "../api";
  import {
    PresetsSchema,
    ConfigSchema,
    SessionEventSchema,
    RepositoriesSchema,
    NoContentSchema,
    type Preset,
    type SessionInfo,
    type Repository,
    type Worktree,
  } from "../schemas";
  import { connect, type Connection } from "../ws";
  import { Button } from "$lib/components/ui/button";
  import { Badge } from "$lib/components/ui/badge";
  import { Input } from "$lib/components/ui/input";
  import { Skeleton } from "$lib/components/ui/skeleton";
  import * as DropdownMenu from "$lib/components/ui/dropdown-menu";
  import * as AlertDialog from "$lib/components/ui/alert-dialog";
  import * as Collapsible from "$lib/components/ui/collapsible";
  import * as Empty from "$lib/components/ui/empty";
  import NewSessionSheet from "../components/NewSessionSheet.svelte";
  import CloseSessionDialog from "../components/CloseSessionDialog.svelte";
  import WorkspaceGroup from "../components/WorkspaceGroup.svelte";
  import AddRepositoryDialog from "../components/AddRepositoryDialog.svelte";
  import RemoveWorktreeDialog from "../components/RemoveWorktreeDialog.svelte";
  import type { TerminalDrafts } from "../terminal-draft.svelte";

  let { drafts }: { drafts: TerminalDrafts } = $props();

  let sessions = $state<SessionInfo[]>([]);
  let repositories = $state<Repository[]>([]);
  let presets = $state<Preset[]>([]);
  let cwd = $state("~");
  let loading = $state(true);
  let refreshing = $state(false);
  let repositoryError = $state("");
  let capabilityError = $state("");
  let gitReady = $state(false);
  let registryReady = $state(false);
  let connection = $state<Connection>("connecting");
  let creating = $state<{
    mode: "direct" | "existing" | "new";
    repositoryId: string;
    worktreeId: string;
  } | null>(null);
  let adding = $state(false);
  let removing = $state<Worktree | null>(null);
  let unregistering = $state<Repository | null>(null);
  let unregisterOpen = $state(false);
  let unregisterBusy = $state(false);
  let unregisterError = $state("");
  let closing = $state<SessionInfo | null>(null);
  let error = $state("");
  let now = $state(Date.now());
  let dark = $state(document.documentElement.classList.contains("dark"));
  let expanded = $state<Record<string, boolean>>({});
  let query = $state("");
  let statusFilter = $state<"all" | "active" | "waiting" | "exited">("all");
  let filterExpanded = $state<Record<string, boolean>>({});
  let refreshGeneration = 0;
  let disposed = false;
  const list = $derived(
    [...sessions].sort(
      (a, b) => Date.parse(b.lastActiveAt) - Date.parse(a.lastActiveAt),
    ),
  );
  const search = $derived(query.trim().toLocaleLowerCase());
  const filtering = $derived(Boolean(search) || statusFilter !== "all");
  const visible = $derived(
    list.filter((session) => {
      const statusMatches =
        statusFilter === "all" ||
        (statusFilter === "active"
          ? session.status === "running" || session.status === "idle"
          : session.status === statusFilter);
      if (!statusMatches) return false;
      if (!search) return true;
      const repository = repositories.find(
        (repo) => repo.id === session.workspace?.repositoryId,
      );
      const tree = repository?.worktrees.find(
        (worktree) => worktree.id === session.workspace?.worktreeId,
      );
      return [session.title, session.cwd, repository?.name, tree?.branch].some(
        (value) => value?.toLocaleLowerCase().includes(search),
      );
    }),
  );
  const visibleRepositories = $derived(
    repositories.filter(
      (repo) =>
        !filtering ||
        visible.some((session) => session.workspace?.repositoryId === repo.id),
    ),
  );
  const filters = $derived([
    { id: "all", label: "全部", count: list.length },
    {
      id: "active",
      label: "运行中",
      count: list.filter((s) => s.status === "running" || s.status === "idle")
        .length,
    },
    {
      id: "waiting",
      label: "等待中",
      count: list.filter((s) => s.status === "waiting").length,
    },
    {
      id: "exited",
      label: "已退出",
      count: list.filter((s) => s.status === "exited").length,
    },
  ] as const);
  $effect(() => {
    // A new query opens matching repositories, while still allowing the user
    // to collapse them without changing the unfiltered workspace layout.
    query;
    statusFilter;
    filterExpanded = {};
  });
  function clearFilters() {
    query = "";
    statusFilter = "all";
  }
  const direct = $derived(
    visible.filter(
      (s) =>
        !s.workspace ||
        !repositories.some((r) => r.id === s.workspace?.repositoryId),
    ),
  );
  const directPaths = $derived([...new Set(direct.map((s) => s.cwd))].sort());
  $effect(() => {
    if (!loading)
      for (const repo of repositories) {
        if (!(repo.id in expanded))
          expanded[repo.id] = sessions.some(
            (s) => s.workspace?.repositoryId === repo.id,
          );
      }
  });
  function start(
    mode: "direct" | "existing" | "new" = "direct",
    repositoryId = "",
    worktreeId = "",
  ) {
    creating = { mode, repositoryId, worktreeId };
  }
  function counts(items: SessionInfo[]) {
    return `运行 ${items.filter((s) => s.status === "running" || s.status === "idle").length} · 等待 ${items.filter((s) => s.status === "waiting").length} · 退出 ${items.filter((s) => s.status === "exited").length}`;
  }
  async function refresh() {
    const generation = ++refreshGeneration;
    refreshing = true;
    repositoryError = "";
    try {
      const result = await api("/repositories", RepositoriesSchema);
      if (!disposed && generation === refreshGeneration) repositories = result;
    } catch (err) {
      if (!disposed && generation === refreshGeneration)
        repositoryError = (err as Error).message;
    } finally {
      if (!disposed && generation === refreshGeneration) refreshing = false;
    }
  }
  async function unregister() {
    if (!unregistering) return;
    unregisterBusy = true;
    unregisterError = "";
    try {
      await api(`/repositories/${unregistering.id}`, NoContentSchema, "DELETE");
      unregisterOpen = false;
      await refresh();
    } catch (err) {
      unregisterError = (err as Error).message;
    } finally {
      unregisterBusy = false;
    }
  }
  onMount(() => {
    document.title = "WebMux · 工作区";
    const theme = new MutationObserver(() => {
      dark = document.documentElement.classList.contains("dark");
    });
    theme.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ["class"],
    });
    void Promise.all([
      api("/presets", PresetsSchema),
      api("/config", ConfigSchema),
    ])
      .then(([p, c]) => {
        if (!disposed) {
          presets = p;
          cwd = c.defaultCwd;
          gitReady = c.workspace.gitAvailable;
          registryReady = c.workspace.registryAvailable;
          capabilityError = [c.workspace.gitError, c.workspace.registryError]
            .filter(Boolean)
            .join(" · ");
        }
      })
      .catch((err) => {
        if (!disposed) error = (err as Error).message;
      });
    void refresh();
    const ws = connect(
      "/ws/sessions",
      (data) => {
        if (typeof data !== "string")
          throw new Error("会话列表帧必须是 JSON 文本");
        const event = SessionEventSchema.parse(JSON.parse(data));
        if (event.type === "snapshot") {
          sessions = event.sessions;
          drafts.reconcile(event.sessions.map((session) => session.id));
          loading = false;
        } else if (event.type === "created" || event.type === "updated")
          sessions = [
            ...sessions.filter((s) => s.id !== event.session.id),
            event.session,
          ];
        else if (event.type === "removed") {
          drafts.remove(event.id);
          sessions = sessions.filter((s) => s.id !== event.id);
        }
        now = Date.now();
      },
      (state) => {
        const reconnected =
          state === "connected" && connection === "reconnecting";
        connection = state;
        if (reconnected) void refresh();
      },
      (message) => {
        error = message;
      },
    );
    const visible = () => {
      if (!document.hidden) void refresh();
    };
    document.addEventListener("visibilitychange", visible);
    window.addEventListener("pageshow", visible);
    const clock = setInterval(() => {
      now = Date.now();
    }, 30000);
    return () => {
      disposed = true;
      ++refreshGeneration;
      theme.disconnect();
      ws.close();
      clearInterval(clock);
      document.removeEventListener("visibilitychange", visible);
      window.removeEventListener("pageshow", visible);
    };
  });
</script>

<div class="workspace-page min-h-dvh">
  <header class="border-b bg-card">
    <div
      class="mx-auto flex min-h-20 max-w-6xl flex-wrap items-center justify-between gap-3 px-4 py-3 sm:px-8"
    >
      <a class="brand" href="/"><span class="brand-mark">WM</span> WebMux</a>
      <div
        class="ml-auto flex min-w-0 flex-wrap items-center justify-end gap-1 sm:gap-3"
      >
        <span
          class="hidden items-center gap-2 text-xs text-muted-foreground sm:flex"
          ><span
            class={`status-dot ${connection === "connected" ? "status-running" : "status-waiting"}`}
          ></span>{connection === "connected" ? "本机在线" : "正在连接"}</span
        >
        <Button disabled={!presets.length} onclick={() => start()}
          >＋ 新建会话</Button
        >
        <Button
          href="/settings"
          variant="ghost"
          size="icon"
          aria-label="设置"
          title="设置"><IconSettings aria-hidden="true" /></Button
        >
        <Button
          variant="ghost"
          size="icon"
          class="shrink-0"
          aria-label={dark ? "切换为白色主题" : "切换为黑色主题"}
          title={dark ? "切换为白色主题" : "切换为黑色主题"}
          onclick={toggleTheme}
          >{#if dark}<IconSun class="size-4" />{:else}<IconMoon
              class="size-4"
            />{/if}</Button
        >
      </div>
    </div>
  </header>
  {#if connection === "reconnecting"}<div role="status" class="notice-warning">
      连接已断开，重连中…
    </div>{/if}
  <main class="mx-auto max-w-6xl px-4 py-8 sm:px-8 sm:py-10">
    <div class="mb-6 flex flex-wrap items-end justify-between gap-4">
      <div>
        <p
          class="mb-2 text-xs font-medium tracking-widest text-muted-foreground"
        >
          工作区 / WORKSPACE
        </p>
        <h1
          class="flex flex-wrap items-center gap-3 text-2xl font-semibold tracking-tight sm:text-3xl"
        >
          继续你的工作 <Badge variant="secondary">{list.length} 会话</Badge>
        </h1>
        <p class="mt-2 text-sm text-muted-foreground">
          按仓库组织终端，随时回到正在进行的工作。
        </p>
      </div>
      <div class="flex flex-wrap gap-2">
        <Button
          variant="outline"
          disabled={!gitReady || !registryReady}
          onclick={() => {
            adding = true;
          }}><IconFolderPlus aria-hidden="true" />添加仓库</Button
        >
        <Button variant="ghost" disabled={refreshing} onclick={refresh}
          ><IconRefresh aria-hidden="true" />{refreshing
            ? "正在刷新…"
            : "刷新 Git 状态"}</Button
        >
      </div>
    </div>
    {#if error}<p role="alert" class="mb-4 break-all text-destructive">
        {error}
      </p>{/if}
    {#if capabilityError}<p
        role="status"
        class="mb-4 rounded-lg border p-3 text-sm break-all"
      >
        仓库管理部分不可用：{capabilityError}。仍可直接启动会话。
      </p>{/if}
    {#if repositoryError}<p
        role="alert"
        class="mb-4 break-all text-destructive"
      >
        Git 状态刷新失败：{repositoryError}。保留上次快照与全部会话。
      </p>{/if}
    {#if loading}
      <div class="space-y-4" aria-label="正在加载会话">
        {#each [0, 1, 2] as item (item)}<Skeleton class="h-40 w-full" />{/each}
      </div>
    {:else}
      <div class="mb-7 space-y-3">
        <div class="relative max-w-lg">
          <IconSearch
            class="pointer-events-none absolute top-3.5 left-3.5 size-4 text-muted-foreground"
            aria-hidden="true"
          />
          <Input
            type="search"
            aria-label="搜索会话"
            placeholder="搜索会话、目录或分支…"
            bind:value={query}
            class="workspace-search h-11 rounded-lg bg-card pl-10 text-base md:text-sm"
          />
        </div>
        <div class="flex flex-wrap items-center justify-between gap-3">
          <div
            class="session-filters flex flex-wrap gap-1"
            role="group"
            aria-label="按会话状态筛选"
          >
            {#each filters as filter (filter.id)}
              <Button
                variant="ghost"
                aria-pressed={statusFilter === filter.id}
                onclick={() => (statusFilter = filter.id)}
              >
                {filter.label}
                <span class="filter-count font-mono text-xs tabular-nums"
                  >{filter.count}</span
                >
              </Button>
            {/each}
          </div>
          <p role="status" class="text-xs text-muted-foreground">
            {#if filtering}显示 {visible.length} / {list.length} 个会话
            {:else}{repositories.length} 个仓库 · {list.length} 个会话{/if}
          </p>
        </div>
      </div>
      <div class="space-y-8">
        {#each visibleRepositories as repo (repo.id)}
          {@const repoSessions = visible.filter(
            (s) => s.workspace?.repositoryId === repo.id,
          )}
          {@const missing = repoSessions.filter(
            (s) =>
              !repo.worktrees.some((w) => w.id === s.workspace?.worktreeId),
          )}
          <Collapsible.Root
            class="min-w-0 space-y-4"
            data-repository-id={repo.id}
            role="region"
            aria-label={`仓库 ${repo.name}`}
            open={filtering
              ? (filterExpanded[repo.id] ?? true)
              : (expanded[repo.id] ?? false)}
            onOpenChange={(open) => {
              if (filtering) filterExpanded[repo.id] = open;
              else expanded[repo.id] = open;
            }}
          >
            <div class="flex flex-wrap items-center gap-2">
              <h2 class="w-full min-w-0 flex-none sm:w-auto sm:flex-1">
                <Collapsible.Trigger
                  class="group flex min-w-0 max-w-full items-center gap-2 text-left font-semibold outline-none focus-visible:ring-2 focus-visible:ring-ring"
                >
                  <IconChevronDown
                    class="size-4 shrink-0 -rotate-90 transition-transform group-data-[state=open]:rotate-0"
                  />
                  <span class="min-w-0 break-all">{repo.name}</span>
                </Collapsible.Trigger>
              </h2>
              <Button
                variant="outline"
                size="sm"
                disabled={repo.availability !== "available" || !gitReady}
                onclick={() => start("new", repo.id)}>新建 worktree</Button
              >
              <DropdownMenu.Root>
                <DropdownMenu.Trigger
                  >{#snippet child({ props })}<Button
                      {...props}
                      variant="ghost"
                      size="icon-sm"
                      aria-label={`仓库菜单 ${repo.name}`}>⋯</Button
                    >{/snippet}</DropdownMenu.Trigger
                >
                <DropdownMenu.Content align="end"
                  ><DropdownMenu.Item
                    disabled={!registryReady}
                    onSelect={() => {
                      unregistering = repo;
                      unregisterError = "";
                      unregisterOpen = true;
                    }}>从列表移除仓库</DropdownMenu.Item
                  ></DropdownMenu.Content
                >
              </DropdownMenu.Root>
              <p class="w-full text-xs text-muted-foreground">
                {repo.worktrees.length} 工作区 · {counts(repoSessions)}
              </p>
              <p
                class="w-full font-mono text-xs break-all whitespace-pre-wrap text-muted-foreground"
              >
                {repo.anchorPath}
              </p>
              <p class="w-full text-xs text-muted-foreground">
                {refreshing
                  ? "正在重新读取 Git 状态…"
                  : `Git 读取时间：${new Date(repo.checkedAt).toLocaleString()}`}{repo.availability ===
                "unavailable"
                  ? " · 不可用，状态未知"
                  : ""}
              </p>
              {#if repo.error}<p
                  role="alert"
                  class="w-full text-sm break-all text-destructive"
                >
                  {repo.error}
                </p>{/if}
            </div>
            <Collapsible.Content class="space-y-4">
              {#each repo.worktrees
                .filter((tree) => !filtering || repoSessions.some((session) => session.workspace?.worktreeId === tree.id))
                .sort( (a, b) => (a.kind === "main" ? -1 : b.kind === "main" ? 1 : a.path < b.path ? -1 : a.path > b.path ? 1 : 0) ) as tree (tree.id)}
                <WorkspaceGroup
                  title={tree.branch ||
                    (tree.detached
                      ? `detached · ${tree.head?.slice(0, 12)}`
                      : tree.kind === "bare"
                        ? "Bare 仓库"
                        : "尚无提交")}
                  path={tree.path}
                  worktree={tree}
                  sessions={repoSessions.filter(
                    (s) => s.workspace?.worktreeId === tree.id,
                  )}
                  {now}
                  onclose={(s) => {
                    closing = s;
                  }}
                  onstart={() => start("existing", repo.id, tree.id)}
                  onremove={tree.kind === "linked"
                    ? () => {
                        removing = tree;
                      }
                    : undefined}
                />
              {/each}
              {#if missing.length}<WorkspaceGroup
                  title="已不可用工作区"
                  sessions={missing}
                  {now}
                  onclose={(s) => {
                    closing = s;
                  }}
                />{/if}
              {#if !repo.worktrees.length && !missing.length}<p
                  class="text-sm text-muted-foreground"
                >
                  {repo.error
                    ? "无法读取工作树，请检查路径与 Git 权限，或从列表移除。"
                    : "暂无可用工作树。"}
                </p>{/if}
            </Collapsible.Content>
          </Collapsible.Root>
        {/each}
        {#if direct.length}
          <section class="min-w-0 space-y-4" aria-label="直接会话">
            <div>
              <h2 class="font-semibold">直接会话</h2>
              <p class="mt-1 text-xs text-muted-foreground">
                普通目录或未注册的仓库 · {counts(direct)}
              </p>
            </div>
            {#each directPaths as path (path)}<WorkspaceGroup
                title={path.split("/").pop() || "/"}
                {path}
                sessions={direct.filter((s) => s.cwd === path)}
                {now}
                onclose={(s) => {
                  closing = s;
                }}
              />{/each}
          </section>
        {/if}
        {#if filtering && !visible.length}
          <Empty.Root class="workspace-empty">
            <Empty.Header>
              <Empty.Media variant="icon"><IconSearch /></Empty.Media>
              <Empty.Title>没有匹配的会话</Empty.Title>
              <Empty.Description
                >试试其他名称、目录或分支，或清除筛选查看全部会话。</Empty.Description
              >
            </Empty.Header>
            <Empty.Content>
              <Button variant="outline" onclick={clearFilters}>清除筛选</Button>
            </Empty.Content>
          </Empty.Root>
        {:else if !repositories.length && !list.length}
          <Empty.Root class="workspace-empty">
            <Empty.Header>
              <Empty.Media variant="icon"><IconTerminal2 /></Empty.Media>
              <Empty.Title>还没有会话</Empty.Title>
              <Empty.Description>
                直接启动 Shell 或 agent；也可添加已有仓库，按 worktree
                组织多个独立终端。
              </Empty.Description>
            </Empty.Header>
            <Empty.Content>
              <Button disabled={!presets.length} onclick={() => start()}
                >＋ 新建会话</Button
              >
            </Empty.Content>
          </Empty.Root>
        {/if}
      </div>
    {/if}
    <footer
      class="mt-10 flex flex-wrap justify-between gap-2 border-t pt-5 text-xs text-muted-foreground"
    >
      <span>WebMux · 仓库清单持久保存</span><span
        >仅在可信内网使用 · 服务重启会结束所有会话</span
      >
    </footer>
  </main>
</div>
{#if creating}<NewSessionSheet
    {presets}
    {repositories}
    defaultCwd={cwd}
    initialMode={creating.mode}
    initialRepositoryId={creating.repositoryId}
    initialWorktreeId={creating.worktreeId}
    onchange={refresh}
    onclose={() => {
      creating = null;
    }}
  />{/if}
{#if closing}<CloseSessionDialog
    session={closing}
    onremoved={() => {
      if (closing) drafts.remove(closing.id);
    }}
    onclose={() => {
      closing = null;
    }}
  />{/if}
{#if adding}<AddRepositoryDialog
    defaultCwd={cwd}
    onchange={refresh}
    onclose={() => {
      adding = false;
    }}
  />{/if}
{#if removing}<RemoveWorktreeDialog
    worktree={removing}
    onchange={refresh}
    onclose={() => {
      removing = null;
    }}
  />{/if}
<AlertDialog.Root bind:open={unregisterOpen}>
  <AlertDialog.Content>
    <AlertDialog.Header
      ><AlertDialog.Title>从列表移除仓库？</AlertDialog.Title
      ><AlertDialog.Description class="break-all whitespace-pre-wrap"
        >{unregistering?.anchorPath}<br
        />只移除持久登记，不删除任何目录或分支，不关闭会话。原会话仍可在「直接会话」中访问。</AlertDialog.Description
      ></AlertDialog.Header
    >
    {#if unregisterError}<p
        role="alert"
        class="text-sm break-all text-destructive"
      >
        {unregisterError}
      </p>{/if}
    <AlertDialog.Footer
      ><AlertDialog.Cancel disabled={unregisterBusy}>取消</AlertDialog.Cancel
      ><Button disabled={unregisterBusy} onclick={unregister}
        >{unregisterBusy ? "正在移除…" : "确认移除登记"}</Button
      ></AlertDialog.Footer
    >
  </AlertDialog.Content>
</AlertDialog.Root>
