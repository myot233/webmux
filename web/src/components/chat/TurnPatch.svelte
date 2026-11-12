<script lang="ts">
  import type { SelectedLineRange } from "@pierre/diffs";
  import { calculateDiff } from "../../domains/code/code-diff-client";
  import type { DiffResult } from "../../domains/code/code-diff-model";
  import { errorMessage } from "../../platform/error";
  import type { ChatContent } from "../../contracts/schemas";
  import CodeDiffReader from "../code/CodeDiffReader.svelte";

  let { patch }: { patch: ChatContent } = $props();
  let result = $state.raw<DiffResult>();
  let error = $state("");
  const defaultSide = $derived<"old" | "new">(
    patch.oldText && !patch.newText ? "old" : "new",
  );
  let side = $state<"old" | "new">("new");

  function selectDiff(range: SelectedLineRange | null) {
    if (range?.endSide && range.endSide !== range.side) return;
    side = range ? (range.side === "deletions" ? "old" : "new") : defaultSide;
  }

  $effect(() => {
    const { path = "", oldText = "", newText = "" } = patch;
    const controller = new AbortController();
    result = undefined;
    error = "";
    side = defaultSide;
    void calculateDiff(path, oldText, newText, controller.signal).then(
      (diff) => {
        if (!controller.signal.aborted) result = diff;
      },
      (err: unknown) => {
        if (!controller.signal.aborted) error = errorMessage(err);
      },
    );
    return () => controller.abort();
  });
</script>

{#if patch.truncated}<p class="text-xs">Agent 报告的改动内容已截断。</p>{/if}
{#if result?.kind === "ready"}
  <div
    class="flex h-64 min-w-0 flex-col"
    role="region"
    aria-label={`回合差异：${patch.path}`}
  >
    <CodeDiffReader
      fileDiff={{
        ...result.fileDiff,
        name: patch.path ?? result.fileDiff.name,
      }}
      {side}
      expired={false}
      draftEnabled={false}
      onselect={selectDiff}
    />
  </div>
{:else}
  <p class="text-xs" role="status">
    {error ||
      (result?.kind === "limited"
        ? result.reason.replace("，请查看两侧完整文件", "")
        : "正在计算差异…")}
  </p>
{/if}
