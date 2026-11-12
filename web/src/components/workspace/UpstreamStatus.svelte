<script lang="ts">
  import IconArrowDown from "~icons/tabler/arrow-down";
  import IconArrowUp from "~icons/tabler/arrow-up";
  import { Badge } from "$lib/components/ui/badge";
  import {
    aheadLabel,
    behindLabel,
    upstreamText,
    type UpstreamState,
  } from "../../domains/workspace/worktree-sync";

  // A branch's or worktree's upstream: its name, then arrows with the
  // commits ahead and behind, or the words for a state without counts.
  // `indicator` keeps only what needs attention (counts, a deleted
  // upstream). Items flow into the parent's flex row.
  let {
    state,
    indicator = false,
  }: {
    state: UpstreamState | { kind: "unknown" };
    indicator?: boolean;
  } = $props();
  const text = $derived(upstreamText(state));
</script>

{#if !indicator && "upstream" in state}<span class="min-w-0 font-mono break-all"
    >{state.upstream}</span
  >{/if}
{#if state.kind === "tracked"}
  {#if state.ahead}<Badge
      variant="outline"
      role="img"
      aria-label={aheadLabel(state.ahead)}
      title={aheadLabel(state.ahead)}
      ><IconArrowUp aria-hidden="true" /><span class="tabular-nums"
        >{state.ahead}</span
      ></Badge
    >{/if}
  {#if state.behind}<Badge
      variant="outline"
      role="img"
      aria-label={behindLabel(state.behind)}
      title={behindLabel(state.behind)}
      ><IconArrowDown aria-hidden="true" /><span class="tabular-nums"
        >{state.behind}</span
      ></Badge
    >{/if}
{:else if state.kind === "gone"}<Badge variant="warning">{text}</Badge>{/if}
{#if !indicator && state.kind !== "gone" && text}<span
    class="text-muted-foreground">{text}</span
  >{/if}
