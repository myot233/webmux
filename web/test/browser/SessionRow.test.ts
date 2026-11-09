import { afterEach, expect, test, vi } from "vitest";
import { page, userEvent } from "vitest/browser";
import { cleanup, render } from "vitest-browser-svelte/pure";
import SessionRow from "../../src/components/SessionRow.svelte";
import { setLaunchPreviews } from "../../src/launch-previews.svelte";
import type { SessionInfo } from "../../src/schemas";

afterEach(async () => {
  vi.restoreAllMocks();
  await cleanup();
  await userEvent.cleanup();
});

const base: SessionInfo = {
  id: "abcdefgh",
  generation: "0123456789abcdef",
  title: "omp agent",
  cwd: "/work",
  cmd: ["omp"],
  status: "waiting",
  exitCode: null,
  listeningPorts: [],
  state: "active",
  views: ["terminal"],
  view: "terminal",
  transition: null,
  agent: null,
  preview: "⠋ Working",
  createdAt: "2026-10-16T00:00:00Z",
  lastActiveAt: "2026-10-16T00:00:01Z",
  cols: 80,
  rows: 24,
  clients: 0,
  workspace: null,
  attention: null,
  reportedCwd: null,
};

test("opens the terminal in this tab", async () => {
  render(SessionRow, {
    session: base,
    now: Date.parse("2026-10-16T00:00:03Z"),
    onclose: () => {},
  });
  const link = page.getByRole("link", { name: base.title, exact: true });
  await expect.element(link).toHaveAttribute("href", `/s/${base.id}`);
  await expect.element(link).not.toHaveAttribute("target");
  const start = location.href;
  const before = history.length;
  link
    .element()
    .dispatchEvent(
      new MouseEvent("click", { bubbles: true, cancelable: true, button: 0 }),
    );
  expect(location.pathname).toBe(`/s/${base.id}`);
  expect(history.length).toBe(before + 1);
  history.replaceState(null, "", start);
});

test("blocking and error attention kinds use the destructive badge", async () => {
  for (const [kind, label] of [
    ["question", "需要回答"],
    ["permission", "等待授权"],
    ["error", "出错停止"],
  ] as const) {
    render(SessionRow, {
      session: {
        ...base,
        attention: {
          kind,
          source: "agent",
          since: "2026-10-16T00:00:02Z",
        },
      },
      now: Date.parse("2026-10-16T00:00:03Z"),
      onclose: () => {},
    });
    await expect.element(page.getByText(label)).toBeVisible();
    expect(page.getByText("可能在等你").elements()).toHaveLength(0);
    expect(
      document.querySelector(
        '[data-status="waiting"] [data-slot="session-status"][data-tone="destructive"]',
      ),
    ).not.toBeNull();
    await cleanup();
  }
});

test("non-blocking attention and heuristic waiting keep the warning badge", async () => {
  for (const [attention, label] of [
    [
      {
        kind: "done" as const,
        source: "agent" as const,
        since: "2026-10-16T00:00:02Z",
      },
      "本轮完成",
    ],
    [null, "可能在等你"],
  ] as const) {
    render(SessionRow, {
      session: { ...base, attention },
      now: Date.parse("2026-10-16T00:00:03Z"),
      onclose: () => {},
    });
    await expect.element(page.getByText(label)).toBeVisible();
    expect(
      document.querySelector(
        '[data-status="waiting"] [data-slot="session-status"][data-tone="warning"]',
      ),
    ).not.toBeNull();
    await cleanup();
  }
});

test("shows the shell directory without repeating the group path", async () => {
  render(SessionRow, {
    session: {
      ...base,
      cwd: "/work/main",
      reportedCwd: {
        path: "/work/main/src",
        host: "",
        remote: false,
        since: "2026-10-16T00:00:02Z",
      },
    },
    groupPath: "/work/main",
    now: Date.parse("2026-10-16T00:00:03Z"),
    onclose: () => {},
  });
  await expect.element(page.getByText("Shell 报告目录")).toBeVisible();
  await expect.element(page.getByText("/work/main/src")).toBeVisible();
  expect(page.getByText("启动目录").elements()).toHaveLength(0);
});

