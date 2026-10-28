<script lang="ts">
  import IconChevronDown from "~icons/tabler/chevron-down";
  import { Button } from "$lib/components/ui/button";
  import * as DropdownMenu from "$lib/components/ui/dropdown-menu";
  import { Label } from "$lib/components/ui/label";
  import { Switch } from "$lib/components/ui/switch";
  import type { ConfigValue } from "../../chat-api";
  import type { ChatConfig } from "../../schemas";

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
  const current = (option: ChatConfig) =>
    option.options.find((choice) => choice.value === option.value)?.name ??
    option.value ??
    "";
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
    <DropdownMenu.Root>
      <DropdownMenu.Trigger {disabled}>
        {#snippet child({ props })}
          <Button
            {...props}
            variant="quiet"
            size="xs"
            class="max-w-[min(10rem,100%)] min-w-0"
            aria-label={`${option.name}：${current(option)}`}
            title={option.name}
          >
            <span class="truncate">{current(option)}</span>
            <IconChevronDown aria-hidden="true" />
          </Button>
        {/snippet}
      </DropdownMenu.Trigger>
      <DropdownMenu.Content align="start">
        <DropdownMenu.Label>{option.name}</DropdownMenu.Label>
        <DropdownMenu.RadioGroup
          value={option.value ?? ""}
          onValueChange={(value) => onchange(option.id, { value })}
        >
          {#each option.options as choice (choice.value)}
            <DropdownMenu.RadioItem value={choice.value}
              >{choice.name}</DropdownMenu.RadioItem
            >
          {/each}
        </DropdownMenu.RadioGroup>
      </DropdownMenu.Content>
    </DropdownMenu.Root>
  {/if}
{/each}
