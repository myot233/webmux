import { expect, test, vi } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte/pure";
import WorktreeDetailsDialog from "../../src/components/workspace/WorktreeDetailsDialog.svelte";
import { WorktreeUsageSchema } from "../../src/contracts/schemas";
import { commands, installChatFixture, respond } from "./chat-fixture";
import { worktree } from "./shell-fixture";

installChatFixture();

const usagePath = /^POST \/api\/repositories\/[^/]+\/worktrees\/[^/]+\/usage$/;

function open(patch: Parameters<typeof worktree>[0] = {}) {
  const tree = worktree(patch);
  render(WorktreeDetailsDialog, {
    title: "main",
    path: tree.path,
    worktree: tree,
    sessions: [],
    onclose: vi.fn(),
  });
  return tree;
}

test("the size is only walked when asked for, and a partial count says so", async () => {
  respond(usagePath, () =>
    Response.json(
      WorktreeUsageSchema.parse({
        bytes: 5 * 1024 * 1024,
        entries: 12345,
        complete: false,
      }),
    ),
  );
  open();
  const measure = page.getByRole("button", { name: "计算容量" });
  await expect.element(measure).toBeVisible();
  expect(commands()).toHaveLength(0);
  await measure.click();
  await expect
    .element(page.getByRole("status"))
    .toHaveTextContent("5 MiB · 12,345 个条目（未统计完整，实际更大）");
  expect(commands()).toHaveLength(1);
  expect(commands()[0]).toMatchObject({
    method: "POST",
    path: "/api/repositories/repo1/worktrees/main/usage",
  });
  await expect
    .element(page.getByRole("button", { name: "重新计算" }))
    .toBeEnabled();
});

test("a failed measurement shows the error and can be retried", async () => {
  respond(usagePath, () =>
    Response.json({ error: "无法读取工作树目录" }, { status: 503 }),
  );
  open();
  await page.getByRole("button", { name: "计算容量" }).click();
  await expect
    .element(page.getByRole("alert"))
    .toHaveTextContent("无法读取工作树目录");
  await expect
    .element(page.getByRole("button", { name: "计算容量" }))
    .toBeEnabled();
});

test("a worktree whose directory is missing cannot be measured", async () => {
  open({ available: false });
  await expect
    .element(page.getByRole("button", { name: "计算容量" }))
    .toBeDisabled();
});

test("the details add branch and upstream, the last commit and change counts", async () => {
  const tree = worktree({
    kind: "linked",
    branch: "feat/x",
    upstream: "origin/feat/x",
    ahead: 2,
    behind: 1,
    lastCommit: {
      hash: "b".repeat(40),
      subject: "修复登录跳转",
      author: "dev",
      committedAt: "2026-10-27T00:00:00Z",
    },
    changes: { staged: 1, unstaged: 2, untracked: 3, conflicted: 0 },
    dirty: true,
    hasUntracked: true,
    operation: "rebase",
  });
  render(WorktreeDetailsDialog, {
    title: "feat/x",
    path: tree.path,
    worktree: tree,
    sessions: [],
    lastFetchedAt: null,
    onclose: vi.fn(),
  });
  const dialog = page.getByRole("dialog", { name: "feat/x" });
  await expect.element(dialog).toMatchTextContent(/正在变基：Git 操作尚未完成/);

  const upstream = dialog.getByRole("region", { name: "分支与上游" });
  await expect.element(upstream).toMatchTextContent(/origin\/feat\/x/);
  await expect
    .element(upstream.getByRole("img", { name: "领先上游 2 个提交" }))
    .toBeVisible();
  await expect
    .element(upstream.getByRole("img", { name: "落后上游 1 个提交" }))
    .toBeVisible();
  // Never fetched: the counts are as of the local remote-tracking refs.
  await expect.element(upstream).toMatchTextContent(/上次获取\s*未知（近似/);

  const commit = dialog.getByRole("region", { name: "最近提交" });
  await expect.element(commit).toMatchTextContent(/修复登录跳转/);
  await expect.element(commit).toMatchTextContent("b".repeat(40));
  await expect.element(commit).toMatchTextContent(/dev/);

  const changes = dialog.getByRole("region", { name: "修改统计" });
  await expect
    .element(changes)
    .toMatchTextContent(/已暂存\s*1\s*未暂存\s*2\s*未跟踪\s*3\s*冲突\s*0/);
});

test("a main worktree without commits says so and shows no upstream", async () => {
  open({ head: null, lastCommit: null });
  const dialog = page.getByRole("dialog", { name: "main" });
  await expect
    .element(dialog.getByRole("region", { name: "最近提交" }))
    .toMatchTextContent(/尚无提交/);
  expect(dialog.getByText("上游", { exact: true }).elements()).toEqual([]);
  await expect
    .element(dialog.getByRole("region", { name: "修改统计" }))
    .toMatchTextContent(/已暂存\s*0/);
});
