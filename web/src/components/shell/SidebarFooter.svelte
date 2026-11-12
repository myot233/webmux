<script lang="ts">
  import { onMount, untrack } from "svelte";
  import IconMoon from "~icons/tabler/moon";
  import IconFolderPlus from "~icons/tabler/folder-plus";
  import IconSettings from "~icons/tabler/settings";
  import IconSun from "~icons/tabler/sun";
  import { api } from "../../platform/api";
  import { Button } from "$lib/components/ui/button";
  import * as Popover from "$lib/components/ui/popover";
  import {
    BUSY_LOAD_RATIO,
    formatBytes,
    hostStrain,
    SLOW_LATENCY_MS,
  } from "../../domains/sessions/host-health";
  import { p2pState } from "../../platform/p2p.svelte";
  import { HealthSchema, type Health } from "../../contracts/schemas";
  import { toggleTheme } from "../../hooks/settings/theme";
  import type { Connection } from "../../platform/ws";
  import ForwardButton from "../workspace/ForwardButton.svelte";

  // The sidebar's global footer, shared by both modes: the connection and
  // host, adding a repository, port forwarding, settings and the theme. A
  // healthy connection is only the host and a dot; reconnecting says so in
  // words.
  let {
    connection,
    hostname,
    setupSettled,
    rail = false,
    onforward,
    onadd,
    canAdd = true,
  }: {
    connection: Connection;
    hostname: string;
    setupSettled: boolean;
    /** The collapsed rail stacks the buttons and drops the host text. */
    rail?: boolean;
    onforward: () => void;
    /** Opens the add-repository dialog. */
    onadd: () => void;
    /** Git and the registry can take a new repository. */
    canAdd?: boolean;
  } = $props();

  let dark = $state(document.documentElement.classList.contains("dark"));
  let detailsOpen = $state(false);
  let latency = $state<number | null>(null);
  let measuring = $state(false);
  let health = $state<Health | null>(null);
  const effectiveLatency = $derived(
    p2pState.mode === "p2p" && p2pState.p2pRttMs !== null
      ? p2pState.p2pRttMs
      : latency,
  );
  const strain = $derived(
    connection === "connected" ? hostStrain(effectiveLatency, health) : "",
  );
  const statusLabel = $derived(
    connection === "connected"
      ? (hostname ? `${hostname} 在线` : "在线") +
          (strain ? `（${strain}）` : "")
      : connection === "reconnecting"
        ? "连接已断开，重连中…"
        : "正在连接",
  );
  const connectionLabel = $derived(
    connection === "connected"
      ? "已连接"
      : connection === "reconnecting"
        ? "重连中"
        : "连接中",
  );
  // A probe that never answers (a stalled path, e.g. over a VPN after a
  // network change) must end, or `measuring` would block every later one.
  const PROBE_TIMEOUT_MS = 10_000;
  async function measureLatency() {
    if (measuring || connection !== "connected") return;
    measuring = true;
    const started = performance.now();
    try {
      const next = await api(
        "/health",
        HealthSchema,
        "GET",
        undefined,
        AbortSignal.timeout(PROBE_TIMEOUT_MS),
      );
      latency = Math.max(0, Math.round(performance.now() - started));
      health = next;
    } catch {
      latency = null;
      health = null;
    } finally {
      measuring = false;
    }
  }
  function detailsChanged(open: boolean) {
    detailsOpen = open;
    if (open) void measureLatency();
  }
  // The dot turns yellow on a slow or loaded host, so it is measured while
  // connected, not only when the details are open.
  $effect(() => {
    if (connection !== "connected") return;
    // measureLatency reads and writes state; only `connection` may re-run
    // this, or each measurement would start the next.
    untrack(() => void measureLatency());
    const timer = setInterval(() => void measureLatency(), 15_000);
    return () => clearInterval(timer);
  });
  onMount(() => {
    const theme = new MutationObserver(() => {
      dark = document.documentElement.classList.contains("dark");
    });
    theme.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ["class"],
    });
    return () => theme.disconnect();
  });
</script>

<div
  class={rail
    ? "flex flex-col items-center gap-1 border-t py-2"
    : "flex items-center gap-1 border-t px-3 py-1.5"}
