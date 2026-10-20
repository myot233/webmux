import { expect, test } from "@playwright/test";
import { overflowViolations } from "./overflow";
import { assertRequiredButtons } from "./controls";

import { stories } from "./cases";

for (const story of stories) {
  test(`${story} fits and keeps its controls reachable`, async ({
    page,
  }, testInfo) => {
    const { theme, textScale } = testInfo.project.metadata;
    const failures: string[] = [];
    page.on("pageerror", (error) => failures.push(error.message));
    await page.route("**/api/**", (route) => route.abort());
    await page.goto(
      `/iframe.html?id=ui-overflow--${story}&viewMode=story&globals=theme:${theme}`,
    );
    await expect(page.getByTestId("overflow-story")).toBeVisible();
    await page.evaluate(async (scale) => {
      document.documentElement.style.fontSize = `${Number(scale) * 100}%`;
      await document.fonts.ready;
    }, textScale);
    await expect(page.locator("html")).toHaveClass(
      theme === "dark" ? /dark/ : /^(?!.*\bdark\b)/,
    );
    if (story === "quick-command-error") {
      await page.getByRole("button", { name: "保存", exact: true }).click();
      await expect(page.getByRole("alert")).toBeVisible();
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
    await expect.poll(() => overflowViolations(page)).toEqual([]);

    const dialogs = page.locator('[role="dialog"], [role="alertdialog"]');
    if (await dialogs.count()) {
      const dialog = dialogs.first();
      const bounds = await dialog.boundingBox();
      expect(bounds).not.toBeNull();
      expect(bounds!.y).toBeGreaterThanOrEqual(-1);
      expect(bounds!.y + bounds!.height).toBeLessThanOrEqual(
        page.viewportSize()!.height + 1,
      );
    }
    const controls = (await dialogs.count())
      ? dialogs.first()
      : page.getByTestId("overflow-story");
    await assertRequiredButtons(controls, story);
    for (const button of await controls.getByRole("button").all()) {
      await expect(button).toBeVisible();
      await button.scrollIntoViewIfNeeded();
      await expect(button).toBeInViewport({ ratio: 1 });
      if (await button.isEnabled()) await button.click({ trial: true });
    }
    if (story === "workspace-dialog") {
      const consent = page.getByRole("checkbox", { name: "已检查工作区路径" });
      await consent.check();
      await expect(consent).toBeChecked();
    }
    if (story === "session-row")
      await expect(page.getByRole("link")).toHaveAccessibleName(
        /unbroken_identifier/,
      );
    if (story === "code-changes")
      await expect(page.getByRole("button").first()).toHaveAttribute(
        "title",
        /unbroken_identifier/,
      );
    expect(failures).toEqual([]);
  });
}
