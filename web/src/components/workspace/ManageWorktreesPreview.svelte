<script lang="ts">
  import { worktreeTitle } from "../../domains/workspace/home-worktrees";
  import type { RemovalPreviewItem } from "../../contracts/schemas";
  import type { BatchPreviewGroups } from "../../domains/workspace/worktree-batch";
  import { Badge } from "$lib/components/ui/badge";
  import { Checkbox } from "$lib/components/ui/checkbox";
  import { Label } from "$lib/components/ui/label";
  import InlineNotice from "../shared/InlineNotice.svelte";
  import PushCheckNotice from "../settings/PushCheckNotice.svelte";
  import WorktreeSessionList from "./WorktreeSessionList.svelte";

  let {
    groups,
    includeForce = $bindable(false),
    busy = false,
  }: {
    groups: BatchPreviewGroups;
    includeForce: boolean;
    busy?: boolean;
  } = $props();

  function keepsRecords(item: RemovalPreviewItem, forced: boolean): boolean {
    if (item.commit === "") return false;
    if (item.archived.length > 0) return true;
    return forced && item.sessions.some((s) => s.agent !== null);
  }
</script>

<div class="space-y-4">
  <div class="rounded-lg border bg-muted/40 p-3 text-sm">
    <div class="flex flex-wrap items-center gap-2 font-medium">
      <span>可安全清理 {groups.safe.length} 个</span>
      {#if groups.blocked.length > 0}
        <span class="text-muted-foreground">·</span>
        <span>有阻塞 {groups.blocked.length} 个</span>
      {/if}
      {#if groups.unsupported.length > 0}
        <span class="text-muted-foreground">·</span>
        <span>不可删除（将跳过） {groups.unsupported.length} 个</span>
      {/if}
    </div>
    <p class="mt-1.5 text-xs text-muted-foreground">
      本地分支始终保留；可解析提交的已归档混合会话记录将转为“已释放空间”保留。确认
      5 分钟内有效，期间目标或会话变化将单列失败而不会波及其余工作树。
    </p>
  </div>

  <div class="max-h-[48dvh] space-y-4 overflow-y-auto pr-1">
    {#if groups.safe.length > 0}
      <section class="space-y-2" aria-label="可安全清理项">
        <h3 class="text-sm font-medium">
          可安全清理（{groups.safe.length}）
        </h3>
        <ul class="space-y-2">
          {#each groups.safe as item (item.worktree.id)}
            {@const retained = item.commit !== "" && item.archived.length > 0}
            <li class="space-y-1.5 rounded-lg border p-3 text-sm">
              <div class="flex flex-wrap items-center gap-1.5">
                <span class="font-medium break-all"
                  >{worktreeTitle(item.worktree)}</span
                >
                {#if item.worktree.prunable && !item.worktree.available}
                  <Badge variant="outline">清理失效登记</Badge>
                {/if}
                {#if item.archived.length > 0}
                  <Badge variant="secondary"
                    >{retained
                      ? `保留 ${item.archived.length} 条归档聊天（转为已释放空间）`
                      : `${item.archived.length} 条归档聊天原样保留`}</Badge
                  >
                {/if}
              </div>
              <p class="font-mono text-xs break-all text-muted-foreground">
                {item.worktree.path}
              </p>
              {#if item.worktree.detached && !retained}
                <p class="text-xs text-destructive">
                  detached 提交可能没有分支保护，请确认需要的提交已保留。
                </p>
              {/if}
              {#if item.warnings.length > 0}
                <InlineNotice tone="warning">
                  <ul class="space-y-1">
                    {#each item.warnings as warning (warning)}
                      <li>{warning}</li>
                    {/each}
                  </ul>
                </InlineNotice>
              {/if}
              {#if keepsRecords(item, false)}
                <PushCheckNotice push={item.push} />
              {/if}
            </li>
          {/each}
        </ul>
      </section>
    {/if}

    {#if groups.blocked.length > 0}
      <section class="space-y-2.5" aria-label="受阻工作树">
        <div class="flex flex-wrap items-baseline justify-between gap-2">
          <h3 class="text-sm font-medium">
            有阻塞的工作树（{groups.blocked.length}）
          </h3>
          <span class="text-xs text-muted-foreground">
            {includeForce ? "已勾选一并强制删除" : "默认跳过，不改动文件与会话"}
          </span>
        </div>
        <ul class="space-y-2">
          {#each groups.blocked as item (item.worktree.id)}
            {@const activeSessions = item.sessions.filter(
              (s) => s.status !== "exited",
            )}
            <li
              class="space-y-1.5 rounded-lg border border-destructive/30 p-3 text-sm"
            >
              <div class="flex flex-wrap items-center gap-1.5">
                <span class="font-medium break-all"
                  >{worktreeTitle(item.worktree)}</span
                >
                {#each item.blockers as blocker (blocker)}
                  <Badge variant="warning">{blocker}</Badge>
                {/each}
              </div>
              <p class="font-mono text-xs break-all text-muted-foreground">
                {item.worktree.path}
              </p>
              {#if activeSessions.length > 0}
                <WorktreeSessionList
                  title="活动会话（强制删除时将先停止）"
                  sessions={activeSessions}
                />
              {/if}
              {#if item.warnings.length > 0}
                <p class="text-xs text-muted-foreground">
                  {item.warnings.join("；")}
                </p>
              {/if}
              {#if includeForce && keepsRecords(item, true)}
                <PushCheckNotice push={item.push} />
              {/if}
            </li>
          {/each}
        </ul>
        <div class="rounded-lg border border-destructive/40 bg-muted/30 p-3">
          <Label class="items-start">
            <Checkbox
              bind:checked={includeForce}
              disabled={busy}
              class="mt-0.5"
            />
            <span class="text-sm leading-snug">
              同时强制删除以上 {groups.blocked.length} 个受阻工作树：关闭列出的全部活动会话，并永久丢弃这些目录中的未提交修改、未跟踪文件与忽略文件。
            </span>
          </Label>
        </div>
      </section>
    {/if}

    {#if groups.unsupported.length > 0}
      <section class="space-y-2" aria-label="不可删除项">
        <h3 class="text-sm font-medium text-muted-foreground">
          不可删除，将自动跳过（{groups.unsupported.length}）
        </h3>
        <ul class="space-y-2">
          {#each groups.unsupported as item (item.worktree.id)}
            <li class="space-y-1 rounded-lg border bg-muted/30 p-3 text-sm">
              <p class="font-medium break-all">
                {worktreeTitle(item.worktree)}
              </p>
              <p class="font-mono text-xs break-all text-muted-foreground">
                {item.worktree.path}
              </p>
              <p class="text-xs break-all text-destructive">
                {item.blockers.join("；")}
              </p>
            </li>
          {/each}
        </ul>
      </section>
    {/if}
  </div>
</div>
