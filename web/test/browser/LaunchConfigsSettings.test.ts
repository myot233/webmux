import {
  reloadSettings,
  stopSettings,
} from "../../src/domains/settings/settings.svelte";
import { emptySettings } from "../../src/domains/settings/settings-intent";
import { afterEach, expect, test, vi } from "vitest";
import { page, userEvent } from "vitest/browser";
import { cleanup, render } from "vitest-browser-svelte/pure";
import Settings from "../../src/pages/Settings.svelte";

const config = {
  id: "abcdefg2",
  name: "Claude 恢复",
  cmd: ["claude", "--resume", "a b"],
  envNames: ["API_KEY"],
  chatOptions: [],
  chat: false,
  chatSupport: {
    supported: false,
    supportedAgents: [],
    reason: "unknown_agent",
  },
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
      if (path === "/api/launch-configs/presets" && method === "GET")
        return Response.json([]);
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
  // The list shows a summary; the command itself is in the editor.
  await expect.element(item).toMatchTextContent(/1 个环境变量/);
  await expect.element(item).not.toMatchTextContent(/claude --resume/);
  expect(
    item.element().querySelector('[data-launch-icon="claudecode"] svg'),
  ).not.toBeNull();
  await card().getByRole("button", { name: "操作 Claude 恢复" }).click();
  await page.getByRole("menuitem", { name: "删除 Claude 恢复" }).click();
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
        chatOptions: [],
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

test("hybrid configs are marked in settings", async () => {
  const values = { ...emptySettings };
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>(async (input) => {
      const path = new URL(String(input), location.href).pathname;
      if (path === "/api/settings")
        return Response.json({ revision: 0, updatedAt: "", values });
      if (path === "/api/launch-configs")
        return Response.json([
          {
            ...config,
            name: "omp",
            cmd: ["omp"],
            chat: true,
            chatSupport: {
              supported: true,
              supportedAgents: [],
              agent: "omp",
            },
          },
        ]);
      return Response.json({ error: "not found" }, { status: 404 });
    }),
  );
  stopSettings();
  await reloadSettings();
  render(Settings);
  await expect
    .element(card().getByRole("button", { name: "编辑 omp" }))
    .toMatchTextContent(/混合会话/);
});

test("an unavailable presets endpoint explains why the menu is disabled", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>(async (input, init) => {
      const path = new URL(String(input), location.href).pathname;
      const method = init?.method ?? "GET";
      if (path === "/api/settings")
        return Response.json({
          revision: 0,
          updatedAt: "",
          values: emptySettings,
        });
      if (path === "/api/launch-configs" && method === "GET")
        return Response.json([]);
      if (path === "/api/launch-configs/presets" && method === "GET")
        return Response.json({ error: "系统预设暂不可用" }, { status: 503 });
      return Response.json({ error: "not found" }, { status: 404 });
    }),
  );
  stopSettings();
  await reloadSettings();
  render(Settings);
  await expect
    .element(card().getByRole("alert"))
    .toMatchTextContent(/系统预设暂不可用/);
  await expect
    .element(
      card().getByRole("button", { name: "重新读取系统预设", exact: true }),
    )
    .toBeVisible();
  await expect
    .element(card().getByRole("button", { name: "添加系统预设", exact: true }))
    .toBeDisabled();
});

test("preset menu marks existing configs and restores one or all missing presets", async () => {
  const presets = [
    {
      name: "Shell",
      icon: "terminal",
      cmd: ["/bin/bash", "-l", "-i"],
      envNames: [],
      chatOptions: [],
      chat: false,
    },
    {
      name: "Claude Code",
      icon: "claudecode",
      cmd: ["claude", "--dangerously-skip-permissions"],
      envNames: [],
      chatOptions: [],
      chat: true,
    },
    {
      name: "omp",
      icon: "pi",
      cmd: ["omp", "--auto-approve"],
      envNames: ["PI_FORCE_IMAGE_PROTOCOL"],
      chatOptions: [],
      chat: true,
    },
  ];
  let configs = [
    {
      ...config,
      id: "abcdefg2",
      name: "Shell",
      icon: "terminal",
      cmd: ["/bin/zsh", "-l"],
      envNames: [],
      chatOptions: [],
    },
  ];
  const restoredCalls: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>(async (input, init) => {
      const path = new URL(String(input), location.href).pathname;
      const method = init?.method ?? "GET";
      if (path === "/api/settings")
        return Response.json({
          revision: 0,
          updatedAt: "",
          values: emptySettings,
        });
      if (path === "/api/launch-configs" && method === "GET")
        return Response.json(configs);
      if (path === "/api/launch-configs/presets" && method === "GET")
        return Response.json(presets);
      if (path === "/api/launch-configs/presets" && method === "POST") {
        const body = JSON.parse(String(init?.body ?? "{}")) as {
          name?: string;
        };
        restoredCalls.push(body);
        const toAdd = body.name
          ? presets.filter((p) => p.name === body.name)
          : presets.filter((p) => !configs.some((c) => c.name === p.name));
        configs = [
          ...configs,
          ...toAdd.map((p, index) => ({
            ...config,
            id: `abcd234${"bcdefgh"[configs.length + index]}`,
            name: p.name,
            icon: p.icon,
            cmd: p.cmd,
            envNames: p.envNames,
            chatOptions: [],
            chat: p.chat,
          })),
        ];
        return Response.json(configs);
      }
      return Response.json({ error: "not found" }, { status: 404 });
    }),
  );
  stopSettings();
  await reloadSettings();
  render(Settings);

  const menuButton = card().getByRole("button", {
    name: "添加系统预设",
    exact: true,
  });
  await expect.element(menuButton).toBeEnabled();
  await menuButton.click();

  const existingItem = page.getByRole("menuitem", {
    name: "Shell（已存在）",
    exact: true,
  });
  await expect.element(existingItem).toHaveAttribute("aria-disabled", "true");
  await expect
    .element(page.getByRole("menuitem", { name: /补回全部缺失预设/ }))
    .toMatchTextContent(/2 项/);

  await page
    .getByRole("menuitem", { name: "添加预设 omp", exact: true })
    .click();
  await expect
    .element(card().getByRole("button", { name: "编辑 omp" }))
    .toBeVisible();
  expect(restoredCalls).toEqual([{ name: "omp" }]);

  await menuButton.click();
  await page.getByRole("menuitem", { name: /补回全部缺失预设/ }).click();
  await expect
    .element(card().getByRole("button", { name: "编辑 Claude Code" }))
    .toBeVisible();
  expect(restoredCalls).toEqual([{ name: "omp" }, {}]);

  // Existing customized Shell command was not overwritten.
  await expect
    .element(card().getByRole("button", { name: "编辑 Shell" }))
    .toBeVisible();
  expect(configs.find((c) => c.name === "Shell")?.cmd).toEqual([
    "/bin/zsh",
    "-l",
  ]);
  expect(configs.filter((c) => c.name === "Shell")).toHaveLength(1);
});
