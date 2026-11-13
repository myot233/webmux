<script lang="ts">
  import { SvelteMap } from "svelte/reactivity";
  import { ListButton } from "$lib/components/ui/list-button";
  import type { ChatContent } from "../../contracts/schemas";
  import { diffStats } from "../../domains/chat/chat-model";
  import DiffCount from "./DiffCount.svelte";
  import ChatDisclosure from "./ChatDisclosure.svelte";
  import TurnPatch from "./TurnPatch.svelte";

  let { changes }: { changes: readonly ChatContent[] } = $props();
  let selectedPath = $state<string>();
  const byFile = $derived.by(() => {
    const files = new SvelteMap<string, ChatContent[]>();
    for (const change of changes) {
      const path = change.path!;
      const patches = files.get(path) ?? [];
      patches.push(change);
      files.set(path, patches);
    }
    return files;
  });
  const files = $derived(
    [...byFile].map(([path, patches]) => ({
      path,
      patches,
      hasLineStats: patches.some(
        (patch) => patch.oldText !== undefined || patch.newText !== undefined,
      ),
      stats: patches.reduce(
        (total, patch) => {
          const stats = diffStats(patch);
          return {
            added: total.added + stats.added,
            removed: total.removed + stats.removed,
          };
        },
        { added: 0, removed: 0 },
      ),
    })),
  );
</script>

<ChatDisclosure>
  {#snippet summary()}本轮改动 · {byFile.size} 个文件{/snippet}
  <p class="text-xs">根据 Agent 在本轮报告的工具改动。</p>
  <ul>
    {#each files as { path, patches, hasLineStats, stats } (path)}
      <li>
        <ListButton
          variant="row"
          class="w-full min-w-0"
          aria-pressed={selectedPath === path}
          onclick={() =>
            (selectedPath = selectedPath === path ? undefined : path)}
        >
          <span class="truncate" title={path}>{path}</span>
          {#if hasLineStats}
            <DiffCount added={stats.added} removed={stats.removed} />
          {/if}
        </ListButton>
        {#if selectedPath === path}
          {#each patches as patch, i (i)}
            {#if patches.length > 1}<p class="text-xs text-muted-foreground">
                片段 {i + 1} / {patches.length}
              </p>{/if}
            <TurnPatch {patch} />
          {/each}
        {/if}
      </li>
    {/each}
  </ul>
</ChatDisclosure>
