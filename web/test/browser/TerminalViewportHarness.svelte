<script lang="ts">
  import type { Connection } from "../../src/platform/ws";
  import TerminalViewport from "../../src/components/terminal/TerminalViewport.svelte";

  let {
    connection = "reconnecting" as Connection,
    restoring = false,
    atBottom = true,
    controlling = true,
    touchGamepad = false,
    send = () => {},
    toggle = () => {},
    setTouchGamepad = () => {},
  }: {
    connection?: Connection;
    restoring?: boolean;
    atBottom?: boolean;
    controlling?: boolean;
    touchGamepad?: boolean;
    send?: (input: string) => void;
    toggle?: (key: "ctrl" | "alt" | "shift") => void;
    setTouchGamepad?: (value: boolean) => void;
  } = $props();

  const terminal = {
    selection: "",
    get restoring() {
      return restoring;
    },
    get atBottom() {
      return atBottom;
    },
    get connection() {
      return connection;
    },
    get controlling() {
      return controlling;
    },
    exitCode: null,
    modifiers: { ctrl: false, alt: false, shift: false },
    shiftLocked: false,
    copySelection() {},
    clearSelection() {},
    scrollToBottom() {},
  };
  const uploads = { dragging: false, progress: null };
</script>

{#snippet quickMenu()}<span></span>{/snippet}
{#snippet draftToggle()}<span></span>{/snippet}

<div
  class="session-terminal force-soft-keys"
  style="position: relative; width: 400px; height: 200px"
>
  <TerminalViewport
    {terminal}
    visible
    {touchGamepad}
    {setTouchGamepad}
    {uploads}
    {quickMenu}
    {draftToggle}
    {send}
    {toggle}
    lock={() => {}}
    paste={async () => {}}
    keyboard={() => {}}
  />
</div>
