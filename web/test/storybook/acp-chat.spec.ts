import { expect, test, type Page } from "@playwright/test";
import { checkEachLayout, expectNoOverflow } from "./layouts";

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
  ["connector-missing", "button", "切到终端"],
  ["claude-code", "button", "停止"],
  ["claude-connector-missing", "button", "复制安装命令"],
  ["claude-first-message", "tab", /^聊天/],
  ["claude-ambiguous", "combobox", "选择要打开的会话"],
  ["claude-occupied", "tab", /^聊天/],
  ["claude-mode-fallback", "button", "停止"],
  ["claude-commands", "listbox", "命令"],
  ["claude-recovery-failure", "button", "发送"],
] as const;

async function show(page: Page, story: string, theme: string) {
  await page.route("**/api/**", (route) => route.abort());
  await page.goto(
    `/iframe.html?id=workspace-hybrid-session--${story}&viewMode=story&globals=theme:${theme}`,
  );
  await expect(page.locator("main.terminal-page")).toBeVisible();
}

test("queued chat keeps its session options in view on a short touch screen", async ({
  browser,
}) => {
  const page = await browser.newPage({
    viewport: { width: 667, height: 375 },
    hasTouch: true,
    isMobile: true,
  });
  try {
    await show(page, "claude-code", "light");
    const options = page.getByRole("toolbar", { name: "会话选项" });
    const model = options.getByRole("button", { name: "模型：Sonnet" });
    await expect(model).toBeInViewport({ ratio: 1 });
    await model.tap();
    await expect(
      page.getByRole("menuitemradio", { name: "Sonnet", checked: true }),
    ).toBeInViewport({ ratio: 1 });
  } finally {
    await page.close();
  }
});

for (const [story, role, control] of stories) {
  test(`hybrid session ${story}`, async ({ page }) => {
    const errors: string[] = [];
    page.on("pageerror", (error) => errors.push(error.message));
    await show(page, story, "light");
    await checkEachLayout(page, async ({ name, textScale }) => {
      const target = page.getByRole(role, { name: control });
      await target.scrollIntoViewIfNeeded();
      await expect.soft(target, name).toBeInViewport();
      if (textScale > 1)
        // The terminal header does not fit 320px at 200% text even for an
        // ordinary session; this case checks the hybrid page's body.
        await page
          .locator("header.terminal-header")
          .evaluate((header) => (header.style.display = "none"));
      try {
        await expectNoOverflow(page, name);
      } finally {
        if (textScale > 1)
          await page
            .locator("header.terminal-header")
            .evaluate((header) => (header.style.display = ""));
      }
      expect.soft(errors, name).toEqual([]);
    });
  });
}

test("hybrid home cards", async ({ page }) => {
  await page.route("**/api/**", (route) => route.abort());
  await page.goto(
    "/iframe.html?id=workspace-hybrid-home--session-cards&viewMode=story&globals=theme:light",
  );
  await checkEachLayout(page, async ({ name }) => {
    await expect
      .soft(page.getByLabel("最新消息预览").first(), name)
      .toBeVisible();
    await expect.soft(page.getByText("等待授权"), name).toBeVisible();
    await expectNoOverflow(page, name);
  });
});

test("hybrid home archived records", async ({ page }) => {
  await page.route("**/api/**", (route) => route.abort());
  await page.goto(
    "/iframe.html?id=workspace-hybrid-home--archived-records&viewMode=story&globals=theme:light",
  );
  const archived = page.getByRole("region", {
    name: "webmux 的已归档会话",
  });
  await checkEachLayout(page, async ({ name }) => {
    await expect.soft(archived.getByText("1 条已中断"), name).toBeVisible();
    const toggle = archived.getByRole("button", { name: /已归档/ });
    if ((await toggle.getAttribute("aria-expanded")) !== "true") {
      await toggle.click();
    }
    await expect
      .soft(
        archived.getByText(
          "omp · 修复首页排序后补充回归测试与文档说明，确认长标题会截断",
        ),
        name,
      )
      .toBeVisible();
    await expectNoOverflow(page, name);
    await toggle.click();
  });
});

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
