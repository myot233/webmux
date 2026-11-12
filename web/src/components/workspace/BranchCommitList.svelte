<script lang="ts">
  import { relativeTime } from "../../domains/workspace/commit-choices";
  import type { CommitList } from "../../contracts/schemas";

  // The commits a branch flow moves, pushes or would leave unreferenced:
  // the first ones of the range (at most 20) and how many more there are.
  let {
    title,
    commits,
  }: {
    title: string;
    commits: CommitList;
  } = $props();
  const more = $derived(commits.total - commits.items.length);
</script>

<section class="min-w-0 space-y-1.5" aria-label={title}>
  <h4 class="text-xs font-medium">
    {title}<span class="ml-1 text-muted-foreground tabular-nums"
      >（{commits.total}）</span
    >
  </h4>
  {#if commits.items.length}
    <ul class="space-y-1 text-xs">
      {#each commits.items as commit (commit.hash)}
        <li
          class="grid grid-cols-[auto_minmax(0,1fr)] gap-x-2"
          title={commit.hash}
        >
          <span class="font-mono text-muted-foreground"
            >{commit.hash.slice(0, 8)}</span
          >
          <span class="min-w-0 break-all"
            >{commit.subject || "无标题提交"}<span
              class="text-muted-foreground"
            >
              · {commit.author} · {relativeTime(commit.committedAt)}</span
            ></span
          >
        </li>
      {/each}
    </ul>
  {/if}
  {#if more > 0}
    <p class="text-xs text-muted-foreground">另有 {more} 个提交未列出。</p>
  {/if}
</section>
