import { spawn, spawnSync } from "node:child_process";
import { once } from "node:events";
import { access, mkdir, readFile, rm, writeFile } from "node:fs/promises";
import { basename, join } from "node:path";
import { setTimeout as delay } from "node:timers/promises";
import type { APIRequestContext, Page } from "@playwright/test";
import {
  chooseLaunch,
  expect,
  expectTerminalLine,
  nameSession,
  openSessionLink,
  runCommand,
  test,
  useDirectory,
} from "./fixtures";
import type { E2EServer } from "./server.mjs";

// Hybrid sessions run the fake agent (internal/acp/testdata/fakeagent), which
// global-setup puts every supported fake agent and connector first on each
// backend's PATH. Its TUI does not echo typing: a line
// later run replays the stored conversation, and replies stream as
// "< echo: text". Its ACP side answers "echo: text" after a thought and a
// "Read file" tool call. FAKEAGENT_TURN_DELAY paces every streamed piece
// (TUI: every word).

type Side = "terminal" | "chat";
type Options = {
  name?: string;
  agent?:
    | "omp"
    | "opencode"
    | "codex"
    | "claude"
    | "kimi"
    | "qoder"
    | "qodercn";
  env?: Record<string, string>;
};

async function hybridConfig(
  request: APIRequestContext,
  name: string,
  { agent = "omp", env = {} }: Options,
): Promise<string> {
  const response = await request.post("/api/launch-configs", {
    data: { name, cmd: [agent], env, chat: true },
  });
  expect(response.status(), await response.text()).toBe(201);
  return (await response.json()).id;
}

async function createHybrid(
  request: APIRequestContext,
  cwd: string,
  view: Side,
  options: Options = {},
): Promise<string> {
  const name = options.name ?? `E2E ${view}`;
  const configId = await hybridConfig(request, name, options);
  const response = await request.post("/api/sessions", {
    data: { cwd, configId, view, name },
  });
  expect(response.status(), await response.text()).toBe(201);
  return (await response.json()).session.id;
}

const sideTab = (page: Page, side: Side) =>
  page
    .getByRole("tablist", { name: "会话视图", exact: true })
    .getByRole(
      "tab",
      side === "chat" ? { name: /^聊天/ } : { name: "终端", exact: true },
    );
const conversation = (page: Page) =>
  page.getByRole("list", { name: "对话", exact: true });
const composer = (page: Page) =>
  page.getByRole("textbox", { name: "消息", exact: true });
const working = (page: Page) =>
  page.getByRole("status").filter({ hasText: "正在工作…" });
const statusDot = (page: Page) => page.locator(".terminal-header .status-dot");

async function expectSide(page: Page, side: Side) {
  await expect(sideTab(page, side)).toHaveAttribute("aria-selected", "true");
  // The switch has settled: its progress is gone and the tabs work again.
  await expect(sideTab(page, side)).toBeEnabled();
  if (side === "chat") await expect(composer(page)).toBeEditable();
  else {
    await expect(page.locator("[data-chat-view]")).toHaveCount(0);
    await expect(page.locator(".xterm")).toBeVisible();
  }
}

async function openHybrid(page: Page, id: string, side: Side) {
  await page.goto(`/s/${id}`);
  await expectSide(page, side);
  if (side === "terminal") await expectTerminalLine(page, "prompt>");
}

async function sendChat(page: Page, text: string) {
  await composer(page).fill(text);
  await page.getByRole("button", { name: /^(发送|加入排队)$/ }).click();
  // The composer empties once the prompt is taken. A permission or form
  // request opening at once takes the composer's place, which counts too.
  await expect
    .poll(() =>
      composer(page).evaluateAll((boxes) =>
        boxes.every((box) => (box as HTMLTextAreaElement).value === ""),
      ),
    )
    .toBe(true);
}

async function chatTurn(page: Page, text: string) {
  await sendChat(page, text);
  await expect(conversation(page)).toContainText(`echo: ${text}`);
  await expect(working(page)).toHaveCount(0);
}

async function tuiTurn(page: Page, text: string) {
  await runCommand(page, text);
  await expectTerminalLine(page, `< echo: ${text}`);
  await expect(statusDot(page)).not.toHaveClass(/status-running/);
}

// Switches through the header tabs. Chat waits until the session is
// eligible: the server reads the TUI's session ID twice a second, then
// starts a short-lived ACP process to see that the agent lists it.
async function switchTo(page: Page, side: Side, confirm = false) {
  if (side === "chat")
    await expect(sideTab(page, "chat")).toHaveAccessibleName("聊天", {
      timeout: 20_000,
    });
  await sideTab(page, side).click();
  if (confirm) {
    const dialog = page.getByRole("alertdialog");
    await expect(dialog).toContainText(
      side === "chat" ? "切换到聊天？" : "切换到终端？",
    );
    await dialog.getByRole("button", { name: "确认", exact: true }).click();
  }
  await expectSide(page, side);
}

test.use({
  nextPromptCmd: [
    "/bin/sh",
    "-c",
    `input=$(cat)
       case "$input" in
         *"Suggest at most one short next message"*" /plan"*'pendingStep: "answer"'*)
           printf '%s\n' '请补全 answer 步骤，并运行相关测试。'
           ;;
       esac`,
  ],
});

test("next prompt suggestions preserve Tab, dismissal and live-turn boundaries", async ({
  page,
  request,
  workDir,
}) => {
  await page.goto("/settings");
  const setting = page.getByRole("switch", {
    name: "下一步提示词建议（实验）",
  });
  await expect(setting).not.toBeChecked();
  await setting.click();
  await expect(setting).toBeChecked();
  const id = await createHybrid(request, workDir, "chat", {
    name: "Suggestion source",
    env: { FAKEAGENT_TURN_DELAY: "120ms" },
  });
  await openHybrid(page, id, "chat");
  let requests = 0;
  page.on("request", (req) => {
    if (req.url().endsWith("/chat/next-prompt")) requests++;
  });
  const suggestion = page.getByRole("button", {
    name: "请补全 answer 步骤，并运行相关测试。",
    exact: true,
  });
  await chatTurn(page, "A normal turn");
  await expect(suggestion).toHaveCount(0);
  expect(requests).toBe(0);
  await chatTurn(page, "Check /plan");
  await expect(suggestion).toBeVisible();
  expect(requests).toBe(1);
  await composer(page).focus();
  await page.keyboard.press("Tab");
  await expect(composer(page)).not.toBeFocused();
  await expect(composer(page)).toHaveValue("");
  await page.getByRole("button", { name: "忽略", exact: true }).click();
  await expect(suggestion).toHaveCount(0);
  expect(
    await page.evaluate((session) => {
      const entries = JSON.parse(
        localStorage.getItem("webmux.nextPromptDismissals") ?? "[]",
      ) as [string, string][];
      return entries.some(([key, value]) => key === session && !!value);
    }, id),
  ).toBe(true);
  const other = await createHybrid(request, workDir, "chat", {
    name: "Other conversation",
  });
  for (const [session, name] of [
    [other, "Other conversation"],
    [id, "Suggestion source"],
  ] as const) {
    await page
      .getByRole("button", { name: "快速切换会话", exact: true })
      .click();
    await page
      .getByRole("dialog", { name: "快速切换会话", exact: true })
      .getByRole("option", { name: new RegExp(name) })
      .click();
    await expect(page).toHaveURL(new RegExp(`/s/${session}$`));
    await expectSide(page, "chat");
    await expect(suggestion).toHaveCount(0);
  }
  expect(requests).toBe(1);
  await page.reload();
  await expect(conversation(page)).toContainText("Check /plan");
  await expect(suggestion).toHaveCount(0);
  expect(requests).toBe(1);
  await chatTurn(page, "Try another /plan");
  await expect(suggestion).toBeVisible();
  expect(requests).toBe(2);
  // Even a non-dismissed suggestion must not be offered from history.
  await page.reload();
  await expect(conversation(page)).toContainText("Try another /plan");
  await expect(suggestion).toHaveCount(0);
  expect(requests).toBe(2);
  await chatTurn(page, "A fresh /plan");
  await suggestion.click();
  await expect(composer(page)).toHaveValue(
    "请补全 answer 步骤，并运行相关测试。",
  );
});

