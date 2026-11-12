import { afterEach, expect, test, vi } from "vitest";
import { page, userEvent } from "vitest/browser";
import { cleanup, render } from "vitest-browser-svelte/pure";
import WorkbenchTree, {
  workbenchTree,
  type TreeNode,
} from "../../src/components/shell/WorkbenchTree.svelte";
import type { BranchOperation } from "../../src/domains/workspace/branch-actions";
import type { Repository, SessionInfo } from "../../src/contracts/schemas";
import { chat, record, repository, session, worktree } from "./shell-fixture";

afterEach(async () => {
  await cleanup();
  await userEvent.cleanup();
  localStorage.removeItem("webmux.workbenchTree");
});

const webmux = repository({
  worktrees: [
    worktree(),
    worktree({
      id: "otter",
      kind: "linked",
      branch: "wt/brisk-otter",
      path: "/repos/webmux.worktrees/brisk-otter",
    }),
  ],
});
const legacy = repository({
  id: "repo2",
  name: "legacy-api",
  availability: "unavailable",
  error: "路径不可用：/srv/legacy-api 不存在",
  worktrees: [],
});
const inMain = {
  repositoryId: "repo1",
  worktreeId: "main",
  worktreePath: "/repos/webmux",
};
const inOtter = {
  repositoryId: "repo1",
  worktreeId: "otter",
  worktreePath: "/repos/webmux.worktrees/brisk-otter",
};

function renderTree(
  sessions: SessionInfo[],
  props: Partial<{
    current: string;
    scope: string;
    canCreate: boolean;
    repositories: Repository[];
    onbranchoperation: (
      repositoryId: string,
      operation: BranchOperation,
    ) => void;
  }> = {},
) {
  const handlers = {
    onscope: vi.fn(),
    onopen: vi.fn(),
    onnewworktree: vi.fn(),
    onmanageworktrees: vi.fn(),
    onremoveworktree: vi.fn(),
    onworktreedetails: vi.fn(),
    onswitchbranch: vi.fn(),
    onrename: vi.fn(),
    onclose: vi.fn(),
    onresume: vi.fn(),
    onremove: vi.fn(),
  };
  render(WorkbenchTree, {
    repositories: props.repositories ?? [webmux, legacy],
    sessions,
    records: [],
    // Both worktrees are on the home page.
    home: { repo1: ["main", "otter"] },
    scope: props.scope ?? "",
    current: props.current ?? "",
    canCreate: props.canCreate ?? true,
    configIcons: new Map(),
    onbranchoperation: props.onbranchoperation,
    ...handlers,
  });
  return handlers;
}

const tree = () => page.getByRole("tree", { name: "仓库与会话" });
// Scope rows add their counts after a comma; session rows their state.
const item = (name: string) =>
  tree().getByRole("treeitem", {
    name: new RegExp(`^${name.replace(/[()（）]/g, "\\$&")}(，|$)`),
  });

test("repositories hold the home page's worktrees and their sessions", async () => {
  renderTree(
    [
      chat({
        id: "agent001",
        title: "修复排序",
        workspace: inMain,
        createdAt: "2026-10-28T00:00:00Z",
      }),
      session({
        id: "shell001",
        title: "pnpm dev",
        workspace: inMain,
        createdAt: "2026-10-28T01:00:00Z",
      }),
      session({
        id: "ended001",
        title: "make test",
        status: "exited",
        exitCode: 0,
        workspace: inOtter,
      }),
      session({ id: "direct01", title: "htop" }),
    ],
    { current: "ended001" },
  );
  // Level one: all, each repository, direct sessions; in that order.
  const top = tree()
    .getByRole("treeitem")
    .elements()
    .filter((el) => el.getAttribute("aria-level") === "1");
  expect(top.map((el) => el.textContent?.trim().split(/\s/)[0])).toEqual([
    "全部仓库",
    "webmux",
    "legacy-api",
    "直接会话",
  ]);
  // Worktrees and direct sessions with live sessions start open down to the
  // session rows, and the newer session comes first.
  await expect
    .element(item("make test（已结束）"))
    .toHaveAttribute("aria-current", "page");
  await expect.element(item("make test（已结束）")).toBeVisible();
  await expect.element(item("main")).toHaveAttribute("aria-expanded", "true");
  await expect
    .element(item("直接会话"))
    .toHaveAttribute("aria-expanded", "true");
  const rows = tree()
    .getByRole("treeitem", { name: /（(空闲|工作中|等待|已结束)）$/ })
    .elements();
  expect(rows.map((row) => row.getAttribute("aria-label"))).toEqual([
    "pnpm dev（空闲）",
    "修复排序（空闲）",
    "make test（已结束）",
    "htop（空闲）",
  ]);
  // The unavailable repository stays, marked, without worktrees; its menu
  // only offers the details, where it can be removed from the list.
  await expect.element(item("legacy-api（不可用）")).toBeVisible();
  expect(item("legacy-api（不可用）").element().getAttribute("title")).toBe(
    "legacy-api：路径不可用：/srv/legacy-api 不存在",
  );
  await tree().getByRole("button", { name: "仓库操作：legacy-api" }).click();
  expect(
    page
      .getByRole("menuitem")
      .elements()
      .map((el) => el.textContent?.trim()),
  ).toEqual(["仓库详情…"]);
});

