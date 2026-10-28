<script lang="ts">
  import { Button } from "$lib/components/ui/button";
  import type { UploadProgress } from "../hooks/upload.svelte";

  // The upload strip shown by both editors while a batch is running.
  let {
    progress,
    cancel,
    variant = "strip",
  }: {
    progress: UploadProgress;
    cancel: () => void;
    variant?: "strip" | "card";
  } = $props();
</script>

<div
  class={variant === "card"
    ? "space-y-1 border-t px-2 py-1.5"
    : "shrink-0 space-y-1 border-b bg-muted px-3 py-2"}
  aria-label="文件上传状态"
>
  <div class="flex items-center gap-3 text-xs" role="status">
    <span class="min-w-0 flex-1 truncate"
      >{progress.waiting
        ? "文件已上传，等待终端恢复："
        : "正在上传 "}{progress.name}{progress.total > 1
        ? `（${progress.index}/${progress.total}）`
        : ""}</span
    >
    <span>{progress.percent}%</span>
    <Button variant="ghost" size="sm" onclick={cancel}>取消</Button>
  </div>
  <progress
    aria-label="文件上传进度"
    max={100}
    value={progress.percent}
    class="block h-1 w-full accent-primary"
  ></progress>
</div>
