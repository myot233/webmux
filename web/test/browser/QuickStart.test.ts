import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { page, userEvent } from "vitest/browser";
import { render } from "vitest-browser-svelte/pure";
import { openSession, shellUi } from "../../src/shell.svelte";
import { createTerminalDrafts } from "../../src/terminal-draft.svelte";
import {
  commands,
  installChatFixture,
  respond,
  socketAt,
} from "./chat-fixture";
import ShellHarness from "./ShellHarness.svelte";
import { chat, launchConfig, repository, worktree } from "./shell-fixture";

installChatFixture();

const effort = {
  id: "effort",
  name: "思考",
  category: "thought_level",
  type: "select" as const,
  value: "medium",
  options: [
    { value: "medium", name: "中" },
    { value: "high", name: "高" },
  ],
};
const branches = {
  branches: [
    { name: "main", head: "a".repeat(40), worktreePaths: ["/repos/webmux"] },
    // Shares main's commit: branches are told apart by name.
    { name: "release", head: "a".repeat(40), worktreePaths: [] },
    { name: "feature", head: "b".repeat(40), worktreePaths: [] },
  ],
  bases: [
    {
      name: "refs/remotes/origin/HEAD",
      head: "a".repeat(40),
      subject: "init",
    },
    {
      name: "refs/remotes/origin/upstream-fix",
      head: "c".repeat(40),
      subject: "upstream fix",
    },
  ],
  defaultBase: "a".repeat(40),
  defaultBaseSubject: "init",
  defaultBranch: "main",
  defaultParent: "/repos/webmux.worktrees",
};

beforeEach(() => {
  shellUi.setMode("chat");
  history.replaceState(null, "", "/new");
  respond(/^GET \/api\/repositories$/, () =>
    Response.json([
      repository({
        id: "gone",
        name: "legacy-api",
        availability: "unavailable",
        error: "路径不可用",
        worktrees: [],
      }),
      repository({ initInstruction: "先读 AGENTS.md" }),
    ]),
  );
  respond(/^GET \/api\/repositories\/repo1\/branches$/, () =>
    Response.json(branches),
  );
  respond(/^GET \/api\/launch-configs$/, () =>
    Response.json([
      launchConfig({ id: "cfgplain", name: "Shell", chat: false }),
      launchConfig({ chatOptions: [effort] }),
      launchConfig({ id: "cfgcodex", name: "Codex", icon: "codex" }),
    ]),
  );
  respond(/^GET \/api\/sessions$/, () => Response.json([]));
});
afterEach(() => {
  for (const key of [
    "shellMode",
    "quickStart",
    "quickStartSide",
    "quick-start-draft",
  ])
    localStorage.removeItem(`webmux.${key}`);
  history.replaceState(null, "", "/");
});

async function open() {
  render(ShellHarness, { drafts: createTerminalDrafts() });
  const socket = await socketAt("/ws/sessions");
  socket.frame({ type: "snapshot", sessions: [], forwards: [] } as never);
  const input = page.getByRole("textbox", { name: "任务描述" });
  await expect.element(input).toHaveFocus();
  const toolbar = page.getByRole("toolbar", { name: "启动选项" });
  // Repository and branch sit above the input.
  const place = page.getByRole("toolbar", { name: "仓库与分支" });
  // Branches arrive after the repository list.
  await expect
    .element(place.getByRole("button", { name: "基准分支：main" }))
    .toBeVisible();
  return { input, toolbar, place };
}

