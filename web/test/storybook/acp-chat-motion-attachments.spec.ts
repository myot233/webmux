import { expect, test } from "@playwright/test";
import { expectNoOverflow } from "./layouts";
import { show } from "./chat-story";

test("attachment additions and removals preserve composer input and settle", async ({
  page,
}) => {
  await page.setViewportSize({ width: 320, height: 568 });
  await page.emulateMedia({ reducedMotion: "no-preference" });
  await show(page, "conversation", "light");
  const input = page.getByRole("textbox", { name: "消息", exact: true });
  await input.fill("保留草稿与光标");
  await page.locator('[data-composer-card] input[type="file"]').setInputFiles([
    {
      name: "long-attachment-one.txt",
      mimeType: "text/plain",
      buffer: Buffer.from("one"),
    },
    {
      name: "long-attachment-two.txt",
      mimeType: "text/plain",
      buffer: Buffer.from("two"),
    },
  ]);
  const attachments = page.locator('ul[aria-label="待发送附件"]');
  await expect(attachments.getByRole("listitem")).toHaveCount(2);
  await attachments.getByRole("button").first().click();
  await expect(attachments.getByRole("listitem")).toHaveCount(1);
  await expect(input).toHaveValue("保留草稿与光标");
  await attachments.getByRole("button").click();
  await expect(attachments).toBeHidden();
  await expect
    .poll(() =>
      attachments.evaluate(
        (node) => (node.parentElement as HTMLElement).style.height,
      ),
    )
    .toBe("");
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.locator('[data-composer-card] input[type="file"]').setInputFiles({
    name: "reduced.txt",
    mimeType: "text/plain",
    buffer: Buffer.from("x"),
  });
  await expect(attachments).toBeVisible();
  expect(
    await attachments.evaluate(
      (node) => node.getAnimations({ subtree: true }).length,
    ),
  ).toBe(0);
  await expectNoOverflow(page, "attachments on iPhone SE");
});
