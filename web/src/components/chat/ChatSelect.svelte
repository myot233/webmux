<script lang="ts">
  import type { Snippet } from "svelte";
  import IconChevronDown from "~icons/tabler/chevron-down";
  import IconSettings from "~icons/tabler/settings";
  import { Button } from "$lib/components/ui/button";
  import * as DropdownMenu from "$lib/components/ui/dropdown-menu";

  // A compact icon-only select below the chat input: a quiet button showing
  // the option's icon that opens a radio menu. The value comes from the
  // server state; the accessible name and tooltip keep the full name and
  // current value, and a pick only reports the choice.
  let {
    name,
    value,
    choices,
    disabled,
    title,
    icon,
    onchange,
  }: {
    name: string;
    value: string;
    choices: { value: string; name: string }[];
    disabled: boolean;
    title?: string;
    icon?: Snippet;
    onchange: (value: string) => void;
  } = $props();
  const current = $derived(
    choices.find((choice) => choice.value === value)?.name ?? value,
  );
  const label = $derived(`${name}：${current}`);
</script>

<DropdownMenu.Root>
  <DropdownMenu.Trigger {disabled}>
    {#snippet child({ props })}
      <Button
        {...props}
        variant="quiet"
        size="xs"
        class="shrink-0"
        aria-label={label}
        title={title ?? label}
      >
        {#if icon}{@render icon()}{:else}<IconSettings
            aria-hidden="true"
          />{/if}
        <IconChevronDown aria-hidden="true" />
      </Button>
    {/snippet}
  </DropdownMenu.Trigger>
  <DropdownMenu.Content align="start" class="min-w-52">
    <DropdownMenu.Label>{name}</DropdownMenu.Label>
    <DropdownMenu.RadioGroup {value} onValueChange={onchange}>
      {#each choices as choice (choice.value)}
        <DropdownMenu.RadioItem value={choice.value} {disabled}
          >{choice.name}</DropdownMenu.RadioItem
        >
      {/each}
    </DropdownMenu.RadioGroup>
  </DropdownMenu.Content>
</DropdownMenu.Root>
