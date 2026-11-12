import { expect, test } from "@playwright/test";
import { assertRequiredButtons } from "./controls";

import { stories } from "./cases";
import { checkEachLayout, expectDialogFits, expectNoOverflow } from "./layouts";

test("worktree heading keeps its width above mobile actions", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=workspace-flat-layout--worktree-actions&viewMode=story&globals=theme:light",
  );
  const heading = page.getByRole("heading", {
    name: "codex/flatten-workspace-session-layout-for-mobile",
  });
  const start = page.getByRole("button", { name: "新建会话", exact: true });
  const menu = page.getByRole("button", { name: /^工作树菜单 / });

  await checkEachLayout(page, async ({ name }) => {
    await expect.soft(heading, name).toBeVisible();
    await expect.soft(start, name).toBeVisible();
    await expect.soft(menu, name).toBeVisible();

    const layout = await heading.evaluate((element) => {
      const title = element.parentElement!.getBoundingClientRect();
      const group = element.closest("section")!.getBoundingClientRect();
      const actions =
        element.parentElement!.nextElementSibling!.getBoundingClientRect();
      return {
        title: { width: title.width, top: title.top, bottom: title.bottom },
        groupWidth: group.width,
        actions: { top: actions.top, bottom: actions.bottom },
        mobile: !matchMedia("(min-width: 40rem)").matches,
      };
    });
    if (layout.mobile) {
      expect.soft(layout.title.width, name).toBeCloseTo(layout.groupWidth, 0);
      expect
        .soft(layout.actions.top, name)
        .toBeGreaterThanOrEqual(layout.title.bottom);
    } else {
      expect.soft(layout.actions.top, name).toBeLessThan(layout.title.bottom);
      expect
        .soft(layout.actions.bottom, name)
        .toBeGreaterThan(layout.title.top);
    }
    await expectNoOverflow(page, name);
    await menu.click();
    await expect
      .soft(page.getByRole("menuitem", { name: "从主页移除" }), name)
      .toBeVisible();
    await page.keyboard.press("Escape");
  });
});

for (const story of stories) {
  test(`${story} fits and keeps its controls reachable`, async ({ page }) => {
    const failures: string[] = [];
    page.on("pageerror", (error) => failures.push(error.message));
    await page.route("**/api/**", (route) => route.abort());
    await page.goto(
      `/iframe.html?id=ui-overflow--${story}&viewMode=story&globals=theme:light`,
    );
    await expect(page.getByTestId("overflow-story")).toBeVisible();

    await checkEachLayout(page, async ({ name, theme }) => {
      await expect
        .soft(page.locator("html"), name)
        .toHaveClass(theme === "dark" ? /dark/ : /^(?!.*\bdark\b)/);
      if (story === "quick-command-error") {
        await page.getByRole("button", { name: "保存", exact: true }).click();
        await expect.soft(page.getByRole("alert"), name).toBeVisible();
      }
      await page.evaluate(async () => {
        await Promise.all(
          document
            .getAnimations()
            .filter(
              (animation) =>
                animation.effect?.getComputedTiming().endTime !== Infinity,
            )
            .map((animation) => animation.finished.catch(() => {})),
        );
      });
      await expectNoOverflow(page, name);

      const dialogs = page.locator('[role="dialog"], [role="alertdialog"]');
      if (await dialogs.count()) {
        const dialog = dialogs.first();
        const bounds = await dialog.boundingBox();
        expect.soft(bounds, name).not.toBeNull();
        expect.soft(bounds!.y, name).toBeGreaterThanOrEqual(-1);
        expect
          .soft(bounds!.y + bounds!.height, name)
          .toBeLessThanOrEqual(page.viewportSize()!.height + 1);
      }
      const controls = (await dialogs.count())
        ? dialogs.first()
        : page.getByTestId("overflow-story");
      await assertRequiredButtons(controls, story);
      for (const button of await controls.getByRole("button").all()) {
        await expect.soft(button, name).toBeVisible();
        // Trial clicks already scroll and check stability. Keep an explicit
        // scroll for disabled controls, then assert full visibility in both cases.
        if (await button.isEnabled()) await button.click({ trial: true });
        else await button.scrollIntoViewIfNeeded();
        // Chromium can round the scroll offset by a fraction of a pixel.
        await expect.soft(button, name).toBeInViewport({ ratio: 0.99 });
      }
      if (story === "workspace-dialog") {
        const consent = page.getByRole("checkbox", {
          name: "已检查工作区路径",
        });
        await consent.check();
        await expect.soft(consent, name).toBeChecked();
        await consent.uncheck();
      }
      if (story === "session-row")
        await expect
          .soft(page.getByRole("link"), name)
          .toHaveAccessibleName(/unbroken_identifier/);
      if (story === "code-changes")
        await expect
          .soft(page.getByRole("button").first(), name)
          .toHaveAttribute("title", /unbroken_identifier/);
      expect.soft(failures, name).toEqual([]);
    });
  });
}

