import type {
  ArchivedSession,
  Branch,
  BranchBase,
  LaunchConfigSummary,
  Repository,
  SessionInfo,
  SessionView,
  Worktree,
} from "../../src/contracts/schemas";

// Data for the app shell's tests: sessions, repositories, archived records
// and launch configs as the server sends them.

export function session(patch: Partial<SessionInfo> = {}): SessionInfo {
  return {
    id: "shellzzz",
    generation: "0123456789abcdef",
    title: "bash",
    cwd: "/work",
    cmd: ["bash"],
    status: "idle",
    exitCode: null,
    listeningPorts: [],
    state: "active",
    views: ["terminal"],
    view: "terminal",
    transition: null,
    agent: null,
    preview: "",
    createdAt: "2026-10-28T00:00:00Z",
    lastActiveAt: "2026-10-28T00:00:00Z",
    cols: 80,
    rows: 24,
    clients: 0,
    workspace: null,
    attention: null,
    reportedCwd: null,
    ...patch,
  };
}

export function chat(
  patch: Partial<SessionInfo> = {},
  view: SessionView = "chat",
): SessionInfo {
  return session({
    cmd: ["claude"],
    views: ["terminal", "chat"],
    view,
    agent: {
      name: "claude",
      title: "Claude Code",
      icon: "claudecode",
      sessionId: "01a",
    },
    ...patch,
  });
}

export function worktree(patch: Partial<Worktree> = {}): Worktree {
  return {
    id: "main",
    repositoryId: "repo1",
    path: "/repos/webmux",
    kind: "main",
    branch: "main",
    head: "a".repeat(40),
    detached: false,
    locked: false,
    lockReason: "",
    prunable: false,
    available: true,
    dirty: false,
    hasUntracked: false,
    hasIgnored: false,
    error: "",
    upstream: null,
    ahead: null,
    behind: null,
    upstreamGone: false,
    lastCommit: null,
    changes: { staged: 0, unstaged: 0, untracked: 0, conflicted: 0 },
    operation: null,
    ...patch,
  };
}

export function repository(patch: Partial<Repository> = {}): Repository {
  return {
    id: "repo1",
    name: "webmux",
    commonDir: "/repos/webmux/.git",
    anchorPath: "/repos/webmux",
    availability: "available",
    error: "",
    checkedAt: "2026-10-28T00:00:00Z",
    lastFetchedAt: null,
    initInstruction: "",
    worktrees: [worktree()],
    ...patch,
  };
}

export function branch(
  name: string,
  head: string,
  worktreePaths: string[] = [],
  patch: Partial<Branch> = {},
): Branch {
  return {
    name,
    head,
    worktreePaths,
    subject: "",
    committedAt: "2026-10-28T00:00:00Z",
    upstream: null,
    ahead: null,
    behind: null,
    upstreamGone: false,
    mergedIntoDefault: false,
    ...patch,
  };
}

export function base(name: string, head: string, subject = ""): BranchBase {
  return {
    name,
    head,
    subject,
    kind: name.startsWith("refs/remotes/") ? "remote" : "local",
    committedAt: "2026-10-28T00:00:00Z",
  };
}

export function record(
  patch: Partial<ArchivedSession> = {},
  archive: Partial<ArchivedSession["archive"]> = {},
): ArchivedSession {
  const updatedAt = archive.updatedAt ?? "2026-10-27T00:00:00Z";
  return {
    ...session({
      id: "archivea",
      generation: "",
      title: "整理文档",
      configId: "cfgclaud",
      cwd: "/repos/webmux",
      cmd: ["claude"],
      status: "exited",
      cols: 0,
      rows: 0,
      createdAt: "2026-10-27T00:00:00Z",
      lastActiveAt: updatedAt,
    }),
    state: "closed",
    views: ["terminal", "chat"],
    view: "chat",
    agent: {
      name: "claude",
      title: "Claude Code",
      icon: "claudecode",
      sessionId: "01c",
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
}

export function launchConfig(
  patch: Partial<LaunchConfigSummary> = {},
): LaunchConfigSummary {
  return {
    id: "cfgclaud",
    name: "Claude Code",
    icon: "claudecode",
    cmd: ["claude"],
    envNames: [],
    chatOptions: [],
    preview: null,
    chat: true,
    chatSupport: { supported: true, supportedAgents: [], agent: "claude" },
    createdAt: "2026-10-01T00:00:00Z",
    updatedAt: "2026-10-01T00:00:00Z",
    ...patch,
  };
}
