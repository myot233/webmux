<script lang="ts">
  import { Button } from "$lib/components/ui/button";
  import { api } from "../../api";
  import type { TimelineEntry } from "../../chat-model";
  import { errorMessage } from "../../error";
  import { CreateSessionResultSchema, SessionInfoSchema } from "../../schemas";
  import { findShell, openSession } from "../../shell.svelte";
  import type { TerminalDrafts } from "../../terminal-draft.svelte";

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
  const storageKey = (id: string) => `webmux.sideChat.${id}`;
  function parentId() {
    try {
      return SessionInfoSchema.shape.id.safeParse(
        sessionStorage.getItem(storageKey(sessionId)),
      ).data;
    } catch {
      return undefined;
    }
  }
  const parent = $derived(
    shell?.sessions.sessions.find(
      (item) =>
        item.id === parentId() &&
        item.view === "chat" &&
        item.state === "active",
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

  async function start() {
    if (!source?.configId || reply?.type !== "message" || busy) return;
    const parent = sessionId;
    const text = `主会话：${source.title}\n\n上下文摘录：\n${reply.text.slice(0, 4000)}\n\n我的问题：`;
    busy = true;
    error = "";
    try {
      const { session } = await api(
        "/sessions",
        CreateSessionResultSchema,
        "POST",
        {
          configId: source.configId,
          view: "chat",
          permissionMode: "ask",
          name: `旁聊 · ${source.title.slice(0, 60)}`,
          cwd: source.cwd,
          ...(source.workspace
            ? {
                workspace: {
                  repositoryId: source.workspace.repositoryId,
                  worktreeId: source.workspace.worktreeId,
                },
              }
            : {}),
        },
      );
      try {
        sessionStorage.setItem(storageKey(session.id), parent);
      } catch {
        /* optional tab metadata */
      }
      const draft = drafts.get(session.id);
      drafts.edit(draft, text, text.length);
      openSession(session);
    } catch (err) {
      error = errorMessage(err);
    } finally {
      busy = false;
    }
  }

  function bringBack() {
    if (!parent || reply?.type !== "message") return;
    const draft = drafts.get(parent.id);
    if (draft.submission) {
      error = "主会话草稿正在提交，请稍后重试。";
      return;
    }
    const text = [draft.text, `旁聊结论：\n${reply.text.slice(0, 4000)}`]
      .filter(Boolean)
      .join("\n\n");
    drafts.edit(draft, text, text.length);
    openSession(parent);
  }
</script>

{#if parent}
  <div class="flex flex-wrap gap-2">
    <Button variant="ghost" size="xs" onclick={() => openSession(parent!)}
      >返回主会话</Button
    >
    <Button variant="outline" size="xs" disabled={!reply} onclick={bringBack}
      >把结论带回草稿</Button
    >
  </div>
{:else if source?.configId && reply}
  <Button
    variant="ghost"
    size="xs"
    disabled={busy}
    onclick={() => void start()}
  >
    {busy ? "正在打开旁聊…" : "从最新回复开启旁聊"}
  </Button>
{/if}
{#if error}<p role="alert" class="text-sm text-destructive">{error}</p>{/if}