test("rows navigate: scopes, worktrees and sessions", async () => {
  const live = session({
    id: "shell001",
    title: "pnpm dev",
    workspace: inMain,
  });
  const handlers = renderTree([live], { scope: "all" });
  await expect
    .element(item("全部仓库"))
    .toHaveAttribute("aria-current", "page");
  await item("webmux").click();
  expect(handlers.onscope).toHaveBeenLastCalledWith("repo1");
  await item("直接会话").click();
  expect(handlers.onscope).toHaveBeenLastCalledWith("direct");
  await item("wt/brisk-otter").click();
  expect(handlers.onscope).toHaveBeenLastCalledWith("repo1", "otter");
  await expect.element(item("main")).toHaveAttribute("aria-expanded", "true");
  await item("pnpm dev（空闲）").click();
  expect(handlers.onopen).toHaveBeenCalledWith(live);
});

test("each available repository offers new-worktree and manage-worktrees actions", async () => {
  const handlers = renderTree([]);
  await tree().getByRole("button", { name: "仓库操作：webmux" }).click();
  await page.getByRole("menuitem", { name: "新建工作树…" }).click();
  expect(handlers.onnewworktree).toHaveBeenCalledWith("repo1");

  await tree().getByRole("button", { name: "仓库操作：webmux" }).click();
  await page.getByRole("menuitem", { name: "管理工作树…" }).click();
  expect(handlers.onmanageworktrees).toHaveBeenCalledWith("repo1");

  await cleanup();
  renderTree([], { canCreate: false });
  await expect.element(item("webmux")).toBeVisible();
  expect(page.getByRole("button", { name: /仓库操作：/ }).elements()).toEqual(
    [],
  );
});

test("a linked worktree's menu offers its details, a branch switch and removal", async () => {
  const handlers = renderTree([]);
  await expect
    .element(item("webmux"))
    .toHaveAttribute("aria-expanded", "false");
  await tree().getByRole("button", { name: "展开webmux" }).click();
  await expect.element(item("wt/brisk-otter")).toBeVisible();
  await tree()
    .getByRole("button", { name: "工作树操作：wt/brisk-otter" })
    .click();
  expect(
    page
      .getByRole("menuitem")
      .elements()
      .map((el) => el.textContent?.trim()),
  ).toEqual(["工作树详情…", "切换分支…", "删除 worktree…"]);
  await page.getByRole("menuitem", { name: "删除 worktree…" }).click();
  expect(handlers.onremoveworktree).toHaveBeenCalledWith("repo1", "otter");

  await tree()
    .getByRole("button", { name: "工作树操作：wt/brisk-otter" })
    .click();
  await page.getByRole("menuitem", { name: "工作树详情…" }).click();
  expect(handlers.onworktreedetails).toHaveBeenCalledWith("repo1", "otter");

  await tree()
    .getByRole("button", { name: "工作树操作：wt/brisk-otter" })
    .click();
  await page.getByRole("menuitem", { name: "切换分支…" }).click();
  expect(handlers.onswitchbranch).toHaveBeenCalledWith("repo1", "otter");
});

