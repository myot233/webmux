import { test, expect, createShell } from "./fixtures";
import type { Locator, Page } from "@playwright/test";

async function split(pane: Locator, direction: "左右" | "上下") {
  await pane.getByRole("button", { name: "分屏", exact: true }).click();
  await pane
    .page()
    .getByRole("menuitem", { name: `${direction}分屏 · 新建 Shell` })
    .click();
}
async function command(page: Page, pane: Locator, value: string) {
  await pane.locator(".xterm-helper-textarea").focus();
  await page.keyboard.type(value);
  await page.keyboard.press("Enter");
}
async function ready(pane: Locator) {
  await expect(pane.locator(".terminal-header .status-dot")).toHaveAttribute(
    "aria-label",
    /已连接/,
  );
  await expect(pane.locator(".xterm")).toBeAttached();
}

test("nested splits preserve terminal state, isolate input and restore layout", async ({
  page,
  request,
  workDir,
}, testInfo) => {
  await page.addInitScript(() => {
    // Trusted HTTP deployments expose getRandomValues, but not randomUUID.
    Object.defineProperty(crypto, "randomUUID", { value: undefined });
  });
  const id = await createShell(page, "Primary shell", workDir);
  const original = page.locator(`[data-session-id="${id}"]`);
  await original
    .locator(".xterm")
    .evaluate((element) => element.setAttribute("data-retained", "yes"));
  await command(
    page,
    original,
    "export SPLIT_ORIGINAL=original; printf 'original-pane\\n'",
  );
  await original.getByRole("button", { name: "草稿模式" }).click();
  const draftInput = original.locator(".terminal-composer textarea");
  await draftInput.fill("keep this draft");
  await split(original, "左右");
  await expect(page.locator(".split-terminal")).toHaveCount(2);
  const second = page.locator(".split-terminal").nth(1);
  await ready(second);
  await expect(original.locator(".xterm")).toHaveAttribute(
    "data-retained",
    "yes",
  );
  await expect(draftInput).toHaveValue("keep this draft");
  await command(
    page,
    second,
    "printf 'second-pane:%s\\n' \"${SPLIT_ORIGINAL:-independent}\"",
  );
  await expect(second.locator(".xterm-rows")).toContainText(
    "second-pane:independent",
  );
  await expect(original.locator(".xterm-rows")).not.toContainText(
    "second-pane",
  );
  await second.getByRole("button", { name: "代码", exact: true }).click();
  await expect(
    second.getByRole("region", { name: "代码工作区" }),
  ).toBeVisible();
  await expect(second.locator(".terminal-host")).toBeHidden();
  await second.getByRole("button", { name: "终端", exact: true }).click();
  await expect(second.locator(".terminal-host")).toBeVisible();
  await expect(original.locator(".xterm")).toHaveAttribute(
    "data-retained",
    "yes",
  );
  await split(second, "上下");
  await expect(page.locator(".split-terminal")).toHaveCount(3);
  await ready(page.locator(".split-terminal").nth(2));
  if (testInfo.project.name === "chromium-desktop") {
    const divider = page.getByRole("separator", { name: "调整窗格宽度" });
    await divider.focus();
    await page.keyboard.press("ArrowLeft");
    await expect(divider).toHaveAttribute("aria-valuenow", "45");
    // Wait for the split menu's modal pointer lock to finish closing.
    await divider.hover();
    const bounds = (await divider.boundingBox())!;
    await page.mouse.move(bounds.x + bounds.width / 2, bounds.y + 20);
    await page.mouse.down();
    await page.mouse.move(bounds.x + 100, bounds.y + 20);
    await page.mouse.up();
    await expect
      .poll(async () => Number(await divider.getAttribute("aria-valuenow")))
      .toBeGreaterThan(45);
  } else {
    await expect(page.locator(".split-terminal:visible")).toHaveCount(1);
    await page
      .getByRole("button", { name: "切换到窗格 1：Primary shell" })
      .click();
    await expect(original).toBeVisible();
    await expect(draftInput).toHaveValue("keep this draft");
  }
  const saved = await page.evaluate(
    (key) => localStorage.getItem(`webmux.layout.${key}`),
    id,
  );
  await page.reload();
  await expect(page.locator(".split-terminal")).toHaveCount(3);
  expect(
    await page.evaluate(
      (key) => localStorage.getItem(`webmux.layout.${key}`),
      id,
    ),
  ).toBe(saved);
  for (const pane of await page.locator(".split-terminal").all())
    await ready(pane);
  const sessions = await (await request.get("/api/sessions")).json();
  expect(sessions).toHaveLength(3);
  expect(
    sessions.every((session: { cwd: string }) => session.cwd === workDir),
  ).toBe(true);
  await page
    .getByRole("button", { name: "切换到窗格 1：Primary shell" })
    .click();
  await expect(original.locator(".terminal-composer textarea")).toHaveValue(
    "keep this draft",
  );
  for (const theme of ["light", "dark"] as const) {
    await page.emulateMedia({ colorScheme: theme });
    await expect(page.locator("html")).toHaveClass(
      theme === "dark" ? /dark/ : /^(?!.*dark)/,
    );
    const screenshot = await page.screenshot({
      path: `/tmp/webmux-split-pr-evidence/after-${testInfo.project.use.isMobile ? "mobile" : "desktop"}-${theme}.jpg`,
      type: "jpeg",
      quality: 75,
      scale: "css",
      animations: "disabled",
    });
    await testInfo.attach(`splits-${theme}`, {
      body: screenshot,
      contentType: "image/jpeg",
    });
  }
});

