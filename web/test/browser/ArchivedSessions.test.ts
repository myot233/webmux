import { afterEach, expect, test, vi } from "vitest";
import { page, userEvent } from "vitest/browser";
import { cleanup, render } from "vitest-browser-svelte/pure";
import ArchivedSessions from "../../src/components/sessions/ArchivedSessions.svelte";
import type { ArchiveActions } from "../../src/domains/sessions/archived";
import type { ResumeOutcome } from "../../src/domains/sessions/recoverable";
import type {
  ArchivedSession,
  SessionInfo,
  Worktree,
} from "../../src/contracts/schemas";

afterEach(async () => {
  await cleanup();
  await userEvent.cleanup();
  vi.unstubAllGlobals();
});

const record: ArchivedSession = {
  id: "hybrid22",
  generation: "",
  title: "修复排序",
  cwd: "/repos/project",
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
  state: "interrupted",
  views: ["terminal", "chat"],
  view: "chat",
  transition: null,
  agent: { name: "omp", title: "omp", icon: "pi", sessionId: "01a" },
  archive: {
    configMissing: false,
    previousAgentSessionIds: [],
    queuedCount: 0,
    interruptedAt: "2026-10-28T00:00:00Z",
    updatedAt: "2026-10-28T00:00:00Z",
  },
  listeningPorts: [],
};
const updatedAt = record.archive.updatedAt;
const resumed = { id: record.id } as SessionInfo;
const now = Date.parse("2026-10-28T00:05:00Z");

function show(
  records: ArchivedSession[],
  resume: (...args: unknown[]) => Promise<ResumeOutcome>,
  patch: Partial<ArchiveActions> & {
    groupPath?: string;
    worktree?: Worktree;
  } = {},
) {
  const remove = vi.fn(async () => {});
  const onresumed = vi.fn();
  const { groupPath, worktree, ...actions } = patch;
  const archive: ArchiveActions = {
    now,
    filtering: false,
    resume,
    remove,
    onresumed,
    ...actions,
  };
  render(ArchivedSessions, {
    title: "main",
    records,
    groupPath,
    worktree,
    archive,
  });
  return { remove, onresumed };
}
const group = () => page.getByRole("region", { name: "main 的已归档会话" });
const expand = () =>
  group()
    .getByRole("button", { name: /已归档/ })
    .click();
const removeRecord = async () => {
  await page.getByRole("button", { name: "更多操作 修复排序" }).click();
  await page.getByRole("menuitem", { name: "删除记录" }).click();
};

test("is hidden without records and collapsed by default with a count", async () => {
  show([], vi.fn());
  expect(group().elements()).toHaveLength(0);
  await cleanup();
  show(
    [
      {
        ...record,
        archive: { ...record.archive, activity: "turn", queuedCount: 2 },
      },
      {
        ...record,
        id: "hybrid33",
        state: "closed",
        archive: { ...record.archive, activity: "turn" },
      },
    ],
    vi.fn(),
  );
  await expect.element(group()).toMatchTextContent(/已归档\s*2\s*1 条已中断/);
  expect(
    page.getByRole("button", { name: "恢复", exact: true }).elements(),
  ).toEqual([]);
  await expand();
  await expect.element(group().getByText("已中断")).toBeVisible();
  await expect.element(group().getByText("已关闭")).toBeVisible();
  await expect
    .element(group().getByText("/repos/project").first())
    .toBeVisible();
  await expect.element(group().getByText("5 分钟前").first()).toBeVisible();
  // Only the interrupted record warns; a closed one finished cleanly.
  expect(
    group()
      .getByText(/上一轮可能未完成/)
      .elements(),
  ).toHaveLength(1);
  await expect.element(group().getByText(/有 2 条排队消息/)).toBeVisible();
});

test("shows each session's title", async () => {
  show([record, { ...record, id: "hybrid33", title: "旧会话名称" }], vi.fn());
  await expand();
  await expect.element(group().getByText("修复排序")).toBeVisible();
  await expect.element(group().getByText("旧会话名称")).toBeVisible();
});

test.each(["chat", "terminal"] as const)(
  "resumes on the recorded %s view without asking for a view",
  async (view) => {
    const resume = vi.fn(async () => ({
      status: "resumed" as const,
      session: resumed,
    }));
    const { onresumed } = show([{ ...record, view }], resume);
    await expand();
    await page.getByRole("button", { name: "恢复", exact: true }).click();
    expect(resume).toHaveBeenCalledWith("hybrid22", view, {});
    await vi.waitFor(() => expect(onresumed).toHaveBeenCalledWith(resumed));
  },
);

