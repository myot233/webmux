import { afterEach, expect, test, vi } from "vitest";
import { page, userEvent } from "vitest/browser";
import { cleanup, render } from "vitest-browser-svelte/pure";
import HomeWorktreeDialog from "../../src/components/workspace/HomeWorktreeDialog.svelte";
import RepositoryPanel, {
  type RepositorySource,
} from "../../src/components/workspace/RepositoryPanel.svelte";
import type { SwitchSource } from "../../src/components/workspace/SwitchBranch.svelte";
import type {
  ArchivedSession,
  Branches,
  FetchRemotesResponse,
  PushCheck,
  Remote,
  Repository,
  SessionInfo,
  SwitchPreview,
  Worktree,
} from "../../src/contracts/schemas";
import type { RepositoryTab } from "../../src/hooks/shared/shell.svelte";
import {
  base,
  branch,
  chat,
  record,
  repository,
  session,
  worktree,
} from "./shell-fixture";

afterEach(async () => {
  try {
    await cleanup();
    await userEvent.cleanup();
  } finally {
    vi.unstubAllGlobals();
  }
});

const mainTree = worktree({
  id: "main",
  repositoryId: "repo1",
  kind: "main",
  branch: "main",
  path: "/repos/webmux",
  upstream: "origin/main",
  ahead: 0,
  behind: 0,
});

const unpinnedClean = worktree({
  id: "clean1",
  repositoryId: "repo1",
  kind: "linked",
  branch: "wt/clean-feature",
  path: "/repos/webmux.worktrees/clean-feature",
  head: "a".repeat(40),
  lastCommit: {
    hash: "a".repeat(40),
    subject: "整理清理流程",
    author: "dev",
    committedAt: "2026-10-27T00:00:00Z",
  },
});

const prunableTree = worktree({
  id: "gone1",
  repositoryId: "repo1",
  kind: "linked",
  branch: "wt/stale-reg",
  path: "/repos/webmux.worktrees/stale-reg",
  head: "b".repeat(40),
  prunable: true,
  available: false,
  dirty: null,
  changes: null,
});

const blockedTree = worktree({
  id: "dirty1",
  repositoryId: "repo1",
  kind: "linked",
  branch: "wt/wip-agent",
  path: "/repos/webmux.worktrees/wip-agent",
  head: "c".repeat(40),
  dirty: true,
  upstream: "origin/wt/wip-agent",
  ahead: 2,
  behind: 1,
  changes: { staged: 1, unstaged: 2, untracked: 0, conflicted: 0 },
  operation: "rebase",
});

const lockedTree = worktree({
  id: "lock1",
  repositoryId: "repo1",
  kind: "linked",
  branch: "wt/locked-prod",
  path: "/repos/webmux.worktrees/locked-prod",
  locked: true,
  lockReason: "prod",
});

const repo = repository({
  id: "repo1",
  name: "webmux",
  lastFetchedAt: "2026-10-27T00:00:00Z",
  worktrees: [mainTree, unpinnedClean, prunableTree, blockedTree, lockedTree],
});

const liveChat = chat({
  id: "livechat",
  title: "实现批量清理",
  cwd: blockedTree.path,
  workspace: {
    repositoryId: "repo1",
    worktreeId: blockedTree.id,
    worktreePath: blockedTree.path,
  },
});

const keptArchive = record({
  id: "keptabcd",
  title: "归档讨论",
  cwd: prunableTree.path,
});

const noRemote: PushCheck = { state: "noRemote", branches: [], more: 0 };

const origin: Remote = {
  name: "origin",
  fetchUrl: "https://user:***@github.com/o/r.git",
  pushUrl: "https://user:***@github.com/o/r.git",
};

function source(
  patch: Partial<{
    remotes: Remote[];
    branches: Branches;
    fetch: RepositorySource["fetch"];
  }> = {},
): RepositorySource {
  return {
    remotes: vi.fn(async () => ({ remotes: patch.remotes ?? [origin] })),
    branches: vi.fn(async () =>
      patch.branches
        ? patch.branches
        : {
            branches: [],
            bases: [],
            defaultBase: null,
            defaultBaseSubject: "",
            defaultBranch: "main",
            defaultParent: "/repos/webmux.worktrees",
          },
    ),
    fetch:
      patch.fetch ??
      vi.fn(async (): Promise<FetchRemotesResponse> => ({
        results: [{ remote: "origin", status: "ok", message: "" }],
        repository: repo,
      })),
  };
}

function renderPanel(
  props: Partial<{
    repository: Repository;
    sessions: SessionInfo[];
    records: ArchivedSession[];
    initialTab: RepositoryTab;
    refreshing: boolean;
    refreshError: string;
    source: RepositorySource;
    switchSource: SwitchSource;
  }> = {},
) {
  const handlers = {
    onunregister: vi.fn(),
    onchange: vi.fn(),
    onrefresh: vi.fn(),
    onnewworktree: vi.fn(),
    onrepository: vi.fn(),
    onremoved: vi.fn(),
    onclose: vi.fn(),
  };
  render(RepositoryPanel, {
    repository: repo,
    sessions: [],
    records: [],
    home: {}, // Only main is pinned
    refreshing: false,
    refreshError: "",
    registryReady: true,
    source: source(),
    ...props,
    ...handlers,
  });
  return handlers;
}

const dialog = () => page.getByRole("dialog", { name: "webmux" });
const tab = (name: string) => dialog().getByRole("tab", { name });

