<script lang="ts">
  import type { ComponentProps } from "svelte";
  import CodeMarkdownPreview from "../../src/components/code/CodeMarkdownPreview.svelte";
  let {
    hidden = false,
    ...props
  }: ComponentProps<typeof CodeMarkdownPreview> & { hidden?: boolean } =
    $props();
  let preview = $state<CodeMarkdownPreview>();
  export function capture() {
    return preview!.capture();
  }
</script>

<!-- A bounded box, as in the code panel, so the preview scrolls itself. -->
<div style="height:400px;width:600px" style:display={hidden ? "none" : "block"}>
  <CodeMarkdownPreview bind:this={preview} {...props} />
</div>
