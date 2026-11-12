<script lang="ts">
  import { onMount, untrack } from "svelte";
  import { SvelteMap } from "svelte/reactivity";
  import IconCheck from "~icons/tabler/check";
  import IconCopy from "~icons/tabler/copy";
  import { Badge } from "$lib/components/ui/badge";
  import { Button } from "$lib/components/ui/button";
  import { Checkbox } from "$lib/components/ui/checkbox";
  import { Label } from "$lib/components/ui/label";
  import { APIError } from "../../platform/api";
  import { restoreCommand } from "../../domains/workspace/branch-actions";
  import {
    branchOperations,
    type BranchFlowProps,
  } from "../../domains/workspace/branch-operations";
  import { copyText } from "../../platform/clipboard";
  import { PreviewFlow } from "../../hooks/workspace/preview-flow.svelte";
  import {
    MAX_BRANCH_DELETE,
    type BranchDeleteItem,
    type BranchDeletePreview,
    type BranchDeleteResult,
  } from "../../contracts/schemas";
  import BranchCommitList from "./BranchCommitList.svelte";
  import FlowActions from "../shared/FlowActions.svelte";
  import FlowStatus from "../shared/FlowStatus.svelte";
  import InlineNotice from "../shared/InlineNotice.svelte";

  // Deletes local branches after a preview that sorts them into "已包含"
  // (every commit stays reachable; preselected) and "有独有提交" (commits only
  // the branch keeps; each needs its own tick). Checked-out or vanished
  // branches cannot be deleted. Results list each branch, with the command
  // that recreates a deleted one.
  let {
    repositoryId,
    branches,
    busy = $bindable(false),
    source = branchOperations,
    onchange,
    onclose,
  }: BranchFlowProps & { branches: readonly string[] } = $props();

  const asked = untrack(() => branches.slice(0, MAX_BRANCH_DELETE));
  const left = untrack(() => branches.length - asked.length);
  const flow = new PreviewFlow<BranchDeletePreview>((signal) =>
    source.deletePreview(repositoryId, asked, signal),
  );
  onMount(() => {
    void flow.refresh();
    return () => flow.dispose();
  });

  const items = $derived(flow.preview?.items ?? []);
  const blocked = $derived(items.filter((item) => item.blockers.length > 0));
  const contained = $derived(
    items.filter(
      (item) => item.blockers.length === 0 && item.group === "contained",
    ),
  );
  const unmerged = $derived(
    items.filter(
      (item) => item.blockers.length === 0 && item.group === "unmerged",
    ),
  );
  // The user's ticks by branch and commit: a branch that moved is shown as
  // new, with its group's default.
  const choices = new SvelteMap<string, boolean>();
  const choiceKey = (item: BranchDeleteItem) => `${item.branch}@${item.commit}`;
  const chosen = (item: BranchDeleteItem) =>
    choices.get(choiceKey(item)) ?? item.group === "contained";
  const picked = $derived(
    [...contained, ...unmerged].filter((item) => chosen(item)),
  );
  const ready = $derived(
    !!flow.preview?.confirmationId &&
      picked.length > 0 &&
      !flow.loading &&
      !flow.running,
  );

  let result = $state<BranchDeleteResult | null>(null);
  let copied = $state("");

  // The server deletes all of a preview's contained branches and, when
  // acknowledged, all of its unmerged ones. Another selection is previewed
  // on its own first and must match what was shown.
  async function confirm() {
    const shown = flow.preview;
    if (!shown || !ready) return;
    const selection = picked;
    const withUnmerged = selection.some((item) => item.group === "unmerged");
    const pickedUnmerged = selection.length - contained.length;
    const whole =
      contained.every((item) => chosen(item)) &&
      (pickedUnmerged === 0 || pickedUnmerged === unmerged.length);
    busy = true;
    const done = await flow.run(async () => {
      let confirmationId = shown.confirmationId;
      if (!whole) {
        const own = await source.deletePreview(
          repositoryId,
          selection.map((item) => item.branch),
        );
        const same =
          own.confirmationId !== "" &&
          own.items.length === selection.length &&
          own.items.every((item, index) => {
            const expected = selection[index];
            return (
              item.blockers.length === 0 &&
              item.branch === expected?.branch &&
              item.commit === expected.commit &&
              item.group === expected.group
            );
          });
        if (!same) throw new APIError(409, "所选分支已变化");
        confirmationId = own.confirmationId;
      }
      return source.deleteBranches(repositoryId, confirmationId, withUnmerged);
    });
    busy = false;
    onchange();
    if (done) result = done;
  }

  async function copyRestore(branch: string, commit: string) {
    if (await copyText(restoreCommand(branch, commit))) copied = branch;
  }
  const statusLabels = {
    deleted: { text: "已删除", variant: "success" },
    skipped: { text: "已跳过", variant: "warning" },
    failed: { text: "失败", variant: "destructive" },
  } as const;
