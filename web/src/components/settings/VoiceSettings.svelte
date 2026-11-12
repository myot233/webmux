<script lang="ts">
  import IconExternalLink from "~icons/tabler/external-link";
  import { Badge } from "$lib/components/ui/badge";
  import { Button } from "$lib/components/ui/button";
  import { Label } from "$lib/components/ui/label";
  import { Switch } from "$lib/components/ui/switch";
  import * as Card from "$lib/components/ui/card";
  import { voicePreference } from "../../hooks/shared/voice-preference.svelte";
  import { useVoiceSettings } from "../../hooks/settings/voice-settings.svelte";
  import InlineNotice from "../shared/InlineNotice.svelte";
  import VoiceConfiguration from "./VoiceConfiguration.svelte";

  const protocolDoc =
    "https://github.com/mzdluo123/webmux/blob/main/docs/deployment/configuration.md#stdin--stdout-外部识别协议";

  const voice = useVoiceSettings();
  const view = $derived(voice.state.view);
</script>

<Card.Root flat id="voice" class="scroll-mt-4">
  <Card.Header>
    <Card.Title>语音输入</Card.Title>
    <Card.Description
      >聊天输入框旁与终端快捷栏的麦克风按钮，把语音交给服务端的外部识别程序，识别出的文字插入当前输入位置，不会自动回车。识别命令与环境变量保存在服务端并立即生效；下方开关只对当前浏览器生效。</Card.Description
    >
  </Card.Header>
  <Card.Content class="space-y-4">
    <div class="flex flex-wrap items-center gap-2 text-sm" role="status">
      <span class="text-muted-foreground">识别程序</span>
      {#if !view}
        <Badge variant="outline">读取中</Badge>
      {:else if view.enabled}
        <Badge variant="success">已配置</Badge>
      {:else}
        <Badge variant="outline">未配置</Badge>
      {/if}
      {#if view}<span class="text-xs text-muted-foreground"
          >{view.source === "web" ? "来自网页设置" : "来自配置文件"}</span
        >{/if}
    </div>
    <div class="flex items-center justify-between gap-4">
      <Label for="voice-enabled">在本设备启用语音输入</Label>
      <Switch
        id="voice-enabled"
        checked={voicePreference.enabled}
        onCheckedChange={(value) => (voicePreference.enabled = value)}
        disabled={!view?.enabled}
      />
    </div>
    {#if voice.state.error}
      <InlineNotice tone="danger">{voice.state.error}</InlineNotice>
    {/if}
    {#if view && !view.storageAvailable}
      <InlineNotice tone="warning"
        >网页设置的存储不可用，暂时不能保存：{view.storageError}</InlineNotice
      >
    {/if}
    {#if view}
      {#key voice.state.editVersion}
        <VoiceConfiguration
          current={view}
          disabled={voice.state.busy || !view.storageAvailable}
          onsave={(draft) => void voice.save(draft)}
          onreset={() => void voice.reset()}
        />
      {/key}
    {/if}
    {#if voice.state.notice}
      <InlineNotice tone="success">{voice.state.notice}</InlineNotice>
    {/if}
    <Button
      variant="outline"
      href={protocolDoc}
      target="_blank"
      rel="noreferrer"
      >语音识别协议文档<IconExternalLink aria-hidden="true" /></Button
    >
  </Card.Content>
</Card.Root>
