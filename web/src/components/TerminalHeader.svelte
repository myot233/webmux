<script lang="ts">
  import SidebarButton from "./SidebarButton.svelte";
  import { findShell, shellUi } from "../shell.svelte";
  import type { Snippet } from "svelte";
  import IconArrowLeft from "~icons/tabler/arrow-left";
  import IconCode from "~icons/tabler/code";
  import IconArrowDown from "~icons/tabler/arrow-down";
  import IconMaximize from "~icons/tabler/maximize";
  import IconMinimize from "~icons/tabler/minimize";
  import IconUpload from "~icons/tabler/upload";
  import IconSearch from "~icons/tabler/search";
  import { sessionStatusText } from "../api";
  import {
    Button,
    type ButtonSize,
    type ButtonVariant,
  } from "$lib/components/ui/button";
  import * as DropdownMenu from "$lib/components/ui/dropdown-menu";
  import type { Fullscreen } from "../hooks/fullscreen.svelte";
  import type { HybridView } from "../hooks/hybrid.svelte";
  import type { SwitchTarget } from "../hybrid";
  import type { SessionInfo } from "../schemas";
  import { sessionStatusTone } from "../session-status";
  import type { TerminalDraft } from "../terminal-draft.svelte";
  import type { Connection } from "../ws";
  import DraftToggle from "./DraftToggle.svelte";
  import SessionViewTabs from "./SessionViewTabs.svelte";
  import LaunchConfigIcon, { hasLaunchIcon } from "./LaunchConfigIcon.svelte";
  import UploadFileInput from "./UploadFileInput.svelte";
  import TerminalShortcuts from "./TerminalShortcuts.svelte";

  interface HeaderTerminal {
    readonly info: SessionInfo | undefined;
    readonly connection: Connection;
    readonly exitCode: number | null;
    readonly restoring: boolean;
    readonly historyTruncated: boolean;
    readonly canFind: boolean;
    scrollToBottom(): void;
    clear(): void;
    resync(): void;
  }
  // A hybrid session switches between its chat view and its terminal.
  interface HeaderHybrid {
    readonly view: HybridView;
    readonly side: SwitchTarget | null;
    readonly target: SwitchTarget | null;
    readonly chatUnavailable: string;
    request(to: SwitchTarget): void;
  }

  let {
    terminal: t,
    fullscreen,
    draft,
    draftSaved,
    scratchSaved,
    uploading,
    desktop,
    sizeLocked,
    setSizeLocked,
    keyboardSizeHold,
    setKeyboardSizeHold,
    codeShown = $bindable(),
    forceSoftKeys = $bindable(),
    quickMenu,
    upload,
    toggleDraft,
    copy,
    paste,
    showScratch,
    showFontSize,
    close,
    leave,
    showFind,
    hybrid,
    paneActions,
  }: {
    terminal: HeaderTerminal;
    fullscreen: Fullscreen;
    draft: TerminalDraft;
    draftSaved: string;
    scratchSaved: string;
    uploading: boolean;
    desktop: boolean;
    sizeLocked: boolean;
    setSizeLocked: (value: boolean) => void;
    keyboardSizeHold: boolean;
    setKeyboardSizeHold: (value: boolean) => void;
    codeShown: boolean;
    forceSoftKeys: boolean;
    quickMenu: Snippet<["top" | "bottom", ButtonSize?, ButtonVariant?]>;
    upload: (files: File[]) => Promise<void>;
    toggleDraft: () => void;
    copy: () => Promise<void>;
    paste: () => Promise<void>;
    showScratch: () => void;
    showFontSize: () => void;
    close: () => void;
    /** Leaves this session: the page returns home, a split closes its pane. */
    leave: () => void;
    showFind: () => void;
    hybrid?: HeaderHybrid;
    paneActions?: Snippet<[boolean]>;
  } = $props();
  // A rename reaches the session list first; the list title wins here so
  // the header follows it on either side.
  const shell = findShell();
  const title = $derived(
    shell?.sessions.sessions.find((session) => session.id === t.info?.id)
      ?.title ||
      t.info?.title ||
      "终端",
  );
  let fileInput = $state<{ open(): void }>();
  let openingFind = false;
  let shortcutsOpen = $state(false);
  // Off the terminal view there is no PTY: terminal-only actions are hidden;
  // fullscreen, the code panel and closing the session stay.
  const terminalShown = $derived(!hybrid || hybrid.view === "terminal");
  const codeLabel = $derived(
    codeShown
      ? desktop
        ? "折叠代码"
        : terminalShown
          ? "终端"
          : "聊天"
      : "代码",
  );
  // The chat view names the agent it talks to beside the session title.
  const chatAgent = $derived(
    hybrid?.view === "chat" ? (t.info?.agent ?? undefined) : undefined,
  );
  const connectionText = $derived(
    t.connection === "connected"
      ? "已连接"
      : t.connection === "exited"
        ? "已退出"
        : t.connection === "missing"
          ? "不存在"
          : "重连中",
  );
  const uploadDisabled = $derived(
    uploading ||
      (!draft.enabled && t.connection !== "connected") ||
      t.connection === "missing" ||
      t.exitCode !== null,
  );
  function scrollToLatest() {
    t.scrollToBottom();
    if (!desktop) codeShown = false;
  }