test("tmux prefix splits, focuses, zooms and closes only the pane", async ({
  page,
  request,
  workDir,
}, testInfo) => {
  test.skip(
    testInfo.project.name !== "chromium-desktop",
    "Keyboard shortcuts use a physical keyboard",
  );
  const id = await createShell(page, "Keyboard shell", workDir);
  await page.locator(".xterm-helper-textarea").focus();
  await page.keyboard.press("Control+b");
  await page.keyboard.press("Shift+5");
  await expect(page.locator(".split-terminal")).toHaveCount(2);
  const second = page.locator(".split-terminal").nth(1);
  await ready(second);
  const secondId = await second.getAttribute("data-session-id");
  await second.locator(".xterm-helper-textarea").focus();
  await page.keyboard.press("Control+b");
  await page.keyboard.press("o");
  await expect(
    page.locator(`[data-session-id="${id}"] .xterm-helper-textarea`),
  ).toBeFocused();
  await page.keyboard.press("Control+b");
  await page.keyboard.press("z");
  await expect(second).toBeHidden();
  await page.keyboard.press("Control+b");
  await page.keyboard.press("z");
  await expect(second).toBeVisible();
  await second.locator(".xterm-helper-textarea").focus();
  await page.keyboard.press("Control+b");
  await page.keyboard.press("x");
  await expect(page.locator(".split-terminal")).toHaveCount(1);
  const retained = await (
    await request.get(`/api/sessions/${secondId}`)
  ).json();
  expect(retained.status).not.toBe("exited");
  await command(
    page,
    page.locator(".split-terminal"),
    "printf 'shortcuts-do-not-leak\\n'",
  );
  await expect(page.locator(".xterm-rows")).toContainText(
    "shortcuts-do-not-leak",
  );
});

