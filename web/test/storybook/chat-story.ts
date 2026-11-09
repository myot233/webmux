import { expect, type Page } from "@playwright/test";

export async function show(page: Page, story: string, theme: string) {
  await page.route("**/api/**", (route) => route.abort());
  await page.goto(
    `/iframe.html?id=workspace-hybrid-session--${story}&viewMode=story&globals=theme:${theme}`,
  );
  await expect(page.locator("main.terminal-page")).toBeVisible();
}
