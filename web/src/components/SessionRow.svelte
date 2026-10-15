<script lang="ts">
  import { navigate, statusText } from "../api";
  import type { SessionInfo } from "../schemas";
  import { previewSettings } from "../previewSettings.svelte";
  import { selectPreview } from "../preview";
  import { Button } from "$lib/components/ui/button";
  import * as Item from "$lib/components/ui/item";
  import * as DropdownMenu from "$lib/components/ui/dropdown-menu";

  let {
    session,
    now,
    onclose,
  }: {
    session: SessionInfo;
    now: number;
    onclose: (session: SessionInfo) => void;
  } = $props();
  const seconds = $derived(
    Math.max(0, Math.floor((now - Date.parse(session.lastActiveAt)) / 1000)),
  );
  const relative = $derived(
    seconds < 60
      ? "刚刚"
      : seconds < 3600
        ? `${Math.floor(seconds / 60)} 分钟前`
        : `${Math.floor(seconds / 3600)} 小时前`,
  );
  const preview = $derived(selectPreview(session.preview, previewSettings));
</script>

<Item.Root
  role="listitem"
  class="session-row relative min-h-14 min-w-0 flex-nowrap items-start"
  data-session-id={session.id}
  data-status={session.status}
>
  <a
    class="session-link group min-w-0 flex-1 outline-none after:absolute after:inset-0 after:content-[''] focus-visible:after:ring-2 focus-visible:after:ring-ring"
    aria-label={session.title}
    href={`/s/${session.id}`}
    onclick={(event) => {
      if (
        event.button === 0 &&
        !event.ctrlKey &&
        !event.metaKey &&
        !event.shiftKey &&
        !event.altKey
      ) {
        event.preventDefault();
        navigate(`/s/${session.id}`);
      }
    }}
  >
    <Item.Content class="min-w-0">
      <div
        class="flex min-w-0 flex-col gap-1 sm:flex-row sm:items-center sm:justify-between sm:gap-4"
      >
        <Item.Title class="min-w-0 max-w-full group-hover:underline">
          <span class="truncate">{session.title}</span>
          <span class="shrink-0 text-primary" aria-hidden="true">→</span>
        </Item.Title>
        <div
          class="flex flex-wrap items-center gap-x-4 gap-y-1 text-muted-foreground sm:shrink-0"
        >
          <span class="flex items-center gap-2">
            <span
              class={`status-dot status-${session.status}`}
              aria-hidden="true"
            ></span>
            <span
              >{statusText[session.status]}{session.exitCode !== null
                ? ` · code ${session.exitCode}`
                : ""}</span
            >
          </span>
          <span>
            {session.clients ? `${session.clients} 个设备已连接` : "后台会话"} · {relative}
          </span>
        </div>
      </div>
      <pre
        data-slot="terminal-preview"
        aria-label="终端输出预览"
        style:height={`calc(${previewSettings.lines}lh + 1rem)`}
        class="mt-1 min-w-0 overflow-hidden bg-muted p-2 font-mono text-xs leading-5 whitespace-pre text-foreground dark:bg-background">{preview ||
          (session.preview
            ? "末尾行已隐藏，暂无可显示内容"
            : "等待终端输出…")}</pre>
    </Item.Content>
  </a>
  <Item.Actions class="relative z-10">
    <DropdownMenu.Root>
      <DropdownMenu.Trigger>
        {#snippet child({ props })}
          <Button
            {...props}
            variant="ghost"
            size="icon-sm"
            aria-label={`会话菜单 ${session.title}`}>⋯</Button
          >
        {/snippet}
      </DropdownMenu.Trigger>
      <DropdownMenu.Content align="end">
        <DropdownMenu.Item
          variant="destructive"
          onSelect={() => onclose(session)}
        >
          {session.status === "exited" ? "移除会话" : "关闭会话"}
        </DropdownMenu.Item>
      </DropdownMenu.Content>
    </DropdownMenu.Root>
  </Item.Actions>
</Item.Root>
