import "../../src/styles.css";
import { expect, test } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte/pure";
import Terminal from "../../src/pages/Terminal.svelte";
import type {
  ChatState,
  SessionInfo,
  ViewChangeResult,
} from "../../src/contracts/schemas";
import { createTerminalDrafts } from "../../src/domains/terminal/terminal-draft.svelte";
import {
  chatState,
  claudeConnector,
  claudeSessionIds,
  commands,
  events,
  FakeSocket,
  installChatFixture,
  respond,
  snapshot,
  socketAt,
  sockets,
} from "./chat-fixture";
import { seedForwards } from "./forward-fixture";

installChatFixture();

const id = "hybrid22";
const terminalPath = `/ws/s/${id}`;
const chatPath = `/ws/s/${id}/chat`;
const viewPath = `/api/sessions/${id}/view`;

const info: SessionInfo = {
  id,
  generation: "0123456789abcdef",
  title: "omp · 修复首页排序",
  cwd: "/work",
  cmd: ["omp"],
  status: "idle",
  exitCode: null,
  listeningPorts: [],
  state: "active",
  views: ["terminal", "chat"],
  view: "terminal",
  transition: null,
  agent: {
    name: "omp",
    title: "omp",
    icon: "pi",
    sessionId: "01a17e26c4b84f0e9d2a",
  },
  preview: "",
  createdAt: "2026-10-28T00:00:00Z",
  lastActiveAt: "2026-10-28T00:00:00Z",
  cols: 80,
  rows: 24,
  clients: 1,
  workspace: null,
  attention: null,
  reportedCwd: null,
};

// The session as the server describes it now: GET /api/sessions/hybrid22
// answers it, and the attached terminal stream reports each change.
let server: SessionInfo = info;
const described = () => ({
  ...server,
  generation: server.view === "chat" ? "" : server.generation,
});

const tuiState = (patch: Partial<ChatState> = {}) => chatState(patch);

/**
 * The session's view or transition changes: the attached terminal stream
 * reports it, and the chat channel publishes its state, after which the page
 * reads the session again.
 */
function move(
  streams: { terminal: FakeSocket; chat: FakeSocket },
  patch: Partial<SessionInfo>,
  state: ChatState = chatState(),
) {
  server = { ...server, ...patch };
  if (streams.terminal.readyState === FakeSocket.OPEN)
    streams.terminal.binary(2, server);
  streams.chat.frame({ type: "state", state });
}

/** The page on a session's terminal view, both streams attached. */
async function onTerminalSide(state: ChatState, claude = false) {
  await page.viewport(1280, 900);
  const sessionInfo: SessionInfo = {
    ...info,
    title: claude ? "Claude Code · 修复首页排序" : info.title,
    cmd: [claude ? "claude" : "omp"],
    agent: claude
      ? {
          name: "claudecode",
          title: "Claude Code",
          icon: "claudecode",
          sessionId: claudeSessionIds.current,
        }
      : info.agent,
  };
  server = sessionInfo;
  respond(/^GET \/api\/sessions\/hybrid22$/, () => Response.json(described()));
  const drafts = createTerminalDrafts();
  render(Terminal, {
    id,
    drafts,
    onUploadNotice: () => {},
    reopen: () => {},
  });
  const terminal = await socketAt(terminalPath);
  terminal.binary(2, sessionInfo);
  const chat = await socketAt(chatPath);
  chat.frame(snapshot(state));
  return { terminal, chat, draft: drafts.get(id) };
}

/** A switch request the test finishes when it chooses. */
function pendingSwitch() {
  const done = Promise.withResolvers<Response>();
  respond(/^POST \/api\/sessions\/hybrid22\/view$/, (request) => {
    const body = request.body;
    if (body && typeof body === "object" && "choice" in body)
      return Response.json({
        view: server.view,
        dropped: [],
        unsupported: [],
      });
    return done.promise;
  });
  return (result: ViewChangeResult) => done.resolve(Response.json(result));
}

const chatTab = (name: string | RegExp = /^聊天/) =>
  page.getByRole("tab", { name });
