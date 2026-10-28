import { reloadSettings, stopSettings } from "../../src/settings.svelte";
import { emptySettings } from "../../src/settings-intent";
import { afterEach, expect, test, vi } from "vitest";
import { page, userEvent } from "vitest/browser";
import { cleanup, render } from "vitest-browser-svelte/pure";
import Settings from "../../src/pages/Settings.svelte";

const config = {
  id: "abcdefg2",
  name: "Claude 恢复",
  cmd: ["claude", "--resume", "a b"],
  envNames: ["API_KEY"],
  hybrid: false,
  hybridSupport: { supported: false, reason: "unknown_agent" },
  createdAt: "2026-10-23T00:00:00Z",
  updatedAt: "2026-10-23T00:00:00Z",
};

function serve(list: () => Response) {
  const requests: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>(async (input, init) => {
      const path = new URL(String(input), location.href).pathname;
      if (path === "/api/settings")
        return Response.json({
          revision: 0,
          updatedAt: "",
          values: emptySettings,
        });
      const method = init?.method ?? "GET";
      requests.push(`${method} ${path}`);
      if (path === "/api/launch-configs" && method === "GET") return list();
      if (path === `/api/launch-configs/${config.id}` && method === "DELETE")
        return new Response(null, { status: 204 });
      // Other cards (push, tunnel) degrade on their own; not under test here.
      return Response.json({ error: "not found" }, { status: 404 });
    }),
  );
  return requests;
}

afterEach(async () => {
  try {
    await cleanup();
    await userEvent.cleanup();
  } finally {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
    localStorage.clear();
  }
});

const card = () => page.getByRole("region", { name: "启动配置", exact: true });

test("an unavailable store disables writes but keeps the rest of the page", async () => {
  serve(() =>
    Response.json(
      {
        error:
          "启动配置不可用: 状态目录已被另一实例使用（/state/launch-configs.json）",
      },
      { status: 503 },
    ),
  );
  stopSettings();
  await reloadSettings();
  render(Settings);
  await expect
    .element(card().getByRole("alert"))
    .toMatchTextContent(/\/state\/launch-configs\.json/);
  expect(
    card()
      .getByRole("button", { name: /新建启动配置/ })
      .elements(),
  ).toHaveLength(0);
  await expect
    .element(page.getByRole("button", { name: /添加快捷指令/ }).first())
    .toBeEnabled();
});

test("lists configs without env values and confirms deletion", async () => {
  let configs = [config];
  const requests = serve(() => Response.json(configs));
  stopSettings();
  await reloadSettings();
  render(Settings);
  const item = card().getByRole("button", { name: "编辑 Claude 恢复" });
  await expect.element(item).toMatchTextContent(/claude --resume 'a b'/);
  expect(
    item.element().querySelector('[data-launch-icon="claudecode"] svg'),
  ).not.toBeNull();
  await expect.element(card().getByText("1 个环境变量")).toBeVisible();
  await card().getByRole("button", { name: "删除 Claude 恢复" }).click();
  const confirm = page.getByRole("alertdialog");
  await expect.element(confirm).toMatchTextContent(/Claude 恢复/);
  await expect.element(confirm).toMatchTextContent(/claude --resume 'a b'/);
  configs = [];
  await confirm.getByRole("button", { name: "删除", exact: true }).click();
  await expect.element(card().getByText("还没有启动配置")).toBeVisible();
  expect(requests).toContain(`DELETE /api/launch-configs/${config.id}`);
});

test("the new button is disabled at the 100 config limit", async () => {
  serve(() =>
    Response.json(
      Array.from({ length: 100 }, (_, index) => ({
        ...config,
        id: `abcd${"234567"[index % 6]}${"234567"[Math.floor(index / 6) % 6]}${"abcdef"[Math.floor(index / 36)]}a`,
        name: `配置 ${index}`,
        envNames: [],
      })),
    ),
  );
  stopSettings();
  await reloadSettings();
  render(Settings);
  await expect
    .element(card().getByRole("button", { name: /新建启动配置/ }))
    .toBeDisabled();
  await expect.element(card().getByText(/最多 100 条/)).toBeVisible();
});

test("hybrid configs are marked and the chat idle sleep saves to the server", async () => {
  const patches: unknown[] = [];
  let values = { ...emptySettings };
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>(async (input, init) => {
      const path = new URL(String(input), location.href).pathname;
      const method = init?.method ?? "GET";
      if (path === "/api/settings" && method === "PATCH") {
        const body = JSON.parse(String(init!.body)) as {
          changes: Partial<typeof values>;
        };
        patches.push(body.changes);
        values = { ...values, ...body.changes };
        return Response.json({ revision: 1, updatedAt: "", values });
      }
      if (path === "/api/settings")
        return Response.json({ revision: 0, updatedAt: "", values });
      if (path === "/api/launch-configs")
        return Response.json([
          {
            ...config,
            name: "omp",
            cmd: ["omp"],
            hybrid: true,
            hybridSupport: { supported: true, agent: "omp" },
          },
        ]);
      return Response.json({ error: "not found" }, { status: 404 });
    }),
  );
  stopSettings();
  await reloadSettings();
  render(Settings);
  await expect.element(card().getByText("混合", { exact: true })).toBeVisible();
  const idle = page.getByLabelText("混合会话聊天端空闲休眠（分钟）");
  await expect.element(idle).toHaveValue(30);
  await idle.fill("0");
  await idle.element().dispatchEvent(new Event("change", { bubbles: true }));
  await vi.waitFor(() =>
    expect(patches).toEqual([{ chatIdleSleepMinutes: 0 }]),
  );
  await idle.fill("2000");
  await expect
    .element(page.getByText("请输入 0–1440 之间的整数。"))
    .toBeVisible();
});
