<script lang="ts">
  import type { Snippet } from "svelte";
  import IconDots from "~icons/tabler/dots";
  import * as DropdownMenu from "$lib/components/ui/dropdown-menu";
  import { ListButton } from "$lib/components/ui/list-button";

  // Shared "⋯" trigger + lazy DropdownMenu.Content for sidebar rows.
  let {
    ariaLabel,
    tabindex,
    disabled = false,
    children,
  }: {
    ariaLabel: string;
    tabindex?: number;
    disabled?: boolean;
    children: Snippet;
  } = $props();

  let ready = $state(false);
</script>

<DropdownMenu.Root
  onOpenChange={(open) => {
    if (open) ready = true;
  }}
>
  <DropdownMenu.Trigger {disabled}>
    {#snippet child({ props })}
      <ListButton
        {...props}
        variant="file-tree-action"
        aria-label={ariaLabel}
        title={ariaLabel}
        {tabindex}><IconDots aria-hidden="true" /></ListButton
      >
    {/snippet}
  </DropdownMenu.Trigger>
  {#if ready}
    <DropdownMenu.Content align="end" class="min-w-48">
      {@render children()}
    </DropdownMenu.Content>
  {/if}
</DropdownMenu.Root>
