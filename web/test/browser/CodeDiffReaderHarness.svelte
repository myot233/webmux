<script lang="ts">
  import type { FileDiffMetadata } from "@pierre/diffs";
  import CodeDiffReader from "../../src/components/code/CodeDiffReader.svelte";
  import type { CodeDiffReaderState } from "../../src/contracts/schemas";
  let {
    fileDiff,
    restore,
    hidden = false,
  }: {
    fileDiff: FileDiffMetadata;
    restore?: CodeDiffReaderState;
    hidden?: boolean;
  } = $props();
  let reader = $state<CodeDiffReader>();
  let selected = $state("");
  export function capture() {
    return reader!.capture();
  }
</script>

<div
  style="height:500px;width:650px;flex-direction:column"
  style:display={hidden ? "none" : "flex"}
>
  <CodeDiffReader
    bind:this={reader}
    {fileDiff}
    {restore}
    side="new"
    expired={false}
    draftEnabled={false}
    onselect={(range) =>
      (selected = range ? `${range.start}-${range.end}` : "")}
  />
</div>
<output aria-label="Selected lines">{selected}</output>
