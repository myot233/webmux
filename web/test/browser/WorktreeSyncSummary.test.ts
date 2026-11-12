import { afterEach, expect, test } from "vitest";
import { page } from "vitest/browser";
import { cleanup, render } from "vitest-browser-svelte/pure";
import WorktreeSyncSummary from "../../src/components/workspace/WorktreeSyncSummary.svelte";
import type { Worktree } from "../../src/contracts/schemas";
import { worktree } from "./shell-fixture";

afterEach(async () => {
  await cleanup();
});

const linked = (patch: Partial<Worktree>) =>
  worktree({ kind: "linked", branch: "feat/x", ...patch });

test("ahead and behind counts carry their words as accessible names", async () => {
  render(WorktreeSyncSummary, {
    worktree: linked({ upstream: "origin/feat/x", ahead: 2, behind: 1 }),
  });
  await expect
    .element(page.getByRole("img", { name: "领先上游 2 个提交" }))
    .toHaveTextContent("2");
  await expect
    .element(page.getByRole("img", { name: "落后上游 1 个提交" }))
    .toHaveTextContent("1");
  await expect.element(page.getByText("origin/feat/x")).toBeVisible();
});

test.each([
  ["no upstream", { upstream: null }, "无上游"],
  [
    "a deleted upstream",
    { upstream: "origin/feat/x", upstreamGone: true },
    "上游已删除",
  ],
  ["counts not computed", { upstream: "origin/feat/x" }, "未计算"],
  ["an unreadable status", { changes: null, dirty: null }, "未知"],
  [
    "a synced branch",
    { upstream: "origin/feat/x", ahead: 0, behind: 0 },
    "已同步",
  ],
] as const)("%s says so instead of 0/0", async (_, patch, text) => {
  render(WorktreeSyncSummary, { worktree: linked(patch) });
  await expect.element(page.getByText(text, { exact: true })).toBeVisible();
  expect(page.getByRole("img").elements()).toEqual([]);
});

test("the indicator shows only what needs attention", async () => {
  render(WorktreeSyncSummary, {
    worktree: linked({ upstream: "origin/feat/x", ahead: 0, behind: 0 }),
    variant: "indicator",
  });
  expect(document.body.textContent?.trim()).toBe("");
  await cleanup();

  render(WorktreeSyncSummary, {
    worktree: linked({ upstream: "origin/feat/x", upstreamGone: true }),
    variant: "indicator",
  });
  await expect.element(page.getByText("上游已删除")).toBeVisible();
  expect(page.getByText("origin/feat/x").elements()).toEqual([]);
});

test("a detached worktree shows no upstream; changes and an operation do", async () => {
  render(WorktreeSyncSummary, {
    worktree: linked({
      branch: null,
      detached: true,
      operation: "rebase",
      changes: { staged: 1, unstaged: 2, untracked: 3, conflicted: 0 },
    }),
  });
  await expect.element(page.getByText("正在变基")).toBeVisible();
  await expect
    .element(page.getByText("已暂存 1 · 未暂存 2 · 未跟踪 3"))
    .toBeVisible();
  expect(page.getByText("无上游").elements()).toEqual([]);
});