test("defaults skip what is unusable, and the first send goes in one request", async () => {
  respond(/^POST \/api\/sessions$/, () =>
    Response.json(
      {
        session: chat({ id: "chataaaa" }),
        worktree: worktree({ id: "otter", kind: "linked" }),
        configWarnings: [],
      },
      { status: 201 },
    ),
  );
  const { input, toolbar, place } = await open();
  // The unavailable repository is skipped; only hybrid configs are agents.
  await expect
    .element(place.getByRole("button", { name: "仓库：webmux" }))
    .toBeVisible();
  await expect
    .element(toolbar.getByRole("button", { name: "Agent：Claude Code" }))
    .toBeVisible();
  await toolbar.getByRole("button", { name: "Agent：Claude Code" }).click();
  await expect
    .element(page.getByRole("menuitemradio", { name: /Codex/ }))
    .toBeVisible();
  expect(page.getByRole("menuitemradio").elements()).toHaveLength(2);
  await userEvent.keyboard("{Escape}");
  // The init instruction is an explicit, default-ticked choice above the input.
  await expect
    .element(page.getByRole("checkbox", { name: "使用初始化指令" }))
    .toBeChecked();
  // The remembered option catalog is editable before the agent starts.
  await toolbar.getByRole("button", { name: "思考：中" }).click();
  await page.getByRole("menuitemradio", { name: "高" }).click();

  await input.fill("修复首页排序");
  await userEvent.keyboard("{Enter}");
  await expect.poll(() => location.pathname).toBe("/s/chataaaa");
  const sent = commands().find((c) => c.path === "/api/sessions");
  expect(sent?.body).toEqual({
    clientRequestId: expect.stringMatching(/\S/),
    // The server names the new branch.
    newWorktree: { repositoryId: "repo1", mode: "new", base: "a".repeat(40) },
    configId: "cfgclaud",
    view: "chat",
    prompt: "修复首页排序",
    agentConfig: { effort: "high" },
    permissionMode: "auto",
    useInitInstruction: true,
    expandPrompt: true,
  });
  expect(JSON.parse(localStorage.getItem("webmux.quickStart")!)).toEqual({
    repositoryId: "repo1",
    configId: "cfgclaud",
  });
  expect(localStorage.getItem("webmux.quick-start-draft")).toBeNull();
});

test("long option labels leave both ends of the startup toolbar reachable", async () => {
  await page.viewport(667, 720);
  try {
    const longOptions = Array.from({ length: 4 }, (_, index) => ({
      id: `model-${index}`,
      name: `模型 ${index + 1}`,
      category: "model",
      type: "select" as const,
      value: "default",
      options: [
        { value: "default", name: "Long Provider / High Capability Model" },
        { value: "other", name: "Fast Model" },
      ],
    }));
    respond(/^GET \/api\/launch-configs$/, () =>
      Response.json([launchConfig({ chatOptions: longOptions })]),
    );
    respond(/^POST \/api\/launch-configs\/\w+\/options$/, () =>
      Response.json(longOptions),
    );
    const { toolbar } = await open();
    const agent = toolbar.getByRole("button", { name: "Agent：Claude Code" });
    await expect.element(agent).toBeEnabled();
    const row = toolbar.element();
    expect(row.scrollWidth).toBeGreaterThan(row.clientWidth);
    expect(agent.element().getBoundingClientRect().left).toBeGreaterThanOrEqual(
      row.getBoundingClientRect().left,
    );
    await agent.click();
    await expect
      .element(page.getByRole("menuitemradio", { name: /Claude Code/ }))
      .toBeVisible();
    await userEvent.keyboard("{Escape}");

    const last = toolbar.getByRole("button", { name: /^模型 4：/ });
    await last.click();
    expect(row.scrollLeft).toBeGreaterThan(0);
    await expect
      .element(page.getByRole("menuitemradio", { name: "Fast Model" }))
      .toBeVisible();
  } finally {
    await page.viewport(1024, 900);
  }
});

