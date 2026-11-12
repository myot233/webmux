import { afterEach, expect, test, vi } from "vitest";
import { page, userEvent } from "vitest/browser";
import { cleanup, render } from "vitest-browser-svelte/pure";
import { suggestWorktreeBranch } from "../../src/domains/workspace/branch-name";
import type { Repository } from "../../src/contracts/schemas";
import Harness from "./WorktreeCreateFieldsHarness.svelte";
import { base, branch } from "./shell-fixture";

const head = "a".repeat(40);
const repository = {
  id: "repo-1",
  name: "project",
  commonDir: "/repos/project/.git",
  anchorPath: "/repos/project",
  availability: "available",
  error: "",
  checkedAt: "2026-10-14T00:00:00Z",
  lastFetchedAt: null,
  initInstruction: "",
  worktrees: [
    {
      id: "main",
      repositoryId: "repo-1",
      path: "/repos/project",
      kind: "main",
      branch: "main",
      head,
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
    },
    {
      id: "linked",
      repositoryId: "repo-1",
      path: "/repos/project.worktrees/calm-river",
      kind: "linked",
      branch: "feature/calm-river",
      head,
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
    },
  ],
} satisfies Repository;

const branches = {
  branches: [
    branch("main", head, ["/repos/project"]),
    branch("wt/taken-branch", head),
    branch("feature/calm-river", head, ["/repos/project.worktrees/calm-river"]),
  ],
  bases: [base("refs/heads/main", head, "修复终端重连")],
  defaultBase: head,
  defaultBaseSubject: "修复终端重连",
  defaultBranch: "main",
  defaultParent: "/repos/project.worktrees",
};

type RefreshHandler = (
  branch: string,
  signal: AbortSignal | null,
) => Response | Promise<Response>;

function localRefresh(response: typeof branches, branch: string) {
  const selected = response.branches.find((item) => item.name === branch);
  return {
    branch,
    base: selected?.head || response.defaultBase || "",
    baseSubject:
      response.bases.find((item) => item.name === `refs/heads/${branch}`)
        ?.subject || response.defaultBaseSubject,
    baseSource: "local",
    upstream: "",
    upstreamHead: "",
    fetchWarning: "该分支未配置上游，保留本地基线",
  };
}

function remoteRefresh(
  branch: string,
  base: string,
  subject = "remote",
  upstreamHead = base,
) {
  return {
    branch,
    base,
    baseSubject: subject,
    baseSource: "remote",
    upstream: `origin/${branch}`,
    upstreamHead,
    fetchWarning: "",
  };
}

function deferred<T>() {
  return Promise.withResolvers<T>();
}

const older = "e".repeat(40);
const committedAt = new Date(Date.now() - 3 * 3600 * 1000).toISOString();

// History served for the requested tips: each tip, newest first, on top of
// one shared older commit.
function history(from: string[]) {
  return {
    commits: [
      ...from.map((hash, index) => ({
        hash,
        subject: index === 0 ? "最新提交" : `提交 ${hash.slice(0, 7)}`,
        author: "dev",
        committedAt,
      })),
      { hash: older, subject: "较早提交", author: "dev", committedAt },
    ],
  };
}

function serve(
  status = 200,
  response = branches,
  onrefresh?: RefreshHandler,
  onbranches?: (signal: AbortSignal | null) => Response | Promise<Response>,
) {
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>(async (input, init) => {
      const url = new URL(String(input), location.href);
      if (url.pathname === "/api/repositories/repo-1/branches/refresh") {
        const body = JSON.parse(String(init?.body)) as { branch: string };
        if (onrefresh) return onrefresh(body.branch, init?.signal ?? null);
        return Response.json(localRefresh(response, body.branch));
      }
      if (url.pathname === "/api/repositories/repo-1/commits") {
        return Response.json(history(url.searchParams.getAll("from")));
      }
      if (url.pathname !== "/api/repositories/repo-1/branches") {
        throw new Error(`Unexpected request: ${url.pathname}`);
      }
      if (status !== 200) {
        return Response.json({ error: "无法读取分支" }, { status });
      }
      if (onbranches) return onbranches(init?.signal ?? null);
      return Response.json(response);
    }),
  );
}

