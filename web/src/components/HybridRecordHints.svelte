<script lang="ts">
  import type { HybridRecord } from "../schemas";

  // What an interruption may have cost a closed or interrupted hybrid
  // session: an unfinished turn and the prompts its queue lost.
  let { record }: { record: HybridRecord } = $props();
</script>

{#if record.state === "interrupted" && (record.activity === "turn" || record.activity === "switching")}
  <p class="text-xs text-warning">
    {record.activity === "turn"
      ? "中断时有回合在运行，上一轮可能未完成。"
      : "中断时正在切换，上一轮可能未完成。"}
  </p>
{/if}
{#if record.queuedCount}
  <p class="text-xs text-muted-foreground">
    中断时有 {record.queuedCount} 条排队消息，未发送的内容已丢失。
  </p>
{/if}