test("switching pages away from /new and back keeps all composer choices, attachments and text", async () => {
  StagingXHR.sent = [];
  vi.stubGlobal("XMLHttpRequest", StagingXHR);
  const { input, toolbar, place } = await open();
  await input.fill("先看别的会话再回来");

  await place.getByRole("button", { name: "基准分支：main" }).click();
  await page.getByRole("option", { name: "feature", exact: true }).click();
  await expect
    .element(place.getByRole("button", { name: "基准分支：feature" }))
    .toBeVisible();

  const box = page.getByRole("checkbox", { name: "使用初始化指令" });
  await box.click();
  await expect.element(box).not.toBeChecked();

  await toolbar.getByRole("button", { name: "权限：全部接受" }).click();
  await page.getByRole("menuitemradio", { name: /每次询问/ }).click();
  await userEvent.keyboard("{Escape}");

  await toolbar.getByRole("button", { name: "思考：中" }).click();
  await page.getByRole("menuitemradio", { name: /高/ }).click();
  await userEvent.keyboard("{Escape}");

  const picker = page
    .getByLabelText("选择上传文件")
    .element() as HTMLInputElement;
  const transfer = new DataTransfer();
  transfer.items.add(
    new File(["# 笔记"], "note.md", { type: "text/markdown" }),
  );
  picker.files = transfer.files;
  picker.dispatchEvent(new Event("change", { bubbles: true }));
  await expect.poll(() => StagingXHR.sent.length).toBe(1);
  const { xhr } = StagingXHR.sent[0]!;
  xhr.status = 201;
  xhr.response = { path: "/tmp/staging/note.md" };
  xhr.onload?.();
  await expect
    .element(page.getByRole("list", { name: "待发送附件" }))
    .toHaveTextContent("note.md");

  // Switch away to an open session page and back to /new.
  openSession({ id: "chataaaa" });
  await expect.poll(() => location.pathname).toBe("/s/chataaaa");
  await expect.element(page.getByTestId("session-page")).toBeVisible();
  history.pushState(null, "", "/new");
  window.dispatchEvent(new PopStateEvent("popstate"));
  await expect.poll(() => location.pathname).toBe("/new");

  await expect
    .element(page.getByRole("textbox", { name: "任务描述" }))
    .toHaveValue("先看别的会话再回来");
  await expect
    .element(place.getByRole("button", { name: "基准分支：feature" }))
    .toBeVisible();
  await expect
    .element(page.getByRole("checkbox", { name: "使用初始化指令" }))
    .not.toBeChecked();
  await expect
    .element(toolbar.getByRole("button", { name: "权限：每次询问" }))
    .toBeVisible();
  await expect
    .element(toolbar.getByRole("button", { name: "思考：高" }))
    .toBeVisible();
  await expect
    .element(page.getByRole("list", { name: "待发送附件" }))
    .toHaveTextContent("note.md");
});

const created = () =>
  Response.json(
    {
      session: chat({ id: "chataaaa" }),
      worktree: worktree({ id: "otter", kind: "linked" }),
      configWarnings: [],
    },
    { status: 201 },
  );

test("starting in the terminal drops chat-only controls, keeps attachments and is remembered", async () => {
  StagingXHR.sent = [];
  vi.stubGlobal("XMLHttpRequest", StagingXHR);
  respond(/^POST \/api\/sessions$/, created);
  const { input, toolbar } = await open();
  await expect
    .element(toolbar.getByRole("button", { name: "思考：中" }))
    .toBeVisible();
  await toolbar.getByRole("button", { name: "启动方式：聊天" }).click();
  await page.getByRole("menuitemradio", { name: "终端" }).click();
  await expect
    .element(toolbar.getByRole("button", { name: "启动方式：终端" }))
    .toBeVisible();
  expect(
    toolbar.getByRole("button", { name: "思考：中" }).elements(),
  ).toHaveLength(0);
  await expect
    .element(page.getByRole("button", { name: "上传文件" }))
    .toBeVisible();
  const picker = page
    .getByLabelText("选择上传文件")
    .element() as HTMLInputElement;
  const transfer = new DataTransfer();
  transfer.items.add(new File(["png"], "shot.png", { type: "image/png" }));
  picker.files = transfer.files;
  picker.dispatchEvent(new Event("change", { bubbles: true }));
  await expect.poll(() => StagingXHR.sent.length).toBe(1);
  const { xhr } = StagingXHR.sent[0]!;
  xhr.status = 201;
  xhr.response = { path: "/tmp/staging/shot.png" };
  xhr.onload?.();
  await expect
    .element(page.getByRole("list", { name: "待发送附件" }))
    .toHaveTextContent("shot.png");
  await input.fill("修复首页排序");
  await userEvent.keyboard("{Enter}");
  await expect.poll(() => location.pathname).toBe("/s/chataaaa");
  const sent = commands().find((c) => c.path === "/api/sessions");
  expect(sent?.body).toMatchObject({
    view: "terminal",
    attachments: [{ path: "/tmp/staging/shot.png" }],
  });
  expect(sent?.body).not.toHaveProperty("agentConfig");
  expect(sent?.body).not.toHaveProperty("permissionMode");
  expect(localStorage.getItem("webmux.quickStartSide")).toBe("terminal");
});

