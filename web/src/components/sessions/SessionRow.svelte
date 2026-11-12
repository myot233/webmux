<script lang="ts">
  import { openSession } from "../../hooks/shared/shell.svelte";
  import SessionStatus from "./SessionStatus.svelte";
  import RenameSessionDialog from "./RenameSessionDialog.svelte";
  import type { SessionInfo } from "../../contracts/schemas";
  import { previewFor } from "../../domains/settings/launch-previews.svelte";
  import { selectPreview } from "../../domains/workspace/preview";
  import { useCopied } from "../../hooks/shared/copied.svelte";
  import { locationHints } from "../../domains/sessions/session-location";
  import IconChevronRight from "~icons/tabler/chevron-right";
  import IconDots from "~icons/tabler/dots";
  import IconLoader2 from "~icons/tabler/loader-2";
  import { sessionViewLabel } from "../../domains/sessions/hybrid";
  import LaunchConfigIcon, {
    hasLaunchIcon,
  } from "../settings/LaunchConfigIcon.svelte";
  import { Badge } from "$lib/components/ui/badge";
  import { Button } from "$lib/components/ui/button";
  import * as Item from "$lib/components/ui/item";
  import * as DropdownMenu from "$lib/components/ui/dropdown-menu";

  let {
    session,
    hidden = false,
    compact = false,
    now,
    groupPath,
    onclose,
  }: {
    session: SessionInfo;
    hidden?: boolean;
    compact?: boolean;
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
  // On the chat view the server's preview is the latest agent message.
  const chat = $derived(session.view === "chat" && !session.transition);
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
  let renaming = $state(false);
  const copyFeedback = useCopied({ expireFailure: true });
  const copyNotice = $derived(
    copyFeedback.copied
      ? "已复制"
      : copyFeedback.failed
        ? "复制失败，请手动选择路径"
        : "",
  );
</script>

<Item.Root
  role="listitem"
  variant={compact ? "row" : "card"}
  size="flush"
  class="relative min-h-14 min-w-0"
  data-session-id={session.id}
  {hidden}
  data-status={session.status}
>
  <a
    data-session-link
    class:pb-4={!compact && chat}
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
        openSession(session);
      }
    }}
  >
    <div
      class={compact ? "min-w-0 px-3 py-3 pr-14" : "min-w-0 px-4 pt-4 pr-14"}
    >
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
        {#if session.agent}<span
            class="inline-flex min-w-0 items-center gap-1"
            data-slot="session-agent"
            >{#if hasLaunchIcon(session.agent.icon)}<LaunchConfigIcon
                icon={session.agent.icon}
                size="xs"
              />{/if}{session.agent.title}</span
          ><span aria-hidden="true">·</span><span
            >{sessionViewLabel(session)}</span
          ><span aria-hidden="true">·</span>{/if}
        <span
          >{session.clients ? `${session.clients} 个设备已连接` : "后台会话"} ·
          {relative}</span
        >
        {#if copyNotice}<span role="status" class="ml-1.5 text-success"
            >{copyNotice}</span
          >{/if}
      </div>
      {#if !compact && session.agent?.work}
        <!-- What the agent is doing right now: the running turn's tool
             call or plan step, one line. -->
        <p
          aria-label="当前工作"
          title={session.agent.work}
          class="mt-2 flex min-w-0 items-center gap-1.5 text-xs text-foreground/80"
        >
          <IconLoader2
            class="size-3.5 shrink-0 animate-spin text-info"
            aria-hidden="true"
          />
          <span class="min-w-0 truncate">{session.agent.work}</span>
        </p>
      {/if}
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
    {#if !compact}
      {#if chat}
        {#if session.preview}
          <p
            aria-label="最新消息预览"
            class="mt-3 line-clamp-3 min-w-0 px-4 text-sm leading-6 break-words whitespace-pre-line text-foreground/85"
          >
            {session.preview}
          </p>
        {/if}
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
      {#if menuReady}<DropdownMenu.Content align="end" preventScroll={false}>
          <DropdownMenu.Item
            onSelect={() => void copyFeedback.copy(copyTarget.value)}
            >{copyTarget.label}</DropdownMenu.Item
          >
          <DropdownMenu.Item onSelect={() => (renaming = true)}
            >修改标题</DropdownMenu.Item
          >
          <DropdownMenu.Separator />
          {@const archivable =
            session.status !== "exited" &&
            session.views.includes("chat") &&
            session.state === "active"}
          <DropdownMenu.Item
            variant={archivable ? "default" : "destructive"}
            onSelect={() => onclose(session)}
          >
            {session.status === "exited"
              ? "移除会话"
              : archivable
                ? "停止会话并归档"
                : "关闭会话"}
          </DropdownMenu.Item>
        </DropdownMenu.Content>{/if}
    </DropdownMenu.Root>
  </Item.Actions>
  {#if renaming}<RenameSessionDialog
      {session}
      onclose={() => (renaming = false)}
    />{/if}
</Item.Root>

<style>
  .preview-empty {
    color: var(--muted-foreground);
    white-space: normal;
    overflow-wrap: anywhere;
  }
</style>
