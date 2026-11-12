<script lang="ts">
  import WorktreeCreateFields from "../../src/components/workspace/WorktreeCreateFields.svelte";
  import type { WorktreePreset } from "../../src/domains/sessions/new-session-defaults";
  import type { Repository } from "../../src/contracts/schemas";

  let {
    repository,
    preset,
  }: { repository: Repository; preset?: WorktreePreset } = $props();
  let path = $state("");
  let mode = $state<"new" | "existing">("new");
  let branch = $state("");
  let base = $state("");
  let fields = $state<WorktreeCreateFields>();
  const ready = $derived(fields?.isReady() ?? false);
</script>

<WorktreeCreateFields
  bind:this={fields}
  {repository}
  {preset}
  bind:path
  bind:mode
  bind:branch
  bind:base
/>
<output aria-label="表单就绪">{ready ? "ready" : "not-ready"}</output>
<output aria-label="提交引用">{base}</output>
