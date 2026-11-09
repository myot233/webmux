import { expect, test } from "@playwright/test";
import { expectNoOverflow } from "./layouts";
import { show } from "./chat-story";

test("tool disclosure reverses smoothly and settles without clipping", async ({
  page,
}) => {
  await page.emulateMedia({ reducedMotion: "no-preference" });
  await show(page, "conversation", "light");
  const group = page.locator("[data-tool-group]").first();
  const trigger = group.locator("[data-collapsible-trigger]").first();
  const content = group.locator("[data-collapsible-content]").first();
  await expect(group.locator("[data-tool-call]")).toHaveCount(0);
  const middle = await trigger.evaluate(async (node: HTMLElement) => {
    node.click();
    await new Promise<void>((resolve) =>
      requestAnimationFrame(() => resolve()),
    );
    const panel = document.getElementById(node.getAttribute("aria-controls")!)!;
    const animation = panel.getAnimations()[0];
    if (!animation) return null;
    animation.pause();
    animation.currentTime = 100;
    return {
      height: panel.getBoundingClientRect().height,
      full: panel.scrollHeight,
    };
  });
  expect(middle).not.toBeNull();
  expect(middle!.height).toBeGreaterThan(0);
  expect(middle!.height).toBeLessThan(middle!.full);
  await content.evaluate((node) =>
    node.getAnimations().forEach((animation) => animation.finish()),
  );
  await expect(trigger).toHaveAttribute("aria-expanded", "true");
  await expect
    .poll(() => content.evaluate((node) => (node as HTMLElement).style.height))
    .toBe("");
  await expect(content).toBeVisible();
  await trigger.evaluate((node: HTMLElement) => {
    node.click();
  });
  await expect(content).toHaveAttribute("inert", "");
  await trigger.evaluate((node: HTMLElement) => {
    node.click();
  });
  await expect(content).toBeVisible();
  await expect
    .poll(() => content.evaluate((node) => (node as HTMLElement).style.height))
    .toBe("");
  await page.setViewportSize({ width: 320, height: 568 });
  await expectNoOverflow(page, "expanded tool on iPhone SE");
  await page.emulateMedia({ reducedMotion: "reduce" });
  await trigger.click();
  await expect(content).toBeHidden();
  await expect(group.locator("[data-tool-call]")).toHaveCount(0);
  expect(await content.evaluate((node) => node.getAnimations().length)).toBe(0);
  await trigger.click();
  await expect(content).toBeVisible();
  expect(await content.evaluate((node) => node.getAnimations().length)).toBe(0);
});
