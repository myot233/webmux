<script lang="ts">
  import CodeReader from "../../src/components/code/CodeReader.svelte";
  import type { CodeReaderState } from "../../src/contracts/schemas";
  let selected = $state("");
  let {
    text,
    path = "test.ts",
    restore,
    hidden = false,
  }: {
    text: string;
    path?: string;
    restore?: CodeReaderState;
    hidden?: boolean;
  } = $props();
  let reader = $state<CodeReader>();
  export function capture() {
    return reader!.capture();
  }
  export function line() {
    return reader!.line();
  }
</script>

<div
  style="height:500px;flex-direction:column;overflow:hidden"
  style:display={hidden ? "none" : "flex"}
>
  <CodeReader
    bind:this={reader}
    {text}
    {path}
    {restore}
    onselect={(a, z) => (selected = `${a}-${z}`)}
  />
</div>
<output aria-label="Selected reference">{selected}</output>
