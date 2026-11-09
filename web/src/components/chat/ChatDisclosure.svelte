<script lang="ts">
  import { onMount, tick, type Snippet } from "svelte";
  import { animate } from "motion/mini";
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
  let content = $state<HTMLDivElement | null>(null);
  let revealing = $state(false);
  let previous: boolean | undefined;
  let revision = 0;
  let cleanup: (() => void) | undefined;

  function settle() {
    revision += 1;
    cleanup?.();
    cleanup = undefined;
    revealing = false;
  }

  $effect.pre(() => {
    const expanded = open;
    const node = content;
    if (!node) return;
    const changed = previous !== undefined && previous !== expanded;
    previous = expanded;
    if (!changed) return;
    const height = node.getBoundingClientRect().height;
    settle();
    if (window.matchMedia("(prefers-reduced-motion: reduce)").matches) return;
    revealing = true;
    const current = revision;
    void tick().then(() => {
      if (revision !== current || !node.isConnected) return;
      node.style.overflow = "clip";
      const animation = animate(
        node,
        { height: [`${height}px`, `${expanded ? node.scrollHeight : 0}px`] },
        { duration: 0.25, ease: [0.22, 1, 0.36, 1] },
      );
      cleanup = () => {
        animation.cancel();
        node.style.removeProperty("height");
        node.style.removeProperty("overflow");
      };
      void animation.then(() => {
        if (revision === current) settle();
      });
    });
  });

  onMount(() => {
    const reduced = window.matchMedia("(prefers-reduced-motion: reduce)");
    window.addEventListener("resize", settle);
    reduced.addEventListener("change", settle);
    return () => {
      settle();
      window.removeEventListener("resize", settle);
      reduced.removeEventListener("change", settle);
    };
  });
</script>

<Collapsible.Root bind:open class="min-w-0">
  <Collapsible.Trigger
    class="flex w-full min-w-0 items-center gap-1.5 rounded-md px-1 py-0.5 text-left text-sm text-muted-foreground hover:bg-muted hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring/50 focus-visible:outline-none"
  >
    {@render summary()}
    <IconChevronRight
      class={open
        ? "size-3.5 shrink-0 rotate-90 motion-safe:transition-transform motion-safe:duration-250"
        : "size-3.5 shrink-0 motion-safe:transition-transform motion-safe:duration-250"}
      aria-hidden="true"
    />
  </Collapsible.Trigger>
  <Collapsible.Content forceMount>
    {#snippet child({ props })}
      <div
        {...props}
        bind:this={content}
        hidden={!open && !revealing}
        inert={!open}
        class="ml-2"
      >
        {#if open || revealing}
          <div class="pt-1">
            <div
              class="min-w-0 space-y-1.5 border-l-2 pl-3 text-sm leading-relaxed text-muted-foreground"
            >
              {@render children()}
            </div>
          </div>
        {/if}
      </div>
    {/snippet}
  </Collapsible.Content>
</Collapsible.Root>