test("copies the shell directory and shows confirmation", async () => {
  const writeText = vi
    .spyOn(navigator.clipboard, "writeText")
    .mockResolvedValue(undefined);
  render(SessionRow, {
    session: {
      ...base,
      cwd: "/work/main",
      reportedCwd: {
        path: "/work/main/src",
        host: "",
        remote: false,
        since: "2026-10-16T00:00:02Z",
      },
    },
    groupPath: "/work/main",
    now: Date.parse("2026-10-16T00:00:03Z"),
    onclose: () => {},
  });
  await page.getByRole("button", { name: /会话菜单/ }).click();
  await page.getByRole("menuitem", { name: "复制 Shell 报告目录" }).click();
  expect(writeText).toHaveBeenCalledWith("/work/main/src");
  await expect
    .element(page.getByRole("status").filter({ hasText: "已复制" }))
    .toBeVisible();
});

test("shows a launch directory below the worktree root", async () => {
  render(SessionRow, {
    session: { ...base, cwd: "/work/main/sub" },
    groupPath: "/work/main",
    now: Date.parse("2026-10-16T00:00:03Z"),
    onclose: () => {},
  });
  await expect.element(page.getByText("启动目录")).toBeVisible();
  await expect.element(page.getByText("/work/main/sub")).toBeVisible();
  expect(page.getByText("Shell 报告目录").elements()).toHaveLength(0);
});

test("marks a remote shell directory", async () => {
  render(SessionRow, {
    session: {
      ...base,
      cwd: "/work/main",
      reportedCwd: {
        path: "/srv/agent",
        host: "build.example",
        remote: true,
        since: "2026-10-16T00:00:02Z",
      },
    },
    groupPath: "/work/main",
    now: Date.parse("2026-10-16T00:00:03Z"),
    onclose: () => {},
  });
  await expect.element(page.getByText("Shell 报告目录")).toBeVisible();
  await expect.element(page.getByText("/srv/agent")).toBeVisible();
  await expect.element(page.getByText("远端 build.example")).toBeVisible();
});

test("the status follows the title and the facts line sits below it", async () => {
  render(SessionRow, {
    session: { ...base, status: "exited", exitCode: 137 },
    now: Date.parse("2026-10-16T00:00:03Z"),
    onclose: () => {},
  });
  const labels = [base.title, "已退出 · code 137", "后台会话 · 刚刚"];
  await expect.element(page.getByText(labels[1])).toBeVisible();
  const [title, status, facts] = labels.map((label) => {
    const element = page.getByText(label, { exact: true }).element();
    const range = document.createRange();
    range.selectNodeContents(element);
    return range.getBoundingClientRect();
  });
  // The status sits on the title's line, right after it, so a status
  // change grows only to the right and moves nothing else.
  expect(status!.left).toBeGreaterThan(title!.right);
  // Between them: the open arrow, the gap and the badge's dot.
  expect(status!.left - title!.right).toBeLessThanOrEqual(64);
  expect(
    Math.abs(
      status!.top + status!.height / 2 - (title!.top + title!.height / 2),
    ),
  ).toBeLessThanOrEqual(2);
  // The facts start below the title, aligned with it.
  expect(facts!.top).toBeGreaterThanOrEqual(title!.bottom);
  expect(Math.abs(facts!.left - title!.left)).toBeLessThanOrEqual(1);
});

test("uses the launch config preview rows and height", async () => {
  setLaunchPreviews([{ id: "abcdefg2", preview: { lines: 3, skip: 2 } }]);
  const rows = Array.from({ length: 12 }, (_, index) => `row ${index + 1}`);
  render(SessionRow, {
    session: {
      ...base,
      configId: "abcdefg2",
      preview: [...rows, "status bar", "prompt"].join("\n"),
    },
    now: Date.parse("2026-10-16T00:00:03Z"),
    onclose: () => {},
  });
  const preview = document.querySelector('[data-slot="terminal-preview"]');
  expect(preview?.textContent?.trim()).toBe("row 10\nrow 11\nrow 12");
  expect(preview?.getAttribute("style")).toContain("--preview-lines: 3");
  setLaunchPreviews([]);
});

