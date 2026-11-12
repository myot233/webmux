<script lang="ts">
  import { untrack } from "svelte";
  import DirectoryPicker from "../../src/components/workspace/DirectoryPicker.svelte";

  let {
    initialValue = "/workspace",
    anchorTop = 0,
  }: { initialValue?: string; anchorTop?: number } = $props();
  let value = $state(untrack(() => initialValue));
</script>

<div
  class="fixed inset-x-0 flex items-center justify-end gap-2 p-4"
  style:top={`${anchorTop}px`}
>
  <output aria-label="已确认目录" class="min-w-0 flex-1 truncate"
    >{value}</output
  >
  <DirectoryPicker bind:value />
</div>