test("the overview lists the repository's facts and its removal", async () => {
  const handlers = renderPanel({
    sessions: [
      session({
        id: "run00001",
        workspace: {
          repositoryId: "repo1",
          worktreeId: "main",
          worktreePath: "/repos/webmux",
        },
      }),
    ],
  });
  await expect.element(tab("概览")).toHaveAttribute("aria-selected", "true");
  for (const name of ["工作树", "分支", "远程"])
    await expect.element(tab(name)).toBeVisible();
  const overview = dialog().getByRole("tabpanel");
  // Rows are terms and definitions; compare their text without spacing.
  const text = () => (overview.element().textContent ?? "").replace(/\s+/g, "");
  await expect.element(overview).toMatchTextContent(/远程\s*1 个/);
  expect(text()).toMatch(
    /路径\/repos\/webmux.*公共目录\/repos\/webmux\/\.git.*默认分支main.*工作区主页1\/共5.*远程1个.*上次获取.*（近似）.*会话.*Git读取于/,
  );
  await overview.getByRole("button", { name: "从列表移除仓库…" }).click();
  expect(handlers.onunregister).toHaveBeenCalledOnce();
});

test("a never-fetched repository shows the last fetch as unknown", async () => {
  renderPanel({ repository: { ...repo, lastFetchedAt: null } });
  await expect
    .element(dialog().getByRole("tabpanel"))
    .toMatchTextContent(/上次获取\s*未知（近似）/);
});

test("an unavailable repository offers only its overview", async () => {
  const remotes = source();
  renderPanel({
    repository: repository({
      availability: "unavailable",
      error: "路径不可用：/srv/webmux 不存在",
      worktrees: [],
    }),
    source: remotes,
  });
  await expect.element(dialog()).toMatchTextContent(/路径不可用/);
  expect(dialog().getByRole("tablist").elements()).toEqual([]);
  await expect
    .element(dialog().getByRole("button", { name: "从列表移除仓库…" }))
    .toBeVisible();
  expect(remotes.remotes).not.toHaveBeenCalled();
});

test("the last snapshot stays while Git is read again, and a failed read says so", async () => {
  renderPanel({ refreshing: true });
  await expect.element(dialog().getByText(/正在重新读取状态…/)).toBeVisible();
  await cleanup();
  renderPanel({ refreshError: "git 超时" });
  await expect
    .element(dialog().getByRole("alert"))
    .toMatchTextContent(/重新读取失败：git 超时.*可能已过时/);
});

test("browsing a worktree opens its details inside the panel; 返回 keeps the search and scroll", async () => {
  await page.viewport(1024, 560);
  renderPanel({ initialTab: "worktrees" });
  await expect.element(tab("工作树")).toHaveAttribute("aria-selected", "true");
  await dialog().getByRole("searchbox", { name: "搜索工作树" }).fill("wt/");
  const list = dialog().getByRole("list", { name: "仓库工作树列表" });
  await expect.element(list).toBeVisible();
  // The dialog body is the one scroll container; the list grows inside it.
  const rows = dialog().element();
  rows.scrollTop = 60;
  const scrolled = rows.scrollTop;
  expect(scrolled).toBeGreaterThan(0);

  // The row shows the same sync facts as its details.
  const row = list.getByRole("button", { name: /^wt\/wip-agent/ });
  await expect.element(row).toMatchTextContent(/正在变基/);
  await expect
    .element(row.getByRole("img", { name: "领先上游 2 个提交" }))
    .toBeVisible();
  // A DOM click: the pointer's scroll-into-view would move the list.
  row.element().click();

  const details = dialog().getByRole("region", {
    name: "工作树详情：wt/wip-agent",
  });
  await expect.element(details).toBeVisible();
  await expect.element(details).toMatchTextContent(/origin\/wt\/wip-agent/);
  await expect.element(details).toMatchTextContent(/正在变基/);
  // One dialog: the details replace the tabs instead of stacking.
  expect(page.getByRole("dialog").elements()).toHaveLength(1);
  expect(dialog().getByRole("tablist").elements()).toEqual([]);

  await details.getByRole("button", { name: "返回" }).click();
  await expect
    .element(dialog().getByRole("searchbox", { name: "搜索工作树" }))
    .toHaveValue("wt/");
  await expect.poll(() => rows.scrollTop).toBe(scrolled);
});

test("a worktree row's menu offers details, a branch switch and branch actions but no removal", async () => {
  renderPanel({ initialTab: "worktrees" });
  const items = () =>
    page
      .getByRole("menuitem")
      .elements()
      .map((el) => el.textContent?.trim());
  await dialog()
    .getByRole("button", { name: "工作树操作：wt/stale-reg" })
    .click();
  // A prunable worktree has no checkout to switch.
  await expect.poll(items).toEqual(["工作树详情…"]);
  await userEvent.keyboard("{Escape}");
  // The first menu animates out; its items stay in the DOM until it is gone.
  await expect.poll(items).toEqual([]);
  await dialog()
    .getByRole("button", { name: "工作树操作：wt/clean-feature" })
    .click();
  // Without an upstream the branch can also be published.
  await expect.poll(items).toEqual(["工作树详情…", "切换分支…", "发布分支…"]);
  await userEvent.keyboard("{Escape}");
  await expect.poll(items).toEqual([]);
  // A diverged or synced branch offers no sync action.
  for (const name of ["main", "wt/wip-agent"]) {
    await dialog()
      .getByRole("button", { name: `工作树操作：${name}` })
      .click();
    await expect.poll(items).toEqual(["工作树详情…", "切换分支…"]);
    await userEvent.keyboard("{Escape}");
    await expect.poll(items).toEqual([]);
  }
  await dialog()
    .getByRole("button", { name: "工作树操作：wt/clean-feature" })
    .click();
  await page.getByRole("menuitem", { name: "工作树详情…" }).click();
  await expect
    .element(
      dialog().getByRole("region", { name: "工作树详情：wt/clean-feature" }),
    )
    .toMatchTextContent(/整理清理流程/);
});

