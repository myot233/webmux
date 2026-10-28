<script lang="ts">
  import type { Snippet } from "svelte";
  import IconChevronDown from "~icons/tabler/chevron-down";
  import { Button } from "$lib/components/ui/button";
  import * as DropdownMenu from "$lib/components/ui/dropdown-menu";

  // A compact select below the chat input: a quiet button showing the
  // current choice that opens a radio menu. The value comes from the server
  // state; a pick only reports the choice.
  let {
    name,
    value,
    choices,
    disabled,
    title = name,
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
</script>

<DropdownMenu.Root>
  <DropdownMenu.Trigger {disabled}>
    {#snippet child({ props })}
      <Button
        {...props}
        variant="quiet"
        size="xs"
        class="max-w-[min(10rem,100%)] min-w-0"
        aria-label={`${name}：${current}`}
        {title}
      >
        {@render icon?.()}
        <span class="truncate">{current}</span>
        <IconChevronDown aria-hidden="true" />
      </Button>
    {/snippet}
  </DropdownMenu.Trigger>
  <DropdownMenu.Content align="start">
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
