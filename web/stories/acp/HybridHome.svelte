<script lang="ts">
  import WorkspaceGroup from "../../src/components/workspace/WorkspaceGroup.svelte";
  import type { ArchiveActions } from "../../src/domains/sessions/archived";
  import type {
    ArchivedSession,
    SessionInfo,
    SessionView,
  } from "../../src/contracts/schemas";
  import { now, session } from "../fixtures";
  import { agentMetadata, sessionIds } from "./fixtures";

  // The home page's workspace group with hybrid sessions on each side beside
  // a plain shell: the card names the side and, on the chat side, previews
  // the latest agent message instead of terminal lines. A running turn
  // shows its live one-line work detail above the preview. With `archived`,
  // the group also lists archived records, the first one just archived.
  let { archived = false }: { archived?: boolean } = $props();
  let notice = $state("");
  const record = (
    id: string,
    title: string,
    patch: Partial<ArchivedSession> = {},
    archive: Partial<ArchivedSession["archive"]> = {},
  ): ArchivedSession => {
    const updatedAt = archive.updatedAt ?? "2026-01-01T11:58:00Z";
    return {
      ...session,
      id,
      generation: "",
      title,
      configId: "cfgompaa",
      cwd: "/home/developer/webmux",
      cmd: ["omp"],
      status: "exited",
      exitCode: null,
      preview: "",
      cols: 0,
      rows: 0,
      clients: 0,
      attention: null,
      reportedCwd: null,
      workspace: null,
      createdAt: "2026-01-01T09:00:00Z",
      lastActiveAt: updatedAt,
      listeningPorts: [],
      state: "closed",
      views: ["terminal", "chat"],
      view: "chat",
      transition: null,
      agent: {
        name: "omp",
        title: agentMetadata.omp.title,
        icon: agentMetadata.omp.icon,
        sessionId: sessionIds.omp.current,
      },
      archive: {
        configMissing: false,
        previousAgentSessionIds: [],
        queuedCount: 0,
        interruptedAt: null,
        ...archive,
        updatedAt,
      },
      ...patch,
    };
  };
  const records: ArchivedSession[] = [
    record(
      "archivda",
      "omp · 修复首页排序后补充回归测试与文档说明，确认长标题会截断",
    ),
    record(
      "archivdb",
      "OpenCode · 升级依赖",
      {
        agent: {
          name: "opencode",
          title: agentMetadata.opencode.title,
          icon: agentMetadata.opencode.icon,
          sessionId: sessionIds.opencode.current,
        },
        cmd: ["opencode"],
        view: "terminal",
        state: "interrupted",
      },
      {
        activity: "turn",
        queuedCount: 2,
        interruptedAt: "2026-01-01T10:00:00Z",
        updatedAt: "2026-01-01T10:00:00Z",
      },
    ),
    record(
      "archivdc",
      "omp · web 子目录的样式调整",
      { cwd: "/home/developer/webmux/web" },
      { updatedAt: "2025-12-30T12:00:00Z" },
    ),
  ];
  const archive: ArchiveActions = {
    now,
    filtering: false,
    resume: async () => {
      notice = "此预览使用示例数据，未恢复真实会话。";
      return new Promise(() => {});
    },
    remove: async () => {
      notice = "此预览使用示例数据，未删除真实记录。";
    },
    onresumed: () => {},
  };
  const base = {
    ...session,
    cwd: "/home/developer/webmux",
    reportedCwd: null,
    clients: 0,
  };
  // The fields of a session with a chat view, on view `view`.
  const hybrid = (
    agent: "omp" | "opencode",
    view: SessionView,
    work?: string,
  ): Pick<SessionInfo, "views" | "view" | "agent"> => ({
    views: ["terminal", "chat"],
    view,
    agent: {
      name: agent,
      title: agentMetadata[agent].title,
      icon: agentMetadata[agent].icon,
      sessionId: sessionIds[agent].current,
      ...(work ? { work } : {}),
    },
  });
  const sessions: SessionInfo[] = [
    {
      ...base,
      id: "hybridac",
      generation: "",
      title: "omp · 修复首页排序",
      cmd: ["omp"],
      status: "waiting",
      attention: {
        kind: "permission",
        source: "agent",
        since: "2026-01-01T11:59:00Z",
      },
      preview:
        "排序只比较了最近活动时间，没有考虑置顶。我先补测试，再修改 sortWorkspaces。需要授权运行 pnpm --dir web exec vitest。",
      ...hybrid("omp", "chat", "pnpm --dir web exec vitest"),
    },
    {
      ...base,
      id: "hybridwk",
      generation: "",
      title: "OpenCode · 排查工具调用",
      cmd: ["opencode"],
      status: "running",
      attention: null,
      preview: "",
      ...hybrid("opencode", "chat", "读取 internal/hybrid/work.go"),
    },
    {
      ...base,
      id: "hybridtu",
      title: "OpenCode · 更新 README",
      cmd: ["opencode"],
      status: "running",
      attention: null,
      preview: [
        "› 继续：更新 README 中的排序说明",
        "",
        "  ✓ Read README.md",
        "  ● Editing README.md …",
      ].join("\n"),
      ...hybrid("opencode", "terminal"),
    },
    {
      ...base,
      id: "hybridsl",
      generation: "",
      title: "omp · 整理测试",
      cmd: ["omp"],
      status: "idle",
      attention: null,
      preview: "测试已整理完毕，全部通过。",
      ...hybrid("omp", "chat"),
    },
    {
      ...base,
      id: "plainshl",
      title: "bash",
      cmd: ["/usr/bin/env bash"],
      status: "idle",
      attention: null,
      preview: "$ git status\nOn branch main\nnothing to commit",
    },
  ];
</script>

<main class="mx-auto max-w-5xl px-4 py-8 sm:px-8 sm:py-12">
  <WorkspaceGroup
    title="webmux"
    path="/home/developer/webmux"
    {sessions}
    {now}
    onclose={() => (notice = "此预览使用示例数据，未关闭真实会话。")}
    archived={archived ? records : []}
    {archive}
  />
  {#if notice}<p role="status" class="mt-4 text-sm text-muted-foreground">
      {notice}
    </p>{/if}
</main>