const chatAgent = {
  name: "opencode",
  title: "OpenCode",
  icon: "opencode",
  sessionId: "01a",
};
const chatHybrid = {
  views: ["terminal", "chat"] as SessionInfo["views"],
  view: "chat" as const,
  agent: chatAgent,
};
// Rendered text: an SVG <title> is not drawn, so innerText leaves it out.
const agentElement = () =>
  document.querySelector<HTMLElement>('[data-slot="session-agent"]');
const agentLine = () => agentElement()?.parentElement?.innerText;

test("a chat-side hybrid card names its agent and shows the latest agent message", async () => {
  render(SessionRow, {
    session: {
      ...base,
      status: "idle",
      clients: 2,
      preview: "已修复排序。\n新增两个测试。\n全部通过。",
      ...chatHybrid,
    },
    now: Date.parse("2026-10-16T00:00:03Z"),
    onclose: () => {},
  });
  await expect.element(page.getByText("聊天", { exact: true })).toBeVisible();
  // The agent's brand glyph replaces the generic chat icon.
  const agent = agentElement();
  expect(agent?.innerText).toBe("OpenCode");
  expect(
    agent?.querySelector('[data-launch-icon="opencode"] svg'),
  ).not.toBeNull();
  expect(agent?.querySelector('[aria-hidden="true"]')).not.toBeNull();
  expect(document.querySelectorAll("[data-launch-icon]")).toHaveLength(1);
  expect(agentLine()?.replace(/\s+/g, " ").trim()).toMatch(
    /^OpenCode · 聊天 · 2 个设备已连接 ·/,
  );
  const message = document.querySelector('[aria-label="最新消息预览"]');
  expect(message?.textContent?.trim()).toBe(
    "已修复排序。\n新增两个测试。\n全部通过。",
  );
  expect(document.querySelector('[data-slot="terminal-preview"]')).toBeNull();
});

test("a chat card renders known brand glyphs; an unknown glyph leaves the name alone", async () => {
  render(SessionRow, {
    session: {
      ...base,
      ...chatHybrid,
      agent: { ...chatAgent, title: "omp", icon: "pi" },
    },
    now: Date.parse("2026-10-16T00:00:03Z"),
    onclose: () => {},
  });
  await expect.element(page.getByText("聊天", { exact: true })).toBeVisible();
  expect(
    document.querySelector(
      '[data-slot="session-agent"] [data-launch-icon="pi"]',
    ),
  ).not.toBeNull();
  await cleanup();
  render(SessionRow, {
    session: {
      ...base,
      ...chatHybrid,
      agent: { ...chatAgent, title: "Future", icon: "future" },
    },
    now: Date.parse("2026-10-16T00:00:03Z"),
    onclose: () => {},
  });
  await expect.element(page.getByText("Future", { exact: true })).toBeVisible();
  expect(document.querySelector("[data-launch-icon]")).toBeNull();
  expect(document.querySelector('[data-slot="session-agent"] svg')).toBeNull();
});

test("a working chat card shows its live work detail instead of a placeholder", async () => {
  render(SessionRow, {
    session: {
      ...base,
      status: "running",
      preview: "",
      ...chatHybrid,
      agent: { ...chatAgent, work: "读取 internal/wire/wire.go" },
    },
    now: Date.parse("2026-10-16T00:00:03Z"),
    onclose: () => {},
  });
  const work = document.querySelector('[aria-label="当前工作"]');
  expect(work?.textContent).toContain("读取 internal/wire/wire.go");
  expect(page.getByText("等待 Agent 回复…").elements()).toHaveLength(0);
  expect(page.getByLabelText("最新消息预览").elements()).toHaveLength(0);
  const link = document.querySelector("[data-session-link]");
  expect(link && getComputedStyle(link).paddingBottom).toBe("16px");
});