test("existing sessions can join a split and missing panes can be dismissed", async ({
  page,
  request,
  workDir,
}) => {
  const id = await createShell(page, "Anchor", workDir);
  const response = await request.post("/api/sessions", {
    data: {
      cmd: ["/bin/bash", "--noprofile", "--norc", "-i"],
      cwd: workDir,
      name: "Existing agent",
    },
  });
  expect(response.ok()).toBe(true);
  const existing = await response.json();
  await page.getByRole("button", { name: "分屏", exact: true }).click();
  await page.getByRole("menuitem", { name: "加入已有会话…" }).click();
  await page.getByRole("button", { name: "上下分屏", exact: true }).click();
  await page.getByRole("button", { name: /Existing agent/ }).click();
  await expect(page.locator(".split-terminal")).toHaveCount(2);
  const joined = page.locator(`[data-session-id="${existing.id}"]`);
  await ready(joined);
  expect(await (await request.get("/api/sessions")).json()).toHaveLength(2);
  await request.delete(`/api/sessions/${existing.id}`);
  await expect
    .poll(
      async () =>
        (await (await request.get(`/api/sessions/${existing.id}`)).json())
          .status,
    )
    .toBe("exited");
  await request.delete(`/api/sessions/${existing.id}?remove=1`);
  await page.reload();
  await expect(joined).toContainText("会话不存在或已被移除");
  await joined.getByRole("button", { name: "分屏", exact: true }).click();
  await page.getByRole("menuitem", { name: "关闭窗格（保留会话）" }).click();
  await expect(page.locator(".split-terminal")).toHaveCount(1);
  await expect(page.locator(`[data-session-id="${id}"]`)).toBeVisible();
});

test("failed split creation leaves the current terminal intact", async ({
  page,
  workDir,
}) => {
  await createShell(page, "Failure recovery", workDir);
  await page.route("**/api/sessions", (route) =>
    route.request().method() === "POST"
      ? route.fulfill({
          status: 400,
          json: { error: "configured shell unavailable" },
        })
      : route.continue(),
  );
  await split(page.locator(".split-terminal"), "左右");
  await expect(page.getByRole("alert")).toContainText(
    "configured shell unavailable",
  );
  await expect(page.locator(".split-terminal")).toHaveCount(1);
  await command(
    page,
    page.locator(".split-terminal"),
    "printf 'still-usable\\n'",
  );
  await expect(page.locator(".xterm-rows")).toContainText("still-usable");
});

test("a split preserves hybrid chat and terminal switching beside a shell", async ({
  page,
  request,
  workDir,
}) => {
  const id = await createShell(page, "Shell beside chat", workDir);
  const anchor = page.locator(`[data-session-id="${id}"]`);
  await anchor
    .locator(".xterm")
    .evaluate((element) => element.setAttribute("data-retained", "yes"));
  const config = await request.post("/api/launch-configs", {
    data: { name: "Chat agent", cmd: ["omp"], env: {}, hybrid: true },
  });
  expect(config.status()).toBe(201);
  const created = await request.post("/api/sessions", {
    data: {
      cwd: workDir,
      configId: (await config.json()).id,
      startSide: "acp",
      name: "Chat agent",
    },
  });
  expect(created.status()).toBe(201);
  const chatId = (await created.json()).id;
  await anchor.getByRole("button", { name: "分屏", exact: true }).click();
  await page.getByRole("menuitem", { name: "加入已有会话…" }).click();
  await page.getByRole("button", { name: /Chat agent/ }).click();
  const chat = page.locator(`[data-session-id="${chatId}"]`);
  await expect(
    chat.getByRole("textbox", { name: "消息", exact: true }),
  ).toBeEditable();
  await chat
    .getByRole("textbox", { name: "消息", exact: true })
    .fill("split chat stays independent");
  await chat.getByRole("button", { name: /^发送$/ }).click();
  await expect(
    chat.getByRole("list", { name: "对话", exact: true }),
  ).toContainText("echo: split chat stays independent");
  await chat
    .getByRole("tablist", { name: "会话视图", exact: true })
    .getByRole("tab", { name: "终端", exact: true })
    .click();
  await expect(chat.locator(".xterm-rows")).toContainText("prompt>");
  await expect(anchor.locator(".xterm")).toHaveAttribute(
    "data-retained",
    "yes",
  );
  await page
    .getByRole("button", { name: "切换到窗格 1：Shell beside chat" })
    .click();
  await expect(page).toHaveTitle(/Shell beside chat · WebMux$/);
  await expect(page.locator(".split-terminal")).toHaveCount(2);
});
