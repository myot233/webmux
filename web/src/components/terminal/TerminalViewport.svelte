<script lang="ts">
  import type { Snippet } from "svelte";
  import IconArrowDown from "~icons/tabler/arrow-down";
  import IconDeviceGamepad2 from "~icons/tabler/device-gamepad-2";
  import {
    Button,
    type ButtonSize,
    type ButtonVariant,
  } from "$lib/components/ui/button";
  import SoftKeys from "./SoftKeys.svelte";
  import type { Modifiers } from "../../platform/input";
  import type { UploadProgress } from "../../hooks/shared/upload.svelte";
  import type { Connection } from "../../platform/ws";
  import { hasTouchInput } from "../../platform/touch-input";

  interface ViewportTerminal {
    readonly selection: string;
    readonly restoring: boolean;
    readonly atBottom: boolean;
    readonly connection: Connection;
    readonly controlling?: boolean;
    readonly exitCode?: number | null;
    readonly modifiers: Modifiers;
    readonly shiftLocked: boolean;
    copySelection(): void;
    clearSelection(): void;
    scrollToBottom(): void;
  }
  interface ViewportUploads {
    readonly dragging: boolean;
    readonly progress: UploadProgress | null;
  }

  // The xterm host with its overlays, above the soft-key toolbar.
  let {
    container = $bindable(),
    terminal: t,
    visible,
    sizeLocked = false,
    uploads,
    quickMenu,
    draftToggle,
    send,
    toggle,
    lock,
    paste,
    keyboard,
    touchGamepad = false,
    setTouchGamepad,
    findOpen = false,
    findBar,
  }: {
    container?: HTMLDivElement;
    terminal: ViewportTerminal;
    // Whether the terminal pane is on screen (mobile may show code instead).
    visible: boolean;
    // Keeps the PTY grid fixed and lets the host scroll past its edges.
    sizeLocked?: boolean;
    uploads: ViewportUploads;
    quickMenu: Snippet<["top" | "bottom", ButtonSize?, ButtonVariant?]>;
    // Phones keep the draft switch beside the quick commands.
    draftToggle: Snippet;
    send: (input: string) => void;
    toggle: (key: keyof Modifiers) => void;
    lock: (key: "shift") => void;
    paste: () => Promise<void>;
    keyboard: () => void;
    touchGamepad?: boolean;
    setTouchGamepad: (value: boolean) => void;
    findOpen?: boolean;
    findBar?: Snippet;
  } = $props();

  const gamepadAvailable = $derived(
    t.controlling === true &&
      t.connection !== "missing" &&
      (t.exitCode === null || t.exitCode === undefined) &&
      visible,
  );
  const gamepadActive = $derived(touchGamepad && gamepadAvailable);
  const gamepadToggleLabel = $derived(gamepadActive ? "收起摇杆" : "展开摇杆");

  // Keep the hint off touch screens, including hybrids whose secondary pointer
  // is coarse, and mirror the platform's actual copy shortcut.
  const mac = /Mac/.test(navigator.platform);
  const shortcutHint =
    !hasTouchInput() && matchMedia("(pointer: fine)").matches;
</script>

{#snippet gamepadToggle()}
  <Button
    variant="ghost"
    size="icon-sm"
    aria-label={gamepadToggleLabel}
    title={gamepadToggleLabel}
    aria-expanded={gamepadActive}
    disabled={!gamepadAvailable}
    onmousedown={(event) => event.preventDefault()}
    onclick={() => setTouchGamepad(!touchGamepad)}
  >
    <IconDeviceGamepad2 aria-hidden="true" />
  </Button>
{/snippet}

<div class="terminal-column">
  <div class="terminal-area">
    <div
      class="terminal-host"
      class:size-locked={sizeLocked}
      class:restoring={t.restoring && t.connection !== "missing"}
      bind:this={container}
    ></div>
    {@render findBar?.()}
    {#if uploads.dragging}
      <div
        class="pointer-events-none absolute inset-0 z-20 flex items-center justify-center border-2 border-dashed border-primary bg-background/90"
        role="status"
      >
        <p class="text-sm">
          {uploads.progress ? "请等待当前上传完成" : "松开以上传文件到临时目录"}
        </p>
      </div>
    {/if}
    {#if t.selection && !findOpen}
      <div
        class="absolute right-2 top-2 z-10 flex items-center gap-1 rounded-md border bg-popover p-1 shadow-md"
        role="toolbar"
        aria-label="终端文字选择"
      >
        <Button
          size="sm"
          aria-keyshortcuts={mac ? "Meta+C" : "Control+C"}
          onclick={() => t.copySelection()}>复制选中</Button
        >
        {#if shortcutHint}<kbd class="border px-1" aria-hidden="true"
            >{mac ? "⌘C" : "Ctrl+C"}</kbd
          >{/if}
        <Button variant="ghost" size="sm" onclick={() => t.clearSelection()}
          >取消选择</Button
        >
      </div>
    {/if}
    {#if t.restoring && t.connection !== "missing"}<div
        class="restore-overlay"
        role="status"
      >
        {t.connection === "connecting" ? "正在连接…" : "正在恢复…"}
      </div>{:else if t.connection === "connecting" || t.connection === "reconnecting"}<div
        class="pointer-events-none absolute top-2 right-2 z-20"
      >
        <span class="text-xs text-muted-foreground" role="status"
          >正在恢复…</span
        >
      </div>{/if}
    {#if !t.atBottom && !t.restoring && visible}<Button
        variant="outline"
        size="sm"
        raised
        class="absolute right-3 bottom-3 z-10 sm:hidden"
        aria-label="回到最新"
        onmousedown={(event) => event.preventDefault()}
        onclick={() => t.scrollToBottom()}
        ><IconArrowDown aria-hidden="true" />回到最新</Button
      >{/if}
  </div>
  <SoftKeys
    {send}
    modifiers={t.modifiers}
    locked={t.shiftLocked}
    gamepad={gamepadActive}
    {gamepadToggle}
    {toggle}
    {lock}
    paste={() => {
      void paste();
    }}
    {keyboard}
  >
    {#snippet trailing()}
      {@render draftToggle()}{@render quickMenu("top", "icon-sm", "ghost")}
    {/snippet}
  </SoftKeys>
</div>
