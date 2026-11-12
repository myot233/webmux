<script lang="ts">
  // A small label/value summary of the thing a dialog acts on, e.g. a
  // worktree's path, branch and saved commit.
  let {
    rows,
  }: {
    rows: { label: string; value: string; mono?: boolean; wrap?: boolean }[];
  } = $props();
</script>

<dl
  class="grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 gap-y-1 rounded-lg border bg-muted/40 p-3 text-sm"
>
  {#each rows as row (row.label)}
    <dt class="text-muted-foreground">{row.label}</dt>
    <dd class={[row.mono && "font-mono", row.wrap !== false && "break-all"]}>
      {row.value}
    </dd>
  {/each}
</dl>
