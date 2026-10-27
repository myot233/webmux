<script lang="ts">
  import { Badge } from "$lib/components/ui/badge";
  import { Button } from "$lib/components/ui/button";
  import * as Card from "$lib/components/ui/card";
  import InlineNotice from "./InlineNotice.svelte";
  import { detectSupport } from "../push";
  import {
    adviseInstall,
    installation,
    promptInstall,
    type InstallPlatform,
  } from "../install.svelte";

  // Injectable so component tests need no real install prompt.
  let {
    platform = detectSupport(),
    status = installation,
    prompt = promptInstall,
  }: {
    platform?: InstallPlatform;
    status?: { promptable: boolean; installed: boolean };
    prompt?: () => Promise<"accepted" | "dismissed" | "unavailable">;
  } = $props();

  const advice = $derived(adviseInstall(platform, status));
  let busy = $state(false);
  let dismissed = $state(false);

  async function install() {
    busy = true;
    dismissed = false;
    try {
      dismissed = (await prompt()) !== "accepted";
    } finally {
      busy = false;
    }
  }
</script>

<Card.Root id="install" class="scroll-mt-4">
  <Card.Header>
    <Card.Title>安装应用</Card.Title>
    <Card.Description
      >把 WebMux
      安装为独立应用：从主屏幕或应用列表直接打开，没有浏览器地址栏；iOS
      还需要从主屏幕打开才能接收推送。只对当前设备生效。</Card.Description
    >
  </Card.Header>
  <Card.Content class="space-y-4">
    <div class="flex flex-wrap items-center gap-2 text-sm" role="status">
      {#if advice === "standalone"}
        <Badge>正在以应用方式运行</Badge>
      {:else if advice === "installed"}
        <Badge>已安装</Badge>
        <span class="text-muted-foreground">可从主屏幕或应用列表打开</span>
      {:else if advice === "prompt"}
        <Badge variant="secondary">未安装</Badge>
      {:else}
        <!-- Without a prompt the browser does not say whether the app is
             installed: a later tab of an installed app gets none either. -->
        <Badge variant="secondary">未以应用方式打开</Badge>
      {/if}
    </div>

    {#if !platform.secure}
      <InlineNotice tone="warning">
        <p class="font-medium">当前地址不能提供完整 PWA 能力</p>
        <p>
          浏览器只为 HTTPS
          或本机回环地址（127.0.0.1、localhost）提供应用安装、推送和剪贴板等能力；当前地址仍可作为普通网页使用。请改用
          Tailscale Serve、Cloudflare Named Tunnel 或 HTTPS 反向代理的地址打开
          WebMux。
        </p>
        <a class="underline underline-offset-2" href="#access"
          >查看公网访问配置</a
        >
      </InlineNotice>
    {/if}

    {#if advice === "prompt"}
      <Button disabled={busy} onclick={install}>安装 WebMux</Button>
    {:else if advice === "ios"}
      <div class="space-y-1 text-sm">
        <p class="font-medium">在 iOS / iPadOS 上添加到主屏幕</p>
        <ol class="list-decimal space-y-1 pl-5 text-muted-foreground">
          <li>在 Safari 中点“分享 → 添加到主屏幕”；</li>
          <li>
            从主屏幕图标打开 WebMux，并重新输入 Token（主屏幕应用不共享 Safari
            的登录）。
          </li>
        </ol>
        {#if !platform.secure}
          <p class="text-xs text-muted-foreground">
            当前地址可以添加到主屏幕，但推送仍需要 HTTPS。
          </p>
        {/if}
      </div>
    {:else if advice === "browser-menu" && platform.secure && !dismissed}
      <p class="text-sm text-muted-foreground">
        浏览器暂未提供安装提示；若已安装，可从主屏幕或应用列表打开。Chrome、Edge
        可从地址栏的安装图标或浏览器菜单中的“安装应用”“添加到主屏幕”安装；macOS
        Safari 使用“文件 → 添加到程序坞”；其他浏览器请查看其菜单。
      </p>
    {/if}
    {#if dismissed && advice !== "installed"}
      <p class="text-xs text-muted-foreground">
        已取消安装。浏览器允许时“安装
        WebMux”按钮会重新出现，也可以从浏览器菜单安装。
      </p>
    {/if}
  </Card.Content>
</Card.Root>
