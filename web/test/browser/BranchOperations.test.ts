import { tick } from "svelte";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { page, userEvent } from "vitest/browser";
import { cleanup, render } from "vitest-browser-svelte/pure";
import { APIError } from "../../src/platform/api";
import type { BranchOperation } from "../../src/domains/workspace/branch-actions";
import type { BranchOperations } from "../../src/domains/workspace/branch-operations";
import BranchOperationDialog from "../../src/components/workspace/BranchOperationDialog.svelte";
import type {
  BranchDeleteItem,
  BranchDeletePreview,
  Commit,
  FastForwardPreview,
  PushPreview,
} from "../../src/contracts/schemas";
import { repository, session, worktree } from "./shell-fixture";

beforeEach(async () => {
  await page.viewport(1024, 900);
});
afterEach(async () => {
  try {
    await cleanup();
    await userEvent.cleanup();
  } finally {
    vi.restoreAllMocks();
  }
});

const commit = (n: number): Commit => ({
  hash: String(n).repeat(40).slice(0, 40),
  subject: `提交 ${n}`,
  author: "dev",
  committedAt: "2026-10-28T00:00:00Z",
});
const at = "2026-10-28T00:05:00Z";

function ffPreview(
  patch: Partial<FastForwardPreview> = {},
): FastForwardPreview {
  return {
    confirmationId: "ff1",
    expiresAt: at,
    branch: "feat/a",
    from: "a".repeat(40),
    to: "b".repeat(40),
    upstream: "origin/feat/a",
    commits: { total: 3, items: [commit(1), commit(2), commit(3)] },
    worktree: worktree({ id: "w1", kind: "linked", branch: "feat/a" }),
    sessions: [],
    blockers: [],
    warnings: [],
    ...patch,
  };
}

function pushPreview(patch: Partial<PushPreview> = {}): PushPreview {
  return {
    confirmationId: "push1",
    expiresAt: at,
    branch: "feat/c",
    commit: "c".repeat(40),
    remote: "origin",
    pushUrl: "https://user:***@example.invalid/o/r.git",
    target: "feat/c",
    publish: true,
    commits: { total: 1, items: [commit(4)] },
    blockers: [],
    warnings: [],
    ...patch,
  };
}

const item = (patch: Partial<BranchDeleteItem>): BranchDeleteItem => ({
  branch: "merged/old",
  commit: "d".repeat(40),
  group: "contained",
  unmerged: { total: 0, items: [] },
  blockers: [],
  ...patch,
});
const deletePreview = (
  items: BranchDeleteItem[],
  confirmationId = "del1",
): BranchDeletePreview => ({ confirmationId, expiresAt: at, items });
const contained = item({ branch: "merged/old" });
const squashed = item({
  branch: "feat/sq",
  commit: "e".repeat(40),
  group: "unmerged",
  unmerged: { total: 3, items: [commit(5), commit(6), commit(7)] },
});
const occupied = item({
  branch: "feat/a",
  commit: "f".repeat(40),
  blockers: [
    {
      code: "occupied",
      message: "分支 feat/a 已被工作树检出，不能删除",
      path: "/repos/webmux.worktrees/a",
    },
  ],
});

function fakeSource(patch: Partial<BranchOperations> = {}): BranchOperations {
  const repo = repository();
  return {
    fastForwardPreview: vi.fn(async () => ffPreview()),
    fastForward: vi.fn(async () => ({ repository: repo })),
    pushPreview: vi.fn(async () => pushPreview()),
    push: vi.fn(async () => ({ status: "ok", message: "", repository: repo })),
    deletePreview: vi.fn(async () => deletePreview([contained])),
    deleteBranches: vi.fn(async () => ({ results: [], repository: repo })),
    remotes: vi.fn(async () => ({
      remotes: [
        { name: "origin", fetchUrl: "o", pushUrl: "o" },
        { name: "fork", fetchUrl: "f", pushUrl: "f" },
      ],
    })),
    ...patch,
  } as BranchOperations;
}

function open(operation: BranchOperation, source: BranchOperations) {
  const handlers = { onchange: vi.fn(), onclose: vi.fn() };
  render(BranchOperationDialog, {
    repositoryId: "repo1",
    operation,
    source,
    ...handlers,
  });
  return handlers;
}

const dialog = () => page.getByRole("dialog");
const button = (name: string | RegExp) =>
  dialog().getByRole("button", { name, exact: typeof name === "string" });

