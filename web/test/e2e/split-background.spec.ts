import { writeFile } from "node:fs/promises";
import { join } from "node:path";
import type { Locator, Page } from "@playwright/test";
import type { ChatFrame } from "../../src/schemas";
import { test, expect, createShell } from "./fixtures";

async function command(page: Page, pane: Locator, value: string) {
  await pane.locator(".xterm-helper-textarea").focus();
  await page.keyboard.type(value);
  await page.keyboard.press("Enter");
}

for (const clipboard of ["allowed", "denied", "unavailable"] as const) {
  test(`background OSC 52 waits for explicit copy with clipboard ${clipboard}`, async ({
    page,
    workDir,
  }, testInfo) => {
    await page.addInitScript((mode) => {
      const writes: string[] = [];
      Object.assign(window, { clipboardWrites: writes });
      Object.defineProperty(navigator, "clipboard", {
        configurable: true,
        value:
          mode === "unavailable"
            ? undefined
            : {
                writeText: (text: string) => {
                  writes.push(text);
                  return mode === "denied"
                    ? Promise.reject(
                        new DOMException("Denied", "NotAllowedError"),
                      )
                    : Promise.resolve();
                },
              },
      });
    }, clipboard);
    const id = await createShell(page, "Foreground shell", workDir);
    const foreground = page.locator(`[data-session-id="${id}"]`);
    await foreground.getByRole("button", { name: "分屏", exact: true }).click();
    await page.getByRole("menuitem", { name: "左右分屏 · 新建 Shell" }).click();
    const background = page.locator(".split-terminal").nth(1);
    await expect(
      background.locator(".terminal-header .status-dot"),
    ).toHaveAttribute("aria-label", /已连接/);
    const gate = join(workDir, "copy-ready");
    const text = `pending-${clipboard}`;
    const payload = Buffer.from(text).toString("base64");
    await command(
      page,
      background,
      `while [ ! -f '${gate}' ]; do sleep 0.05; done; printf '\\033]52;c;${payload}\\a'; printf 'copy-emitted\\n'`,
    );
    await page
      .getByRole("button", { name: "切换到窗格 1：Foreground shell" })
      .click();
    await foreground.locator(".xterm-helper-textarea").focus();
    await writeFile(gate, "ready");
    const pending = background.getByRole("status", {
      name: "待处理的复制请求",
      includeHidden: true,
    });
    await expect(pending).toBeAttached();
    await expect(
      page.getByRole("dialog", { name: "复制终端文字" }),
    ).toBeHidden();
    await expect(foreground).toHaveAttribute("data-active", "true");
    await expect(foreground.locator(".xterm-helper-textarea")).toBeFocused();
    expect(
      await page.evaluate(() => Reflect.get(window, "clipboardWrites")),
    ).toEqual([]);
    await command(page, foreground, "printf 'foreground-still-typing\\n'");
    await expect(foreground.locator(".xterm-rows")).toContainText(
      "foreground-still-typing",
    );
    await page.getByRole("button", { name: /切换到窗格 2：/ }).click();
    await expect(pending).toBeVisible();
    // Selecting the pane alone must not replay its clipboard request.
    expect(
      await page.evaluate(() => Reflect.get(window, "clipboardWrites")),
    ).toEqual([]);
    if (clipboard === "allowed") {
      for (const theme of ["light", "dark"] as const) {
        await page.emulateMedia({ colorScheme: theme });
        await expect(page.locator("html")).toHaveClass(
          theme === "dark" ? /dark/ : /^(?!.*dark)/,
        );
        await testInfo.attach(`pending-copy-${theme}`, {
          body: await page.screenshot({
            path: `/tmp/webmux-split-pr-evidence/pending-copy-${testInfo.project.use.isMobile ? "mobile" : "desktop"}-${theme}.jpg`,
            type: "jpeg",
            quality: 75,
            scale: "css",
            animations: "disabled",
          }),
          contentType: "image/jpeg",
        });
      }
    }
    await background.getByRole("button", { name: "复制待处理文字" }).click();
    await expect(pending).toBeHidden();
    if (clipboard === "allowed") {
      expect(
        await page.evaluate(() => Reflect.get(window, "clipboardWrites")),
      ).toEqual([text]);
      await expect(
        page.getByRole("dialog", { name: "复制终端文字" }),
      ).toBeHidden();
    } else {
      await expect(
        page.getByRole("dialog", { name: "复制终端文字" }),
      ).toBeVisible();
      await expect(page.getByRole("textbox", { name: "复制内容" })).toHaveValue(
        text,
      );
      await page.getByRole("button", { name: "关闭", exact: true }).click();
    }
    await expect(background).toHaveAttribute("data-active", "true");
  });
}

test("background hybrid choice waits for pane activation", async ({
  page,
  request,
  workDir,
}) => {
  const id = await createShell(page, "Typing shell", workDir);
  const foreground = page.locator(`[data-session-id="${id}"]`);
  const config = await request.post("/api/launch-configs", {
    data: { name: "Pending agent", cmd: ["omp"], env: {}, hybrid: true },
  });
  expect(config.status()).toBe(201);
  const created = await request.post("/api/sessions", {
    data: {
      cwd: workDir,
      configId: (await config.json()).id,
      startSide: "tui",
      name: "Pending agent",
    },
  });
  expect(created.status()).toBe(201);
  const agentId = (await created.json()).id;
  let ask: (() => void) | undefined;
  await page.routeWebSocket(`**/ws/s/${agentId}/chat`, (socket) => {
    let awaiting = false;
    const server = socket.connectToServer();
    server.onMessage((message) => {
      if (awaiting) return;
      socket.send(message);
      const frame = JSON.parse(message.toString()) as ChatFrame;
      if (frame.type !== "events") {
        // The same frame arrives when a different device starts the switch.
        ask = () => {
          awaiting = true;
          socket.send(
            JSON.stringify({
              type: "state",
              state: {
                ...frame.state,
                side: "switching",
                phase: "awaiting_choice",
              },
            }),
          );
        };
      }
    });
  });
  await foreground.getByRole("button", { name: "分屏", exact: true }).click();
  await page.getByRole("menuitem", { name: "加入已有会话…" }).click();
  await page.getByRole("button", { name: /Pending agent/ }).click();
  const background = page.locator(`[data-session-id="${agentId}"]`);
  await expect(background.locator(".xterm-rows")).toContainText("prompt>");
  await expect.poll(() => !!ask).toBe(true);
  await page
    .getByRole("button", { name: "切换到窗格 1：Typing shell" })
    .click();
  await foreground.locator(".xterm-helper-textarea").focus();
  ask!();
  await expect(
    background.getByRole("button", {
      name: "选择…",
      includeHidden: true,
    }),
  ).toBeAttached();
  const dialog = page.getByRole("alertdialog", { name: "TUI 仍在执行回合" });
  await expect(dialog).toBeHidden();
  await expect(foreground).toHaveAttribute("data-active", "true");
  await expect(foreground.locator(".xterm-helper-textarea")).toBeFocused();
  await page
    .getByRole("button", { name: /切换到窗格 2：Pending agent/ })
    .click();
  await expect(dialog).toBeVisible();
  await expect(background).toHaveAttribute("data-active", "true");
});