const branchInput = () =>
  page.getByRole("textbox", { name: "新分支名称", exact: true });
const pathInput = () =>
  page.getByRole("textbox", { name: "新工作树完整路径", exact: true });
const ready = () => page.getByRole("status", { name: "表单就绪" });
const baseInput = () =>
  page.getByRole("combobox", { name: "起始提交", exact: true });
const options = () => page.getByRole("listbox").getByRole("option");
async function openBase() {
  await baseInput().click();
  await expect.element(options().first()).toBeVisible();
}
async function closeBase() {
  await userEvent.keyboard("{Escape}");
  await expect.element(options().first()).not.toBeInTheDocument();
}
const startBranchInput = () =>
  page.getByRole("button", { name: "起始分支", exact: true });
async function chooseStartBranch(name: string) {
  await startBranchInput().click();
  await page.getByRole("option", { name, exact: true }).click();
}
const reference = () => page.getByRole("status", { name: "提交引用" });
// The start commit and path live in the folded advanced section.
async function show() {
  render(Harness, { repository });
  await page.getByRole("button", { name: /^高级：/ }).click();
}

afterEach(async () => {
  try {
    await cleanup();
    await userEvent.cleanup();
  } finally {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  }
});

test("lists recent commits of the start branch and selects one by title", async () => {
  serve();
  await show();
  await expect.element(baseInput()).toMatchTextContent(/最新提交/);
  await expect.element(reference()).toHaveTextContent(head);

  await openBase();
  await expect.element(options()).toHaveLength(2);
  await expect
    .element(options().first())
    .toHaveAccessibleName("最新提交 · aaaaaaa · 本地最新 · 3小时前");
  await options().getByText("较早提交").click();
  await expect.element(reference()).toHaveTextContent(older);
  await expect.element(baseInput()).toMatchTextContent(/较早提交/);
  await expect.element(ready()).toHaveTextContent("ready");
});

test("switching the start branch lists that branch's history", async () => {
  const other = "b".repeat(40);
  serve(200, {
    ...branches,
    branches: [...branches.branches, branch("release", other)],
  });
  await show();
  await expect.element(reference()).toHaveTextContent(head);
  await chooseStartBranch("release");
  await expect.element(reference()).toHaveTextContent(other);
  await openBase();
  await expect.element(options().first()).toMatchTextContent(/bbbbbbb/);
  await closeBase();
});

test("start branch picker searches and selects remote-tracking branches", async () => {
  const remoteHead = "c".repeat(40);
  const refreshedRemoteHead = "d".repeat(40);
  serve(
    200,
    {
      ...branches,
      bases: [
        ...branches.bases,
        base("refs/remotes/origin/HEAD", head, "修复终端重连"),
        base(
          "refs/remotes/origin/feature/remote-task",
          remoteHead,
          "远程功能提交",
        ),
      ],
    },
    (branch) => {
      if (branch === "origin/feature/remote-task") {
        return Response.json({
          branch,
          base: refreshedRemoteHead,
          baseSubject: "远程最新功能提交",
          baseSource: "remote",
          upstream: "origin/feature/remote-task",
          upstreamHead: refreshedRemoteHead,
          fetchWarning: "",
        });
      }
      return Response.json(localRefresh(branches, branch));
    },
  );
  await show();
  await startBranchInput().click();
  const search = page.getByRole("combobox", { name: "起始分支", exact: true });
  await search.fill("remote-task");
  await expect
    .element(
      page.getByRole("option", {
        name: "origin/feature/remote-task",
        exact: true,
      }),
    )
    .toBeVisible();
  await expect
    .element(page.getByRole("option", { name: "main", exact: true }))
    .not.toBeInTheDocument();
  await page
    .getByRole("option", { name: "origin/feature/remote-task", exact: true })
    .click();
  await expect
    .element(startBranchInput())
    .toMatchTextContent(/origin\/feature\/remote-task/);
  await expect.element(reference()).toHaveTextContent(refreshedRemoteHead);
  await expect
    .element(page.getByRole("status", { name: "上游刷新状态" }))
    .toMatchTextContent(
      /远程最新 · origin\/feature\/remote-task · 远程最新功能提交/,
    );
});