test("fast-forward lists the commits and needs the sessions acknowledged", async () => {
  const running = session({ id: "run00001", title: "agent shell" });
  const source = fakeSource({
    fastForwardPreview: vi.fn(async () => ffPreview({ sessions: [running] })),
  });
  const handlers = open({ kind: "fast-forward", branch: "feat/a" }, source);
  await expect.element(dialog()).toHaveAccessibleName("快进到上游");
  const commits = dialog().getByRole("region", { name: "将前进的提交" });
  await expect.element(commits).toMatchTextContent(/提交 1.*提交 2.*提交 3/);
  await expect
    .element(dialog().getByText("agent shell", { exact: false }))
    .toBeVisible();
  const confirm = button("快进到上游");
  await expect.element(confirm).toBeDisabled();
  await dialog().getByRole("checkbox").click();
  await confirm.click();
  expect(source.fastForward).toHaveBeenCalledWith("repo1", "ff1", true);
  await expect
    .element(dialog().getByRole("status"))
    .toMatchTextContent(/已把 feat\/a 快进到 origin\/feat\/a/);
  expect(handlers.onchange).toHaveBeenCalledOnce();
  await button("完成").click();
  expect(handlers.onclose).toHaveBeenCalledOnce();
});

test("a blocked fast-forward explains why and cannot be confirmed", async () => {
  const source = fakeSource({
    fastForwardPreview: vi.fn(async () =>
      ffPreview({
        confirmationId: "",
        expiresAt: null,
        commits: { total: 0, items: [] },
        blockers: [
          {
            code: "diverged",
            message: "已分叉（领先 1、落后 2），需要在终端合并或变基",
          },
        ],
      }),
    ),
  });
  open({ kind: "fast-forward", branch: "feat/a" }, source);
  await expect
    .element(dialog().getByRole("alert"))
    .toMatchTextContent(/已分叉/);
  await expect.element(button("快进到上游")).toBeDisabled();
});

test("a 409 previews again and confirms with the new preview", async () => {
  let previews = 0;
  const source = fakeSource({
    fastForwardPreview: vi.fn(async () =>
      ffPreview({ confirmationId: `ff${++previews}` }),
    ),
    fastForward: vi
      .fn<BranchOperations["fastForward"]>()
      .mockRejectedValueOnce(new APIError(409, "分支已变化，请重新预览"))
      .mockResolvedValue({ repository: repository() }),
  });
  open({ kind: "fast-forward", branch: "feat/a" }, source);
  await button("快进到上游").click();
  await expect
    .element(dialog().getByText(/分支已变化.*已重新预览/))
    .toBeVisible();
  expect(source.fastForwardPreview).toHaveBeenCalledTimes(2);
  await button("快进到上游").click();
  expect(source.fastForward).toHaveBeenLastCalledWith("repo1", "ff2", false);
});

test("publishing offers the remote and name, and drops an outdated preview", async () => {
  const slow = Promise.withResolvers<PushPreview>();
  const pushPreviewFn = vi
    .fn<BranchOperations["pushPreview"]>()
    .mockResolvedValueOnce(pushPreview())
    .mockReturnValueOnce(slow.promise)
    .mockResolvedValueOnce(
      pushPreview({ remote: "origin", pushUrl: "https://example.invalid/b" }),
    );
  const source = fakeSource({ pushPreview: pushPreviewFn });
  const handlers = open(
    { kind: "push", branch: "feat/c", publish: true },
    source,
  );
  await expect.element(dialog()).toHaveAccessibleName("发布分支");
  await expect
    .element(dialog().getByText("https://user:***@example.invalid/o/r.git"))
    .toBeVisible();
  const remote = dialog().getByRole("combobox", { name: "远程" });
  await expect.element(remote).toHaveValue("origin");
  await expect
    .element(dialog().getByRole("textbox", { name: "远端分支名" }))
    .toHaveValue("feat/c");
  // fork is asked for first, then origin again; fork's late answer is
  // dropped.
  await remote.selectOptions("fork");
  await remote.selectOptions("origin");
  await expect
    .element(dialog().getByText("https://example.invalid/b"))
    .toBeVisible();
  slow.resolve(pushPreview({ remote: "fork", pushUrl: "https://fork/x" }));
  await slow.promise;
  await tick();
  await expect
    .element(dialog().getByText("https://example.invalid/b"))
    .toBeVisible();
  expect(dialog().getByText("https://fork/x").elements()).toEqual([]);
  expect(pushPreviewFn).toHaveBeenLastCalledWith(
    "repo1",
    { branch: "feat/c", remote: "origin", target: "feat/c" },
    expect.any(AbortSignal),
  );

  // A changed name is previewed before it can be published.
  const target = dialog().getByRole("textbox", { name: "远端分支名" });
  pushPreviewFn.mockResolvedValue(pushPreview({ target: "feat/renamed" }));
  await target.fill("feat/renamed");
  await expect.element(button("发布")).toBeEnabled();
  expect(pushPreviewFn).toHaveBeenLastCalledWith(
    "repo1",
    { branch: "feat/c", remote: "origin", target: "feat/renamed" },
    expect.any(AbortSignal),
  );
  await button("发布").click();
  expect(source.push).toHaveBeenCalledWith("repo1", "push1");
  await expect
    .element(dialog().getByRole("status"))
    .toMatchTextContent(/已发布到 origin\/feat\/renamed，并设为本地分支的上游/);
  expect(handlers.onchange).toHaveBeenCalledOnce();
});

