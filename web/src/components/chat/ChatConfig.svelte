<script lang="ts">
  import { Label } from "$lib/components/ui/label";
  import { Switch } from "$lib/components/ui/switch";
  import type { ConfigValue } from "../../chat-api";
  import type { ChatConfig } from "../../schemas";
  import ChatSelect from "./ChatSelect.svelte";

  // The agent's session options below the input: selects as menus, booleans
  // as switches. A change goes to the server; every device then shows the
  // value from the next state.
  let {
    config,
    disabled,
    onchange,
  }: {
    config: ChatConfig[];
    disabled: boolean;
    onchange: (configId: string, value: ConfigValue) => void;
  } = $props();
</script>

{#each config as option (option.id)}
  {#if option.type === "boolean"}
    <Label size="sm" class="mx-2">
      <Switch
        size="sm"
        checked={!!option.enabled}
        {disabled}
        onCheckedChange={(enabled) => onchange(option.id, { enabled })}
      />
      {option.name}
    </Label>
  {:else}
    <ChatSelect
      name={option.name}
      value={option.value ?? ""}
      choices={option.options}
      {disabled}
      onchange={(value) => onchange(option.id, { value })}
    />
  {/if}
{/each}
