<script lang="ts">
  import IconCopy from "~icons/tabler/copy";
  import { Button } from "$lib/components/ui/button";
  import { copyText } from "../../platform/clipboard";
  import { placeArchived } from "../../domains/sessions/archived";
  import { homeLayout } from "../../domains/workspace/home-worktrees";
  import type {
    ArchivedSession,
    HomeWorktrees,
    Repository,
    SessionInfo,
  } from "../../contracts/schemas";
  import { statusSummary } from "../../domains/sessions/session-status";
  import { fetchedText } from "../../domains/workspace/worktree-sync";
  import InlineNotice from "../shared/InlineNotice.svelte";

  // The repository panel's overview: where the repository lives, its
  // default branch, how many worktrees the home page lists, the remotes, the
  // last fetch, the sessions and when Git was read. Removing the
  // registration starts here.
  let {
    repository,
    sessions,
    records,
    home,
    remoteCount,
    registryReady,
    onunregister,
  }: {
    repository: Repository;
    sessions: readonly SessionInfo[];
    records: readonly ArchivedSession[];
    home: HomeWorktrees;
    /** Undefined until the remotes are read, or when they cannot be. */
    remoteCount: number | undefined;
    registryReady: boolean;
    onunregister: () => void;
  } = $props();
  const available = $derived(repository.availability === "available");
  const own = $derived(
    sessions.filter((s) => s.workspace?.repositoryId === repository.id),
  );
  const shown = $derived(
    homeLayout(
      home,
      repository,
      own,
      placeArchived(
        records.filter((record) => !sessions.some((s) => s.id === record.id)),
        [repository],
      ).repositories.get(repository.id)?.worktrees,
    ).shown.length,
  );
  const main = $derived(repository.worktrees.find((w) => w.kind === "main"));
  const defaultBranch = $derived(
    main
      ? (main.branch ??
          (main.head ? `（分离 HEAD）${main.head.slice(0, 8)}` : "尚无提交"))
      : "—",
  );
</script>

{#snippet copyable(value: string, label: string)}
  <dd class="flex min-w-0 items-start gap-1">
    <span class="min-w-0 flex-1 font-mono break-all">{value}</span>
    <Button
      size="icon-xs"
      variant="ghost"
      aria-label={label}
      title={label}
      onclick={() => void copyText(value)}
      ><IconCopy aria-hidden="true" /></Button
    >
  </dd>
{/snippet}

<div class="space-y-3">
  {#if !available}
    <InlineNotice tone="danger"
      >仓库不可用：{repository.error ||
        "路径缺失或无法读取"}。工作树、分支与远程暂不可用，可以从列表移除这个仓库。</InlineNotice
    >
  {/if}
  <dl class="grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 gap-y-2 text-xs">
    <dt class="text-muted-foreground">路径</dt>
    {@render copyable(repository.anchorPath, "复制仓库路径")}
    {#if repository.commonDir}
      <dt class="text-muted-foreground">公共目录</dt>
      {@render copyable(repository.commonDir, "复制 Git 公共目录")}
    {/if}
    {#if available}
      <dt class="text-muted-foreground">默认分支</dt>
      <dd class="font-mono break-all">{defaultBranch}</dd>
      <dt class="text-muted-foreground">工作区</dt>
      <dd>主页 {shown} / 共 {repository.worktrees.length}</dd>
      <dt class="text-muted-foreground">远程</dt>
      <dd>{remoteCount === undefined ? "—" : `${remoteCount} 个`}</dd>
      <dt class="text-muted-foreground">上次获取</dt>
      <dd
        title={repository.lastFetchedAt
          ? new Date(repository.lastFetchedAt).toLocaleString()
          : undefined}
      >
        {fetchedText(repository.lastFetchedAt)}（近似）
      </dd>
    {/if}
    <dt class="text-muted-foreground">会话</dt>
    <dd>{statusSummary(own)}</dd>
    <dt class="text-muted-foreground">Git</dt>
    <dd>
      读取于 {new Date(repository.checkedAt).toLocaleString()}{available
        ? ""
        : " · 不可用，状态未知"}
    </dd>
  </dl>
  <div class="border-t pt-3">
    <Button
      size="sm"
      variant="destructive"
      disabled={!registryReady}
      onclick={onunregister}>从列表移除仓库…</Button
    >
  </div>
</div>
