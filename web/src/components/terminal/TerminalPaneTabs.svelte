<script lang="ts">
  import type { SessionInfo } from "../../contracts/schemas";
  import { Button } from "$lib/components/ui/button";

  let {
    ids,
    info,
    active,
    zoomed,
    onfocus,
    onzoom,
  }: {
    ids: readonly string[];
    info: Record<string, SessionInfo>;
    active: string;
    zoomed: boolean;
    onfocus: (sessionId: string) => void;
    onzoom: () => void;
  } = $props();
</script>

<nav class="pane-tabs" aria-label="终端窗格">
  {#each ids as sessionId, index (sessionId)}
    <Button
      variant="ghost"
      size="sm"
      aria-label={`切换到窗格 ${index + 1}：${info[sessionId]?.title || sessionId}`}
      aria-current={active === sessionId ? "true" : undefined}
      onclick={() => onfocus(sessionId)}
    >
      <span class="truncate"
        >{index + 1} · {info[sessionId]?.title || sessionId}</span
      >
    </Button>
  {/each}
  <Button variant="ghost" size="sm" aria-pressed={zoomed} onclick={onzoom}
    >{zoomed ? "恢复分屏" : "放大窗格"}</Button
  >
</nav>

<style>
  .pane-tabs {
    display: flex;
    flex-shrink: 0;
    gap: 2px;
    padding: 3px 6px;
    overflow-x: auto;
    border-bottom: 1px solid var(--border);
    background: var(--card);
  }
  .pane-tabs :global(button) {
    flex-shrink: 0;
    max-inline-size: 12rem;
  }
</style>
