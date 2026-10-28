import { spawn, spawnSync } from "node:child_process";
import type { APIRequestContext, Page } from "@playwright/test";
import {
  expect,
  expectTerminalLine,
  openSessionLink,
  runCommand,
  test,
} from "./fixtures";
import type { E2EServer } from "./server.mjs";

// Hybrid sessions run the fake agent (internal/acp/testdata/fakeagent), which
// global-setup puts first on every backend's PATH as omp and opencode. Its
// TUI does not echo typing: a line shows up as "> text" when a later run
// replays the stored conversation, and replies stream as "< echo: text".
// Its ACP side answers "echo: text" after a thought and a "Read file" tool
// call. FAKEAGENT_TURN_DELAY paces every streamed piece (TUI: every word).

type Side = "tui" | "acp";
type Options = {
  name?: string;
  agent?: "omp" | "opencode";
  env?: Record<string, string>;
};

async function hybridConfig(
  request: APIRequestContext,
  name: string,
  { agent = "omp", env = {} }: Options,
): Promise<string> {
  const response = await request.post("/api/launch-configs", {
    data: { name, cmd: [agent], env, hybrid: true },
  });
  expect(response.status(), await response.text()).toBe(201);
  return (await response.json()).id;
}

async function createHybrid(
  request: APIRequestContext,
  cwd: string,
  startSide: Side,
  options: Options = {},
): Promise<string> {
  const name = options.name ?? `E2E ${startSide}`;
  const configId = await hybridConfig(request, name, options);
  const response = await request.post("/api/sessions", {
    data: { cwd, configId, startSide, name },
  });
  expect(response.status(), await response.text()).toBe(201);
  return (await response.json()).id;
}

const sideTab = (page: Page, side: Side) =>
  page
    .getByRole("tablist", { name: "会话视图", exact: true })
    .getByRole(
      "tab",
      side === "acp" ? { name: /^聊天/ } : { name: "终端", exact: true },
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
  if (side === "acp") await expect(composer(page)).toBeEditable();
  else {
    await expect(page.locator("[data-chat-view]")).toHaveCount(0);
    await expect(page.locator(".xterm")).toBeVisible();
  }
}

async function openHybrid(page: Page, id: string, side: Side) {
  await page.goto(`/s/${id}`);
  await expectSide(page, side);
  if (side === "tui") await expectTerminalLine(page, "prompt>");
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
  if (side === "acp")
    await expect(sideTab(page, "acp")).toHaveAccessibleName("聊天", {
      timeout: 20_000,
    });
  await sideTab(page, side).click();
  if (confirm) {
    const dialog = page.getByRole("alertdialog");
    await expect(dialog).toContainText(
      side === "acp" ? "切换到聊天？" : "切换到终端？",
    );
    await dialog.getByRole("button", { name: "确认", exact: true }).click();
  }
  await expectSide(page, side);
}

