<script lang="ts">
  import type { Snippet } from "svelte";
  import IconInfoCircle from "~icons/tabler/info-circle";
  import { Button } from "$lib/components/ui/button";
  import * as Popover from "$lib/components/ui/popover";

  // "ⓘ" beside a one-line summary: the longer explanation opens on demand
  // instead of taking the first screen of a small phone.
  let {
    label,
    children,
  }: {
    /** What the hint explains; it names the button. */
    label: string;
    children: Snippet;
  } = $props();
</script>

<Popover.Root>
  <Popover.Trigger>
    {#snippet child({ props })}
      <Button
        {...props}
        variant="quiet"
        size="icon-xs"
        aria-label={`${label}说明`}
        title="说明"><IconInfoCircle aria-hidden="true" /></Button
      >
    {/snippet}
  </Popover.Trigger>
  <Popover.Content align="start" class="w-80">
    <div class="text-sm text-muted-foreground">{@render children()}</div>
  </Popover.Content>
</Popover.Root>