test("a chat card without preview or work detail renders no message block", async () => {
  render(SessionRow, {
    session: {
      ...base,
      status: "idle",
      preview: "",
      ...chatHybrid,
    },
    now: Date.parse("2026-10-16T00:00:03Z"),
    onclose: () => {},
  });
  await expect.element(page.getByText("聊天", { exact: true })).toBeVisible();
  expect(page.getByLabelText("当前工作").elements()).toHaveLength(0);
  expect(page.getByLabelText("最新消息预览").elements()).toHaveLength(0);
  expect(page.getByText("等待 Agent 回复…").elements()).toHaveLength(0);
  const link = document.querySelector("[data-session-link]");
  expect(link && getComputedStyle(link).paddingBottom).toBe("16px");
});

test("terminal-side and switching hybrid cards keep the agent and the terminal preview", async () => {
  // A session resumed to the terminal still names the agent it runs.
  const hybrid = {
    ...chatHybrid,
    view: "terminal" as const,
    agent: { ...chatAgent, sessionId: null },
  };
  render(SessionRow, {
    session: { ...base, clients: 1, ...hybrid },
    now: Date.parse("2026-10-16T00:00:03Z"),
    onclose: () => {},
  });
  await expect.element(page.getByText("终端", { exact: true })).toBeVisible();
  expect(
    agentElement()?.querySelector('[data-launch-icon="opencode"] svg'),
  ).not.toBeNull();
  expect(agentLine()?.replace(/\s+/g, " ").trim()).toMatch(
    /^OpenCode · 终端 · 1 个设备已连接 ·/,
  );
  expect(
    document.querySelector('[data-slot="terminal-preview"]'),
  ).not.toBeNull();
  await cleanup();
  render(SessionRow, {
    session: {
      ...base,
      ...hybrid,
      transition: { to: "chat", phase: "stopping" },
    },
    now: Date.parse("2026-10-16T00:00:03Z"),
    onclose: () => {},
  });
  await expect.element(page.getByText("切换中", { exact: true })).toBeVisible();
  const agent = agentElement();
  expect(agent?.innerText).toBe("OpenCode");
  expect(
    agent?.querySelector('[data-launch-icon="opencode"] svg'),
  ).not.toBeNull();
  expect(
    document.querySelector('[data-slot="terminal-preview"]'),
  ).not.toBeNull();
});

test("a deferred menu keeps mouse, keyboard, Escape focus and close actions", async () => {
  const onclose = vi.fn();
  render(SessionRow, {
    session: base,
    now: Date.parse("2026-10-16T00:00:03Z"),
    onclose,
  });
  const trigger = page.getByRole("button", { name: "会话菜单 omp agent" });
  const copy = page.getByRole("menuitem", { name: "复制启动目录" });
  await trigger.click();
  await expect.element(copy).toBeVisible();
  await userEvent.keyboard("{Escape}");
  await expect.element(trigger).toHaveFocus();
  await expect.poll(() => copy.elements()).toHaveLength(0);
  await userEvent.keyboard("{ArrowDown}");
  await expect.element(copy).toHaveFocus();
  await userEvent.keyboard("{End}{Enter}");
  expect(onclose).toHaveBeenCalledExactlyOnceWith(base);
  await expect.element(trigger).toHaveFocus();
});

test("compact rows keep session state and location hints without rendering previews", async () => {
  render(SessionRow, {
    session: {
      ...base,
      reportedCwd: {
        path: "/work/sub",
        host: "",
        remote: false,
        since: base.lastActiveAt,
      },
      ...chatHybrid,
    },
    compact: true,
    groupPath: base.cwd,
    now: Date.parse("2026-10-16T00:00:03Z"),
    onclose: () => {},
  });
  await expect.element(page.getByText("聊天", { exact: true })).toBeVisible();
  await expect.element(page.getByText("可能在等你")).toBeVisible();
  await expect
    .element(page.getByText("/work/sub", { exact: true }))
    .toBeVisible();
  expect(page.getByLabelText("最新消息预览").elements()).toHaveLength(0);
  expect(document.querySelector('[data-slot="terminal-preview"]')).toBeNull();
});