test.describe("hybrid sessions with the fake agent", () => {
  for (const { start, agent } of [
    { start: "acp", agent: "omp" },
    { start: "tui", agent: "opencode" },
  ] as const) {
    test(`an ${agent} conversation started on ${start === "acp" ? "chat" : "the terminal"} round-trips with its history on both sides`, async ({
      page,
      request,
      workDir,
    }) => {
      test.slow();
      const other: Side = start === "acp" ? "tui" : "acp";
      const name = `E2E ${agent}`;
      await hybridConfig(request, name, { agent });
      await page.goto("/");
      await page
        .locator("header")
        .getByRole("button", { name: "＋ 新建会话", exact: true })
        .click();
      await page.getByRole("tab", { name: "预先输入", exact: true }).click();
      await page
        .getByRole("combobox", { name: "选择启动配置", exact: true })
        .click();
      await page.getByRole("option", { name, exact: true }).click();
      await page
        .getByRole("tablist", { name: "起始端", exact: true })
        .getByRole("tab", {
          name: start === "acp" ? "从聊天开始" : "从终端开始",
          exact: true,
        })
        .click();
      await page.getByLabel("工作目录", { exact: true }).fill(workDir);
      await page
        .getByLabel("会话名称", { exact: false })
        .fill("E2E round trip");
      await page
        .getByRole("button", { name: "创建并进入 →", exact: true })
        .click();
      await expect(page).toHaveURL(/\/s\/[a-z0-9]+$/);
      await expectSide(page, start);
      if (start === "tui") await expectTerminalLine(page, "prompt>");

      const turn = (side: Side, text: string) =>
        side === "acp" ? chatTurn(page, text) : tuiTurn(page, text);
      const expectHistory = async (side: Side, texts: string[]) => {
        for (const text of texts) {
          if (side === "tui") {
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

  test("switching mid-turn interrupts the running side and keeps what it said", async ({
    page,
    request,
    workDir,
  }) => {
    test.slow();
    // Two seconds per streamed piece leave each turn running for several
    // seconds: four pieces in chat, one per word in the TUI.
    const id = await createHybrid(request, workDir, "acp", {
      env: { FAKEAGENT_TURN_DELAY: "2s" },
    });
    await openHybrid(page, id, "acp");

    await sendChat(page, "chat-mid");
    await expect(working(page)).toBeVisible();
    await switchTo(page, "tui", true);
    // The cancelled turn kept the prompt and produced no reply.
    await expectTerminalLine(page, "> chat-mid");
    await expect(statusDot(page)).not.toHaveClass(/status-running/);
    await expect(sideTab(page, "acp")).toHaveAccessibleName("聊天", {
      timeout: 20_000,
    });

    await runCommand(page, "tui-mid a b c");
    await expect(statusDot(page)).toHaveClass(/status-running/);
    await switchTo(page, "acp", true);
    // The interrupt key stopped the TUI, which saved what it had.
    await expect(conversation(page)).toContainText("tui-mid a b c");
    await expect(conversation(page)).toContainText("echo: (interrupted)");
    await chatTurn(page, "after-mid");
  });

  test("a permission request is answered once for every device", async ({
    page,
    request,
    browser,
    server,
    workDir,
  }, testInfo) => {
    const id = await createHybrid(request, workDir, "acp", {
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
    await openHybrid(page, id, "acp");
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
      await openHybrid(phone, id, "acp");
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

  test("queued prompts pause after a stop until continued or cleared", async ({
    page,
    request,
    workDir,
  }) => {
    test.slow();
    // Each turn streams four pieces two seconds apart: time to queue behind it.
    const id = await createHybrid(request, workDir, "acp", {
      env: { FAKEAGENT_TURN_DELAY: "2s" },
    });
    await openHybrid(page, id, "acp");
    const queue = page.getByRole("region", { name: "排队的消息", exact: true });
    const paused = queue.getByRole("status");

    await sendChat(page, "q-first");
    await expect(working(page)).toBeVisible();
    await expect(
      page.getByRole("button", { name: "加入排队", exact: true }),
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

  test(
    "an idle chat side sleeps and wakes for the next message",
    { tag: "@slow" },
    async ({ page, request, workDir }, testInfo) => {
      // The shortest setting is one minute; one project is enough to wait.
      test.skip(
        testInfo.project.name !== "chromium-desktop",
        "Waits a real minute of idleness; the desktop project covers it",
      );
      test.setTimeout(180_000);
      const prefs = await (await request.get("/api/settings")).json();
      expect(
        (
          await request.patch("/api/settings", {
            data: {
              baseRevision: prefs.revision,
              changes: { chatIdleSleepMinutes: 1 },
            },
          })
        ).ok(),
      ).toBeTruthy();
      const id = await createHybrid(request, workDir, "acp");
      await openHybrid(page, id, "acp");
      await chatTurn(page, "before-sleep");

      await expect(
        page.getByRole("status").filter({ hasText: "Agent 已休眠" }),
      ).toBeVisible({ timeout: 100_000 });
      const info = await (await request.get(`/api/sessions/${id}`)).json();
      expect(info.hybrid).toMatchObject({ side: "acp", dormant: true });

      await chatTurn(page, "after-sleep");
      await expect(conversation(page)).toContainText("Agent 已唤醒");
      await expect(conversation(page)).toContainText("echo: before-sleep");
      const awake = await (await request.get(`/api/sessions/${id}`)).json();
      expect(awake.hybrid.dormant).toBe(false);
    },
  );

  test("after a backend restart both sides resume the recorded conversation", async ({
    page,
    request,
    server,
    workDir,
  }) => {
    test.slow();
    const chat = await createHybrid(request, workDir, "acp", {
      name: "E2E resume chat",
    });
    await openHybrid(page, chat, "acp");
    await chatTurn(page, "kept-in-chat");
    const terminal = await createHybrid(request, workDir, "tui", {
      name: "E2E resume terminal",
    });
    await openHybrid(page, terminal, "tui");
    await tuiTurn(page, "kept-in-tui");
    // The terminal's agent session is known (read from its signals about
    // twice a second) once chat becomes available; a resume needs it.
    await expect(sideTab(page, "acp")).toHaveAccessibleName("聊天");

    await server.restart();
    await page.goto("/");
    const recoverable = page.getByRole("region", { name: "可恢复" });
    await recoverable.getByRole("button", { name: /^可恢复/ }).click();
    const records = recoverable.locator('[data-slot="item"]');
    await expect(records).toHaveCount(2);
    // From the home page, the chat session comes back on the chat side.
    await records
      .filter({ hasText: "E2E resume chat" })
      .getByRole("button", { name: "恢复到聊天", exact: true })
      .click();
    await expect(records).toHaveCount(1);
    const resumed = await openSessionLink(page, "E2E resume chat");
    await expectSide(resumed, "acp");
    await expect(conversation(resumed)).toContainText("kept-in-chat");
    await expect(conversation(resumed)).toContainText("echo: kept-in-chat");
    await resumed.close();

    // On its own page, the terminal session comes back in the terminal.
    await page.goto(`/s/${terminal}`);
    const recovery = page.getByRole("status", { name: "恢复会话" });
    await expect(recovery).toContainText("该混合会话已中断");
    await recovery
      .getByRole("button", { name: "恢复到终端", exact: true })
      .click();
    await expectSide(page, "tui");
    await expectTerminalLine(page, "> kept-in-tui");
    await expectTerminalLine(page, "< echo: kept-in-tui");
    await tuiTurn(page, "after-restart");
  });

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
    const id = await createHybrid(request, workDir, "tui");
    await openHybrid(page, id, "tui");
    await tuiTurn(page, "before-attach");

    const attached = attach(server, id);
    await expect
      .poll(() => attached.output())
      .toContain("< echo: before-attach");
    await switchTo(page, "acp");
    expect(await attached.exit).toBe(0);
    expect(attached.output()).toContain("已切换到聊天视图");
    expect(attached.output()).toContain(`webmux attach ${id}`);

    const refused = attach(server, id);
    expect(await refused.exit).toBe(1);
    expect(refused.output()).toContain(
      "会话在聊天视图，请在 Web 端操作，或切回终端后重新附着",
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
