import { afterEach, expect, test, vi } from "vitest";
import { page, userEvent } from "vitest/browser";
import { cleanup, render } from "vitest-browser-svelte/pure";
import LaunchConfigDialog from "../../src/components/settings/LaunchConfigDialog.svelte";
import type { LaunchConfig } from "../../src/contracts/schemas";

const saved: LaunchConfig = {
  id: "abcdefg2",
  name: "Claude 恢复",
  cmd: ["claude", "--resume"],
  env: { API_KEY: "secret-value" },
  chat: false,
  chatSupport: {
    supported: false,
    supportedAgents: [],
    reason: "unknown_agent",
  },
  createdAt: "2026-10-23T00:00:00Z",
  updatedAt: "2026-10-23T00:00:00Z",
};

type Handler = (
  method: string,
  body: unknown,
) => Response | Promise<Response> | undefined;

function serve(routes: Record<string, Handler>) {
  const requests: { method: string; path: string; body: unknown }[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>(async (input, init) => {
      const path = new URL(String(input), location.href).pathname;
      const method = init?.method ?? "GET";
      const body = init?.body ? JSON.parse(String(init.body)) : undefined;
      requests.push({ method, path, body });
      const response = await routes[path]?.(method, body);
      if (!response) throw new Error(`Unexpected request: ${method} ${path}`);
      return response;
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
  }
});

const save = () => page.getByRole("button", { name: "保存", exact: true });

test.each([
  ["Antigravity CLI", "antigravity"],
  ["Gemini CLI（旧配置）", "geminicli"],
  ["Cursor CLI", "cursor"],
  ["GitHub Copilot CLI", "githubcopilot"],
  ["Qwen Code", "qwen"],
  ["Kimi Code CLI", "kimi"],
  ["CodeBuddy Code", "codebuddy"],
  ["自动匹配", undefined],
])(
  "edits the icon to %s while preserving the command and env",
  async (label, icon) => {
    const onsaved = vi.fn();
    const requests = serve({
      "/api/launch-configs/abcdefg2": (method, body) =>
        method === "GET"
          ? Response.json({ ...saved, icon: "codex" })
          : Response.json({ ...saved, ...(body as object) }),
    });
    render(LaunchConfigDialog, { open: true, configId: saved.id, onsaved });
    const picker = page.getByRole("combobox", { name: "图标", exact: true });
    await expect.element(picker).toMatchTextContent(/Codex/);
    await picker.click();
    await page.getByRole("option", { name: label, exact: true }).click();
    await save().click();
    await vi.waitFor(() => expect(onsaved).toHaveBeenCalledOnce());
    expect(requests.at(-1)).toMatchObject({
      method: "PUT",
      body: { cmd: saved.cmd, env: saved.env },
    });
    expect((requests.at(-1)!.body as { icon?: string }).icon).toBe(icon);
  },
);

test("shows the legacy Gemini icon as a labeled option", async () => {
  serve({
    "/api/launch-configs/abcdefg2": () =>
      Response.json({ ...saved, icon: "geminicli" }),
  });
  render(LaunchConfigDialog, {
    open: true,
    configId: saved.id,
    onsaved: () => {},
  });
  const picker = page.getByRole("combobox", { name: "图标", exact: true });
  await expect.element(picker).toMatchTextContent("Gemini CLI（旧配置）");
  await picker.click();
  await expect
    .element(
      page.getByRole("option", {
        name: "Gemini CLI（旧配置）",
        exact: true,
      }),
    )
    .toBeVisible();
});

test("masks env values by default and validates variable names", async () => {
  serve({});
  render(LaunchConfigDialog, {
    open: true,
    configId: undefined,
    onsaved: () => {},
  });
  await page.getByLabelText("名称").fill("Agent");
  await page.getByLabelText("启动命令").fill("claude");
  await page.getByRole("button", { name: "添加环境变量" }).click();
  const value = page.getByLabelText("变量值 1");
  await expect.element(value).toHaveAttribute("type", "password");
  const toggle = page.getByRole("button", { name: "显示变量值 1" });
  await expect.element(toggle).toHaveAttribute("aria-pressed", "false");
  await toggle.click();
  await expect.element(value).toHaveAttribute("type", "text");
  await expect.element(toggle).toHaveAttribute("aria-pressed", "true");
  await toggle.click();
  await expect.element(value).toHaveAttribute("type", "password");

  const name = page.getByLabelText("变量名 1");
  await name.fill("WEBMUX_SESSION_ID");
  await expect
    .element(page.getByText("WEBMUX_SESSION_ID 由 WebMux 保留"))
    .toBeVisible();
  await expect.element(save()).toBeDisabled();
  await name.fill("1BAD");
  await expect
    .element(page.getByText(/变量名只能包含字母、数字和下划线/))
    .toBeVisible();
  await expect.element(save()).toBeDisabled();
  await name.fill("API_KEY");
  await expect.element(save()).toBeEnabled();
});

test("the startup-file checkbox saves a normal command without expanding its arguments", async () => {
  const onsaved = vi.fn();
  const requests = serve({
    "/api/launch-configs": (method, body) =>
      method === "POST"
        ? Response.json({ ...saved, ...(body as object) }, { status: 201 })
        : undefined,
  });
  render(LaunchConfigDialog, { open: true, configId: undefined, onsaved });
  await page.getByLabelText("名称").fill("Open Code");
  await page
    .getByLabelText("启动命令")
    .fill(`opencode --continue 'a b' '' '$HOME'`);
  await page
    .getByRole("checkbox", { name: "加载 shell 启动文件", exact: true })
    .click();
  await expect
    .element(page.getByRole("combobox", { name: "启动文件 shell" }))
    .toMatchTextContent(/Bash/);
  await save().click();
  await vi.waitFor(() => expect(onsaved).toHaveBeenCalledOnce());
  expect(requests.at(-1)).toEqual({
    method: "POST",
    path: "/api/launch-configs",
    body: {
      name: "Open Code",
      cmd: [
        "bash",
        "-ic",
        'exec -- "$@"',
        "webmux",
        "opencode",
        "--continue",
        "a b",
        "",
        "$HOME",
      ],
      env: {},
      preview: null,
      attention: null,
      chat: false,
    },
  });
});

test("the default shell preselects Zsh and can be switched back to Bash", async () => {
  const onsaved = vi.fn();
  const requests = serve({
    "/api/launch-configs": (method, body) =>
      method === "POST"
        ? Response.json({ ...saved, ...(body as object) }, { status: 201 })
        : undefined,
  });
  render(LaunchConfigDialog, {
    open: true,
    configId: undefined,
    defaultShell: "zsh",
    onsaved,
  });
  await page.getByLabelText("名称").fill("omp");
  await page.getByLabelText("启动命令").fill("omp --auto-approve");
  await page
    .getByRole("checkbox", { name: "加载 shell 启动文件", exact: true })
    .click();
  const picker = page.getByRole("combobox", { name: "启动文件 shell" });
  await expect.element(picker).toMatchTextContent(/Zsh/);
  await picker.click();
  await page.getByRole("option", { name: "Bash（~/.bashrc）" }).click();
  await save().click();
  await vi.waitFor(() => expect(onsaved).toHaveBeenCalledOnce());
  expect(requests.at(-1)).toMatchObject({
    method: "POST",
    body: {
      cmd: ["bash", "-ic", 'exec -- "$@"', "webmux", "omp", "--auto-approve"],
    },
  });
});

test("saves per-config preview settings", async () => {
  const onsaved = vi.fn();
  const requests = serve({
    "/api/launch-configs/abcdefg2": (method, body) =>
      method === "GET"
        ? Response.json({ ...saved, preview: { lines: 24, skip: 3 } })
        : Response.json({ ...saved, ...(body as object) }),
  });
  render(LaunchConfigDialog, {
    open: true,
    configId: saved.id,
    onsaved,
  });
  const lines = page.getByLabelText("显示行数");
  const skip = page.getByLabelText("跳过末尾行数");
  await expect.element(lines).toHaveValue(24);
  await expect.element(skip).toHaveValue(3);
  await lines.fill("30");
  await skip.fill("5");
  await save().click();
  await vi.waitFor(() => expect(onsaved).toHaveBeenCalledOnce());
  expect(requests.at(-1)?.body).toMatchObject({
    preview: { lines: 30, skip: 5 },
  });
});

test("following the global default saves a null preview", async () => {
  const onsaved = vi.fn();
  const requests = serve({
    "/api/launch-configs/abcdefg2": (method, body) =>
      method === "GET"
        ? Response.json({ ...saved, preview: { lines: 24, skip: 3 } })
        : Response.json({ ...saved, ...(body as object) }),
  });
  render(LaunchConfigDialog, {
    open: true,
    configId: saved.id,
    onsaved,
  });
  await page
    .getByRole("checkbox", { name: /跟随全局默认\s*（输出预览）/ })
    .click();
  await expect
    .poll(() => page.getByLabelText("显示行数").elements().length)
    .toBe(0);
  await save().click();
  await vi.waitFor(() => expect(onsaved).toHaveBeenCalledOnce());
  expect(requests.at(-1)?.body).toMatchObject({ preview: null });
});

test("configures per-config attention settings, shows signal priority, and hides irrelevant fields by mode", async () => {
  const onsaved = vi.fn();
  const requests = serve({
    "/api/launch-configs/abcdefg2": (method, body) =>
      method === "GET"
        ? Response.json(saved)
        : Response.json({ ...saved, ...(body as object) }),
  });
  render(LaunchConfigDialog, {
    open: true,
    configId: saved.id,
    onsaved,
  });
  const followDefault = page.getByRole("checkbox", {
    name: /跟随全局默认\s*（完成与介入提醒）/,
  });
  await expect.element(followDefault).toBeChecked();
  await followDefault.click();
  await expect
    .element(page.getByText(/优先级：状态协议 \(OSC 7501\) > Agent 事件/))
    .toBeVisible();

  await page.getByRole("checkbox", { name: "状态协议 (OSC 7501)" }).click();
  await page.getByLabelText("最短回合耗时（秒）").fill("10");
  await page.getByLabelText("完成确认延迟（秒）").fill("3");

  // Switching to "仅需介入" hides threshold inputs while keeping signal sources.
  await page.getByRole("radio", { name: "仅需介入", exact: true }).click();
  await expect
    .poll(() => page.getByLabelText("最短回合耗时（秒）").elements().length)
    .toBe(0);
  await expect
    .element(page.getByRole("checkbox", { name: "Agent 事件 (Warp 协议)" }))
    .toBeVisible();

  // Switching to "关闭" hides both signal sources and threshold inputs.
  await page.getByRole("radio", { name: "关闭", exact: true }).click();
  await expect
    .poll(
      () =>
        page
          .getByRole("checkbox", { name: "Agent 事件 (Warp 协议)" })
          .elements().length,
    )
    .toBe(0);

  // Switch back to "全部提醒" and save.
  await page.getByRole("radio", { name: "全部提醒", exact: true }).click();
  const scrollBody = page.getByTestId("launch-config-scroll");
  await expect.element(scrollBody).toBeVisible();
  await save().click();
  await vi.waitFor(() => expect(onsaved).toHaveBeenCalledOnce());
  expect(requests.at(-1)?.body).toMatchObject({
    attention: {
      mode: "all",
      programStatus: false,
      agentProtocol: true,
      notify: true,
      bell: true,
      minTurnSeconds: 10,
      doneDelaySeconds: 3,
    },
  });
});

test.each([
  ["bash", true],
  ["bash", false],
  ["zsh", true],
  ["zsh", false],
] as const)(
  "editing a wrapped %s config keeps loading %s without losing argv or env",
  async (shell, enabled) => {
    const argv = ["codex", "--resume", "a b", "", "$HOME"];
    const wrapped = [shell, "-ic", 'exec -- "$@"', "webmux", ...argv];
    const onsaved = vi.fn();
    const requests = serve({
      "/api/launch-configs/abcdefg2": (method, body) =>
        method === "GET"
          ? Response.json({ ...saved, cmd: wrapped })
          : Response.json({ ...saved, ...(body as object) }),
    });
    await render(LaunchConfigDialog, {
      open: true,
      configId: saved.id,
      defaultShell: shell === "zsh" ? "bash" : "zsh",
      onsaved,
    });
    const checkbox = page.getByRole("checkbox", {
      name: "加载 shell 启动文件",
      exact: true,
    });
    await expect.element(checkbox).toBeChecked();
    await expect
      .element(page.getByRole("combobox", { name: "启动文件 shell" }))
      .toMatchTextContent(shell === "zsh" ? /Zsh/ : /Bash/);
    await expect
      .element(page.getByLabelText("启动命令"))
      .toHaveValue("codex --resume 'a b' '' '$HOME'");
    if (!enabled) await checkbox.click();
    await save().click();
    await vi.waitFor(() => expect(onsaved).toHaveBeenCalledOnce());
    expect(requests.at(-1)).toMatchObject({
      method: "PUT",
      body: { cmd: enabled ? wrapped : argv, env: saved.env },
    });
  },
);

test("editing only the name keeps every env variable in the PUT body", async () => {
  const loaded = Promise.withResolvers<Response>();
  const onsaved = vi.fn();
  const requests = serve({
    "/api/launch-configs/abcdefg2": (method, body) =>
      method === "GET"
        ? loaded.promise
        : Response.json({ ...saved, ...(body as object) }),
  });
  render(LaunchConfigDialog, { open: true, configId: "abcdefg2", onsaved });
  await expect.element(page.getByText("正在读取启动配置…")).toBeVisible();
  await expect.element(save()).toBeDisabled();
  loaded.resolve(Response.json(saved));
  const name = page.getByLabelText("名称");
  await expect.element(name).toHaveValue("Claude 恢复");
  await expect
    .element(page.getByLabelText("启动命令"))
    .toHaveValue("claude --resume");
  await expect
    .element(page.getByLabelText("变量值 1"))
    .toHaveAttribute("type", "password");
  await name.fill("Claude 新名称");
  await save().click();
  await vi.waitFor(() => expect(onsaved).toHaveBeenCalledOnce());
  expect(requests.at(-1)).toEqual({
    method: "PUT",
    path: "/api/launch-configs/abcdefg2",
    body: {
      name: "Claude 新名称",
      cmd: ["claude", "--resume"],
      env: { API_KEY: "secret-value" },
      preview: null,
      attention: null,
      chat: false,
    },
  });
});

test("a failed load shows the error instead of a savable form", async () => {
  serve({
    "/api/launch-configs/abcdefg2": () =>
      Response.json({ error: "启动配置不存在" }, { status: 404 }),
  });
  render(LaunchConfigDialog, {
    open: true,
    configId: "abcdefg2",
    onsaved: () => {},
  });
  await expect
    .element(page.getByRole("alert"))
    .toHaveTextContent("无法读取启动配置：启动配置不存在");
  expect(save().elements()).toHaveLength(0);
  expect(page.getByLabelText("名称").elements()).toHaveLength(0);
});

test("save errors stay inline and keep the input", async () => {
  let status = 409;
  serve({
    "/api/launch-configs": () =>
      Response.json(
        {
          error:
            status === 409
              ? "启动配置名称已存在"
              : "启动配置不可用: 文件损坏（/state/launch-configs.json）",
        },
        { status },
      ),
  });
  const onsaved = vi.fn();
  render(LaunchConfigDialog, { open: true, configId: undefined, onsaved });
  await page.getByLabelText("名称").fill("Agent");
  await page.getByLabelText("启动命令").fill("claude");
  await save().click();
  await expect
    .element(page.getByRole("alert"))
    .toMatchTextContent(/已有同名启动配置“Agent”/);
  await expect.element(page.getByLabelText("名称")).toHaveValue("Agent");
  status = 503;
  await save().click();
  await expect
    .element(page.getByRole("alert"))
    .toMatchTextContent(/\/state\/launch-configs\.json/);
  await expect.element(page.getByLabelText("启动命令")).toHaveValue("claude");
  expect(onsaved).not.toHaveBeenCalled();
});

test("the hybrid checkbox follows the server's verdict for the edited command", async () => {
  const onsaved = vi.fn();
  const asked: unknown[] = [];
  const requests = serve({
    "/api/launch-configs/abcdefg2": (method, body) =>
      method === "GET"
        ? Response.json({ ...saved, cmd: ["/bin/bash", "-l"], env: {} })
        : Response.json({ ...saved, ...(body as object) }),
    "/api/launch-configs/chat-support": (_method, body) => {
      asked.push(body);
      return Response.json({
        supported: true,
        supportedAgents: [],
        agent: "omp",
      });
    },
  });
  render(LaunchConfigDialog, { open: true, configId: saved.id, onsaved });
  const hybrid = page.getByRole("checkbox", {
    name: "混合模式（聊天 ↔ 终端）",
  });
  await expect.element(hybrid).toBeDisabled();
  await expect.element(page.getByText("未识别到支持的 Agent")).toBeVisible();
  // Opening the dialog reuses the saved verdict instead of asking again.
  expect(asked).toEqual([]);
  await page.getByLabelText("启动命令").fill("omp --model haiku");
  await expect.element(hybrid).toBeEnabled();
  expect(asked).toEqual([{ cmd: ["omp", "--model", "haiku"] }]);
  await hybrid.click();
  await save().click();
  await vi.waitFor(() => expect(onsaved).toHaveBeenCalledOnce());
  expect(requests.at(-1)).toMatchObject({
    method: "PUT",
    body: { cmd: ["omp", "--model", "haiku"], chat: true },
  });
});
