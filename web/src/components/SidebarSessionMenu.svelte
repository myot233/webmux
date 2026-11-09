<script lang="ts">
  import * as DropdownMenu from "$lib/components/ui/dropdown-menu";
  import type { SessionInfo } from "../schemas";
  import { findShell, shellUi } from "../shell.svelte";
  import { insertAtSelection } from "../quick-commands";
  import { sessionReference } from "../session-reference";
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
    current = "",
    tabindex,
    disabled = false,
  }: {
    /** The row's title, naming the button. */
    label: string;
    current?: string;
    target: Target;
    tabindex?: number;
    disabled?: boolean;
  } = $props();

  const shell = findShell();
  const destination = $derived(
    shell?.sessions.sessions.find(
      (session) =>
        session.id === current &&
        session.view === "chat" &&
        session.state === "active",
    ),
  );
  function reference(session: SessionInfo) {
    if (!destination || !shell || destination.id === session.id) return;
    const draft = shell.drafts.get(destination.id);
    if (draft.submission) return;
    const start = draft.selectionStart ?? draft.text.length;
    const end = draft.selectionEnd ?? start;
    const prefix = start && !/\s/.test(draft.text[start - 1] ?? "") ? " " : "";
    const suffix =
      end < draft.text.length && !/\s/.test(draft.text[end] ?? "") ? " " : "";
    const result = insertAtSelection(
      draft.text,
      start,
      end,
      prefix + sessionReference(session.title, session.id) + suffix,
    );
    shell.drafts.edit(draft, result.text, result.caret);
    shellUi.drawerOpen = false;
  }

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
    {#if destination && destination.id !== session.id}
      <DropdownMenu.Item onSelect={() => reference(session)}
        >引用到当前聊天</DropdownMenu.Item
      >
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
