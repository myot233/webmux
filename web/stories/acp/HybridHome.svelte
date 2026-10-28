<script lang="ts">
  import WorkspaceGroup from "../../src/components/WorkspaceGroup.svelte";
  import type { SessionInfo } from "../../src/schemas";
  import { now, session } from "../fixtures";
  import { sessionIds } from "./fixtures";

  // The home page's workspace group with hybrid sessions on each side beside
  // a plain shell: the card names the side and, on the chat side, previews
  // the latest agent message instead of terminal lines.
  let notice = $state("");
  const base = {
    ...session,
    cwd: "/home/developer/webmux",
    reportedCwd: null,
    clients: 0,
  };
  const hybrid = (
    agent: "omp" | "opencode",
    side: "acp" | "tui" | "switching",
    patch: Partial<NonNullable<SessionInfo["hybrid"]>> = {},
  ) => ({
    side,
    dormant: false,
    state: "active" as const,
    agent,
    agentSessionId: sessionIds[agent].current,
    ...patch,
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
      hybrid: hybrid("omp", "acp"),
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
      hybrid: hybrid("opencode", "tui"),
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
      hybrid: hybrid("omp", "acp", { dormant: true }),
    },
    {
      ...base,
      id: "plainshl",
      title: "bash",
      cmd: ["/bin/bash"],
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
  />
  {#if notice}<p role="status" class="mt-4 text-sm text-muted-foreground">
      {notice}
    </p>{/if}
</main>
