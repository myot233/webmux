import { afterEach, expect, test, vi } from "vitest";
import { page, userEvent } from "vitest/browser";
import { cleanup, render } from "vitest-browser-svelte/pure";
import TerminalNotices from "../../src/components/terminal/TerminalNotices.svelte";
import type { ListeningPort } from "../../src/contracts/schemas";
import { createTerminalDrafts } from "../../src/domains/terminal/terminal-draft.svelte";
import { body, forward, seedForwards, stubFetch } from "./forward-fixture";
import { seedSettings } from "./settings-fixture";
import { stopSettings } from "../../src/domains/settings/settings.svelte";

afterEach(async () => {
  try {
    await cleanup();
    await userEvent.cleanup();
  } finally {
    stopSettings();
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  }
});

const terminal = {
  control: undefined,
  controlling: false,
  connection: "connected" as const,
  exitCode: null,
  error: "",
  claim: vi.fn(),
};

function notices(fields: Record<string, unknown> = {}) {
  return render(TerminalNotices, {
    terminal,
    uploads: { progress: null, cancel: vi.fn() },
    draft: createTerminalDrafts().get("notice-test"),
    draftSaved: "",
    error: "",
    notice: "",
    showScratch: vi.fn(),
    dismissScratch: vi.fn(),
    remove: vi.fn(),
    ...fields,
  });
}

const ports = (...list: number[]): ListeningPort[] =>
  list.map((port) => ({ port, process: "node" }));
const prompt = (port: number) =>
  page.getByText(`检测到端口 ${port}（node）正在监听`);

test("dismissed terminal alerts stay hidden without suppressing later alerts", async () => {
  const view = await notices({
    terminal: { ...terminal, error: "History was discarded" },
  });
  const alert = page.getByRole("alert");
  const close = page.getByRole("button", { name: "关闭提示" });
  await expect.element(alert).toBeVisible();
  await close.click();
  await expect.element(alert).not.toBeInTheDocument();

  await view.rerender({ terminal: { ...terminal, error: "Connection lost" } });
  await expect.element(alert).toBeVisible();
  await userEvent.keyboard("{Tab}");
  await expect.element(close).toHaveFocus();
  await userEvent.keyboard("{Enter}");
  await expect.element(alert).not.toBeInTheDocument();

  await view.rerender({ terminal: { ...terminal, error: "" } });
  await view.rerender({
    terminal: { ...terminal, error: "History was discarded" },
  });
  await expect.element(alert).toBeVisible();
});

test("ports forwarded elsewhere are not prompted; the rest are, ascending", async () => {
  await notices({ session: "webwebwe", listening: ports(5173) });
  // Until the forward list is known, nothing is prompted.
  await expect.element(prompt(5173)).not.toBeInTheDocument();
  seedForwards([forward(5173)]);
  const view = await notices({
    session: "apiserve",
    listening: ports(9229, 3000),
  });
  await expect.element(prompt(3000)).toBeVisible();
  await expect.element(prompt(9229)).toBeVisible();
  await expect.element(prompt(5173)).not.toBeInTheDocument();
  const strips = page.getByText(/^检测到端口/).elements();
  expect(strips.map((strip) => strip.textContent)).toEqual([
    "检测到端口 3000（node）正在监听",
    "检测到端口 9229（node）正在监听",
  ]);
  // Forwarded from another device: the prompt goes away.
  seedForwards([forward(5173), forward(3000)]);
  await expect.element(prompt(3000)).not.toBeInTheDocument();
  await view.rerender({ listening: ports(9229) });
  await expect.element(prompt(9229)).toBeVisible();
});

test("an ignored port prompts again after it stops and restarts listening", async () => {
  seedForwards([]);
  // A session of its own: the ignore set outlives the rendered component.
  const view = await notices({ session: "ignoring", listening: ports(5173) });
  await expect.element(prompt(5173)).toBeVisible();
  await page.getByRole("button", { name: "忽略" }).click();
  await expect.element(prompt(5173)).not.toBeInTheDocument();
  // Still listening: still ignored.
  await view.rerender({ listening: ports(5173) });
  await expect.element(prompt(5173)).not.toBeInTheDocument();
  // Leaving the page and coming back: the remount starts before the session
  // info arrives, and that unknown state must not forget the ignored port.
  await cleanup();
  const remounted = await notices({ session: "ignoring" });
  await remounted.rerender({ listening: ports(5173) });
  await expect.element(prompt(5173)).not.toBeInTheDocument();

  await remounted.rerender({ listening: [] });
  await remounted.rerender({ listening: ports(5173) });
  await expect.element(prompt(5173)).toBeVisible();
});