test("lists all worktrees including unpinned ones, filters, selects safe items, and executes safe-only batch removal by default", async () => {
  const fetch = vi.fn<typeof globalThis.fetch>(async (input) => {
    const url = String(input);
    if (url.endsWith("/batch-removal-preview")) {
      return Response.json({
        confirmationId: "batchconf1",
        expiresAt: "2099-01-01T00:00:00Z",
        items: [
          {
            worktree: unpinnedClean,
            sessions: [],
            archived: [],
            commit: "a".repeat(40),
            push: noRemote,
            blockers: [],
            warnings: [],
            forceAllowed: true,
          },
          {
            worktree: prunableTree,
            sessions: [],
            archived: [keptArchive],
            commit: "b".repeat(40),
            push: { state: "unpushed", branches: [], more: 0 },
            blockers: [],
            warnings: [],
            forceAllowed: true,
          },
          {
            worktree: blockedTree,
            sessions: [liveChat],
            archived: [],
            commit: "c".repeat(40),
            push: noRemote,
            blockers: ["存在未提交修改", "存在运行中的 WebMux 会话"],
            warnings: [],
            forceAllowed: true,
          },
        ],
      });
    }
    return Response.json({
      results: [
        {
          worktreeId: unpinnedClean.id,
          path: unpinnedClean.path,
          branch: "wt/clean-feature",
          status: "removed",
          error: "",
        },
        {
          worktreeId: prunableTree.id,
          path: prunableTree.path,
          branch: "wt/stale-reg",
          status: "removed",
          error: "",
        },
        {
          worktreeId: blockedTree.id,
          path: blockedTree.path,
          branch: "wt/wip-agent",
          status: "skipped",
          error: "",
        },
      ],
    });
  });
  vi.stubGlobal("fetch", fetch);

  const handlers = renderPanel({
    initialTab: "worktrees",
    sessions: [liveChat],
    records: [keptArchive],
  });

  await expect.element(dialog()).toMatchTextContent(/未添加到主页/);
  await expect.element(dialog()).toMatchTextContent(/Git 登记待清理/);
  // Browsing shows no checkboxes; batch cleanup adds them.
  expect(dialog().getByRole("checkbox").elements()).toEqual([]);
  await dialog().getByRole("button", { name: "批量清理" }).click();

  // Protected items have disabled checkboxes.
  await expect
    .element(dialog().getByRole("checkbox", { name: "main" }))
    .toBeDisabled();
  await expect
    .element(dialog().getByRole("checkbox", { name: "wt/locked-prod" }))
    .toBeDisabled();

  // Select all safe items (2), then also check the blocked item.
  await dialog()
    .getByRole("button", { name: "选中全部可安全清理（2）" })
    .click();
  await dialog().getByRole("checkbox", { name: "wt/wip-agent" }).click();
  await dialog().getByRole("button", { name: "下一步：预览删除 (3)" }).click();

  await expect
    .element(dialog().getByRole("heading", { name: "确认批量清理" }))
    .toBeVisible();
  await expect
    .element(dialog())
    .toMatchTextContent(/可安全清理 2 个.*有阻塞 1 个/);
  await expect
    .element(dialog())
    .toMatchTextContent(/清理失效登记.*保留 1 条归档聊天/);
  await expect
    .element(dialog())
    .toMatchTextContent(/默认跳过，不改动文件与会话/);

  // Default submit runs safe-only without includeForce.
  await dialog().getByRole("button", { name: "确认安全删除 (2)" }).click();

  const lastCall = fetch.mock.calls.at(-1);
  expect(lastCall?.[0]).toBe("/api/repositories/repo1/worktrees/batch-remove");
  expect(JSON.parse(String(lastCall?.[1]?.body))).toEqual({
    confirmationId: "batchconf1",
    includeForce: false,
    closeSessions: false,
  });
  expect(handlers.onremoved).toHaveBeenCalledWith(["clean1", "gone1"]);
  expect(handlers.onchange).toHaveBeenCalledOnce();
  // The panel stays open on the refreshed list.
  expect(handlers.onclose).not.toHaveBeenCalled();
  await expect.element(dialog().getByText("已删除 2 个工作树。")).toBeVisible();
  await expect
    .element(dialog().getByRole("button", { name: "批量清理" }))
    .toBeVisible();
});

test("supports explicit force authorization and keeps failed items selected for retry", async () => {
  let previewCalls = 0;
  const fetch = vi.fn<typeof globalThis.fetch>(async (input) => {
    const url = String(input);
    if (url.endsWith("/batch-removal-preview")) {
      previewCalls++;
      return Response.json({
        confirmationId: `conf-${previewCalls}`,
        expiresAt: "2099-01-01T00:00:00Z",
        items:
          previewCalls === 1
            ? [
                {
                  worktree: unpinnedClean,
                  sessions: [],
                  archived: [],
                  commit: "a".repeat(40),
                  push: noRemote,
                  blockers: [],
                  warnings: [],
                  forceAllowed: true,
                },
                {
                  worktree: blockedTree,
                  sessions: [liveChat],
                  archived: [],
                  commit: "c".repeat(40),
                  push: noRemote,
                  blockers: ["存在运行中的 WebMux 会话"],
                  warnings: [],
                  forceAllowed: true,
                },
              ]
            : [
                {
                  worktree: blockedTree,
                  sessions: [],
                  archived: [],
                  commit: "c".repeat(40),
                  push: noRemote,
                  blockers: [],
                  warnings: [],
                  forceAllowed: true,
                },
              ],
      });
    }
    return Response.json({
      results: [
        {
          worktreeId: unpinnedClean.id,
          path: unpinnedClean.path,
          branch: "wt/clean-feature",
          status: "removed",
          error: "",
        },
        {
          worktreeId: blockedTree.id,
          path: blockedTree.path,
          branch: "wt/wip-agent",
          status: "failed",
          error: "工作树关联会话已变化，请重新读取预览",
        },
      ],
    });
  });
  vi.stubGlobal("fetch", fetch);

  const handlers = renderPanel({
    initialTab: "worktrees",
    sessions: [liveChat],
  });

  await dialog().getByRole("button", { name: "批量清理" }).click();
  await dialog().getByRole("checkbox", { name: "wt/clean-feature" }).click();
  await dialog().getByRole("checkbox", { name: "wt/wip-agent" }).click();
  await dialog().getByRole("button", { name: "下一步：预览删除 (2)" }).click();

  await dialog()
    .getByRole("checkbox", { name: /同时强制删除以上 1 个受阻工作树/ })
    .click();
  await expect
    .element(dialog().getByRole("heading", { name: "批量强制删除" }))
    .toBeVisible();
  await dialog().getByRole("button", { name: "关闭会话并删除 (2)" }).click();

  const removeBody = JSON.parse(String(fetch.mock.calls.at(-1)?.[1]?.body));
  expect(removeBody).toEqual({
    confirmationId: "conf-1",
    includeForce: true,
    closeSessions: true,
  });
  expect(handlers.onremoved).toHaveBeenCalledWith(["clean1"]);
  expect(handlers.onclose).not.toHaveBeenCalled();

  await expect
    .element(dialog().getByRole("heading", { name: "批量清理部分完成" }))
    .toBeVisible();
  await expect
    .element(dialog())
    .toMatchTextContent(/已删除 1 个.*失败 1 个.*工作树关联会话已变化/);
  await dialog().getByRole("button", { name: "重新预览重试 (1)" }).click();

  expect(JSON.parse(String(fetch.mock.calls.at(-1)?.[1]?.body))).toEqual({
    worktreeIds: ["dirty1"],
  });
});