test("asks which agent session to use when the recorded one is gone", async () => {
  const resume = vi
    .fn<(...args: unknown[]) => Promise<ResumeOutcome>>()
    .mockResolvedValueOnce({
      status: "candidates",
      message: "agent 会话 01a 已不存在",
      candidates: [
        {
          agentSessionId: "01b",
          title: "较早的对话",
          updatedAt,
        },
        { agentSessionId: "01c", title: "", updatedAt },
      ],
    })
    .mockResolvedValueOnce({ status: "resumed", session: resumed });
  const { onresumed } = show([record], resume);
  await expand();
  await page.getByRole("button", { name: "恢复", exact: true }).click();
  const dialog = page.getByRole("alertdialog");
  await expect.element(dialog).toMatchTextContent(/01a 已不存在/);
  await dialog
    .getByRole("combobox", { name: "选择要恢复的会话" })
    .selectOptions("01c");
  await dialog.getByRole("button", { name: "确认" }).click();
  await vi.waitFor(() => expect(onresumed).toHaveBeenCalledOnce());
  expect(resume).toHaveBeenLastCalledWith("hybrid22", "chat", {
    agentSessionId: "01c",
  });
});

test("confirms resuming without the deleted launch config's environment", async () => {
  const resume = vi
    .fn<(...args: unknown[]) => Promise<ResumeOutcome>>()
    .mockResolvedValueOnce({
      status: "config_missing",
      message: "启动配置已删除",
    })
    .mockResolvedValueOnce({ status: "resumed", session: resumed });
  show([{ ...record, view: "terminal" }], resume);
  await expand();
  await page.getByRole("button", { name: "恢复", exact: true }).click();
  const dialog = page.getByRole("alertdialog");
  await expect
    .element(dialog)
    .toMatchTextContent(/不再带有该配置保存的环境变量/);
  await dialog.getByRole("button", { name: "确认" }).click();
  await vi.waitFor(() =>
    expect(resume).toHaveBeenLastCalledWith("hybrid22", "terminal", {
      confirmWithoutEnv: true,
    }),
  );
});

test("a second question keeps the first answer", async () => {
  const resume = vi
    .fn<(...args: unknown[]) => Promise<ResumeOutcome>>()
    .mockResolvedValueOnce({
      status: "config_missing",
      message: "启动配置已删除",
    })
    .mockResolvedValueOnce({
      status: "candidates",
      message: "agent 会话 01a 已不存在",
      candidates: [{ agentSessionId: "01b", title: "", updatedAt }],
    })
    .mockResolvedValueOnce({ status: "resumed", session: resumed });
  const { onresumed } = show([record], resume);
  await expand();
  await page.getByRole("button", { name: "恢复", exact: true }).click();
  await page
    .getByRole("alertdialog")
    .getByRole("button", { name: "确认" })
    .click();
  await expect
    .element(page.getByRole("alertdialog"))
    .toMatchTextContent(/01a 已不存在/);
  await page
    .getByRole("alertdialog")
    .getByRole("button", { name: "确认" })
    .click();
  await vi.waitFor(() => expect(onresumed).toHaveBeenCalledOnce());
  expect(resume).toHaveBeenLastCalledWith("hybrid22", "chat", {
    confirmWithoutEnv: true,
    agentSessionId: "01b",
  });
});

test("removing needs confirmation and failures stay inline", async () => {
  const { remove } = show([record], vi.fn());
  await expand();
  await removeRecord();
  const dialog = page.getByRole("alertdialog");
  await expect.element(dialog).toMatchTextContent(/修复排序/);
  await dialog.getByRole("button", { name: "取消" }).click();
  expect(remove).not.toHaveBeenCalled();
  await removeRecord();
  await page
    .getByRole("alertdialog")
    .getByRole("button", { name: "确认" })
    .click();
  await vi.waitFor(() => expect(remove).toHaveBeenCalledWith("hybrid22"));

  await cleanup();
  show(
    [
      {
        ...record,
        cmd: ["claude"],
        agent: {
          ...record.agent,
          name: "claudecode",
          sessionId: "01a18715-8504-7020-9f3d-257900000000",
        },
      },
    ],
    vi.fn(async () => Promise.reject(new Error("网络中断"))),
  );
  await expand();
  await page.getByRole("button", { name: "恢复", exact: true }).click();
  await expect
    .element(page.getByRole("alert"))
    .toMatchTextContent(/无法恢复“修复排序”：网络中断/);
});

test("hides the directory a group already names and pages long lists", async () => {
  const records = Array.from({ length: 7 }, (_, i) => ({
    ...record,
    id: `hybrid${i}0`,
    title: `记录 ${i}`,
    state: "closed" as const,
  }));
  show(
    [...records, { ...record, id: "hybridsu", cwd: "/repos/project/web" }],
    vi.fn(),
    { groupPath: "/repos/project" },
  );
  await expand();
  expect(
    group().getByText("/repos/project", { exact: true }).elements(),
  ).toEqual([]);
  await expect.element(group().getByText("记录 4")).toBeVisible();
  expect(group().getByText("记录 5").elements()).toEqual([]);
  await group().getByRole("button", { name: "显示全部 8 条" }).click();
  await expect.element(group().getByText("记录 6")).toBeVisible();
  await expect.element(group().getByText("/repos/project/web")).toBeVisible();
});

