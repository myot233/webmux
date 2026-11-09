import { expect, test } from "@playwright/test";
import { expectNoOverflow } from "./layouts";
import { show } from "./chat-story";

test("session indicator follows selection without skipping switch progress", async ({
  page,
}) => {
  await page.emulateMedia({ reducedMotion: "no-preference" });
  await show(page, "terminal-side", "light");
  const tabs = page.getByRole("tablist", { name: "会话视图" });
  await tabs.getByRole("tab", { name: "聊天", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: /正在切换到聊天/ }),
  ).toBeVisible();
  await expect(
    tabs.getByRole("tab", { name: "聊天", exact: true }),
  ).toHaveAttribute("aria-selected", "true");
  await expect(
    page.getByRole("textbox", { name: "消息", exact: true }),
  ).toBeVisible();
  const indicator = tabs.locator('span[aria-hidden="true"]');
  await expect(indicator).toBeVisible();
  await expect
    .poll(() => indicator.evaluate((node) => (node as HTMLElement).style.width))
    .toBe("");
  const tab = await tabs
    .getByRole("tab", { name: "聊天", exact: true })
    .boundingBox();
  const line = await indicator.boundingBox();
  expect(Math.abs(line!.x - tab!.x)).toBeLessThan(1);
  expect(Math.abs(line!.width - tab!.width)).toBeLessThan(1);
  await page.setViewportSize({ width: 320, height: 568 });
  await expectNoOverflow(page, "session tabs on iPhone SE");
});
