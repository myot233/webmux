<script lang="ts">
  import { onMount } from "svelte";
  import { Button } from "$lib/components/ui/button";
  import { errorMessage } from "../../platform/error";
  import {
    previewRelease,
    releaseWorktree,
  } from "../../domains/sessions/recoverable";
  import type { ReleasePreview, Worktree } from "../../contracts/schemas";
  import BlockerList from "../shared/BlockerList.svelte";
  import InlineNotice from "../shared/InlineNotice.svelte";
  import KeyValueSummary from "../shared/KeyValueSummary.svelte";
  import PreviewForm from "./PreviewForm.svelte";
  import PushCheckNotice from "../settings/PushCheckNotice.svelte";
  import ResponsiveDialog from "../shared/ResponsiveDialog.svelte";

  // Frees the disk space of a linked worktree whose sessions are archived:
  // the directory and its Git registration go, the branch, the archived
  // records and the commit stay so a resume can rebuild it later. The
  // preview lists what would block it and the archived records that stay;
  // ignored files are deleted with the directory and only warned about.
  let {
    worktree,
    onclose,
    onchange,
  }: {
    worktree: Worktree;
    onclose: () => void;
    /** The worktree or its records may have changed. */
    onchange: () => void;
  } = $props();
  let preview = $state<ReleasePreview | null>(null);
  let busy = $state(false);
  let error = $state("");

  const commit = $derived(
    (preview?.commit || worktree.head || "").slice(0, 12),
  );
  const hasIgnored = $derived(
    !!preview && (preview.ignored.length > 0 || preview.ignoredMore > 0),
  );

  async function load() {
    busy = true;
    error = "";
    preview = null;
    try {
      preview = await previewRelease(worktree);
    } catch (err) {
      error = errorMessage(err);
    } finally {
      busy = false;
    }
  }
  onMount(() => {
    void load();
  });

  async function release() {
    if (!preview || preview.blockers.length) return;
    busy = true;
    error = "";
    try {
      await releaseWorktree(worktree, preview.confirmationId);
      onclose();
    } catch (err) {
      error = `${errorMessage(err)}。工作树与记录保持原样，请重新读取预览后再确认。`;
      preview = null;
    } finally {
      busy = false;
      onchange();
    }
  }
</script>

<ResponsiveDialog
  title="释放空间"
  description="删除工作树目录与 Git 登记以回收磁盘。分支与已归档记录保留，之后恢复聊天时可在原路径重建工作树。"
  drawerDescription="删除工作树目录以回收磁盘；分支与已归档记录保留，可在恢复时重建。"
  {onclose}
  {busy}
>
  <PreviewForm {busy} {error} onsubmit={() => void release()} {onclose}>
    {#snippet summary()}
      <KeyValueSummary
        rows={[
          { label: "路径", value: worktree.path, mono: true },
          { label: "分支", value: worktree.branch || "detached（无分支）" },
          {
            label: "保存提交",
            value: commit || "无法解析",
            mono: true,
            wrap: false,
          },
        ]}
      />
    {/snippet}
    {#if preview}
      {#if preview.blockers.length}
        <BlockerList title="无法释放空间" blockers={preview.blockers} />
      {/if}
      {#if preview.archived.length}
        <div class="space-y-2 text-sm">
          <p class="font-medium">保留的已归档会话（之后可重建并恢复）</p>
          <ul class="space-y-1">
            {#each preview.archived as record (record.id)}<li class="break-all">
                {record.title} · {record.cwd}
              </li>{/each}
          </ul>
        </div>
      {/if}
      {#if preview.archived.length && preview.commit}<PushCheckNotice
          push={preview.push}
        />{/if}
      {#if hasIgnored}
        <InlineNotice tone="warning">
          忽略文件（例如依赖、构建产物、.env）会随目录永久删除，重建工作树也无法找回，需要重新生成。
        </InlineNotice>
      {/if}
      <p class="text-xs text-muted-foreground">
        确认 5 分钟内有效；目标或会话变化后必须重新预览。
      </p>
    {/if}
    {#snippet actions()}
      {#if preview}<Button
          type="submit"
          variant="destructive"
          disabled={busy || preview.blockers.length > 0}>释放空间</Button
        >{:else}<Button type="button" disabled={busy} onclick={load}
          >重新读取预览</Button
        >{/if}
    {/snippet}
  </PreviewForm>
</ResponsiveDialog>
