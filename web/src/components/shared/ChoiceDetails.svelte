<script lang="ts">
  // A picker row or trigger: a title over a monospace command or path, and
  // an optional environment variable count.
  let {
    primary,
    secondary = "",
    envCount = 0,
  }: { primary: string; secondary?: string; envCount?: number } = $props();
</script>

<span class="grid min-w-0 flex-1 gap-0.5">
  <span class="truncate text-sm font-medium" title={primary}>{primary}</span>
  {#if secondary}<span
      class="truncate font-mono text-xs text-muted-foreground"
      title={secondary}>{secondary}</span
    >{/if}
  {#if envCount}<span class="text-xs text-muted-foreground"
      >{envCount} 个环境变量</span
    >{/if}
</span>
