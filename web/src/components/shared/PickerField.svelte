<script lang="ts">
  import type { Snippet } from "svelte";
  import { MediaQuery } from "svelte/reactivity";
  import IconSelector from "~icons/tabler/selector";
  import * as Collapsible from "$lib/components/ui/collapsible";
  import * as Popover from "$lib/components/ui/popover";

  // A form row whose value is chosen from a list: a popover on desktop, and
  // on narrow screens a panel expanded in place so that nothing floats
  // outside the bottom drawer. `trigger` renders the current value.
  let {
    id,
    open = $bindable(false),
    disabled = false,
    trigger,
    children,
  }: {
    id: string;
    open?: boolean;
    disabled?: boolean;
    trigger: Snippet;
    children: Snippet;
  } = $props();
  const desktop = new MediaQuery("(min-width: 640px)");
  let inlineTrigger = $state<HTMLElement | null>(null);
  // Mounting the list on the first click would make that click slow, so the
  // desktop popover content is mounted, hidden, once the page is idle. An
  // open list is never remounted: that would drop its focus and search text.
  let warm = $state(false);
  $effect(() => {
    if (!desktop.current || warm || open) return;
    // Safari has no requestIdleCallback.
    if (!("requestIdleCallback" in window)) {
      const timer = setTimeout(() => (warm = true), 200);
      return () => clearTimeout(timer);
    }
    const handle = requestIdleCallback(() => (warm = true), { timeout: 1000 });
    return () => cancelIdleCallback(handle);
  });
  const triggerClass =
    "flex min-h-(--control-default) w-full min-w-0 items-center justify-between gap-2 rounded-md border border-input bg-card px-3 py-2 text-left text-sm outline-none hover:bg-muted focus-visible:border-ring focus-visible:ring-2 focus-visible:ring-ring/50 disabled:pointer-events-none disabled:opacity-50 aria-expanded:border-ring dark:bg-input/30 any-pointer-coarse:min-h-11";
</script>

{#snippet face()}
  <span id="{id}-value" class="flex min-w-0 flex-1 items-center gap-2"
    >{@render trigger()}</span
  >
  <IconSelector
    class="size-4 shrink-0 text-muted-foreground"
    aria-hidden="true"
  />
{/snippet}

{#if desktop.current}
  <Popover.Root bind:open>
    <Popover.Trigger
      {id}
      {disabled}
      aria-describedby="{id}-value"
      class={triggerClass}>{@render face()}</Popover.Trigger
    >
    <Popover.Content
      forceMount={warm}
      align="start"
      class="w-(--bits-popover-anchor-width) min-w-72 gap-0 overflow-hidden p-0 data-closed:hidden"
    >
      {@render children()}
    </Popover.Content>
  </Popover.Root>
{:else}
  <Collapsible.Root
    bind:open
    class="space-y-2"
    onkeydown={(event: KeyboardEvent) => {
      // Close only this panel; the drawer around it stays open.
      if (!open || event.key !== "Escape") return;
      event.preventDefault();
      event.stopPropagation();
      open = false;
      inlineTrigger?.focus();
    }}
  >
    <Collapsible.Trigger
      bind:ref={inlineTrigger}
      {id}
      {disabled}
      aria-describedby="{id}-value"
      class={triggerClass}>{@render face()}</Collapsible.Trigger
    >
    <Collapsible.Content>
      <div class="overflow-hidden rounded-lg border">{@render children()}</div>
    </Collapsible.Content>
  </Collapsible.Root>
{/if}
