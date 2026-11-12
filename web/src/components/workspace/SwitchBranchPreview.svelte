<script lang="ts">
  import IconArrowRight from "~icons/tabler/arrow-right";
  import { Checkbox } from "$lib/components/ui/checkbox";
  import { Label } from "$lib/components/ui/label";
  import type { SwitchPreview, Worktree } from "../../contracts/schemas";
  import BlockerList from "../shared/BlockerList.svelte";
  import InlineNotice from "../shared/InlineNotice.svelte";
  import WorktreeSessionList from "./WorktreeSessionList.svelte";

  let {
    preview,
    previewing = false,
    previewError = "",
    conflict = "",
    worktree,
    acknowledged = $bindable(false),
    busy = false,
  }: {
    preview: SwitchPreview | null;
    previewing?: boolean;
    previewError?: string;
    conflict?: string;
    worktree: Worktree;
    acknowledged?: boolean;
    busy?: boolean;
  } = $props();

  const short = (hash: string | null | undefined) => hash?.slice(0, 8) ?? "";
  const currentLabel = $derived.by(() => {
    const tree = preview?.worktree ?? worktree;
    return tree.branch ?? `分离 · ${short(tree.head) || "无提交"}`;
  });
  const sessionsTitle = $derived(
    preview?.warnings.find((item) => item.code === "sessions")?.message ??
      "运行中的会话",
  );
  const otherWarnings = $derived(
    (preview?.warnings ?? []).filter((item) => item.code !== "sessions"),
  );
  const agentWorking = $derived(
    (preview?.sessions ?? []).some(
      (item) => item.agent !== null && item.status === "running",
    ),
  );
</script>

{#if previewing}
  <p role="status" class="text-sm text-muted-foreground">正在预览切换…</p>
{:else if previewError}
  <p role="alert" class="text-sm break-all text-destructive">
    无法预览：{previewError}
  </p>
{/if}
{#if conflict}<InlineNotice tone="warning"
    >{conflict}；已按当前状态重新预览。</InlineNotice
  >{/if}
{#if preview}
  <section aria-label="切换预览" class="space-y-3">
    <div class="space-y-1 rounded-lg border p-3 text-sm">
      <p
        class="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1 font-medium"
      >
        <span class="break-all">{currentLabel}</span>
        <IconArrowRight
          class="size-4 shrink-0 text-muted-foreground"
          aria-hidden="true"
        /><span class="sr-only">切换到</span>
        <span class="break-all"
          >{preview.target.branch ??
            `分离 · ${short(preview.target.commit)}`}</span
        >
      </p>
      <p class="text-xs break-all text-muted-foreground">
        <span class="font-mono">{short(preview.target.commit)}</span>
        · {preview.target.subject || "无标题提交"}
      </p>
      {#if preview.target.createsBranch}<p
          class="text-xs break-all text-muted-foreground"
        >
          将新建分支 {preview.target.branch}{preview.target.upstream
            ? `，上游为 ${preview.target.upstream}`
            : "，不设置上游"}。
        </p>{/if}
    </div>
    {#if preview.blockers.length}
      <BlockerList
        title="无法切换"
        blockers={preview.blockers.map((item) => item.message)}
      />
    {/if}
    {#if otherWarnings.length}
      <InlineNotice tone="warning">
        <ul class="space-y-1">
          {#each otherWarnings as item (item.code)}<li class="break-all">
              {item.message}
            </li>{/each}
        </ul>
      </InlineNotice>
    {/if}
    {#if preview.sessions.length}
      <WorktreeSessionList title={sessionsTitle} sessions={preview.sessions} />
      {#if agentWorking}<InlineNotice tone="warning"
          >有 agent 正在工作，可能正在写文件；切换后它的写入会落在新的分支上。</InlineNotice
        >{/if}
      {#if !preview.blockers.length}
        <Label class="items-start">
          <Checkbox
            bind:checked={acknowledged}
            disabled={busy}
            class="mt-0.5"
          />
          <span class="text-sm leading-snug"
            >我已了解：这些会话不会被关闭或重启，会直接看到切换后的文件。</span
          >
        </Label>
      {/if}
    {/if}
    {#if !preview.blockers.length}<p class="text-xs text-muted-foreground">
        不会贮藏或丢弃任何修改；确认 5 分钟内有效，状态变化后需要重新预览。
      </p>{/if}
  </section>
{/if}
