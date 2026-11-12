import { afterEach, expect, test, vi } from "vitest";
import { page, userEvent } from "vitest/browser";
import { cleanup, render } from "vitest-browser-svelte/pure";
import WorkspaceGroup from "../../src/components/workspace/WorkspaceGroup.svelte";
import { session, now } from "../../stories/fixtures";
import type { Worktree } from "../../src/contracts/schemas";

const previousDark = document.documentElement.classList.contains("dark");
afterEach(() => {
  cleanup();
  document.documentElement.classList.toggle("dark", previousDark);
});

const worktree: Worktree = {
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
};

const transparent = "rgba(0, 0, 0, 0)";

for (const theme of ["light", "dark"])
  for (const available of [true, false])
    test(`${available ? "an available" : "an unavailable"} worktree's 新建会话 reads as a button before hover (${theme})`, async () => {
      document.documentElement.classList.toggle("dark", theme === "dark");
      const onstart = vi.fn();
      render(WorkspaceGroup, {
        title: "main",
        path: worktree.path,
        worktree: { ...worktree, available },
        sessions: [],
        now: Date.now(),
        onclose: () => {},
        onstart,
      });
      const button = page.getByRole("button", { name: "新建会话" });
      await expect.element(button).toBeVisible();
      // Touch screens never hover: the boundary must show at rest.
      const style = getComputedStyle(button.element());
      expect(
        (parseFloat(style.borderTopWidth) > 0 &&
          style.borderTopColor !== transparent) ||
          style.backgroundColor !== transparent,
      ).toBe(true);
      if (available) {
        await button.click();
        expect(onstart).toHaveBeenCalledOnce();
      } else await expect.element(button).toBeDisabled();
    });

const healthy: Worktree = {
  ...worktree,
  path: "/projects/webmux",
  kind: "main",
  branch: "main",
  available: true,
  dirty: false,
  hasUntracked: false,
  locked: false,
  lockReason: "",
  prunable: false,
  error: "",
};

test("healthy worktrees keep paths, normal badges and status totals in the details dialog", async () => {
  render(WorkspaceGroup, {
    title: "main",
    path: healthy.path,
    worktree: healthy,
    sessions: [{ ...session, title: "Build agent" }],
    now,
    compact: true,
    onclose: () => {},
    onstart: () => {},
  });
  await expect
    .element(page.getByRole("link", { name: "Build agent" }))
    .toBeVisible();
  expect(page.getByText(healthy.path, { exact: true }).elements()).toEqual([]);
  await page.getByRole("button", { name: "工作树详情 main" }).click();
  const dialog = page.getByRole("dialog");
  await expect
    .element(dialog.getByText(healthy.path, { exact: true }))
    .toBeVisible();
  await expect
    .element(dialog.getByText("主工作区", { exact: true }))
    .toBeVisible();
  await expect
    .element(dialog.getByText("已跟踪文件干净", { exact: true }))
    .toBeVisible();
  await expect
    .element(dialog.getByText("共 1 个会话", { exact: true }))
    .toBeVisible();
  await userEvent.keyboard("{Escape}");
  await expect.element(dialog).not.toBeInTheDocument();
  await expect
    .element(page.getByRole("link", { name: "Build agent" }))
    .toBeVisible();
});

test("dirty, untracked and locked warnings stay visible without opening details", async () => {
  render(WorkspaceGroup, {
    title: "main",
    path: healthy.path,
    worktree: { ...healthy, dirty: true, hasUntracked: true, locked: true },
    sessions: [],
    now,
    compact: true,
    onclose: () => {},
    onstart: () => {},
  });
  await expect.element(page.getByText("有修改", { exact: true })).toBeVisible();
  await expect
    .element(page.getByText("未跟踪文件", { exact: true }))
    .toBeVisible();
  await expect.element(page.getByText("已锁定", { exact: true })).toBeVisible();
  expect(page.getByRole("dialog").elements()).toEqual([]);
});