test("sections cannot be switched and the panel cannot close while a removal runs", async () => {
  const removal = Promise.withResolvers<Response>();
  const fetch = vi.fn<typeof globalThis.fetch>(async (input) => {
    if (String(input).endsWith("/batch-removal-preview"))
      return Response.json({
        confirmationId: "conf",
        expiresAt: "2099-01-01T00:00:00Z",
        items: [
          {
            worktree: unpinnedClean,
            sessions: [],
            archived: [],
            commit: "a".repeat(40),
            push: noRemote,
            blockers: [],
            warnings: [],
            forceAllowed: true,
          },
        ],
      });
    return removal.promise;
  });
  vi.stubGlobal("fetch", fetch);
  const handlers = renderPanel({ initialTab: "worktrees" });

  await dialog().getByRole("button", { name: "批量清理" }).click();
  await dialog().getByRole("checkbox", { name: "wt/clean-feature" }).click();
  await dialog().getByRole("button", { name: "下一步：预览删除 (1)" }).click();
  await dialog().getByRole("button", { name: "确认安全删除 (1)" }).click();
  await expect
    .element(dialog().getByText("正在处理，请勿重复提交…"))
    .toBeVisible();

  await expect.element(tab("分支")).toBeDisabled();
  await expect.element(tab("概览")).toBeDisabled();
  await userEvent.keyboard("{Escape}");
  expect(handlers.onclose).not.toHaveBeenCalled();

  removal.resolve(
    Response.json({
      results: [
        {
          worktreeId: unpinnedClean.id,
          path: unpinnedClean.path,
          branch: "wt/clean-feature",
          status: "removed",
          error: "",
        },
      ],
    }),
  );
  await expect.element(tab("分支")).toBeEnabled();
  await tab("分支").click();
  await expect.element(tab("分支")).toHaveAttribute("aria-selected", "true");
});

const branches = {
  branches: [
    branch("main", "a".repeat(40), ["/repos/webmux"], {
      subject: "发布 1.0",
      upstream: "origin/main",
      ahead: 0,
      behind: 0,
    }),
    branch("feat/login-a", "d".repeat(40), [], {
      subject: "登录页",
      upstream: "origin/feat/login-a",
      ahead: 2,
      behind: 0,
    }),
    branch("wt/clean-feature", "a".repeat(40), [unpinnedClean.path]),
    branch("wt/gone", "e".repeat(40), [], {
      upstream: "origin/wt/gone",
      upstreamGone: true,
    }),
    branch("wt/huge", "f".repeat(40), [], { upstream: "origin/wt/huge" }),
  ],
  bases: [
    base("refs/heads/main", "a".repeat(40), "发布 1.0"),
    base("refs/remotes/origin/main", "a".repeat(40), "发布 1.0"),
    base("refs/remotes/origin/feat/login-b", "9".repeat(40), "远程登录"),
  ],
  defaultBase: "a".repeat(40),
  defaultBaseSubject: "发布 1.0",
  defaultBranch: "main",
  defaultParent: "/repos/webmux.worktrees",
};

test("branches show their upstream state, search both groups and open the occupying worktree", async () => {
  const data = source({ branches });
  renderPanel({ initialTab: "branches", source: data });
  const local = dialog().getByRole("region", { name: "本地分支" });
  await expect.element(local).toMatchTextContent(/^本地分支\s*5/);
  expect(data.branches).toHaveBeenCalledOnce();
  await expect
    .element(local.getByRole("img", { name: "领先上游 2 个提交" }))
    .toBeVisible();
  await expect.element(local.getByText("无上游")).toBeVisible();
  await expect.element(local.getByText("上游已删除")).toBeVisible();
  await expect.element(local.getByText("未计算")).toBeVisible();
  await expect.element(local.getByText("已同步")).toBeVisible();

  const remote = dialog().getByRole("region", { name: "远程分支" });
  await expect
    .element(remote.getByRole("button", { name: /远程分支/ }))
    .toHaveAttribute("aria-expanded", "false");

  await dialog().getByRole("searchbox", { name: "搜索分支" }).fill("login");
  await expect.element(local).toMatchTextContent(/匹配 1 \/ 5/);
  await expect.element(remote).toMatchTextContent(/匹配 1 \/ 2/);
  await expect.element(remote.getByText("origin/feat/login-b")).toBeVisible();
  expect(local.getByText("wt/gone").elements()).toEqual([]);

  await dialog().getByRole("searchbox", { name: "搜索分支" }).fill("clean");
  await local.getByRole("button", { name: unpinnedClean.path }).click();
  const details = dialog().getByRole("region", {
    name: "工作树详情：wt/clean-feature",
  });
  await expect.element(details).toBeVisible();
  expect(page.getByRole("dialog").elements()).toHaveLength(1);
  await details.getByRole("button", { name: "返回" }).click();
  await expect
    .element(dialog().getByRole("searchbox", { name: "搜索分支" }))
    .toHaveValue("clean");
  expect(data.branches).toHaveBeenCalledOnce();
});

