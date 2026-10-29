<script lang="ts">
  import type { Component } from "svelte";
  import IconAdjustments from "~icons/tabler/adjustments";
  import IconAdjustmentsHorizontal from "~icons/tabler/adjustments-horizontal";
  import IconBrain from "~icons/tabler/brain";
  import IconCpu from "~icons/tabler/cpu";
  import IconSettings from "~icons/tabler/settings";
  import { Label } from "$lib/components/ui/label";
  import { Switch } from "$lib/components/ui/switch";
  import type { ConfigValue } from "../../chat-api";
  import type { ChatConfig } from "../../schemas";
  import ChatSelect from "./ChatSelect.svelte";

  // The agent's session options below the input: selects as icon menus,
  // booleans as icon switches. A change goes to the server; every device then
  // shows the value from the next state. Icons follow the option's semantic
  // category, so the bar stays quiet at every width; the full name and value
  // live in the tooltip, accessible name and menu.
  let {
    config,
    disabled,
    onchange,
  }: {
    config: ChatConfig[];
    disabled: boolean;
    onchange: (configId: string, value: ConfigValue) => void;
  } = $props();
  const icons: Record<string, Component> = {
    mode: IconAdjustmentsHorizontal,
    model: IconCpu,
    model_config: IconAdjustments,
    thought_level: IconBrain,
  };
  const iconFor = (category?: string) =>
    (category && Object.hasOwn(icons, category)
      ? icons[category]
      : undefined) ?? IconSettings;
</script>

{#each config as option (option.id)}
  {@const OptionIcon = iconFor(option.category)}
  {#if option.type === "boolean"}
    {@const state = option.enabled ? "已开启" : "已关闭"}
    <Label size="sm" class="mx-1 shrink-0" title={`${option.name}：${state}`}>
      <OptionIcon aria-hidden="true" />
      <Switch
        size="sm"
        checked={!!option.enabled}
        aria-label={`${option.name}：${state}`}
        {disabled}
        onCheckedChange={(enabled) => onchange(option.id, { enabled })}
      />
    </Label>
  {:else}
    <ChatSelect
      name={option.name}
      value={option.value ?? ""}
      choices={option.options}
      {disabled}
      onchange={(value) => onchange(option.id, { value })}
    >
      {#snippet icon()}<OptionIcon aria-hidden="true" />{/snippet}
    </ChatSelect>
  {/if}
{/each}
