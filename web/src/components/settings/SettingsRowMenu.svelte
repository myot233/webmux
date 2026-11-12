<script lang="ts">
  import type { Snippet } from "svelte";
  import IconDots from "~icons/tabler/dots";
  import { Button } from "$lib/components/ui/button";
  import * as DropdownMenu from "$lib/components/ui/dropdown-menu";

  // The one action entry at the end of a settings list row: a "⋯" button that
  // opens the row's menu, so a row never carries a column of icon buttons.
  let {
    label,
    disabled = false,
    children,
  }: {
    /** The row's name; it completes the accessible name "操作 <label>". */
    label: string;
    disabled?: boolean;
    children: Snippet;
  } = $props();
</script>

<DropdownMenu.Root>
  <DropdownMenu.Trigger {disabled}>
    {#snippet child({ props })}
      <Button
        {...props}
        variant="ghost"
        size="icon"
        aria-label={`操作 ${label}`}
        title="操作"><IconDots aria-hidden="true" /></Button
      >
    {/snippet}
  </DropdownMenu.Trigger>
  <DropdownMenu.Content align="end" class="min-w-40">
    {@render children()}
  </DropdownMenu.Content>
</DropdownMenu.Root>
