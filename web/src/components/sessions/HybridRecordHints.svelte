<script lang="ts">
  import type { ArchivedSession } from "../../contracts/schemas";

  // What an interruption may have cost a closed or interrupted hybrid
  // session: an unfinished turn and the prompts its queue lost.
  let { record }: { record: ArchivedSession } = $props();
</script>

{#if record.state === "interrupted" && (record.archive.activity === "turn" || record.archive.activity === "switching")}
  <p class="text-xs text-warning">
    {record.archive.activity === "turn"
      ? "中断时有回合在运行，上一轮可能未完成。"
      : "中断时正在切换，上一轮可能未完成。"}
  </p>
{/if}
{#if record.archive.queuedCount}
  <p class="text-xs text-muted-foreground">
    中断时有 {record.archive.queuedCount} 条排队消息，未发送的内容已丢失。
  </p>
{/if}