test("a refused push says why and previews again", async () => {
  const source = fakeSource({
    pushPreview: vi.fn(async () =>
      pushPreview({ publish: false, remote: "origin", target: "feat/a" }),
    ),
    push: vi.fn(async () => ({
      status: "rejected" as const,
      message: "远端有新的提交，请先获取远程再快进",
      repository: repository(),
    })),
  });
  open({ kind: "push", branch: "feat/a", publish: false }, source);
  await expect.element(dialog()).toHaveAccessibleName("推送分支");
  expect(dialog().getByRole("combobox").elements()).toEqual([]);
  await button("推送").click();
  await expect
    .element(dialog().getByRole("alert"))
    .toMatchTextContent(/推送未完成：远端有新的提交，请先获取远程再快进/);
  expect(source.pushPreview).toHaveBeenCalledTimes(2);
});

test("deleting preselects contained branches and skips unmerged ones unless ticked", async () => {
  const writeText = vi
    .spyOn(navigator.clipboard, "writeText")
    .mockResolvedValue(undefined);
  const source = fakeSource({
    deletePreview: vi.fn(async () =>
      deletePreview([contained, squashed, occupied]),
    ),
    deleteBranches: vi.fn(async () => ({
      results: [
        {
          branch: "merged/old",
          status: "deleted" as const,
          commit: contained.commit,
          message: `已删除；可用 git branch merged/old ${contained.commit} 恢复`,
        },
      ],
      repository: repository(),
    })),
  });
  const handlers = open(
    {
      kind: "delete",
      branches: ["merged/old", "feat/sq", "feat/a"],
      cleanup: true,
    },
    source,
  );
  await expect.element(dialog()).toHaveAccessibleName("清理分支");
  const keep = dialog().getByRole("checkbox", { name: "删除 merged/old" });
  const unmerged = dialog().getByRole("checkbox", {
    name: "删除 feat/sq 及其 3 个独有提交",
  });
  await expect.element(keep).toBeChecked();
  await expect.element(unmerged).not.toBeChecked();
  await expect
    .element(dialog().getByRole("region", { name: "有独有提交" }))
    .toMatchTextContent(/提交 5.*提交 6.*提交 7/);
  await expect
    .element(dialog().getByRole("region", { name: "不能删除" }))
    .toMatchTextContent(/已被工作树检出.*\/repos\/webmux.worktrees\/a/);
  await button("删除 1 个分支").click();
  expect(source.deleteBranches).toHaveBeenCalledWith("repo1", "del1", false);
  expect(source.deletePreview).toHaveBeenCalledOnce();
  const results = dialog().getByRole("list", { name: "删除结果" });
  await expect.element(results).toMatchTextContent(/merged\/old.*已删除/);
  await results
    .getByRole("button", { name: "复制恢复命令：merged/old" })
    .click();
  expect(writeText).toHaveBeenCalledWith(
    `git branch merged/old ${contained.commit}`,
  );
  expect(handlers.onchange).toHaveBeenCalledOnce();
});

test("ticking every unmerged branch acknowledges them on the same preview", async () => {
  const source = fakeSource({
    deletePreview: vi.fn(async () => deletePreview([contained, squashed])),
  });
  open(
    { kind: "delete", branches: ["merged/old", "feat/sq"], cleanup: true },
    source,
  );
  await dialog()
    .getByRole("checkbox", { name: /^删除 feat\/sq/ })
    .click();
  await button("删除 2 个分支").click();
  expect(source.deletePreview).toHaveBeenCalledOnce();
  expect(source.deleteBranches).toHaveBeenCalledWith("repo1", "del1", true);
});

test("another selection is previewed on its own before deleting", async () => {
  const deletePreviewFn = vi
    .fn<BranchOperations["deletePreview"]>()
    .mockResolvedValueOnce(deletePreview([contained, squashed]))
    .mockResolvedValueOnce(deletePreview([squashed], "del2"));
  const source = fakeSource({ deletePreview: deletePreviewFn });
  open(
    { kind: "delete", branches: ["merged/old", "feat/sq"], cleanup: false },
    source,
  );
  await dialog().getByRole("checkbox", { name: "删除 merged/old" }).click();
  await dialog()
    .getByRole("checkbox", { name: /^删除 feat\/sq/ })
    .click();
  await button("删除 1 个分支").click();
  expect(deletePreviewFn).toHaveBeenLastCalledWith("repo1", ["feat/sq"]);
  expect(source.deleteBranches).toHaveBeenCalledWith("repo1", "del2", true);
});

test("a delete whose branches moved previews again", async () => {
  const source = fakeSource({
    deleteBranches: vi
      .fn<BranchOperations["deleteBranches"]>()
      .mockRejectedValue(new APIError(409, "分支已变化，请重新预览")),
  });
  open({ kind: "delete", branches: ["merged/old"], cleanup: false }, source);
  await button("删除 1 个分支").click();
  await expect
    .element(dialog().getByText(/分支已变化.*已重新预览/))
    .toBeVisible();
  expect(source.deletePreview).toHaveBeenCalledTimes(2);
  await expect.element(button("删除 1 个分支")).toBeEnabled();
});
