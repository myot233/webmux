import { afterEach, expect, test, vi } from "vitest";
import { page, userEvent } from "vitest/browser";
import { cleanup, render } from "vitest-browser-svelte/pure";
import SessionRow from "../../src/components/sessions/SessionRow.svelte";
import type { SessionInfo } from "../../src/contracts/schemas";

afterEach(async () => {
  try {
    await cleanup();
    await userEvent.cleanup();
  } finally {
    vi.unstubAllGlobals();
  }
});

const session: SessionInfo = {
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
  preview: "",
  createdAt: "2026-10-16T00:00:00Z",
  lastActiveAt: "2026-10-16T00:00:01Z",
  cols: 80,
  rows: 24,
  clients: 0,
  workspace: null,
  attention: null,
  reportedCwd: null,
};

async function open(response: () => Response) {
  const fetch = vi.fn<typeof globalThis.fetch>(async () => response());
  vi.stubGlobal("fetch", fetch);
  render(SessionRow, {
    session,
    now: Date.parse("2026-10-16T00:00:03Z"),
    onclose: () => {},
  });
  await page.getByRole("button", { name: /会话菜单/ }).click();
  await page.getByRole("menuitem", { name: "修改标题" }).click();
  return fetch;
}

test("the row menu renames a session through PATCH", async () => {
  const fetch = await open(() =>
    Response.json({ ...session, title: "发布检查" }),
  );
  const field = page.getByRole("textbox", { name: "标题" });
  await expect.element(field).toHaveValue("omp agent");
  await field.fill("  发布检查 ");
  await page.getByRole("button", { name: "保存" }).click();
  await vi.waitFor(() => expect(fetch).toHaveBeenCalledOnce());
  const [url, init] = fetch.mock.calls[0]!;
  expect(String(url)).toMatch(/\/sessions\/abcdefgh$/);
  expect(init?.method).toBe("PATCH");
  expect(JSON.parse(String(init?.body))).toEqual({ name: "发布检查" });
  await expect
    .element(page.getByRole("dialog", { name: "修改标题" }))
    .not.toBeInTheDocument();
});

test("an empty title is sent so the automatic one returns", async () => {
  const fetch = await open(() => Response.json(session));
  await page.getByRole("textbox", { name: "标题" }).fill("");
  await userEvent.keyboard("{Enter}");
  await vi.waitFor(() => expect(fetch).toHaveBeenCalledOnce());
  expect(JSON.parse(String(fetch.mock.calls[0]![1]?.body))).toEqual({
    name: "",
  });
});

test("a refused rename stays open and shows the reason", async () => {
  await open(() =>
    Response.json({ error: "名称不能含控制字符" }, { status: 400 }),
  );
  await page.getByRole("textbox", { name: "标题" }).fill("x");
  await page.getByRole("button", { name: "保存" }).click();
  await expect
    .element(page.getByRole("alert"))
    .toHaveTextContent("名称不能含控制字符");
  await expect
    .element(page.getByRole("textbox", { name: "标题" }))
    .toHaveValue("x");
});