>
  <Popover.Root open={detailsOpen} onOpenChange={detailsChanged}>
    <Popover.Trigger
      type="button"
      aria-label={`查看主机状态：${statusLabel}`}
      title={statusLabel}
      class={rail
        ? "flex size-8 cursor-pointer items-center justify-center rounded-sm hover:bg-muted/60"
        : "flex h-5 min-w-0 flex-1 cursor-pointer items-center gap-2 rounded-sm text-xs text-muted-foreground hover:bg-muted/60"}
    >
      <!-- min-h-4 is the text line: the hostname arriving later must not move the dot. -->
      <span role="status" class="flex min-h-4 min-w-0 items-center gap-2">
        <span
          class={`status-dot shrink-0 ${connection === "connected" && !strain ? "status-running" : "status-waiting"}`}
        ></span>{#if rail}<span class="sr-only">{statusLabel}</span
          >{:else if connection === "reconnecting"}<span
            class="truncate text-warning">连接已断开，重连中…</span
          >{:else if connection === "connected" && setupSettled}<span
            class="truncate">{hostname || "在线"}</span
          >{:else}<span class="sr-only">{statusLabel}</span>{/if}
      </span>
    </Popover.Trigger>
    <Popover.Content
      side="top"
      align="start"
      role="dialog"
      aria-label="主机状态详情"
      class="w-[min(18rem,calc(100vw-2rem))] gap-3"
    >
      <Popover.Header>
        <Popover.Title>主机状态</Popover.Title>
        <Popover.Description>当前 WebMux 服务连接信息</Popover.Description>
      </Popover.Header>
      <dl class="grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 text-xs">
        <dt class="text-muted-foreground">主机名</dt>
        <dd class="min-w-0 truncate font-medium" title={hostname}>
          {hostname || "未获取"}
        </dd>
        <dt class="text-muted-foreground">连接状态</dt>
        <dd
          class={connection === "connected" ? "text-success" : "text-warning"}
        >
          {connectionLabel}
        </dd>
        <dt class="text-muted-foreground">传输路径</dt>
        <dd class={p2pState.mode === "p2p" ? "text-success" : ""}>
          {#if p2pState.mode === "p2p"}P2P 直连{p2pState.candidateType
              ? `（${p2pState.candidateType}${p2pState.ipFamily ? `/${p2pState.ipFamily}` : ""}）`
              : ""}{:else}WebSocket{/if}
        </dd>
        <dt class="text-muted-foreground">网络延时</dt>
        <dd
          class={effectiveLatency !== null && effectiveLatency > SLOW_LATENCY_MS
            ? "text-warning"
            : ""}
        >
          {#if connection !== "connected"}不可用{:else if measuring && effectiveLatency === null}测量中…{:else if effectiveLatency !== null}{effectiveLatency}
            ms{:else}不可用{/if}
        </dd>
        {#if health?.load}
          <dt class="text-muted-foreground">系统负载</dt>
          <dd
            class={health.load.one > health.load.cpus * BUSY_LOAD_RATIO
              ? "text-warning"
              : ""}
            title="1 / 5 / 15 分钟平均，{health.load.cpus} 个逻辑核心"
          >
            {health.load.one.toFixed(2)} / {health.load.five.toFixed(2)} /
            {health.load.fifteen.toFixed(2)}（{health.load.cpus} 核）
          </dd>
        {/if}
        {#if health?.memory}
          <dt class="text-muted-foreground">内存</dt>
          <dd>
            可用 {formatBytes(health.memory.available)} / 共 {formatBytes(
              health.memory.total,
            )}
          </dd>
        {/if}
      </dl>
    </Popover.Content>
  </Popover.Root>
  <Button
    variant="ghost"
    size="icon-touch"
    aria-label="添加仓库"
    title="添加仓库"
    disabled={!canAdd}
    onclick={() => onadd()}><IconFolderPlus aria-hidden="true" /></Button
  >
  <ForwardButton onclick={onforward} />
  <Button
    href="/settings"
    variant="ghost"
    size="icon-touch"
    aria-label="设置"
    title="设置"><IconSettings aria-hidden="true" /></Button
  >
  <Button
    variant="ghost"
    size="icon-touch"
    class="shrink-0"
    aria-label={dark ? "切换为白色主题" : "切换为黑色主题"}
    title={dark ? "切换为白色主题" : "切换为黑色主题"}
    onclick={toggleTheme}
    >{#if dark}<IconSun aria-hidden="true" />{:else}<IconMoon
        aria-hidden="true"
      />{/if}</Button
  >
</div>
