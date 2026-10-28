<script lang="ts">
  // Added and removed line counts of a change.
  let { added, removed }: { added: number; removed: number } = $props();
</script>

<span class="shrink-0 font-mono text-xs tabular-nums"
  ><span class="sr-only">新增 {added} 行，删除 {removed} 行</span><span
    class="text-success"
    aria-hidden="true">+{added}</span
  >
  <span class="text-destructive" aria-hidden="true">−{removed}</span></span
>
