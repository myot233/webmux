<script lang="ts">
  import IconList from "~icons/tabler/list";
  import IconTerminal2 from "~icons/tabler/terminal-2";
  import * as SegmentedControl from "$lib/components/ui/segmented-control";
  let {
    compact,
    onchange,
  }: { compact: boolean; onchange: (compact: boolean) => void } = $props();
</script>

<SegmentedControl.Root
  value={compact ? "compact" : "preview"}
  onValueChange={(value) => onchange(value === "compact")}
  aria-label="会话显示方式"
  class="h-auto min-w-0"
>
  <SegmentedControl.Item
    value="compact"
    class="min-h-11 min-w-11 sm:min-h-8 sm:min-w-0"
    aria-label="紧凑列表"
    title="紧凑列表"
  >
    <IconList aria-hidden="true" /><span class="hidden sm:inline">紧凑</span>
  </SegmentedControl.Item>
  <SegmentedControl.Item
    value="preview"
    class="min-h-11 min-w-11 sm:min-h-8 sm:min-w-0"
    aria-label="输出预览"
    title="输出预览"
  >
    <IconTerminal2 aria-hidden="true" /><span class="hidden sm:inline"
      >预览</span
    >
  </SegmentedControl.Item>
</SegmentedControl.Root>
