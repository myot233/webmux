<script lang="ts">
  import type { Snippet } from "svelte";
  import IconChevronDown from "~icons/tabler/chevron-down";
  import { Button } from "$lib/components/ui/button";

  // The trigger of a compact picker below the chat input: a quiet button with
  // the choice's icon, optionally its text, and a chevron. Dropdown menus and
  // popovers pass their trigger props through `props`.
  let {
    props = {},
    label,
    title,
    text,
    textClass = "",
    icon,
  }: {
    props?: Record<string, unknown>;
    /** Accessible name: the full name and current value. */
    label: string;
    title?: string;
    /** The current value as text, for choices that must read at a glance. */
    text?: string;
    /** Extra classes on the value text, e.g. to hide it in a narrow card. */
    textClass?: string;
    icon: Snippet;
  } = $props();
</script>

<Button
  {...props}
  variant="quiet"
  size="xs"
  class={[
    // Option icons read at the same 16px as the neighbouring icon buttons.
    "[&_svg:not([class*='size-'])]:size-4",
    text === undefined ? "shrink-0" : "max-w-48 min-w-0 shrink",
  ]}
  aria-label={label}
  title={title ?? label}
>
  {@render icon()}
  {#if text !== undefined}<span class={["min-w-0 truncate", textClass]}
      >{text}</span
    >{/if}
  <IconChevronDown class="size-3" aria-hidden="true" />
</Button>
