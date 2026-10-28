<script lang="ts">
  import IconChevronDown from "~icons/tabler/chevron-down";
  import IconCircle from "~icons/tabler/circle";
  import IconCircleCheck from "~icons/tabler/circle-check";
  import IconLoader2 from "~icons/tabler/loader-2";
  import { Button } from "$lib/components/ui/button";
  import * as Collapsible from "$lib/components/ui/collapsible";
  import type { ChatPlan } from "../../schemas";

  // The agent's plan above the input: "计划 x/y" with the step in progress,
  // expanding to every step.
  let { plan }: { plan: ChatPlan } = $props();
  let open = $state(false);
  const current = $derived(
    plan.entries.find((entry) => entry.status === "in_progress"),
  );
  const done = $derived(
    plan.entries.filter((entry) => entry.status === "completed").length,
  );
</script>

<div class="rounded-xl border bg-card text-sm">
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
            class={open ? "ml-auto shrink-0 rotate-180" : "ml-auto shrink-0"}
            aria-hidden="true"
          />
        </Button>
      {/snippet}
    </Collapsible.Trigger>
    <Collapsible.Content>
      <ol class="space-y-1.5 border-t px-3 py-2">
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
    </Collapsible.Content>
  </Collapsible.Root>
</div>