test("a detached default lists history from the main worktree commit", async () => {
  serve(200, {
    ...branches,
    bases: [],
    defaultBaseSubject: "独立提交 <title> & 引号",
    defaultBranch: null,
  });
  await show();
  await expect.element(reference()).toHaveTextContent(head);
  await openBase();
  await expect
    .element(options().first())
    .toHaveAccessibleName(/^最新提交 · aaaaaaa · 当前主工作区/);
  await closeBase();
  await expect.element(ready()).toHaveTextContent("ready");
});

test("a failed history read keeps the default base selectable", async () => {
  serve();
  const fetcher = vi.mocked(fetch);
  const base = fetcher.getMockImplementation()!;
  fetcher.mockImplementation(async (input, init) =>
    String(input).includes("/commits")
      ? Response.json({ error: "boom" }, { status: 500 })
      : base(input, init),
  );
  await show();
  await expect.element(page.getByText(/无法读取最近提交/)).toBeVisible();
  await openBase();
  await expect.element(options()).toHaveLength(1);
  await closeBase();
  await expect.element(reference()).toHaveTextContent(head);
  await expect.element(ready()).toHaveTextContent("ready");
});

test("refresh lists a diverged upstream tip without making it the default", async () => {
  const upstreamHead = "d".repeat(40);
  serve(200, branches, (branch) =>
    Response.json({
      ...localRefresh(branches, branch),
      upstream: "origin/main",
      upstreamHead,
      fetchWarning: "本地分支包含上游未有的提交，保留本地基线",
    }),
  );
  await show();
  await expect.element(page.getByText(/上游未有/)).toBeVisible();
  await openBase();
  await expect.element(options()).toHaveLength(3);
  await expect
    .element(options().first())
    .toHaveAccessibleName(/^最新提交 · ddddddd · 远程最新/);
  await expect
    .element(options().nth(1))
    .toHaveAccessibleName(/aaaaaaa · 本地最新/);
  await closeBase();
  await expect.element(reference()).toHaveTextContent(head);
});

test("prefills an editable suggestion and does not refill after a manual clear", async () => {
  vi.spyOn(Math, "random").mockReturnValue(0);
  serve();
  const expected = suggestWorktreeBranch(
    {
      branchNames: branches.branches.map((item) => item.name),
      worktreePaths: repository.worktrees.map((tree) => tree.path),
    },
    { random: () => 0 },
  );
  await show();

  await expect.element(branchInput()).toHaveValue(expected);
  await expect
    .element(pathInput())
    .toHaveValue(`/repos/project.worktrees/${expected.split("/").at(-1)}`);
  await expect.element(ready()).toHaveTextContent("ready");
  expect(expected.split("/").at(-1)).not.toBe("calm-river");

  await branchInput().fill("feature/my-task");
  await expect.element(branchInput()).toHaveValue("feature/my-task");
  await expect
    .element(pathInput())
    .toHaveValue("/repos/project.worktrees/my-task");

  await branchInput().fill("");
  await expect.element(branchInput()).toHaveValue("");
  await expect.element(ready()).toHaveTextContent("not-ready");

  await page.getByRole("button", { name: "换一个", exact: true }).click();
  await expect.element(branchInput()).toHaveValue(expected);
  await expect.element(ready()).toHaveTextContent("ready");
  await page.getByRole("button", { name: "换一个", exact: true }).click();
  await expect.element(branchInput()).not.toHaveValue(expected);
  await expect
    .poll(() => branchInput().element().value)
    .toMatch(/^wt\/[a-z]+-[a-z]+$/);
});