test("unoccupied local and every remote branch open in a new worktree", async () => {
  const handlers = renderPanel({
    initialTab: "branches",
    source: source({ branches }),
  });
  const local = dialog().getByRole("region", { name: "本地分支" });
  const open = "在新工作树打开…";
  await expect
    .element(local.getByRole("listitem").filter({ hasText: "feat/login-a" }))
    .toBeVisible();
  // Branches a worktree has checked out offer no new worktree.
  for (const name of ["main", "wt/clean-feature"])
    expect(
      local
        .getByRole("listitem")
        .filter({ hasText: name })
        .getByRole("button", { name: open })
        .elements(),
    ).toEqual([]);
  await local
    .getByRole("listitem")
    .filter({ hasText: "feat/login-a" })
    .getByRole("button", { name: open })
    .click();
  expect(handlers.onnewworktree).toHaveBeenLastCalledWith({
    mode: "existing",
    branch: "feat/login-a",
  });

  const remote = dialog().getByRole("region", { name: "远程分支" });
  await remote.getByRole("button", { name: /远程分支/ }).click();
  await remote
    .getByRole("listitem")
    .filter({ hasText: "origin/feat/login-b" })
    .getByRole("button", { name: open })
    .click();
  expect(handlers.onnewworktree).toHaveBeenLastCalledWith({
    mode: "new",
    startBranch: "origin/feat/login-b",
    name: "feat/login-b",
  });
});

function switchSource(
  preview: Partial<SwitchPreview> = {},
  switched: Worktree = { ...unpinnedClean, branch: "feat/login-a" },
): SwitchSource {
  return {
    branches: vi.fn(async () => branches),
    commits: vi.fn(async () => ({ commits: [] })),
    preview: vi.fn(async (tree: Worktree) => ({
      confirmationId: "switch01",
      expiresAt: "2026-10-28T00:05:00Z",
      worktree: tree,
      target: {
        kind: "local" as const,
        branch: "feat/login-a",
        commit: "d".repeat(40),
        subject: "登录页",
        createsBranch: false,
        upstream: null,
      },
      sessions: [],
      blockers: [],
      warnings: [],
      ...preview,
    })),
    switch: vi.fn(async () => switched),
  };
}

async function chooseBranch(name: string) {
  await dialog().getByRole("button", { name: "目标分支", exact: true }).click();
  await page.getByRole("option", { name, exact: true }).click();
}

test("a worktree row's switch is a subview; a finished switch shows the worktree's details", async () => {
  const data = switchSource();
  const handlers = renderPanel({ initialTab: "worktrees", switchSource: data });
  await dialog()
    .getByRole("button", { name: "工作树操作：wt/clean-feature" })
    .click();
  await page.getByRole("menuitem", { name: "切换分支…" }).click();
  const view = dialog().getByRole("region", {
    name: "切换分支：wt/clean-feature",
  });
  await expect.element(view).toBeVisible();
  // One dialog: the switch replaces the tabs instead of stacking.
  expect(page.getByRole("dialog").elements()).toHaveLength(1);
  expect(dialog().getByRole("tablist").elements()).toEqual([]);

  await chooseBranch("feat/login-a");
  await expect
    .element(view.getByRole("region", { name: "切换预览" }))
    .toMatchTextContent(/wt\/clean-feature\s*切换到\s*feat\/login-a/);
  await view.getByRole("button", { name: "切换", exact: true }).click();
  expect(data.switch).toHaveBeenCalledWith(
    expect.objectContaining({ id: "clean1" }),
    { confirmationId: "switch01", acknowledgeSessions: false },
  );
  expect(handlers.onrefresh).toHaveBeenCalledOnce();
  await expect
    .element(
      dialog().getByRole("region", { name: "工作树详情：wt/clean-feature" }),
    )
    .toBeVisible();
  // The details' "返回" leads back to the tabs.
  await dialog().getByRole("button", { name: "返回" }).click();
  await expect.element(tab("工作树")).toHaveAttribute("aria-selected", "true");
});

test("the details subview opens the switch; 返回 and 转到该工作树 stay in the panel", async () => {
  const data = switchSource({
    confirmationId: "",
    expiresAt: null,
    blockers: [
      {
        code: "occupied",
        message: "分支 feat/login-a 已被另一个工作树检出",
        path: mainTree.path,
      },
    ],
  });
  renderPanel({ initialTab: "worktrees", switchSource: data });
  await dialog()
    .getByRole("list", { name: "仓库工作树列表" })
    .getByRole("button", { name: /^wt\/clean-feature/ })
    .click();
  const details = dialog().getByRole("region", {
    name: "工作树详情：wt/clean-feature",
  });
  await details.getByRole("button", { name: "切换分支…" }).click();
  const view = dialog().getByRole("region", {
    name: "切换分支：wt/clean-feature",
  });
  await view.getByRole("button", { name: "返回" }).click();
  await expect.element(details).toBeVisible();

  await details.getByRole("button", { name: "切换分支…" }).click();
  await chooseBranch("feat/login-a");
  await expect
    .element(view.getByRole("button", { name: "切换", exact: true }))
    .toBeDisabled();
  await view.getByRole("button", { name: "转到该工作树" }).click();
  await expect
    .element(dialog().getByRole("region", { name: "工作树详情：main" }))
    .toBeVisible();
  expect(page.getByRole("dialog").elements()).toHaveLength(1);
  expect(data.switch).not.toHaveBeenCalled();
});

