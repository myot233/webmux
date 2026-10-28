import "../../src/styles.css";
import { expect, test } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte/pure";
import Terminal from "../../src/pages/Terminal.svelte";
import type {
  ChatState,
  HybridSwitchResult,
  SessionInfo,
} from "../../src/schemas";
import { createTerminalDrafts } from "../../src/terminal-draft.svelte";
import {
  chatState,
  commands,
  installChatFixture,
  respond,
  snapshot,
  socketAt,
  sockets,
} from "./chat-fixture";

installChatFixture();

const id = "hybrid22";
const terminalPath = `/ws/s/${id}`;
const chatPath = `/ws/s/${id}/chat`;
const switchPath = `/api/sessions/${id}/switch`;

const info: SessionInfo = {
  id,
  generation: "0123456789abcdef",
  title: "omp · 修复首页排序",
  cwd: "/work",
  cmd: ["omp"],
  status: "idle",
  exitCode: null,
  preview: "",
  createdAt: "2026-10-28T00:00:00Z",
  lastActiveAt: "2026-10-28T00:00:00Z",
  cols: 80,
  rows: 24,
  clients: 1,
  workspace: null,
  attention: null,
  reportedCwd: null,
  hybrid: {
    side: "tui",
    dormant: false,
    state: "active",
    agent: "omp",
    agentSessionId: "01a17e26c4b84f0e9d2a",
  },
};

const tuiState = (patch: Partial<ChatState> = {}) =>
  chatState({ side: "tui", ...patch });

/** The page on a hybrid session's terminal side, both streams attached. */
async function onTerminalSide(state: ChatState) {
  await page.viewport(1280, 900);
  respond(/^GET \/api\/sessions\/hybrid22$/, () =>
    Response.json({ ...info, generation: "" }),
  );
  const drafts = createTerminalDrafts();
  render(Terminal, {
    id,
    drafts,
    onUploadNotice: () => {},
    reopen: () => {},
  });
  const terminal = await socketAt(terminalPath);
  terminal.binary(2, info);
  const chat = await socketAt(chatPath);
  chat.frame(snapshot(state));
  return { terminal, chat, draft: drafts.get(id) };
}

/** A switch request the test finishes when it chooses. */
function pendingSwitch() {
  const done = Promise.withResolvers<Response>();
  respond(/^POST \/api\/sessions\/hybrid22\/switch$/, (request) => {
    const body = request.body;
    if (body && typeof body === "object" && "choice" in body)
      return Response.json({ side: "switching", dropped: [], unsupported: [] });
    return done.promise;
  });
  return (result: HybridSwitchResult) => done.resolve(Response.json(result));
}

const chatTab = (name: string | RegExp = /^聊天/) =>
  page.getByRole("tab", { name });
const progress = () => page.getByRole("status").filter({ hasText: "正在切换" });
const steps = () =>
  progress()
    .getByRole("listitem")
    .elements()
    .map((step) => [
      step.textContent?.trim(),
      step.querySelector("[aria-label]")?.getAttribute("aria-label"),
    ]);

test("the chat tab explains why the session cannot switch yet", async () => {
  const { chat } = await onTerminalSide(
    tuiState({ eligibility: { available: false, reason: "no_session_id" } }),
  );
  const tab = chatTab("聊天（请先在 TUI 中发送一条消息）");
  await expect.element(tab).toHaveAttribute("aria-disabled", "true");
  await expect
    .element(tab)
    .toHaveAttribute("title", "现在不能切到聊天：请先在 TUI 中发送一条消息");
  await expect
    .element(page.getByRole("tab", { name: "终端" }))
    .toHaveAttribute("aria-selected", "true");
  // aria-disabled keeps the tab focusable; choosing it explains instead.
  await tab.click({ force: true });
  await expect
    .element(page.getByText("现在不能切到聊天：请先在 TUI 中发送一条消息"))
    .toBeVisible();
  expect(commands()).toEqual([]);

  chat.frame({
    type: "state",
    state: tuiState({ eligibility: { available: false, reason: "occupied" } }),
  });
  await expect
    .element(chatTab("聊天（该会话已在另一个会话中打开）"))
    .toHaveAttribute("aria-disabled", "true");
  chat.frame({ type: "state", state: tuiState() });
  await expect.element(chatTab("聊天")).not.toHaveAttribute("aria-disabled");
});

