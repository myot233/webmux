import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { page, userEvent } from "vitest/browser";
import { cleanup, render } from "vitest-browser-svelte/pure";
import ShellHarness from "./ShellHarness.svelte";
import { createTerminalDrafts } from "../../src/domains/terminal/terminal-draft.svelte";
import type {
  ArchivedSession,
  Repository,
  SessionInfo,
} from "../../src/contracts/schemas";

const record: ArchivedSession = {
  id: "hybrid22",
  generation: "",
  title: "修复排序",
  cwd: "/work",
  cmd: ["omp"],
  status: "exited",
  exitCode: null,
  preview: "",
  createdAt: "2026-10-28T00:00:00Z",
  lastActiveAt: "2026-10-28T00:00:00Z",
  cols: 0,
  rows: 0,
  clients: 0,
  workspace: null,
  attention: null,
  reportedCwd: null,
  configId: "cfgompaa",
  state: "closed",
  views: ["terminal", "chat"],
  view: "terminal",
  transition: null,
  agent: { name: "omp", title: "omp", icon: "pi", sessionId: "01a" },
  archive: {
    configMissing: false,
    previousAgentSessionIds: [],
    queuedCount: 0,
    interruptedAt: null,
    updatedAt: "2026-10-28T00:00:00Z",
  },
  listeningPorts: [],
};
const active: SessionInfo = {
  id: "hybrid33",
  generation: "0123456789abcdef",
  title: "进行中",
  cwd: "/work",
  cmd: ["omp"],
  status: "idle",
  exitCode: null,
  listeningPorts: [],
  state: "active",
  views: ["terminal", "chat"],
  view: "chat",
  transition: null,
  agent: { name: "omp", title: "omp", icon: "pi", sessionId: "01b" },
  preview: "",
  createdAt: "2026-10-28T00:00:00Z",
  lastActiveAt: "2026-10-28T00:00:00Z",
  cols: 80,
  rows: 24,
  clients: 0,
  workspace: null,
  attention: null,
  reportedCwd: null,
};

const worktree = {
  id: "main",
  repositoryId: "repo-1",
  path: "/repos/project",
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
} satisfies Repository["worktrees"][number];
const repository = {
  id: "repo-1",
  name: "project",
  commonDir: "/repos/project/.git",
  anchorPath: "/repos/project",
  availability: "available",
  error: "",
  checkedAt: "2026-10-28T00:00:00Z",
  lastFetchedAt: null,
  initInstruction: "",
  worktrees: [
    worktree,
    {
      ...worktree,
      id: "feature",
      path: "/repos/project-feature",
      kind: "linked",
      branch: "feature",
    },
  ],
} satisfies Repository;

let records: ArchivedSession[] = [];
let repositories: Repository[] = [];
const sockets: FakeSocket[] = [];

class FakeSocket {
  static readonly OPEN = 1;
  readyState = 0;
  bufferedAmount = 0;
  binaryType = "blob";
  onopen?: () => void;
  onmessage?: (event: { data: string }) => void;
  onclose?: (event: { code: number }) => void;
  onerror?: () => void;
  constructor(readonly url: string) {
    sockets.push(this);
  }
  send() {}
  close() {}
  open() {
    this.readyState = 1;
    this.onopen?.();
  }
  message(event: unknown) {
    this.onmessage?.({ data: JSON.stringify(event) });
  }
}

beforeEach(() => {
  records = [record];
  repositories = [];
  sockets.length = 0;
  vi.stubGlobal("WebSocket", FakeSocket);
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>(async (input, init) => {
      const url = new URL(String(input), location.href);
      const method = init?.method ?? "GET";
      if (
        url.pathname === "/api/sessions" &&
        url.searchParams.get("state") === "archived" &&
        method === "GET"
      )
        return Response.json(records);
      if (url.pathname === "/api/repositories" && method === "GET")
        return Response.json(repositories);
      if (
        url.pathname === "/api/sessions/hybrid22" &&
        url.searchParams.get("forget") === "1" &&
        method === "DELETE"
      ) {
        records = [];
        return new Response(null, { status: 204 });
      }
      // The rest of the home page degrades on its own; not under test.
      return Response.json({ error: "not found" }, { status: 404 });
    }),
  );
});

