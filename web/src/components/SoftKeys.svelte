<script lang="ts">
  import { onDestroy, type Snippet } from "svelte";
  import IconClipboard from "~icons/tabler/clipboard";
  import IconKeyboard from "~icons/tabler/keyboard";
  import IconLock from "~icons/tabler/lock";
  import { Button } from "$lib/components/ui/button";
  import { Separator } from "$lib/components/ui/separator";
  import { createLongPress } from "../hooks/long-press";
  import type { Modifiers } from "../input";
  import TouchGamepad from "./TouchGamepad.svelte";

  let {
    send,
    modifiers,
    locked = false,
    gamepad = false,
    toggle,
    lock = () => {},
    paste,
    keyboard,
    gamepadToggle,
    trailing,
  }: {
    send: (input: string) => void;
    modifiers: Modifiers;
    locked?: boolean;
    gamepad?: boolean;
    toggle: (key: keyof Modifiers) => void;
    lock?: (key: "shift") => void;
    paste: () => void;
    keyboard: () => void;
    gamepadToggle?: Snippet;
    trailing?: Snippet;
  } = $props();
  const symbols = ["/", "@", "#", "!"] as const;
  const leadingKeys: Array<[string, string]> = [
    ["Esc", "\x1b"],
    ["Tab", "\t"],
    ["↵", "\r"],
  ];
  const arrowKeys: Array<[string, string]> = [
    ["↑", "\x1b[A"],
    ["↓", "\x1b[B"],
    ["←", "\x1b[D"],
    ["→", "\x1b[C"],
  ];
  const navigationKeys: Array<[string, string]> = [
    ["Home", "\x1b[H"],
    ["End", "\x1b[F"],
    ["PgUp", "\x1b[5~"],
    ["PgDn", "\x1b[6~"],
  ];
  const combinations: Array<[string, string]> = [
    ["Ctrl+C", "\x03"],
    ["Ctrl+D", "\x04"],
    ["Ctrl+Z", "\x1a"],
    ["Ctrl+L", "\x0c"],
    ["Ctrl+R", "\x12"],
  ];
  const modifierKeys: Array<[keyof Modifiers, string]> = [
    ["ctrl", "Ctrl"],
    ["alt", "Alt"],
    ["shift", "Shift"],
  ];
  const visibleCombinations = $derived(
    gamepad
      ? combinations.filter(([label]) => label !== "Ctrl+C")
      : combinations,
  );
  const shiftTitle = $derived(
    locked ? "已锁定，点按解除" : "点按一次性，长按锁定",
  );
  const shiftHold = createLongPress();
  // A long press locks Shift; the click that follows the release must not
  // toggle it back off.
  let held = false;
  function startHold(key: keyof Modifiers, event: PointerEvent) {
    if (key !== "shift") return;
    held = false;
    shiftHold.start(event, () => {
      held = true;
      lock("shift");
    });
  }
  function stopHold(key: keyof Modifiers) {
    if (key === "shift") shiftHold.cancel();
  }
  onDestroy(shiftHold.cancel);
  // iOS Safari drops the active element on pointerdown even when the event is
  // canceled if the element has a click handler, so cancel the compatibility
  // mousedown instead. Canceling pointerdown would suppress that mousedown and
  // leave the keyboard to close. Click still fires after a canceled mousedown.
  const keepFocus = (event: MouseEvent) => event.preventDefault();
</script>

{#snippet sendKey(label: string, input: string)}<Button
    variant="ghost"
    size="sm"
    onmousedown={keepFocus}
    onclick={() => send(input)}>{label}</Button
  >{/snippet}

<aside class="soft-keys shrink-0 border-t bg-card" aria-label="终端软键">
  {#if gamepad}<TouchGamepad {send} />{/if}
  <div class="soft-keys-main">
    <div class="soft-row">
      <Button
        variant="ghost"
        size="sm"
        aria-label="唤起或收起键盘"
        title="唤起或收起键盘"
        onmousedown={keepFocus}
        onclick={keyboard}><IconKeyboard aria-hidden="true" /></Button
      >
      <Button
        variant="ghost"
        size="sm"
        aria-label="粘贴"
        title="粘贴"
        onmousedown={keepFocus}
        onclick={paste}><IconClipboard aria-hidden="true" /></Button
      >
      {@render gamepadToggle?.()}
      {#each symbols as symbol (symbol)}
        <Button
          variant="ghost"
          size="sm"
          onmousedown={keepFocus}
          onclick={() => send(symbol)}>{symbol}</Button
        >
      {/each}
      {#if !gamepad}
        {#each leadingKeys as [label, input] (label)}{@render sendKey(
            label,
            input,
          )}{/each}
      {/if}
      {#each modifierKeys as [key, label] (key)}
        <Button
          variant="ghost"
          size="sm"
          aria-pressed={modifiers[key]}
          title={key === "shift" ? shiftTitle : undefined}
          onmousedown={keepFocus}
          onpointerdown={(event) => startHold(key, event)}
          onpointermove={shiftHold.move}
          onpointerup={() => stopHold(key)}
          onpointercancel={() => stopHold(key)}
          onpointerleave={() => stopHold(key)}
          onclick={() => {
            if (key === "shift" && held) {
              held = false;
              return;
            }
            toggle(key);
          }}
          >{label}{#if key === "shift" && locked}<IconLock
              aria-hidden="true"
            />{/if}</Button
        >
      {/each}
      {#if !gamepad}
        {#each arrowKeys as [label, input] (label)}{@render sendKey(
            label,
            input,
          )}{/each}
      {/if}
      {#each navigationKeys as [label, input] (label)}{@render sendKey(
          label,
          input,
        )}{/each}
      {#each visibleCombinations as [label, input] (label)}{@render sendKey(
          label,
          input,
        )}{/each}
    </div>
    {#if trailing}
      <Separator orientation="vertical" class="mx-1.5 h-6 self-center" />
      <div class="soft-trailing">{@render trailing()}</div>
    {/if}
  </div>
</aside>