test("unticking the init instruction applies to that send only", async () => {
  respond(/^POST \/api\/sessions$/, created);
  const { input } = await open();
  const box = page.getByRole("checkbox", { name: "使用初始化指令" });
  await box.click();
  await expect.element(box).not.toBeChecked();
  await input.fill("修复首页排序");
  await userEvent.keyboard("{Enter}");
  await expect.poll(() => location.pathname).toBe("/s/chataaaa");
  const sent = commands().find((c) => c.path === "/api/sessions");
  expect(sent?.body).toMatchObject({ useInitInstruction: false });
});

test("the quick command menu inserts at the caret as written, keeps the draft, and never sends", async () => {
  respond(/^POST \/api\/sessions$/, created);
  const { input } = await open();
  await input.fill("前后");
  const area = input.element() as HTMLTextAreaElement;
  area.setSelectionRange(1, 1);
  await page.getByRole("button", { name: "快捷指令" }).click();
  await page.getByRole("menuitem", { name: /WebMux 工具/ }).click();
  await expect.element(input).toHaveValue("前{{webmux}}后");
  await expect.element(input).toHaveFocus();
  expect(area.selectionStart).toBe("前{{webmux}}".length);
  expect(commands().some((c) => c.path === "/api/sessions")).toBe(false);
  // The placeholder note appears while the text holds one.
  await expect
    .poll(() => document.body.textContent)
    .toContain("会在会话创建后替换为 WebMux 工具说明");
  expect(JSON.parse(localStorage.getItem("webmux.quick-start-draft")!)).toEqual(
    expect.objectContaining({ text: "前{{webmux}}后" }),
  );

  await userEvent.keyboard("{Enter}");
  await expect.poll(() => location.pathname).toBe("/s/chataaaa");
  expect(
    commands().find((c) => c.path === "/api/sessions")?.body,
  ).toMatchObject({ prompt: "前{{webmux}}后", expandPrompt: true });
});

test("a quick command replaces the selection, and managing keeps the draft", async () => {
  const { input } = await open();
  await input.fill("甲乙丙");
  (input.element() as HTMLTextAreaElement).setSelectionRange(1, 2);
  await page.getByRole("button", { name: "快捷指令" }).click();
  await page.getByRole("menuitem", { name: /恢复会话/ }).click();
  await expect.element(input).toHaveValue("甲/resume丙");
  // `enter` does not apply: the message is not sent.
  expect(commands().some((c) => c.path === "/api/sessions")).toBe(false);

  await page.getByRole("button", { name: "快捷指令" }).click();
  await page.getByRole("menuitem", { name: /管理快捷指令/ }).click();
  expect(location.pathname + location.hash).toBe("/settings#quick-commands");
  expect(JSON.parse(localStorage.getItem("webmux.quick-start-draft")!)).toEqual(
    expect.objectContaining({ text: "甲/resume丙" }),
  );
});

test("the quick command menu is off while the session is being created", async () => {
  const pending = Promise.withResolvers<Response>();
  respond(/^POST \/api\/sessions$/, () => pending.promise);
  const { input } = await open();
  await input.fill("修复首页排序");
  await userEvent.keyboard("{Enter}");
  await expect
    .element(page.getByRole("button", { name: "快捷指令" }))
    .toBeDisabled();
  pending.resolve(created());
  await expect.poll(() => location.pathname).toBe("/s/chataaaa");
});