test.describe("hybrid sessions with the fake agent", () => {
  test("phones switch chat and terminal in tabs and open code with the header button", async ({
    page,
    request,
    workDir,
  }, testInfo) => {
    test.skip(!testInfo.project.use.isMobile, "A phone layout");
    const id = await createHybrid(request, workDir, "chat");
    await page.setViewportSize({ width: 393, height: 851 });
    await openHybrid(page, id, "chat");
    const header = page.locator(".terminal-header");
    const views = page.getByRole("tablist", { name: "会话视图", exact: true });
    await expect(views.getByRole("tab")).toHaveText(["聊天", "终端"]);
    // Code replaces the pane without moving the session, like the desktop
    // button: it turns pressed and no side stays selected.
    const code = header.getByRole("button", { name: "代码", exact: true });
    await code.click();
    await expect(
      page.getByRole("region", { name: "代码工作区" }),
    ).toBeVisible();
    await expect(sideTab(page, "chat")).toHaveAttribute(
      "aria-selected",
      "false",
    );
    // Choosing the side the session is already on only leaves the code view.
    await sideTab(page, "chat").click();
    await expectSide(page, "chat");
    await expect(page.getByRole("region", { name: "代码工作区" })).toBeHidden();
  });

  test("mobile chat scrolls the document while resting and falls back to the fixed shell on focus, code, or terminal", async ({
    page,
    request,
    workDir,
  }, testInfo) => {
    test.skip(!testInfo.project.use.isMobile, "A phone layout");
    test.slow();
    const id = await createHybrid(request, workDir, "chat", {
      agent: "claude",
    });
    await page.setViewportSize({ width: 390, height: 844 });
    await openHybrid(page, id, "chat");

    const terminalPage = page.locator(".terminal-page");
    await expect(terminalPage).toHaveAttribute("data-document-scroll", "true");
    await expect(conversation(page).locator("li")).toHaveCount(0);
    await expect(composer(page)).toBeInViewport();
    await expect
      .poll(() =>
        page.evaluate(() =>
          Math.round(
            document.querySelector("[data-chat-dock]")!.getBoundingClientRect()
              .bottom - window.innerHeight,
          ),
        ),
      )
      .toBe(0);

    for (let i = 0; i < 6; i++) {
      await chatTurn(
        page,
        `scroll-msg-${i} line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8`,
      );
    }
    // sendChat fills the composer (focusing it); blur to return to resting reading mode.
    await composer(page).evaluate((el: HTMLTextAreaElement) => el.blur());

    await expect(terminalPage).toHaveAttribute("data-document-scroll", "true");
    await expect
      .poll(() =>
        page.evaluate(
          () => getComputedStyle(document.documentElement).overflow,
        ),
      )
      .not.toBe("hidden");
    const initialMetrics = await page.evaluate(() => ({
      scrollHeight: document.scrollingElement!.scrollHeight,
      innerHeight: window.innerHeight,
      scrollY: window.scrollY,
      rootOverflow: getComputedStyle(document.documentElement).overflow,
      headerTop: document
        .querySelector(".terminal-header")!
        .getBoundingClientRect().top,
      dockBottom: document
        .querySelector("[data-chat-dock]")!
        .getBoundingClientRect().bottom,
    }));
    expect(initialMetrics.scrollHeight).toBeGreaterThan(
      initialMetrics.innerHeight,
    );
    expect(initialMetrics.scrollY).toBeGreaterThan(0);
    expect(initialMetrics.rootOverflow).not.toBe("hidden");
    expect(Math.round(initialMetrics.headerTop)).toBe(0);
    expect(Math.round(initialMetrics.dockBottom)).toBe(
      initialMetrics.innerHeight,
    );

    // Scroll up to a reading offset and verify sticky header/dock and jump button.
    await page.evaluate(() => {
      window.scrollTo(0, 120);
      window.dispatchEvent(new Event("scroll"));
    });
    await expect
      .poll(() => page.evaluate(() => Math.round(window.scrollY)))
      .toBe(120);
    const jump = page.getByRole("button", { name: "回到最新", exact: true });
    await expect(jump).toBeVisible();
    const stickyBounds = await page.evaluate(() => ({
      headerTop: Math.round(
        document.querySelector(".terminal-header")!.getBoundingClientRect().top,
      ),
      dockBottom: Math.round(
        document.querySelector("[data-chat-dock]")!.getBoundingClientRect()
          .bottom,
      ),
      innerHeight: window.innerHeight,
    }));
    expect(stickyBounds.headerTop).toBe(0);
    expect(stickyBounds.dockBottom).toBe(stickyBounds.innerHeight);

    // Simulating mobile browser toolbar collapse (viewport height grows) must not reset scrollY to 0.
    await page.setViewportSize({ width: 390, height: 900 });
    await expect
      .poll(() => page.evaluate(() => Math.round(window.scrollY)))
      .toBeGreaterThan(0);
    await page.setViewportSize({ width: 390, height: 844 });

    // Focusing the composer falls back to the fixed shell and preserves the reading offset.
    await page.evaluate(() => {
      window.scrollTo(0, 100);
      window.dispatchEvent(new Event("scroll"));
    });
    await expect
      .poll(() => page.evaluate(() => Math.round(window.scrollY)))
      .toBe(100);
    await composer(page).focus();
    await expect(terminalPage).not.toHaveAttribute(
      "data-document-scroll",
      "true",
    );
    await expect
      .poll(() =>
        page.evaluate(() => ({
          scrollY: Math.round(window.scrollY),
          scrollerTop: Math.round(
            document.querySelector<HTMLElement>(
              "[data-chat-view] [data-slot=scroll-area-viewport]",
            )?.scrollTop ?? 0,
          ),
          rootOverflow: getComputedStyle(document.documentElement).overflow,
        })),
      )
      .toEqual({ scrollY: 0, scrollerTop: 100, rootOverflow: "hidden" });

    // Blurring returns to document scroll and restores window.scrollY.
    await composer(page).evaluate((el: HTMLTextAreaElement) => el.blur());
    await expect(terminalPage).toHaveAttribute("data-document-scroll", "true");
    await expect
      .poll(() => page.evaluate(() => Math.round(window.scrollY)))
      .toBe(100);

    // Desktop pane width (>= 1024px) uses the fixed shell.
    await page.setViewportSize({ width: 1440, height: 720 });
    await expect(terminalPage).not.toHaveAttribute(
      "data-document-scroll",
      "true",
    );
    await page.setViewportSize({ width: 390, height: 844 });
    await expect(terminalPage).toHaveAttribute("data-document-scroll", "true");

    // Opening the code panel on mobile uses the fixed shell.
    const code = page
      .locator(".terminal-header")
      .getByRole("button", { name: "代码", exact: true });
    await code.click();
    await expect(terminalPage).not.toHaveAttribute(
      "data-document-scroll",
      "true",
    );
    await sideTab(page, "chat").click();
    await expect(terminalPage).toHaveAttribute("data-document-scroll", "true");

    // Switching to the terminal resets window.scrollY to 0 and fits the PTY cleanly.
    await switchTo(page, "terminal");
    await expect(terminalPage).not.toHaveAttribute(
      "data-document-scroll",
      "true",
    );
    expect(await page.evaluate(() => window.scrollY)).toBe(0);
  });

  test("wide document-scroll chat keeps turn ticks beside the app sidebar", async ({
    page,
    request,
    workDir,
  }, testInfo) => {
    test.skip(!!testInfo.project.use.isMobile, "A tablet-width desktop pane");
    const id = await createHybrid(request, workDir, "chat");
    await page.setViewportSize({ width: 1000, height: 900 });
    await openHybrid(page, id, "chat");
    for (let i = 0; i < 8; i++) {
      await chatTurn(
        page,
        `turn-nav-${i} line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8`,
      );
    }

    const navigation = page.getByRole("navigation", { name: "对话轮次" });
    await expect(navigation).toBeVisible();
    await expect
      .poll(() =>
        page
          .locator("[data-chat-view]")
          .evaluate((node) => node.getBoundingClientRect().width),
      )
      .toBeGreaterThanOrEqual(896);
    await expect
      .poll(() =>
        page
          .locator("[data-chat-view]")
          .evaluate((node) => node.getBoundingClientRect().width),
      )
      .toBeLessThan(1024);
    await expect
      .poll(() =>
        page.locator("[data-chat-view]").getAttribute("data-document-scroll"),
      )
      .toBe("true");
    const geometry = await page.evaluate(() => ({
      sidebarRight: document
        .querySelector(".app-sidebar")!
        .getBoundingClientRect().right,
      navigationLeft: document
        .querySelector('nav[aria-label="对话轮次"]')!
        .getBoundingClientRect().left,
    }));
    expect(geometry.navigationLeft).toBeGreaterThanOrEqual(
      geometry.sidebarRight,
    );

    const first = navigation.getByRole("button", {
      name: /^第 1 轮 · 你：turn-nav-0/,
    });
    await first.click();
    await expect(first).toHaveAttribute("aria-current", "location");
    await expect
      .poll(() => page.evaluate(() => window.scrollY))
      .toBeLessThan(100);
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth),
    ).toBeLessThanOrEqual(1000);

    await page.setViewportSize({ width: 1440, height: 900 });
    await expect(page.locator("[data-chat-view]")).not.toHaveAttribute(
      "data-document-scroll",
    );
    const panel = page.locator(".terminal-header, [data-chat-dock]");
    const panelOrigin = await panel.evaluateAll((nodes) =>
      nodes.map((node) => node.getBoundingClientRect().top),
    );
    const windowOrigin = await page.evaluate(() => window.scrollY);
    await first.click();
    await first.press("End");
    await expect(navigation.getByRole("button").last()).toBeFocused();
    expect(
      await panel.evaluateAll((nodes) =>
        nodes.map((node) => node.getBoundingClientRect().top),
      ),
    ).toEqual(panelOrigin);
    expect(await page.evaluate(() => window.scrollY)).toBe(windowOrigin);
  });

  test("phones gain the side tabs when the session info arrives", async ({
    page,
    request,
    workDir,
  }, testInfo) => {
    test.skip(!testInfo.project.use.isMobile, "A phone layout");
    const id = await createHybrid(request, workDir, "terminal");
    await page.setViewportSize({ width: 393, height: 851 });
    // Until the terminal stream connects the page cannot know the session is
    // hybrid: it shows the ordinary terminal, which has no side tabs.
    const gate = Promise.withResolvers<void>();
    await page.routeWebSocket("**/ws/s/*", async (socket) => {
      await gate.promise;
      socket.connectToServer();
    });
    await page.goto(`/s/${id}`);
    const views = page.getByRole("tablist", { name: "会话视图", exact: true });
    const code = page
      .locator(".terminal-header")
      .getByRole("button", { name: "代码", exact: true });
    await expect(code).toBeVisible();
    await expect(views).toHaveCount(0);
    const button = await code.elementHandle();
    gate.resolve();
    await expectSide(page, "terminal");
    await expect(views.getByRole("tab")).toHaveText(["聊天", "终端"]);
    // The code button stays the same element.
    expect(await button!.evaluate((node) => node.isConnected)).toBe(true);
  });

  for (const { start, agent } of [
    { start: "chat", agent: "omp" },
    { start: "terminal", agent: "opencode" },
    { start: "chat", agent: "claude" },
    { start: "terminal", agent: "claude" },
    { start: "terminal", agent: "kimi" },
    { start: "chat", agent: "qoder" },
    { start: "terminal", agent: "qodercn" },
  ] as const) {
    test(`an ${agent} conversation started on ${start === "chat" ? "chat" : "the terminal"} round-trips with its history on both sides`, async ({
      page,
      request,
      workDir,
    }) => {
      test.slow();
      const other: Side = start === "chat" ? "terminal" : "chat";
      const name = `E2E ${agent}`;
      await hybridConfig(request, name, { agent });
      await page.goto("/");
      await page
        .getByRole("main")
        .getByRole("button", { name: "＋ 新建会话", exact: true })
        .first()
        .click();
      await chooseLaunch(page, name);
      await page
        .getByRole("radiogroup", { name: "起始端", exact: true })
        .getByRole("radio", {
          name: start === "chat" ? "从聊天开始" : "从终端开始",
          exact: true,
        })
        .click();
      await useDirectory(page, workDir);
      await nameSession(page, "E2E round trip");
      await page
        .getByRole("button", { name: "创建并进入 →", exact: true })
        .click();
      await expect(page).toHaveURL(/\/s\/[a-z0-9]+$/);
      await expectSide(page, start);
      if (start === "terminal") await expectTerminalLine(page, "prompt>");

      const turn = (side: Side, text: string) =>
        side === "chat" ? chatTurn(page, text) : tuiTurn(page, text);
      const expectHistory = async (side: Side, texts: string[]) => {
        for (const text of texts) {
          if (side === "terminal") {
            await expectTerminalLine(page, `> ${text}`);
            await expectTerminalLine(page, `< echo: ${text}`);
          } else {
            await expect(conversation(page)).toContainText(text);
            await expect(conversation(page)).toContainText(`echo: ${text}`);
          }
        }
      };

      await turn(start, `${start}-one`);
      await switchTo(page, other);
      await expectHistory(other, [`${start}-one`]);
      await turn(other, `${other}-two`);
      await switchTo(page, start);
      await expectHistory(start, [`${start}-one`, `${other}-two`]);
      // The conversation goes on where it came back.
      await turn(start, `${start}-three`);
    });
  }

  test("Claude replays a long conversation on both sides", async ({
    page,
    request,
    workDir,
  }) => {
    test.slow();
    const id = await createHybrid(request, workDir, "chat", {
      agent: "claude",
    });
    await openHybrid(page, id, "chat");
    for (let i = 0; i < 24; i++) await chatTurn(page, `history-${i}`);
    await switchTo(page, "terminal");
    await expectTerminalLine(page, "> history-23");
    await expectTerminalLine(page, "< echo: history-23");
    await switchTo(page, "chat");
    await expect(conversation(page)).toContainText("history-0");
    await expect(conversation(page)).toContainText("echo: history-23");
  });

  test("Claude steers a running turn without starting an extra idle turn", async ({
    page,
    request,
    workDir,
  }) => {
    const id = await createHybrid(request, workDir, "chat", {
      agent: "claude",
      env: { FAKEAGENT_TURN_DELAY: "2s" },
    });
    await openHybrid(page, id, "chat");
    await sendChat(page, "steer-first");
    await expect(working(page)).toBeVisible();
    await expect(working(page)).toContainText(/· \d+ 秒/);
    await composer(page).fill("steer-nudge");
    await composer(page).press("Control+Enter");
    await expect(composer(page)).toHaveValue("");
    await expect(conversation(page)).toContainText("steer-nudge");
    await expect(conversation(page)).toContainText("echo: steer-first");
    await expect(working(page)).toHaveCount(0);
    await expect(
      page.getByRole("status").filter({ hasText: "已完成 · 用时" }),
    ).toContainText(/· 用时 \d+ 秒/);
    await chatTurn(page, "steer-after");
  });

  test("switching mid-turn interrupts the running side and keeps what it said", async ({
    page,
    request,
    workDir,
  }) => {
    test.slow();
    // Two seconds per streamed piece leave each turn running for several
    // seconds: four pieces in chat, one per word in the TUI.
    const id = await createHybrid(request, workDir, "chat", {
      agent: "claude",
      env: { FAKEAGENT_TURN_DELAY: "2s" },
    });
    await openHybrid(page, id, "chat");

    await sendChat(page, "chat-mid");
    await expect(working(page)).toBeVisible();
    await switchTo(page, "terminal", true);
    // The cancelled turn kept the prompt and produced no reply.
    await expectTerminalLine(page, "> chat-mid");
    await expect(statusDot(page)).not.toHaveClass(/status-running/);
    await expect(sideTab(page, "chat")).toHaveAccessibleName("聊天", {
      timeout: 20_000,
    });

    await runCommand(page, "tui-mid a b c");
    await expect(statusDot(page)).toHaveClass(/status-running/);
    await switchTo(page, "chat", true);
    // The interrupt key stopped the TUI, which saved what it had.
    await expect(conversation(page)).toContainText("tui-mid a b c");
    await expect(conversation(page)).toContainText("echo: (interrupted)");
    await chatTurn(page, "after-mid");
  });

  test("cancelling the interrupt confirmation does not reopen it", async ({
    page,
    request,
    workDir,
  }) => {
    const id = await createHybrid(request, workDir, "chat", {
      agent: "claude",
      env: { FAKEAGENT_TURN_DELAY: "2s" },
    });
    await openHybrid(page, id, "chat");

    await sendChat(page, "cancel-mid");
    await expect(working(page)).toBeVisible();
    await sideTab(page, "terminal").click();
    const dialog = page.getByRole("alertdialog");
    await expect(dialog).toContainText("切换到终端？");
    await dialog.getByRole("button", { name: "取消", exact: true }).click();
    await expect(dialog).toHaveCount(0);
    // Focus returns to the tab; that must not count as choosing it.
    await page.waitForTimeout(500);
    await expect(dialog).toHaveCount(0);
    await expectSide(page, "chat");
  });

  test("a permission request is answered once for every device", async ({
    page,
    request,
    browser,
    server,
    workDir,
  }, testInfo) => {
    const id = await createHybrid(request, workDir, "chat", {
      agent: "claude",
      env: { FAKEAGENT_PERMISSION: "1" },
    });
    // Sessions start in auto mode; ask so the request reaches the user.
    const mode = await request.put(`/api/sessions/${id}/chat/permission-mode`, {
      data: { clientRequestId: "e2e-permission-ask", mode: "ask" },
    });
    expect(mode.status(), await mode.text()).toBe(204);
    // Record the request IDs this device sees on the chat channel.
    const requestIds = new Set<string>();
    page.on("websocket", (socket) => {
      if (!new URL(socket.url()).pathname.endsWith("/chat")) return;
      socket.on("framereceived", ({ payload }) => {
        const frame = JSON.parse(String(payload));
        for (const item of frame.state?.requests ?? [])
          requestIds.add(item.requestId);
      });
    });
    await openHybrid(page, id, "chat");
    // A second device with this project's screen and input.
    const { viewport, isMobile, hasTouch, userAgent, deviceScaleFactor } =
      testInfo.project.use;
    const other = await browser.newContext({
      baseURL: server.baseURL,
      viewport,
      isMobile,
      hasTouch,
      userAgent,
      deviceScaleFactor,
    });
    try {
      const phone = await other.newPage();
      await openHybrid(phone, id, "chat");
      const devices = [page, phone];
      const prompt = (device: Page) =>
        device.getByRole("region", { name: "等待授权", exact: true });

      await sendChat(page, "needs-ok");
      for (const device of devices) {
        await expect(prompt(device)).toContainText("Read file");
        await expect(
          prompt(device).getByRole("button", { name: "Reject", exact: true }),
        ).toBeVisible();
      }
      await prompt(phone)
        .getByRole("button", { name: "Reject", exact: true })
        .click();
      for (const device of devices) {
        await expect(prompt(device)).toHaveCount(0);
        await expect(conversation(device)).toContainText(
          "echo: needs-ok (rejected)",
        );
      }
      // The first answer won: a late answer from this device is refused.
      expect(requestIds.size).toBe(1);
      const late = await request.post(
        `/api/sessions/${id}/chat/requests/${[...requestIds][0]}`,
        {
          data: {
            clientRequestId: "e2e-late-answer",
            outcome: "selected",
            optionId: "allow",
          },
        },
      );
      expect(late.status()).toBe(409);
      expect((await late.json()).reason).toBe("answered");

      // The next request, asked from the phone, is answered here by its key.
      await sendChat(phone, "needs-again");
      for (const device of devices)
        await expect(prompt(device)).toContainText("Read file");
      await page.keyboard.press("1");
      for (const device of devices) {
        await expect(prompt(device)).toHaveCount(0);
        await expect(conversation(device)).toContainText("echo: needs-again");
      }
      await expect(conversation(phone)).not.toContainText(
        "needs-again (rejected)",
      );
    } finally {
      await other.close();
    }
  });

  test("Claude sends image and embedded-resource attachments", async ({
    page,
    request,
    server,
    workDir,
  }) => {
    const id = await createHybrid(request, workDir, "chat", {
      agent: "claude",
      env: {
        FAKEAGENT_PROMPT_IMAGE: "1",
        FAKEAGENT_EMBEDDED_CONTEXT: "1",
      },
    });
    await openHybrid(page, id, "chat");
    const upload = async (
      name: string,
      mimeType: string,
      buffer: Buffer,
    ): Promise<string> => {
      const response = await request.post(`/api/sessions/${id}/upload`, {
        multipart: { file: { name, mimeType, buffer } },
      });
      expect(response.status(), await response.text()).toBe(201);
      return (await response.json()).path;
    };
    const image = await upload(
      "pixel.png",
      "image/png",
      Buffer.from(
        "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=",
        "base64",
      ),
    );
    const text = await upload(
      "note.txt",
      "text/plain",
      Buffer.from("attached context\n"),
    );
    const prompted = await request.post(`/api/sessions/${id}/input`, {
      data: {
        clientRequestId: "e2e-claude-attachments",
        text: "inspect attachments",
        attachments: [
          {
            path: image,
            name: "pixel.png",
            mimeType: "image/png",
            size: 68,
            kind: "image",
          },
          {
            path: text,
            name: "note.txt",
            mimeType: "text/plain",
            size: 17,
            kind: "file",
          },
        ],
      },
    });
    expect(prompted.status(), await prompted.text()).toBe(200);
    await expect(conversation(page)).toContainText("echo: inspect attachments");
    await expect(
      conversation(page).getByRole("img", { name: "pixel.png", exact: true }),
    ).toHaveAttribute("src", /^blob:/);
    await expect(working(page)).toHaveCount(0);

    const info = await (await request.get(`/api/sessions/${id}`)).json();
    const stored = JSON.parse(
      await readFile(
        join(
          server.env.FAKEAGENT_STORE,
          "sessions",
          `${info.agent.sessionId}.json`,
        ),
        "utf8",
      ),
    );
    const message = stored.messages.find(
      (item: { role: string; text: string }) =>
        item.role === "user" && item.text === "inspect attachments",
    );
    expect(
      message.blocks.map((block: { type: string }) => block.type).sort(),
    ).toEqual(["image", "resource", "text"]);
  });

  test("queued prompts pause after a stop until continued or cleared", async ({
    page,
    request,
    workDir,
  }) => {
    test.slow();
    // Each turn streams four pieces two seconds apart: time to queue behind it.
    const id = await createHybrid(request, workDir, "chat", {
      agent: "claude",
      env: { FAKEAGENT_TURN_DELAY: "2s" },
    });
    await openHybrid(page, id, "chat");
    const queue = page.getByRole("region", { name: "排队的消息", exact: true });
    const paused = queue.getByRole("status");

    await sendChat(page, "q-first");
    await expect(working(page)).toBeVisible();
    // With nothing typed the send button is the stop button.
    await expect(
      page.getByRole("button", { name: "停止", exact: true }),
    ).toBeVisible();
    await sendChat(page, "q-second");
    await sendChat(page, "q-third");
    await expect(queue).toContainText("排队 2");

    await page.getByRole("button", { name: "停止", exact: true }).click();
    await expect(paused).toHaveText("回合已中断，队列暂停");
    await expect(conversation(page)).toContainText("已中断 · 已输出的内容保留");
    await expect(working(page)).toHaveCount(0);
    await expect(queue).toContainText("排队 2");

    // Continuing sends from the head of the queue.
    await queue.getByRole("button", { name: "继续", exact: true }).click();
    await expect(queue).toContainText("排队 1");
    await expect(paused).toHaveCount(0);
    await expect(working(page)).toBeVisible();
    await expect(conversation(page)).toContainText("q-second");
    await page.getByRole("button", { name: "停止", exact: true }).click();
    await expect(paused).toHaveText("回合已中断，队列暂停");

    await queue.getByRole("button", { name: "清空", exact: true }).click();
    await expect(queue).toHaveCount(0);
    await expect(working(page)).toHaveCount(0);
    await expect(conversation(page)).not.toContainText("q-third");
    await chatTurn(page, "q-after");
  });

  test("quick commands insert into the chat message box without sending", async ({
    page,
    request,
    workDir,
  }) => {
    const id = await createHybrid(request, workDir, "chat");
    await openHybrid(page, id, "chat");
    await composer(page).fill("请执行 ");
    // The chat side has no soft-key bar: the entry sits beside the send button.
    await page
      .locator("[data-chat-composer]")
      .getByRole("button", { name: "快捷指令", exact: true })
      .click();
    const menu = page.getByRole("menu");
    await expect(menu).toContainText("插入消息");
    // An Enter command only inserts here: sending stays with the message box.
    await menu
      .getByRole("menuitem", { name: "恢复会话：/resume", exact: true })
      .click();
    await expect(composer(page)).toHaveValue("请执行 /resume");
    await expect(conversation(page)).not.toContainText("/resume");
  });

  test("clicking a code link in chat opens the file at its line range in the code browser", async ({
    page,
    request,
    workDir,
  }, testInfo) => {
    const srcDir = join(workDir, "src");
    await mkdir(srcDir);
    const lines = Array.from(
      { length: 40 },
      (_, i) => `export const line${i + 1} = ${i + 1};`,
    );
    await writeFile(join(srcDir, "target.ts"), `${lines.join("\n")}\n`);
    const id = await createHybrid(request, workDir, "chat");
    await openHybrid(page, id, "chat");

    await sendChat(page, "请看 [target.ts:25-27](src/target.ts#L25-L27)");
    const link = conversation(page).getByRole("link", {
      name: "target.ts:25-27",
      exact: true,
    });
    await expect(link).toBeVisible();
    await expect(working(page)).toHaveCount(0);
    await link.click();

    const code = page.getByRole("region", { name: "代码工作区", exact: true });
    await expect(code).toBeVisible();
    await expect(
      code.getByRole("tab", { name: "src/target.ts · worktree", exact: true }),
    ).toHaveAttribute("aria-selected", "true");
    const sendRange = code.getByRole("button", {
      name: "发送位置到草稿（25-27 行）",
      exact: true,
    });
    await expect(sendRange).toBeEnabled();
    await sendRange.click();

    if (testInfo.project.use.isMobile) {
      await sideTab(page, "chat").click();
      await expectSide(page, "chat");
    }
    await expect(composer(page)).toHaveValue(/src\/target\.ts.*25.*27/);
  });

  test("an idle chat side keeps the same agent process alive until explicitly archived", async ({
    page,
    request,
    workDir,
  }) => {
    const store = join(workDir, "idle-alive-store");
    await mkdir(store);
    const id = await createHybrid(request, workDir, "chat", {
      env: { FAKEAGENT_STORE: store },
    });
    await openHybrid(page, id, "chat");
    await chatTurn(page, "before-idle");

    const eventsPath = join(store, "events.log");
    const openPids = async () =>
      (await readFile(eventsPath, "utf8"))
        .trim()
        .split("\n")
        .map((line) => line.split(" "))
        .filter((parts) => parts[2] === "acp" && parts[3] === "open")
        .map((parts) => Number(parts[1]));
    const pidAlive = (pid: number) => {
      try {
        process.kill(pid, 0);
        return true;
      } catch {
        return false;
      }
    };

    const pids = await openPids();
    expect(pids).toHaveLength(1);
    const pid = pids[0]!;
    expect(pidAlive(pid)).toBe(true);

    await delay(1500);
    expect(pidAlive(pid)).toBe(true);
    const info = await (await request.get(`/api/sessions/${id}`)).json();
    expect(info.status).not.toBe("exited");
    expect(info).toMatchObject({ view: "chat", transition: null });

    await chatTurn(page, "after-idle");
    expect(await openPids()).toEqual([pid]);
    expect(await readFile(eventsPath, "utf8")).not.toContain(
      "call session/close",
    );

    expect((await request.delete(`/api/sessions/${id}`)).status()).toBe(204);
    await expect.poll(() => pidAlive(pid)).toBe(false);
    expect(await readFile(eventsPath, "utf8")).toContain("call session/close");
  });

  test("an unnamed chat session takes its title from the first prompt, then from the agent", async ({
    page,
    request,
    workDir,
  }) => {
    const configId = await hybridConfig(request, "E2E titles", {});
    const created = await request.post("/api/sessions", {
      data: { cwd: workDir, configId, view: "chat" },
    });
    expect(created.status(), await created.text()).toBe(201);
    const id: string = (await created.json()).session.id;
    const title = async () =>
      (await (await request.get(`/api/sessions/${id}`)).json()).title;
    await openHybrid(page, id, "chat");
    await chatTurn(page, "fix the   login redirect");
    await expect.poll(title).toBe("fix the login redirect");
    await chatTurn(page, "name it /title");
    await expect.poll(title).toBe("fake title");

    await page.goto("/");
    await expect(
      page.getByRole("link", { name: "fake title", exact: true }),
    ).toBeVisible();
    expect((await request.delete(`/api/sessions/${id}`)).status()).toBe(204);
    const archived = page.getByRole("region", {
      name: `${basename(workDir)} 的已归档会话`,
    });
    await archived.getByRole("button", { name: /^已归档/ }).click();
    await expect(
      archived.locator('[data-slot="item"]').filter({ hasText: "fake title" }),
    ).toHaveCount(1);
  });

  test("archiving from the session page returns home with the archived section collapsed", async ({
    page,
    request,
    workDir,
  }) => {
    const root = join(workDir, "archive-repo");
    await mkdir(root);
    for (const args of [
      ["init", "-b", "main"],
      [
        "-c",
        "user.name=WebMux QA",
        "-c",
        "user.email=qa@example.invalid",
        "commit",
        "--allow-empty",
        "-m",
        "initial",
      ],
    ])
      expect(spawnSync("git", ["-C", root, ...args]).status).toBe(0);
    const registered = await request.post("/api/repositories", {
      data: { path: root },
    });
    expect(registered.status(), await registered.text()).toBe(201);
    const id = await createHybrid(request, root, "chat", {
      name: "E2E archive",
    });
    await openHybrid(page, id, "chat");
    await chatTurn(page, "keep-this");

    await page.getByRole("button", { name: "会话菜单", exact: true }).click();
    await page
      .getByRole("menuitem", { name: "停止会话并归档", exact: true })
      .click();
    const dialog = page.getByRole("alertdialog");
    await expect(dialog).toContainText("对话保留在 agent 中");
    await dialog
      .getByRole("button", { name: "停止并归档", exact: true })
      .click();

    await expect(page).toHaveURL(/\/$/);
    const archived = page.getByRole("region", { name: "main 的已归档会话" });
    await expect(archived).toBeVisible();
    const row = archived
      .locator('[data-slot="item"]')
      .filter({ hasText: "E2E archive" });
    await expect(row).toBeHidden();
    await archived.getByRole("button", { name: /已归档/ }).click();
    await expect(row).toBeVisible();

    await row.getByRole("button", { name: "恢复", exact: true }).click();
    await expect(row).toHaveCount(0);
    const resumed = await openSessionLink(page, "E2E archive");
    await expectSide(resumed, "chat");
    await expect(conversation(resumed)).toContainText("echo: keep-this");
  });

  for (const view of ["chat", "terminal"] as const) {
    for (const mode of ["chat", "workbench"] as const) {
      test(`archived ${view} resumes its remembered view with the ${mode} sidebar`, async ({
        page,
        request,
        workDir,
      }) => {
        for (const args of [
          ["init", "-b", "main"],
          [
            "-c",
            "user.name=WebMux QA",
            "-c",
            "user.email=qa@example.invalid",
            "commit",
            "--allow-empty",
            "-m",
            "initial",
          ],
        ])
          expect(spawnSync("git", ["-C", workDir, ...args]).status).toBe(0);
        const registered = await request.post("/api/repositories", {
          data: { path: workDir },
        });
        expect(registered.status(), await registered.text()).toBe(201);
        const name = `Remember ${view}`;
        const initial = view === "chat" ? "terminal" : "chat";
        const id = await createHybrid(request, workDir, initial, { name });
        await openHybrid(page, id, initial);
        if (initial === "chat") await chatTurn(page, "remember-me");
        else await tuiTurn(page, "remember-me");
        await switchTo(page, view);
        const archived = await request.delete(`/api/sessions/${id}`);
        expect(archived.status(), await archived.text()).toBe(204);
        await page.goto("/");
        const showSidebar = page
          .getByRole("button", { name: "打开侧栏" })
          .first();
        if (await showSidebar.isVisible()) await showSidebar.click();
        await page
          .getByRole("button", {
            name: mode === "chat" ? "聊天模式" : "工作台模式",
            exact: true,
          })
          .click();
        if (mode === "chat") {
          await page
            .getByRole("button", { name: /已归档/ })
            .first()
            .click();
          await page
            .getByRole("list", { name: "已归档聊天" })
            .getByRole("button", { name: `${name}（已结束）`, exact: true })
            .click();
        } else {
          if (test.info().project.use.isMobile)
            await page.keyboard.press("Escape");
          const archived = page.getByRole("region", {
            name: "main 的已归档会话",
          });
          await archived.getByRole("button", { name: /已归档/ }).click();
          await archived
            .getByRole("button", { name: "恢复", exact: true })
            .click();
          await openSessionLink(page, name);
        }
        await expect(page).toHaveURL(new RegExp(`/s/${id}$`));
        await expectSide(page, view);
        if (view === "chat")
          await expect(conversation(page)).toContainText("remember-me");
        else await expectTerminalLine(page, "> remember-me");
        if (test.info().project.use.isMobile) {
          await page.getByRole("button", { name: "打开侧栏" }).first().click();
        }
        await expect(
          page.getByRole("button", {
            name: mode === "chat" ? "聊天模式" : "工作台模式",
            exact: true,
          }),
        ).toHaveAttribute("aria-pressed", "true");
      });
    }
  }

  test("stopping the open session from the sidebar leaves its page instead of offering to resume", async ({
    page,
    request,
    workDir,
  }) => {
    const id = await createHybrid(request, workDir, "chat", {
      name: "E2E sidebar stop",
    });
    await openHybrid(page, id, "chat");
    await chatTurn(page, "stop-me");

    const open = page.getByRole("button", { name: "打开侧栏" }).first();
    if (await open.isVisible()) await open.click();
    await page
      .getByRole("button", { name: "会话操作：E2E sidebar stop" })
      .first()
      .click();
    await page
      .getByRole("menuitem", { name: "停止会话并归档", exact: true })
      .click();
    await page
      .getByRole("alertdialog")
      .getByRole("button", { name: "停止并归档", exact: true })
      .click();

    await expect(page).not.toHaveURL(new RegExp(`/s/${id}`));
    await expect(page.getByRole("status", { name: "恢复会话" })).toHaveCount(0);
  });

  test("releasing an archived chat's worktree keeps the record, and resuming rebuilds it first", async ({
    page,
    request,
    workDir,
  }) => {
    const root = join(workDir, "release-repo");
    const tree = join(workDir, "release-repo-feature");
    await mkdir(root);
    await writeFile(join(root, ".gitignore"), "node_modules/\n");
    const git = (...args: string[]) =>
      expect(spawnSync("git", ["-C", root, ...args]).status).toBe(0);
    git("init", "-b", "main");
    git("add", ".gitignore");
    git(
      "-c",
      "user.name=WebMux QA",
      "-c",
      "user.email=qa@example.invalid",
      "commit",
      "-m",
      "initial",
    );
    git("worktree", "add", "-b", "feature-x", tree);
    // Ignored files never block a release; they go with the directory.
    await mkdir(join(tree, "node_modules", "left-pad"), { recursive: true });
    await writeFile(
      join(tree, "node_modules", "left-pad", "index.js"),
      "module.exports = 1;\n",
    );
    const registered = await request.post("/api/repositories", {
      data: { path: root },
    });
    expect(registered.status(), await registered.text()).toBe(201);
    const id = await createHybrid(request, tree, "chat", {
      name: "E2E release",
    });
    await openHybrid(page, id, "chat");
    await chatTurn(page, "before-release");
    const archived = await request.delete(`/api/sessions/${id}`);
    expect(archived.status(), await archived.text()).toBe(204);

    await page.goto("/");
    const feature = page.getByRole("region", {
      name: "feature-x 的已归档会话",
    });
    await feature.getByRole("button", { name: /已归档/ }).click();
    await feature
      .getByRole("button", { name: "更多操作 E2E release", exact: true })
      .click();
    await page.getByRole("menuitem", { name: "释放空间", exact: true }).click();
    const release = page.getByRole("dialog", { name: "释放空间" });
    await expect(release).toContainText("重建工作树也无法找回");
    await release
      .getByRole("button", { name: "释放空间", exact: true })
      .click();
    await expect(release).toHaveCount(0);
    await expect(access(tree)).rejects.toThrow();
    git("rev-parse", "--verify", "refs/heads/feature-x");

    // The record stays archived, now under the unavailable workspaces.
    const gone = page.getByRole("region", {
      name: "已不可用工作区 的已归档会话",
    });
    await gone.getByRole("button", { name: /已归档/ }).click();
    const row = gone.locator(`[data-archived-id="${id}"]`);
    await expect(row.getByText("已释放空间")).toBeVisible();

    await row.getByRole("button", { name: "恢复", exact: true }).click();
    const rebuild = page.getByRole("dialog", { name: "重建工作树后恢复" });
    await expect(rebuild).toContainText("将在原路径检出分支 feature-x");
    await rebuild
      .getByRole("button", { name: "重建并恢复", exact: true })
      .click();
    await expect(row).toHaveCount(0);
    await access(join(tree, ".gitignore"));
    const resumed = await openSessionLink(page, "E2E release");
    await expectSide(resumed, "chat");
    await expect(conversation(resumed)).toContainText("echo: before-release");
  });

  for (const view of ["chat", "terminal"] as const) {
    test(`deleting a worktree keeps its archived ${view} released, and the sidebar tree resumes it`, async ({
      page,
      request,
      workDir,
    }, testInfo) => {
      test.skip(!!testInfo.project.use.isMobile, "the desktop sidebar tree");
      const root = join(workDir, "delete-repo");
      const tree = join(workDir, "delete-repo-feature");
      await mkdir(root);
      await writeFile(join(root, ".gitignore"), "node_modules/\n");
      const git = (...args: string[]) =>
        expect(spawnSync("git", ["-C", root, ...args]).status).toBe(0);
      git("init", "-b", "main");
      git("add", ".gitignore");
      git(
        "-c",
        "user.name=WebMux QA",
        "-c",
        "user.email=qa@example.invalid",
        "commit",
        "-m",
        "initial",
      );
      git("worktree", "add", "-b", "feature-y", tree);
      const registered = await request.post("/api/repositories", {
        data: { path: root },
      });
      expect(registered.status(), await registered.text()).toBe(201);
      const id = await createHybrid(request, tree, "chat", {
        name: "E2E delete",
      });
      await openHybrid(page, id, "chat");
      await chatTurn(page, "before-delete");
      await switchTo(page, view);
      const archived = await request.delete(`/api/sessions/${id}`);
      expect(archived.status(), await archived.text()).toBe(204);

      await page.goto("/");
      await page
        .getByRole("button", { name: "工作树菜单 feature-y", exact: true })
        .click();
      await page
        .getByRole("menuitem", { name: "删除 worktree…", exact: true })
        .click();
      const removal = page.getByRole("dialog", { name: "删除 worktree" });
      await expect(removal).toContainText(
        "保留的已归档会话（之后可重建并恢复）",
      );
      await expect(removal).toContainText("E2E delete");
      await removal.getByRole("button", { name: "确认安全删除" }).click();
      await expect(removal).toHaveCount(0);
      await expect(access(tree)).rejects.toThrow();
      git("rev-parse", "--verify", "refs/heads/feature-y");

      // The record stays, released, under the repository's unavailable
      // workspaces in the sidebar tree; neither the repository nor the
      // unavailable workspace auto-expands when the only session is archived.
      const sidebarTree = page.getByRole("tree", { name: "仓库与会话" });
      await sidebarTree
        .getByRole("button", { name: "展开delete-repo", exact: true })
        .click();
      await sidebarTree
        .getByRole("button", { name: "展开已不可用工作区", exact: true })
        .click();
      const row = sidebarTree.getByRole("treeitem", {
        name: "E2E delete（已释放空间，已结束）",
      });
      await row.click();
      const rebuild = page.getByRole("dialog", { name: "重建工作树后恢复" });
      await expect(rebuild).toContainText("将在原路径检出分支 feature-y");
      await rebuild
        .getByRole("button", { name: "重建并恢复", exact: true })
        .click();
      await expectSide(page, view);
      if (view === "chat")
        await expect(conversation(page)).toContainText("echo: before-delete");
      else await expectTerminalLine(page, "> before-delete");
      await access(join(tree, ".gitignore"));
    });
  }

  test("after a backend restart both sides resume the recorded conversation", async ({
    page,
    request,
    server,
    workDir,
  }) => {
    test.slow();
    const chat = await createHybrid(request, workDir, "chat", {
      name: "E2E resume chat",
      agent: "claude",
    });
    await openHybrid(page, chat, "chat");
    await chatTurn(page, "kept-in-chat");
    const terminal = await createHybrid(request, workDir, "terminal", {
      name: "E2E resume terminal",
      agent: "claude",
    });
    await openHybrid(page, terminal, "terminal");
    await tuiTurn(page, "kept-in-tui");
    // The terminal's agent session is known (read from its signals about
    // twice a second) once chat becomes available; a resume needs it.
    await expect(sideTab(page, "chat")).toHaveAccessibleName("聊天");

    await server.restart();
    await page.goto("/");
    // A restart does not archive what the user left running: both sessions
    // come back on their recorded side by themselves.
    const resumedChat = await openSessionLink(page, "E2E resume chat");
    await expectSide(resumedChat, "chat");
    await expect(conversation(resumedChat)).toContainText("kept-in-chat");
    await expect(conversation(resumedChat)).toContainText("echo: kept-in-chat");

    await page.goto("/");
    const resumedTerminal = await openSessionLink(page, "E2E resume terminal");
    await expectSide(resumedTerminal, "terminal");
    await expectTerminalLine(resumedTerminal, "> kept-in-tui");
    await expectTerminalLine(resumedTerminal, "< echo: kept-in-tui");
    await tuiTurn(resumedTerminal, "after-restart");
  });

  test("Claude falls back to candidates when its process record is unavailable", async ({
    page,
    request,
    server,
    workDir,
  }) => {
    test.slow();
    const id = await createHybrid(request, workDir, "terminal", {
      agent: "claude",
      name: "E2E Claude candidates",
    });
    await openHybrid(page, id, "terminal");
    await tuiTurn(page, "candidate-one");
    await expect(sideTab(page, "chat")).toHaveAccessibleName("聊天");
    const info = await (await request.get(`/api/sessions/${id}`)).json();
    const sid = info.agent.sessionId;

    const other = await createHybrid(request, workDir, "chat", {
      agent: "claude",
      name: "E2E Claude other candidate",
    });
    await openHybrid(page, other, "chat");
    await chatTurn(page, "candidate-two");
    const stored = JSON.parse(
      await readFile(join(server.stateDir, "hybrid-sessions.json"), "utf8"),
    );
    const record = stored.records.find(
      (item: { id: string }) => item.id === id,
    );
    await rm(
      join(server.env.HOME, ".claude", "sessions", `${record.pid}.json`),
    );
    await page.goto(`/s/${id}`);
    await delay(2_600);
    await expect(sideTab(page, "chat")).toHaveAccessibleName("聊天", {
      timeout: 20_000,
    });
    await sideTab(page, "chat").click();
    const picker = page.getByRole("combobox", { name: "选择要打开的会话" });
    await expect(picker).toBeVisible();
    await picker.selectOption(sid);
    await page.getByRole("button", { name: "确认", exact: true }).click();
    await expectSide(page, "chat");
    await expect(conversation(page)).toContainText("candidate-one");
  });

  test("Claude refuses an externally occupied session without forking it", async ({
    page,
    request,
    server,
    workDir,
  }) => {
    const id = await createHybrid(request, workDir, "terminal", {
      agent: "claude",
      name: "E2E Claude external",
    });
    await openHybrid(page, id, "terminal");
    await tuiTurn(page, "external-one");
    await expect(sideTab(page, "chat")).toHaveAccessibleName("聊天");
    const info = await (await request.get(`/api/sessions/${id}`)).json();
    const sid = info.agent.sessionId;
    expect((await request.delete(`/api/sessions/${id}`)).status()).toBe(204);
    // DELETE initiates process shutdown; the hybrid actor publishes the
    // archived record asynchronously. Resume must exercise the external
    // owner check, not race the still-active record.
    await expect
      .poll(async () => {
        const response = await request.get(`/api/sessions/${id}`);
        expect(response.status()).toBe(200);
        return (await response.json()).state;
      })
      .toBe("closed");

    const external = spawn(
      join(process.env.WEBMUX_E2E_AGENTS!, "claude"),
      ["--resume", sid],
      { cwd: workDir, env: server.env, stdio: ["pipe", "pipe", "pipe"] },
    );
    let output = "";
    external.stdout.on("data", (chunk) => (output += String(chunk)));
    external.stderr.on("data", (chunk) => (output += String(chunk)));
    try {
      await expect.poll(() => output).toContain("> external-one");
      const resumed = await request.post(`/api/sessions/${id}/resume`, {
        data: { clientRequestId: "e2e-claude-external", view: "chat" },
      });
      expect(resumed.status()).toBe(409);
      expect((await resumed.json()).reason).toBe("occupied_external");
    } finally {
      if (external.exitCode === null && external.signalCode === null) {
        const exited = once(external, "exit");
        external.kill("SIGTERM");
        await exited;
      }
    }
  });

  test("a Codex conversation round-trips through the codex-acp connector", async ({
    page,
    request,
    workDir,
  }) => {
    test.slow();
    const id = await createHybrid(request, workDir, "terminal", {
      name: "E2E Codex",
      agent: "codex",
    });
    await openHybrid(page, id, "terminal");
    await tuiTurn(page, "codex-one");
    await switchTo(page, "chat");
    await expect(conversation(page)).toContainText("codex-one");
    await chatTurn(page, "codex-two");
    await switchTo(page, "terminal");
    await expectTerminalLine(page, "> codex-two");
  });

  for (const row of [
    {
      agent: "codex",
      connector: "codex-acp",
      title: "Codex ACP 适配器",
      package: "@agentclientprotocol/codex-acp",
    },
    {
      agent: "claude",
      connector: "claude-agent-acp",
      title: "Claude Code ACP 适配器",
      package: "@agentclientprotocol/claude-agent-acp",
    },
  ] as const) {
    test(`a ${row.agent} session without ${row.connector} starts on the terminal and shows how to install it`, async ({
      page,
      request,
      workDir,
    }) => {
      const name = `E2E ${row.agent} without connector`;
      // The fake agent is named by its path; the config's PATH has no connector.
      const response = await request.post("/api/launch-configs", {
        data: {
          name,
          cmd: [join(process.env.WEBMUX_E2E_AGENTS!, row.agent)],
          env: { PATH: "/usr/bin:/bin" },
          chat: true,
        },
      });
      expect(response.status(), await response.text()).toBe(201);
      await page.goto("/");
      await page
        .getByRole("main")
        .getByRole("button", { name: "＋ 新建会话", exact: true })
        .first()
        .click();
      await chooseLaunch(page, name);
      await expect(
        page.getByText(`聊天端需要另外安装 ${row.connector}`),
      ).toBeVisible();
      await page
        .getByRole("radiogroup", { name: "起始端", exact: true })
        .getByRole("radio", { name: "从聊天开始", exact: true })
        .click();
      await useDirectory(page, workDir);
      await page
        .getByRole("button", { name: "创建并进入 →", exact: true })
        .click();
      await expect(
        page.getByText(`未安装 ${row.title}（${row.connector}）`),
      ).toBeVisible();
      await expect(
        page.getByText(`npm install -g ${row.package}`),
      ).toBeVisible();
      await page
        .getByRole("button", { name: "改为从终端开始", exact: true })
        .click();
      await expect(page).toHaveURL(/\/s\/[a-z0-9]+$/);
      await expectSide(page, "terminal");
      await expectTerminalLine(page, "prompt>");
      await expect(sideTab(page, "chat")).toBeDisabled({ timeout: 20_000 });
      await expect(page.getByText("安装后会自动重新检测")).toBeVisible();
    });
  }

  test("webmux attach detaches when the session moves to chat and refuses it there", async ({
    page,
    request,
    server,
    workDir,
  }) => {
    const python = spawnSync("python3", ["-c", "import pty"]);
    test.skip(
      python.status !== 0,
      "webmux attach needs a terminal; this runner has no python3 pty module",
    );
    const id = await createHybrid(request, workDir, "terminal");
    await openHybrid(page, id, "terminal");
    await tuiTurn(page, "before-attach");

    const attached = attach(server, id);
    await expect
      .poll(() => attached.output())
      .toContain("< echo: before-attach");
    await switchTo(page, "chat");
    expect(await attached.exit).toBe(0);
    expect(attached.output()).toContain("已切换到聊天视图");
    expect(attached.output()).toContain(`webmux attach ${id}`);

    const refused = attach(server, id);
    expect(await refused.exit).toBe(1);
    expect(refused.output()).toContain(
      "会话在聊天视图，请用 webmux view SESSION terminal 切回终端后重新附着，或在 Web 端操作",
    );
  });
});