test("mode switch regenerates a new branch and a failed list stays empty", async () => {
  vi.spyOn(Math, "random").mockReturnValue(0);
  serve();
  await show();
  await expect
    .poll(() => branchInput().element().value)
    .toMatch(/^wt\/[a-z]+-[a-z]+$/);

  await page.getByRole("radio", { name: "使用已有分支", exact: true }).click();
  await expect.element(ready()).toHaveTextContent("not-ready");
  await page.getByRole("radio", { name: "创建新分支", exact: true }).click();
  await expect
    .poll(() => branchInput().element().value)
    .toMatch(/^wt\/[a-z]+-[a-z]+$/);

  await cleanup();
  serve(503);
  await show();
  await expect
    .element(page.getByRole("alert"))
    .toHaveTextContent("无法读取分支");
  await expect.element(branchInput()).toHaveValue("");
  await expect
    .element(page.getByRole("button", { name: "换一个", exact: true }))
    .toBeDisabled();
});

test("search selects the literal branch and ignores occupied matches without losing the choice", async () => {
  serve(200, {
    ...branches,
    branches: [
      ...branches.branches,
      branch("feature/branch-024", head),
      branch("feature/branch-042", head),
    ],
  });
  await show();
  await page.getByRole("radio", { name: "使用已有分支", exact: true }).click();
  const branchPicker = () =>
    page.getByRole("button", { name: "已有本地分支", exact: true });
  await branchPicker().click();
  const search = page.getByRole("combobox", {
    name: "已有本地分支",
    exact: true,
  });
  await expect.element(search).toBeEnabled();
  await search.fill("BRANCH-042");
  await expect
    .element(
      page.getByRole("option", { name: "feature/branch-042", exact: true }),
    )
    .toBeVisible();
  await expect
    .element(
      page.getByRole("option", { name: "feature/branch-024", exact: true }),
    )
    .not.toBeInTheDocument();
  await expect.element(ready()).toHaveTextContent("not-ready");
  await userEvent.keyboard("{Enter}");
  await expect
    .element(pathInput())
    .toHaveValue("/repos/project.worktrees/branch-042");
  await expect.element(ready()).toHaveTextContent("ready");
  await expect.element(branchPicker()).toHaveTextContent("feature/branch-042");

  await branchPicker().click();
  await search.fill("main");
  await expect
    .element(page.getByRole("option", { name: /main/ }))
    .toHaveAttribute("aria-disabled", "true");
  await userEvent.keyboard("{Enter}");
  await expect
    .element(pathInput())
    .toHaveValue("/repos/project.worktrees/branch-042");
  await search.fill("no-such-branch");
  await userEvent.keyboard("{Enter}");
  await expect.element(ready()).toHaveTextContent("ready");
  await expect
    .element(pathInput())
    .toHaveValue("/repos/project.worktrees/branch-042");
});

test("a preset existing branch opens in that mode with the branch chosen", async () => {
  serve(200, {
    ...branches,
    branches: [...branches.branches, branch("feat/d", head)],
  });
  render(Harness, {
    repository,
    preset: { mode: "existing", branch: "feat/d" },
  });
  await expect
    .element(page.getByRole("radio", { name: "使用已有分支", exact: true }))
    .toHaveAttribute("aria-checked", "true");
  const picker = page.getByRole("button", {
    name: "已有本地分支",
    exact: true,
  });
  await expect.element(picker).toHaveTextContent("feat/d");
  await page.getByRole("button", { name: /^高级：/ }).click();
  await expect.element(pathInput()).toHaveValue("/repos/project.worktrees/d");
  await expect.element(ready()).toHaveTextContent("ready");
  // Only the initial values: choosing another branch works as usual.
  await picker.click();
  await page
    .getByRole("option", { name: "wt/taken-branch", exact: true })
    .click();
  await expect.element(picker).toHaveTextContent("wt/taken-branch");
});

