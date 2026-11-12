<script lang="ts">
  import { Button } from "$lib/components/ui/button";
  import { Textarea } from "$lib/components/ui/textarea";
  import InlineNotice from "../shared/InlineNotice.svelte";
  import { threadAPI } from "../../platform/thread-api";
  import { errorMessage } from "../../platform/error";
  import type { ConversationActivitySchema } from "../../contracts/schemas";
  import type { z } from "zod";
  let {
    handoffId,
    revision,
    sourceId,
    point,
    omittedCount,
  }: {
    handoffId: string;
    revision: number;
    sourceId: string;
    point: string;
    omittedCount: number;
  } = $props();
  let open = $state(false);
  let item = $state<z.infer<typeof ConversationActivitySchema> | null>(null);
  let offset = $state("0");
  let loading = $state(false);
  let error = $state("");
  let request: AbortController | null = null;
  let disposed = false;
  const itemIds: string[] = [];
  let selectionPosition = "0";
  let selectionDone = false;
  const index = $derived(
    item ? Math.max(0, itemIds.indexOf(item.activityId)) : 0,
  );
  async function load(next: number, start = "0") {
    if (next < 0 || next >= omittedCount) return;
    request?.abort();
    const active = new AbortController();
    request = active;
    loading = true;
    error = "";
    try {
      while (next >= itemIds.length && !selectionDone) {
        const page = await threadAPI.selection(
          handoffId,
          selectionPosition,
          active.signal,
        );
        if (disposed || request !== active) return;
        if (
          page.sourceConversationId !== sourceId ||
          page.sourcePosition !== point ||
          page.revision !== revision
        )
          throw new Error("预览已变化，请刷新预览后重试");
        if (
          !page.done &&
          BigInt(page.nextPosition) <= BigInt(selectionPosition)
        )
          throw new Error("历史分页游标没有前进");
        for (const entry of page.items) {
          if (!entry.selected) itemIds.push(entry.activityId);
        }
        selectionPosition = page.nextPosition;
        selectionDone = page.done;
      }
      const id = itemIds[next];
      if (!id) throw new Error("省略历史与预览不一致，请重新打开");
      const result = await threadAPI.activity(
        sourceId,
        id,
        start,
        active.signal,
      );
      if (
        result.conversationId !== sourceId ||
        BigInt(result.position) > BigInt(point)
      )
        throw new Error("项目不属于冻结来源");
      if (!disposed && request === active) {
        item = result;
        offset = start;
      }
    } catch (cause) {
      if (!disposed && request === active && !active.signal.aborted)
        error = errorMessage(cause);
    } finally {
      if (!disposed && request === active) loading = false;
    }
  }
  $effect(() => () => {
    disposed = true;
    request?.abort();
  });
</script>

<div class="space-y-2">
  <Button
    variant="outline"
    size="sm"
    aria-expanded={open}
    onclick={() => {
      open = !open;
      if (open && !item) void load(0);
    }}>查看省略历史</Button
  >
  {#if open}
    <div class="min-w-0 space-y-2" aria-label="省略历史读取">
      <p class="text-xs text-muted-foreground">
        历史仅供参考，不会自动拼接到当前任务；不读取附件正文或私有推理。
      </p>
      {#if error}<InlineNotice tone="danger">{error}</InlineNotice>{/if}
      {#if loading}<p role="status" class="text-sm">正在读取已保存历史…</p>{/if}
      {#if item}
        <p class="break-all text-xs text-muted-foreground">
          第 {index + 1} / {omittedCount} 项 · {item.role} · Run {item.runOrdinal}
          · position {item.position} · 字节 offset {offset}<br
          />{item.activityId}
        </p>
        {#if item.exportable}<Textarea
            readonly
            value={item.text}
            rows={5}
            class="max-h-64"
            aria-label="省略历史片段"
          />{:else}<InlineNotice
            >这项内容不能导出：{item.exclusionReason}</InlineNotice
          >{/if}
        {#if item.attachments.length}<p class="text-xs text-muted-foreground">
            附件引用：{item.attachments
              .map((attachment) => attachment.name)
              .join("、")}
          </p>{/if}
        <div class="flex flex-wrap gap-2">
          <Button
            variant="outline"
            size="sm"
            disabled={loading || index === 0}
            onclick={() => void load(index - 1)}>上一项</Button
          >
          <Button
            variant="outline"
            size="sm"
            disabled={loading || index >= omittedCount - 1}
            onclick={() => void load(index + 1)}>下一项</Button
          >
          {#if item.truncated}<Button
              variant="outline"
              size="sm"
              disabled={loading}
              onclick={() => void load(index, item?.nextTextOffset)}
              >下一片段</Button
            >{/if}
          {#if offset !== "0"}<Button
              variant="outline"
              size="sm"
              disabled={loading}
              onclick={() => void load(index)}>从头读取</Button
            >{/if}
        </div>
      {/if}
    </div>
  {/if}
</div>
