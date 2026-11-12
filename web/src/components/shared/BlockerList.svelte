<script lang="ts">
  // Why a previewed worktree operation (removal, release, rebuild) cannot go
  // ahead, one reason per line.
  let { title, blockers }: { title: string; blockers: readonly string[] } =
    $props();
</script>

<div class="rounded-lg border border-destructive/30 p-3 text-sm">
  <p class="font-medium">{title}</p>
  <ul class="mt-2 list-disc space-y-1 pl-5">
    {#each blockers as blocker (blocker)}<li class="break-all">
        {blocker}
      </li>{/each}
  </ul>
</div>