// The repository panel's sections and the worktree sync summary, with names
// and URLs long enough to wrap; a worktree's details open in the same
// dialog (drawer on phones) instead of a second one. The fetch stories keep
// "获取远程" running or show its per-remote results in every layout.
for (const story of [
  "overview",
  "worktrees",
  "branches",
  "remotes",
  "worktree-sync",
  "fetching",
  "fetch-partial",
]) {
  test(`repository panel ${story} fits every layout`, async ({ page }) => {
    // Twelve layouts, and the worktrees story also opens a subview in each.
    test.slow();
    const failures: string[] = [];
    page.on("pageerror", (error) => failures.push(error.message));
    await page.route("**/api/**", (route) => route.abort());
    await page.goto(
      `/iframe.html?id=workspace-repository-panel--${story}&viewMode=story&globals=theme:light`,
    );
    await expect(page.getByTestId("repository-story")).toBeVisible();
    const dialog = page.getByRole("dialog");
    const fetchButton = dialog.getByRole("button", { name: /获取远程$/ });
    if (story.startsWith("fetch")) await fetchButton.click();

    await checkEachLayout(page, async ({ name }) => {
      if (story === "fetching")
        await expect
          .soft(fetchButton, name)
          .toHaveAccessibleName("正在获取远程");
      if (story === "fetch-partial")
        await expect
          .soft(dialog.getByRole("list", { name: "获取结果" }), name)
          .toBeVisible();
      if (story.startsWith("fetch"))
        await expect.soft(fetchButton, name).toBeInViewport();
      if (story === "branches")
        await expect
          .soft(page.getByRole("region", { name: "本地分支" }), name)
          .toBeVisible();
      if (story === "remotes")
        await expect
          .soft(page.getByRole("list", { name: "远程列表" }), name)
          .toBeVisible();
      await expectNoOverflow(page, name);
      if (story !== "worktree-sync") await expectDialogFits(page, dialog, name);
      if (story === "worktrees") {
        await dialog
          .getByRole("list", { name: "仓库工作树列表" })
          .getByRole("button", { name: /^feature\/ahead-/ })
          .click();
        const back = dialog.getByRole("button", { name: "返回" });
        await expect.soft(back, name).toBeVisible();
        await expect.soft(dialog, name).toHaveCount(1);
        await expectNoOverflow(page, name);
        await back.click();
      }
      expect.soft(failures, name).toEqual([]);
    });
  });
}

