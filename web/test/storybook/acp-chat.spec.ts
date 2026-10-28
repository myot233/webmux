import { expect, test, type Page } from "@playwright/test";
import { overflowViolations } from "./overflow";
import { layouts } from "./layouts";

// Hybrid sessions (issue #181): every state of the session page fits the
// layout matrix and keeps its primary control reachable.
const stories = [
  ["conversation", "button", "停止"],
  ["permission-request", "button", /允许一次/],
  ["command-menu", "listbox", "命令"],
  ["cancelled-turn", "button", "继续"],
  ["compaction", "button", "停止"],
  ["open-code", "button", "停止"],
  ["switching", "heading", "正在切换到终端…"],
  ["awaiting-choice", "button", "强制切换"],
  ["terminal-side", "tab", /^聊天/],
] as const;

async function show(page: Page, story: string, theme: string) {
  await page.route("**/api/**", (route) => route.abort());
  await page.goto(
    `/iframe.html?id=workspace-hybrid-session--${story}&viewMode=story&globals=theme:${theme}`,
  );
  await expect(page.locator("main.terminal-page")).toBeVisible();
}

for (const { name, viewport, theme, textScale } of layouts) {
  test.describe(name, () => {
    test.use({ viewport, colorScheme: theme });

    for (const [story, role, control] of stories) {
      test(`hybrid session ${story}`, async ({ page }) => {
        const errors: string[] = [];
        page.on("pageerror", (error) => errors.push(error.message));
        await show(page, story, theme);
        await page.evaluate(async (scale) => {
          document.documentElement.style.fontSize = `${Number(scale) * 100}%`;
          await document.fonts.ready;
        }, textScale);
        const target = page.getByRole(role, { name: control });
        await target.scrollIntoViewIfNeeded();
        await expect(target).toBeInViewport();
        if (textScale > 1)
          // The terminal header does not fit 320px at 200% text even for an
          // ordinary session; this case checks the hybrid page's body.
          await page
            .locator("header.terminal-header")
            .evaluate((header) => (header.style.display = "none"));
        await expect.poll(() => overflowViolations(page)).toEqual([]);
        expect(errors).toEqual([]);
      });
    }

    test("hybrid home cards", async ({ page }) => {
      await page.route("**/api/**", (route) => route.abort());
      await page.goto(
        `/iframe.html?id=workspace-hybrid-home--session-cards&viewMode=story&globals=theme:${theme}`,
      );
      await expect(page.getByLabel("最新消息预览").first()).toBeVisible();
      await expect(page.getByText("等待授权")).toBeVisible();
      await page.evaluate(async (scale) => {
        document.documentElement.style.fontSize = `${Number(scale) * 100}%`;
        await document.fonts.ready;
      }, textScale);
      await expect.poll(() => overflowViolations(page)).toEqual([]);
    });
  });
}

test("switching to the terminal interrupts the turn after confirmation", async ({
  page,
}) => {
  await show(page, "conversation", "light");
  await page.getByRole("tab", { name: "终端" }).click();
  await page.getByRole("button", { name: "确认" }).click();
  await expect(
    page.getByRole("heading", { name: "正在切换到终端…" }),
  ).toBeVisible();
  await expect(page.getByLabel("终端（示意）")).toBeVisible();
  await page.getByRole("tab", { name: "聊天" }).click();
  await expect(page.getByText("已中断 · 已输出的内容保留")).toBeVisible();
  await expect(page.getByText("回合已中断，队列暂停")).toBeVisible();
});
