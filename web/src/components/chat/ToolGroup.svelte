<script lang="ts">
  import type { ToolEntry, WorkStep } from "../../domains/chat/chat-model";
  import type { ChatCodeLink } from "../../domains/code/markdown-link";
  import ChatDisclosure from "./ChatDisclosure.svelte";
  import ToolDetails from "./ToolDetails.svelte";
  import ToolRow from "./ToolRow.svelte";
  import ToolSummary from "./ToolSummary.svelte";

  // A run of the agent's work (thinking and tool calls) folded into one row.
  // The collapsed row shows the latest tool call, so its spinner is the live
  // progress indicator; a call that waits for permission takes its place so it
  // is never hidden. A run without tool calls shows "已思考". Opening the row
  // lists every step in order, each with its own details. A run of one tool
  // call opens straight to that call's details.
  let {
    steps,
    waiting,
    onreview,
    onopenlocation,
  }: {
    steps: readonly WorkStep[];
    /** Tool calls whose permission request waits for an answer. */
    waiting: readonly string[];
    /** Opens a changed file in the code panel. */
    onreview: (path: string) => void;
    /** Opens a referenced code file or line in the code panel. */
    onopenlocation?: (target: ChatCodeLink) => void;
  } = $props();

  const calls = $derived(
    steps.filter((step): step is ToolEntry => step.type === "tool"),
  );
  const shown = $derived(
    calls.find((call) => waiting.includes(call.id)) ?? calls.at(-1),
  );
  const only = $derived(steps.length === 1 ? steps[0] : undefined);
</script>

{#if steps.length}
  <div
    class="min-w-0"
    data-tool-call={only?.type === "tool" ? only.id : undefined}
    data-tool-group={steps[0]?.id}
  >
    <ChatDisclosure>
      {#snippet summary()}
        {#if shown}
          <ToolSummary tool={shown} waiting={waiting.includes(shown.id)} />
          {#if calls.length > 1}
            <span class="shrink-0 text-xs tabular-nums @max-xs:hidden"
              >共 {calls.length} 步</span
            >
          {/if}
        {:else}
          <span class="min-w-0 flex-1 truncate">已思考</span>
        {/if}
      {/snippet}
      {#if only?.type === "tool"}
        <ToolDetails tool={only} {onreview} {onopenlocation} />
      {:else if only}
        <p class="break-words whitespace-pre-wrap">{only.text}</p>
      {:else}
        {#each steps as step (step.id)}
          {#if step.type === "tool"}
            <ToolRow
              tool={step}
              waiting={waiting.includes(step.id)}
              {onreview}
              {onopenlocation}
            />
          {:else}
            <ChatDisclosure>
              {#snippet summary()}已思考{/snippet}
              <p class="break-words whitespace-pre-wrap">{step.text}</p>
            </ChatDisclosure>
          {/if}
        {/each}
      {/if}
    </ChatDisclosure>
  </div>
{/if}