const linked: Worktree = {
  id: "wt1",
  repositoryId: "repo1",
  path: "/repos/project-wt",
  kind: "linked",
  branch: "wt/fix",
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
};

test("deleting a linked worktree's record leaves the worktree alone", async () => {
  const removeRecordAndWorktree = vi.fn();
  const { remove } = show([record], vi.fn(), {
    worktree: linked,
    removeRecordAndWorktree,
  });
  await expand();
  await removeRecord();
  await page
    .getByRole("alertdialog")
    .getByRole("button", { name: "确认" })
    .click();
  await vi.waitFor(() => expect(remove).toHaveBeenCalledWith("hybrid22"));
  expect(removeRecordAndWorktree).not.toHaveBeenCalled();
});

test("deleting a record and its worktree opens the worktree confirmation", async () => {
  const removeRecordAndWorktree = vi.fn();
  show([record], vi.fn(), { worktree: linked, removeRecordAndWorktree });
  await expand();
  await page.getByRole("button", { name: "更多操作 修复排序" }).click();
  expect(
    page
      .getByRole("menuitem")
      .elements()
      .map((item) => item.textContent?.trim()),
  ).toEqual(["删除记录", "删除记录和工作树"]);
  await page.getByRole("menuitem", { name: "删除记录和工作树" }).click();
  expect(removeRecordAndWorktree).toHaveBeenCalledWith(record, linked);
});

test("filters open the section on their own", async () => {
  show([record], vi.fn(), { filtering: true });
  await expect
    .element(group().getByRole("button", { name: "恢复", exact: true }))
    .toBeVisible();
});

const feature: Worktree = {
  id: "wt1",
  repositoryId: "repo1",
  path: "/repos/project-wt",
  kind: "linked",
  branch: "feature-x",
  head: "c".repeat(40),
  detached: false,
  locked: false,
  lockReason: "",
  prunable: false,
  available: true,
  dirty: false,
  hasUntracked: false,
  hasIgnored: true,
  error: "",
  upstream: null,
  ahead: null,
  behind: null,
  upstreamGone: false,
  lastCommit: null,
  changes: { staged: 0, unstaged: 0, untracked: 0, conflicted: 0 },
  operation: null,
};
const released: HybridRecord = {
  ...record,
  cwd: "/repos/project-wt",
  released: {
    id: "rel1",
    repositoryId: "repo1",
    path: "/repos/project-wt",
    commit: "c".repeat(40),
    branch: "feature-x",
    releasedAt: "2026-10-28T00:01:00Z",
  },
};
const menuItems = async () => {
  await page.getByRole("button", { name: "更多操作 修复排序" }).click();
  const menu = page.getByRole("menu");
  await expect.element(menu).toBeVisible();
  return menu
    .getByRole("menuitem")
    .elements()
    .map((item) => item.textContent?.trim());
};
// Answers the rebuild preview and the rebuild the dialog sends.
function stubRebuild(preview: Record<string, unknown>) {
  const fetch = vi.fn<typeof globalThis.fetch>(async (input) =>
    String(input).endsWith("/rebuild-preview")
      ? Response.json({
          releaseId: "rel1",
          path: "/repos/project-wt",
          commit: "c".repeat(40),
          branch: "feature-x",
          mode: "branch",
          blockers: [],
          ...preview,
        })
      : Response.json(feature),
  );
  vi.stubGlobal("fetch", fetch);
  return fetch;
}
const rebuildCalls = (fetch: ReturnType<typeof stubRebuild>) =>
  fetch.mock.calls.filter(([input]) => String(input).endsWith("/rebuild"));

test("offers releasing a linked worktree's space between resuming and deleting", async () => {
  const release = vi.fn();
  show([record], vi.fn(), { worktree: feature, release });
  await expand();
  expect(await menuItems()).toEqual(["释放空间", "删除记录"]);
  await page.getByRole("menuitem", { name: "释放空间" }).click();
  expect(release).toHaveBeenCalledWith(feature);

  // Outside a releasable worktree only the deletion is offered.
  await cleanup();
  show([record], vi.fn(), { release });
  await expand();
  expect(await menuItems()).toEqual(["删除记录"]);
});

