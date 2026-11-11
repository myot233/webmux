<script lang="ts">
  let {
    refreshing,
    startBranch,
    baseSource,
    upstream,
    baseSubject,
    lastRefreshedAt,
    refreshWarning,
  }: {
    refreshing: boolean;
    startBranch: string;
    baseSource: "local" | "remote" | "";
    upstream: string;
    baseSubject: string;
    lastRefreshedAt: Date | null;
    refreshWarning: string;
  } = $props();
</script>

{#if refreshing}
  <p
    role="status"
    aria-label="上游刷新状态"
    class="text-xs text-muted-foreground"
  >
    正在获取 {startBranch} 对应的上游最新提交；可继续编辑或创建。
  </p>
{:else if baseSource === "remote" && upstream}
  <p
    role="status"
    aria-label="上游刷新状态"
    class="break-all text-xs text-muted-foreground"
  >
    远程最新 · {upstream} · {baseSubject || "无标题提交"}{#if lastRefreshedAt}
      · 最近刷新 {lastRefreshedAt.toLocaleTimeString()}{/if}
  </p>
{:else if baseSource === "local" && startBranch}
  <p
    role="status"
    aria-label="上游刷新状态"
    class="break-all text-xs text-muted-foreground"
  >
    本地：{startBranch}
  </p>
{/if}
{#if refreshWarning}<p
    role="status"
    aria-label="上游刷新警告"
    class="text-xs text-warning"
  >
    {refreshWarning}
  </p>{/if}
