<script lang="ts">
  import type { Snippet } from "svelte";
  import IconChevronRight from "~icons/tabler/chevron-right";
  import * as Collapsible from "$lib/components/ui/collapsible";

  // A quiet one-line timeline row (thinking, browsed files, a tool call or a
  // compaction) whose details open below it.
  let {
    summary,
    children,
  }: {
    summary: Snippet;
    children: Snippet;
  } = $props();
  let open = $state(false);
</script>

<Collapsible.Root bind:open class="min-w-0">
  <Collapsible.Trigger
    class="flex w-full min-w-0 items-center gap-1.5 rounded-md px-1 py-0.5 text-left text-sm text-muted-foreground hover:bg-muted hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring/50 focus-visible:outline-none"
  >
    {@render summary()}
    <IconChevronRight
      class={open
        ? "size-3.5 shrink-0 rotate-90 transition-transform"
        : "size-3.5 shrink-0 transition-transform"}
      aria-hidden="true"
    />
  </Collapsible.Trigger>
  <Collapsible.Content class="mt-1 ml-2">
    <div
      class="min-w-0 space-y-1.5 border-l-2 pl-3 text-sm leading-relaxed text-muted-foreground"
    >
      {@render children()}
    </div>
  </Collapsible.Content>
</Collapsible.Root>
