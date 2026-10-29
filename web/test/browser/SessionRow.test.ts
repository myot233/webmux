import { afterEach, expect, test, vi } from "vitest";
import { page, userEvent } from "vitest/browser";
import { cleanup, render } from "vitest-browser-svelte/pure";
import SessionRow from "../../src/components/SessionRow.svelte";
import { setLaunchPreviews } from "../../src/launchPreviews.svelte";
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

test("an agent permission request is labelled precisely", async () => {
  render(SessionRow, {
    session: {
      ...base,
      attention: {
        kind: "permission",
        source: "agent",
        since: "2026-10-16T00:00:02Z",
      },
    },
    now: Date.parse("2026-10-16T00:00:03Z"),
    onclose: () => {},
  });
  await expect.element(page.getByText("等待授权")).toBeVisible();
  expect(page.getByText("可能在等你").elements()).toHaveLength(0);
  expect(
    document.querySelector(
      '[data-status="waiting"] [data-slot="session-status"][data-tone="warning"]',
    ),
  ).not.toBeNull();
});

test("heuristic waiting keeps the cautious label", async () => {
  render(SessionRow, {
    session: base,
    now: Date.parse("2026-10-16T00:00:03Z"),
    onclose: () => {},
  });
  await expect.element(page.getByText("可能在等你")).toBeVisible();
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

test("offers a copy action for the shell directory", async () => {
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
  await expect
    .element(page.getByRole("menuitem", { name: "复制 Shell 报告目录" }))
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

test("a chat-side hybrid card shows its side and the latest agent message", async () => {
  render(SessionRow, {
    session: {
      ...base,
      status: "idle",
      preview: "已修复排序。\n新增两个测试。\n全部通过。",
      hybrid: {
        side: "acp",
        dormant: false,
        state: "active",
        agent: "omp",
        agentSessionId: "01a",
      },
    },
    now: Date.parse("2026-10-16T00:00:03Z"),
    onclose: () => {},
  });
  await expect.element(page.getByText("聊天", { exact: true })).toBeVisible();
  const message = document.querySelector('[aria-label="最新消息预览"]');
  expect(message?.textContent?.trim()).toBe(
    "已修复排序。\n新增两个测试。\n全部通过。",
  );
  expect(document.querySelector('[data-slot="terminal-preview"]')).toBeNull();
});

test("terminal-side and switching hybrid cards keep the terminal preview", async () => {
  const hybrid = {
    side: "tui" as const,
    dormant: false,
    state: "active" as const,
    agent: "opencode",
    agentSessionId: null,
  };
  render(SessionRow, {
    session: { ...base, hybrid },
    now: Date.parse("2026-10-16T00:00:03Z"),
    onclose: () => {},
  });
  await expect.element(page.getByText("终端", { exact: true })).toBeVisible();
  expect(
    document.querySelector('[data-slot="terminal-preview"]'),
  ).not.toBeNull();
  await cleanup();
  render(SessionRow, {
    session: {
      ...base,
      hybrid: { ...hybrid, side: "switching", phase: "stopping" },
    },
    now: Date.parse("2026-10-16T00:00:03Z"),
    onclose: () => {},
  });
  await expect.element(page.getByText("切换中", { exact: true })).toBeVisible();
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
      hybrid: {
        side: "acp",
        dormant: false,
        state: "active",
        agent: "omp",
        agentSessionId: "01a",
      },
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
