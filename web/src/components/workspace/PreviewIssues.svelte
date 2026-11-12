<script lang="ts">
  import type { Issue } from "../../contracts/schemas";
  import InlineNotice from "../shared/InlineNotice.svelte";

  // A preview's blockers (nothing can be confirmed) and warnings, each with
  // the worktree path it refers to.
  let {
    blockers,
    warnings,
  }: {
    blockers: readonly Issue[];
    warnings: readonly Issue[];
  } = $props();
</script>

{#snippet list(issues: readonly Issue[])}
  <ul class="space-y-1">
    {#each issues as issue (issue.code + issue.message)}
      <li class="break-all">
        {issue.message}{#if issue.path}<span class="font-mono text-xs">
            （{issue.path}）</span
          >{/if}
      </li>
    {/each}
  </ul>
{/snippet}

{#if blockers.length}
  <InlineNotice tone="danger">{@render list(blockers)}</InlineNotice>
{/if}
{#if warnings.length}
  <InlineNotice tone="warning">{@render list(warnings)}</InlineNotice>
{/if}