test("the main worktree's menu has no removal", async () => {
  const handlers = renderTree([]);
  await tree().getByRole("button", { name: "展开webmux" }).click();
  await tree().getByRole("button", { name: "工作树操作：main" }).click();
  expect(
    page
      .getByRole("menuitem")
      .elements()
      .map((el) => el.textContent?.trim()),
  ).toEqual(["工作树详情…", "切换分支…"]);
  await page.getByRole("menuitem", { name: "工作树详情…" }).click();
  expect(handlers.onworktreedetails).toHaveBeenCalledWith("repo1", "main");
});

test("bare, missing and prunable worktrees offer no branch switch", async () => {
  renderTree([], {
    repositories: [
      repository({
        worktrees: [
          worktree({ kind: "bare", branch: null, path: "/repos/webmux.git" }),
          worktree({
            id: "otter",
            kind: "linked",
            branch: "wt/gone",
            path: "/repos/webmux.worktrees/gone",
            available: false,
            prunable: true,
          }),
        ],
      }),
    ],
  });
  await tree().getByRole("button", { name: "展开webmux" }).click();
  for (const name of ["工作树操作：Bare 仓库", "工作树操作：wt/gone"]) {
    await tree().getByRole("button", { name }).click();
    await expect
      .element(page.getByRole("menuitem", { name: "工作树详情…" }))
      .toBeVisible();
    expect(
      page.getByRole("menuitem", { name: "切换分支…" }).elements(),
    ).toEqual([]);
    await userEvent.keyboard("{Escape}");
  }
});

test("a worktree row shows commits ahead of its upstream, named as on the home page", async () => {
  renderTree([], {
    repositories: [
      repository({
        worktrees: [
          worktree({ upstream: "origin/main", ahead: 0, behind: 0 }),
          worktree({
            id: "otter",
            kind: "linked",
            branch: "wt/brisk-otter",
            path: "/repos/webmux.worktrees/brisk-otter",
            upstream: "origin/wt/brisk-otter",
            ahead: 3,
            behind: 0,
          }),
        ],
      }),
    ],
  });
  await tree().getByRole("button", { name: "展开webmux" }).click();
  await expect
    .element(tree().getByRole("img", { name: "领先上游 3 个提交" }))
    .toBeVisible();
  // The synced main worktree shows no indicator.
  expect(tree().getByRole("img").elements()).toHaveLength(1);
});

test("folded nodes keep waiting counts, and folds survive a remount", async () => {
  const waitingChat = chat({
    id: "agent001",
    title: "等权限",
    workspace: inOtter,
    status: "waiting",
    attention: {
      kind: "permission",
      source: "agent",
      since: "2026-10-28T00:00:00Z",
    },
  });
  renderTree([waitingChat]);
  await expect
    .element(item("wt/brisk-otter"))
    .toHaveAttribute("aria-expanded", "true");
  await tree().getByRole("button", { name: "收起wt/brisk-otter" }).click();
  await expect
    .element(item("wt/brisk-otter"))
    .toHaveAttribute("aria-expanded", "false");
  await expect
    .element(item("wt/brisk-otter"))
    .toHaveAccessibleName("wt/brisk-otter，1 个会话，1 个等待");
  await expect
    .element(item("webmux"))
    .toHaveAccessibleName("webmux，1 个会话，1 个等待");
  await tree().getByRole("button", { name: "收起webmux" }).click();
  await expect
    .element(item("webmux"))
    .toHaveAttribute("aria-expanded", "false");
  await expect
    .element(item("webmux"))
    .toHaveAccessibleName("webmux，1 个会话，1 个等待");
  await cleanup();
  renderTree([waitingChat]);
  await expect
    .element(item("webmux"))
    .toHaveAttribute("aria-expanded", "false");
  await tree().getByRole("button", { name: "展开webmux" }).click();
  await expect
    .element(item("wt/brisk-otter"))
    .toHaveAttribute("aria-expanded", "false");
});