const progress = () => page.getByRole("status").filter({ hasText: "正在切换" });
const agentBadge = () => page.getByRole("img", { name: "Agent：omp" });
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
    tuiState({
      eligibility: { available: false, reason: "no_session_id" },
    }),
    true,
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
    state: tuiState({
      eligibility: { available: false, reason: "occupied" },
    }),
  });
  await expect
    .element(chatTab("聊天（该会话已在另一个会话中打开）"))
    .toHaveAttribute("aria-disabled", "true");
  chat.frame({
    type: "state",
    state: tuiState({}),
  });
  await expect.element(chatTab("聊天")).not.toHaveAttribute("aria-disabled");
});

test("a missing connector disables chat, explains installation and rechecks", async () => {
  const { chat } = await onTerminalSide(
    tuiState({
      eligibility: {
        available: false,
        reason: "connector_missing",
        connector: claudeConnector,
      },
    }),
    true,
  );
  await expect
    .element(
      chatTab("聊天（未安装 Claude Code ACP 适配器（claude-agent-acp））"),
    )
    .toHaveAttribute("aria-disabled", "true");
  await expect
    .element(page.getByText(claudeConnector.installCommand))
    .toBeVisible();
  await expect.element(page.getByText("安装后会自动重新检测")).toBeVisible();
  await page.getByRole("button", { name: "关闭安装提示" }).click();
  await expect
    .element(page.getByText(claudeConnector.installCommand))
    .not.toBeInTheDocument();

  // The next eligibility probe finds the installed connector without a reload.
  chat.frame({
    type: "state",
    state: tuiState({}),
  });
  await expect.element(chatTab("聊天")).not.toHaveAttribute("aria-disabled");
});

test("an ambiguous switch lets the user pick the agent session and resends it", async () => {
  let attempts = 0;
  respond(/^POST \/api\/sessions\/hybrid22\/view$/, () => {
    attempts += 1;
    if (attempts === 1)
      return Response.json(
        {
          error: "无法确定当前会话",
          reason: "session_ambiguous",
          candidates: [
            {
              agentSessionId: claudeSessionIds.current,
              title: "修复排序",
              updatedAt: "2026-10-28T00:00:00Z",
            },
            {
              agentSessionId: claudeSessionIds.alternative,
              title: "更新 README",
              updatedAt: "2026-10-28T00:01:00Z",
            },
          ],
        },
        { status: 409 },
      );
    return Response.json({ view: "chat", dropped: [], unsupported: [] });
  });
  await onTerminalSide(tuiState(), true);
  await chatTab("聊天").click();
  const picker = page.getByRole("combobox", { name: "选择要打开的会话" });
  await expect.element(picker).toBeVisible();
  await picker.selectOptions(claudeSessionIds.alternative);
  await page.getByRole("button", { name: "确认" }).click();
  await expect
    .poll(() => commands().map((command) => command.body))
    .toEqual([
      { clientRequestId: expect.any(String), to: "chat" },
      {
        clientRequestId: expect.any(String),
        to: "chat",
        agentSessionId: claudeSessionIds.alternative,
      },
    ]);
});

