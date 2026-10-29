import { afterEach, expect, test, vi } from "vitest";
import { page } from "vitest/browser";
import { cleanup, render } from "vitest-browser-svelte/pure";
import WorkspaceGroup from "../../src/components/WorkspaceGroup.svelte";
import { session, now } from "../../stories/fixtures";
import type { Worktree } from "../../src/schemas";

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
};

const transparent = "rgba(0, 0, 0, 0)";

for (const theme of ["light", "dark"])
  for (const available of [true, false])
    test(`${available ? "an available" : "an unavailable"} worktree's 启动会话 reads as a button before hover (${theme})`, async () => {
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
      const button = page.getByRole("button", { name: "启动会话" });
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

test("healthy worktrees keep paths, normal badges and status totals behind details", async () => {
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
  await expect
    .element(page.getByText(healthy.path, { exact: true }))
    .not.toBeVisible();
  await expect
    .element(page.getByText("主工作区", { exact: true }))
    .not.toBeVisible();
  await expect
    .element(page.getByText("已跟踪文件干净", { exact: true }))
    .not.toBeVisible();
  const details = page.getByRole("button", { name: "工作树详情 main" });
  await expect.element(details).toHaveAttribute("aria-expanded", "false");
  await details.click();
  await expect
    .element(page.getByText(healthy.path, { exact: true }))
    .toBeVisible();
  await expect
    .element(page.getByText("主工作区", { exact: true }))
    .toBeVisible();
  await expect
    .element(page.getByText("已跟踪文件干净", { exact: true }))
    .toBeVisible();
  await expect
    .element(page.getByText("共 1 个会话", { exact: true }))
    .toBeVisible();
  await details.click();
  await expect.element(details).toHaveAttribute("aria-expanded", "false");
  await expect
    .element(page.getByRole("link", { name: "Build agent" }))
    .toBeVisible();
});

test("dirty, untracked and locked warnings remain visible without expanding details", async () => {
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
  await expect
    .element(page.getByRole("button", { name: "工作树详情 main" }))
    .toHaveAttribute("aria-expanded", "false");
});