test("folder folds reset when live session contents change, without auto-expanding archived or unavailable workspaces", async () => {
  const active = chat({
    id: "agent001",
    title: "主干任务",
    workspace: inMain,
  });
  renderTree([]);
  await expect
    .element(item("webmux"))
    .toHaveAttribute("aria-expanded", "false");
  expect(item("main").elements()).toEqual([]);

  // Starting a session inside a folded repository/worktree opens both down to
  // the session row so it can be clicked immediately.
  await cleanup();
  renderTree([active]);
  await expect.element(item("webmux")).toHaveAttribute("aria-expanded", "true");
  await expect.element(item("main")).toHaveAttribute("aria-expanded", "true");
  await expect.element(item("主干任务（空闲）")).toBeVisible();

  // User manually folds `main`; while `main` keeps the same session, adding a
  // session to `wt/brisk-otter` leaves `main` folded and opens `wt/brisk-otter`.
  await tree().getByRole("button", { name: "收起main" }).click();
  await expect.element(item("main")).toHaveAttribute("aria-expanded", "false");
  await cleanup();
  const otterSession = session({
    id: "shell002",
    title: "分支调试",
    workspace: inOtter,
  });
  renderTree([active, otterSession]);
  await expect.element(item("main")).toHaveAttribute("aria-expanded", "false");
  await expect
    .element(item("wt/brisk-otter"))
    .toHaveAttribute("aria-expanded", "true");
  await expect.element(item("分支调试（空闲）")).toBeVisible();

  // When all live sessions leave a folder (e.g. archived), it collapses by
  // default and never auto-expands "已不可用工作区".
  await cleanup();
  render(WorkbenchTree, {
    repositories: [webmux, legacy],
    sessions: [],
    records: [keptRecord, inMainRecord],
    home: { repo1: ["main", "otter"] },
    scope: "",
    worktree: "",
    current: "",
    canCreate: true,
    configIcons: new Map(),
    onscope: vi.fn(),
    onopen: vi.fn(),
    onnewworktree: vi.fn(),
    onhomeworktrees: vi.fn(),
    onmanageworktrees: vi.fn(),
    onrepositorydetails: vi.fn(),
    onremoveworktree: vi.fn(),
    onworktreedetails: vi.fn(),
    onrename: vi.fn(),
    onclose: vi.fn(),
    onresume: vi.fn(),
    onremove: vi.fn(),
  });
  await expect
    .element(item("webmux"))
    .toHaveAttribute("aria-expanded", "false");
  await tree().getByRole("button", { name: "展开webmux" }).click();
  await expect
    .element(item("已不可用工作区"))
    .toHaveAttribute("aria-expanded", "false");
});

test("arrow keys move through the visible rows; the action is the next tab stop", async () => {
  renderTree([
    session({ id: "shell001", title: "pnpm dev", workspace: inMain }),
  ]);
  await item("全部仓库").click();
  await userEvent.keyboard("{ArrowDown}");
  await expect.element(item("webmux")).toHaveFocus();
  await userEvent.keyboard("{ArrowDown}");
  await expect.element(item("main")).toHaveFocus();
  await expect.element(item("main")).toHaveAttribute("aria-expanded", "true");
  await userEvent.keyboard("{ArrowRight}");
  await expect.element(item("pnpm dev（空闲）")).toHaveFocus();
  await userEvent.keyboard("{ArrowLeft}");
  await expect.element(item("main")).toHaveFocus();
  await userEvent.keyboard("{ArrowLeft}");
  await expect.element(item("main")).toHaveAttribute("aria-expanded", "false");
  await userEvent.keyboard("{ArrowLeft}");
  await expect.element(item("webmux")).toHaveFocus();
  // One stop for the rows; the focused repository's action menu comes next.
  await userEvent.tab();
  await expect
    .element(tree().getByRole("button", { name: "仓库操作：webmux" }))
    .toHaveFocus();
});