afterEach(async () => {
  try {
    await cleanup();
    await userEvent.cleanup();
  } finally {
    vi.unstubAllGlobals();
    history.replaceState(null, "", "/");
    vi.restoreAllMocks();
  }
});

function draftsWith(...ids: string[]) {
  const drafts = createTerminalDrafts();
  const entries = ids.map((id) => {
    const entry = drafts.get(id);
    drafts.setEnabled(entry, true);
    drafts.edit(entry, `draft of ${id}`);
    return entry;
  });
  return { drafts, entries };
}

async function connected() {
  await vi.waitFor(() => expect(sockets.length).toBeGreaterThan(0));
  const socket = sockets.find((s) => s.url.endsWith("/ws/sessions"))!;
  socket.open();
  return socket;
}

test("a snapshot keeps drafts of archived hybrid sessions and drops the rest", async () => {
  const {
    drafts,
    entries: [recoverable, gone],
  } = draftsWith("hybrid22", "gone0000");
  render(ShellHarness, { drafts });
  const socket = await connected();
  socket.message({ type: "snapshot", sessions: [], forwards: [] });
  await vi.waitFor(() => expect(drafts.valid(gone!)).toBe(false));
  expect(drafts.valid(recoverable!)).toBe(true);
  await expect
    .element(page.getByRole("region", { name: "work 的已归档会话" }))
    .toMatchTextContent(/已归档\s*1/);
});

test("archiving a hybrid session keeps its draft; deleting the record deletes it", async () => {
  records = [];
  const {
    drafts,
    entries: [closed],
  } = draftsWith("hybrid33");
  render(ShellHarness, { drafts });
  const socket = await connected();
  socket.message({ type: "snapshot", sessions: [active], forwards: [] });
  records = [{ ...record, id: "hybrid33", title: "进行中" }];
  socket.message({ type: "removed", id: "hybrid33" });
  const group = page.getByRole("region", { name: "work 的已归档会话" });
  await expect.element(group).toMatchTextContent(/已归档\s*1/);
  expect(drafts.valid(closed!)).toBe(true);

  await group.getByRole("button", { name: /已归档/ }).click();
  await group.getByRole("button", { name: "更多操作 进行中" }).click();
  await page.getByRole("menuitem", { name: "删除记录" }).click();
  // The fetch stub only knows how to delete hybrid22; answer for this one.
  vi.mocked(fetch).mockImplementationOnce(async () => {
    records = [];
    return new Response(null, { status: 204 });
  });
  await page
    .getByRole("alertdialog")
    .getByRole("button", { name: "确认" })
    .click();
  await vi.waitFor(() => expect(drafts.valid(closed!)).toBe(false));
});

const archived = (
  id: string,
  patch: Partial<ArchivedSession>,
): ArchivedSession => ({
  ...record,
  id,
  title: id,
  ...patch,
});

test("archived records sit in the group a live session of theirs would", async () => {
  repositories = [repository];
  records = [
    archived("recorded", {
      cwd: "/repos/project-feature",
      workspace: {
        repositoryId: "repo-1",
        worktreeId: "feature",
        worktreePath: "/repos/project-feature",
      },
    }),
    archived("legacysb", { cwd: "/repos/project/web" }),
    archived("goneworx", {
      cwd: "/repos/project-old",
      workspace: {
        repositoryId: "repo-1",
        worktreeId: "old",
        worktreePath: "/repos/project-old",
      },
    }),
    archived("elsewher", { cwd: "/srv/elsewhere" }),
  ];
  render(ShellHarness, { drafts: createTerminalDrafts() });
  const socket = await connected();
  socket.message({ type: "snapshot", sessions: [], forwards: [] });
  for (const [group, title] of [
    ["feature", "recorded"],
    ["main", "legacysb"],
    ["已不可用工作区", "goneworx"],
    ["elsewhere", "elsewher"],
  ] as const) {
    const region = page.getByRole("region", { name: `${group} 的已归档会话` });
    await expect.element(region).toMatchTextContent(/已归档\s*1/);
    await region.getByRole("button", { name: /已归档/ }).click();
    await expect
      .element(region.getByText(title, { exact: true }))
      .toBeVisible();
  }
});

