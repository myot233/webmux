<script lang="ts">
  import { settings, changeSettings, reloadSettings } from "../settings.svelte";
  import { defaultCodeReader } from "../settings-intent";
  import {
    terminalFont,
    minFontSize,
    maxFontSize,
  } from "../hooks/terminal-font-preference.svelte";
  import { Button } from "$lib/components/ui/button";
  import { Input } from "$lib/components/ui/input";
  import { Label } from "$lib/components/ui/label";
  import { Switch } from "$lib/components/ui/switch";
  import * as Card from "$lib/components/ui/card";
  import * as SegmentedControl from "$lib/components/ui/segmented-control";
  import InlineNotice from "./InlineNotice.svelte";
  const code = $derived(settings.values.codeReader ?? defaultCodeReader);
  let fontDraft = $state<number | undefined>(undefined);
  const fontInput = $derived(fontDraft ?? terminalFont.size);
  const validFont = $derived(
    Number.isInteger(fontInput) &&
      fontInput >= minFontSize &&
      fontInput <= maxFontSize,
  );
</script>

<Card.Root flat id="appearance" class="scroll-mt-4">
  <Card.Header>
    <Card.Title>外观与阅读</Card.Title>
    <Card.Description
      >主题与阅读偏好保存到当前服务端，在已连接的设备间即时同步；终端字号只保存在此浏览器。恢复默认后使用设备的系统主题与默认字号。</Card.Description
    >
  </Card.Header>
  <Card.Content class="space-y-6">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <p role="status" class="text-sm text-muted-foreground">
        {settings.saving
          ? "正在保存…"
          : !settings.ready
            ? "设置不可用"
            : settings.connected
              ? "已连接 · 设置已同步"
              : "正在重连 · 显示最后确认的设置"}
      </p>
      <Button variant="outline" size="sm" onclick={() => void reloadSettings()}
        >重新读取</Button
      >
    </div>
    {#if settings.error}<InlineNotice tone="danger"
        >{settings.error}</InlineNotice
      >{/if}
    <div class="space-y-2">
      <Label for="settings-font">终端字号</Label>
      <Input
        id="settings-font"
        type="number"
        min={minFontSize}
        max={maxFontSize}
        step={1}
        value={fontInput}
        oninput={(event) => {
          fontDraft = event.currentTarget.valueAsNumber;
          if (validFont) terminalFont.set(fontDraft);
        }}
        onblur={() => (fontDraft = undefined)}
        aria-invalid={!validFont}
      />
      {#if !validFont}<p role="alert" class="text-sm text-destructive">
          请输入 {minFontSize}–{maxFontSize} 之间的整数。
        </p>{/if}
      <Button
        variant="outline"
        size="sm"
        onclick={() => {
          fontDraft = undefined;
          terminalFont.reset();
        }}>恢复默认字号</Button
      >
    </div>
    <fieldset disabled={!settings.ready} class="space-y-6">
      <div class="space-y-2">
        <p class="text-sm font-medium">主题</p>
        <SegmentedControl.Root
          value={settings.values.theme ?? "system"}
          onValueChange={(value) =>
            changeSettings({
              theme: value === "light" || value === "dark" ? value : null,
            })}
          disabled={!settings.ready}
          aria-label="主题"
        >
          <SegmentedControl.Item value="system">跟随系统</SegmentedControl.Item>
          <SegmentedControl.Item value="light">浅色</SegmentedControl.Item>
          <SegmentedControl.Item value="dark">深色</SegmentedControl.Item>
        </SegmentedControl.Root>
      </div>
      <div class="flex items-center justify-between gap-3">
        <Label for="settings-lines">代码行号</Label>
        <Switch
          id="settings-lines"
          checked={code.lineNumbers === "show"}
          onCheckedChange={(value) =>
            changeSettings({
              codeReader: { ...code, lineNumbers: value ? "show" : "hide" },
            })}
        />
      </div>
      <div class="flex items-center justify-between gap-3">
        <Label for="settings-wrap">Diff 自动换行</Label>
        <Switch
          id="settings-wrap"
          checked={code.diffOverflow === "wrap"}
          onCheckedChange={(value) =>
            changeSettings({
              codeReader: { ...code, diffOverflow: value ? "wrap" : "scroll" },
            })}
        />
      </div>
      <Button
        variant="outline"
        size="sm"
        onclick={() => changeSettings({ codeReader: null })}
        >恢复默认阅读设置</Button
      >
      <div class="flex items-center justify-between gap-3">
        <Label for="settings-softkeys">始终显示底部工具栏</Label>
        <Switch
          id="settings-softkeys"
          checked={settings.values.forceSoftKeys ?? false}
          onCheckedChange={(value) => changeSettings({ forceSoftKeys: value })}
        />
      </div>
      <Button
        variant="outline"
        size="sm"
        onclick={() => changeSettings({ forceSoftKeys: null })}
        >恢复默认工具栏</Button
      >
    </fieldset>
  </Card.Content>
</Card.Root>
