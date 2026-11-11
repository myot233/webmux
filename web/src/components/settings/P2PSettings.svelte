<script lang="ts">
  import { Badge } from "$lib/components/ui/badge";
  import { Button } from "$lib/components/ui/button";
  import { Label } from "$lib/components/ui/label";
  import { Switch } from "$lib/components/ui/switch";
  import { Textarea } from "$lib/components/ui/textarea";
  import { useP2PSettings } from "../../hooks/settings/p2p-settings.svelte";
  import {
    p2pState,
    runP2PEchoABTest,
    type P2PPhase,
  } from "../../platform/p2p.svelte";
  import InlineNotice from "../shared/InlineNotice.svelte";

  const p2p = useP2PSettings();
  const ui = p2p.ui;
  const status = $derived(ui.status);
  const canEdit = $derived(
    !ui.saving && !!status?.supported && !!status?.storageAvailable,
  );
  const canReset = $derived(canEdit && status?.source === "web");

  let stunText = $state("");
  let stunDirty = $state(false);
  let stunError = $state("");

  $effect(() => {
    void ui.revision;
    if (status && !stunDirty) {
      stunText = status.stunServers.join("\n");
      stunError = "";
    }
  });

  function parseStunLines(raw: string): string[] | null {
    const lines = raw
      .split(/\r?\n/)
      .map((line) => line.trim())
      .filter((line) => line.length > 0);
    if (lines.length > 8) {
      stunError = "最多配置 8 个 STUN 服务器地址。";
      return null;
    }
    for (const line of lines) {
      if (!/^stuns?:[^\s/?#]+/i.test(line)) {
        stunError = `无效的 STUN 地址“${line}”，必须以 stun: 或 stuns: 开头。`;
        return null;
      }
    }
    stunError = "";
    return lines;
  }

  async function toggleEnabled(checked: boolean) {
    if (!status) return;
    await p2p.save({
      enabled: checked,
      portMapping: status.portMapping,
      stunServers: status.stunServers,
    });
  }

  async function togglePortMapping(checked: boolean) {
    if (!status) return;
    await p2p.save({
      enabled: status.enabled,
      portMapping: checked,
      stunServers: status.stunServers,
    });
  }

  async function saveStunServers() {
    if (!status) return;
    const parsed = parseStunLines(stunText);
    if (!parsed) return;
    const ok = await p2p.save({
      enabled: status.enabled,
      portMapping: status.portMapping,
      stunServers: parsed,
    });
    if (ok) stunDirty = false;
  }

  async function resetToConfig() {
    const ok = await p2p.reset();
    if (ok) {
      stunDirty = false;
      stunError = "";
    }
  }

  const phaseText: Record<P2PPhase, string> = {
    idle: "待命",
    disabled: "已关闭",
    unsupported: "当前环境不支持",
    skipped_local: "本机/内网或加密虚拟网直连，已跳过",
    skipped_low_rtt: "基线低延迟（< 15 ms），保持 WebSocket",
    probing: "正在测量基线 RTT…",
    negotiating: "正在协商 WebRTC 直连…",
    connected: "WebRTC DataChannel 已就绪",
    rejected_relay: "仅发现中继候选（relay），已拒绝升级",
    standby_hysteresis: "直连增益未达阈值，保持基线",
    fallback: "直连中断，已无感回退至 WebSocket 并后台重探",
  };

  const portMapBadge = $derived.by(() => {
    const pm = status?.portMap;
    if (!pm || pm.state === "off") {
      return { variant: "outline" as const, text: "已关闭" };
    }
    if (pm.state === "probing") {
      return { variant: "info" as const, text: "探测中…" };
    }
    if (pm.state === "mapped" && pm.externalPort) {
      const proto = (pm.protocol ?? "UDP").toUpperCase();
      return {
        variant: "success" as const,
        text: `已映射 ${proto} :${pm.externalPort}`,
      };
    }
    return {
      variant: "secondary" as const,
      text: "网关不支持（使用 STUN 打洞）",
    };
  });
</script>

<div class="space-y-4">
  <div class="flex flex-wrap items-center justify-between gap-2">
    <div class="space-y-1">
      <h3 class="text-base font-medium">机会性 P2P 直连升级</h3>
      <p class="text-xs text-muted-foreground">
        在公网高延迟隧道（如 Cloudflare Tunnel）下后台建立 WebRTC DataChannel
        直连，首屏不阻塞，失败或断连立即无感回落 WebSocket，且永不使用 TURN
        中继。
      </p>
    </div>
    {#if status}
      <Badge variant={status.source === "web" ? "info" : "outline"}>
        {status.source === "web" ? "网页设置" : "配置文件"}
      </Badge>
    {/if}
  </div>

  {#if ui.failure && !status}
    <InlineNotice tone="warning"
      >无法读取 P2P 状态：{ui.failure}。实时连接仍正常使用 WebSocket。</InlineNotice
    >
  {:else if status && !status.supported}
    <InlineNotice tone="info">
      当前服务端构建已裁剪 P2P 模块（<code>-tags nop2p</code>），始终使用标准
      HTTP / WebSocket。
    </InlineNotice>
  {:else if status}
    {#if !status.storageAvailable && status.storageError}
      <InlineNotice tone="warning"
        >网页设置不可写：{status.storageError}</InlineNotice
      >
    {/if}

    <div class="space-y-2">
      <div class="flex items-center justify-between gap-4">
        <Label for="p2p-enabled">自动尝试 P2P 直连升级</Label>
        <Switch
          id="p2p-enabled"
          checked={status.enabled}
          disabled={!canEdit}
          aria-describedby="p2p-enabled-help"
          onCheckedChange={(checked) => void toggleEnabled(checked)}
        />
      </div>
      <p id="p2p-enabled-help" class="text-xs text-muted-foreground">
        默认开启。回环地址、局域网私有地址、Tailscale 或基线 RTT 低于 15 ms
        时自动跳过。
      </p>
    </div>

    <div class="space-y-2">
      <div class="flex items-center justify-between gap-4">
        <div class="flex flex-wrap items-center gap-2">
          <Label for="p2p-port-mapping"
            >本地网关自动端口映射（UPnP / NAT-PMP / PCP）</Label
          >
          <Badge variant={portMapBadge.variant}>{portMapBadge.text}</Badge>
        </div>
        <Switch
          id="p2p-port-mapping"
          checked={status.portMapping}
          disabled={!canEdit || !status.enabled}
          aria-describedby="p2p-port-mapping-help"
          onCheckedChange={(checked) => void togglePortMapping(checked)}
        />
      </div>
      <p id="p2p-port-mapping-help" class="text-xs text-muted-foreground">
        向路由器申请临时 UDP 端口映射以提高 NAT
        直连成功率；服务退出时自动释放，不支持时退回纯 STUN 打洞。
      </p>
    </div>

    <div class="space-y-2">
      <Label for="p2p-stun-servers">STUN 服务器（每行一个，留空恢复预置）</Label
      >
      <Textarea
        id="p2p-stun-servers"
        code
        rows={3}
        disabled={!canEdit}
        bind:value={stunText}
        oninput={() => {
          stunDirty = true;
          if (stunError) parseStunLines(stunText);
        }}
        aria-invalid={!!stunError}
        aria-describedby="p2p-stun-help"
        placeholder="stun:stun.cloudflare.com:3478"
      />
      <p id="p2p-stun-help" class="text-xs text-muted-foreground">
        仅支持 <code>stun:</code> 或 <code>stuns:</code> 地址（最多 8
        个），不支持
        <code>turn:</code>。
      </p>
      {#if stunError}
        <p role="alert" class="text-xs text-destructive">{stunError}</p>
      {/if}
      <div class="flex flex-wrap items-center gap-2">
        <Button
          variant="outline"
          disabled={!canEdit || !stunDirty}
          onclick={() => void saveStunServers()}>保存 STUN 列表</Button
        >
        <Button
          variant="outline"
          disabled={!canReset}
          onclick={() => void resetToConfig()}>恢复配置文件默认</Button
        >
        {#if ui.feedback}
          <span role="status" class="text-xs text-muted-foreground"
            >{ui.feedback}</span
          >
        {/if}
      </div>
    </div>

    {#if ui.failure}
      <InlineNotice tone="danger">{ui.failure}</InlineNotice>
    {/if}

    <section
      aria-label="P2P 路径诊断"
      class="space-y-3 rounded-lg border bg-muted/30 p-3"
    >
      <div class="flex flex-wrap items-center justify-between gap-2">
        <div class="flex flex-wrap items-center gap-2" role="status">
          <Badge variant={p2pState.mode === "p2p" ? "success" : "secondary"}>
            {p2pState.mode === "p2p" ? "P2P 直连" : "WebSocket 基线"}
          </Badge>
          <span class="text-xs text-muted-foreground"
            >{phaseText[p2pState.phase]}</span
          >
        </div>
        <Button
          variant="outline"
          disabled={p2pState.abTest.running || !status.supported}
          onclick={() => void runP2PEchoABTest(status.stunServers)}
        >
          {p2pState.abTest.running ? "测速中…" : "双路径回显测速"}
        </Button>
      </div>

      <dl class="grid gap-x-6 gap-y-2 text-xs sm:grid-cols-3">
        <div>
          <dt class="text-muted-foreground">候选对 / 协议族</dt>
          <dd class="font-mono">
            {p2pState.candidateType
              ? `${p2pState.candidateType}${p2pState.ipFamily ? ` (${p2pState.ipFamily})` : ""}`
              : "—"}
          </dd>
        </div>
        <div>
          <dt class="text-muted-foreground">基线 RTT / P2P RTT</dt>
          <dd class="font-mono">
            {p2pState.baselineRttMs !== null
              ? `${p2pState.baselineRttMs} ms`
              : "—"} / {p2pState.p2pRttMs !== null
              ? `${p2pState.p2pRttMs} ms`
              : "—"}
          </dd>
        </div>
        <div>
          <dt class="text-muted-foreground">首屏 / 升级耗时 / 切换</dt>
          <dd class="font-mono">
            {p2pState.firstReadyMs !== null
              ? `${p2pState.firstReadyMs} ms`
              : "—"} / {p2pState.upgradeDurationMs !== null
              ? `${p2pState.upgradeDurationMs} ms`
              : "—"} / {p2pState.switchCount} 次
          </dd>
        </div>
      </dl>

      {#if p2pState.abTest.error}
        <p role="alert" class="text-xs text-destructive">
          测速结果：{p2pState.abTest.error}
        </p>
      {:else if p2pState.abTest.baselineMedianMs !== null}
        <div
          role="status"
          class="grid gap-2 border-t pt-2 text-xs sm:grid-cols-2"
        >
          <div>
            <span class="text-muted-foreground">基线 HTTP 回显：</span>
            <span class="font-mono"
              >最小 {p2pState.abTest.baselineMinMs} ms · 中位数 {p2pState.abTest
                .baselineMedianMs} ms</span
            >
          </div>
          <div>
            <span class="text-muted-foreground">P2P DataChannel 回显：</span>
            {#if p2pState.abTest.directSuccess}
              <span class="font-mono text-success"
                >最小 {p2pState.abTest.p2pMinMs} ms · 中位数 {p2pState.abTest
                  .p2pMedianMs} ms（{p2pState.abTest.candidateType ||
                  "direct"}{p2pState.abTest.ipFamily
                  ? `/${p2pState.abTest.ipFamily}`
                  : ""}）</span
              >
            {:else}
              <span class="text-muted-foreground">未建立直连</span>
            {/if}
          </div>
        </div>
      {/if}
    </section>
  {/if}
</div>
