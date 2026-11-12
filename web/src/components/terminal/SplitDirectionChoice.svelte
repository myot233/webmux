<script lang="ts">
  import type { SplitDirection } from "../../domains/terminal/terminal-layout";
  import * as SegmentedControl from "$lib/components/ui/segmented-control";

  let { direction = $bindable() }: { direction: SplitDirection } = $props();
</script>

<SegmentedControl.Root
  bind:value={
    () => direction,
    (next) => {
      if (next === "horizontal" || next === "vertical") direction = next;
    }
  }
  aria-label="分屏方向"
  size="form"
  class="w-full"
>
  <SegmentedControl.Item value="horizontal">左右分屏</SegmentedControl.Item>
  <SegmentedControl.Item value="vertical">上下分屏</SegmentedControl.Item>
</SegmentedControl.Root>
