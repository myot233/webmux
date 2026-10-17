<script lang="ts">
  import { useViewport } from "../../src/hooks/viewport.svelte";

  const viewport = useViewport();
</script>

<input aria-label="Composition input" />
<output aria-label="Viewport height">{viewport.height}</output>
<div style="height: 3000px"></div>
