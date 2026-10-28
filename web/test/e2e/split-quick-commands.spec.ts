import { test, expect, createShell } from "./fixtures";

test("compact split code view retains quick commands on a wide touch screen", async ({
  page,
  workDir,
}, testInfo) => {
  test.skip(!testInfo.project.use.isMobile, "Requires a coarse touch pointer");
  await page.setViewportSize({ width: 1280, height: 800 });
  expect(
    await page.evaluate(() => matchMedia("(any-pointer: coarse)").matches),
  ).toBe(true);
  const id = await createShell(page, "Wide touch quick commands", workDir);
  const original = page.locator(`[data-session-id="${id}"]`);
  await original.getByRole("button", { name: "分屏", exact: true }).click();
  await page.getByRole("menuitem", { name: "左右分屏 · 新建 Shell" }).click();
  await expect(page.locator(".split-terminal:visible")).toHaveCount(2);
  const second = page.locator(".split-terminal").nth(1);
  await expect(second.locator(".terminal-header .status-dot")).toHaveAttribute(
    "aria-label",
    /已连接/,
  );
  const entry = second
    .locator(".terminal-header")
    .getByRole("button", { name: "快捷指令", exact: true });
  const originalEntry = original
    .locator(".terminal-header")
    .getByRole("button", { name: "快捷指令", exact: true });
  const bar = second.getByRole("complementary", { name: "终端软键" });

  for (const forced of [false, true]) {
    if (forced) {
      await second
        .getByRole("button", { name: "终端菜单", exact: true })
        .click();
      await page
        .getByRole("menuitemcheckbox", { name: "强制显示底部工具栏" })
        .click();
      await page.keyboard.press("Escape");
    }
    await expect(entry).toBeHidden();
    await expect(bar).toBeVisible();
    await second.getByRole("button", { name: "代码", exact: true }).click();
    await expect(second.locator(".terminal-host")).toBeHidden();
    await expect(bar).toBeHidden();
    await expect(entry).toBeVisible();
    await expect(originalEntry).toBeHidden();
    await entry.click();
    await expect(page.getByRole("menu")).toBeVisible();
    await expect(
      page.getByRole("menuitem", { name: "管理快捷指令…", exact: true }),
    ).toBeEnabled();
    await page.keyboard.press("Escape");
    await second.getByRole("button", { name: "终端", exact: true }).click();
    await expect(entry).toBeHidden();
    await expect(bar).toBeVisible();
  }
});
