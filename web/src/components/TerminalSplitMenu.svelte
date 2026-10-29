<script lang="ts">
  import IconLayoutColumns from "~icons/tabler/layout-columns";
  import { Button } from "$lib/components/ui/button";
  import * as DropdownMenu from "$lib/components/ui/dropdown-menu";
  import type { SplitDirection } from "../terminal-layout";

  let {
    busy,
    limit,
    ready,
    zoomed,
    onsplit,
    onchoose,
    onclose,
    onzoom,
  }: {
    busy: boolean;
    limit: boolean;
    ready: boolean;
    zoomed: boolean;
    onsplit: (direction: SplitDirection) => void;
    onchoose: () => void;
    onclose: () => void;
    onzoom?: () => void;
  } = $props();
</script>

<DropdownMenu.Root>
  <DropdownMenu.Trigger>
    {#snippet child({ props })}
      <Button
        {...props}
        variant="ghost"
        size="icon-sm"
        aria-label="分屏"
        title="分屏（Ctrl+B）"
      >
        <IconLayoutColumns class="size-4" aria-hidden="true" />
      </Button>
    {/snippet}
  </DropdownMenu.Trigger>
  <DropdownMenu.Content align="end">
    <DropdownMenu.Item
      disabled={busy || limit || !ready}
      onSelect={() => onsplit("horizontal")}
    >
      左右分屏 · 新建 Shell <span class="ml-auto text-xs text-muted-foreground"
        >Ctrl+B %</span
      >
    </DropdownMenu.Item>
    <DropdownMenu.Item
      disabled={busy || limit || !ready}
      onSelect={() => onsplit("vertical")}
    >
      上下分屏 · 新建 Shell <span class="ml-auto text-xs text-muted-foreground"
        >Ctrl+B "</span
      >
    </DropdownMenu.Item>
    <DropdownMenu.Item disabled={busy || limit} onSelect={onchoose}
      >加入已有会话…</DropdownMenu.Item
    >
    {#if limit}<DropdownMenu.Label>已达到 8 个窗格上限</DropdownMenu.Label>{/if}
    {#if onzoom}
      <DropdownMenu.Separator />
      <DropdownMenu.Item onSelect={onzoom}
        >{zoomed ? "恢复分屏" : "放大窗格"}<span
          class="ml-auto text-xs text-muted-foreground">Ctrl+B z</span
        ></DropdownMenu.Item
      >
    {/if}
    <DropdownMenu.Separator />
    <DropdownMenu.Item disabled={busy} onSelect={onclose}
      >关闭窗格（保留会话）<span class="ml-auto text-xs text-muted-foreground"
        >Ctrl+B x</span
      ></DropdownMenu.Item
    >
  </DropdownMenu.Content>
</DropdownMenu.Root>
