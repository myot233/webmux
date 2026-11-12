<script lang="ts">
  import { Badge } from "$lib/components/ui/badge";
  import * as DropdownMenu from "$lib/components/ui/dropdown-menu";
  import { joinArgs } from "../../platform/api";
  import {
    LAUNCH_CONFIGS_LIMIT,
    launchConfigIcon,
  } from "../../domains/settings/launch-configs";
  import type {
    LaunchConfigSummary,
    LaunchPreset,
  } from "../../contracts/schemas";
  import LaunchConfigIcon from "./LaunchConfigIcon.svelte";
  import SystemPresetMenu from "./SystemPresetMenu.svelte";

  let {
    presets,
    configs,
    busy,
    onrestore,
  }: {
    presets: LaunchPreset[];
    configs: LaunchConfigSummary[];
    busy: boolean;
    onrestore: (name?: string) => void;
  } = $props();

  const existingNames = $derived(new Set(configs.map((config) => config.name)));
  const full = $derived(configs.length >= LAUNCH_CONFIGS_LIMIT);
  const missingCount = $derived(
    presets.filter((preset) => !existingNames.has(preset.name)).length,
  );
  const canRestoreAll = $derived(
    missingCount > 0 && configs.length + missingCount <= LAUNCH_CONFIGS_LIMIT,
  );
  const disabled = $derived(busy || full || presets.length === 0);
</script>

<SystemPresetMenu
  {disabled}
  {busy}
  {missingCount}
  canAddAll={canRestoreAll}
  onaddall={() => onrestore()}
>
  {#each presets as preset (preset.name)}
    {@const exists = existingNames.has(preset.name)}
    <DropdownMenu.Item
      class="min-h-9 any-pointer-coarse:min-h-11"
      disabled={busy || exists || full}
      aria-label={exists
        ? `${preset.name}（已存在）`
        : `添加预设 ${preset.name}`}
      onSelect={() => onrestore(preset.name)}
    >
      <LaunchConfigIcon icon={launchConfigIcon(preset)} />
      <span class="flex min-w-0 flex-1 flex-col gap-0.5">
        <span class="flex min-w-0 items-center gap-1.5">
          <span class="truncate font-medium">{preset.name}</span>
          {#if preset.chat}<Badge variant="secondary">混合</Badge>{/if}
          {#if preset.envNames.length}<Badge variant="secondary"
              >{preset.envNames.length} 个环境变量</Badge
            >{/if}
        </span>
        <span class="truncate font-mono text-xs text-muted-foreground"
          >{joinArgs(preset.cmd)}</span
        >
      </span>
      {#if exists}
        <Badge variant="outline">已存在</Badge>
      {/if}
    </DropdownMenu.Item>
  {/each}
</SystemPresetMenu>