</script>

{#snippet row(item: BranchDeleteItem, label: string)}
  <Label class="w-full items-start">
    <Checkbox
      checked={chosen(item)}
      onCheckedChange={(value) => choices.set(choiceKey(item), value === true)}
      disabled={busy}
      aria-label={label}
      class="mt-0.5"
    />
    <span class="min-w-0 flex-1 space-x-1.5 text-sm">
      <span class="font-mono font-medium break-all">{item.branch}</span>
      <span class="font-mono text-xs text-muted-foreground"
        >{item.commit.slice(0, 8)}</span
      >
    </span>
  </Label>
{/snippet}

<div class="min-w-0 space-y-3">
  {#if result}
    <ul class="space-y-2" aria-label="删除结果">
      {#each result.results as entry (entry.branch)}
        {@const status = statusLabels[entry.status]}
        <li class="min-w-0 space-y-1 rounded-lg border p-3 text-xs">
          <div class="flex min-w-0 flex-wrap items-center gap-1.5">
            <span class="font-mono text-sm font-medium break-all"
              >{entry.branch}</span
            >
            <Badge variant={status.variant}>{status.text}</Badge>
          </div>
          {#if entry.status === "deleted"}
            <div class="flex min-w-0 items-start gap-1">
              <code class="min-w-0 flex-1 font-mono break-all"
                >{restoreCommand(entry.branch, entry.commit)}</code
              >
              <Button
                size="icon-xs"
                variant="ghost"
                aria-label={`复制恢复命令：${entry.branch}`}
                title={copied === entry.branch ? "已复制" : "复制恢复命令"}
                onclick={() => void copyRestore(entry.branch, entry.commit)}
                >{#if copied === entry.branch}<IconCheck
                    aria-hidden="true"
                  />{:else}<IconCopy aria-hidden="true" />{/if}</Button
              >
            </div>
          {:else if entry.message}
            <p class="break-all text-muted-foreground">{entry.message}</p>
          {/if}
        </li>
      {/each}
    </ul>
    <p class="text-xs text-muted-foreground">
      没有其他引用的提交会保留到 Git
      垃圾回收清除它们为止（默认约两周），在此之前可用恢复命令找回分支。
    </p>
  {:else}
    <FlowStatus {flow} />
    {#if left > 0}
      <InlineNotice
        >单次最多删除 {MAX_BRANCH_DELETE} 个分支；其余 {left} 个请在本次完成后再次清理。</InlineNotice
      >
    {/if}
    {#if contained.length}
      <section class="space-y-2" aria-label="已包含">
        <h3 class="text-sm font-medium">已包含（{contained.length}）</h3>
        <p class="text-xs text-muted-foreground">
          这些分支的提交都能从其他分支、远程跟踪分支、标签或工作树到达，删除不会丢失提交。
        </p>
        <ul class="space-y-2">
          {#each contained as item (item.branch)}
            <li class="rounded-lg border p-3">
              {@render row(item, `删除 ${item.branch}`)}
            </li>
          {/each}
        </ul>
      </section>
    {/if}
    {#if unmerged.length}
      <section class="space-y-2" aria-label="有独有提交">
        <h3 class="text-sm font-medium">有独有提交（{unmerged.length}）</h3>
        <p class="text-xs text-muted-foreground">
          删除后这些提交将不再被任何分支引用。默认不删除，需要逐个勾选确认。
        </p>
        <ul class="space-y-2">
          {#each unmerged as item (item.branch)}
            <li class="space-y-2 rounded-lg border border-destructive/30 p-3">
              {@render row(
                item,
                `删除 ${item.branch} 及其 ${item.unmerged.total} 个独有提交`,
              )}
              <BranchCommitList title="独有提交" commits={item.unmerged} />
            </li>
          {/each}
        </ul>
      </section>
    {/if}
    {#if blocked.length}
      <section class="space-y-2" aria-label="不能删除">
        <h3 class="text-sm font-medium text-muted-foreground">
          不能删除（{blocked.length}）
        </h3>
        <ul class="space-y-2">
          {#each blocked as item (item.branch)}
            <li class="space-y-1 rounded-lg border bg-muted/30 p-3 text-xs">
              <p class="font-mono text-sm font-medium break-all">
                {item.branch}
              </p>
              {#each item.blockers as blocker (blocker.code + blocker.message)}
                <p class="break-all text-destructive">
                  {blocker.message}{#if blocker.path}<span class="font-mono">
                      （{blocker.path}）</span
                    >{/if}
                </p>
              {/each}
            </li>
          {/each}
        </ul>
      </section>
    {/if}
  {/if}
  <FlowActions
    done={!!result}
    {busy}
    {ready}
    label={busy ? "正在删除…" : `删除 ${picked.length} 个分支`}
    destructive
    onconfirm={() => void confirm()}
    {onclose}
  />
</div>
