<script lang="ts">
  import * as DropdownMenu from "$lib/components/ui/dropdown-menu";
  import {
    QUICK_COMMANDS_LIMIT,
    QUICK_COMMAND_PRESETS,
    commandPreview,
  } from "../../domains/settings/quick-commands";
  import type { QuickCommand } from "../../contracts/schemas";
  import SystemPresetMenu from "./SystemPresetMenu.svelte";

  let {
    commands,
    busy,
    onadd,
  }: {
    commands: QuickCommand[];
    busy: boolean;
    /** Appends the given presets to the saved list. */
    onadd: (presets: QuickCommand[]) => void;
  } = $props();

  const existingIds = $derived(new Set(commands.map((cmd) => cmd.id)));
  const missing = $derived(
    QUICK_COMMAND_PRESETS.filter((preset) => !existingIds.has(preset.id)),
  );
  const full = $derived(commands.length >= QUICK_COMMANDS_LIMIT);
  const canAddAll = $derived(
    missing.length > 0 &&
      commands.length + missing.length <= QUICK_COMMANDS_LIMIT,
  );
  const disabled = $derived(busy || full || missing.length === 0);
</script>

<SystemPresetMenu
  {disabled}
  {busy}
  missingCount={missing.length}
  {canAddAll}
  onaddall={() => onadd(missing.map((preset) => ({ ...preset })))}
>
  {#each missing as preset (preset.id)}
    <DropdownMenu.Item
      class="min-h-9 any-pointer-coarse:min-h-11"
      disabled={busy || full}
      aria-label={`添加预设 ${preset.name}`}
      onSelect={() => onadd([{ ...preset }])}
    >
      <span class="flex min-w-0 flex-1 flex-col gap-0.5">
        <span class="truncate font-medium">{preset.name}</span>
        <span class="truncate font-mono text-xs text-muted-foreground"
          >{commandPreview(preset)}</span
        >
      </span>
    </DropdownMenu.Item>
  {/each}
</SystemPresetMenu>
