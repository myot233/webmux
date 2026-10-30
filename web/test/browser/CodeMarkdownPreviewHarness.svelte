<script lang="ts">
  import type { ComponentProps } from "svelte";
  import CodeMarkdownPreview from "../../src/components/CodeMarkdownPreview.svelte";
  let props: ComponentProps<typeof CodeMarkdownPreview> = $props();
</script>

<!-- A bounded box, as in the code panel, so the preview scrolls itself. -->
<div style="height:400px;width:600px">
  <CodeMarkdownPreview {...props} />
</div>