// Runs `webmux attach` on an 80x24 pseudo-terminal from Python's pty module,
// with the backend's environment, and collects what it writes. A stuck
// client is killed after 30 seconds, which fails its exit code check.
function attach(server: E2EServer, id: string) {
  const script = `
import fcntl, os, pty, struct, sys, termios
pid, fd = pty.fork()
if pid == 0:
    fcntl.ioctl(0, termios.TIOCSWINSZ, struct.pack("HHHH", 24, 80, 0, 0))
    os.execv(sys.argv[1], sys.argv[1:])
while True:
    try:
        data = os.read(fd, 65536)
    except OSError:
        break
    if not data:
        break
    os.write(1, data)
sys.exit(os.waitstatus_to_exitcode(os.waitpid(pid, 0)[1]))
`;
  const child = spawn(
    "python3",
    [
      "-c",
      script,
      process.env.WEBMUX_E2E_BINARY!,
      "attach",
      "--config",
      server.config,
      id,
    ],
    { env: server.env, stdio: ["ignore", "pipe", "pipe"], timeout: 30_000 },
  );
  let output = "";
  child.stdout.on("data", (chunk) => (output += chunk));
  child.stderr.on("data", (chunk) => (output += chunk));
  const exit = Promise.withResolvers<number | null>();
  child.on("error", exit.reject);
  // "close" follows the end of both pipes, so the output is complete.
  child.on("close", (code) => exit.resolve(code));
  return { exit: exit.promise, output: () => output };
}
