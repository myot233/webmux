import { afterEach, expect, test, vi } from "vitest";
import { page, userEvent } from "vitest/browser";
import { cleanup, render } from "vitest-browser-svelte/pure";
import CloseSessionDialog from "../../src/components/sessions/CloseSessionDialog.svelte";
import type { SessionInfo } from "../../src/contracts/schemas";

afterEach(async () => {
  try {
    await cleanup();
    await userEvent.cleanup();
  } finally {
    vi.unstubAllGlobals();
  }
});

const shell: SessionInfo = {
  id: "shell001",
  generation: "0123456789abcdef",
  title: "bash",
  cwd: "/work",
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
  createdAt: "2026-10-28T00:00:00Z",
  lastActiveAt: "2026-10-28T00:00:00Z",
  cols: 80,
  rows: 24,
  clients: 0,
  workspace: null,
  attention: null,
  reportedCwd: null,
};
const hybrid: SessionInfo = {
  ...shell,
  id: "hybrid01",
  title: "修复排序",
  views: ["terminal", "chat"],
  view: "chat",
  agent: { name: "omp", title: "omp", icon: "pi", sessionId: "01a" },
};

function show(session: SessionInfo, status: number) {
  const fetch = vi.fn<typeof globalThis.fetch>(async () =>
    status === 204
      ? new Response(null, { status })
      : Response.json({ error: "无法关闭" }, { status }),
  );
  vi.stubGlobal("fetch", fetch);
  const onarchived = vi.fn();
  const onclose = vi.fn();
  render(CloseSessionDialog, { session, onclose, onarchived });
  return { fetch, onarchived, onclose };
}

test("a live hybrid session is archived without a destructive confirmation", async () => {
  const { fetch, onarchived, onclose } = show(hybrid, 204);
  const dialog = page.getByRole("alertdialog");
  await expect
    .element(dialog.getByRole("heading"))
    .toHaveTextContent("停止并归档“修复排序”？");
  await expect.element(dialog).toMatchTextContent(/对话保留在 agent 中/);
  const archive = dialog.getByRole("button", {
    name: "停止并归档",
    exact: true,
  });
  // Archiving keeps the conversation: not styled as a destructive action.
  expect(archive.element().className).not.toMatch(/\bbg-destructive/);
  await archive.click();
  await vi.waitFor(() => expect(onclose).toHaveBeenCalledOnce());
  expect(onarchived).toHaveBeenCalledWith(hybrid);
  expect(String(fetch.mock.calls[0]![0])).toMatch(/\/sessions\/hybrid01$/);
});

test("a failed archive stays in the dialog with the error", async () => {
  const { onarchived, onclose } = show(hybrid, 500);
  await page.getByRole("button", { name: "停止并归档", exact: true }).click();
  await expect.element(page.getByRole("alert")).toHaveTextContent("无法关闭");
  expect(onarchived).not.toHaveBeenCalled();
  expect(onclose).not.toHaveBeenCalled();
});

test("an ordinary session keeps the close wording", async () => {
  const { onarchived, onclose } = show(shell, 204);
  const dialog = page.getByRole("alertdialog");
  await expect
    .element(dialog.getByRole("heading"))
    .toHaveTextContent("关闭“bash”？");
  await dialog.getByRole("button", { name: "确认关闭" }).click();
  await vi.waitFor(() => expect(onclose).toHaveBeenCalledOnce());
  expect(onarchived).not.toHaveBeenCalled();
});
