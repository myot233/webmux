<script lang="ts">
  import Brand from "../components/shell/Brand.svelte";
  import AppearanceSettings from "../components/shell/AppearanceSettings.svelte";
  import QuickCommandsSettings from "../components/settings/QuickCommandsSettings.svelte";
  import LaunchConfigsSettings from "../components/settings/LaunchConfigsSettings.svelte";
  import {
    settings,
    changeSettings,
    saveSettings,
  } from "../domains/settings/settings.svelte";
  import { onMount } from "svelte";
  import { Button } from "$lib/components/ui/button";
  import { Input } from "$lib/components/ui/input";
  import { Label } from "$lib/components/ui/label";
  import InfoHint from "../components/shared/InfoHint.svelte";
  import * as AlertDialog from "$lib/components/ui/alert-dialog";
  import * as Card from "$lib/components/ui/card";
  import { api, navigate } from "../platform/api";
  import {
    ConfigSchema,
    PreviewSettingsSchema,
    type PendingUpdate,
  } from "../contracts/schemas";
  import type { StartupShell } from "../domains/settings/launch-configs";
  import {
    defaultPreviewSettings,
    previewSettings,
  } from "../domains/settings/preview-settings.svelte";
  import { selectPreview } from "../domains/workspace/preview";
  import PendingUpdateNotice from "../components/notifications/PendingUpdateNotice.svelte";
  import TunnelSettings from "../components/settings/TunnelSettings.svelte";
  import { setBrowserChrome } from "../platform/browser-chrome";
  import TailscaleSettings from "../components/settings/TailscaleSettings.svelte";
  import PushSettings from "../components/settings/PushSettings.svelte";
  import InstallSettings from "../components/settings/InstallSettings.svelte";
  import VoiceSettings from "../components/settings/VoiceSettings.svelte";
  import SessionSwitcherTrigger from "../components/shell/SessionSwitcherTrigger.svelte";
  import SettingsNavigation from "../components/settings/SettingsNavigation.svelte";
  import NetworkSettings from "../components/settings/NetworkSettings.svelte";
  import { Switch } from "$lib/components/ui/switch";
  import { browserDraftStorage } from "../domains/terminal/draft-storage";

  // Only a session ID is accepted as the return target, never an arbitrary path.
  const returnSession = new URLSearchParams(location.search).get("session");
  const sessionId =
    returnSession && /^[a-z2-7]{8}$/.test(returnSession) ? returnSession : "";
  let defaultShell = $state<"" | StartupShell>("");
  let pendingUpdate = $state<PendingUpdate>();

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
    void loadConfig();
  });
</script>

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
      <QuickCommandsSettings />
      <VoiceSettings />
      <LaunchConfigsSettings {defaultShell} />
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
