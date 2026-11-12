<script lang="ts">
  import * as NativeSelect from "$lib/components/ui/native-select";
  import type { AgentCandidate } from "../../contracts/schemas";

  // The agent sessions a hybrid request may continue with, when the server
  // could not settle on one by itself; `picked` holds the chosen ID.
  let {
    candidates,
    picked = $bindable(""),
    label,
  }: {
    candidates: AgentCandidate[];
    picked?: string;
    label: string;
  } = $props();
</script>

{#if candidates.length}
  <NativeSelect.Root aria-label={label} class="mt-3 w-full" bind:value={picked}>
    {#each candidates as candidate (candidate.agentSessionId)}
      <NativeSelect.Option value={candidate.agentSessionId}
        >{candidate.title || candidate.agentSessionId}</NativeSelect.Option
      >
    {/each}
  </NativeSelect.Root>
{:else}
  同一目录下没有其他可恢复的会话。
{/if}
