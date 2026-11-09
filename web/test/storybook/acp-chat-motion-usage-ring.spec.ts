import { expect, test } from "@playwright/test";

test("usage reports update exact values immediately and leave no animation styles", async ({
  page,
}) => {
  await page.emulateMedia({ reducedMotion: "no-preference" });
  await page.goto("/iframe.html?id=chat-usage-updates--reports&viewMode=story");
  await page.getByRole("button", { name: "报告 80%", exact: true }).click();
  const usage = page.getByRole("button", { name: /^上下文已用 80%/ });
  await expect(usage).toBeVisible();
  await usage.click();
  const details = page.getByRole("dialog", { name: "Token 使用情况" });
  await expect(
    details.getByText("160,000 tokens", { exact: true }),
  ).toBeVisible();
  await expect(details.getByRole("progressbar")).toHaveAttribute(
    "aria-valuenow",
    "80",
  );
  // A report arriving while the popover stays open must update both visuals.
  await page
    .getByRole("button", { name: "报告 20%", exact: true })
    .evaluate((node: HTMLElement) => node.click());
  await expect(details.getByRole("progressbar")).toHaveAttribute(
    "aria-valuenow",
    "20",
  );
  await expect(
    details.getByText("40,000 tokens", { exact: true }),
  ).toBeVisible();
  await expect
    .poll(() =>
      page
        .locator("circle")
        .last()
        .evaluate((node) => node.style.strokeDasharray),
    )
    .toBe("");
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page
    .getByRole("button", { name: "报告 80%", exact: true })
    .evaluate((node: HTMLElement) => node.click());
  await expect(details.getByRole("progressbar")).toHaveAttribute(
    "aria-valuenow",
    "80",
  );
  await expect
    .poll(() =>
      page
        .locator("circle")
        .last()
        .evaluate((node) =>
          node
            .getAnimations()
            .some(
              (animation) =>
                animation.effect instanceof KeyframeEffect &&
                animation.effect
                  .getKeyframes()
                  .some((frame) => "strokeDasharray" in frame),
            ),
        ),
    )
    .toBe(false);
});
