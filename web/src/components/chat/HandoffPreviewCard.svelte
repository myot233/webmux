<script lang="ts">
  import { Badge } from "$lib/components/ui/badge";
  import { Button } from "$lib/components/ui/button";
  import { Label } from "$lib/components/ui/label";
  import { Textarea } from "$lib/components/ui/textarea";
  import HandoffHistoryReader from "./HandoffHistoryReader.svelte";
  import InlineNotice from "../shared/InlineNotice.svelte";
  import type { HandoffEditor } from "../../hooks/sessions/handoff.svelte";
  import { openSession } from "../../hooks/shared/shell.svelte";
  let { editor: e }: { editor: HandoffEditor } = $props();
  const state = $derived(
    e.delivery?.status === "unknown"
      ? "unknown"
      : e.delivery?.status === "pending"
        ? "delivering"
        : e.delivery?.status === "failed"
          ? "failed"
          : e.sealed
            ? "delivered"
            : (e.preview?.state ?? "ready"),
  );
  const labels: Record<string, string> = {
    ready: "可发送",
    stale: "来源已变化",
    over_budget: "超出预算",
    delivering: "正在投递",
    delivered: "已投递",
    unknown: "结果待对账",
    failed: "投递失败",
    preparing: "正在准备",
    superseded: "已有新版本",
  };
</script>

{#if e.preview}
  {@const p = e.preview}
  <section
    class="min-w-0 space-y-3 rounded-lg border p-4"
    aria-label="交接历史"
  >
    <div class="flex flex-wrap items-center gap-2">
      <h2 class="text-base font-medium">
        {e.sealed ? "上次交接" : "交接历史"}
      </h2>
      <Badge
        variant={state === "delivered"
          ? "success"
          : ["failed", "over_budget"].includes(state)
            ? "destructive"
            : ["unknown", "stale"].includes(state)
              ? "warning"
              : "outline"}>{labels[state] ?? state}</Badge
      >
      <Badge variant="outline"
        >{p.manual
          ? "手工上下文 · inline"
          : p.strategy === "delta_since_target_last_seen"
            ? "增量历史"
            : "完整来源选择"}</Badge
      >
    </div>
    <dl
      class="grid min-w-0 grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-1 text-xs"
    >
      <dt class="text-muted-foreground">目标线程</dt>
      <dd class="break-all">{p.targetProviderThreadId}</dd>
      <dt class="text-muted-foreground">冻结位置</dt>
      <dd>{p.sourcePosition} · revision {p.revision}</dd>
      <dt class="text-muted-foreground">历史预算</dt>
      <dd>
        {p.budget.used.toLocaleString()} / {p.budget.history.toLocaleString()} · {p
          .budget.source === "provider"
          ? "provider 用量"
          : "保守估算"}
      </dd>
      <dt class="text-muted-foreground">上下文窗口</dt>
      <dd>
        {p.budget.window.toLocaleString()} · 已占用 {p.budget.nativeUsed.toLocaleString()}
        · 工作预留 {p.budget.reserve.toLocaleString()}
      </dd>
      <dt class="text-muted-foreground">覆盖</dt>
      <dd>
        {p.selectedCount} 项已选择 · {p.omittedCount} 项省略
      </dd>
      <dt class="text-muted-foreground">恢复工具</dt>
      <dd>
        {p.historyToolAvailable
          ? "thread_read 可用"
          : "thread_read 不可用，可从历史 API 读取"}
      </dd>
      <dt class="text-muted-foreground">传输</dt>
      <dd>
        {p.nativeAvailable
          ? "原生历史；用户请求单独发送"
          : "inline；用户请求独立保存"}
      </dd>
    </dl>
    {#if p.omittedCount}{#key `${p.id}:${p.revision}`}<HandoffHistoryReader
          handoffId={p.id}
          revision={p.revision}
          sourceId={p.sourceConversationId}
          point={p.sourcePosition}
          omittedCount={p.omittedCount}
        />{/key}{/if}
    {#if p.reconcileRequired || p.historyOrigin !== "complete"}
      <InlineNotice tone="warning"
        >来源有历史缺口，请检查重要约束；未从 provider transcript 猜测补齐。</InlineNotice
      >
    {/if}
    {#if p.stale && !e.sealed && !e.locked}
      <InlineNotice tone="warning"
        >来源或目标配置已变化，编辑仍保留。请重新预览后发送。</InlineNotice
      >
    {/if}
    {#if p.budget.overBudget}
      <InlineNotice tone="danger"
        >超过上下文或单次请求预算。历史与用户任务均已保留，请调整后重新预览；不会截短发送。</InlineNotice
      >
    {/if}
    <div class="space-y-2">
      <Label for="handoff-history">历史正文</Label>
      <Textarea
        id="handoff-history"
        class="max-h-72 min-h-32"
        rows={6}
        value={e.context}
        oninput={(event) => (e.context = event.currentTarget.value)}
        disabled={e.busy ||
          e.locked ||
          e.sealed ||
          (e.unpreviewed && e.conflict)}
      />
      <p class="text-xs text-muted-foreground">
        历史是参考材料。手工修改正文会使用 inline；用户当前任务在下面单独编辑。{e.unpreviewed &&
        e.conflict
          ? "这是另一设备的预览，请先选择读取服务端版本或保留编辑重新预览。"
          : e.saving
            ? "正在保存…"
            : e.dirty
              ? "编辑尚未保存"
              : "已保存到服务端"}
      </p>
    </div>
    {#if e.delivery}
      {@const d = e.delivery}
      {#if d.status === "unknown"}
        <InlineNotice tone="warning"
          >历史：{d.historyStatus}；用户请求：{d.promptStatus}。结果不明，不能盲目重试。请打开目标聊天核对；确认后刷新状态。</InlineNotice
        >
      {:else if d.status === "failed"}
        <InlineNotice tone="danger"
          >在 {d.step} 阶段失败：{d.errorClass}。历史：{d.historyStatus}；用户请求：{d.promptStatus}。草稿仍保留。</InlineNotice
        >
      {:else if d.status === "pending"}
        <InlineNotice
          >正在{d.step === "worktree"
            ? "准备工作树"
            : d.step === "prompt"
              ? "发送用户请求"
              : "准备目标会话"}。刷新或重启后可沿用原命令继续准备。</InlineNotice
        >
      {:else}
        <InlineNotice tone="success"
          >交接已完成。历史与用户请求分别记账。</InlineNotice
        >
      {/if}
      <div class="flex flex-wrap gap-2">
        {#if d.sessionId}<Button
            variant="outline"
            onclick={() => openSession({ id: d.sessionId })}
            >打开目标聊天</Button
          >{/if}
        {#if d.replacementHandoffId}<Button
            variant="outline"
            disabled={e.busy}
            onclick={() => void e.replacement()}>检查新的原生聊天预览</Button
          >{/if}
        {#if d.status === "unknown"}<Button
            variant="outline"
            onclick={() => void e.refresh()}>刷新对账状态</Button
          >{/if}
      </div>
    {/if}
  </section>
{/if}
