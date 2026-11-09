import { expect, test } from "@playwright/test";
import { expectNoOverflow } from "./layouts";
import { show } from "./chat-story";

test("plan and confirmed queue changes settle with editable rows intact", async ({
  page,
}) => {
  await page.setViewportSize({ width: 375, height: 667 });
  await page.emulateMedia({ reducedMotion: "no-preference" });
  await show(page, "conversation", "dark");
  const queue = page.getByRole("region", { name: "排队的消息" });
  const toggle = queue.getByRole("button", { name: /^排队/ });
  await toggle.click();
  const fields = queue.getByRole("textbox");
  await expect(fields).toHaveCount(2);
  const second = await fields.nth(1).inputValue();
  await queue.getByRole("button", { name: "上移", exact: true }).click();
  await expect(fields.first()).toHaveValue(second);
  await expect
    .poll(() => queue.locator("ol").evaluate((node) => node.style.height))
    .toBe("");
  await fields.first().fill("编辑仍可用");
  await fields.first().press("Tab");
  await expect(fields.first()).toHaveValue("编辑仍可用");
  await queue
    .getByRole("button", { name: "删除", exact: true })
    .first()
    .click();
  await expect(fields).toHaveCount(1);
  await toggle.click();
  await expect(queue.locator("ol")).toBeHidden();
  const plan = page.locator('[data-chat-plan="dock"]');
  await plan.getByRole("button").click();
  await expect(plan.getByRole("list")).toBeVisible();
  await page.emulateMedia({ reducedMotion: "reduce" });
  await plan.getByRole("button").click();
  await expect(plan.getByRole("list")).toBeHidden();
  await expectNoOverflow(page, "plan and queue on iPhone SE");
});
