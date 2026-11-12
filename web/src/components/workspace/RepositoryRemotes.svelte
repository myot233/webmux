<script lang="ts">
  import IconCopy from "~icons/tabler/copy";
  import IconWorldOff from "~icons/tabler/world-off";
  import { Button } from "$lib/components/ui/button";
  import * as Empty from "$lib/components/ui/empty";
  import { copyText } from "../../platform/clipboard";
  import type { Remote } from "../../contracts/schemas";
  import InlineNotice from "../shared/InlineNotice.svelte";

  // The repository panel's remotes. The server redacts credentials, so the
  // URLs shown and copied never hold a password or token.
  let {
    remotes,
    error,
    onretry,
  }: {
    /** Undefined while the first read runs. */
    remotes: readonly Remote[] | undefined;
    error: string;
    onretry: () => void;
  } = $props();
</script>

{#snippet url(label: string, name: string, value: string)}
  <dt class="text-muted-foreground">{label}</dt>
  <dd class="flex min-w-0 items-start gap-1">
    <span class="min-w-0 flex-1 font-mono break-all">{value}</span>
    <Button
      size="icon-xs"
      variant="ghost"
      aria-label={`复制 ${name} 的${label}地址`}
      title={`复制 ${name} 的${label}地址`}
      onclick={() => void copyText(value)}
      ><IconCopy aria-hidden="true" /></Button
    >
  </dd>
{/snippet}

{#if error}
  <div class="space-y-2">
    <InlineNotice tone="danger">无法读取远程：{error}</InlineNotice>
    <Button size="sm" variant="outline" onclick={onretry}>重新读取</Button>
  </div>
{:else if !remotes}
  <p role="status" class="text-xs text-muted-foreground">正在读取远程…</p>
{:else if remotes.length === 0}
  <Empty.Root variant="outline">
    <Empty.Header>
      <Empty.Media variant="icon"
        ><IconWorldOff aria-hidden="true" /></Empty.Media
      >
      <Empty.Title>没有远程</Empty.Title>
      <Empty.Description
        >这个仓库还没有配置远程；可以在终端用 git remote add 添加。</Empty.Description
      >
    </Empty.Header>
  </Empty.Root>
{:else}
  <ul class="space-y-2" aria-label="远程列表">
    {#each remotes as remote (remote.name)}
      <li class="space-y-2 rounded-lg border p-3">
        <p class="text-sm font-medium break-all">{remote.name}</p>
        <dl class="grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 gap-y-2 text-xs">
          {#if remote.fetchUrl === remote.pushUrl}
            {@render url("获取与推送", remote.name, remote.fetchUrl)}
          {:else}
            {@render url("获取", remote.name, remote.fetchUrl)}
            {@render url("推送", remote.name, remote.pushUrl)}
          {/if}
        </dl>
      </li>
    {/each}
  </ul>
{/if}
