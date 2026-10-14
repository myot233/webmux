<script lang="ts">
  import { onMount } from "svelte";
  import RiSunLine from "remixicon-svelte/icons/sun-line";
  import RiMoonLine from "remixicon-svelte/icons/moon-line";
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
  import { Skeleton } from "$lib/components/ui/skeleton";
  import * as DropdownMenu from "$lib/components/ui/dropdown-menu";
  import * as AlertDialog from "$lib/components/ui/alert-dialog";
  import NewSessionSheet from "../components/NewSessionSheet.svelte";
  import CloseSessionDialog from "../components/CloseSessionDialog.svelte";
  import WorkspaceGroup from "../components/WorkspaceGroup.svelte";
  import AddRepositoryDialog from "../components/AddRepositoryDialog.svelte";
  import RemoveWorktreeDialog from "../components/RemoveWorktreeDialog.svelte";

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
  let refreshGeneration = 0;
  let disposed = false;
  const list = $derived(
    [...sessions].sort(
      (a, b) => Date.parse(b.lastActiveAt) - Date.parse(a.lastActiveAt),
    ),
  );
  const direct = $derived(
    list.filter(
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
          loading = false;
        } else if (event.type === "created" || event.type === "updated")
          sessions = [
            ...sessions.filter((s) => s.id !== event.session.id),
            event.session,
          ];
        else if (event.type === "removed")
          sessions = sessions.filter((s) => s.id !== event.id);
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

<div class="min-h-dvh">
  <header class="border-b bg-card/60">
    <div
      class="mx-auto flex min-h-20 max-w-6xl flex-wrap items-center justify-between gap-3 px-4 py-3 sm:px-8"
    >
      <a class="brand" href="/"><span class="brand-mark">WM</span> WebMux</a>
      <div class="ml-auto flex items-center gap-2 sm:gap-4">
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
          variant="outline"
          size="sm"
          class="shrink-0"
          aria-label={dark ? "切换为白色主题" : "切换为黑色主题"}
          onclick={toggleTheme}
          >{#if dark}<RiSunLine class="size-4" />白色{:else}<RiMoonLine
              class="size-4"
            />黑色{/if}</Button
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
        <h1
          class="flex items-center gap-3 text-2xl font-semibold tracking-tight sm:text-3xl"
        >
          继续你的工作 <Badge variant="secondary">{list.length} 会话</Badge>
        </h1>
        <p class="mt-2 text-sm text-muted-foreground">
          按仓库组织工作，或直接开启终端。连接可以断开，工作不会停下。
        </p>
      </div>
      <div class="flex gap-2">
        <Button
          variant="outline"
          disabled={!gitReady || !registryReady}
          onclick={() => {
            adding = true;
          }}>添加仓库</Button
        >
        <Button variant="ghost" disabled={refreshing} onclick={refresh}
          >{refreshing ? "正在刷新…" : "刷新 Git 状态"}</Button
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
      <div
        class="grid gap-4 md:grid-cols-2 lg:grid-cols-3"
        aria-label="正在加载会话"
      >
        {#each [0, 1, 2] as item (item)}<Skeleton
            class="h-56 w-full rounded-xl"
          />{/each}
      </div>
    {:else}
      <div class="space-y-5">
        {#each repositories as repo (repo.id)}
          {@const repoSessions = list.filter(
            (s) => s.workspace?.repositoryId === repo.id,
          )}
          {@const missing = repoSessions.filter(
            (s) =>
              !repo.worktrees.some((w) => w.id === s.workspace?.worktreeId),
          )}
          <section
            class="min-w-0 rounded-xl border bg-card/50"
            data-repository-id={repo.id}
            aria-label={`仓库 ${repo.name}`}
          >
            <div class="flex flex-wrap items-center gap-2 p-4 sm:p-5">
              <button
                type="button"
                class="flex w-full min-w-0 flex-none items-start gap-2 text-left focus-visible:outline-2 focus-visible:outline-ring sm:w-auto sm:flex-1"
                aria-expanded={expanded[repo.id] ?? false}
                onclick={() => {
                  expanded[repo.id] = !expanded[repo.id];
                }}
              >
                <span
                  class="mt-1 shrink-0 text-muted-foreground"
                  aria-hidden="true">{expanded[repo.id] ? "▾" : "▸"}</span
                >
                <span class="min-w-0"
                  ><span class="block font-semibold break-all">{repo.name}</span
                  ><span class="mt-1 block text-xs text-muted-foreground"
                    >{repo.worktrees.length} 工作区 · {counts(
                      repoSessions,
                    )}</span
                  ></span
                >
              </button>
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
              <p
                class="w-full pl-5 font-mono text-xs break-all whitespace-pre-wrap text-muted-foreground"
              >
                {repo.anchorPath}
              </p>
              <p class="w-full pl-5 text-xs text-muted-foreground">
                {refreshing
                  ? "正在重新读取 Git 状态…"
                  : `Git 读取时间：${new Date(repo.checkedAt).toLocaleString()}`}{repo.availability ===
                "unavailable"
                  ? " · 不可用，状态未知"
                  : ""}
              </p>
              {#if repo.error}<p
                  role="alert"
                  class="w-full pl-5 text-sm break-all text-destructive"
                >
                  {repo.error}
                </p>{/if}
            </div>
            <div
              hidden={!expanded[repo.id]}
              class="space-y-3 border-t p-3 sm:p-4"
            >
              {#each [...repo.worktrees].sort( (a, b) => (a.kind === "main" ? -1 : b.kind === "main" ? 1 : a.path < b.path ? -1 : a.path > b.path ? 1 : 0) ) as tree (tree.id)}
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
            </div>
          </section>
        {/each}
        {#if direct.length}
          <section class="min-w-0 space-y-3" aria-label="直接会话">
            <div class="pt-3">
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
        {#if !repositories.length && !list.length}
          <section
            class="flex min-h-64 flex-col items-center justify-center rounded-xl border border-dashed bg-card/40 p-6 text-center"
          >
            <h2 class="text-xl font-medium">还没有会话</h2>
            <p class="mt-2 mb-6 max-w-md text-sm text-muted-foreground">
              直接启动 Shell 或 agent；也可添加已有仓库，按 worktree
              组织多个独立终端。
            </p>
            <Button disabled={!presets.length} onclick={() => start()}
              >＋ 新建会话</Button
            >
          </section>
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