test("the panel cannot close or go back while a switch runs", async () => {
  const pending = Promise.withResolvers<Worktree>();
  const data = switchSource();
  data.switch = vi.fn(() => pending.promise);
  const handlers = renderPanel({ initialTab: "worktrees", switchSource: data });
  await dialog()
    .getByRole("button", { name: "工作树操作：wt/clean-feature" })
    .click();
  await page.getByRole("menuitem", { name: "切换分支…" }).click();
  await chooseBranch("feat/login-a");
  const view = dialog().getByRole("region", {
    name: "切换分支：wt/clean-feature",
  });
  await view.getByRole("button", { name: "切换", exact: true }).click();
  await expect
    .element(view.getByRole("button", { name: "返回" }))
    .toBeDisabled();
  await userEvent.keyboard("{Escape}");
  expect(handlers.onclose).not.toHaveBeenCalled();
  pending.resolve({ ...unpinnedClean, branch: "feat/login-a" });
  await expect
    .element(
      dialog().getByRole("region", { name: "工作树详情：wt/clean-feature" }),
    )
    .toBeVisible();
});

test("remotes show one address when fetch and push match, both when they differ", async () => {
  renderPanel({
    initialTab: "remotes",
    source: source({
      remotes: [
        origin,
        {
          name: "fork",
          fetchUrl: "https://***@github.com/me/r.git",
          pushUrl: "git@github.com:me/r.git",
        },
      ],
    }),
  });
  const list = dialog().getByRole("list", { name: "远程列表" });
  await expect
    .element(list)
    .toMatchTextContent(
      /^origin\s*获取与推送\s*https:\/\/user:\*\*\*@github\.com\/o\/r\.git\s*fork/,
    );
  expect(list.getByText(origin.fetchUrl).elements()).toHaveLength(1);
  await expect
    .element(list.getByRole("button", { name: "复制 fork 的推送地址" }))
    .toBeVisible();
  await expect
    .element(list.getByRole("button", { name: "复制 fork 的获取地址" }))
    .toBeVisible();
  await expect.element(list.getByText("git@github.com:me/r.git")).toBeVisible();
});

test("a repository without remotes says so", async () => {
  renderPanel({ initialTab: "remotes", source: source({ remotes: [] }) });
  await expect.element(dialog().getByText("没有远程")).toBeVisible();
});

test("the home worktree dialog's footer hands over to batch management", async () => {
  const onmanage = vi.fn();
  const onclose = vi.fn();
  render(HomeWorktreeDialog, {
    repository: repo,
    sessions: [],
    records: [],
    home: { repo1: ["main"] },
    ontoggle: vi.fn(),
    onmanage,
    onclose,
  });

  await page.getByRole("button", { name: "批量管理…" }).click();
  expect(onmanage).toHaveBeenCalledOnce();
  expect(onclose).toHaveBeenCalledOnce();
});

const fork: Remote = {
  name: "fork",
  fetchUrl: "https://***@github.com/me/r.git",
  pushUrl: "https://***@github.com/me/r.git",
};
const fetchButton = () => dialog().getByRole("button", { name: /获取远程$/ });

test("获取远程 sits in the title bar of every section and replaces the repository with the reply", async () => {
  const fetched = repository({
    ...repo,
    lastFetchedAt: "2026-11-01T00:00:00Z",
    worktrees: [{ ...mainTree, behind: 2 }, unpinnedClean],
  });
  const fetch = vi.fn(async (): Promise<FetchRemotesResponse> => ({
    results: [{ remote: "origin", status: "ok", message: "" }],
    repository: fetched,
  }));
  const handlers = renderPanel({
    initialTab: "worktrees",
    source: source({ fetch }),
  });
  await expect.element(fetchButton()).toBeEnabled();
  await tab("分支").click();
  await expect.element(fetchButton()).toBeEnabled();
  await fetchButton().click();

  expect(fetch).toHaveBeenCalledExactlyOnceWith("repo1");
  const results = dialog().getByRole("list", { name: "获取结果" });
  await expect.element(results).toHaveTextContent("origin：已获取");
  await expect
    .element(dialog().getByRole("status").filter({ hasText: "已获取" }))
    .toHaveAttribute("data-tone", "success");
  expect(handlers.onrepository).toHaveBeenCalledExactlyOnceWith(fetched);
  // The results stay while switching sections.
  await tab("远程").click();
  await expect.element(results).toBeVisible();
});

test("a partly failed fetch lists each remote's result", async () => {
  const fetch = vi.fn(async (): Promise<FetchRemotesResponse> => ({
    results: [
      {
        remote: "fork",
        status: "auth",
        message: "认证失败，请在终端完成认证后重试：Permission denied",
      },
      { remote: "origin", status: "ok", message: "" },
    ],
    repository: repo,
  }));
  const handlers = renderPanel({
    source: source({ remotes: [fork, origin], fetch }),
  });
  await fetchButton().click();
  const results = dialog().getByRole("list", { name: "获取结果" });
  await expect
    .element(results.getByRole("listitem").nth(0))
    .toHaveTextContent(
      "fork：认证失败，请在终端完成认证后重试：Permission denied",
    );
  await expect
    .element(results.getByRole("listitem").nth(1))
    .toHaveTextContent("origin：已获取");
  await expect
    .element(dialog().getByRole("status").filter({ hasText: "已获取" }))
    .toHaveAttribute("data-tone", "warning");
  expect(handlers.onrepository).toHaveBeenCalledOnce();
});

test("获取远程 is disabled with its reason when the repository has no remotes", async () => {
  const fetch = vi.fn();
  renderPanel({ source: source({ remotes: [], fetch }) });
  await expect.element(fetchButton()).toBeDisabled();
  await expect
    .element(fetchButton())
    .toHaveAccessibleDescription("这个仓库没有配置远程，无法获取");
  expect(fetchButton().element().parentElement?.title).toBe(
    "这个仓库没有配置远程，无法获取",
  );
  expect(fetch).not.toHaveBeenCalled();
});

