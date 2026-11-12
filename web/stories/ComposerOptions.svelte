<script lang="ts">
  import QuickStartAgentToolbar from "../src/components/sessions/QuickStartAgentToolbar.svelte";
  import { launchConfig } from "../test/browser/shell-fixture";
  const agent = launchConfig();
</script>

<div class="@container p-6">
  {#each [false, true] as compact}
    <QuickStartAgentToolbar
      {compact}
      agents={[agent]}
      {agent}
      startSide="chat"
      chat={true}
      permissionMode="default"
      locked={false}
      probing={false}
      sending={false}
      options={Array.from({ length: 12 }, (_, index) => ({
        id: `option-${index}`,
        name: `配置 ${index + 1}`,
        type: "select" as const,
        value: "default",
        options: [{ value: "default", name: `选项 ${index + 1}` }],
      }))}
      onagentchange={() => {}}
      onsidechange={() => {}}
      onmodechange={() => {}}
      onoptionchange={() => {}}
    />
  {/each}
</div>
