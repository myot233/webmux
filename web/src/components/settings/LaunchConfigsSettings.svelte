<script lang="ts">
  import { onMount } from "svelte";
  import { settings } from "../../domains/settings/settings.svelte";
  import { errorMessage } from "../../platform/error";
  import IconPlus from "~icons/tabler/plus";
  import IconRocket from "~icons/tabler/rocket";
  import IconTrash from "~icons/tabler/trash";
  import { Button } from "$lib/components/ui/button";
  import { ListButton } from "$lib/components/ui/list-button";
  import * as DropdownMenu from "$lib/components/ui/dropdown-menu";
  import InfoHint from "../shared/InfoHint.svelte";
  import SettingsRowMenu from "./SettingsRowMenu.svelte";
  import * as AlertDialog from "$lib/components/ui/alert-dialog";
  import * as Card from "$lib/components/ui/card";
  import * as Empty from "$lib/components/ui/empty";
  import * as Item from "$lib/components/ui/item";
  import { api, joinArgs } from "../../platform/api";
  import {
    LaunchConfigsSchema,
    LaunchPresetsSchema,
    NoContentSchema,
    type LaunchConfigSummary,
    type LaunchPreset,
  } from "../../contracts/schemas";
  import {
    LAUNCH_CONFIGS_LIMIT,
    launchConfigIcon,
    launchConfigSummary,
    type StartupShell,
  } from "../../domains/settings/launch-configs";
  import LaunchConfigIcon from "./LaunchConfigIcon.svelte";
  import LaunchPresetMenu from "./LaunchPresetMenu.svelte";
  import LaunchConfigDialog from "./LaunchConfigDialog.svelte";
  import InlineNotice from "../shared/InlineNotice.svelte";

  let { defaultShell = "" }: { defaultShell?: "" | StartupShell } = $props();

  // Launch configs live on the server; a load failure (for example 503 when
  // the state file is corrupt or locked) only disables this card.
  let launchConfigs = $state<LaunchConfigSummary[]>([]);
  let launchPresets = $state<LaunchPreset[]>([]);
  let launchState = $state<"loading" | "ready" | "unavailable">("loading");
  let launchError = $state("");
  let launchPresetBusy = $state(false);
  let launchPresetError = $state("");
  let launchDialogOpen = $state(false);
  let launchEditing = $state<string>();
  let launchDeleting = $state<LaunchConfigSummary>();
  let launchDeleteOpen = $state(false);
  let launchDeleteBusy = $state(false);
  let launchDeleteError = $state("");
  const launchFull = $derived(launchConfigs.length >= LAUNCH_CONFIGS_LIMIT);

  async function loadLaunchConfigs() {
    try {
      launchConfigs = await api("/launch-configs", LaunchConfigsSchema);
      launchError = "";
      launchState = "ready";
    } catch (err) {
      launchError = errorMessage(err);
      launchState = "unavailable";
    }
  }

  async function loadLaunchPresets() {
    try {
      launchPresets = await api("/launch-configs/presets", LaunchPresetsSchema);
      launchPresetError = "";
    } catch (err) {
      // An empty list disables the menu; keep the reason so the card can
      // explain why and offer a retry.
      launchPresets = [];
      launchPresetError = errorMessage(err);
    }
  }

  async function restoreLaunchPresets(name?: string) {
    if (launchPresetBusy) return;
    launchPresetBusy = true;
    launchPresetError = "";
    try {
      launchConfigs = await api(
        "/launch-configs/presets",
        LaunchConfigsSchema,
        "POST",
        name ? { name } : {},
      );
    } catch (err) {
      launchPresetError = errorMessage(err);
    } finally {
      launchPresetBusy = false;
    }
  }

  function editLaunchConfig(id?: string) {
    launchEditing = id;
    launchDialogOpen = true;
  }

  async function deleteLaunchConfig() {
    if (!launchDeleting) return;
    launchDeleteBusy = true;
    launchDeleteError = "";
    try {
      await api(
        `/launch-configs/${launchDeleting.id}`,
        NoContentSchema,
        "DELETE",
      );
      launchDeleteOpen = false;
      await loadLaunchConfigs();
    } catch (err) {
      launchDeleteError = errorMessage(err);
    } finally {
      launchDeleteBusy = false;
    }
  }

  onMount(() => {
    void loadLaunchConfigs();
    void loadLaunchPresets();
  });
</script>

{#snippet launchCreateActions(
  variant: "default" | "outline",
  disabled: boolean,
)}
  <Button {variant} {disabled} onclick={() => editLaunchConfig()}
    ><IconPlus aria-hidden="true" />新建启动配置</Button
  >
  <LaunchPresetMenu
    presets={launchPresets}
    configs={launchConfigs}
    busy={disabled || launchPresetBusy}
    onrestore={restoreLaunchPresets}
  />
{/snippet}

<Card.Root
  flat
  role="region"
  id="launch-configs"
  class="scroll-mt-4"
  aria-labelledby="launch-configs-title"
