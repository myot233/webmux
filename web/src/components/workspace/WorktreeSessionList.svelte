<script lang="ts">
  import { sessionStatusText } from "../../platform/api";
  import type { SessionInfo } from "../../contracts/schemas";

  let {
    title,
    sessions,
  }: {
    title: string;
    sessions: readonly SessionInfo[];
  } = $props();
</script>

<div class="space-y-1.5 text-sm">
  <p class="font-medium">{title}</p>
  <ul class="space-y-1">
    {#each sessions as session (session.id)}
      <li class="break-all">
        {session.title} · {sessionStatusText(session)} · {session.cwd}
      </li>
    {/each}
  </ul>
</div>
