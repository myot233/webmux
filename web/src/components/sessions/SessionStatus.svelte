<script lang="ts">
  import { Badge } from "$lib/components/ui/badge";
  import { sessionStatusText } from "../../platform/api";
  import type { SessionInfo } from "../../contracts/schemas";
  import { sessionStatusTone } from "../../domains/sessions/session-status";

  let { session }: { session: SessionInfo } = $props();
  const variant = $derived(sessionStatusTone(session));
</script>

<Badge
  {variant}
  class="h-auto min-h-5 max-w-full items-start justify-self-start whitespace-normal"
  data-slot="session-status"
  data-tone={variant}
>
  <span
    class="mt-1.25 size-1.5 shrink-0 rounded-full bg-current"
    aria-hidden="true"
  ></span>
  <span class="min-w-0 break-words"
    >{sessionStatusText(session)}{session.exitCode !== null
      ? ` · code ${session.exitCode}`
      : ""}</span
  >
</Badge>