test("a preset remote start branch fills its short name as the new branch", async () => {
  const remoteHead = "c".repeat(40);
  serve(
    200,
    {
      ...branches,
      bases: [
        ...branches.bases,
        base("refs/remotes/origin/fix/b", remoteHead, "远程修复"),
      ],
    },
    (name) => Response.json(remoteRefresh(name, remoteHead, "远程修复")),
  );
  render(Harness, {
    repository,
    preset: { mode: "new", startBranch: "origin/fix/b", name: "fix/b" },
  });
  await expect
    .element(page.getByRole("radio", { name: "创建新分支", exact: true }))
    .toHaveAttribute("aria-checked", "true");
  await expect.element(startBranchInput()).toHaveTextContent("origin/fix/b");
  await expect.element(branchInput()).toHaveValue("fix/b");
  await expect.element(reference()).toHaveTextContent(remoteHead);
  await page.getByRole("button", { name: /^高级：/ }).click();
  await expect.element(pathInput()).toHaveValue("/repos/project.worktrees/b");
  await expect.element(ready()).toHaveTextContent("ready");
  // The preset name counts as typed: "换一个" still offers a suggestion.
  await page.getByRole("button", { name: "换一个", exact: true }).click();
  await expect.element(branchInput()).not.toHaveValue("fix/b");
});

test("reopening the branch picker starts unfiltered and mode switches close it", async () => {
  serve(200, branches);
  await show();
  await page.getByRole("radio", { name: "使用已有分支", exact: true }).click();
  const picker = () =>
    page.getByRole("button", { name: "已有本地分支", exact: true });
  await picker().click();
  const search = page.getByRole("combobox", {
    name: "已有本地分支",
    exact: true,
  });
  await search.fill("no-such-branch");
  await expect
    .element(page.getByText("没有匹配的本地分支"))
    .toBeInTheDocument();
  await userEvent.keyboard("{Escape}");
  await picker().click();
  await expect.element(search).toHaveValue("");
  await page.getByRole("radio", { name: "创建新分支", exact: true }).click();
  await page.getByRole("radio", { name: "使用已有分支", exact: true }).click();
  await expect.element(picker()).toHaveAttribute("aria-expanded", "false");
});

test("upstream refresh stays nonblocking and manual refresh updates the default", async () => {
  const first = deferred<Response>();
  const remoteHead = "b".repeat(40);
  const manualHead = "c".repeat(40);
  let calls = 0;
  serve(200, branches, (branch) => {
    calls += 1;
    return calls === 1
      ? first.promise
      : Response.json(remoteRefresh(branch, manualHead, "手动刷新"));
  });
  await show();

  await expect
    .element(page.getByRole("status", { name: "上游刷新状态" }))
    .toMatchTextContent(/正在获取 main/);
  await expect.element(reference()).toHaveTextContent(head);
  await expect.element(ready()).toHaveTextContent("ready");
  first.resolve(Response.json(remoteRefresh("main", remoteHead)));
  await expect.element(reference()).toHaveTextContent(remoteHead);
  await expect
    .element(page.getByRole("status", { name: "上游刷新状态" }))
    .toMatchTextContent(/远程最新 · origin\/main/);

  await page.getByRole("button", { name: "刷新上游", exact: true }).click();
  await expect.element(reference()).toHaveTextContent(manualHead);
  expect(calls).toBe(2);
});

test("a late response cannot replace a newer branch selection", async () => {
  const main = deferred<Response>();
  const selectedHead = "b".repeat(40);
  let mainSignal: AbortSignal | null = null;
  serve(200, branches, (branch, signal) => {
    if (branch === "main") {
      mainSignal = signal;
      return main.promise;
    }
    return Response.json(remoteRefresh(branch, selectedHead));
  });
  await show();
  await expect
    .element(page.getByRole("status", { name: "上游刷新状态" }))
    .toMatchTextContent(/正在获取 main/);

  await chooseStartBranch("wt/taken-branch");
  await expect.element(reference()).toHaveTextContent(selectedHead);
  expect(mainSignal?.aborted).toBe(true);
  const lateResponse = Response.json(remoteRefresh("main", "c".repeat(40)));
  const lateRead = vi.spyOn(lateResponse, "json");
  main.resolve(lateResponse);
  await vi.waitFor(() => expect(lateRead).toHaveBeenCalled());
  await Promise.resolve();
  await expect.element(reference()).toHaveTextContent(selectedHead);
  await expect
    .element(startBranchInput())
    .toMatchTextContent(/wt\/taken-branch/);
});

