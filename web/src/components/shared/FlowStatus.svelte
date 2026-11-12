<script lang="ts">
  import { Button } from "$lib/components/ui/button";
  import type { PreviewFlow } from "../../hooks/workspace/preview-flow.svelte";
  import InlineNotice from "./InlineNotice.svelte";

  // What a branch flow's preview is doing: reading, failed to read, or read
  // again after an execution failed or found the state changed.
  let {
    flow,
  }: {
    flow: PreviewFlow<unknown>;
  } = $props();
</script>

{#if flow.stale}
  <InlineNotice tone="warning"
    >{flow.stale}。已重新预览，请核对后再次确认。</InlineNotice
  >
{:else if flow.failure}
  <InlineNotice tone="danger"
    >操作失败：{flow.failure}。已按当前状态重新预览。</InlineNotice
  >
{/if}
{#if flow.error}
  <InlineNotice tone="danger">无法读取预览：{flow.error}</InlineNotice>
  <Button
    size="sm"
    variant="outline"
    disabled={flow.loading}
    onclick={() => void flow.refresh()}>重新预览</Button
  >
{:else if flow.loading && !flow.preview}
  <p role="status" class="text-xs text-muted-foreground">正在读取预览…</p>
{/if}
