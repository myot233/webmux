<script lang="ts">
  import { createTwoFilesPatch } from "diff";
  import { ListButton } from "$lib/components/ui/list-button";
  import type { ChatContent } from "../../schemas";
  import ChatDisclosure from "./ChatDisclosure.svelte";

  let { changes }: { changes: readonly ChatContent[] } = $props();
  let selected = $state<ChatContent>();
  const tooLong = $derived(
    (selected?.oldText?.length ?? 0) + (selected?.newText?.length ?? 0) >
      24_000,
  );
  const patch = $derived(
    selected && !tooLong
      ? createTwoFilesPatch(
          selected.path ?? "",
          selected.path ?? "",
          selected.oldText ?? "",
          selected.newText ?? "",
        )
      : "",
  );
</script>

<ChatDisclosure>
  {#snippet summary()}本轮改动 · {changes.length} 个文件{/snippet}
  <p class="text-xs">根据 Agent 在本轮报告的工具改动。</p>
  <ul>
    {#each changes as change (change.path)}
      <li>
        <ListButton
          variant="row"
          class="w-full min-w-0"
          aria-pressed={selected === change}
          onclick={() => (selected = selected === change ? undefined : change)}
        >
          <span class="truncate" title={change.path}>{change.path}</span>
        </ListButton>
      </li>
    {/each}
  </ul>
  {#if selected?.truncated || tooLong}
    <p class="text-xs">改动内容过长或已截断。</p>
  {/if}
  {#if patch}
    <pre
      class="max-h-64 overflow-auto rounded-md bg-muted p-2 font-mono text-xs"
      aria-label="回合差异">{patch}</pre>
  {/if}
</ChatDisclosure>
