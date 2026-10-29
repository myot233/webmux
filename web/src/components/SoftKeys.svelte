<script lang="ts">
  import { onDestroy, type Snippet } from "svelte";
  import IconLock from "~icons/tabler/lock";
  import { Button } from "$lib/components/ui/button";
  import { Separator } from "$lib/components/ui/separator";
  import type { Modifiers } from "../input";

  let {
    send,
    modifiers,
    locked = false,
    toggle,
    lock = () => {},
    paste,
    keyboard,
    trailing,
  }: {
    send: (input: string) => void;
    modifiers: Modifiers;
    locked?: boolean;
    toggle: (key: keyof Modifiers) => void;
    lock?: (key: "shift") => void;
    paste: () => void;
    keyboard: () => void;
    trailing?: Snippet;
  } = $props();
  const symbols = ["/", "@", "#", "!"] as const;
  const keys: Array<[string, string]> = [
    ["Esc", "\x1b"],
    ["Tab", "\t"],
    ["↑", "\x1b[A"],
    ["↓", "\x1b[B"],
    ["←", "\x1b[D"],
    ["→", "\x1b[C"],
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
  const shiftTitle = $derived(
    locked ? "已锁定，点按解除" : "点按一次性，长按锁定",
  );
  const longPressMs = 500;
  const moveTolerance = 8;
  let holdTimer: ReturnType<typeof setTimeout> | undefined;
  let holdOrigin: { x: number; y: number } | undefined;
  // A long press locks Shift; the click that follows the release must not
  // toggle it back off.
  let held = false;
  function startHold(key: keyof Modifiers, event: PointerEvent) {
    if (key !== "shift") return;
    held = false;
    clearTimeout(holdTimer);
    holdOrigin = { x: event.clientX, y: event.clientY };
    holdTimer = setTimeout(() => {
      holdTimer = undefined;
      held = true;
      lock("shift");
    }, longPressMs);
  }
  // A press that turns into a horizontal scroll of the soft-key row is not a
  // long press. Mirror the touch-selection movement tolerance.
  function moveHold(event: PointerEvent) {
    if (!holdOrigin) return;
    if (
      Math.hypot(event.clientX - holdOrigin.x, event.clientY - holdOrigin.y) >=
      moveTolerance
    )
      stopHold("shift");
  }
  function stopHold(key: keyof Modifiers) {
    if (key !== "shift") return;
    clearTimeout(holdTimer);
    holdTimer = undefined;
    holdOrigin = undefined;
  }
  onDestroy(() => clearTimeout(holdTimer));
  // iOS Safari drops the active element on pointerdown even when the event is
  // canceled if the element has a click handler, so cancel the compatibility
  // mousedown instead. Canceling pointerdown would suppress that mousedown and
  // leave the keyboard to close. Click still fires after a canceled mousedown.
  const keepFocus = (event: MouseEvent) => event.preventDefault();
</script>

{#snippet sendKey(label: string, input: string)}<Button
    variant="outline"
    size="sm"
    onmousedown={keepFocus}
    onclick={() => send(input)}>{label}</Button
  >{/snippet}

<aside class="soft-keys shrink-0 border-t bg-card" aria-label="终端软键">
  <div class="soft-row">
    <Button
      variant="outline"
      size="sm"
      aria-label="唤起或收起键盘"
      onmousedown={keepFocus}
      onclick={keyboard}>⌨</Button
    >
    <Button variant="outline" size="sm" onmousedown={keepFocus} onclick={paste}
      >粘贴</Button
    >
    {#each symbols as symbol (symbol)}
      <Button
        variant="outline"
        size="sm"
        onmousedown={keepFocus}
        onclick={() => send(symbol)}>{symbol}</Button
      >
    {/each}
    {#each keys.slice(0, 2) as [label, input] (label)}{@render sendKey(
        label,
        input,
      )}{/each}
    {#each modifierKeys as [key, label] (key)}
      <Button
        variant={modifiers[key] ? "default" : "outline"}
        size="sm"
        aria-pressed={modifiers[key]}
        title={key === "shift" ? shiftTitle : undefined}
        onmousedown={keepFocus}
        onpointerdown={(event) => startHold(key, event)}
        onpointermove={moveHold}
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
    {#each keys.slice(2) as [label, input] (label)}{@render sendKey(
        label,
        input,
      )}{/each}
    {#each combinations as [label, input] (label)}{@render sendKey(
        label,
        input,
      )}{/each}
  </div>
  {#if trailing}
    <Separator orientation="vertical" class="mx-1.5 h-6 self-center" />
    <div class="soft-trailing">{@render trailing()}</div>
  {/if}
</aside>
