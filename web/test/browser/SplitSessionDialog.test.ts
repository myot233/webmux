import { afterEach, expect, test, vi } from "vitest";
import { page, userEvent } from "vitest/browser";
import { cleanup, render } from "vitest-browser-svelte/pure";
import SplitSessionDialog from "../../src/components/terminal/SplitSessionDialog.svelte";
import type { SessionInfo } from "../../src/contracts/schemas";

afterEach(async () => {
  try {
    await cleanup();
    await userEvent.cleanup();
  } finally {
    vi.unstubAllGlobals();
  }
});

function makeSession(overrides: Partial<SessionInfo>): SessionInfo {
  return {
    id: "sessa234",
    generation: "0123456789abcdef",
    title: "后端构建",
    cwd: "/repo/backend",
    cmd: ["bash"],
    status: "running",
    exitCode: null,
    listeningPorts: [],
    state: "active",
    views: ["terminal"],
    view: "terminal",
    transition: null,
    agent: null,
    preview: "",
    createdAt: "2026-10-16T00:00:00Z",
    lastActiveAt: "2026-10-16T00:00:01Z",
    cols: 80,
    rows: 24,
    clients: 0,
    workspace: null,
    attention: null,
    reportedCwd: null,
    ...overrides,
  };
}

const sessions: SessionInfo[] = [
  makeSession({ id: "sessa234", title: "后端构建", cwd: "/repo/backend" }),
  makeSession({
    id: "sessb234",
    title: "前端调试",
    cwd: "/repo/web",
    status: "exited",
    exitCode: 0,
  }),
  makeSession({ id: "sessc234", title: "当前会话", cwd: "/repo/current" }),
];

test("filters out excluded sessions, searches by title or cwd, and selects with direction", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof globalThis.fetch>(async () => Response.json(sessions)),
  );
  const onclose = vi.fn();
  const onselect = vi.fn();
  render(SplitSessionDialog, {
    excluded: ["sessc234"],
    onclose,
    onselect,
  });

  await expect
    .element(page.getByRole("button", { name: /后端构建/ }))
    .toBeVisible();
  await expect
    .element(page.getByRole("button", { name: /前端调试（已退出）/ }))
    .toBeVisible();
  await expect
    .element(page.getByRole("button", { name: /当前会话/ }))
    .not.toBeInTheDocument();

  const search = page.getByRole("textbox", { name: "搜索已有会话" });
  await search.fill("/repo/web");
  await expect
    .element(page.getByRole("button", { name: /后端构建/ }))
    .not.toBeInTheDocument();
  const exitedBtn = page.getByRole("button", { name: /前端调试（已退出）/ });
  await expect.element(exitedBtn).toBeVisible();

  await exitedBtn.click();
  expect(onselect).toHaveBeenCalledWith("sessb234", "horizontal");

  await page.getByRole("button", { name: "取消" }).click();
  expect(onclose).toHaveBeenCalledOnce();
});

test("shows empty state when no session matches and surfaces API errors", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof globalThis.fetch>(async () => Response.json([])),
  );
  render(SplitSessionDialog, {
    excluded: [],
    onclose: () => {},
    onselect: () => {},
  });
  await expect
    .element(page.getByText("没有可加入的会话。可从分屏菜单新建 Shell。"))
    .toBeVisible();

  await cleanup();
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof globalThis.fetch>(async () =>
      Response.json({ error: "加载会话失败" }, { status: 500 }),
    ),
  );
  render(SplitSessionDialog, {
    excluded: [],
    onclose: () => {},
    onselect: () => {},
  });
  await expect
    .element(page.getByRole("alert"))
    .toHaveTextContent("加载会话失败");
});
