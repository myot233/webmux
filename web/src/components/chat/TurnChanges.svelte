<script lang="ts">
  import { SvelteMap } from "svelte/reactivity";
  import { ListButton } from "$lib/components/ui/list-button";
  import type { ChatContent } from "../../schemas";
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
</script>

<ChatDisclosure>
  {#snippet summary()}本轮改动 · {byFile.size} 个文件{/snippet}
  <p class="text-xs">根据 Agent 在本轮报告的工具改动。</p>
  <ul>
    {#each byFile as [path, patches] (path)}
      <li>
        <ListButton
          variant="row"
          class="w-full min-w-0"
          aria-pressed={selectedPath === path}
          onclick={() =>
            (selectedPath = selectedPath === path ? undefined : path)}
        >
          <span class="truncate" title={path}>{path}</span>
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