</script>

<header
  class="app-header terminal-header flex h-(--app-header-height) shrink-0 items-center gap-2 border-b bg-card px-1 sm:px-2"
>
  <!-- Phones open the sidebar drawer here (a home-screen app has no back
       button); wider screens return to the current mode's home. -->
  <SidebarButton class="sm:hidden" />
  <Button
    class="max-sm:hidden"
    variant="ghost"
    size="icon-sm"
    aria-label={shellUi.mode === "chat" ? "返回新聊天" : "返回会话列表"}
    title={shellUi.mode === "chat" ? "返回新聊天" : "返回会话列表"}
    onclick={leave}
  >
    <IconArrowLeft class="size-4" aria-hidden="true" />
  </Button>
  <!-- The title and the session's state read as one unit; the actions keep
       their places while the title truncates. Phones put the dot first and
       move a connection problem to a second line, so it never pushes the
       title out. -->
  <div class="flex min-w-8 flex-1 items-center gap-2">
    {#if t.info}<span
        class={`status-dot shrink-0 status-${t.info.status} sm:order-3`}
        class:status-failed={sessionStatusTone(t.info) === "destructive"}
        class:motion-safe:animate-pulse={t.connection !== "connected"}
        role="img"
        aria-label={`${sessionStatusText(t.info)} · ${connectionText}`}
        title={`${sessionStatusText(t.info)} · ${connectionText}`}

      ></span>{:else}<!-- Holds the dot's place on phones, where it leads the
           title, so the title does not shift when the session loads. -->
      <span class="shrink-0 sm:hidden" aria-hidden="true"
        ><span class="status-dot invisible"></span></span
      >{/if}
    <div class="relative flex min-w-0 flex-col sm:order-1">
      <span class="h-5 min-w-0 truncate text-sm leading-5 font-medium"
        >{title}</span
      >
      <!-- Overlaid under the title, so a status change never moves it. -->
      {#if !t.info || t.connection !== "connected"}<span
          class="absolute top-full left-0 -mt-0.5 max-w-full truncate text-xs leading-none text-muted-foreground sm:hidden"
          role="status">{connectionText}</span
        >{/if}
    </div>
    {#if chatAgent}<span
        class="inline-flex shrink-0 items-center gap-1 text-xs text-muted-foreground sm:order-2"
        role="img"
        aria-label={`Agent：${chatAgent.title}`}
        title={`Agent：${chatAgent.title}`}
        data-slot="chat-agent"
        >{#if hasLaunchIcon(chatAgent.icon)}<LaunchConfigIcon
            icon={chatAgent.icon}
            size="sm"
          /><span class="max-sm:hidden">{chatAgent.title}</span>{:else}<span
            >{chatAgent.title}</span
          >{/if}</span
      >{/if}
    {#if !t.info || t.connection !== "connected"}<span
        class="shrink-0 text-xs whitespace-nowrap text-muted-foreground max-sm:hidden sm:order-4"
        role="status">{connectionText}</span
      >{/if}
  </div>
  <!-- Actions, left to right: where the session runs, terminal actions
       (icons), the quick commands and the panes it opens (outlined), then
       the window and the menu. -->
  <SessionViewTabs
    hybrid={hybrid && hybrid.view !== "recovery"
      ? {
          side: hybrid.side,
          target: hybrid.target,
          switching: hybrid.view === "switching",
          chatUnavailable: hybrid.chatUnavailable,
          request: (to) => hybrid.request(to),
        }
      : undefined}
    code={{ shown: codeShown, set: (value) => (codeShown = value) }}
  />
  {#if terminalShown}
    <Button
      variant="ghost"
      size="icon-sm"
      class="max-sm:hidden"
      aria-label="上传文件"
      title="上传文件"
      disabled={uploadDisabled}
      onclick={() => fileInput?.open()}
      ><IconUpload class="size-4" aria-hidden="true" /></Button
    >
    <Button
      variant="ghost"
      size="icon-sm"
      class="max-sm:hidden"
      aria-label="滚动到最新位置"
      title="滚动到最新位置"
      disabled={t.restoring}
      onclick={scrollToLatest}
      ><IconArrowDown class="size-4" aria-hidden="true" /></Button
    >
  {/if}
  <UploadFileInput bind:this={fileInput} {upload} />
  {#if terminalShown}
    <div class="quick-commands-header flex items-center gap-1">
      {@render quickMenu("bottom", undefined, "ghost")}
    </div>
    <DraftToggle
      class="max-sm:hidden"
      {draft}
      {draftSaved}
      {scratchSaved}
      disabled={t.connection === "missing"}
      toggle={toggleDraft}
    />
  {/if}
  <!-- Green while the code panel is open; otherwise a plain icon button
       like its neighbours. The name tells where the toggle goes next. -->
  <Button
    variant={codeShown ? "success" : "ghost"}
    size="icon-sm"
    aria-label={codeLabel}
    title={codeLabel}
    aria-pressed={codeShown}
    onclick={() => (codeShown = !codeShown)}
    ><IconCode class="size-4" aria-hidden="true" /></Button
  >
  {#if fullscreen.offered}<Button
      variant="ghost"
      size="icon-sm"
      class="max-sm:hidden"
      aria-label={fullscreen.active ? "退出全屏" : "全屏"}
      title={fullscreen.active ? "退出全屏" : "全屏"}
      aria-pressed={fullscreen.active}
      aria-busy={fullscreen.busy}
      disabled={fullscreen.busy}
      onclick={() => {
        void fullscreen.toggle();
      }}
    >
      {#if fullscreen.active}<IconMinimize class="size-4" aria-hidden="true" />
      {:else}<IconMaximize class="size-4" aria-hidden="true" />{/if}
    </Button>{/if}
  {#if paneActions}<div class="max-sm:hidden">
      {@render paneActions(false)}
    </div>{/if}
  <DropdownMenu.Root>
    <DropdownMenu.Trigger>
      {#snippet child({ props })}<Button
          {...props}
          variant="ghost"
          size="icon-touch-sm"
          aria-label={terminalShown ? "终端菜单" : "会话菜单"}
          title={terminalShown ? "终端菜单" : "会话菜单"}>⋯</Button
        >{/snippet}
    </DropdownMenu.Trigger>
    <DropdownMenu.Content
      align="end"
      onCloseAutoFocus={(event) => {
        if (openingFind) {
          event.preventDefault();
          openingFind = false;
        }
      }}
    >
      <!-- Grouped by purpose. Below 640px the header keeps only the view
           switch and this menu, so the entries of the hidden buttons live
           here (marked sm:hidden). -->
      {#if fullscreen.offered}<DropdownMenu.Item
          class="sm:hidden"
          disabled={fullscreen.busy}
          onSelect={() => {
            void fullscreen.toggle();
          }}>{fullscreen.active ? "退出全屏" : "全屏"}</DropdownMenu.Item
        >{#if terminalShown}<DropdownMenu.Separator
            class="sm:hidden"
          />{/if}{/if}
      {#if terminalShown}
        <DropdownMenu.Group>
          <DropdownMenu.GroupHeading>会话</DropdownMenu.GroupHeading>
          <DropdownMenu.CheckboxItem
            class="sm:hidden"
            closeOnSelect
            checked={draft.enabled}
            disabled={t.connection === "missing"}
            onCheckedChange={toggleDraft}
            >草稿模式{draft.scratch.length
              ? `（${draft.scratch.length} 条暂存）`
              : ""}</DropdownMenu.CheckboxItem
          >
          <DropdownMenu.Item
            disabled={!t.canFind}
            onSelect={() => {
              openingFind = true;
              showFind();
            }}><IconSearch aria-hidden="true" />查找</DropdownMenu.Item
          >
          <DropdownMenu.Item onSelect={showScratch}
            >暂存{draft.scratch.length
              ? ` (${draft.scratch.length})`
              : ""}</DropdownMenu.Item
          >
          <DropdownMenu.Item
            class="sm:hidden"
            disabled={uploadDisabled}
            onSelect={() => fileInput?.open()}>上传文件</DropdownMenu.Item
          >
        </DropdownMenu.Group>
        <DropdownMenu.Separator />
        <DropdownMenu.Group>
          <DropdownMenu.GroupHeading>剪贴板</DropdownMenu.GroupHeading>
          <DropdownMenu.Item
            onSelect={() => {
              void copy();
            }}
            >{t.historyTruncated
              ? "复制全部缓冲（更早输出已被覆盖）"
              : "复制全部缓冲"}</DropdownMenu.Item
          >
          <DropdownMenu.Item
            onSelect={() => {
              void paste();
            }}>粘贴</DropdownMenu.Item
          >
        </DropdownMenu.Group>
        <DropdownMenu.Separator />
        <DropdownMenu.Group>
          <DropdownMenu.GroupHeading>终端</DropdownMenu.GroupHeading>
          <DropdownMenu.Item onSelect={() => t.clear()}>清屏</DropdownMenu.Item>
          <DropdownMenu.Item onSelect={() => t.resync()}
            >重新同步</DropdownMenu.Item
          >
          <DropdownMenu.Item onSelect={showFontSize}>字体大小</DropdownMenu.Item
          >
          <DropdownMenu.Item
            class="sm:hidden"
            onSelect={() => (shortcutsOpen = true)}>快捷键</DropdownMenu.Item
          >
        </DropdownMenu.Group>
        <DropdownMenu.Separator />
        <DropdownMenu.Group>
          <DropdownMenu.GroupHeading>显示</DropdownMenu.GroupHeading>
          <DropdownMenu.CheckboxItem
            checked={sizeLocked}
            onCheckedChange={(value) => setSizeLocked(value === true)}
          >
            锁定尺寸
          </DropdownMenu.CheckboxItem>
          <DropdownMenu.CheckboxItem
            checked={keyboardSizeHold}
            onCheckedChange={(value) => setKeyboardSizeHold(value === true)}
          >
            键盘弹出时保持尺寸
          </DropdownMenu.CheckboxItem>
          <DropdownMenu.CheckboxItem bind:checked={forceSoftKeys}>
            强制显示底部工具栏
          </DropdownMenu.CheckboxItem>
        </DropdownMenu.Group>
      {/if}
      {#if paneActions}<div role="presentation" class="hidden max-sm:contents">
          <DropdownMenu.Separator />
          {@render paneActions(true)}
        </div>{/if}
      <DropdownMenu.Separator />
      {@const archivable =
        t.exitCode === null &&
        !!t.info?.views.includes("chat") &&
        t.info.state === "active"}
      <DropdownMenu.Item
        variant={archivable ? "default" : "destructive"}
        disabled={!t.info}
        onSelect={close}
        >{t.exitCode !== null
          ? "移除会话"
          : archivable
            ? "停止会话并归档"
            : "关闭会话"}</DropdownMenu.Item
      >
    </DropdownMenu.Content>
  </DropdownMenu.Root>
  {#if terminalShown}<TerminalShortcuts
      bind:open={shortcutsOpen}
      class="max-sm:hidden"
    />{/if}
</header>
