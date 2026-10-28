<script lang="ts">
  import { onDestroy } from "svelte";
  import { navigate } from "../api";
  import SessionStatus from "./SessionStatus.svelte";
  import type { SessionInfo } from "../schemas";
  import { previewFor } from "../launchPreviews.svelte";
  import { selectPreview } from "../preview";
  import { copyText } from "../clipboard";
  import { locationHints } from "../session-location";
  import IconChevronRight from "~icons/tabler/chevron-right";
  import IconDots from "~icons/tabler/dots";
  import IconMessageCircle from "~icons/tabler/message-circle";
  import IconTerminal2 from "~icons/tabler/terminal-2";
  import { hybridSideLabel } from "../hybrid";
  import { Badge } from "$lib/components/ui/badge";
  import { Button } from "$lib/components/ui/button";
  import * as Item from "$lib/components/ui/item";
  import * as DropdownMenu from "$lib/components/ui/dropdown-menu";

  let {
    session,
    hidden = false,
    now,
    groupPath,
    onclose,
  }: {
    session: SessionInfo;
    hidden?: boolean;
    now: number;
    groupPath?: string;
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
  const settings = $derived(previewFor(session.configId));
  const preview = $derived(selectPreview(session.preview, settings));
  // On the chat side the server's preview is the latest agent message.
  const chat = $derived(
    session.hybrid?.side === "acp" && !session.hybrid.phase,
  );
  const hints = $derived(locationHints(session, groupPath));
  const copyTarget = $derived(
    session.reportedCwd
      ? { label: "复制 Shell 报告目录", value: session.reportedCwd.path }
      : { label: "复制启动目录", value: session.cwd },
  );
  const reportedSince = $derived(
    session.reportedCwd
      ? new Date(session.reportedCwd.since).toLocaleString()
      : "",
  );
  // Defer floating-layer setup until this row is actually used. Keep it
  // mounted afterwards so close transitions and focus restoration stay native.
  let menuReady = $state(false);
  let menuOpen = $state(false);
  $effect(() => {
    if (hidden) menuOpen = false;
  });
  let copyNotice = $state("");
  let copyTimer: ReturnType<typeof setTimeout> | undefined;
  onDestroy(() => clearTimeout(copyTimer));
  async function copyDirectory() {
    const copied = await copyText(copyTarget.value);
    copyNotice = copied ? "已复制" : "复制失败，请手动选择路径";
    clearTimeout(copyTimer);
    copyTimer = setTimeout(() => (copyNotice = ""), 2000);
  }
</script>

<Item.Root
  role="listitem"
  variant="card"
  size="flush"
  class="relative min-h-14 min-w-0"
  data-session-id={session.id}
  {hidden}
  data-status={session.status}
>
  <a
    data-session-link
    class="group block w-full min-w-0 rounded-lg outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background"
    aria-label={session.title}
    href={`/s/${session.id}`}
    onclick={(event) => {
      // A plain click stays in this tab; modified clicks keep the browser's
      // own new-tab and new-window behaviour.
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
    <div class="min-w-0 px-4 pt-4 pr-14">
      <!-- The status follows the title, so a status change grows only to
           the right and moves nothing else; the facts below stay put. Large
           text on a narrow card moves the status under the title. -->
      <div class="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
        <Item.Title class="min-w-0 max-w-full" title={session.title}>
          <span class="truncate group-hover:underline">{session.title}</span>
          <IconChevronRight
            class="size-4 shrink-0 text-muted-foreground"
            aria-hidden="true"
          />
        </Item.Title>
        <div class="max-w-full min-w-0"><SessionStatus {session} /></div>
      </div>
      <!-- Where the session runs, who is connected and when it was active. -->
      <div
        class="mt-1.5 flex flex-wrap items-center gap-x-1.5 gap-y-1 text-xs text-muted-foreground"
      >
        {#if session.hybrid}<span class="inline-flex items-center gap-1"
            >{#if chat}<IconMessageCircle
                class="size-3.5"
                aria-hidden="true"
              />{:else}<IconTerminal2
                class="size-3.5"
                aria-hidden="true"
              />{/if}{hybridSideLabel(session.hybrid)}</span
          ><span aria-hidden="true">·</span>{/if}
        <span
          >{session.clients ? `${session.clients} 个设备已连接` : "后台会话"} ·
          {relative}</span
        >
        {#if copyNotice}<span role="status" class="ml-1.5 text-success"
            >{copyNotice}</span
          >{/if}
      </div>
      {#if hints.launch || hints.reported}
        <div class="mt-2 space-y-1 text-xs text-muted-foreground">
          {#if hints.launch}
            <p class="flex min-w-0 items-baseline gap-1.5">
              <span class="shrink-0">启动目录</span>
              <span class="min-w-0 font-mono break-all" title={hints.launch}
                >{hints.launch}</span
              >
            </p>
          {/if}
          {#if hints.reported}
            <p
              class="flex min-w-0 flex-wrap items-baseline gap-x-1.5 gap-y-1"
              title={`Shell 通过 OSC 7 于 ${reportedSince} 报告；仅作导航线索，不改变会话归属`}
            >
              <span class="shrink-0">Shell 报告目录</span>
              <span class="min-w-0 font-mono break-all"
                >{hints.reported.path}</span
              >
              {#if hints.reported.remote}<Badge variant="outline"
                  >远端 {hints.reported.host}</Badge
                >{/if}
            </p>
          {/if}
        </div>
      {/if}
    </div>
    {#if chat}
      <p
        aria-label="最新消息预览"
        class="mt-3 line-clamp-3 min-w-0 px-4 pb-4 text-sm leading-6 break-words whitespace-pre-line text-foreground/85"
      >
        {session.preview || "等待 Agent 回复…"}
      </p>
    {:else}
      <pre
        data-slot="terminal-preview"
        aria-label="终端输出预览"
        style:--preview-lines={settings.lines}
        class:preview-empty={!preview}
        class="h-preview mt-4 min-w-0 overflow-hidden px-4 pb-4 font-mono text-xs leading-5 whitespace-pre text-foreground/85">{preview ||
          (session.preview
            ? "末尾行已隐藏，暂无可显示内容"
            : "等待终端输出…")}</pre>
    {/if}
  </a>
  <Item.Actions class="absolute top-1.5 right-1.5">
    <DropdownMenu.Root
      bind:open={menuOpen}
      onOpenChange={(open) => {
        if (open) menuReady = true;
      }}
    >
      <DropdownMenu.Trigger>
        {#snippet child({ props })}
          <Button
            {...props}
            variant="ghost"
            size="icon-sm"
            class="size-9 sm:size-11"
            aria-label={`会话菜单 ${session.title}`}
            title={`会话菜单 ${session.title}`}
            ><IconDots class="size-4" aria-hidden="true" /></Button
          >
        {/snippet}
      </DropdownMenu.Trigger>
      {#if menuReady}<DropdownMenu.Content align="end">
          <DropdownMenu.Item onSelect={() => void copyDirectory()}
            >{copyTarget.label}</DropdownMenu.Item
          >
          <DropdownMenu.Separator />
          <DropdownMenu.Item
            variant="destructive"
            onSelect={() => onclose(session)}
          >
            {session.status === "exited" ? "移除会话" : "关闭会话"}
          </DropdownMenu.Item>
        </DropdownMenu.Content>{/if}
    </DropdownMenu.Root>
  </Item.Actions>
</Item.Root>

<style>
  .preview-empty {
    color: var(--muted-foreground);
    white-space: normal;
    overflow-wrap: anywhere;
  }
</style>
