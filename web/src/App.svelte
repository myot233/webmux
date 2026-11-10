<script lang="ts">
  import Brand from "./components/Brand.svelte";
  import { startSettings, stopSettings } from "./settings.svelte";
  import { errorMessage } from "./error";
  import { onMount, tick } from "svelte";
  import { api, consumeTokenFragment, navigate, store, stored } from "./api";
  import { HealthSchema } from "./schemas";
  import { sync as syncPush } from "./push";
  import { Button } from "$lib/components/ui/button";
  import { Input } from "$lib/components/ui/input";
  import { Label } from "$lib/components/ui/label";
  import * as Card from "$lib/components/ui/card";
  import TokenHelp from "./components/TokenHelp.svelte";
  import LazyPage from "./components/LazyPage.svelte";
  import AppShell from "./components/AppShell.svelte";
  import SessionSwitcher from "./components/SessionSwitcher.svelte";
  import type TerminalPage from "./pages/Terminal.svelte";
  import {
    isSessionSwitcherShortcut,
    provideSessionSwitcher,
  } from "./session-switcher";
  import SessionList from "./pages/SessionList.svelte";
  import { matchRoute, type Route } from "./route";
  import { enterHome, openSession } from "./shell.svelte";
  import { createTerminalDrafts } from "./terminal-draft.svelte";
  import { browserDraftStorage } from "./draft-storage";
  import { clearHomeCache } from "./home-cache";
  import { sessionPins } from "./session-pins.svelte";
  import { resetAttentionToasts } from "./attention-toast.svelte";

  const drafts = createTerminalDrafts({ storage: browserDraftStorage() });
  let uploadNotice = $state("");

  enterHome();
  let path = $state(location.pathname);
  let ready = $state(false);
  let checking = $state(true);
  let token = $state(stored("token"));
  let error = $state("");
  const route = $derived(matchRoute(path));
  const sessionId = $derived(route.name === "terminal" ? route.id : "");
  const shellRoute = $derived(
    route.name === "sessions" ||
      route.name === "quick-start" ||
      route.name === "terminal"
      ? (route as Extract<
          Route,
          { name: "sessions" | "quick-start" | "terminal" }
        >)
      : null,
  );
  const canSwitch = $derived(
    ready && (shellRoute !== null || route.name === "settings"),
  );
  let shell = $state<AppShell>();
  let terminal = $state<TerminalPage>();
  let switcherOpen = $state(false);
  let switcherInvoker: HTMLElement | null = null;

  function openSwitcher(invoker: HTMLElement | null) {
    switcherInvoker = invoker;
    switcherOpen = true;
  }
  provideSessionSwitcher(openSwitcher);

  function switcherShortcut(event: KeyboardEvent) {
    if (
      !canSwitch ||
      switcherOpen ||
      !isSessionSwitcherShortcut(event) ||
      document.querySelector(
        '[role="dialog"][data-state="open"], [role="alertdialog"][data-state="open"], [role="menu"][data-state="open"], dialog[open]',
      )
    )
      return;
    // xterm and the pane shortcuts must never receive this chord.
    event.preventDefault();
    event.stopPropagation();
    const active = document.activeElement;
    openSwitcher(active instanceof HTMLElement ? active : null);
  }

  function restoreSwitcherFocus() {
    if (switcherInvoker?.isConnected) switcherInvoker.focus();
    switcherInvoker = null;
  }

  function selectSession(id: string) {
    switcherInvoker = null;
    if (!terminal?.activateSession(id) && id !== sessionId) openSession({ id });
  }

  // The frame of a pop-out code window; it only shows the code workspace.
  const codeId = $derived(route.name === "code" ? route.id : "");

  async function check() {
    checking = true;
    try {
      await api("/health", HealthSchema);
      ready = true;
      startSettings();
      error = "";
      // Re-registers an existing push subscription, e.g. after the server
      // rebuilt its store; without one it touches nothing.
      if (route.name !== "code" && route.name !== "forward-auth")
        void syncPush().catch(() => undefined);
    } catch (err) {
      error = errorMessage(err);
    } finally {
      checking = false;
    }
  }

  onMount(() => {
    let focusRoute = false;
    const focusMain = () => {
      if (!focusRoute) return;
      const main = document.querySelector("main");
      if (!main) return;
      focusRoute = false;
      // A page that already placed focus (quick start's input) keeps it.
      if (main.contains(document.activeElement)) return;
      main.tabIndex = -1;
      main.focus({ preventScroll: true });
    };
    const routes = new MutationObserver(focusMain);
    routes.observe(document.getElementById("app")!, {
      childList: true,
      subtree: true,
    });
    const change = () => {
      if (!shellRoute) enterHome();
      if (path !== location.pathname) switcherOpen = false;
      path = location.pathname;
      focusRoute = true;
      void tick().then(focusMain);
    };
    const auth = () => {
      stopSettings();
      drafts.clearAll();
      clearHomeCache();
      resetAttentionToasts();
      uploadNotice = "";
      ready = false;
      switcherOpen = false;
      switcherInvoker = null;
      checking = false;
      error = "Token 无效或已更改";
    };
    // Plain anchors to app routes would reload the whole document, which
    // blanks an iOS standalone PWA; route them through history instead.
    const link = (event: MouseEvent) => {
      if (
        event.defaultPrevented ||
        event.button !== 0 ||
        event.metaKey ||
        event.ctrlKey ||
        event.shiftKey ||
        event.altKey
      )
        return;
      const anchor =
        event.target instanceof Element ? event.target.closest("a") : null;
      if (!anchor || anchor.hasAttribute("download")) return;
      if (anchor.target && anchor.target !== "_self") return;
      const url = new URL(anchor.href, location.href);
      if (url.origin !== location.origin) return;
      const appRoute =
        url.pathname === "/" || matchRoute(url.pathname).name !== "sessions";
      if (!appRoute) return;
      if (url.pathname === location.pathname && url.search === location.search)
        return;
      event.preventDefault();
      navigate(url.pathname + url.search + url.hash);
    };
    document.addEventListener("click", link);
    window.addEventListener("popstate", change);
    window.addEventListener("keydown", switcherShortcut, true);
    window.addEventListener("webmux:auth", auth);
    if (consumeTokenFragment()) token = stored("token");
    const disconnectPins = sessionPins.connect();
    void check();
    return () => {
      disconnectPins();
      stopSettings();
      routes.disconnect();
      drafts.clearAll();
      document.removeEventListener("click", link);
      window.removeEventListener("popstate", change);
      window.removeEventListener("keydown", switcherShortcut, true);
      window.removeEventListener("webmux:auth", auth);
    };
  });