test("status and search filters cover archived records", async () => {
  records = [archived("archivda", { title: "整理灯塔文档" })];
  render(ShellHarness, { drafts: createTerminalDrafts() });
  const socket = await connected();
  socket.message({
    type: "snapshot",
    sessions: [
      {
        ...active,
        id: "livesess",
        views: ["terminal"],
        view: "terminal",
        agent: null,
      },
    ],
    forwards: [],
  });
  const region = page.getByRole("region", { name: "work 的已归档会话" });
  await expect.element(region).toMatchTextContent(/已归档\s*1/);
  const filters = page.getByRole("radiogroup", { name: "按会话状态筛选" });

  await filters.getByRole("radio", { name: /运行中/ }).click();
  await expect.element(region).not.toBeInTheDocument();
  await filters.getByRole("radio", { name: /已归档/ }).click();
  // Only archived records remain, revealed without a click.
  await expect.element(region.getByText("整理灯塔文档")).toBeVisible();
  await expect
    .element(page.getByRole("main").getByText("进行中", { exact: true }))
    .not.toBeVisible();

  await filters.getByRole("radio", { name: /全部/ }).click();
  await page.getByRole("searchbox", { name: "搜索会话" }).fill("灯塔");
  await expect.element(region.getByText("整理灯塔文档")).toBeVisible();
  await page.getByRole("searchbox", { name: "搜索会话" }).fill("没有这个");
  await expect.element(region).not.toBeInTheDocument();
  // Clearing the filters brings the section back collapsed, as before.
  await page.getByRole("searchbox", { name: "搜索会话" }).click();
  await userEvent.keyboard("{Escape}");
  await expect
    .element(region.getByRole("button", { name: /已归档/ }))
    .toHaveAttribute("aria-expanded", "false");
});

test("a scope without archived matches shows the empty state", async () => {
  const other = {
    ...repository,
    id: "repo-2",
    name: "other",
    commonDir: "/repos/other/.git",
    anchorPath: "/repos/other",
    worktrees: [
      {
        ...worktree,
        repositoryId: "repo-2",
        path: "/repos/other",
      },
    ],
  } satisfies Repository;
  repositories = [repository, other];
  records = [
    archived("archivda", {
      workspace: {
        repositoryId: "repo-1",
        worktreeId: "main",
        worktreePath: "/repos/project",
      },
    }),
  ];
  render(ShellHarness, { drafts: createTerminalDrafts() });
  const socket = await connected();
  socket.message({ type: "snapshot", sessions: [], forwards: [] });
  await page.getByRole("treeitem", { name: "other", exact: true }).click();
  await page
    .getByRole("radiogroup", { name: "按会话状态筛选" })
    .getByRole("radio", { name: /已归档/ })
    .click();
  await expect.element(page.getByText("没有匹配的会话")).toBeVisible();
});

test("returning home after archiving leaves the archived section collapsed", async () => {
  history.replaceState(null, "", "/");
  render(ShellHarness, { drafts: createTerminalDrafts() });
  const socket = await connected();
  socket.message({ type: "snapshot", sessions: [], forwards: [] });
  const region = page.getByRole("region", { name: "work 的已归档会话" });
  await expect.element(region.getByText("修复排序")).not.toBeVisible();
  expect(page.getByText("刚归档").elements()).toEqual([]);
});
