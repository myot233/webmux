<script lang="ts">
  import { onMount } from "svelte";
  import { Button } from "$lib/components/ui/button";
  import { errorMessage } from "../../platform/error";
  import {
    previewRebuild,
    rebuildWorktree,
  } from "../../domains/sessions/recoverable";
  import type { HybridRelease, RebuildPreview } from "../../contracts/schemas";
  import BlockerList from "../shared/BlockerList.svelte";
  import KeyValueSummary from "../shared/KeyValueSummary.svelte";
  import PreviewForm from "./PreviewForm.svelte";
  import ResponsiveDialog from "../shared/ResponsiveDialog.svelte";

  // The second confirmation before a released record resumes: its worktree
  // comes back at the original path, on its branch when that still points at
  // the saved commit, otherwise detached on the commit. Cancelling changes
  // nothing; the caller resumes once `onrebuilt` runs.
  let {
    id,
    title,
    released,
    onclose,
    onrebuilt,
  }: {
    /** The released record. */
    id: string;
    /** The record's title, naming what resumes. */
    title: string;
    released: HybridRelease;
    onclose: () => void;
    /** The worktree is back; runs instead of `onclose`. */
    onrebuilt: () => void;
  } = $props();
  let preview = $state<RebuildPreview | null>(null);
  let busy = $state(false);
  let error = $state("");

  const commit = $derived((preview?.commit ?? released.commit).slice(0, 12));
  const branch = $derived(preview?.branch ?? released.branch ?? "");
  const reasons = {
    branchMissing: "已被删除",
    branchMoved: "已有新的提交",
    branchCheckedOut: "已在其他工作树检出",
  } as const;
  const mode = $derived.by(() => {
    if (!preview) return "";
    if (preview.mode === "reuse")
      return "原路径已是保存提交上的工作树，将直接使用，不重新创建。";
    if (preview.mode === "branch")
      return `将在原路径检出分支 ${branch}，它仍指向保存的提交。`;
    const why = preview.reason
      ? `分支 ${branch} ${reasons[preview.reason]}，`
      : "释放时没有检出分支，";
    return `${why}将以 detached 方式检出保存的提交 ${commit}${preview.reason ? "，分支保持不变" : ""}。`;
  });

  async function load() {
    busy = true;
    error = "";
    preview = null;
    try {
      preview = await previewRebuild(id);
    } catch (err) {
      error = errorMessage(err);
    } finally {
      busy = false;
    }
  }
  onMount(() => {
    void load();
  });

  async function rebuild() {
    if (!preview || preview.blockers.length) return;
    busy = true;
    error = "";
    try {
      await rebuildWorktree(id, preview);
      busy = false;
      onrebuilt();
    } catch (err) {
      error = `${errorMessage(err)}。记录仍为已释放空间，请重新读取预览后再确认。`;
      preview = null;
      busy = false;
    }
  }
</script>

<ResponsiveDialog
  title="重建工作树后恢复"
  description={`“${title}”所在的工作树已释放空间。恢复前需要在原路径重建工作树；释放时删除的忽略文件（依赖、构建产物等）不会回来。`}
  drawerDescription="恢复前需要在原路径重建工作树；已删除的忽略文件不会回来。"
  {onclose}
  {busy}
>
  <PreviewForm {busy} {error} onsubmit={() => void rebuild()} {onclose}>
    {#snippet summary()}
      <KeyValueSummary
        rows={[
          {
            label: "原路径",
            value: preview?.path ?? released.path,
            mono: true,
          },
          { label: "保存提交", value: commit, mono: true, wrap: false },
        ]}
      />
    {/snippet}
    {#if preview}
      {#if preview.blockers.length}
        <BlockerList title="无法重建" blockers={preview.blockers} />
      {:else}<p class="text-sm">{mode}</p>{/if}
    {/if}
    {#snippet actions()}
      {#if preview}<Button
          type="submit"
          disabled={busy || preview.blockers.length > 0}>重建并恢复</Button
        >{:else}<Button type="button" disabled={busy} onclick={load}
          >重新读取预览</Button
        >{/if}
    {/snippet}
  </PreviewForm>
</ResponsiveDialog>
