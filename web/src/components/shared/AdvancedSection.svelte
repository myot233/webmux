<script lang="ts">
  import type { Snippet } from "svelte";
  import IconChevronRight from "~icons/tabler/chevron-right";
  import * as Collapsible from "$lib/components/ui/collapsible";

  // Low-frequency form fields folded behind one line. The summary repeats
  // what is set so the folded state still shows it; it is hidden from
  // assistive technology because the fields carry the same values.
  let {
    title,
    summary = "",
    open = $bindable(false),
    children,
  }: {
    title: string;
    summary?: string;
    open?: boolean;
    children: Snippet;
  } = $props();
</script>

<Collapsible.Root bind:open class="space-y-3">
  <Collapsible.Trigger
    class="flex w-full min-w-0 items-center gap-1.5 rounded-md px-1 py-1 text-left text-sm text-muted-foreground hover:bg-muted hover:text-foreground any-pointer-coarse:min-h-11"
  >
    <IconChevronRight
      class={open
        ? "size-3.5 shrink-0 rotate-90 transition-transform"
        : "size-3.5 shrink-0 transition-transform"}
      aria-hidden="true"
    />
    <span class="shrink-0">{title}</span>
    {#if summary}<span
        class="min-w-0 truncate font-mono text-xs"
        aria-hidden="true">{summary}</span
      >{/if}
  </Collapsible.Trigger>
  <Collapsible.Content class="space-y-3">
    {@render children()}
  </Collapsible.Content>
</Collapsible.Root>