test("switching to chat shows its steps, asks after a stuck interrupt and lands on the chat view", async () => {
  const finish = pendingSwitch();
  const streams = await onTerminalSide(tuiState());
  const { terminal } = streams;
  await chatTab("聊天").click();
  await expect.poll(commands).toEqual([
    {
      method: "POST",
      path: viewPath,
      body: { clientRequestId: expect.any(String), to: "chat" },
    },
  ]);

  move(streams, { transition: { to: "chat", phase: "interrupting" } });
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
  move(
    streams,
    { transition: { to: "chat", phase: "awaiting_choice" } },
    tuiState({ forceSwitchNote: "omp 已输出的部分可能丢失" }),
  );
  const dialog = page.getByRole("alertdialog", { name: "TUI 仍在执行回合" });
  await expect.element(dialog).toBeVisible();
  await expect.element(dialog).toMatchTextContent(/omp 已输出的部分可能丢失/);
  await dialog.getByRole("button", { name: "强制切换" }).click();
  await expect
    .poll(() => commands().at(-1))
    .toEqual({
      method: "POST",
      path: viewPath,
      body: { clientRequestId: expect.any(String), choice: "force" },
    });

  move(streams, { transition: { to: "chat", phase: "stopping" } });
  await expect.element(dialog).not.toBeInTheDocument();
  await expect.poll(steps).toEqual([
    ["中断中", "完成"],
    ["退出中", "进行中"],
    ["启动中", "等待"],
    ["加载历史中", "等待"],
  ]);
  // The TUI exits: its stream closes with 4409 and does not reconnect.
  terminal.drop(4409);
  move(streams, { transition: { to: "chat", phase: "loading" } });
  await expect.poll(steps).toEqual([
    ["中断中", "完成"],
    ["退出中", "完成"],
    ["启动中", "完成"],
    ["加载历史中", "进行中"],
  ]);
  move(streams, { view: "chat", transition: null });
  finish({ view: "chat", dropped: [], unsupported: [] });

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

test("an agent title refreshes the chat side's header", async () => {
  const streams = await onTerminalSide(tuiState());
  const { chat } = streams;
  let title = "creating name";
  respond(/^GET \/api\/sessions\/hybrid22$/, () =>
    Response.json({ ...described(), title }),
  );
  // Another device moved the session to the chat side.
  move(streams, { transition: { to: "chat", phase: "stopping" } });
  streams.terminal.drop(4409);
  move(streams, { view: "chat", transition: null });
  await expect
    .element(page.getByText("creating name", { exact: true }))
    .toBeVisible();

  // The agent names its session: the header re-reads the server info, whose
  // title wins over the model's, and shows it instead.
  title = "server title";
  chat.frame({
    type: "events",
    generation: "g1",
    items: events({ kind: "info.set", info: { title: "agent title" } }),
  });
  await expect
    .element(page.getByText("server title", { exact: true }))
    .toBeVisible();
  await expect
    .element(page.getByText("agent title", { exact: true }))
    .not.toBeInTheDocument();
});

test("port prompts appear on both the terminal and chat views", async () => {
  seedForwards([]);
  const streams = await onTerminalSide(tuiState());
  const { terminal } = streams;
  const port = { port: 5173, process: "node" };
  terminal.binary(2, { ...server, listeningPorts: [port] });
  const prompt = page.getByText("检测到端口 5173（node）正在监听");
  await expect.element(prompt).toBeVisible();

  // Moved to the chat side: the session-list connection supplies live port
  // updates while the terminal stream is detached.
  respond(/^GET \/api\/sessions\/hybrid22$/, () =>
    Response.json({ ...described(), listeningPorts: [] }),
  );
  move(streams, { transition: { to: "chat", phase: "stopping" } });
  streams.terminal.drop(4409);
  move(streams, { view: "chat", transition: null });
  await expect
    .element(page.getByRole("textbox", { name: "消息" }))
    .toBeVisible();
  const listSocket = await socketAt("/ws/sessions", 1);
  listSocket.frame({
    type: "snapshot",
    sessions: [{ ...described(), listeningPorts: [] }],
    forwards: [],
  } as never);
  await expect.element(prompt).not.toBeInTheDocument();

  listSocket.frame({
    type: "updated",
    session: { ...described(), listeningPorts: [port] },
  } as never);
  await expect.element(prompt).toBeVisible();
});

test("a session moved by another device shows the chat view; switching back reattaches the terminal", async () => {
  const finish = pendingSwitch();
  const streams = await onTerminalSide(tuiState());
  const { terminal } = streams;
  // Another device moved the session: the terminal stream ends with 4409.
  move(streams, { transition: { to: "chat", phase: "stopping" } });
  streams.terminal.drop(4409);
  move(streams, { view: "chat", transition: null });
  const input = page.getByRole("textbox", { name: "消息" });
  await expect.element(input).toBeVisible();
  await expect.element(agentBadge()).toBeVisible();
  expect(sockets.filter((socket) => socket.path === terminalPath)).toHaveLength(
    1,
  );

  await page.getByRole("tab", { name: "终端" }).click();
  await expect
    .poll(() => commands().at(-1))
    .toEqual({
      method: "POST",
      path: viewPath,
      body: { clientRequestId: expect.any(String), to: "terminal" },
    });
  move(streams, { transition: { to: "terminal", phase: "stopping" } });
  await expect
    .element(page.getByRole("heading", { name: "正在切换到终端…" }))
    .toBeVisible();
  await expect.poll(steps).toEqual([
    ["中断中", "完成"],
    ["退出中", "进行中"],
    ["启动中", "等待"],
    ["连接终端", "等待"],
  ]);
  move(streams, { view: "terminal", transition: null });
  finish({ view: "terminal", dropped: [], unsupported: [] });

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
  reattached.binary(2, { ...server, generation: "fedcba9876543210" });
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
  await expect.element(agentBadge()).not.toBeInTheDocument();
});

test("the chat view names its agent beside the title, icon only on a phone", async () => {
  const streams = await onTerminalSide(tuiState());
  // The terminal view shows the agent itself; the header adds nothing.
  await expect
    .element(page.getByText(info.title, { exact: true }))
    .toBeVisible();
  await expect.element(agentBadge()).not.toBeInTheDocument();

  const title = `omp · ${"修复首页排序与置顶工作区".repeat(6)}`;
  respond(/^GET \/api\/sessions\/hybrid22$/, () =>
    Response.json({
      ...described(),
      generation: "",
      title,
      status: "waiting",
      attention: {
        kind: "question",
        source: "agent",
        since: "2026-10-16T00:00:02Z",
      },
      view: "chat",
      transition: null,
    }),
  );
  move(streams, { transition: { to: "chat", phase: "stopping" } });
  streams.terminal.drop(4409);
  move(streams, { view: "chat", transition: null });
  await expect.element(agentBadge()).toBeVisible();
  const badge = agentBadge().element() as HTMLElement;
  expect(badge.title).toBe("Agent：omp");
  expect(badge.querySelector('[data-launch-icon="pi"] svg')).not.toBeNull();
  expect(badge.innerText).toBe("omp");
  // Wide headers read title, agent, then the status dot (phones put the dot
  // first, by CSS order).
  await page.viewport(1024, 800);
  await expect.element(page.getByText(title, { exact: true })).toBeVisible();
  const heading = page
    .getByText(title, { exact: true })
    .element() as HTMLElement;
  const dot = badge.closest("header")!.querySelector(".status-dot")!;
  await expect.poll(() => dot.classList.contains("status-failed")).toBe(true);
  expect(dot.getAttribute("aria-label")).toMatch(/^需要回答 · /);
  await expect
    .poll(
      () =>
        heading.getBoundingClientRect().right <=
          badge.getBoundingClientRect().left &&
        badge.getBoundingClientRect().right <= dot.getBoundingClientRect().left,
    )
    .toBe(true);

  await page.viewport(390, 844);
  await expect.poll(() => badge.innerText).toBe("");
  const icon = badge.querySelector("[data-launch-icon]")!;
  expect(icon.getBoundingClientRect().width).toBeGreaterThan(0);
  // The title gives way; the agent keeps its full width inside the header.
  expect(heading.scrollWidth).toBeGreaterThan(heading.clientWidth);
  expect(badge.scrollWidth).toBeLessThanOrEqual(badge.clientWidth);
  const header = badge.closest("header")!.getBoundingClientRect();
  expect(badge.getBoundingClientRect().right).toBeLessThanOrEqual(header.right);
});

test("prompts still queued return to this device's draft; a new TUI session ID is announced", async () => {
  const finish = pendingSwitch();
  const streams = await onTerminalSide(tuiState());
  const { draft } = streams;
  move(streams, { transition: { to: "chat", phase: "stopping" } });
  streams.terminal.drop(4409);
  move(streams, { view: "chat", transition: null });
  await expect
    .element(page.getByRole("textbox", { name: "消息" }))
    .toBeVisible();

  await page.getByRole("tab", { name: "终端" }).click();
  move(streams, { transition: { to: "terminal", phase: "starting" } });
  const ids = { from: "01a17e26c4b84f0e9d2a", to: "01b2c3d4e5f60718293a" };
  move(
    streams,
    { view: "terminal", transition: null },
    tuiState({ idChange: ids }),
  );
  finish({
    view: "terminal",
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
    { clientRequestId: expect.any(String), to: "terminal" },
  ]);
});