test("the preview expands quick command references and flags unknown ones", async () => {
  respond(/^GET \/api\/repositories$/, () =>
    Response.json([
      repository({ initInstruction: "先 {{恢复会话}}，再 {{没有}}\n{{dir}}" }),
    ]),
  );
  await open();
  await page.getByRole("button", { name: "预览" }).click();
  await expect
    .element(page.getByText(/先 \/resume，再 \{\{没有\}\}/))
    .toBeVisible();
  await expect
    .element(page.getByText(/找不到快捷指令：\{\{没有\}\}/))
    .toBeVisible();
  await expect.element(page.getByText(/创建后替换/)).toBeVisible();
});

test("the control is hidden and false is sent without an instruction", async () => {
  respond(/^GET \/api\/repositories$/, () => Response.json([repository()]));
  respond(/^POST \/api\/sessions$/, created);
  const { input } = await open();
  expect(
    page.getByRole("checkbox", { name: "使用初始化指令" }).elements(),
  ).toEqual([]);
  await input.fill("修复首页排序");
  await userEvent.keyboard("{Enter}");
  await expect.poll(() => location.pathname).toBe("/s/chataaaa");
  const sent = commands().find((c) => c.path === "/api/sessions");
  expect(sent?.body).toMatchObject({ useInitInstruction: false });
});

test("the base branch is searched and picked by name", async () => {
  respond(/^POST \/api\/sessions$/, () =>
    Response.json(
      {
        session: chat({ id: "chataaaa" }),
        worktree: worktree({ id: "otter", kind: "linked" }),
        configWarnings: [],
      },
      { status: 201 },
    ),
  );
  const { input, place } = await open();
  await place.getByRole("button", { name: "基准分支：main" }).click();
  const list = page.getByRole("listbox", { name: "本地分支列表" });
  await expect.poll(() => list.getByRole("option").elements()).toHaveLength(4);
  await page.getByRole("combobox", { name: "基准分支" }).fill("upstream");
  await page
    .getByRole("option", { name: "origin/upstream-fix", exact: true })
    .click();
  await expect
    .element(
      place.getByRole("button", { name: "基准分支：origin/upstream-fix" }),
    )
    .toBeVisible();
  await place
    .getByRole("button", { name: "基准分支：origin/upstream-fix" })
    .click();
  await page.getByRole("combobox", { name: "基准分支" }).fill("feat");
  await page.getByRole("option", { name: "feature", exact: true }).click();
  await expect
    .element(place.getByRole("button", { name: "基准分支：feature" }))
    .toBeVisible();
  await input.fill("修复首页排序");
  await userEvent.keyboard("{Enter}");
  await expect.poll(() => location.pathname).toBe("/s/chataaaa");
  const sent = commands().find((c) => c.path === "/api/sessions");
  expect(sent?.body).toMatchObject({
    newWorktree: { base: "b".repeat(40) },
  });
});