test("switching to chat shows its steps, asks after a stuck interrupt and lands on the chat view", async () => {
  const finish = pendingSwitch();
  const { terminal, chat } = await onTerminalSide(tuiState());
  await chatTab("聊天").click();
  await expect.poll(commands).toEqual([
    {
      method: "POST",
      path: switchPath,
      body: { clientRequestId: expect.any(String), to: "acp" },
    },
  ]);

  chat.frame({ type: "state", state: tuiState({ phase: "interrupting" }) });
  await expect
    .element(page.getByRole("heading", { name: "正在切换到聊天…" }))
    .toBeVisible();
  await expect.poll(steps).toEqual([
    ["中断中", "进行中"],
    ["退出中", "等待"],
    ["启动中", "等待"],
    ["加载历史中", "等待"],
  ]);
  // Neither side takes input while the session moves.
  await expect.element(chatTab("聊天")).toBeDisabled();
  await expect.element(page.getByRole("tab", { name: "终端" })).toBeDisabled();

  // D21: the interrupt did not stop the TUI; every device is asked.
  chat.frame({ type: "state", state: tuiState({ phase: "awaiting_choice" }) });
  const dialog = page.getByRole("alertdialog", { name: "TUI 仍在执行回合" });
  await expect.element(dialog).toBeVisible();
  await expect.element(dialog).toMatchTextContent(/omp 已输出的部分可能丢失/);
  await dialog.getByRole("button", { name: "强制切换" }).click();
  await expect
    .poll(() => commands().at(-1))
    .toEqual({
      method: "POST",
      path: switchPath,
      body: { clientRequestId: expect.any(String), choice: "force" },
    });

  chat.frame({ type: "state", state: tuiState({ phase: "stopping" }) });
  await expect.element(dialog).not.toBeInTheDocument();
  await expect.poll(steps).toEqual([
    ["中断中", "完成"],
    ["退出中", "进行中"],
    ["启动中", "等待"],
    ["加载历史中", "等待"],
  ]);
  // The TUI exits: its stream closes with 4409 and does not reconnect.
  terminal.drop(4409);
  chat.frame({
    type: "state",
    state: chatState({ side: "switching", phase: "loading" }),
  });
  await expect.poll(steps).toEqual([
    ["中断中", "完成"],
    ["退出中", "完成"],
    ["启动中", "完成"],
    ["加载历史中", "进行中"],
  ]);
  chat.frame({ type: "state", state: chatState() });
  finish({ side: "acp", dropped: [], unsupported: [] });

  await expect
    .element(page.getByRole("textbox", { name: "消息" }))
    .toBeVisible();
  await expect
    .element(chatTab("聊天"))
    .toHaveAttribute("aria-selected", "true");
  await expect.element(progress()).not.toBeInTheDocument();
  expect(sockets.filter((socket) => socket.path === terminalPath)).toHaveLength(
    1,
  );
});

test("a session moved by another device shows the chat view; switching back reattaches the terminal", async () => {
  const finish = pendingSwitch();
  const { terminal, chat } = await onTerminalSide(tuiState());
  // Another device moved the session: the terminal stream ends with 4409.
  chat.frame({ type: "state", state: tuiState({ phase: "stopping" }) });
  terminal.drop(4409);
  chat.frame({ type: "state", state: chatState() });
  const input = page.getByRole("textbox", { name: "消息" });
  await expect.element(input).toBeVisible();
  expect(sockets.filter((socket) => socket.path === terminalPath)).toHaveLength(
    1,
  );

  await page.getByRole("tab", { name: "终端" }).click();
  await expect
    .poll(() => commands().at(-1))
    .toEqual({
      method: "POST",
      path: switchPath,
      body: { clientRequestId: expect.any(String), to: "tui" },
    });
  chat.frame({
    type: "state",
    state: chatState({ side: "switching", phase: "stopping" }),
  });
  await expect
    .element(page.getByRole("heading", { name: "正在切换到终端…" }))
    .toBeVisible();
  await expect.poll(steps).toEqual([
    ["中断中", "完成"],
    ["退出中", "进行中"],
    ["启动中", "等待"],
    ["连接终端", "等待"],
  ]);
  chat.frame({ type: "state", state: tuiState() });
  finish({ side: "tui", dropped: [], unsupported: [] });

  // The progress stays until the new run has drawn on this page.
  const reattached = await socketAt(terminalPath, 1);
  expect(reattached).not.toBe(terminal);
  await expect.poll(steps).toEqual([
    ["中断中", "完成"],
    ["退出中", "完成"],
    ["启动中", "完成"],
    ["连接终端", "进行中"],
  ]);
  // A new run that has not drawn yet keeps the progress. Wait out the real
  // minimum display (400 ms) first: fake timers would also stop
  // expect.element's polling, so the clock cannot be advanced instead.
  reattached.binary(2, { ...info, generation: "fedcba9876543210" });
  const lingered = Promise.withResolvers<void>();
  setTimeout(lingered.resolve, 600);
  await lingered.promise;
  await expect.element(progress()).toBeVisible();
  reattached.binary(1, "prompt> ");
  await expect.element(progress()).not.toBeInTheDocument();
  await expect.element(input).not.toBeInTheDocument();
  await expect
    .element(page.getByRole("tab", { name: "终端" }))
    .toHaveAttribute("aria-selected", "true");
});

test("prompts still queued return to this device's draft; a new TUI session ID is announced", async () => {
  const finish = pendingSwitch();
  const { terminal, chat, draft } = await onTerminalSide(tuiState());
  chat.frame({ type: "state", state: tuiState({ phase: "stopping" }) });
  terminal.drop(4409);
  chat.frame({ type: "state", state: chatState() });
  await expect
    .element(page.getByRole("textbox", { name: "消息" }))
    .toBeVisible();

  await page.getByRole("tab", { name: "终端" }).click();
  chat.frame({
    type: "state",
    state: chatState({ side: "switching", phase: "starting" }),
  });
  const ids = { from: "01a17e26c4b84f0e9d2a", to: "01b2c3d4e5f60718293a" };
  chat.frame({ type: "state", state: tuiState({ idChange: ids }) });
  finish({
    side: "tui",
    draft: "然后更新 README\n\n最后运行测试",
    dropped: ["--resume"],
    unsupported: [],
  });
  await expect
    .element(page.getByText("排队的消息已退回到本机草稿，不会自动发送。"))
    .toBeVisible();
  await expect
    .element(page.getByText("以下参数在恢复时未保留：--resume"))
    .toBeVisible();
  await expect
    .element(
      page.getByText(
        "TUI 已切换到新会话 01b2c3d4…（上一个：01a17e26…），切到聊天时接手新会话",
      ),
    )
    .toBeVisible();
  expect(draft.text).toBe("然后更新 README\n\n最后运行测试");
  expect(draft.enabled).toBe(true);
  // Returned prompts are never sent by themselves.
  expect(commands().map((request) => request.body)).toEqual([
    { clientRequestId: expect.any(String), to: "tui" },
  ]);
});
