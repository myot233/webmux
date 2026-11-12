<script lang="ts">
  import IconBolt from "~icons/tabler/bolt";
  import IconCornerDownLeft from "~icons/tabler/corner-down-left";
  import IconPlus from "~icons/tabler/plus";
  import IconSettings from "~icons/tabler/settings";
  import {
    Button,
    type ButtonSize,
    type ButtonVariant,
  } from "$lib/components/ui/button";
  import * as DropdownMenu from "$lib/components/ui/dropdown-menu";
  import type { QuickCommand } from "../../contracts/schemas";
  import { commandPreview } from "../../domains/settings/quick-commands";

  let {
    commands,
    target,
    unavailable,
    disabled,
    side,
    size = "icon-sm",
    variant = "outline",
    run,
    manage,
    onCloseAutoFocus,
  }: {
    commands: QuickCommand[];
    // "chat" inserts into a hybrid session's message box, never sending.
    target: "terminal" | "draft" | "chat";
    unavailable: string;
    disabled: boolean;
    side: "top" | "bottom";
    // Match the neighbouring buttons of the toolbar that hosts the trigger.
    size?: ButtonSize;
    variant?: ButtonVariant;
    run: (cmd: QuickCommand) => void;
    manage: () => void;
    onCloseAutoFocus: (event: Event) => void;
  } = $props();
  const blocked = $derived(target === "terminal" && !!unavailable);
  const title = $derived(
    { terminal: "发送到终端", draft: "插入草稿", chat: "插入消息" }[target],
  );
</script>

<DropdownMenu.Root>
  <DropdownMenu.Trigger {disabled}>
    {#snippet child({ props })}<Button
        {...props}
        {variant}
        {size}
        aria-label="快捷指令"
        title="快捷指令"><IconBolt class="size-4" aria-hidden="true" /></Button
      >{/snippet}
  </DropdownMenu.Trigger>
  <DropdownMenu.Content
    {side}
    align="end"
    class="max-h-[min(60dvh,24rem)] w-72 max-w-[calc(100vw-1rem)]"
    {onCloseAutoFocus}
  >
    <DropdownMenu.Label variant="title">{title}</DropdownMenu.Label>
    {#if blocked}
      <p class="px-2 pb-2 text-xs text-muted-foreground" role="status">
        {unavailable}
      </p>
    {/if}
    {#each commands as cmd (cmd.id)}
      {@const preview = commandPreview(cmd)}
      {@const enter = target === "terminal" && cmd.enter}
      <DropdownMenu.Item
        class="min-h-9 any-pointer-coarse:min-h-11"
        disabled={blocked}
        aria-label={[cmd.name, preview].filter(Boolean).join("：") +
          (enter ? "（并回车）" : "")}
        onSelect={() => run(cmd)}
      >
        <span class="flex min-w-0 flex-1 flex-col gap-0.5">
          {#if cmd.name}<span class="truncate">{cmd.name}</span>{/if}
          <span
            class={["truncate font-mono", cmd.name && "text-muted-foreground"]}
            >{preview}</span
          >
        </span>
        {#if enter}<DropdownMenu.Shortcut
            ><IconCornerDownLeft aria-hidden="true" /></DropdownMenu.Shortcut
          >{/if}
      </DropdownMenu.Item>
    {:else}
      <div class="px-2 py-2 text-xs">
        <p class="font-medium">还没有快捷指令</p>
        <p class="mt-1 text-muted-foreground">
          把常用命令或提示词存起来，在这里一键发送。
        </p>
      </div>
      <DropdownMenu.Item class="any-pointer-coarse:min-h-11" onSelect={manage}
        ><IconPlus aria-hidden="true" />添加快捷指令…</DropdownMenu.Item
      >
    {/each}
    <DropdownMenu.Separator />
    <DropdownMenu.Item class="any-pointer-coarse:min-h-11" onSelect={manage}
      ><IconSettings aria-hidden="true" />管理快捷指令…</DropdownMenu.Item
    >
  </DropdownMenu.Content>
</DropdownMenu.Root>