test("a released record is marked and deleting it removes only the record", async () => {
  const release = vi.fn();
  const removeRecordAndWorktree = vi.fn();
  const { remove } = show([released], vi.fn(), {
    worktree: feature,
    release,
    removeRecordAndWorktree,
  });
  await expand();
  const badge = group().getByTitle(
    `原路径 /repos/project-wt · 提交 ${"c".repeat(12)}`,
    { exact: true },
  );
  await expect.element(badge).toBeVisible();
  expect(badge.element().textContent).toContain("已释放空间");
  expect(await menuItems()).toEqual(["删除记录"]);
  await page.getByRole("menuitem", { name: "删除记录" }).click();
  await page
    .getByRole("alertdialog")
    .getByRole("button", { name: "确认" })
    .click();
  await vi.waitFor(() => expect(remove).toHaveBeenCalledWith("hybrid22"));
  expect(removeRecordAndWorktree).not.toHaveBeenCalled();
  expect(release).not.toHaveBeenCalled();
});

test("a released record rebuilds its worktree after confirmation, then resumes", async () => {
  const fetch = stubRebuild({});
  const resume = vi.fn(async () => ({
    status: "resumed" as const,
    session: resumed,
  }));
  const onrebuilt = vi.fn();
  const { onresumed } = show([released], resume, { onrebuilt });
  await expand();
  await page.getByRole("button", { name: "恢复", exact: true }).click();
  const dialog = page.getByRole("dialog");
  await expect.element(dialog).toMatchTextContent(/\/repos\/project-wt/);
  await expect.element(dialog).toMatchTextContent("c".repeat(12));
  await expect
    .element(dialog)
    .toMatchTextContent(/将在原路径检出分支 feature-x/);
  expect(resume).not.toHaveBeenCalled();
  await dialog.getByRole("button", { name: "重建并恢复" }).click();
  await vi.waitFor(() => expect(onresumed).toHaveBeenCalledWith(resumed));
  expect(onrebuilt).toHaveBeenCalledExactlyOnceWith("");
  expect(resume).toHaveBeenCalledWith("hybrid22", "chat", {});
  const [call] = rebuildCalls(fetch);
  expect(JSON.parse(String(call?.[1]?.body))).toEqual({
    releaseId: "rel1",
    mode: "branch",
    commit: "c".repeat(40),
  });
});

test("cancelling the rebuild changes nothing, and blockers disable it", async () => {
  const fetch = stubRebuild({
    mode: "detached",
    reason: "branchMoved",
  });
  const resume = vi.fn();
  show([released], resume);
  await expand();
  await page.getByRole("button", { name: "恢复", exact: true }).click();
  const dialog = page.getByRole("dialog");
  await expect
    .element(dialog)
    .toMatchTextContent(/分支 feature-x 已有新的提交.*detached/);
  await dialog.getByRole("button", { name: "取消" }).click();
  await expect.element(dialog).not.toBeInTheDocument();
  expect(resume).not.toHaveBeenCalled();
  expect(rebuildCalls(fetch)).toEqual([]);

  stubRebuild({ blockers: ["原路径已存在其他目录"] });
  await page.getByRole("button", { name: "恢复", exact: true }).click();
  await expect
    .element(page.getByRole("dialog"))
    .toMatchTextContent(/原路径已存在其他目录/);
  await expect
    .element(
      page.getByRole("dialog").getByRole("button", { name: "重建并恢复" }),
    )
    .toBeDisabled();
});

test("a resume failing after the rebuild hands its reason to the refresh", async () => {
  stubRebuild({});
  const onrebuilt = vi.fn();
  show(
    [released],
    vi.fn(async () => Promise.reject(new Error("agent 不可用"))),
    { onrebuilt },
  );
  await expand();
  await page.getByRole("button", { name: "恢复", exact: true }).click();
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "重建并恢复" })
    .click();
  // The refresh may regroup the record and replace this section, so the
  // page, not the section, shows why the resume failed.
  await vi.waitFor(() =>
    expect(onrebuilt).toHaveBeenCalledExactlyOnceWith(
      "无法恢复“修复排序”：agent 不可用",
    ),
  );
  expect(page.getByRole("alert").elements()).toEqual([]);
});

test("the refresh after a rebuild waits for the questions the resume asks", async () => {
  stubRebuild({});
  const onrebuilt = vi.fn();
  const resume = vi.fn(async () => ({
    status: "config_missing" as const,
    message: "启动配置已删除",
  }));
  show([released], resume, { onrebuilt });
  await expand();
  await page.getByRole("button", { name: "恢复", exact: true }).click();
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "重建并恢复" })
    .click();
  const question = page.getByRole("alertdialog");
  await expect
    .element(question)
    .toMatchTextContent(/不再带有该配置保存的环境变量/);
  expect(onrebuilt).not.toHaveBeenCalled();
  await question.getByRole("button", { name: "取消" }).click();
  await vi.waitFor(() => expect(onrebuilt).toHaveBeenCalledExactlyOnceWith(""));
});