test("choosing an agent reads its options and holds the row until done", async () => {
  const pending = Promise.withResolvers<Response>();
  respond(/^POST \/api\/launch-configs\/\w+\/options$/, (request) =>
    request.path === "/api/launch-configs/cfgcodex/options"
      ? pending.promise
      : Response.json({ error: "agent 已退出" }, { status: 502 }),
  );
  const { input, toolbar } = await open();
  // A failed read keeps the remembered options and offers no retry.
  await expect
    .element(toolbar.getByRole("button", { name: "思考：中" }))
    .toBeEnabled();
  expect(toolbar.getByRole("button", { name: /重新读取/ }).elements()).toEqual(
    [],
  );

  await toolbar.getByRole("button", { name: "Agent：Claude Code" }).click();
  await page.getByRole("menuitemradio", { name: /Codex/ }).click();
  await userEvent.keyboard("{Escape}");
  expect(
    commands().findLast(
      (c) => c.path === "/api/launch-configs/cfgcodex/options",
    )?.body,
  ).toEqual({ repositoryId: "repo1" });
  await expect
    .element(toolbar.getByRole("status"))
    .toHaveTextContent("正在读取配置…");
  await expect
    .element(toolbar.getByRole("button", { name: "Agent：Codex" }))
    .toBeDisabled();
  await page.viewport(390, 844);
  try {
    await expect
      .element(toolbar.getByRole("status"))
      .toHaveTextContent("正在读取配置…");
  } finally {
    await page.viewport(1024, 900);
  }
  await input.fill("修复首页排序");
  await expect
    .element(page.getByRole("button", { name: "发送", exact: true }))
    .toBeDisabled();

  pending.resolve(Response.json([effort]));
  await expect
    .element(toolbar.getByRole("button", { name: "思考：中" }))
    .toBeEnabled();
  await expect
    .element(toolbar.getByRole("button", { name: "Agent：Codex" }))
    .toBeEnabled();
  await expect
    .element(page.getByRole("button", { name: "发送", exact: true }))
    .toBeEnabled();
});

test("a remembered agent that is gone falls back to the first hybrid config", async () => {
  localStorage.setItem(
    "webmux.quickStart",
    JSON.stringify({ repositoryId: "gone", configId: "cfgdeled" }),
  );
  const { toolbar, place } = await open();
  await expect
    .element(toolbar.getByRole("button", { name: "Agent：Claude Code" }))
    .toBeVisible();
  await expect
    .element(place.getByRole("button", { name: "仓库：webmux" }))
    .toBeVisible();
});

test("a failed step keeps the text and offers what was created", async () => {
  respond(/^POST \/api\/sessions$/, () =>
    Response.json(
      {
        error: "claude-agent-acp 在初始化阶段退出（exit 1）",
        step: "session",
        worktree: worktree({
          id: "otter",
          kind: "linked",
          path: "/repos/webmux.worktrees/brisk-otter",
        }),
      },
      { status: 502 },
    ),
  );
  const { input } = await open();
  await input.fill("修复首页排序");
  await page.getByRole("button", { name: "发送", exact: true }).click();
  const alert = page.getByRole("alert");
  await expect
    .element(alert)
    .toMatchTextContent(/启动 agent 失败：claude-agent-acp 在初始化阶段退出/);
  await expect
    .element(alert)
    .toMatchTextContent(
      /已创建 worktree：\/repos\/webmux.worktrees\/brisk-otter/,
    );
  await expect
    .element(alert.getByRole("button", { name: "在此 worktree 新建会话" }))
    .toBeVisible();
  await expect.element(input).toHaveValue("修复首页排序");
  expect(location.pathname).toBe("/new");
});

// The quick start's uploads go to the staging endpoint; record where.
class StagingXHR {
  static sent: { url: string; xhr: StagingXHR }[] = [];
  upload = { onprogress: null as ((event: ProgressEvent) => void) | null };
  status = 0;
  statusText = "";
  response: unknown;
  responseType = "";
  timeout = 0;
  onload: (() => void) | null = null;
  onerror: (() => void) | null = null;
  onabort: (() => void) | null = null;
  ontimeout: (() => void) | null = null;
  url = "";
  open(_method: string, url: string) {
    this.url = url;
  }
  setRequestHeader() {}
  send() {
    StagingXHR.sent.push({ url: this.url, xhr: this });
  }
  abort() {
    this.onabort?.();
  }
}

