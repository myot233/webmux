<script lang="ts">
  import { useDockMotion } from "../../hooks/shared/dock-motion.svelte";
  import IconChevronDown from "~icons/tabler/chevron-down";
  import IconCircle from "~icons/tabler/circle";
  import IconCircleCheck from "~icons/tabler/circle-check";
  import IconLoader2 from "~icons/tabler/loader-2";
  import { Button } from "$lib/components/ui/button";
  import * as Collapsible from "$lib/components/ui/collapsible";
  import type { ChatPlan } from "../../contracts/schemas";

  // The agent's plan. "dock" sits above the input as "计划 x/y" with the step
  // in progress, expanding to every step. "panel" is the always-open side
  // card pinned to the top right of a wide chat pane, where the transcript
  // leaves empty space.
  let {
    plan,
    variant = "dock",
  }: { plan: ChatPlan; variant?: "dock" | "panel" } = $props();
  let open = $state(false);
  let content = $state<HTMLDivElement | null>(null);
  useDockMotion(
    () => content,
    () => open,
  );
  const current = $derived(
    plan.entries.find((entry) => entry.status === "in_progress"),
  );
  const done = $derived(
    plan.entries.filter((entry) => entry.status === "completed").length,
  );
</script>

{#snippet steps()}
  <ol class="space-y-1.5 px-3 py-2">
    {#each plan.entries as entry, i (i)}
      <li class="flex items-start gap-2">
        {#if entry.status === "completed"}
          <IconCircleCheck
            class="mt-0.5 size-4 shrink-0 text-success"
            aria-label="完成"
          />
        {:else if entry.status === "in_progress"}
          <IconLoader2
            class="mt-0.5 size-4 shrink-0 animate-spin text-info"
            aria-label="进行中"
          />
        {:else}
          <IconCircle
            class="mt-0.5 size-4 shrink-0 text-muted-foreground"
            aria-label="待办"
          />
        {/if}
        <span
          class={entry.status === "completed"
            ? "min-w-0 break-words text-muted-foreground line-through"
            : "min-w-0 break-words"}>{entry.content}</span
        >
      </li>
    {/each}
  </ol>
{/snippet}

{#if variant === "panel"}
  <section
    class="pointer-events-auto flex max-h-full min-h-0 flex-col rounded-lg border bg-card text-sm"
    aria-label="计划"
    data-chat-plan="panel"
  >
    <h2
      class="flex shrink-0 items-center gap-2 border-b px-3 py-2 text-xs font-medium text-muted-foreground"
    >
      {#if current}
        <IconLoader2
          class="size-3.5 animate-spin text-info"
          aria-hidden="true"
        />
      {:else}
        <IconCircleCheck class="size-3.5 text-success" aria-hidden="true" />
      {/if}
      计划 {done}/{plan.entries.length}
    </h2>
    <div class="min-h-0 overflow-y-auto">{@render steps()}</div>
  </section>
{:else}
  <div class="rounded-lg border bg-card text-sm" data-chat-plan="dock">
    <Collapsible.Root bind:open>
      <Collapsible.Trigger>
        {#snippet child({ props })}
          <Button
            {...props}
            variant="quiet"
            size="xs"
            class="w-full min-w-0 justify-start"
          >
            {#if current}
              <IconLoader2 class="animate-spin text-info" aria-hidden="true" />
            {:else}
              <IconCircleCheck class="text-success" aria-hidden="true" />
            {/if}
            <span class="shrink-0">计划 {done}/{plan.entries.length}</span>
            {#if current}<span class="min-w-0 truncate text-foreground"
                >{current.content}</span
              >{/if}
            <IconChevronDown
              class={[
                "ml-auto shrink-0 motion-safe:transition-transform motion-safe:duration-200",
                open && "rotate-180",
              ]}
              aria-hidden="true"
            />
          </Button>
        {/snippet}
      </Collapsible.Trigger>
      <Collapsible.Content forceMount>
        {#snippet child({ props })}
          <div {...props} bind:this={content} hidden={!open} inert={!open}>
            <div class="border-t">{@render steps()}</div>
          </div>
        {/snippet}
      </Collapsible.Content>
    </Collapsible.Root>
  </div>
{/if}