// Records of a removed or released worktree: their worktree ID is not in the
// repository's Git list.
const gone = {
  repositoryId: "repo1",
  worktreeId: "gone",
  worktreePath: "/repos/webmux.worktrees/gone",
};
const keptRecord = record({
  id: "released1",
  title: "已释放的聊天",
  cwd: gone.worktreePath,
  workspace: gone,
  released: {
    id: "rel1",
    repositoryId: "repo1",
    path: gone.worktreePath,
    commit: "c".repeat(40),
    branch: "wt/gone",
    releasedAt: "2026-10-28T00:00:00Z",
  },
});
const endedRecord = record({
  id: "ended0002",
  title: "普通归档",
  cwd: gone.worktreePath,
  workspace: gone,
});
const inMainRecord = record({
  id: "inmain001",
  title: "主干归档",
  workspace: inMain,
});

const missingOf = (nodes: TreeNode[]) =>
  nodes
    .find((node) => node.key === "r:repo1")
    ?.children.find((node) => node.key === "m:repo1");

test("archived records that fit no worktree make the unavailable-workspace node; counts stay live-only", () => {
  const home = { repo1: ["main", "otter"] };
  const repositories = [webmux, legacy];
  // Records alone are enough for the node; a record in a listed worktree is not.
  const onlyRecords = missingOf(
    workbenchTree(repositories, [], home, [
      keptRecord,
      endedRecord,
      inMainRecord,
    ]),
  );
  expect(onlyRecords?.children.map((node) => node.key)).toEqual([
    "a:released1",
    "a:ended0002",
  ]);
  expect([onlyRecords?.count, onlyRecords?.waiting]).toEqual([0, 0]);
  expect(onlyRecords?.children.map((node) => node.level)).toEqual([3, 3]);
  expect(missingOf(workbenchTree(repositories, [], home, [inMainRecord]))).toBe(
    undefined,
  );
  // Live sessions come first and are the only ones counted; the totals above
  // them ignore records too.
  const nodes = workbenchTree(
    repositories,
    [session({ id: "orphan01", title: "孤儿", workspace: gone })],
    home,
    [keptRecord],
  );
  const missing = missingOf(nodes);
  expect(missing?.children.map((node) => node.key)).toEqual([
    "s:orphan01",
    "a:released1",
  ]);
  expect(missing?.count).toBe(1);
  expect(nodes.find((node) => node.key === "all")?.count).toBe(1);
  // A record is not listed beside the live session carrying its ID.
  expect(
    missingOf(
      workbenchTree(
        repositories,
        [session({ id: "released1", workspace: gone })],
        home,
        [keptRecord],
      ),
    )?.children.map((node) => node.key),
  ).toEqual(["s:released1"]);
});

function renderArchived(props: { current?: string; resuming?: string } = {}) {
  const onresume = vi.fn();
  const onremove = vi.fn();
  render(WorkbenchTree, {
    repositories: [webmux],
    sessions: [session({ id: "orphan01", title: "孤儿", workspace: gone })],
    records: [keptRecord, endedRecord],
    resuming: props.resuming ?? "",
    home: { repo1: ["main", "otter"] },
    scope: "",
    current: props.current ?? "",
    canCreate: true,
    configIcons: new Map(),
    onscope: vi.fn(),
    onopen: vi.fn(),
    onnewworktree: vi.fn(),
    onmanageworktrees: vi.fn(),
    onrename: vi.fn(),
    onclose: vi.fn(),
    onresume,
    onremove,
  });
  return { onresume, onremove };
}

test("archived rows resume on click and mark a released record", async () => {
  const { onresume } = renderArchived();
  // With only unavailable-workspace sessions/records, the repository and
  // unavailable workspace stay folded until opened.
  await expect
    .element(item("webmux"))
    .toHaveAttribute("aria-expanded", "false");
  await tree().getByRole("button", { name: "展开webmux" }).click();
  // Only the one live session counts.
  await expect
    .element(item("已不可用工作区"))
    .toHaveAttribute("aria-label", "已不可用工作区，1 个会话");
  await expect
    .element(item("已不可用工作区"))
    .toHaveAttribute("aria-expanded", "false");
  await tree().getByRole("button", { name: "展开已不可用工作区" }).click();
  const released = item("已释放的聊天（已释放空间，已结束）");
  await expect.element(released).toHaveAttribute("aria-level", "3");
  expect(released.element().getAttribute("title")).toContain(
    "已释放空间：/repos/webmux.worktrees/gone",
  );
  await released.click();
  expect(onresume).toHaveBeenLastCalledWith(keptRecord);
  await item("普通归档（已结束）").click();
  expect(onresume).toHaveBeenLastCalledWith(endedRecord);
  expect(onresume).toHaveBeenCalledTimes(2);
});