</script>

{#if !ready}
  <main class="flex min-h-dvh items-center justify-center p-6">
    <Card.Root class="w-full max-w-md">
      <Card.Header>
        <p class="brand mb-6"><Brand /></p>
        <Card.Title class="text-2xl">你的终端，始终在线。</Card.Title>
        <Card.Description>在可信内网中连接这台机器。</Card.Description>
      </Card.Header>
      <Card.Content>
        {#if checking}
          <p role="status" class="text-muted-foreground">正在连接…</p>
        {:else}
          <form
            class="space-y-5"
            onsubmit={(event) => {
              event.preventDefault();
              store("token", token);
              void check();
            }}
          >
            <div class="space-y-2">
              <div class="flex items-center gap-1">
                <Label for="token">共享 Token</Label>
                <TokenHelp />
              </div>
              <Input
                id="token"
                type="password"
                bind:value={token}
                autocomplete="current-password"
              />
            </div>
            {#if error}<p role="alert" class="text-destructive">{error}</p>{/if}
            <Button type="submit" class="w-full">连接 WebMux →</Button>
          </form>
        {/if}
      </Card.Content>
    </Card.Root>
  </main>
{:else if codeId}
  <LazyPage
    load={() => import("./pages/CodePopout.svelte")}
    label="代码"
    errorMessage={() => "代码窗口加载失败，请关闭后重新打开。"}
  >
    {#snippet children(page)}<page.default id={codeId} />{/snippet}
  </LazyPage>
{:else if route.name === "settings-tailscale"}
  <LazyPage
    load={() => import("./pages/TailscaleSettingsPage.svelte")}
    label="Tailscale 设置"
    loadingText="正在加载 Tailscale 设置…"
  >
    {#snippet children(page)}<page.default />{/snippet}
  </LazyPage>
{:else if route.name === "settings-tunnel"}
  <LazyPage
    load={() => import("./pages/TunnelSettingsPage.svelte")}
    label="公网访问设置"
  >
    {#snippet children(page)}<page.default />{/snippet}
  </LazyPage>
{:else if route.name === "settings"}
  <LazyPage load={() => import("./pages/Settings.svelte")} label="设置">
    {#snippet children(page)}<page.default />{/snippet}
  </LazyPage>
{:else if route.name === "forward-auth"}
  <LazyPage
    load={() => import("./pages/ForwardAuth.svelte")}
    label="端口转发"
    loadingText="正在打开转发…"
  >
    {#snippet children(page)}<page.default />{/snippet}
  </LazyPage>
{:else if shellRoute}
  <AppShell bind:this={shell} route={shellRoute} {drafts}>
    {#if sessionId}
      <LazyPage
        load={() => import("./pages/Terminal.svelte")}
        label="终端"
        errorMessage={(error) =>
          error instanceof Error ? error.message : "终端加载失败"}
      >
        {#snippet children(page)}
          {#key sessionId}<page.default
              bind:this={terminal}
              id={sessionId}
              {drafts}
              onUploadNotice={(message: string) => {
                if (ready) uploadNotice = message;
              }}
            />{/key}
        {/snippet}
      </LazyPage>
    {:else if route.name === "quick-start"}
      <LazyPage
        load={() => import("./pages/QuickStart.svelte")}
        label="快速开始"
      >
        {#snippet children(page)}<page.default />{/snippet}
      </LazyPage>
    {:else}
      <SessionList />
    {/if}
  </AppShell>
{/if}

{#if canSwitch}
  <SessionSwitcher
    bind:open={switcherOpen}
    source={shell?.sessionSwitcherSource()}
    onselect={selectSession}
    oncancel={restoreSwitcherFocus}
  />
{/if}

{#if ready && uploadNotice}
  <div
    class="fixed inset-x-2 top-2 z-50 flex items-start gap-2 rounded-md border bg-card p-3 text-sm shadow-lg"
    role="alert"
  >
    <p class="min-w-0 flex-1 break-all">{uploadNotice}</p>
    <Button variant="outline" size="sm" onclick={() => (uploadNotice = "")}
      >关闭</Button
    >
  </div>
{/if}
