<script lang="ts">
  import type { Snippet } from "svelte";
  import { Button } from "$lib/components/ui/button";
  import { Separator } from "$lib/components/ui/separator";
  import type { Modifiers } from "../input";

  let {
    send,
    modifiers,
    toggle,
    paste,
    keyboard,
    trailing,
  }: {
    send: (input: string) => void;
    modifiers: Modifiers;
    toggle: (key: keyof Modifiers) => void;
    paste: () => void;
    keyboard: () => void;
    trailing?: Snippet;
  } = $props();
  const symbols = ["/", "@"] as const;
  const keys = [
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
  const combinations = [
    ["Ctrl+C", "\x03"],
    ["Ctrl+D", "\x04"],
    ["Ctrl+Z", "\x1a"],
    ["Ctrl+L", "\x0c"],
    ["Ctrl+R", "\x12"],
  ];
  const keepFocus = (event: PointerEvent) => event.preventDefault();
</script>

<aside class="soft-keys shrink-0 border-t bg-card" aria-label="终端软键">
  <div class="soft-row">
    <Button
      variant="outline"
      size="sm"
      aria-label="唤起或收起键盘"
      onpointerdown={keepFocus}
      onclick={keyboard}>⌨</Button
    >
    <Button
      variant="outline"
      size="sm"
      onpointerdown={keepFocus}
      onclick={paste}>粘贴</Button
    >
    {#each symbols as symbol (symbol)}
      <Button
        variant="outline"
        size="sm"
        onpointerdown={keepFocus}
        onclick={() => send(symbol)}>{symbol}</Button
      >
    {/each}
    {#each keys.slice(0, 2) as [label, input] (label)}<Button
        variant="outline"
        size="sm"
        onpointerdown={keepFocus}
        onclick={() => send(input)}>{label}</Button
      >{/each}
    {#each ["ctrl", "alt"] as key (key)}
      <Button
        variant={modifiers[key as keyof Modifiers] ? "default" : "outline"}
        size="sm"
        aria-pressed={modifiers[key as keyof Modifiers]}
        onpointerdown={keepFocus}
        onclick={() => toggle(key as keyof Modifiers)}
        >{key === "ctrl" ? "Ctrl" : "Alt"}</Button
      >
    {/each}
    {#each keys.slice(2) as [label, input] (label)}<Button
        variant="outline"
        size="sm"
        onpointerdown={keepFocus}
        onclick={() => send(input)}>{label}</Button
      >{/each}
    {#each combinations as [label, input] (label)}<Button
        variant="outline"
        size="sm"
        onpointerdown={keepFocus}
        onclick={() => send(input)}>{label}</Button
      >{/each}
  </div>
  {#if trailing}
    <Separator orientation="vertical" class="mx-1.5 h-6 self-center" />
    <div class="soft-trailing">{@render trailing()}</div>
  {/if}
</aside>
