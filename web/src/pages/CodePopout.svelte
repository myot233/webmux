<script lang="ts">
  import { Button } from "$lib/components/ui/button";
  import CodeWorkspace from "../components/code/CodeWorkspace.svelte";
  import { useCodePopoutFrame } from "../hooks/code/code-popout-frame.svelte";

  let { id }: { id: string } = $props();
  const frame = useCodePopoutFrame();
  let workspace = $state<CodeWorkspace>();
  $effect(() => {
    const current = workspace;
    if (current) frame.ready(() => current.capture());
  });
</script>

<main class="flex h-dvh min-h-0 flex-col">
  {#if frame.phase === "ready"}
    <CodeWorkspace
      bind:this={workspace}
      {id}
      visible
      host={frame.host}
      initial={frame.initial}
      popout={{ popped: true, toggle: () => frame.close() }}
    />
  {:else if frame.phase === "standalone"}
    <div class="space-y-2 p-6 text-sm">
      <p>独立代码窗口需从会话页面打开。</p>
      <Button href={`/s/${id}`} variant="outline" size="sm">返回会话</Button>
    </div>
  {:else}
    <p role="status" class="p-6 text-sm text-muted-foreground">
      正在连接会话页面…
    </p>
  {/if}
</main>