test("a failed forward keeps the prompt and shows the server's reason", async () => {
  seedForwards([]);
  stubFetch(() =>
    Response.json({ error: "port already forwarded: 5173" }, { status: 409 }),
  );
  await notices({ session: "webwebwe", listening: ports(5173) });
  await page.getByRole("button", { name: "转发", exact: true }).click();
  await expect
    .element(page.getByRole("alert"))
    .toHaveTextContent("转发失败：port already forwarded: 5173");
  await expect.element(prompt(5173)).toBeVisible();
});

test("forwarding announces the path until dismissed", async () => {
  seedForwards([]);
  const fetch = stubFetch(() =>
    Response.json(forward(5173, { session: "webwebwe", process: "node" }), {
      status: 201,
    }),
  );
  const write = vi
    .spyOn(navigator.clipboard, "writeText")
    .mockResolvedValue(undefined);
  await notices({ session: "webwebwe", listening: ports(5173) });
  await page.getByRole("button", { name: "转发", exact: true }).click();
  const done = page.getByText("端口 5173 已转发到 /forward/5173/");
  await expect.element(done).toBeVisible();
  expect(body(fetch.mock.calls[0]![1])).toEqual({
    port: 5173,
    session: "webwebwe",
    process: "node",
  });
  await expect.element(prompt(5173)).not.toBeInTheDocument();
  await expect
    .element(page.getByRole("link", { name: "打开" }))
    .toHaveAttribute("href", "/forward/5173/");
  await page.getByRole("button", { name: "复制链接" }).click();
  expect(write).toHaveBeenCalledWith(`${location.origin}/forward/5173/`);
  await page.getByRole("button", { name: "知道了" }).click();
  await expect.element(done).not.toBeInTheDocument();
});

// A stand-in for the blank tab: the prompt touches only opener, location
// and close, so the rest of Window is irrelevant.
function blankTab() {
  const tab = { opener: window, location: { href: "" }, close: vi.fn() };
  const stand = tab as unknown as Window;
  return { tab, open: vi.spyOn(window, "open").mockReturnValue(stand) };
}

test("forward and open points a tab opened inside the click at the forward", async () => {
  seedForwards([]);
  const { tab, open } = blankTab();
  stubFetch(() => {
    // The tab already exists while the request is in flight.
    expect(open).toHaveBeenCalledWith("", "_blank");
    return Response.json(forward(5173), { status: 201 });
  });
  await notices({ session: "webwebwe", listening: ports(5173) });
  await page.getByRole("button", { name: "转发并打开" }).click();
  await vi.waitFor(() =>
    expect(tab.location.href).toBe(`${location.origin}/forward/5173/`),
  );
  expect(tab.opener).toBeNull();
  expect(tab.close).not.toHaveBeenCalled();
});

test("a failed forward and open closes the blank tab", async () => {
  seedForwards([]);
  stubFetch(() => Response.json({ error: "boom" }, { status: 500 }));
  const { tab } = blankTab();
  await notices({ session: "webwebwe", listening: ports(5173) });
  await page.getByRole("button", { name: "转发并打开" }).click();
  await vi.waitFor(() => expect(tab.close).toHaveBeenCalledOnce());
  expect(tab.location.href).toBe("");
  await expect
    .element(page.getByRole("alert"))
    .toHaveTextContent("转发失败：boom");
});

test("more than two ports collapse into one strip that opens the dialog", async () => {
  seedForwards([]);
  const showForwards = vi.fn();
  await notices({
    session: "webwebwe",
    listening: ports(9229, 3000, 6006, 5173),
    showForwards,
  });
  await expect.element(prompt(3000)).toBeVisible();
  await expect.element(prompt(5173)).toBeVisible();
  await expect.element(prompt(6006)).not.toBeInTheDocument();
  await expect.element(page.getByText("另有 2 个端口")).toBeVisible();
  await page.getByRole("button", { name: "查看全部" }).click();
  expect(showForwards).toHaveBeenCalledOnce();
});

test("ports from 30000 up are ignored by default and prompted when the setting is turned off", async () => {
  await seedSettings();
  seedForwards([]);
  await notices({
    session: "webwebwe",
    listening: ports(5173, 29999, 30000, 45000),
  });
  await expect.element(prompt(5173)).toBeVisible();
  await expect.element(prompt(29999)).toBeVisible();
  await expect.element(prompt(30000)).not.toBeInTheDocument();
  await expect.element(page.getByText(/^另有/)).not.toBeInTheDocument();

  await seedSettings({ ignoreHighPorts: false });
  await expect.element(page.getByText("另有 2 个端口")).toBeVisible();
});
