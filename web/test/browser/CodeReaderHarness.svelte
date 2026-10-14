<script lang="ts">
  import CodeReader from "../../src/components/CodeReader.svelte";
  let selected = $state("");
  let { text, path = "test.ts" }: { text: string; path?: string } = $props();
</script>

<div style="height:500px;display:flex;flex-direction:column;overflow:hidden">
  <CodeReader {text} {path} onselect={(a, z) => (selected = `${a}-${z}`)} />
</div>
<output aria-label="Selected reference">{selected}</output>