>
  <Card.Header>
    <Card.Title id="launch-configs-title">启动配置</Card.Title>
    <Card.Description
      ><span class="flex items-center gap-1"
        >新建会话时选用的命令与环境变量。<InfoHint label="启动配置"
          >保存“名称 + 完整命令 +
          环境变量”，在新建会话时选用；自定义启动配置保存在服务端，所有设备共享。也可以在新建会话时输入不保存的临时命令。</InfoHint
        ></span
      ></Card.Description
    >
  </Card.Header>
  <Card.Content class="space-y-4">
    {#if launchState === "loading"}
      <!-- Same shape as the empty state, the usual first result, so the
         cards below do not shift when the list arrives. -->
      <Empty.Root variant="outline" aria-busy="true">
        <Empty.Header>
          <Empty.Media variant="icon"
            ><IconRocket aria-hidden="true" /></Empty.Media
          >
          <Empty.Title role="status">正在读取启动配置…</Empty.Title>
          <Empty.Description
            >把常用命令、参数和环境变量存起来，在新建会话时一键选用。</Empty.Description
          >
        </Empty.Header>
        <Empty.Content>
          <div class="flex flex-wrap items-center justify-center gap-2">
            {@render launchCreateActions("default", true)}
          </div>
        </Empty.Content>
      </Empty.Root>
    {:else if launchState === "unavailable"}
      <InlineNotice tone="danger">
        <p>启动配置暂不可用，新建、编辑与删除已停用：</p>
        <p class="mt-1 font-mono text-xs">{launchError}</p>
      </InlineNotice>
      <Button
        variant="outline"
        onclick={() => {
          void loadLaunchConfigs();
          void loadLaunchPresets();
        }}>重新读取</Button
      >
    {:else if launchConfigs.length === 0}
      <Empty.Root variant="outline">
        <Empty.Header>
          <Empty.Media variant="icon"
            ><IconRocket aria-hidden="true" /></Empty.Media
          >
          <Empty.Title>还没有启动配置</Empty.Title>
          <Empty.Description
            >把常用命令、参数和环境变量存起来，在新建会话时一键选用。</Empty.Description
          >
        </Empty.Header>
        <Empty.Content>
          <div class="flex flex-wrap items-center justify-center gap-2">
            {@render launchCreateActions("default", false)}
          </div>
        </Empty.Content>
      </Empty.Root>
    {:else}
      <Item.Group variant="divided" aria-label="启动配置列表">
        {#each launchConfigs as config (config.id)}
          <Item.Root variant="divided" size="flush" class="min-h-14">
            <ListButton
              variant="content"
              class="flex min-w-0 flex-1 items-center"
              aria-label={`编辑 ${config.name}`}
              onclick={() => editLaunchConfig(config.id)}
            >
              <span class="flex min-w-0 flex-1 items-center gap-3">
                <span
                  class="grid size-9 shrink-0 place-items-center rounded-md bg-muted/60"
                  ><LaunchConfigIcon icon={launchConfigIcon(config)} /></span
                >
                <span class="grid min-w-0 flex-1 gap-0.5">
                  <span class="truncate text-sm font-medium">{config.name}</span
                  >
                  <span class="truncate text-xs text-muted-foreground"
                    >{launchConfigSummary(config)}</span
                  >
                </span>
              </span>
            </ListButton>
            <SettingsRowMenu label={config.name} disabled={!settings.ready}>
              <DropdownMenu.Item
                variant="destructive"
                aria-label={`删除 ${config.name}`}
                onSelect={() => {
                  launchDeleting = config;
                  launchDeleteError = "";
                  launchDeleteOpen = true;
                }}><IconTrash aria-hidden="true" />删除</DropdownMenu.Item
              >
            </SettingsRowMenu>
          </Item.Root>
        {/each}
      </Item.Group>
      <div class="flex flex-wrap items-center gap-3">
        {@render launchCreateActions("outline", launchFull)}
        {#if launchFull}<span class="text-xs text-muted-foreground"
            >最多 {LAUNCH_CONFIGS_LIMIT} 条，请先删除一条</span
          >{/if}
      </div>
    {/if}
    {#if launchPresetError && launchState !== "unavailable"}
      <InlineNotice tone="danger">
        <p>
          {launchPresets.length === 0
            ? "系统预设暂不可用，恢复菜单已停用："
            : "补回预设失败："}{launchPresetError}
        </p>
        {#if launchPresets.length === 0}
          <Button
            variant="outline"
            size="sm"
            class="mt-2"
            onclick={() => void loadLaunchPresets()}>重新读取系统预设</Button
          >
        {/if}
      </InlineNotice>
    {/if}
  </Card.Content>
</Card.Root>

<LaunchConfigDialog
  bind:open={launchDialogOpen}
  configId={launchEditing}
  {defaultShell}
  onsaved={() => void loadLaunchConfigs()}
/>
<AlertDialog.Root bind:open={launchDeleteOpen}>
  <AlertDialog.Content>
    <AlertDialog.Header>
      <AlertDialog.Title>删除启动配置？</AlertDialog.Title>
      <AlertDialog.Description
        >将删除“{launchDeleting?.name}”，此操作不能撤销；已运行的会话不受影响。</AlertDialog.Description
      >
    </AlertDialog.Header>
    {#if launchDeleting}<p
        class="overflow-x-auto rounded-md bg-muted p-3 font-mono text-xs"
      >
        {joinArgs(launchDeleting.cmd)}
      </p>{/if}
    {#if launchDeleteError}<InlineNotice tone="danger"
        >{launchDeleteError}</InlineNotice
      >{/if}
    <AlertDialog.Footer>
      <AlertDialog.Cancel disabled={launchDeleteBusy}>取消</AlertDialog.Cancel>
      <Button
        variant="destructive"
        disabled={launchDeleteBusy}
        onclick={deleteLaunchConfig}>删除</Button
      >
    </AlertDialog.Footer>
  </AlertDialog.Content>
</AlertDialog.Root>
