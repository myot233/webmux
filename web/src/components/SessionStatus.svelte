<script lang="ts">
  import { Badge } from "$lib/components/ui/badge";
  import { sessionStatusText } from "../api";
  import type { SessionInfo } from "../schemas";

  let { session }: { session: SessionInfo } = $props();
  const failed = $derived(
    session.status === "exited" &&
      session.exitCode !== null &&
      session.exitCode !== 0,
  );
  const variant = $derived(
    failed
      ? "destructive"
      : session.status === "waiting"
        ? "warning"
        : session.status === "running"
          ? "success"
          : "secondary",
  );
</script>

<Badge {variant} data-slot="session-status" data-tone={variant}>
  <span class="size-1.5 shrink-0 rounded-full bg-current" aria-hidden="true"
  ></span>
  {sessionStatusText(session)}{session.exitCode !== null
    ? ` · code ${session.exitCode}`
    : ""}
</Badge>