test("a running fetch shows progress and another click sends no second request", async () => {
  let finish: (reply: FetchRemotesResponse) => void = () => {};
  const fetch = vi.fn(
    () =>
      new Promise<FetchRemotesResponse>((resolve) => {
        finish = resolve;
      }),
  );
  renderPanel({ source: source({ fetch }) });
  await fetchButton().click();
  await expect.element(fetchButton()).toHaveAccessibleName("正在获取远程");
  await expect.element(fetchButton()).toBeDisabled();
  (fetchButton().element() as HTMLButtonElement).click();
  expect(fetch).toHaveBeenCalledOnce();

  finish({
    results: [{ remote: "origin", status: "ok", message: "" }],
    repository: repo,
  });
  await expect.element(fetchButton()).toHaveAccessibleName("获取远程");
  expect(fetch).toHaveBeenCalledOnce();
});

const staleTree = worktree({
  id: "gone2",
  repositoryId: "repo1",
  kind: "linked",
  branch: "wt/stale-two",
  path: "/repos/webmux.worktrees/stale-two",
  prunable: true,
  available: false,
  dirty: null,
  changes: null,
});
const lockedStale = worktree({
  id: "lockgone",
  repositoryId: "repo1",
  kind: "linked",
  branch: "wt/locked-gone",
  path: "/repos/webmux.worktrees/locked-gone",
  locked: true,
  lockReason: "usb",
  prunable: true,
  available: false,
  dirty: null,
  changes: null,
});
// Git marks it prunable, but its directory is still there.
const prunablePresent = worktree({
  id: "present",
  repositoryId: "repo1",
  kind: "linked",
  branch: "wt/present",
  path: "/repos/webmux.worktrees/present",
  prunable: true,
  available: true,
});

test("清理失效登记 counts only unlocked stale registrations and previews exactly them", async () => {
  const fetch = vi.fn<typeof globalThis.fetch>(async () =>
    Response.json({
      confirmationId: "staleconf",
      expiresAt: "2099-01-01T00:00:00Z",
      items: [prunableTree, staleTree].map((tree) => ({
        worktree: tree,
        sessions: [],
        archived: [],
        commit: tree.head,
        push: noRemote,
        blockers: [],
        warnings: [],
        forceAllowed: true,
      })),
    }),
  );
  vi.stubGlobal("fetch", fetch);
  renderPanel({
    initialTab: "worktrees",
    repository: repository({
      ...repo,
      worktrees: [
        mainTree,
        unpinnedClean,
        prunableTree,
        staleTree,
        lockedStale,
        prunablePresent,
      ],
    }),
  });
  const list = dialog().getByRole("list", { name: "仓库工作树列表" });
  await expect
    .element(
      list
        .getByRole("listitem")
        .filter({ hasText: "wt/locked-gone" })
        .getByText("已锁定，需在终端解锁后清理"),
    )
    .toBeVisible();
  await dialog().getByRole("button", { name: "清理失效登记（2）" }).click();

  await expect
    .element(dialog().getByRole("heading", { name: "确认批量清理" }))
    .toBeVisible();
  expect(fetch).toHaveBeenCalledOnce();
  expect(fetch.mock.calls[0]?.[0]).toBe(
    "/api/repositories/repo1/worktrees/batch-removal-preview",
  );
  expect(JSON.parse(String(fetch.mock.calls[0]?.[1]?.body))).toEqual({
    worktreeIds: ["gone1", "gone2"],
  });
  // Going back shows the preselection to adjust.
  await dialog().getByRole("button", { name: "返回重选" }).click();
  await expect
    .element(dialog().getByRole("checkbox", { name: "wt/stale-reg" }))
    .toBeChecked();
  await expect
    .element(dialog().getByRole("checkbox", { name: "wt/stale-two" }))
    .toBeChecked();
  await expect
    .element(dialog().getByRole("checkbox", { name: "wt/clean-feature" }))
    .not.toBeChecked();
  await expect
    .element(dialog().getByRole("checkbox", { name: "wt/locked-gone" }))
    .toBeDisabled();
});

test("清理失效登记 is hidden without stale registrations", async () => {
  renderPanel({
    initialTab: "worktrees",
    repository: repository({
      ...repo,
      worktrees: [mainTree, unpinnedClean, lockedStale, prunablePresent],
    }),
  });
  await expect
    .element(dialog().getByRole("button", { name: "批量清理" }))
    .toBeVisible();
  expect(
    dialog()
      .getByRole("button", { name: /清理失效登记/ })
      .elements(),
  ).toEqual([]);
});

// add-branch-sync-actions: the branches section's actions and the flows
// they open as panel subviews.
const syncBranches: Branches = {
  ...branches,
  branches: [
    ...branches.branches.filter((item) => item.name !== "wt/huge"),
    branch("wt/merged", "7".repeat(40), [], {
      upstream: "origin/wt/merged",
      ahead: 0,
      behind: 0,
      mergedIntoDefault: true,
    }),
    branch("wt/behind", "8".repeat(40), [], {
      upstream: "origin/wt/behind",
      ahead: 0,
      behind: 3,
    }),
    branch("wt/diverged", "6".repeat(40), [], {
      upstream: "origin/wt/diverged",
      ahead: 1,
      behind: 2,
    }),
  ],
};

