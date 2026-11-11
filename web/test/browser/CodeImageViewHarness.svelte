<script lang="ts">
  import CodeImageView from "../../src/components/code/CodeImageView.svelte";
  import type {
    CodeDocument,
    CodeImageState,
  } from "../../src/contracts/schemas";
  let {
    document: doc,
    restore,
    hidden = false,
  }: {
    document: CodeDocument;
    restore?: CodeImageState;
    hidden?: boolean;
  } = $props();
  let view = $state<CodeImageView>();
  export function capture() {
    return view!.capture();
  }
</script>

<div
  style="height:650px;width:650px;flex-direction:column"
  style:display={hidden ? "none" : "flex"}
>
  <CodeImageView
    bind:this={view}
    id="session"
    snapshotId="snap-1"
    document={doc}
    {restore}
  />
</div>
