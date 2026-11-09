<script lang="ts">
  import { tick } from "svelte";
  import IconLoader2 from "~icons/tabler/loader-2";
  import IconSettings from "~icons/tabler/settings";
  import IconX from "~icons/tabler/x";
  import { Button } from "$lib/components/ui/button";
  import * as Drawer from "$lib/components/ui/drawer";
  import { Label } from "$lib/components/ui/label";
  import * as NativeSelect from "$lib/components/ui/native-select";
  import { Switch } from "$lib/components/ui/switch";
  import type { ConfigValue } from "../../chat-api";
  import {
    changedPermissionMode,
    permissionChoices,
  } from "../../chat-permission";
  import type { ChatConfig, ChatPermissionMode } from "../../schemas";
  import InlineNotice from "../InlineNotice.svelte";

  // Low-frequency options share one entry inside the mobile composer. The
  // temporary sheet shows names and current values without adding a bottom row.
  let {
    mode,
    config,
    disabled,
    modeDisabled,
    loading = false,
    error = "",
    onmodechange,
    onconfigchange,
  }: {
    mode: ChatPermissionMode;
    config: ChatConfig[];
    disabled: boolean;
    modeDisabled: boolean;
    loading?: boolean;
    error?: string;
    onmodechange: (mode: ChatPermissionMode) => void;
    onconfigchange: (configId: string, value: ConfigValue) => void;
  } = $props();
  let trigger = $state<HTMLButtonElement | null>(null);
  let origin = $state<DOMRectReadOnly | null>(null);

  async function restoreSelection(
    select: HTMLSelectElement,
    current: () => string,
  ) {
    // A native select paints the user's pick before its owner accepts it.
    // Read after the flush: quick-start drafts update immediately, whereas
    // live sessions keep showing their server-confirmed value.
    await tick();
    select.value = current();
  }
</script>

<Drawer.Root
  shouldScaleBackground={false}
  autoFocus
  onOpenChange={(open) => {
    if (open) origin = trigger?.getBoundingClientRect() ?? null;
  }}
>
  <Drawer.Trigger bind:ref={trigger}>
    {#snippet child({ props })}
      <Button
        {...props}
        variant="ghost"
        size="icon"
        aria-label="会话选项"
        title="会话选项"><IconSettings aria-hidden="true" /></Button
      >
    {/snippet}
  </Drawer.Trigger>
  <Drawer.Content
    motion="origin"
    {origin}
    class="data-[vaul-drawer-direction=bottom]:max-h-[calc(100dvh-1rem)]"
    onCloseAutoFocus={(event) => {
      // Safari does not focus tapped buttons. Returning to the previously
      // focused textarea would reopen the keyboard after dismissing the sheet.
      event.preventDefault();
      trigger?.focus({ preventScroll: true });
    }}
  >
    <div class="flex shrink-0 items-center justify-between gap-2 px-4 py-2">
      <Drawer.Title>会话选项</Drawer.Title>
      <Drawer.Close>
        {#snippet child({ props })}
          <Button
            {...props}
            variant="ghost"
            size="icon"
            aria-label="关闭选项"
            title="关闭选项"
          >
            <IconX aria-hidden="true" />
          </Button>
        {/snippet}
      </Drawer.Close>
    </div>
    <Drawer.Description class="sr-only"
      >查看和更改权限与 agent 配置。</Drawer.Description
    >
    {#if error}
      <div class="max-h-[30dvh] shrink-0 overflow-y-auto px-4 pb-3">
        <InlineNotice tone="danger">{error}</InlineNotice>
      </div>
    {/if}
    <div
      class="flex min-h-0 flex-col gap-3 overflow-y-auto overscroll-contain px-4 pb-safe-4"
    >
      {#if loading}
        <p
          class="flex items-center gap-1.5 text-xs text-muted-foreground"
          role="status"
        >
          <IconLoader2
            class="size-3.5 animate-spin motion-reduce:animate-none"
            aria-hidden="true"
          />正在读取配置…
        </p>
      {/if}
      <Label class="grid grid-cols-[minmax(0,1fr)_minmax(0,2fr)]">
        <span class="min-w-0 wrap-anywhere">权限</span>
        <NativeSelect.Root
          class="w-full"
          aria-label="权限"
          disabled={modeDisabled}
          bind:value={
            () => mode,
            (selected) => {
              const changed = changedPermissionMode(String(selected), mode);
              if (changed) onmodechange(changed);
            }
          }
          onchange={(event) =>
            void restoreSelection(event.currentTarget, () => mode)}
        >
          {#each permissionChoices as choice (choice.value)}
            <NativeSelect.Option value={choice.value}
              >{choice.name}</NativeSelect.Option
            >
          {/each}
        </NativeSelect.Root>
      </Label>
      {#each config as option (option.id)}
        {#if option.type === "boolean"}
          <Label class="justify-between">
            <span class="min-w-0 wrap-anywhere">{option.name}</span>
            <Switch
              aria-label={option.name}
              bind:checked={
                () => !!option.enabled,
                (enabled) => onconfigchange(option.id, { enabled })
              }
              {disabled}
            />
          </Label>
        {:else}
          <Label class="grid grid-cols-[minmax(0,1fr)_minmax(0,2fr)]">
            <span class="min-w-0 wrap-anywhere">{option.name}</span>
            <NativeSelect.Root
              class="w-full"
              title={option.options.find(
                (choice) => choice.value === option.value,
              )?.name}
              aria-label={option.name}
              {disabled}
              bind:value={
                () => option.value ?? "",
                (value) => onconfigchange(option.id, { value: String(value) })
              }
              onchange={(event) =>
                void restoreSelection(
                  event.currentTarget,
                  () => option.value ?? "",
                )}
            >
              {#each option.options as choice (choice.value)}
                <NativeSelect.Option value={choice.value}
                  >{choice.name}</NativeSelect.Option
                >
              {/each}
            </NativeSelect.Root>
          </Label>
        {/if}
      {/each}
    </div>
  </Drawer.Content>
</Drawer.Root>
