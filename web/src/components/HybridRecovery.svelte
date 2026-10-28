<script lang="ts">
  import { Button } from "$lib/components/ui/button";
  import { navigate } from "../api";
  import { resumeHybrid } from "../recoverable";
  import type { HybridRecord } from "../schemas";
  import HybridRecordHints from "./HybridRecordHints.svelte";
  import HybridResumeActions from "./HybridResumeActions.svelte";

  // The session page of a closed or interrupted hybrid session: the agent
  // still has the conversation, so it can come back on either side.
  let {
    record,
    onresumed,
  }: {
    record: HybridRecord;
    onresumed: () => void;
  } = $props();
  let busy = $state(false);
  let error = $state("");
</script>

<div
  class="flex shrink-0 flex-col gap-2 border-b bg-muted px-4 py-2 text-sm"
  role="status"
  aria-label="恢复会话"
>
  <p>
    {record.state === "closed"
      ? "该混合会话已关闭"
      : "该混合会话已中断"}，对话仍保存在 {record.agent} 中，可以恢复到聊天或终端。
  </p>
  <HybridRecordHints {record} />
  {#if error}<p class="text-destructive" role="alert">{error}</p>{/if}
  <div class="flex flex-wrap gap-2">
    <HybridResumeActions
      {record}
      resume={resumeHybrid}
      onresumed={() => onresumed()}
      disabled={busy}
      onbusy={(active) => (busy = active)}
      onerror={(message) => (error = message)}
    />
    <Button variant="ghost" size="sm" onclick={() => navigate("/")}
      >返回列表</Button
    >
  </div>
</div>
