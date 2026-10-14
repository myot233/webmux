<script lang="ts">
  import CodePanel from "../../src/components/CodePanel.svelte";
  let visible = $state(true);
  let inserted = $state("");
</script>

<button onclick={() => (visible = !visible)}>Toggle code visibility</button>
<div style="height:650px;display:flex;flex-direction:column">
  <CodePanel
    id="code-session"
    {visible}
    connection="connected"
    paste={(text) => {
      inserted = text;
      return true;
    }}
  />
</div>
<output aria-label="Inserted reference">{inserted}</output>
