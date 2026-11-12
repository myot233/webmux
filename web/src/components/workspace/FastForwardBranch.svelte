<script lang="ts">
  import { onMount } from "svelte";
  import { Checkbox } from "$lib/components/ui/checkbox";
  import { Label } from "$lib/components/ui/label";
  import {
    branchOperations,
    type BranchFlowProps,
  } from "../../domains/workspace/branch-operations";
  import { PreviewFlow } from "../../hooks/workspace/preview-flow.svelte";
  import type { FastForwardPreview } from "../../contracts/schemas";
  import BranchCommitList from "./BranchCommitList.svelte";
  import FlowActions from "../shared/FlowActions.svelte";
  import FlowStatus from "../shared/FlowStatus.svelte";
  import InlineNotice from "../shared/InlineNotice.svelte";
  import PreviewIssues from "./PreviewIssues.svelte";
  import WorktreeSessionList from "./WorktreeSessionList.svelte";

  // Fast-forwards a local branch to its upstream's tip, from the
  // remote-tracking refs at hand: in the worktree that has it checked out,
  // or by moving only the branch ref. Running sessions there keep running
  // and must be acknowledged. Held by a dialog or a repository panel
  // subview, which `busy` keeps open while the request runs.
  let {
    repositoryId,
    branch,
    busy = $bindable(false),
    source = branchOperations,
    onchange,
    onclose,
  }: BranchFlowProps & { branch: string } = $props();

  const flow = new PreviewFlow<FastForwardPreview>((signal) =>
    source.fastForwardPreview(repositoryId, branch, signal),
  );
  onMount(() => {
    void flow.refresh();
    return () => flow.dispose();
  });
  const preview = $derived(flow.preview);
  // The acknowledgement covers the sessions of the preview it was given
  // for; a new preview asks again.
  let acknowledgedFor = $state.raw<FastForwardPreview | null>(null);
  const acknowledged = $derived(!!preview && acknowledgedFor === preview);
  let done = $state<FastForwardPreview | null>(null);
  const sessions = $derived(preview?.sessions ?? []);
  const ready = $derived(
    !!preview?.confirmationId &&
      !flow.loading &&
      !flow.running &&
      (sessions.length === 0 || acknowledged),
  );

  async function confirm() {
    const target = preview;
    if (!target || !ready) return;
    busy = true;
    const result = await flow.run(() =>
      source.fastForward(
        repositoryId,
        target.confirmationId,
        sessions.length > 0 && acknowledged,
      ),
    );
    busy = false;
    onchange();
    if (result) done = target;
  }
  const short = (hash: string) => hash.slice(0, 8);
</script>

<div class="min-w-0 space-y-3">
  {#if done}
    <InlineNotice tone="success"
      >已把 <span class="font-mono break-all">{done.branch}</span> 快进到
      <span class="font-mono break-all">{done.upstream}</span>（{short(
        done.to,
      )}）。</InlineNotice
    >
  {:else}
    <FlowStatus {flow} />
    {#if preview}
      <dl class="grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 gap-y-2 text-xs">
        <dt class="text-muted-foreground">分支</dt>
        <dd class="font-mono break-all">{preview.branch}</dd>
        {#if preview.upstream}
          <dt class="text-muted-foreground">上游</dt>
          <dd class="font-mono break-all">{preview.upstream}</dd>
        {/if}
        {#if preview.to}
          <dt class="text-muted-foreground">提交</dt>
          <dd class="font-mono">{short(preview.from)} → {short(preview.to)}</dd>
        {/if}
        <dt class="text-muted-foreground">工作树</dt>
        <dd class="break-all">
          {#if preview.worktree}<span class="font-mono"
              >{preview.worktree.path}</span
            >{:else}未被检出：只移动分支引用，任何工作树的文件都不变{/if}
        </dd>
      </dl>
      <PreviewIssues blockers={preview.blockers} warnings={preview.warnings} />
      {#if preview.commits.total > 0}
        <BranchCommitList title="将前进的提交" commits={preview.commits} />
      {/if}
      {#if sessions.length}
        <div class="space-y-2 rounded-lg border p-3">
          <WorktreeSessionList
            title="运行中的会话（不会关闭或重启）"
            {sessions}
          />
          <Label class="items-start">
            <Checkbox
              checked={acknowledged}
              onCheckedChange={(value) =>
                (acknowledgedFor = value ? preview : null)}
              disabled={busy}
              class="mt-0.5"
            />
            <span class="text-sm leading-snug"
              >我已知晓：以上 {sessions.length} 个会话会看到快进后的文件。</span
            >
          </Label>
        </div>
      {/if}
    {/if}
  {/if}
  <FlowActions
    done={!!done}
    {busy}
    {ready}
    label={busy ? "正在快进…" : "快进到上游"}
    onconfirm={() => void confirm()}
    {onclose}
  />
</div>
