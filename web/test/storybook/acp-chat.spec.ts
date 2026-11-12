import { expect, test } from "@playwright/test";
import { show } from "./chat-story";
import { checkEachLayout, expectNoOverflow } from "./layouts";

// Hybrid sessions (issue #181): every state of the session page fits the
// layout matrix and keeps its primary control reachable.
const stories = [
  ["conversation", "button", "停止"],
  ["long-conversation", "button", "发送"],
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

for (const viewport of [
  { width: 320, height: 568 },
  { width: 375, height: 667 },
  { width: 390, height: 844 },
  { width: 568, height: 320 },
  { width: 667, height: 375 },
]) {
  test(`mobile options sheet fits ${viewport.width}×${viewport.height}`, async ({
    browser,
  }) => {
    const page = await browser.newPage({
      viewport,
      hasTouch: true,
      isMobile: true,
    });
    try {
      for (const theme of ["light", "dark"]) {
        await show(page, "claude-code", theme);
        await expect(
          page.getByRole("toolbar", { name: "会话选项" }),
        ).toBeHidden();
        const input = page.getByRole("textbox", { name: "消息", exact: true });
        await expect(input).not.toBeFocused();
        const trigger = page.getByRole("button", {
          name: "会话选项",
          exact: true,
        });
        await expect(trigger).toBeHidden();
        await input.tap();
        await expect(trigger).toBeInViewport({ ratio: 1 });
        await trigger.tap();
        const sheet = page.getByRole("dialog", {
          name: "会话选项",
          exact: true,
        });
        // Fractional borders can extend by a subpixel at the screen edge.
        await expect
          .poll(async () => {
            const box = await sheet.boundingBox();
            return (
              !!box &&
              box.x >= -1 &&
              box.y >= -1 &&
              box.x + box.width <= viewport.width + 1 &&
              box.y + box.height <= viewport.height + 1
            );
          })
          .toBe(true);
        const model = sheet.getByRole("button", { name: "模型", exact: true });
        await expect(model).toHaveText("Sonnet");
        const last = sheet.getByRole("combobox", { name: "模式", exact: true });
        await last.scrollIntoViewIfNeeded();
        await expect(last).toBeInViewport({ ratio: 1 });
        await expectNoOverflow(
          page,
          `${viewport.width}×${viewport.height} ${theme}`,
        );
        await model.tap();
        const search = sheet.getByRole("combobox", {
          name: "模型",
          exact: true,
        });
        await expect(search).toBeVisible();
        await search.fill("opus");
        await expect(sheet.getByRole("option", { name: /Opus/ })).toBeVisible();
        await expect(sheet.getByRole("option", { name: /Haiku/ })).toHaveCount(
          0,
        );
        await expectNoOverflow(
          page,
          `${viewport.width}×${viewport.height} ${theme} model search`,
        );
        await model.tap();
        await sheet
          .getByRole("button", { name: "关闭选项", exact: true })
          .tap();
        await expect(sheet).toBeHidden();
        await expect(trigger).toBeFocused();
      }
    } finally {
      await page.close();
    }
  });
}

test("options reveal from their trigger and can close midway without stale clipping", async ({
  page,
}) => {
  await page.setViewportSize({ width: 375, height: 667 });
  await page.emulateMedia({ reducedMotion: "no-preference" });
  await show(page, "claude-code", "light");
  await page.getByRole("textbox", { name: "消息", exact: true }).click();
  await expect
    .poll(() =>
      page
        .locator("[data-composer-card]")
        .evaluate((node) => (node as HTMLElement).style.height),
    )
    .toBe("");

  const reveal = await page.evaluate(async () => {
    document
      .querySelector<HTMLButtonElement>('button[aria-label="会话选项"]')!
      .click();
    await new Promise(requestAnimationFrame);
    const sheet = document.querySelector<HTMLElement>(
      '[data-slot="drawer-content"]',
    )!;
    const animation = sheet
      .getAnimations()
      .find(
        (item) =>
          item.effect instanceof KeyframeEffect &&
          item.effect.getKeyframes().some((frame) => frame.clipPath),
      );
    if (!animation || !(animation.effect instanceof KeyframeEffect))
      return null;
    animation.pause();
    animation.currentTime = 100;
    return {
      frames: animation.effect.getKeyframes().map((frame) => frame.clipPath),
      transform: getComputedStyle(sheet).transform,
      clip: getComputedStyle(sheet).clipPath,
    };
  });
  expect(reveal).not.toBeNull();
  expect(reveal!.frames[0]).not.toEqual(reveal!.frames[1]);
  expect(reveal!.clip).toContain("inset(");
  expect(reveal!.transform).toBe("none");

  const sheet = page.getByRole("dialog", { name: "会话选项", exact: true });
  const trigger = page.getByRole("button", { name: "会话选项", exact: true });
  await page.keyboard.press("Escape");
  await expect(sheet).toBeHidden();
  await expect(trigger).toBeFocused();
  await trigger.click();
  await expect(sheet).toBeVisible();
  await expect
    .poll(() => sheet.evaluate((node) => getComputedStyle(node).clipPath))
    .toBe("none");
  await expect(
    sheet.getByRole("button", { name: "模型", exact: true }),
  ).toHaveText("Sonnet");
  await sheet.getByRole("button", { name: "关闭选项", exact: true }).click();
  await expect(sheet).toBeHidden();
});

test("composer morph preserves control size and recovers from interrupted collapse", async ({
  page,
}) => {
  await page.setViewportSize({ width: 375, height: 667 });
  await page.emulateMedia({ reducedMotion: "no-preference" });
  await show(page, "claude-code", "light");
  const measured = await page.evaluate(async () => {
    const card = document.querySelector<HTMLElement>("[data-composer-card]")!;
    const action = card.querySelector<HTMLElement>("[data-composer-action]")!;
    const before = card.getBoundingClientRect().height;
    const buttonWidth = action.getBoundingClientRect().width;
    document
      .querySelector<HTMLTextAreaElement>('textarea[aria-label="消息"]')!
      .focus();
    await new Promise(requestAnimationFrame);
    const height = card
      .getAnimations()
      .find(
        (animation) =>
          animation.effect instanceof KeyframeEffect &&
          animation.effect.getKeyframes().some((frame) => frame.height),
      );
    if (!height || !(height.effect instanceof KeyframeEffect)) return null;
    for (const animation of card.getAnimations({ subtree: true })) {
      animation.pause();
      animation.currentTime = 100;
    }
    const box = card.getBoundingClientRect();
    const button = action.getBoundingClientRect();
    return {
      before,
      middle: box.height,
      end: Number.parseFloat(
        String(height.effect.getKeyframes().at(-1)!.height),
      ),
      buttonWidth,
      afterWidth: button.width,
      buttonBottom: button.bottom,
      cardBottom: box.bottom,
    };
  });
  expect(measured).not.toBeNull();
  expect(measured!.middle).toBeGreaterThan(measured!.before);
  expect(measured!.middle).toBeLessThan(measured!.end);
  expect(measured!.afterWidth).toBeCloseTo(measured!.buttonWidth);
  expect(measured!.buttonBottom).toBeLessThanOrEqual(measured!.cardBottom + 1);
  await page.evaluate(async () => {
    const card = document.querySelector<HTMLElement>("[data-composer-card]")!;
    card
      .getAnimations({ subtree: true })
      .forEach((animation) => animation.finish());
    await new Promise(requestAnimationFrame);
    const input = document.querySelector<HTMLTextAreaElement>(
      'textarea[aria-label="消息"]',
    )!;
    input.blur();
    await new Promise(requestAnimationFrame);
    await new Promise(requestAnimationFrame);
    input.focus();
  });
  await expect(page.locator("[data-chat-composer]")).not.toHaveAttribute(
    "data-compact",
  );
  await expect
    .poll(() =>
      page
        .locator("[data-composer-card]")
        .evaluate((node) => (node as HTMLElement).style.height),
    )
    .toBe("");
  await expect
    .poll(() =>
      page
        .locator("[data-composer-action]")
        .evaluate((node) => (node as HTMLElement).style.transform),
    )
    .toBe("");
  await expectNoOverflow(page, "interrupted composer morph");
});

test("reduced motion displays options immediately without a reveal or slide", async ({
  page,
}) => {
  await page.setViewportSize({ width: 320, height: 568 });
  await page.emulateMedia({ reducedMotion: "reduce" });
  await show(page, "claude-code", "dark");
  await page.getByRole("textbox", { name: "消息", exact: true }).click();
  expect(
    await page
      .locator("[data-composer-card]")
      .evaluate((node) =>
        node
          .getAnimations()
          .some(
            (animation) =>
              animation.effect instanceof KeyframeEffect &&
              animation.effect.getKeyframes().some((frame) => frame.height),
          ),
      ),
  ).toBe(false);
  await page.getByRole("button", { name: "会话选项", exact: true }).click();
  const sheet = page.getByRole("dialog", { name: "会话选项", exact: true });
  await expect(sheet).toBeVisible();
  expect(
    await sheet.evaluate((node) => ({
      animations: node.getAnimations().length,
      clip: getComputedStyle(node).clipPath,
      transform: getComputedStyle(node).transform,
    })),
  ).toEqual({ animations: 0, clip: "none", transform: "none" });
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

test("long conversation ticks jump to messages and stay out of narrow panes", async ({
  page,
}) => {
  for (const theme of ["light", "dark"]) {
    await page.setViewportSize({ width: 1280, height: 900 });
    await show(page, "long-conversation", theme);
    const navigation = page.getByRole("navigation", { name: "对话轮次" });
    await expect(navigation).toBeVisible();
    await expect(navigation.getByRole("button")).toHaveCount(18);
    const first = navigation.getByRole("button", {
      name: "第 1 轮 · 你：修复首页搜索的排序：置顶的工作区应排在最前。",
    });
    const agent = navigation.getByRole("button", {
      name: /^第 1 轮 · AI：/,
    });
    const colors = await Promise.all(
      [first, agent].map((item) =>
        item
          .locator("span")
          .evaluate((node) => getComputedStyle(node).backgroundColor),
      ),
    );
    expect(colors[0]).not.toBe(colors[1]);
    await first.click();
    await expect(first).toHaveAttribute("aria-current", "location");
    await expect
      .poll(() =>
        page
          .locator("[data-slot=scroll-area-viewport]")
          .first()
          .evaluate((node) => node.scrollTop),
      )
      .toBeLessThan(100);
    await expectNoOverflow(page, `1280×900 ${theme} turn navigation`);

    await page.setViewportSize({ width: 900, height: 900 });
    await expect(navigation).toBeVisible();
    await first.click();
    await expect(first).toHaveAttribute("aria-current", "location");
    await expectNoOverflow(page, `900×900 ${theme} intermediate-width ticks`);

    for (const viewport of [
      { width: 768, height: 800 },
      { width: 390, height: 844 },
    ]) {
      await page.setViewportSize(viewport);
      await expect(navigation).toBeHidden();
      await expectNoOverflow(
        page,
        `${viewport.width}×${viewport.height} ${theme} turn navigation`,
      );
    }
  }
});

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
