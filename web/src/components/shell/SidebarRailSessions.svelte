<script lang="ts">
  import { Button } from "$lib/components/ui/button";
  import { ScrollArea } from "$lib/components/ui/scroll-area";
  import type { SessionInfo } from "../../contracts/schemas";
  import {
    rowStatus,
    sessionRowLabel,
    sessionRowTitle,
  } from "../../domains/sessions/session-status";
  import LaunchConfigIcon from "../settings/LaunchConfigIcon.svelte";
  import RowStatus from "../shared/RowStatus.svelte";

  // The running sessions on the collapsed sidebar rail: one icon each for
  // quick switching, with the same status dot as a full sidebar row.
  let {
    sessions,
    current,
    onopen,
  }: {
    sessions: readonly SessionInfo[];
    /** The session page shown, or "" on a home page. */
    current: string;
    onopen: (session: SessionInfo) => void;
  } = $props();
</script>

{#if sessions.length}
  <ScrollArea
    type="hover"
    class="min-h-0 w-full flex-1 [&_[data-slot=scroll-area-viewport]>div]:!block"
  >
    <ul class="flex flex-col items-center gap-1 py-1" aria-label="运行中的会话">
      {#each sessions as session (session.id)}
        {@const status = rowStatus(session)}
        <li class="relative">
          <Button
            variant="ghost"
            size="icon-touch"
            aria-current={session.id === current ? "true" : undefined}
            aria-label={sessionRowLabel(session)}
            title={sessionRowTitle(session)}
            onclick={() => onopen(session)}
          >
            <LaunchConfigIcon icon={session.agent?.icon ?? ""} />
          </Button>
          <span class="pointer-events-none absolute right-1 bottom-1">
            <RowStatus {status} />
          </span>
        </li>
      {/each}
    </ul>
  </ScrollArea>
{/if}
