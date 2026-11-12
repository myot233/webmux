<script lang="ts">
  import type { Snippet } from "svelte";
  import IconSettings from "~icons/tabler/settings";
  import ChatChoiceCommandList from "./ChatChoiceCommandList.svelte";
  import QuietTrigger from "./QuietTrigger.svelte";
  import * as DropdownMenu from "$lib/components/ui/dropdown-menu";
  import * as Popover from "$lib/components/ui/popover";

  // A compact select below the chat input: a quiet button showing the
  // option's icon that opens a radio menu, or a searchable command list when
  // `searchable` is set. The value comes from the server state; the
  // accessible name and tooltip keep the full name and current value, and a
  // pick only reports the choice. `labeled` also shows the current value's
  // text, for choices that must read at a glance (quick start's repository,
  // base branch and agent); a choice with a `reason` stays in the menu,
  // disabled, with the reason under it.
  let {
    name,
    value,
    choices,
    disabled,
    title,
    labeled = false,
    searchable = false,
    labelClass,
    icon: props_icon,
    choiceIcon,
    onchange,
  }: {
    name: string;
    value: string;
    choices: {
      value: string;
      name: string;
      description?: string;
      reason?: string;
    }[];
    disabled: boolean;
    title?: string;
    labeled?: boolean;
    searchable?: boolean;
    labelClass?: string;
    icon?: Snippet;
    /** A per-choice icon, shown in the menu and, when labeled, the button. */
    choiceIcon?: Snippet<[string]>;
    onchange: (value: string) => void;
  } = $props();
  let open = $state(false);
  let search = $state("");
  function setOpen(next: boolean) {
    open = next;
    if (!next) search = "";
  }
  const current = $derived(
    choices.find((choice) => choice.value === value)?.name ?? value,
  );
  const label = $derived(`${name}：${current}`);
</script>

{#snippet triggerButton(props: Record<string, unknown>)}
  <QuietTrigger
    {props}
    {label}
    {title}
    text={labeled ? current : undefined}
    textClass={labelClass}
  >
    {#snippet icon()}
      {#if labeled && choiceIcon}{@render choiceIcon(
          value,
        )}{:else if props_icon}{@render props_icon()}{:else}<IconSettings
          aria-hidden="true"
        />{/if}
    {/snippet}
  </QuietTrigger>
{/snippet}

{#if searchable}
  <Popover.Root bind:open={() => open, setOpen}>
    <Popover.Trigger {disabled}>
      {#snippet child({ props })}
        {@render triggerButton(props)}
      {/snippet}
    </Popover.Trigger>
    <Popover.Content align="start" class="w-72 gap-0 overflow-hidden p-0">
      <ChatChoiceCommandList
        {name}
        {value}
        {choices}
        {disabled}
        {choiceIcon}
        bind:search
        onselect={(next) => {
          onchange(next);
          setOpen(false);
        }}
      />
    </Popover.Content>
  </Popover.Root>
{:else}
  <DropdownMenu.Root>
    <DropdownMenu.Trigger {disabled}>
      {#snippet child({ props })}
        {@render triggerButton(props)}
      {/snippet}
    </DropdownMenu.Trigger>
    <DropdownMenu.Content
      align="start"
      class="max-h-[min(20rem,var(--bits-dropdown-menu-content-available-height))] min-w-52"
    >
      <DropdownMenu.Label>{name}</DropdownMenu.Label>
      <DropdownMenu.RadioGroup {value} onValueChange={onchange}>
        {#each choices as choice (choice.value)}
          <DropdownMenu.RadioItem
            value={choice.value}
            disabled={disabled || !!choice.reason}
          >
            {#if choiceIcon}{@render choiceIcon(choice.value)}{/if}
            <span class="min-w-0 wrap-anywhere">
              {choice.name}
              {#if choice.reason}<span
                  class="block text-xs text-muted-foreground"
                  >{choice.reason}</span
                >{/if}
            </span>
          </DropdownMenu.RadioItem>
        {/each}
      </DropdownMenu.RadioGroup>
    </DropdownMenu.Content>
  </DropdownMenu.Root>
{/if}