test("a refresh response preserves a commit picked after the request", async () => {
  const pending = deferred<Response>();
  serve(200, branches, () => pending.promise);
  await show();
  await expect
    .element(page.getByRole("status", { name: "上游刷新状态" }))
    .toMatchTextContent(/正在获取 main/);
  await openBase();
  await options().getByText("较早提交").click();
  const response = Response.json(remoteRefresh("main", "b".repeat(40)));
  const responseRead = vi.spyOn(response, "json");
  pending.resolve(response);
  await vi.waitFor(() => expect(responseRead).toHaveBeenCalled());
  await Promise.resolve();
  await expect.element(reference()).toHaveTextContent(older);
  await expect.element(baseInput()).toMatchTextContent(/较早提交/);
});

test("a failed manual refresh clears the previous remote-latest label", async () => {
  const remoteHead = "b".repeat(40);
  let calls = 0;
  serve(200, branches, (branch) => {
    calls += 1;
    return calls === 1
      ? Response.json(remoteRefresh(branch, remoteHead))
      : Response.json({ error: "fetch failed" }, { status: 500 });
  });
  await show();
  await expect
    .element(page.getByRole("status", { name: "上游刷新状态" }))
    .toMatchTextContent(/远程最新/);
  await page.getByRole("button", { name: "刷新上游", exact: true }).click();
  await expect
    .element(page.getByRole("status", { name: "上游刷新警告" }))
    .toMatchTextContent(/保留当前基线/);
  await expect
    .element(page.getByRole("status", { name: "上游刷新状态" }))
    .not.toBeInTheDocument();
  await expect.element(reference()).toHaveTextContent(remoteHead);
});

test("failed refreshes keep the local base and form availability", async () => {
  serve(200, branches, () =>
    Response.json({ error: "fetch failed" }, { status: 500 }),
  );
  await show();
  await expect
    .element(page.getByRole("status", { name: "上游刷新警告" }))
    .toMatchTextContent(/保留当前基线/);
  await expect.element(reference()).toHaveTextContent(head);
  await expect.element(ready()).toHaveTextContent("ready");
});

test("mode switches and unmount abort pending refreshes", async () => {
  const pending = deferred<Response>();
  const signals: Array<AbortSignal | null> = [];
  serve(200, branches, (_branch, signal) => {
    signals.push(signal);
    return pending.promise;
  });
  await show();
  await expect
    .element(page.getByRole("status", { name: "上游刷新状态" }))
    .toMatchTextContent(/正在获取 main/);
  await page.getByRole("radio", { name: "使用已有分支", exact: true }).click();
  expect(signals).toHaveLength(1);
  expect(signals[0]?.aborted).toBe(true);

  await page.getByRole("radio", { name: "创建新分支", exact: true }).click();
  await expect
    .element(page.getByRole("status", { name: "上游刷新状态" }))
    .toMatchTextContent(/正在获取 main/);
  expect(signals).toHaveLength(2);
  await cleanup();
  expect(signals[1]?.aborted).toBe(true);
});

test("leaving the form aborts the branch list reload after an upstream refresh", async () => {
  const reload = deferred<Response>();
  const started = deferred<AbortSignal | null>();
  let reads = 0;
  serve(
    200,
    branches,
    (branch) => Response.json(remoteRefresh(branch, "b".repeat(40))),
    (signal) => {
      if (++reads === 1) return Response.json(branches);
      started.resolve(signal);
      return reload.promise;
    },
  );
  await show();
  const signal = await started.promise;
  await expect.element(reference()).toHaveTextContent("b".repeat(40));

  await cleanup();
  expect(signal?.aborted).toBe(true);
  reload.resolve(Response.json(branches));
});
