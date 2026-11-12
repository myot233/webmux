<script lang="ts">
  import InlineNotice from "../shared/InlineNotice.svelte";
  import { Button } from "$lib/components/ui/button";

  let {
    setupError,
    setupBusy,
    onretrysetup,
    error,
    rebuildFailure,
    recordError,
    capabilityError,
    repositoryError,
    hasRepositories,
  }: {
    setupError: string;
    setupBusy: boolean;
    onretrysetup: () => void;
    error: string;
    rebuildFailure: string;
    recordError: string;
    capabilityError: string;
    repositoryError: string;
    hasRepositories: boolean;
  } = $props();
</script>

{#if setupError}<InlineNotice tone="warning" class="mb-4">
    <div class="flex flex-wrap items-center justify-between gap-2">
      <span>{setupError}</span>
      <Button
        variant="ghost"
        size="sm"
        class="min-h-9 sm:min-h-8"
        disabled={setupBusy}
        onclick={onretrysetup}
      >
        {setupBusy ? "正在重试…" : "重试"}
      </Button>
    </div>
  </InlineNotice>{/if}
{#if error}<InlineNotice tone="danger" class="mb-4">
    {error}
  </InlineNotice>{/if}
{#if rebuildFailure}<InlineNotice tone="danger" class="mb-4">
    {rebuildFailure}
  </InlineNotice>{/if}
{#if recordError}<InlineNotice tone="danger" class="mb-4">
    {recordError}
  </InlineNotice>{/if}
{#if capabilityError}<InlineNotice tone="warning" class="mb-4">
    仓库管理部分不可用：{capabilityError}。仍可直接启动会话。
  </InlineNotice>{/if}
{#if repositoryError}<InlineNotice tone="danger" class="mb-4">
    {hasRepositories
      ? `Git 状态刷新失败：${repositoryError}。保留上次快照与全部会话。`
      : `Git 状态读取失败：${repositoryError}。会话仍可正常使用。`}
  </InlineNotice>{/if}