test("branch rows offer the actions their state allows and count cleanup candidates", async () => {
  renderPanel({
    initialTab: "branches",
    source: source({ branches: syncBranches }),
  });
  const local = dialog().getByRole("region", { name: "本地分支" });
  // Not checked out, and gone upstream or merged: wt/gone and wt/merged.
  await expect
    .element(local.getByRole("button", { name: "清理分支（2）" }))
    .toBeVisible();
  await expect.element(local.getByText("已合入默认分支")).toBeVisible();
  const items = () =>
    page
      .getByRole("menuitem")
      .elements()
      .map((el) => el.textContent?.trim());
  for (const [name, expected] of [
    ["feat/login-a", ["推送…", "删除分支…"]],
    ["wt/gone", ["发布分支…", "删除分支…"]],
    ["wt/behind", ["快进到上游", "删除分支…"]],
    ["wt/diverged", ["删除分支…"]],
  ] as const) {
    await local.getByRole("button", { name: `分支操作：${name}` }).click();
    await expect.element(page.getByRole("menuitem").first()).toBeVisible();
    expect(items()).toEqual(expected);
    await userEvent.keyboard("{Escape}");
    await expect.poll(() => items()).toEqual([]);
  }
  await expect
    .element(local.getByText("已分叉，需要在终端合并或变基。"))
    .toBeVisible();
  // A checked-out branch in sync has nothing to offer.
  expect(
    local.getByRole("button", { name: "分支操作：main" }).elements(),
  ).toEqual([]);
});

test("cleanup runs as a panel subview; 返回 shows the branches read again", async () => {
  await page.viewport(390, 844);
  const data = source();
  const after: Branches = {
    ...syncBranches,
    branches: syncBranches.branches.filter((item) => item.name !== "wt/merged"),
  };
  vi.mocked(data.branches)
    .mockResolvedValueOnce(syncBranches)
    .mockResolvedValue(after);
  const requests: { path: string; body: unknown }[] = [];
  const reply = (body: unknown) =>
    new Response(JSON.stringify(body), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    });
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>(async (input, init) => {
      const path = new URL(String(input), location.href).pathname;
      requests.push({ path, body: JSON.parse(String(init?.body ?? "null")) });
      if (path.endsWith("/branches/delete-preview"))
        return reply({
          confirmationId: "del1",
          expiresAt: "2026-10-28T00:05:00Z",
          items: [
            {
              branch: "wt/gone",
              commit: "e".repeat(40),
              group: "unmerged",
              unmerged: {
                total: 1,
                items: [
                  {
                    hash: "e".repeat(40),
                    subject: "squash 前的提交",
                    author: "dev",
                    committedAt: "2026-10-27T00:00:00Z",
                  },
                ],
              },
              blockers: [],
            },
            {
              branch: "wt/merged",
              commit: "7".repeat(40),
              group: "contained",
              unmerged: { total: 0, items: [] },
              blockers: [],
            },
          ],
        });
      if (path.endsWith("/branches/delete"))
        return reply({
          results: [
            {
              branch: "wt/merged",
              status: "deleted",
              commit: "7".repeat(40),
              message: "已删除",
            },
          ],
          repository: repo,
        });
      return new Response(null, { status: 404 });
    }),
  );
  const handlers = renderPanel({ initialTab: "branches", source: data });
  await dialog().getByRole("button", { name: "清理分支（2）" }).click();
  const flow = dialog().getByRole("region", { name: "清理分支" });
  await expect.element(flow).toBeVisible();
  // The flow takes the panel's place instead of stacking a dialog on it.
  expect(page.getByRole("dialog").elements()).toHaveLength(1);
  expect(requests[0]).toEqual({
    path: "/api/repositories/repo1/branches/delete-preview",
    body: { branches: ["wt/gone", "wt/merged"] },
  });
  await expect
    .element(flow.getByRole("checkbox", { name: "删除 wt/merged" }))
    .toBeChecked();
  await expect
    .element(flow.getByRole("checkbox", { name: /^删除 wt\/gone/ }))
    .not.toBeChecked();
  await flow.getByRole("button", { name: "删除 1 个分支" }).click();
  expect(requests.at(-1)).toEqual({
    path: "/api/repositories/repo1/branches/delete",
    body: { confirmationId: "del1", acknowledgeUnmerged: false },
  });
  await expect
    .element(flow.getByRole("list", { name: "删除结果" }))
    .toMatchTextContent(/wt\/merged.*已删除/);
  expect(handlers.onrefresh).toHaveBeenCalledOnce();
  await flow.getByRole("button", { name: "返回" }).click();
  const local = dialog().getByRole("region", { name: "本地分支" });
  await expect.element(local).toBeVisible();
  expect(data.branches).toHaveBeenCalledTimes(2);
  await expect
    .element(local.getByRole("button", { name: "清理分支（1）" }))
    .toBeVisible();
});

test("a worktree's details offer its branch actions inside the panel", async () => {
  const behind = worktree({
    id: "behind1",
    kind: "linked",
    branch: "wt/behind",
    path: "/repos/webmux.worktrees/behind",
    upstream: "origin/wt/behind",
    ahead: 0,
    behind: 2,
  });
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>(
      async () =>
        new Response(
          JSON.stringify({
            confirmationId: "ff1",
            expiresAt: "2026-10-28T00:05:00Z",
            branch: "wt/behind",
            from: "a".repeat(40),
            to: "b".repeat(40),
            upstream: "origin/wt/behind",
            commits: { total: 0, items: [] },
            worktree: behind,
            sessions: [],
            blockers: [],
            warnings: [],
          }),
          { status: 200 },
        ),
    ),
  );
  renderPanel({
    initialTab: "worktrees",
    repository: { ...repo, worktrees: [mainTree, behind] },
  });
  await dialog()
    .getByRole("button", { name: /^wt\/behind/ })
    .click();
  const details = dialog().getByRole("region", {
    name: "工作树详情：wt/behind",
  });
  const branchSection = details.getByRole("region", { name: "分支与上游" });
  expect(
    branchSection.getByRole("button", { name: "推送…" }).elements(),
  ).toEqual([]);
  await branchSection.getByRole("button", { name: "快进到上游" }).click();
  const flow = dialog().getByRole("region", { name: "快进到上游" });
  await expect.element(flow).toMatchTextContent(/wt\/behind/);
  expect(page.getByRole("dialog").elements()).toHaveLength(1);
  // 返回 leads back to the details it was started from.
  await flow.getByRole("button", { name: "返回" }).click();
  await expect.element(details).toBeVisible();
});