test("files attached before the session exist go with the first message", async () => {
  StagingXHR.sent = [];
  vi.stubGlobal("XMLHttpRequest", StagingXHR);
  let step = "precheck";
  respond(/^POST \/api\/sessions$/, () =>
    Response.json({ error: "不行", step }, { status: 409 }),
  );
  const { input } = await open();
  const picker = page
    .getByLabelText("选择上传文件")
    .element() as HTMLInputElement;
  const transfer = new DataTransfer();
  transfer.items.add(
    new File(["# 规格"], "spec.md", { type: "text/markdown" }),
  );
  picker.files = transfer.files;
  picker.dispatchEvent(new Event("change", { bubbles: true }));
  await expect.poll(() => StagingXHR.sent.length).toBe(1);
  expect(StagingXHR.sent[0]!.url).toBe("/api/uploads");
  const { xhr } = StagingXHR.sent[0]!;
  xhr.status = 201;
  xhr.response = { path: "/tmp/staging/spec.md" };
  xhr.onload?.();
  const chips = page.getByRole("list", { name: "待发送附件" });
  await expect.element(chips).toHaveTextContent("spec.md");

  // A failure before the first message keeps the attachment for a retry.
  await input.fill("按附件修复");
  await page.getByRole("button", { name: "发送", exact: true }).click();
  await expect.element(page.getByRole("alert")).toBeVisible();
  expect(
    commands().findLast((c) => c.path === "/api/sessions")?.body,
  ).toMatchObject({ attachments: [{ path: "/tmp/staging/spec.md" }] });
  await expect.element(chips).toHaveTextContent("spec.md");

  // Once the first message failed, the files belong to the new session.
  step = "prompt";
  await page.getByRole("button", { name: "发送", exact: true }).click();
  await expect.element(chips).not.toBeInTheDocument();
});

test("without hybrid configs the send stays off and says why", async () => {
  respond(/^GET \/api\/launch-configs$/, () =>
    Response.json([launchConfig({ id: "cfgplain", chat: false })]),
  );
  render(ShellHarness, { drafts: createTerminalDrafts() });
  await expect
    .element(page.getByText(/还没有勾选混合模式的启动配置/))
    .toBeVisible();
  await page.getByRole("textbox", { name: "任务描述" }).fill("修复");
  await expect
    .element(page.getByRole("button", { name: "发送", exact: true }))
    .toBeDisabled();
  await expect
    .element(page.getByRole("link", { name: "新建启动配置" }))
    .toBeVisible();
});

test("narrow screens start compact and expand the composer when focused", async () => {
  await page.viewport(390, 844);
  try {
    render(ShellHarness, { drafts: createTerminalDrafts() });
    const socket = await socketAt("/ws/sessions");
    socket.frame({ type: "snapshot", sessions: [], forwards: [] } as never);
    const input = page.getByRole("textbox", { name: "任务描述" });
    await expect.element(input).toBeVisible();
    await expect.element(input).not.toHaveFocus();

    const place = page.getByRole("toolbar", {
      name: "仓库与分支",
      includeHidden: true,
    });
    const initCheckbox = page.getByRole("checkbox", {
      name: "使用初始化指令",
      includeHidden: true,
    });
    const uploadButton = page.getByRole("button", {
      name: "上传文件",
      includeHidden: true,
    });
    await expect.element(place).not.toBeVisible();
    await expect.element(initCheckbox).not.toBeVisible();
    await expect.element(uploadButton).not.toBeVisible();
    const toolbar = page.getByRole("toolbar", { name: "启动选项" });
    const effort = toolbar.getByRole("button", { name: "思考：中" });
    await effort.click();
    await expect
      .element(page.getByRole("menuitemradio", { name: "中" }))
      .toHaveAttribute("aria-checked", "true");
    await userEvent.keyboard("{Escape}");
    await expect.element(input).not.toHaveFocus();
    await expect
      .element(page.getByRole("button", { name: "发送", exact: true }))
      .toBeVisible();

    await input.click();
    await expect
      .element(place.getByRole("button", { name: "基准分支：main" }))
      .toBeVisible();
    await expect.element(initCheckbox).toBeVisible();
    await expect.element(uploadButton).toBeVisible();

    // Blurring while empty returns to the compact bar.
    (input.element() as HTMLTextAreaElement).blur();
    await expect.element(place).not.toBeVisible();
    await expect.element(initCheckbox).not.toBeVisible();
  } finally {
    await page.viewport(1024, 900);
  }
});
