<script lang="ts">
  import { api, APIError } from "../../platform/api";
  import { NoContentSchema, type SessionInfo } from "../../contracts/schemas";
  import ConfirmDialog from "../shared/ConfirmDialog.svelte";

  // Ends a session. A live hybrid session is archived instead: its agent
  // stops, the conversation stays with the agent and the record can be
  // resumed from its worktree's archived section. A plain terminal closed
  // by the user disappears instead of lingering as an exited record.
  let {
    session,
    onclose,
    onremoved,
    onarchived,
  }: {
    session: SessionInfo;
    onclose: () => void;
    onremoved?: () => void;
    onarchived?: (session: SessionInfo) => void;
  } = $props();
  let open = $state(true);
  const removing = $derived(session.status === "exited");
  const plain = $derived(!removing && !session.views.includes("chat"));
  const archiving = $derived(
    !removing && session.views.includes("chat") && session.state === "active",
  );

  // A closed shell reports its exit a moment after the request; removal is
  // refused (409) until then.
  const REMOVE_ATTEMPTS = 25;
  const REMOVE_INTERVAL_MS = 200;

  async function removeWhenExited() {
    for (let attempt = 1; ; attempt++) {
      try {
        await api(
          `/sessions/${session.id}?remove=1`,
          NoContentSchema,
          "DELETE",
        );
        return;
      } catch (err) {
        const running = err instanceof APIError && err.status === 409;
        if (!running || attempt >= REMOVE_ATTEMPTS) throw err;
        await new Promise((resolve) => setTimeout(resolve, REMOVE_INTERVAL_MS));
      }
    }
  }

  async function submit() {
    await api(
      `/sessions/${session.id}${removing ? "?remove=1" : ""}`,
      NoContentSchema,
      "DELETE",
    );
    // A plain terminal closed on purpose is not kept as an exited record.
    if (plain) await removeWhenExited();
    if (removing || plain) onremoved?.();
    // The user ended it on purpose: a page showing it leaves rather than
    // offering to resume or showing an exit banner.
    if (archiving) onarchived?.(session);
  }
</script>

<ConfirmDialog
  bind:open={
    () => open,
    (value) => {
      open = value;
      if (!value) onclose();
    }
  }
  title={`${removing ? "移除" : archiving ? "停止并归档" : "关闭"}“${session.title}”？`}
  confirmLabel={removing ? "确认移除" : archiving ? "停止并归档" : "确认关闭"}
  destructive={!archiving}
  onconfirm={submit}
>
  {removing
    ? "会话及其缓冲将永久移除。"
    : archiving
      ? "agent 进程将停止，对话保留在 agent 中。归档后可在所属工作树的“已归档”中恢复，沿用归档前的聊天或终端模式。"
      : "正在运行的进程及子进程将终止，会话随即移除。"}
</ConfirmDialog>
