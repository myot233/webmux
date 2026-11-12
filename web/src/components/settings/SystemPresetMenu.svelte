<script lang="ts">
  import type { Snippet } from "svelte";
  import IconChevronDown from "~icons/tabler/chevron-down";
  import { Badge } from "$lib/components/ui/badge";
  import { Button } from "$lib/components/ui/button";
  import * as DropdownMenu from "$lib/components/ui/dropdown-menu";

  // The "添加系统预设" dropdown shared by launch configs and quick commands:
  // an add-all row followed by one row per preset (the `children`).
  let {
    disabled,
    busy,
    missingCount,
    canAddAll,
    onaddall,
    children,
  }: {
    disabled: boolean;
    busy: boolean;
    missingCount: number;
    /** False when adding every missing preset would exceed the limit. */
    canAddAll: boolean;
    onaddall: () => void;
    children: Snippet;
  } = $props();
</script>

<DropdownMenu.Root>
  <DropdownMenu.Trigger {disabled}>
    {#snippet child({ props })}
      <Button {...props} variant="outline" {disabled}>
        添加系统预设
        <IconChevronDown aria-hidden="true" />
      </Button>
    {/snippet}
  </DropdownMenu.Trigger>
  <DropdownMenu.Content align="start" class="w-80 max-w-[calc(100vw-1rem)]">
    <DropdownMenu.Item
      class="min-h-9 any-pointer-coarse:min-h-11"
      disabled={busy || !canAddAll}
      onSelect={onaddall}
    >
      <span class="min-w-0 flex-1 truncate">补回全部缺失预设</span>
      {#if missingCount === 0}
        <Badge variant="outline">已齐全</Badge>
      {:else if !canAddAll}
        <Badge variant="outline">超出上限</Badge>
      {:else}
        <Badge variant="secondary">{missingCount} 项</Badge>
      {/if}
    </DropdownMenu.Item>
    <DropdownMenu.Separator />
    <div class="max-h-[min(55dvh,22rem)] overflow-x-hidden overflow-y-auto">
      {@render children()}
    </div>
  </DropdownMenu.Content>
</DropdownMenu.Root>
