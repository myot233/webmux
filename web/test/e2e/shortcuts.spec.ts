import { readFile } from "node:fs/promises";
import { test, expect, createShell, recordInput } from "./fixtures";

for (const theme of ["light", "dark"] as const) {
  test(`shortcut help stays accessible and scrolls in ${theme} theme`, async ({
    page,
    workDir,
  }, testInfo) => {
    if (testInfo.project.use.isMobile)
      await page.setViewportSize({ width: 320, height: 740 });
    await page.emulateMedia({ colorScheme: theme });
    await createShell(page, "Shortcut reference", workDir);
    const trigger = page.getByRole("button", { name: "快捷键", exact: true });
    await expect(trigger).toBeInViewport({ ratio: 1 });
    const bounds = (await trigger.boundingBox())!;
    expect(bounds.x + bounds.width).toBeGreaterThan(
      page.viewportSize()!.width - 24,
    );
    await testInfo.attach(`terminal-${theme}.png`, {
      body: await page.screenshot({ animations: "disabled" }),
      contentType: "image/png",
    });
    await expect(page.locator("html")).toHaveClass(
      theme === "dark" ? /dark/ : /^(?!.*dark)/,
    );
    await trigger.click();
    const dialog = page.getByRole("dialog", { name: "快捷键", exact: true });
    const shortcuts = dialog.getByRole("region", { name: "所有快捷键" });
    await expect(dialog).toBeVisible();
    await expect(dialog).toBeInViewport({ ratio: 1 });
    await expect(
      dialog.getByRole("heading", { name: "窗格管理" }),
    ).toBeVisible();
    await expect(
      dialog.getByText("向终端发送 Ctrl + B", { exact: true }),
    ).toBeVisible();
    await testInfo.attach(`shortcuts-${theme}.png`, {
      body: await page.screenshot({ animations: "disabled" }),
      contentType: "image/png",
    });
    await shortcuts.focus();
    await page.keyboard.press("End");
    await expect(
      dialog.getByRole("heading", { name: "列表与弹窗" }),
    ).toBeInViewport();
    expect(
      await dialog.evaluate(
        (element) => element.scrollWidth <= element.clientWidth,
      ),
    ).toBe(true);
    await page.setViewportSize({ width: 740, height: 320 });
    await expect(dialog).toBeInViewport({ ratio: 1 });
    await expect(
      dialog.getByRole("button", { name: "关闭", exact: true }),
    ).toBeInViewport({ ratio: 1 });
    await dialog.getByRole("button", { name: "关闭", exact: true }).click();
    await expect(dialog).toBeHidden();
    await expect(trigger).toBeFocused();
  });
}

test("help traps keyboard input, preserves drafts and returns focus on Escape", async ({
  page,
  workDir,
}) => {
  await createShell(page, "Help input isolation", workDir);
  const path = await recordInput(page, workDir);
  const trigger = page.getByRole("button", { name: "快捷键", exact: true });
  await trigger.click();
  const dialog = page.getByRole("dialog", { name: "快捷键", exact: true });
  await dialog.getByRole("region", { name: "所有快捷键" }).focus();
  await page.keyboard.press("Control+b");
  await page.keyboard.press("Shift+5");
  await page.keyboard.type("help-only");
  await expect(page.locator(".split-terminal")).toHaveCount(1);
  await expect(dialog).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(dialog).toBeHidden();
  await expect(trigger).toBeFocused();
  expect(await readFile(path, "utf8")).toBe("");

  await page.getByRole("button", { name: "草稿模式" }).click();
  const draft = page.locator(".terminal-composer textarea");
  await draft.fill("Keep this unsent draft");
  await trigger.click();
  await expect(dialog).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(dialog).toBeHidden();
  await expect(trigger).toBeFocused();
  await expect(draft).toHaveValue("Keep this unsent draft");
  expect(await readFile(path, "utf8")).toBe("");
});
