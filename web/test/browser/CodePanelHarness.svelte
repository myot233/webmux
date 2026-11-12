<script lang="ts">
  import CodePanel from "../../src/components/code/CodePanel.svelte";
  import type { CodeReveal } from "../../src/contracts/schemas";
  let { sessionCwd = "" }: { sessionCwd?: string } = $props();
  let visible = $state(true);
  let inserted = $state("");
  let reveal = $state<CodeReveal>();
</script>

<button onclick={() => (visible = !visible)}>Toggle code visibility</button>
<button
  onclick={() =>
    (reveal = { path: "/root/file-3.txt", key: (reveal?.key ?? 0) + 1 })}
  >Reveal file 3</button
>
<button
  onclick={() =>
    (reveal = {
      path: "file-3.txt",
      mode: "file",
      line: 2,
      endLine: 4,
      key: (reveal?.key ?? 0) + 1,
    })}>Reveal file 3 lines</button
>
<button
  onclick={() =>
    (reveal = {
      path: "src/app.ts",
      line: 3,
      key: (reveal?.key ?? 0) + 1,
    })}>Reveal repo relative</button
>
<button
  onclick={() =>
    (reveal = {
      path: "literal",
      rawPath: "literal:1",
      line: 1,
      key: (reveal?.key ?? 0) + 1,
    })}>Reveal raw path</button
>
<div style="height:650px;display:flex;flex-direction:column">
  <CodePanel
    id="code-session"
    {visible}
    {reveal}
    {sessionCwd}
    connection="connected"
    draftEnabled={false}
    paste={(text) => {
      inserted = text;
      return "terminal";
    }}
  />
</div>
<output aria-label="Inserted reference">{inserted}</output>