test("arrow keys reach archived rows, which offer resume and delete in their menu", async () => {
  const { onresume, onremove } = renderArchived();
  await tree().getByRole("button", { name: "展开webmux" }).click();
  await item("已不可用工作区").click();
  await userEvent.keyboard("{ArrowRight}");
  await expect
    .element(item("已不可用工作区"))
    .toHaveAttribute("aria-expanded", "true");
  await userEvent.keyboard("{ArrowRight}");
  await expect.element(item("孤儿（空闲）")).toHaveFocus();
  await userEvent.keyboard("{ArrowDown}");
  await expect
    .element(item("已释放的聊天（已释放空间，已结束）"))
    .toHaveFocus();
  await userEvent.keyboard("{Enter}");
  expect(onresume).toHaveBeenCalledWith(keptRecord);
  await userEvent.keyboard("{ArrowDown}");
  await expect.element(item("普通归档（已结束）")).toHaveFocus();
  await userEvent.keyboard("{ArrowLeft}");
  await expect.element(item("已不可用工作区")).toHaveFocus();
  // Folding keeps the roving stop on the visible node.
  await userEvent.keyboard("{ArrowLeft}");
  await expect
    .element(item("已不可用工作区"))
    .toHaveAttribute("aria-expanded", "false");
  await userEvent.keyboard("{ArrowRight}");
  await userEvent.keyboard("{ArrowRight}");
  await userEvent.keyboard("{ArrowDown}");
  await userEvent.keyboard("{ArrowDown}");
  await expect.element(item("普通归档（已结束）")).toHaveFocus();
  await page.getByRole("button", { name: "会话操作：普通归档" }).click();
  await page.getByRole("menuitem", { name: "删除记录" }).click();
  expect(onremove).toHaveBeenCalledWith(endedRecord);
});

test("the current archived record opens its path, and resuming marks the row busy", async () => {
  renderArchived({ current: "ended0002", resuming: "ended0002" });
  // The current record's page opens the node without saving it.
  await expect
    .element(item("普通归档（正在恢复）"))
    .toHaveAttribute("aria-current", "page");
  await expect
    .element(item("普通归档（正在恢复）"))
    .toHaveAttribute("aria-busy", "true");
  await expect
    .element(item("已释放的聊天（已释放空间，已结束）"))
    .toBeDisabled();
});

test("a worktree's menu offers its branch's sync actions by state", async () => {
  const onbranchoperation = vi.fn();
  const synced = { upstream: "origin/x", ahead: 0, behind: 0 };
  renderTree([], {
    onbranchoperation,
    repositories: [
      repository({
        worktrees: [
          worktree({ ...synced, upstream: "origin/main", behind: 2 }),
          worktree({
            ...synced,
            id: "otter",
            kind: "linked",
            branch: "wt/brisk-otter",
            path: "/repos/webmux.worktrees/brisk-otter",
            ahead: 1,
          }),
        ],
      }),
    ],
  });
  await tree().getByRole("button", { name: "展开webmux" }).click();
  const items = () =>
    page
      .getByRole("menuitem")
      .elements()
      .map((el) => el.textContent?.trim());
  for (const [name, expected] of [
    ["main", ["工作树详情…", "切换分支…", "快进到上游"]],
    ["wt/brisk-otter", ["工作树详情…", "切换分支…", "推送…", "删除 worktree…"]],
  ] as const) {
    await tree()
      .getByRole("button", { name: `工作树操作：${name}` })
      .click();
    await expect.element(page.getByRole("menuitem").first()).toBeVisible();
    expect(items()).toEqual(expected);
    await userEvent.keyboard("{Escape}");
    await expect.poll(() => items()).toEqual([]);
  }
  await tree().getByRole("button", { name: "工作树操作：main" }).click();
  await page.getByRole("menuitem", { name: "快进到上游" }).click();
  expect(onbranchoperation).toHaveBeenCalledWith("repo1", {
    kind: "fast-forward",
    branch: "main",
  });
});
