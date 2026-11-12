<script lang="ts">
  import type { TimelineEntry } from "../../domains/chat/chat-model";
  import { errorMessage } from "../../platform/error";
  import { SessionInfoSchema } from "../../contracts/schemas";
  import { sendQuickStart } from "../../hooks/sessions/quick-start";
  import { findShell, openSession } from "../../hooks/shared/shell.svelte";
  import type { TerminalDrafts } from "../../domains/terminal/terminal-draft.svelte";

  let {
    sessionId,
    entries,
    drafts,
  }: {
    sessionId: string;
    entries: readonly TimelineEntry[];
    drafts: TerminalDrafts;
  } = $props();

  const shell = findShell();
  const source = $derived(
    shell?.sessions.sessions.find((item) => item.id === sessionId),
  );
  const parentLabel = "webmux.side-chat-parent";
  const parentId = $derived(
    SessionInfoSchema.shape.id.safeParse(source?.labels?.[parentLabel]).data,
  );
  const parent = $derived(
    shell?.sessions.sessions.find(
      (item) =>
        item.id === parentId &&
        item.state === "active" &&
        item.status !== "exited",
    ),
  );
  const reply = $derived.by(() => {
    for (let i = entries.length - 1; i >= 0; i--) {
      const entry = entries[i];
      if (entry?.type === "message" && entry.role === "agent" && entry.text)
        return entry;
    }
    return undefined;
  });
  let busy = $state(false);
  let error = $state("");

  export function menu() {
    return {
      child: !!parentId,
      canStart: !!source?.configId && !!reply && !parentId && !busy,
      canReturn: !!parent,
      canBringBack: !!parent && parent.view === "chat" && !!reply,
    };
  }

  export function returnToParent() {
    if (parent) openSession(parent);
  }

  export async function start() {
    if (!source?.configId || !reply || parentId || busy) return;
    const sourceId = sessionId;
    const text = `主会话：${source.title}\n\n上下文摘录：\n${reply.text.slice(0, 4000)}\n\n我的问题：`;
    busy = true;
    error = "";
    try {
      const result = await sendQuickStart({
        configId: source.configId,
        view: "chat",
        permissionMode: "ask",
        name: `旁聊 · ${source.title.slice(0, 60)}`,
        cwd: source.cwd,
        labels: { [parentLabel]: sourceId },
        ...(source.workspace
          ? {
              workspace: {
                repositoryId: source.workspace.repositoryId,
                worktreeId: source.workspace.worktreeId,
              },
            }
          : {}),
      });
      if (!result.ok) {
        error = result.failure.error;
        return;
      }
      drafts.edit(drafts.get(result.sessionId), text, text.length);
      openSession({ id: result.sessionId });
    } catch (err) {
      error = errorMessage(err);
    } finally {
      busy = false;
    }
  }

  export function bringBack() {
    if (!parent || parent.view !== "chat" || !reply) return;
    const draft = drafts.get(parent.id);
    if (draft.submission) {
      error = "主会话草稿正在提交，请稍后重试。";
      return;
    }
    const text = [draft.text, `旁聊结论：\n${reply.text.slice(0, 4000)}`]
      .filter(Boolean)
      .join("\n\n");
    if (new TextEncoder().encode(text).length > 64 * 1024) {
      error = "主会话草稿过长，请缩短后再带回结论。";
      return;
    }
    drafts.edit(draft, text, text.length);
    openSession(parent);
  }
</script>

{#if error}<p role="alert" class="px-2 text-sm text-destructive">
    {error}
  </p>{/if}
