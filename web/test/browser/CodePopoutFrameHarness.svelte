<script lang="ts">
  import { untrack } from "svelte";
  import { useCodePopoutFrame } from "../../src/hooks/code/code-popout-frame.svelte";

  let {
    oninit,
  }: {
    oninit: (frame: ReturnType<typeof useCodePopoutFrame>) => void;
  } = $props();

  const frame = useCodePopoutFrame();
  untrack(() => oninit(frame));
</script>

<div data-phase={frame.phase}></div>
