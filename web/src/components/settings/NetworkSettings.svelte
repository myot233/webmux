<script lang="ts">
  import * as Card from "$lib/components/ui/card";
  import { Label } from "$lib/components/ui/label";
  import { Separator } from "$lib/components/ui/separator";
  import { Switch } from "$lib/components/ui/switch";
  import { HIGH_PORT_MIN } from "../../domains/workspace/forwards.svelte";
  import {
    changeSettings,
    settings,
  } from "../../domains/settings/settings.svelte";
  import { setWsCompression, wsCompression } from "../../platform/ws";
  import P2PSettings from "./P2PSettings.svelte";

  let compression = $state(wsCompression());
  const ignoreHighPorts = $derived(settings.values.ignoreHighPorts ?? true);
</script>

<Card.Root flat id="network" class="scroll-mt-4">
  <Card.Header>
    <Card.Title>网络</Card.Title>
    <Card.Description
      >调整实时连接压缩、端口转发提示与机会性 P2P
      直连升级。连接压缩仅对当前浏览器生效；端口转发提示与 P2P
      设置在服务端持久化。</Card.Description
    >
  </Card.Header>
  <Card.Content class="space-y-6">
    <div class="space-y-2">
      <div class="flex items-center justify-between gap-4">
        <Label for="ws-compression">压缩实时连接（permessage-deflate）</Label>
        <Switch
          id="ws-compression"
          checked={compression}
          aria-describedby="ws-compression-help"
          onCheckedChange={(checked) => {
            setWsCompression(checked);
            compression = wsCompression();
          }}
        />
      </div>
      <p id="ws-compression-help" class="text-xs text-muted-foreground">
        切换后已打开的终端与列表连接会立即重连，输出从断点续传。小于 128
        字节的消息（如按键回显）不压缩，不增加输入延迟；每个连接在服务端多占约
        1.3 MB 内存。手机与平板在未选择时默认开启。
      </p>
    </div>
    <div class="space-y-2">
      <div class="flex items-center justify-between gap-4">
        <Label for="ignore-high-ports"
          >忽略 {HIGH_PORT_MIN} 及以上端口的转发提示</Label
        >
        <Switch
          id="ignore-high-ports"
          checked={ignoreHighPorts}
          disabled={!settings.ready}
          aria-describedby="ignore-high-ports-help"
          onCheckedChange={(checked) =>
            changeSettings({ ignoreHighPorts: checked ? null : false })}
        />
      </div>
      <p id="ignore-high-ports-help" class="text-xs text-muted-foreground">
        默认开启。不为 {HIGH_PORT_MIN}
        及以上的监听端口显示会话页转发提示，也不在端口转发对话框的“检测到的端口”中列出，避免临时通信端口干扰；仍可手动输入端口添加转发。
      </p>
    </div>
    <Separator />
    <P2PSettings />
  </Card.Content>
</Card.Root>
