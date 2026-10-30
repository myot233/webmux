<script lang="ts">
  import { SvelteMap } from "svelte/reactivity";
  import { parseJson, type JsonNode } from "../code-json";
  import { Button } from "$lib/components/ui/button";
  import * as Collapsible from "$lib/components/ui/collapsible";
  import IconChevronRight from "~icons/tabler/chevron-right";
  let { text, onsource }: { text: string; onsource: () => void } = $props();
  const result = $derived(parseJson(text));
  // Large collections reveal their members in batches.
  const batch = 200;
  // Per-collection state keyed by its path; the first two levels start open.
  const opened = new SvelteMap<string, boolean>();
  const shown = new SvelteMap<string, number>();
  const isOpen = (key: string, depth: number) => opened.get(key) ?? depth < 2;
  function members(node: JsonNode) {
    return node.kind === "object"
      ? node.entries.map((entry) => ({ name: entry.key, value: entry.value }))
      : node.kind === "array"
        ? node.items.map((value, index) => ({ name: String(index), value }))
        : [];
  }
</script>

{#snippet tree(node: JsonNode, key: string, depth: number, name?: string)}
  {#if node.kind === "object" || node.kind === "array"}
    {@const all = members(node)}
    {@const open = isOpen(key, depth)}
    {@const limit = shown.get(key) ?? batch}
    <Collapsible.Root
      open={isOpen(key, depth)}
      onOpenChange={(value) => opened.set(key, value)}
    >
      <Collapsible.Trigger
        class="flex items-center gap-1 rounded-sm text-left hover:bg-muted"
      >
        <IconChevronRight
          aria-hidden="true"
          class={open ? "size-3.5 shrink-0 rotate-90" : "size-3.5 shrink-0"}
        />
        {#if name !== undefined}<span class="text-muted-foreground">{name}</span
          >{/if}
        <span
          >{node.kind === "object" ? "{}" : "[]"}
          <span class="text-muted-foreground">{all.length} 项</span></span
        >
      </Collapsible.Trigger>
      <Collapsible.Content class="pl-4">
        <!-- Closed collections render nothing, so large documents stay light. -->
        {#if open}
          {#each all.slice(0, limit) as member, index (index)}
            {@render tree(
              member.value,
              `${key}/${index}`,
              depth + 1,
              member.name,
            )}
          {/each}
          {#if all.length > limit}<Button
              variant="ghost"
              size="sm"
              onclick={() => shown.set(key, limit + batch)}
              >再显示 {batch} 项</Button
            >{/if}
        {/if}
      </Collapsible.Content>
    </Collapsible.Root>
  {:else}
    <p class="break-all whitespace-pre-wrap">
      {#if name !== undefined}<span class="text-muted-foreground">{name}:</span
        >{/if}
      {#if node.kind === "string"}<span class="text-primary"
          >"{node.value}"</span
        >{:else if node.kind === "number"}<span>{node.raw}</span
        >{:else if node.kind === "boolean"}<span>{node.value}</span>{:else}<span
          class="text-muted-foreground">null</span
        >{/if}
    </p>
  {/if}
{/snippet}

<div class="min-h-0 flex-1 overflow-auto p-2 font-mono text-xs">
  {#if result.ok}
    <section aria-label="JSON 预览">
      {@render tree(result.value, "", 0)}
    </section>
  {:else}
    <div class="space-y-2 font-sans text-sm">
      <p class="text-destructive" role="alert">
        第 {result.line} 行第 {result.column} 列：JSON 语法错误
      </p>
      <p class="text-muted-foreground">{result.detail}</p>
      <Button variant="outline" size="sm" onclick={onsource}>返回源码</Button>
    </div>
  {/if}
</div>