// The switch of a worktree's branch, previewed with blockers, with sessions
// to acknowledge and with long names; the preview and its actions must fit
// the dialog (a drawer on phones) without overflowing.
for (const [story, shows] of [
  ["blockers", { role: "button", name: "改为在新工作树打开" }],
  ["sessions", { role: "checkbox", name: /我已了解/ }],
  ["long-names", { role: "button", name: "转到该工作树" }],
] as const) {
  test(`switch branch ${story} fits every layout`, async ({ page }) => {
    test.slow();
    const failures: string[] = [];
    page.on("pageerror", (error) => failures.push(error.message));
    await page.route("**/api/**", (route) => route.abort());
    await page.goto(
      `/iframe.html?id=workspace-switch-branch--${story}&viewMode=story&globals=theme:light`,
    );
    await expect(page.getByTestId("switch-story")).toBeVisible();
    const dialog = page.getByRole("dialog");
    const preview = dialog.getByRole("region", { name: "切换预览" });

    await checkEachLayout(page, async ({ name }) => {
      // Crossing the phone breakpoint swaps the dialog for a drawer, which
      // starts over without a chosen branch.
      if (!(await preview.count())) {
        await dialog
          .getByRole("button", { name: "目标分支", exact: true })
          .click();
        await page.getByRole("option", { name: /^feature\/target/ }).click();
      }
      await expect.soft(preview, name).toBeVisible();
      await expectNoOverflow(page, name);
      await expectDialogFits(page, dialog, name);
      for (const control of [
        dialog.getByRole(shows.role, { name: shows.name }),
        dialog.getByRole("button", { name: "切换", exact: true }),
        dialog.getByRole("button", { name: "取消", exact: true }),
      ]) {
        await control.scrollIntoViewIfNeeded();
        await expect.soft(control, name).toBeInViewport({ ratio: 0.99 });
      }
      expect.soft(failures, name).toEqual([]);
    });
  });
}

test("keeps a full scratch list and its composer reachable", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=workspace-scratch-notes--full&viewMode=story&globals=theme:light",
  );
  const dialog = page.getByRole("dialog", { name: "暂存", exact: true });
  await expect(dialog).toBeVisible();

  await checkEachLayout(page, async ({ name }) => {
    await expectNoOverflow(page, name);
    const add = page.getByRole("button", { name: "添加暂存", exact: true });
    await add.scrollIntoViewIfNeeded();
    await expect.soft(add, name).toBeInViewport();
    await expect.soft(add, name).toBeDisabled();
    await expect.soft(page.getByText(/已达 20 条上限/), name).toBeVisible();
    const last = page.getByRole("textbox", {
      name: "暂存第 20 条",
      exact: true,
    });
    await last.scrollIntoViewIfNeeded();
    await expect.soft(last, name).toBeInViewport();
    await expect.soft(add, name).toBeInViewport();
    await page
      .getByRole("button", { name: "暂存第 20 条的更多操作", exact: true })
      .click();
    await expect
      .soft(
        page.getByRole("menuitem", { name: "下移暂存第 20 条", exact: true }),
        name,
      )
      .toBeDisabled();
    await page
      .getByRole("menuitem", { name: "删除暂存第 20 条", exact: true })
      .click();
    await page
      .getByRole("textbox", { name: "新建暂存", exact: true })
      .fill("稍后检查移动端");
    await expect.soft(add, name).toBeEnabled();
    await add.click();
    await expect.soft(last, name).toHaveValue("稍后检查移动端");
    await expectNoOverflow(page, name);
  });
});

// The branch flows with long branch names and URLs, blockers, sessions and
// both delete groups: the dialog (drawer on phones) stays in the viewport.
for (const story of [
  "fast-forward",
  "fast-forward-blocked",
  "publish",
  "push-blocked",
  "delete-groups",
]) {
  test(`branch operation ${story} fits every layout`, async ({ page }) => {
    test.slow();
    const failures: string[] = [];
    page.on("pageerror", (error) => failures.push(error.message));
    await page.route("**/api/**", (route) => route.abort());
    await page.goto(
      `/iframe.html?id=workspace-branch-operations--${story}&viewMode=story&globals=theme:light`,
    );
    await expect(page.getByTestId("branch-story")).toBeVisible();
    const dialog = page.getByRole("dialog");
    await checkEachLayout(page, async ({ name }) => {
      await expect
        .soft(dialog.getByRole("button", { name: "取消" }), name)
        .toBeVisible();
      await expectNoOverflow(page, name);
      await expectDialogFits(page, dialog, name);
      expect.soft(failures, name).toEqual([]);
    });
  });
}
