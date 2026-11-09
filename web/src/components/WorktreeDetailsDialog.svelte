<script lang="ts">
  import { onDestroy } from "svelte";
  import IconCopy from "~icons/tabler/copy";
  import { Button } from "$lib/components/ui/button";
  import { Badge } from "$lib/components/ui/badge";
  import { copyText } from "../clipboard";
  import { formatBytes } from "../code-image";
  import { errorMessage } from "../error";
  import { LatestRequest } from "../hooks/remote-request.svelte";
  import { measureWorktree } from "../recoverable";
  import type { SessionInfo, Worktree, WorktreeUsage } from "../schemas";
  import { countSessions } from "../session-status";
  import InlineNotice from "./InlineNotice.svelte";
  import ResponsiveDialog from "./ResponsiveDialog.svelte";

  // What a group of the home page stands for: its path, the worktree's Git
  // state, the session totals and, for a worktree, a disk size that is only
  // walked when asked for.
  let {
    title,
    path,
    worktree,
    sessions,
    onclose,
  }: {
    title: string;
    path?: string;
    worktree?: Worktree;
    sessions: readonly SessionInfo[];
    onclose: () => void;
  } = $props();
  const counts = $derived(countSessions(sessions));
  const kindLabel = $derived(
    worktree?.kind === "main"
      ? "主工作区"
      : worktree?.kind === "bare"
        ? "bare 仓库"
        : "关联工作树",
  );

  let usage = $state<WorktreeUsage | null>(null);
  let measuring = $state(false);
  let usageError = $state("");
  const requests = new LatestRequest();
  onDestroy(() => requests.dispose());
  async function measure(target: Worktree) {
    const request = requests.begin();
    measuring = true;
    usageError = "";
    try {
      const result = await measureWorktree(target, request.signal);
      if (request.current()) usage = result;
    } catch (err) {
      if (request.reportable()) usageError = errorMessage(err);
    } finally {
      if (request.current()) measuring = false;
    }
  }
</script>

<ResponsiveDialog
  {title}
  description={worktree ? "工作树路径、Git 状态与占用空间。" : "目录与会话。"}
  {onclose}
>
  <div class="space-y-3">
    <dl class="grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 gap-y-2 text-xs">
      {#if path}
        <dt class="text-muted-foreground">路径</dt>
        <dd class="flex min-w-0 items-start gap-1">
          <span class="min-w-0 flex-1 font-mono break-all">{path}</span>
          <Button
            size="icon-xs"
            variant="ghost"
            aria-label="复制路径"
            title="复制路径"
            onclick={() => void copyText(path)}
            ><IconCopy aria-hidden="true" /></Button
          >
        </dd>
      {/if}
      {#if worktree}
        <dt class="text-muted-foreground">类型</dt>
        <dd class="flex flex-wrap gap-1.5">
          <Badge variant="secondary">{kindLabel}</Badge>
          {#if worktree.available && worktree.dirty === false}<Badge
              variant="secondary">已跟踪文件干净</Badge
            >{/if}
        </dd>
        <dt class="text-muted-foreground">分支</dt>
        <dd class="font-mono break-all">
          {worktree.branch ?? "（分离 HEAD）"}{worktree.head
            ? ` · ${worktree.head.slice(0, 8)}`
            : ""}
        </dd>
      {/if}
      <dt class="text-muted-foreground">会话</dt>
      <dd class="flex flex-wrap gap-x-3 gap-y-1">
        <span>共 {sessions.length} 个会话</span>
        <span
          >运行 <span class="inline-block w-[3ch] text-right tabular-nums"
            >{counts.active}</span
          ></span
        >
        <span
          >等待 <span class="inline-block w-[3ch] text-right tabular-nums"
            >{counts.waiting}</span
          ></span
        >
        <span
          >退出 <span class="inline-block w-[3ch] text-right tabular-nums"
            >{counts.exited}</span
          ></span
        >
      </dd>
      {#if worktree}
        <dt class="text-muted-foreground">容量</dt>
        <dd class="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1">
          {#if measuring}
            <span role="status">正在计算…</span>
          {:else if usage}
            <span role="status" class="tabular-nums"
              >{formatBytes(usage.bytes)} · {usage.entries.toLocaleString()} 个条目{usage.complete
                ? ""
                : "（未统计完整，实际更大）"}</span
            >
          {/if}
          <Button
            size="xs"
            variant="outline"
            disabled={measuring || !worktree.available}
            onclick={() => void measure(worktree)}
            >{usage ? "重新计算" : "计算容量"}</Button
          >
        </dd>
      {/if}
    </dl>
    {#if worktree && !worktree.available}
      <p class="text-xs text-muted-foreground">
        工作树目录缺失或不可验证，无法计算容量。
      </p>
    {/if}
    {#if usageError}<InlineNotice tone="danger">{usageError}</InlineNotice>{/if}
  </div>
</ResponsiveDialog>
