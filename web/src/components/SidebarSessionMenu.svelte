<script lang="ts">
  import * as DropdownMenu from "$lib/components/ui/dropdown-menu";
  import type { SessionInfo } from "../schemas";
  import { sessionPins } from "../session-pins.svelte";
  import { findShell } from "../shell.svelte";
  import SidebarRowMenu from "./SidebarRowMenu.svelte";

  // The "⋯" button over the end of a sidebar session row and its menu: the
  // row's actions, the same in the chat history and the workbench tree.
  type Target =
    | {
        kind: "live";
        session: SessionInfo;
        onrename: (session: SessionInfo) => void;
        onclose: (session: SessionInfo) => void;
      }
    | {
        kind: "archived";
        onresume: () => void;
        /** Offered only while the record's worktree can be released. */
        onrelease?: () => void;
        /** Deletes only the record. */
        onremove: () => void;
        /**
         * Offered while the record's linked worktree can be removed; deletes
         * the worktree and this record after the worktree confirmation.
         */
        onremoveworktree?: () => void;
      };
  let {
    label,
    target,
    tabindex,
    disabled = false,
  }: {
    /** The row's title, naming the button. */
    label: string;
    target: Target;
    tabindex?: number;
    disabled?: boolean;
  } = $props();

  const shell = findShell();
  const live = $derived(shell?.sessions.sessions ?? []);
  const closeLabel = $derived.by(() => {
    if (target.kind !== "live") return "";
    const { session } = target;
    if (session.status === "exited") return "移除会话";
    return session.views.includes("chat") && session.state === "active"
      ? "停止会话并归档"
      : "关闭会话";
  });
</script>

<SidebarRowMenu ariaLabel={`会话操作：${label}`} {tabindex} {disabled}>
  {#if target.kind === "live"}
    {@const { session, onrename, onclose } = target}
    {#if session.status !== "exited"}
      {@const pinned = sessionPins.has(session.id)}
      <DropdownMenu.Item
        disabled={!sessionPins.ready || (!pinned && sessionPins.full)}
        onSelect={() => sessionPins.toggle(session.id)}
      >
        {pinned ? "取消置顶" : "置顶会话"}
      </DropdownMenu.Item>
      {#if pinned}
        <DropdownMenu.Item
          disabled={!sessionPins.ready ||
            !sessionPins.canMove(session.id, -1, live)}
          onSelect={() => sessionPins.move(session.id, -1, live)}
          >上移置顶</DropdownMenu.Item
        >
        <DropdownMenu.Item
          disabled={!sessionPins.ready ||
            !sessionPins.canMove(session.id, 1, live)}
          onSelect={() => sessionPins.move(session.id, 1, live)}
          >下移置顶</DropdownMenu.Item
        >
      {/if}
    {/if}
    <DropdownMenu.Item onSelect={() => onrename(session)}
      >修改标题</DropdownMenu.Item
    >
    <DropdownMenu.Separator />
    <DropdownMenu.Item
      variant={closeLabel === "停止会话并归档" ? "default" : "destructive"}
      onSelect={() => onclose(session)}>{closeLabel}</DropdownMenu.Item
    >
  {:else}
    <DropdownMenu.Item onSelect={target.onresume}>恢复聊天</DropdownMenu.Item>
    {#if target.onrelease}<DropdownMenu.Item onSelect={target.onrelease}
        >释放空间</DropdownMenu.Item
      >{/if}
    <DropdownMenu.Separator />
    <DropdownMenu.Item variant="destructive" onSelect={target.onremove}
      >删除记录</DropdownMenu.Item
    >
    {#if target.onremoveworktree}<DropdownMenu.Item
        variant="destructive"
        onSelect={target.onremoveworktree}>删除记录和工作树</DropdownMenu.Item
      >{/if}
  {/if}
</SidebarRowMenu>
