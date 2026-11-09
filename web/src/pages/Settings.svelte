<script lang="ts">
  import Brand from "../components/Brand.svelte";
  import AppearanceSettings from "../components/AppearanceSettings.svelte";
  import { settings, changeSettings, saveSettings } from "../settings.svelte";
  import { errorMessage } from "../error";
  import { onMount } from "svelte";
  import IconArrowDown from "~icons/tabler/arrow-down";
  import IconArrowUp from "~icons/tabler/arrow-up";
  import IconBolt from "~icons/tabler/bolt";
  import IconPencil from "~icons/tabler/pencil";
  import IconPlus from "~icons/tabler/plus";
  import IconRocket from "~icons/tabler/rocket";
  import IconTrash from "~icons/tabler/trash";
  import { Badge } from "$lib/components/ui/badge";
  import { Button } from "$lib/components/ui/button";
  import { Input } from "$lib/components/ui/input";
  import { Label } from "$lib/components/ui/label";
  import { ListButton } from "$lib/components/ui/list-button";
  import * as DropdownMenu from "$lib/components/ui/dropdown-menu";
  import InfoHint from "../components/InfoHint.svelte";
  import SettingsRowMenu from "../components/SettingsRowMenu.svelte";
  import * as AlertDialog from "$lib/components/ui/alert-dialog";
  import * as Card from "$lib/components/ui/card";
  import * as Empty from "$lib/components/ui/empty";
  import * as Item from "$lib/components/ui/item";
  import { api, joinArgs, navigate } from "../api";
  import {
    ConfigSchema,
    LaunchConfigsSchema,
    LaunchPresetsSchema,
    NoContentSchema,
    PreviewSettingsSchema,
    type LaunchConfigSummary,
    type LaunchPreset,
    type PendingUpdate,
    type QuickCommand,
  } from "../schemas";
  import {
    LAUNCH_CONFIGS_LIMIT,
    launchConfigIcon,
    launchConfigSummary,
    type StartupShell,
  } from "../launch-configs";
  import LaunchConfigIcon from "../components/LaunchConfigIcon.svelte";
  import LaunchPresetMenu from "../components/LaunchPresetMenu.svelte";
  import {
    defaultPreviewSettings,
    previewSettings,
  } from "../preview-settings.svelte";
  import { selectPreview } from "../preview";
  import {
    QUICK_COMMANDS_LIMIT,
    commandLabel,
    commandPreview,
  } from "../quick-commands";
  import { quickCommands, saveQuickCommands } from "../quick-commands.svelte";
  import QuickCommandDialog from "../components/QuickCommandDialog.svelte";
  import QuickCommandPresetMenu from "../components/QuickCommandPresetMenu.svelte";
  import LaunchConfigDialog from "../components/LaunchConfigDialog.svelte";
  import InlineNotice from "../components/InlineNotice.svelte";
  import PendingUpdateNotice from "../components/PendingUpdateNotice.svelte";
  import TunnelSettings from "../components/TunnelSettings.svelte";
  import { setBrowserChrome } from "../browser-chrome";
  import TailscaleSettings from "../components/TailscaleSettings.svelte";
  import PushSettings from "../components/PushSettings.svelte";
  import InstallSettings from "../components/InstallSettings.svelte";
  import VoiceSettings from "../components/VoiceSettings.svelte";
  import SessionSwitcherTrigger from "../components/SessionSwitcherTrigger.svelte";
  import SettingsNavigation from "../components/SettingsNavigation.svelte";
  import NetworkSettings from "../components/NetworkSettings.svelte";
  import { Switch } from "$lib/components/ui/switch";
  import { browserDraftStorage } from "../draft-storage";

  // Only a session ID is accepted as the return target, never an arbitrary path.
  const returnSession = new URLSearchParams(location.search).get("session");
  const sessionId =
    returnSession && /^[a-z2-7]{8}$/.test(returnSession) ? returnSession : "";
  let dialogOpen = $state(false);
  let editing = $state<QuickCommand>();
  let deleteOpen = $state(false);
  let clearQuickOpen = $state(false);
  let deleting = $state<QuickCommand>();
  const full = $derived(quickCommands.list.length >= QUICK_COMMANDS_LIMIT);

  function edit(cmd?: QuickCommand) {
    editing = cmd;
    dialogOpen = true;
  }
  async function upsert(cmd: QuickCommand) {
    const list = quickCommands.list;
    const exists = list.some((item) => item.id === cmd.id);
    if (!exists && list.length >= QUICK_COMMANDS_LIMIT)
      return `最多保存 ${QUICK_COMMANDS_LIMIT} 条快捷指令，请先删除一条后再保存。`;
    await saveQuickCommands(
      exists
        ? list.map((item) => (item.id === cmd.id ? cmd : item))
        : [...list, cmd],
    );
    return "";
  }
  let presetBusy = $state(false);
  let presetError = $state("");
  async function addPresets(presets: QuickCommand[]) {
    presetBusy = true;
    presetError = "";
    try {
      await saveQuickCommands([...quickCommands.list, ...presets]);
    } catch (err) {
      presetError = errorMessage(err);
    } finally {
      presetBusy = false;
    }
  }
  function move(index: number, offset: -1 | 1) {
    const list = [...quickCommands.list];
    const from = list[index];
    const to = list[index + offset];
    if (!from || !to) return;
    list[index] = to;
    list[index + offset] = from;
    void saveQuickCommands(list).catch(() => undefined);
  }
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
  let defaultShell = $state<"" | StartupShell>("");
  let pendingUpdate = $state<PendingUpdate>();
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
  async function loadConfig() {
    try {
      const config = await api("/config", ConfigSchema, "GET");
      pendingUpdate = config.pendingUpdate;
      defaultShell =
        config.defaultShell === "zsh"
          ? "zsh"
          : config.defaultShell === "bash"
            ? "bash"
            : "";
    } catch {
      // Older or unavailable config responses keep the Bash fallback.
      defaultShell = "";
    }
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
  const draftStorage = browserDraftStorage();
  let keepDrafts = $state(draftStorage.enabled);
  let savedDrafts = $state(draftStorage.count());
  let savedScratch = $state(draftStorage.scratchCount());
  let clearDraftsOpen = $state(false);
  let linesDirty = $state(false);
  let skipDirty = $state(false);
  let lines = $state<number | undefined>(previewSettings.lines);
  let skip = $state<number | undefined>(previewSettings.skip);
  const validation = $derived(PreviewSettingsSchema.safeParse({ lines, skip }));
  const example = [
    ...Array.from({ length: 12 }, (_, index) => `输出第 ${index + 1} 行`),
    "模型 · 工作目录 · 状态栏",
    "❯ 输入提示符",
  ].join("\n");
  const preview = $derived(selectPreview(example, previewSettings));

  $effect(() => {
    if (!linesDirty) lines = previewSettings.lines;
    if (!skipDirty) skip = previewSettings.skip;
  });
  async function savePreview(field: "lines" | "skip") {
    const value = field === "lines" ? lines : skip;
    const parsed = PreviewSettingsSchema.safeParse({
      lines: previewSettings.lines,
      skip: previewSettings.skip,
      [field]: value,
    });
    if (!parsed.success) return;
    try {
      await saveSettings({ preview: parsed.data });
      if (field === "lines") linesDirty = false;
      else skipDirty = false;
    } catch {
      /* keep the input; shared status reports the failure */
    }
  }
  onMount(() => {
    setBrowserChrome("WebMux · 设置", false);
    void loadLaunchConfigs();
    void loadLaunchPresets();
    void loadConfig();
  });
</script>

{#snippet presetMenu()}
  <QuickCommandPresetMenu
    commands={quickCommands.list}
    busy={!settings.ready || presetBusy}
    onadd={(presets) => void addPresets(presets)}
  />
{/snippet}
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

<div class="safe-area-page min-h-dvh">
  <header class="app-header border-b bg-card">
    <div
      class="mx-auto flex min-h-(--app-header-height) max-w-6xl items-center justify-between gap-3 px-4 py-1 sm:px-8"
    >
      <a class="brand" href="/"><Brand /></a>
      <div class="flex items-center gap-2">
        <SessionSwitcherTrigger size="icon" />
        {#if sessionId}
          <Button variant="outline" onclick={() => navigate(`/s/${sessionId}`)}
            >返回终端</Button
          >
        {:else}
          <Button href="/" variant="outline">返回工作区</Button>
        {/if}
      </div>
    </div>
  </header>
  <div class="settings-layout mx-auto max-w-6xl px-4 sm:px-8">
    <SettingsNavigation />
    <main
      id="settings-content"
      class="settings-content min-w-0 space-y-6 py-8 sm:py-10"
    >
      <div>
        <h1 class="text-2xl font-semibold tracking-tight sm:text-3xl">设置</h1>
        <p class="mt-2 flex items-center gap-1 text-sm text-muted-foreground">
          大部分设置保存在服务端，在所有设备间同步。<InfoHint label="设置"
            >外观、快捷指令、预览和主页工作树保存在服务端，在所有设备间同步；草稿、最近目录与登录凭据仅在当前设备保存。启动配置与公网访问设置也由服务端管理。隧道配置和启停无需重启；通知推送按当前设备管理。</InfoHint
          >
        </p>
      </div>
      {#if pendingUpdate}<PendingUpdateNotice update={pendingUpdate} />{/if}
      <AppearanceSettings />
      <Card.Root flat id="quick-commands" class="scroll-mt-4">
        <Card.Header>
          <Card.Title>快捷指令</Card.Title>
          <Card.Description
            ><span class="flex items-center gap-1"
              >常用命令一键发送，同步到所有设备。<InfoHint label="快捷指令"
                >点击终端底部工具栏或顶栏的 <IconBolt
                  class="inline size-3.5 align-[-2px]"
                  aria-label="闪电"
                /> 按钮即可发送；保存到服务端并同步至所有设备。</InfoHint
              ></span
            ></Card.Description
          >
        </Card.Header>
        <Card.Content class="space-y-4">
          {#if quickCommands.list.length === 0}
            <Empty.Root variant="outline">
              <Empty.Header>
                <Empty.Media variant="icon"
                  ><IconBolt aria-hidden="true" /></Empty.Media
                >
                <Empty.Title>还没有快捷指令</Empty.Title>
                <Empty.Description
                  >把常用命令或提示词存起来，在终端一键发送。</Empty.Description
                >
              </Empty.Header>
              <Empty.Content>
                <div class="flex flex-wrap items-center justify-center gap-2">
                  <Button disabled={!settings.ready} onclick={() => edit()}
                    ><IconPlus aria-hidden="true" />添加快捷指令</Button
                  >
                  {@render presetMenu()}
                </div>
              </Empty.Content>
            </Empty.Root>
          {:else}
            <Item.Group variant="divided" aria-label="快捷指令列表">
              {#each quickCommands.list as cmd, index (cmd.id)}
                {@const label = commandLabel(cmd)}
                <Item.Root variant="divided" size="flush" class="min-h-14">
                  <ListButton
                    variant="content"
                    class="flex min-w-0 flex-1 items-center"
                    aria-label={`编辑 ${label}`}
                    onclick={() => edit(cmd)}
                  >
                    <span class="flex min-w-0 flex-1 items-center gap-2">
                      <span class="min-w-0 truncate text-sm font-medium"
                        >{label}</span
                      >
                      {#if cmd.enter}<Badge variant="secondary" class="shrink-0"
                          >回车</Badge
                        >{/if}
                      {#if cmd.name}<span
                          class="min-w-0 flex-1 truncate font-mono text-xs text-muted-foreground"
                          >{commandPreview(cmd)}</span
                        >{/if}
                    </span>
                  </ListButton>
                  <SettingsRowMenu {label} disabled={!settings.ready}>
                    <DropdownMenu.Item
                      aria-label={`编辑 ${label}`}
                      onSelect={() => edit(cmd)}
                      ><IconPencil aria-hidden="true" />编辑</DropdownMenu.Item
                    >
                    <DropdownMenu.Item
                      aria-label={`上移 ${label}`}
                      disabled={index === 0}
                      onSelect={() => move(index, -1)}
                      ><IconArrowUp aria-hidden="true" />上移</DropdownMenu.Item
                    >
                    <DropdownMenu.Item
                      aria-label={`下移 ${label}`}
                      disabled={index === quickCommands.list.length - 1}
                      onSelect={() => move(index, 1)}
                      ><IconArrowDown
                        aria-hidden="true"
                      />下移</DropdownMenu.Item
                    >
                    <DropdownMenu.Item
                      variant="destructive"
                      aria-label={`删除 ${label}`}
                      onSelect={() => {
                        deleting = cmd;
                        deleteOpen = true;
                      }}><IconTrash aria-hidden="true" />删除</DropdownMenu.Item
                    >
                  </SettingsRowMenu>
                </Item.Root>
              {/each}
            </Item.Group>
            <div class="flex flex-wrap items-center gap-2">
              <Button
                variant="outline"
                disabled={!settings.ready || full}
                onclick={() => edit()}
                ><IconPlus aria-hidden="true" />添加快捷指令</Button
              >
              {@render presetMenu()}
              {#if full}<span class="text-xs text-muted-foreground"
                  >最多 {QUICK_COMMANDS_LIMIT} 条</span
                >{/if}
            </div>
          {/if}
          {#if presetError}
            <InlineNotice tone="danger">
              <p>添加系统预设失败：{presetError}</p>
            </InlineNotice>
          {/if}
          <div class="flex flex-wrap gap-2">
            <Button
              variant="outline"
              size="sm"
              disabled={!settings.ready}
              onclick={() => changeSettings({ quickCommands: null })}
              >恢复预置指令</Button
            >
            <Button
              variant="destructive-outline"
              size="sm"
              disabled={!settings.ready || quickCommands.list.length === 0}
              onclick={() => {
                clearQuickOpen = true;
              }}>删除全部快捷指令</Button
            >
          </div>
        </Card.Content>
      </Card.Root>
      <VoiceSettings />
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
                        ><LaunchConfigIcon
                          icon={launchConfigIcon(config)}
                        /></span
                      >
                      <span class="grid min-w-0 flex-1 gap-0.5">
                        <span class="truncate text-sm font-medium"
                          >{config.name}</span
                        >
                        <span class="truncate text-xs text-muted-foreground"
                          >{launchConfigSummary(config)}</span
                        >
                      </span>
                    </span>
                  </ListButton>
                  <SettingsRowMenu
                    label={config.name}
                    disabled={!settings.ready}
                  >
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
                  onclick={() => void loadLaunchPresets()}
                  >重新读取系统预设</Button
                >
              {/if}
            </InlineNotice>
          {/if}
        </Card.Content>
      </Card.Root>
      <Card.Root flat id="drafts" class="scroll-mt-4">
        <Card.Header>
          <Card.Title>草稿与暂存</Card.Title>
          <Card.Description
            >草稿正文、暂存与未确认的提交按会话保存在当前浏览器，刷新、关闭页面或认证失效后可恢复；7
            天未修改或会话已不存在时自动清理。恢复后仍需点击发送才会提交。</Card.Description
          >
        </Card.Header>
        <Card.Content class="space-y-4">
          <div class="flex items-center justify-between gap-4">
            <Label for="keep-drafts">在本机保存草稿</Label>
            <Switch
              id="keep-drafts"
              checked={keepDrafts}
              onCheckedChange={(checked) => {
                draftStorage.setEnabled(checked);
                keepDrafts = draftStorage.enabled;
                savedDrafts = draftStorage.count();
                savedScratch = draftStorage.scratchCount();
              }}
            />
          </div>
          <p class="text-xs text-muted-foreground">
            关闭后立即删除本机已保存的草稿和暂存，之后内容只保留在当前页面内存中。
          </p>
          <div class="flex flex-wrap items-center gap-3">
            <span class="text-sm" role="status"
              >本机保存了 {savedDrafts} 个会话的草稿，{savedScratch} 条暂存</span
            >
            <Button
              variant="outline"
              disabled={!savedDrafts && !savedScratch}
              onclick={() => (clearDraftsOpen = true)}>清除本机草稿</Button
            >
          </div>
        </Card.Content>
      </Card.Root>
      <NetworkSettings />
      <TunnelSettings />
      <TailscaleSettings />
      <InstallSettings />
      <PushSettings />
      <Card.Root flat id="preview">
        <Card.Header>
          <Card.Title>会话输出预览</Card.Title>
          <Card.Description
            >从当前终端屏幕的底部向上选取非空行，可隐藏 agent
            的状态栏和输入提示符。启动配置可单独覆盖这些值；此处的全局设置用于临时命令和未配置预览的启动配置。</Card.Description
          >
        </Card.Header>
        <Card.Content class="space-y-6">
          <div class="grid gap-5 sm:grid-cols-2">
            <div class="space-y-2">
              <Label for="preview-lines">显示行数</Label>
              <Input
                id="preview-lines"
                type="number"
                min={1}
                max={50}
                step={1}
                bind:value={lines}
                oninput={() => {
                  linesDirty = true;
                }}
                onchange={() => void savePreview("lines")}
                disabled={!settings.ready}
                aria-invalid={!validation.success}
                aria-describedby="preview-lines-help"
              />
              <p id="preview-lines-help" class="text-xs text-muted-foreground">
                1–50 行；始终保留设定高度，避免后台输出更新时会话卡片跳动。
              </p>
            </div>
            <div class="space-y-2">
              <Label for="preview-skip">跳过末尾行数</Label>
              <Input
                id="preview-skip"
                type="number"
                min={0}
                max={50}
                step={1}
                bind:value={skip}
                oninput={() => {
                  skipDirty = true;
                }}
                onchange={() => void savePreview("skip")}
                disabled={!settings.ready}
                aria-invalid={!validation.success}
                aria-describedby="preview-skip-help"
              />
              <p id="preview-skip-help" class="text-xs text-muted-foreground">
                0–50 行；0 表示不隐藏末尾内容。
              </p>
            </div>
          </div>
          {#if !validation.success}
            <p role="alert" class="text-sm text-destructive">
              请输入有效整数：显示 1–50 行，跳过 0–50
              行。无效输入不会覆盖已保存设置。
            </p>
          {/if}
          <div class="space-y-2">
            <p class="text-sm">
              跳过末尾 {previewSettings.skip} 行，再向上显示 {previewSettings.lines}
              行。
            </p>
            <p class="text-xs text-muted-foreground">
              示例预览（12 行输出 + 2 行底栏，不是实际会话）
            </p>
            <pre
              aria-label="预览效果示例"
              style:--preview-lines={previewSettings.lines}
              class="h-preview min-w-0 overflow-hidden bg-muted p-2 font-mono text-xs leading-5 whitespace-pre text-foreground dark:bg-background">{preview ||
                "末尾行已隐藏，暂无可显示内容"}</pre>
          </div>
          <Button
            variant="outline"
            onclick={() => {
              linesDirty = false;
              skipDirty = false;
              changeSettings({ preview: null });
            }}
            >恢复默认（{defaultPreviewSettings.lines} 行，跳过 {defaultPreviewSettings.skip}
            行）</Button
          >
        </Card.Content>
      </Card.Root>
    </main>
  </div>
</div>

<QuickCommandDialog
  bind:open={dialogOpen}
  initial={editing}
  existing={quickCommands.list}
  save={upsert}
/>
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
<AlertDialog.Root bind:open={deleteOpen}>
  <AlertDialog.Content>
    <AlertDialog.Header>
      <AlertDialog.Title>删除快捷指令？</AlertDialog.Title>
      <AlertDialog.Description
        >将删除“{deleting
          ? commandLabel(deleting)
          : ""}”，此操作不能撤销。</AlertDialog.Description
      >
    </AlertDialog.Header>
    <AlertDialog.Footer>
      <AlertDialog.Cancel>取消</AlertDialog.Cancel>
      <Button
        variant="destructive"
        onclick={async () => {
          try {
            await saveQuickCommands(
              quickCommands.list.filter((item) => item.id !== deleting?.id),
            );
            deleteOpen = false;
          } catch {
            /* shared settings status retains error */
          }
        }}>删除</Button
      >
    </AlertDialog.Footer>
  </AlertDialog.Content>
</AlertDialog.Root>
<AlertDialog.Root bind:open={clearDraftsOpen}>
  <AlertDialog.Content>
    <AlertDialog.Header>
      <AlertDialog.Title>清除本机草稿？</AlertDialog.Title>
      <AlertDialog.Description
        >删除当前浏览器保存的全部草稿、暂存和未确认提交记录，不影响终端。已打开的终端页面在刷新前仍保留其内容。此操作无法撤销。</AlertDialog.Description
      >
    </AlertDialog.Header>
    <AlertDialog.Footer>
      <AlertDialog.Cancel>取消</AlertDialog.Cancel>
      <AlertDialog.Action
        variant="destructive"
        onclick={() => {
          draftStorage.clear();
          savedDrafts = draftStorage.count();
          savedScratch = draftStorage.scratchCount();
          clearDraftsOpen = false;
        }}>确认清除</AlertDialog.Action
      >
    </AlertDialog.Footer>
  </AlertDialog.Content>
</AlertDialog.Root>

<AlertDialog.Root bind:open={clearQuickOpen}>
  <AlertDialog.Content>
    <AlertDialog.Header>
      <AlertDialog.Title>删除全部快捷指令？</AlertDialog.Title>
      <AlertDialog.Description
        >所有设备上的快捷指令列表都会清空。恢复预置指令可重新显示默认列表。</AlertDialog.Description
      >
    </AlertDialog.Header>
    <AlertDialog.Footer>
      <AlertDialog.Cancel>取消</AlertDialog.Cancel>
      <Button
        variant="destructive"
        disabled={!settings.ready || settings.saving}
        onclick={async () => {
          try {
            await saveSettings({ quickCommands: [] });
            clearQuickOpen = false;
          } catch {
            /* shared status */
          }
        }}>删除全部</Button
      >
    </AlertDialog.Footer>
  </AlertDialog.Content>
</AlertDialog.Root>

<style>
  .settings-layout {
    display: grid;
    min-width: 0;
  }
  .settings-content > :global([id]) {
    scroll-margin-top: 6rem;
  }
  @media (min-width: 1024px) {
    .settings-layout {
      grid-template-columns: 12rem minmax(0, 1fr);
      gap: 2rem;
    }
    .settings-content > :global([id]) {
      scroll-margin-top: 1rem;
    }
  }
  @media (display-mode: standalone) {
    .settings-content > :global([id]) {
      scroll-margin-top: calc(9rem + env(safe-area-inset-top));
    }
  }
</style>
