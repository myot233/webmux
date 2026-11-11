import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { page, userEvent } from "vitest/browser";
import { cleanup, render } from "vitest-browser-svelte/pure";
import NewSessionDialog from "../../src/components/sessions/NewSessionDialog.svelte";
import type { LaunchConfigsState } from "../../src/domains/settings/launch-configs";
import type { LaunchEntry } from "../../src/domains/sessions/new-session-defaults";
import type {
  Repository,
  SessionInfo,
  Worktree,
} from "../../src/contracts/schemas";
import { base, branch } from "./shell-fixture";

const config = {
  id: "abcdefg2",
  name: "Claude 恢复",
  cmd: ["claude", "--resume"],
  envNames: ["API_KEY", "BASE_URL"],
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
const otherConfig = {
  ...config,
  id: "hijklm23",
  name: "Open Code",
  cmd: ["opencode", "--continue", "a b"],
  envNames: [],
  chatOptions: [],
  chatSupport: { supported: true, supportedAgents: [], agent: "opencode" },
};
const mainTree: Worktree = {
  id: "main",
  repositoryId: "repo-1",
  path: "/repos/project",
  kind: "main",
  branch: "main",
  head: "a".repeat(40),
  detached: false,
  locked: false,
  lockReason: "",
  prunable: false,
  available: true,
  dirty: false,
  hasUntracked: false,
  hasIgnored: false,
  error: "",
  upstream: null,
  ahead: null,
  behind: null,
  upstreamGone: false,
  lastCommit: null,
  changes: { staged: 0, unstaged: 0, untracked: 0, conflicted: 0 },
  operation: null,
};
const repository = {
  id: "repo-1",
  name: "project",
  commonDir: "/repos/project/.git",
  anchorPath: "/repos/project",
  availability: "available",
  error: "",
  checkedAt: "2026-10-23T00:00:00Z",
  lastFetchedAt: null,
  initInstruction: "",
  worktrees: [mainTree],
} satisfies Repository;
const secondRepository: Repository = {
  ...repository,
  id: "repo-2",
  name: "other project",
  commonDir: "/repos/other/.git",
  anchorPath: "/repos/other",
  worktrees: [
    {
      ...mainTree,
      id: "other-main",
      repositoryId: "repo-2",
      path: "/repos/other",
    },
  ],
};
const createdTree: Worktree = {
  ...mainTree,
  id: "created",
  kind: "linked",
  branch: "feature/fresh",
  path: "/repos/project.worktrees/fresh",
};
const session: SessionInfo = {
  id: "sessio23",
  generation: "0123456789abcdef",
  title: "",
  cwd: "/work",
  cmd: ["claude", "--resume"],
  status: "running",
  exitCode: null,
  listeningPorts: [],
  state: "active",
  views: ["terminal"],
  view: "terminal",
  transition: null,
  agent: null,
  preview: "",
  createdAt: "2026-10-23T00:00:00Z",
  lastActiveAt: "2026-10-23T00:00:00Z",
  cols: 80,
  rows: 24,
  clients: 0,
  workspace: null,
  attention: null,
  reportedCwd: null,
};

let created: unknown[] = [];
let worktrees: unknown[] = [];
let removed: unknown[] = [];
// The 409 body a create starting on the chat view answers, when set.
let acpConflict: unknown = null;
// Session creations answered with this 400 error, when set.
let sessionFailures = 0;
const origin = location.pathname + location.search;

beforeEach(() => {
  localStorage.clear();
  created = [];
  worktrees = [];
  removed = [];
  acpConflict = null;
  sessionFailures = 0;
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>(async (input, init) => {
      const path = new URL(String(input), location.href).pathname;
      if (path === "/api/sessions" && init?.method === "POST") {
        const body = JSON.parse(String(init.body)) as {
          view?: string;
          newWorktree?: unknown;
        };
        created.push(body);
        // A new worktree is made first and is reported by every answer.
        if (body.newWorktree) worktrees.push(body.newWorktree);
        const tree = body.newWorktree ? { worktree: createdTree } : {};
        if (acpConflict && body.view === "chat")
          return Response.json(acpConflict, { status: 409 });
        if (sessionFailures > 0) {
          sessionFailures -= 1;
          return Response.json(
            { error: "启动失败", step: "session", ...tree },
            { status: 400 },
          );
        }
        return Response.json(
          { session, ...tree, configWarnings: [] },
          { status: 201 },
        );
      }
      if (
        path === "/api/repositories/repo-1/worktrees/created/removal-preview"
      ) {
        return Response.json({
          confirmationId: "confirm1",
          expiresAt: "2099-01-01T00:00:00Z",
          worktree: createdTree,
          sessions: [],
          archived: [],
          commit: "c".repeat(40),
          push: { state: "noRemote", branches: [], more: 0 },
          blockers: [],
          warnings: [],
          forceAllowed: true,
        });
      }
      if (path === "/api/repositories/repo-1/worktrees/created/remove") {
        removed.push(JSON.parse(String(init?.body)));
        return new Response(null, { status: 204 });
      }
      if (path === "/api/repositories/repo-1/worktrees") {
        worktrees.push(JSON.parse(String(init?.body)));
        return Response.json(createdTree, { status: 201 });
      }
      if (path === "/api/repositories/repo-1/branches") {
        return Response.json({
          branches: [branch("main", "a".repeat(40), ["/repos/project"])],
          bases: [base("refs/heads/main", "a".repeat(40), "initial")],
          defaultBase: "a".repeat(40),
          defaultBaseSubject: "initial",
          defaultBranch: "main",
          defaultParent: "/repos/project.worktrees",
        });
      }
      if (path === "/api/repositories/repo-1/commits") {
        return Response.json({
          commits: [
            {
              hash: "a".repeat(40),
              subject: "initial",
              author: "dev",
              committedAt: "2026-10-14T00:00:00Z",
            },
          ],
        });
      }
      if (path === "/api/repositories/repo-1/branches/refresh") {
        return Response.json({
          branch: "main",
          base: "a".repeat(40),
          baseSubject: "initial",
          baseSource: "remote",
          upstream: "origin/main",
          upstreamHead: "a".repeat(40),
          fetchWarning: "",
        });
      }
      // Directory suggestions are irrelevant to these launch-choice tests.
      return Response.json({ error: "not found" }, { status: 404 });
    }),
  );
});

afterEach(async () => {
  try {
    await cleanup();
    await userEvent.cleanup();
  } finally {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
    localStorage.clear();
    history.replaceState(null, "", origin);
  }
});

function open(
  launchConfigs: LaunchConfigsState,
  target: {
    repositories?: Repository[];
    entry?: LaunchEntry;
    selectedRepositoryId?: string;
    intent?: "session" | "worktree";
    split?: boolean;
    oncreated?: (
      session: SessionInfo,
      direction: "horizontal" | "vertical",
    ) => void;
  } = {},
) {
  return render(NewSessionDialog, {
    launchConfigs,
    defaultCwd: "/work",
    ...target,
    onchange: () => {},
    onworktreecreated: () => {},
    onclose: () => {},
  });
}
const submit = () =>
  page.getByRole("button", { name: "创建并进入 →", exact: true });
const launchField = () =>
  page.getByRole("button", { name: "运行", exact: true });
const locationField = () =>
  page.getByRole("button", { name: "位置", exact: true });
// Option names concatenate their title, command or path and details.
const option = (name: string) =>
  page.getByRole("option", {
    name: new RegExp(`^${name.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}`),
  });
const command = () => page.getByLabelText("自定义启动命令");
const directory = () => page.getByLabelText("工作目录", { exact: true });
const summary = () => page.getByText(/^(将在|将创建|请)/);

async function chooseLaunch(name: string) {
  await launchField().click();
  await option(name).click();
}
async function chooseLocation(name: string) {
  await locationField().click();
  await option(name).click();
}
async function nameSession(value: string) {
  await page.getByRole("button", { name: "高级：会话名称" }).click();
  await page.getByLabelText("会话名称", { exact: false }).fill(value);
}

test("starts a saved config with only configId and displays its command and environment count", async () => {
  open({ status: "ready", configs: [config, otherConfig] });
  await launchField().click();
  const choice = option(config.name);
  await expect.element(choice).toMatchTextContent(/claude --resume/);
  await expect.element(choice).toMatchTextContent(/2 个环境变量/);
  expect(
    choice.element().querySelector('[data-launch-icon="claudecode"] svg'),
  ).not.toBeNull();
  await expect
    .element(option(otherConfig.name))
    .toMatchTextContent(/opencode --continue 'a b'/);
  await choice.click();
  await expect.element(launchField()).toMatchTextContent(/Claude 恢复/);
  await expect.element(launchField()).toMatchTextContent(/claude --resume/);
  await expect.element(launchField()).toMatchTextContent(/2 个环境变量/);
  expect(
    launchField()
      .element()
      .querySelector('[data-launch-icon="claudecode"] svg'),
  ).not.toBeNull();
  await expect
    .element(summary())
    .toHaveTextContent("将在 /work 运行 Claude 恢复");
  await submit().click();
  await vi.waitFor(() => expect(created).toHaveLength(1));
  expect(created[0]).toEqual({ cwd: "/work", configId: "abcdefg2", name: "" });
  await vi.waitFor(() =>
    expect(localStorage.getItem("webmux.lastLaunch")).toBe("config:abcdefg2"),
  );
});

test("a hybrid config offers and remembers where the session starts", async () => {
  const hybridConfig = {
    ...otherConfig,
    id: "hybrid22",
    name: "omp 混合",
    cmd: ["omp"],
    chat: true,
  };
  open({ status: "ready", configs: [config, hybridConfig] });
  const sides = page.getByRole("radiogroup", { name: "起始端", exact: true });
  await expect.element(launchField()).toMatchTextContent(/Claude 恢复/);
  expect(sides.elements()).toHaveLength(0);
  await chooseLaunch(hybridConfig.name);
  await expect
    .element(sides.getByRole("radio", { name: "从终端开始", exact: true }))
    .toHaveAttribute("aria-checked", "true");
  await sides.getByRole("radio", { name: "从聊天开始", exact: true }).click();
  await submit().click();
  await vi.waitFor(() => expect(created).toHaveLength(1));
  expect(created[0]).toEqual({
    cwd: "/work",
    configId: "hybrid22",
    name: "",
    view: "chat",
  });
  await vi.waitFor(() =>
    expect(localStorage.getItem("webmux.hybridStartSide")).toBe("chat"),
  );
});

test("the last launch choice and start side are filled in on the next open", async () => {
  const hybridConfig = { ...otherConfig, chat: true };
  localStorage.setItem("webmux.lastLaunch", `config:${hybridConfig.id}`);
  localStorage.setItem("webmux.hybridStartSide", "acp");
  open({ status: "ready", configs: [config, hybridConfig] });
  await expect.element(launchField()).toMatchTextContent(/Open Code/);
  await expect
    .element(page.getByRole("radio", { name: "从聊天开始", exact: true }))
    .toHaveAttribute("aria-checked", "true");
  await submit().click();
  await vi.waitFor(() => expect(created).toHaveLength(1));
  expect(created[0]).toEqual({
    cwd: "/work",
    configId: hybridConfig.id,
    name: "",
    view: "chat",
  });
});

test("each hybrid agent remembers its own start side", async () => {
  const first = { ...otherConfig, chat: true };
  const second = { ...config, id: "hybrid33", name: "omp 混合", chat: true };
  localStorage.setItem("webmux.hybridStartSide", "tui");
  localStorage.setItem(`webmux.hybridStartSide:${first.id}`, "acp");
  localStorage.setItem("webmux.lastLaunch", `config:${first.id}`);
  open({ status: "ready", configs: [first, second] });
  const chat = page.getByRole("radio", { name: "从聊天开始", exact: true });
  await expect.element(chat).toHaveAttribute("aria-checked", "true");
  await chooseLaunch(second.name);
  await expect.element(chat).toHaveAttribute("aria-checked", "false");
  await chooseLaunch(first.name);
  await expect.element(chat).toHaveAttribute("aria-checked", "true");
});

test("a missing chat connector keeps the dialog and can start on the terminal", async () => {
  const connector = {
    executable: "codex-acp",
    title: "Codex ACP 适配器",
    installCommand: "npm install -g @agentclientprotocol/codex-acp",
    docsUrl: "https://github.com/agentclientprotocol/codex-acp",
  };
  const codexConfig = {
    ...otherConfig,
    id: "codex222",
    name: "Codex 混合",
    cmd: ["codex"],
    chat: true,
    chatSupport: {
      supported: true,
      supportedAgents: [],
      agent: "codex",
      connector,
    },
  };
  acpConflict = {
    error: "未安装 Codex ACP 适配器（codex-acp）",
    reason: "connector_missing",
    connector,
  };
  open({ status: "ready", configs: [codexConfig] });
  await expect
    .element(page.getByText("聊天端需要另外安装 codex-acp", { exact: false }))
    .toBeVisible();
  const sides = page.getByRole("radiogroup", { name: "起始端", exact: true });
  await sides.getByRole("radio", { name: "从聊天开始", exact: true }).click();
  await nameSession("修复排序");
  await submit().click();
  await expect
    .element(page.getByText("未安装 Codex ACP 适配器（codex-acp）"))
    .toBeVisible();
  await expect.element(page.getByText(connector.installCommand)).toBeVisible();
  await expect
    .element(page.getByRole("link", { name: "安装说明" }))
    .toHaveAttribute("href", connector.docsUrl);
  await expect
    .element(page.getByLabelText("会话名称", { exact: false }))
    .toHaveValue("修复排序");
  expect(created).toHaveLength(1);

  await page.getByRole("button", { name: "改为从终端开始" }).click();
  await vi.waitFor(() => expect(created).toHaveLength(2));
  expect(created[1]).toEqual({
    cwd: "/work",
    configId: "codex222",
    name: "修复排序",
    view: "terminal",
  });
});

test("defaults to the first saved config when no launch choice was remembered", async () => {
  open({ status: "ready", configs: [config, otherConfig] });
  await expect.element(launchField()).toMatchTextContent(/Claude 恢复/);
  await expect.element(submit()).toBeEnabled();
  expect(command().elements()).toHaveLength(0);
});

test("a remembered custom command preserves argument boundaries even when configs exist", async () => {
  localStorage.setItem("webmux.lastLaunch", "custom");
  localStorage.setItem("webmux.customCommand", `htop 'a b' '' '$HOME'`);
  open({ status: "ready", configs: [config] });
  await expect.element(launchField()).toMatchTextContent(/自定义命令/);
  await expect.element(command()).toHaveValue(`htop 'a b' '' '$HOME'`);
  await submit().click();
  await vi.waitFor(() => expect(created).toHaveLength(1));
  expect(created[0]).toEqual({
    cwd: "/work",
    cmd: ["htop", "a b", "", "$HOME"],
    name: "",
  });
  await vi.waitFor(() =>
    expect(localStorage.getItem("webmux.lastLaunch")).toBe("custom"),
  );
});

test("switching sources keeps the config and the typed command but submits only the current one", async () => {
  open({ status: "ready", configs: [config, otherConfig] });
  await chooseLaunch(otherConfig.name);
  await chooseLaunch("自定义命令…");
  await expect.element(command()).toHaveValue("");
  await expect.element(submit()).toBeDisabled();
  await command().fill("echo retained");
  await chooseLaunch(otherConfig.name);
  await expect.element(command()).not.toBeInTheDocument();
  await expect.element(submit()).toBeEnabled();
  await chooseLaunch("自定义命令…");
  await expect.element(command()).toHaveValue("echo retained");
  await submit().click();
  await vi.waitFor(() => expect(created).toHaveLength(1));
  expect(created[0]).toEqual({
    cwd: "/work",
    cmd: ["echo", "retained"],
    name: "",
  });
});

test("the keyboard picks a launch config and submits its exact ID", async () => {
  open({ status: "ready", configs: [config, otherConfig] });
  launchField().element().focus();
  await userEvent.keyboard("{Enter}");
  await expect.element(option(otherConfig.name)).toBeVisible();
  await userEvent.keyboard("{ArrowDown}{Enter}");
  await expect.element(option(otherConfig.name)).not.toBeInTheDocument();
  await expect.element(launchField()).toMatchTextContent(/Open Code/);
  submit().element().focus();
  await userEvent.keyboard("{Enter}");
  await vi.waitFor(() => expect(created).toHaveLength(1));
  expect(created[0]).toEqual({
    cwd: "/work",
    configId: otherConfig.id,
    name: "",
  });
});

test("Escape closes only the open picker and keeps the dialog and its input", async () => {
  const onclose = vi.fn();
  render(NewSessionDialog, {
    launchConfigs: { status: "ready", configs: [config] },
    defaultCwd: "/work",
    onchange: () => {},
    onworktreecreated: () => {},
    onclose,
  });
  await directory().fill("/work/kept");
  await locationField().click();
  await expect.element(option("输入或浏览目录…")).toBeVisible();
  await userEvent.keyboard("{Escape}");
  await expect.element(option("输入或浏览目录…")).not.toBeInTheDocument();
  await expect.element(page.getByRole("dialog")).toBeVisible();
  await expect.element(directory()).toHaveValue("/work/kept");
  expect(onclose).not.toHaveBeenCalled();
});

test("a remembered config that was deleted is not replaced and blocks submission until a choice", async () => {
  localStorage.setItem("webmux.lastLaunch", "config:zzzzzzz2");
  localStorage.setItem("webmux.customCommand", "echo retained");
  open({ status: "ready", configs: [config] });
  await expect.element(launchField()).toMatchTextContent("选择启动方式");
  await expect.element(submit()).toBeDisabled();
  await expect.element(summary()).toHaveTextContent("请选择启动方式");
  await chooseLaunch(config.name);
  await expect.element(submit()).toBeEnabled();
});

test("deleting the selected config while the dialog is open does not select its replacement", async () => {
  const view = await open({ status: "ready", configs: [config, otherConfig] });
  await expect.element(launchField()).toMatchTextContent(/Claude 恢复/);
  await view.rerender({
    launchConfigs: { status: "ready", configs: [otherConfig] },
  });
  await expect.element(launchField()).toMatchTextContent("选择启动方式");
  await expect.element(submit()).toBeDisabled();
  expect(created).toEqual([]);
});

test("an empty config list links to the settings card and starts on a custom command", async () => {
  open({ status: "ready", configs: [] });
  await expect.element(command()).toBeVisible();
  await expect.element(submit()).toBeDisabled();
  await launchField().click();
  await expect
    .element(page.getByRole("link", { name: "前往设置创建启动配置" }))
    .toHaveAttribute("href", "/settings#launch-configs");
});

test("an unavailable store explains why and still allows a custom command", async () => {
  localStorage.setItem("webmux.lastLaunch", "config:abcdefg2");
  open({
    status: "unavailable",
    error: "启动配置不可用: 文件损坏（/state/launch-configs.json）",
  });
  await expect.element(submit()).toBeDisabled();
  await launchField().click();
  await expect
    .element(page.getByText(/启动配置暂不可用：.*launch-configs\.json/))
    .toBeVisible();
  await option("自定义命令…").click();
  await command().fill("/bin/bash --noprofile --norc -i");
  await submit().click();
  await vi.waitFor(() => expect(created).toHaveLength(1));
  expect(created[0]).toEqual({
    cwd: "/work",
    cmd: ["/bin/bash", "--noprofile", "--norc", "-i"],
    name: "",
  });
});

test("loading configs permits an explicit custom command that a later load does not override", async () => {
  localStorage.setItem("webmux.lastLaunch", "config:abcdefg2");
  const view = await open({ status: "loading" });
  await expect.element(launchField()).toMatchTextContent("正在读取启动配置…");
  await expect.element(submit()).toBeDisabled();
  await chooseLaunch("自定义命令…");
  await command().fill("echo ready");
  await view.rerender({
    launchConfigs: { status: "ready", configs: [config] },
  });
  await expect.element(command()).toHaveValue("echo ready");
  await submit().click();
  await vi.waitFor(() => expect(created).toHaveLength(1));
  expect(created[0]).toEqual({
    cwd: "/work",
    cmd: ["echo", "ready"],
    name: "",
  });
});

test("a remembered config applies once loading completes", async () => {
  localStorage.setItem("webmux.lastLaunch", `config:${otherConfig.id}`);
  const view = await open({ status: "loading" });
  await view.rerender({
    launchConfigs: { status: "ready", configs: [config, otherConfig] },
  });
  await expect.element(launchField()).toMatchTextContent(/Open Code/);
  await expect.element(submit()).toBeEnabled();
});

test("empty or malformed custom argv cannot create a session despite a retained valid config", async () => {
  open({ status: "ready", configs: [config] });
  await chooseLaunch("自定义命令…");
  for (const value of ["", "   ", "''", "echo 'unfinished"]) {
    await command().fill(value);
    await expect.element(submit()).toBeDisabled();
  }
  expect(created).toEqual([]);
});

test("a failed creation keeps every input and the previous remembered launch", async () => {
  localStorage.setItem("webmux.lastLaunch", `config:${config.id}`);
  sessionFailures = 1;
  open({ status: "ready", configs: [config, otherConfig] });
  await chooseLaunch("自定义命令…");
  await command().fill("echo next");
  await directory().fill("/work/next");
  await nameSession("下一次");
  await submit().click();
  await expect.element(page.getByRole("alert")).toHaveTextContent("启动失败");
  await expect.element(command()).toHaveValue("echo next");
  await expect.element(directory()).toHaveValue("/work/next");
  await expect
    .element(page.getByLabelText("会话名称", { exact: false }))
    .toHaveValue("下一次");
  expect(localStorage.getItem("webmux.lastLaunch")).toBe(`config:${config.id}`);
  expect(localStorage.getItem("webmux.customCommand")).toBeNull();
});

test("the home page's selected repository wins over a recent directory of another project", async () => {
  localStorage.setItem("webmux.recentDirs", JSON.stringify(["/other-project"]));
  open(
    { status: "ready", configs: [config] },
    {
      repositories: [secondRepository, repository],
      selectedRepositoryId: "repo-1",
    },
  );
  await expect
    .element(locationField())
    .toMatchTextContent(/project · 主工作区 · main/);
  await locationField().click();
  await expect
    .element(page.getByRole("option").first())
    .toMatchTextContent(/^主工作区 · main \/repos\/project$/);
  await userEvent.keyboard("{Escape}");
  await submit().click();
  await vi.waitFor(() => expect(created).toHaveLength(1));
  expect(created[0]).toEqual({
    workspace: { repositoryId: "repo-1", worktreeId: "main" },
    configId: config.id,
    name: "",
  });
});

test("a contextual entry overrides the selected repository", async () => {
  open(
    { status: "ready", configs: [config] },
    {
      repositories: [repository, secondRepository],
      selectedRepositoryId: "repo-1",
      entry: {
        kind: "worktree",
        repositoryId: "repo-2",
        worktreeId: "other-main",
      },
    },
  );
  await expect
    .element(locationField())
    .toMatchTextContent(/other project · 主工作区 · main/);
  await submit().click();
  await vi.waitFor(() => expect(created).toHaveLength(1));
  expect(created[0]).toMatchObject({
    workspace: { repositoryId: "repo-2", worktreeId: "other-main" },
  });
});

const featureTree: Worktree = {
  ...mainTree,
  id: "feature",
  kind: "linked",
  branch: "feature",
  path: "/repos/project-feature",
};
const twoTrees: Repository = {
  ...repository,
  worktrees: [mainTree, featureTree],
};

test("a repository scope lists only its worktrees and a new one", async () => {
  localStorage.setItem("webmux.recentDirs", JSON.stringify(["/recent"]));
  open(
    { status: "ready", configs: [config] },
    {
      repositories: [secondRepository, twoTrees],
      scope: { repositoryId: "repo-1" },
    },
  );
  await locationField().click();
  await expect.poll(() => page.getByRole("option").elements().length).toBe(3);
  await expect.element(page.getByText("新建工作树…")).toBeVisible();
  await expect.element(page.getByText("other project")).not.toBeInTheDocument();
  await expect
    .element(page.getByText("输入或浏览目录…"))
    .not.toBeInTheDocument();
  await expect.element(page.getByText("最近目录")).not.toBeInTheDocument();
});

test("a worktree scope offers that worktree or a new one from its branch", async () => {
  open(
    { status: "ready", configs: [config] },
    {
      repositories: [secondRepository, twoTrees],
      scope: { repositoryId: "repo-1", worktreeId: "feature" },
    },
  );
  await expect.element(locationField()).toMatchTextContent(/project · feature/);
  await locationField().click();
  await expect.poll(() => page.getByRole("option").elements().length).toBe(2);
  await expect.element(page.getByText("从当前工作树分支新建…")).toBeVisible();
  await expect.element(page.getByText("主工作区")).not.toBeInTheDocument();
  await userEvent.keyboard("{Escape}");
  await submit().click();
  await vi.waitFor(() => expect(created).toHaveLength(1));
  expect(created[0]).toMatchObject({
    workspace: { repositoryId: "repo-1", worktreeId: "feature" },
  });
});

test("a selected repository without usable targets stays selected and asks for a choice", async () => {
  open(
    { status: "ready", configs: [config] },
    {
      repositories: [
        {
          ...repository,
          worktrees: [
            { ...mainTree, available: false },
            { ...mainTree, id: "bare", kind: "bare", branch: "" },
          ],
        },
      ],
      selectedRepositoryId: "repo-1",
    },
  );
  await expect.element(locationField()).toMatchTextContent(/选择工作树/);
  await expect.element(locationField()).toMatchTextContent(/project/);
  await expect.element(submit()).toBeDisabled();
  await locationField().click();
  await expect
    .element(option("主工作区 · main"))
    .toHaveAttribute("aria-disabled", "true");
  await expect.element(option("aaaa")).toHaveAttribute("aria-disabled", "true");
  expect(created).toEqual([]);
});

test("the location picker switches between recent directories, typed directories and worktrees", async () => {
  localStorage.setItem(
    "webmux.recentDirs",
    JSON.stringify(["/work/alpha", "/work/beta"]),
  );
  open(
    { status: "ready", configs: [config] },
    { repositories: [repository, secondRepository] },
  );
  await expect.element(directory()).toHaveValue("/work/alpha");
  await chooseLocation("beta");
  await expect.element(directory()).toHaveValue("/work/beta");
  await chooseLocation("主工作区 · main /repos/other");
  await expect.element(directory()).not.toBeInTheDocument();
  await expect.element(locationField()).toMatchTextContent(/\/repos\/other/);
  await chooseLocation("输入或浏览目录…");
  await expect.element(directory()).toHaveFocus();
  await directory().fill("/work/typed");
  await submit().click();
  await vi.waitFor(() => expect(created).toHaveLength(1));
  expect(created[0]).toEqual({
    cwd: "/work/typed",
    configId: config.id,
    name: "",
  });
});

test("a linked worktree can be deleted from the location picker and the main worktree is selected instead", async () => {
  const onchange = vi.fn();
  const onworktreeremoved = vi.fn();
  render(NewSessionDialog, {
    launchConfigs: { status: "ready", configs: [config] },
    defaultCwd: "/work",
    repositories: [{ ...repository, worktrees: [mainTree, createdTree] }],
    entry: {
      kind: "worktree",
      repositoryId: "repo-1",
      worktreeId: createdTree.id,
    },
    onchange,
    onworktreeremoved,
    onworktreecreated: () => {},
    onclose: () => {},
  });
  await expect.element(locationField()).toMatchTextContent(/fresh/);
  await locationField().click();
  await expect
    .element(page.getByRole("button", { name: /^删除工作树 main/ }))
    .not.toBeInTheDocument();
  await page
    .getByRole("button", { name: /^删除工作树 feature\/fresh/ })
    .click();
  await page.getByRole("button", { name: "确认安全删除" }).click();
  await vi.waitFor(() => expect(removed).toHaveLength(1));
  expect(removed[0]).toEqual({ confirmationId: "confirm1", mode: "safe" });
  expect(onchange).toHaveBeenCalled();
  expect(onworktreeremoved).toHaveBeenCalledWith(createdTree);
  await expect.element(locationField()).toMatchTextContent(/\/repos\/project/);
});

test("without repositories only directories are offered", async () => {
  open({ status: "ready", configs: [config] });
  await locationField().click();
  await expect.element(option("输入或浏览目录…")).toBeVisible();
  expect(option("新建工作树…").elements()).toHaveLength(0);
});

test("a new worktree created before a failed start is reused on retry", async () => {
  sessionFailures = 1;
  open(
    { status: "ready", configs: [config] },
    { repositories: [repository], selectedRepositoryId: "repo-1" },
  );
  await chooseLocation("新建工作树…");
  const branch = page.getByLabelText("新分支名称", { exact: true });
  await expect.element(branch).not.toHaveValue("");
  await branch.fill("feature/fresh");
  await expect
    .element(locationField())
    .toMatchTextContent(/feature\/fresh ← main/);
  await page.getByRole("button", { name: "高级：起始提交与存放目录" }).click();
  await expect
    .element(page.getByRole("combobox", { name: "起始提交", exact: true }))
    .toMatchTextContent(/initial/);
  await expect
    .element(page.getByLabelText("新工作树完整路径"))
    .toHaveValue("/repos/project.worktrees/fresh");
  await submit().click();
  await expect.element(page.getByRole("alert")).toHaveTextContent("启动失败");
  expect(worktrees).toEqual([
    {
      repositoryId: "repo-1",
      mode: "new",
      branch: "feature/fresh",
      base: "a".repeat(40),
      path: "/repos/project.worktrees/fresh",
    },
  ]);
  await expect.element(locationField()).toMatchTextContent(/feature\/fresh/);
  await expect
    .element(page.getByText("工作树已创建。", { exact: false }))
    .toBeVisible();
  await submit().click();
  await vi.waitFor(() => expect(created).toHaveLength(2));
  expect(worktrees).toHaveLength(1);
  expect(created[1]).toEqual({
    workspace: { repositoryId: "repo-1", worktreeId: "created" },
    configId: config.id,
    name: "",
  });
});

test("new worktree fields survive switching location away and back", async () => {
  open(
    { status: "ready", configs: [config] },
    { repositories: [repository], selectedRepositoryId: "repo-1" },
  );
  await chooseLocation("新建工作树…");
  const branch = page.getByLabelText("新分支名称", { exact: true });
  await expect.element(branch).not.toHaveValue("");
  await branch.fill("feature/kept");
  await page.getByRole("button", { name: "高级：起始提交与存放目录" }).click();
  const path = page.getByLabelText("新工作树完整路径");
  await path.fill("/elsewhere/kept");
  await chooseLocation("主工作区 · main /repos/project");
  await expect.element(branch).not.toBeVisible();
  await chooseLocation("新建工作树…");
  await expect.element(branch).toHaveValue("feature/kept");
  await expect.element(path).toHaveValue("/elsewhere/kept");
  await expect
    .element(page.getByRole("button", { name: "起始分支", exact: true }))
    .toMatchTextContent(/main/);
  await expect
    .element(page.getByRole("combobox", { name: "起始提交", exact: true }))
    .toMatchTextContent(/initial/);
  await expect
    .element(locationField())
    .toMatchTextContent(/feature\/kept ← main/);
  await submit().click();
  await vi.waitFor(() => expect(created).toHaveLength(1));
  expect(worktrees).toEqual([
    {
      repositoryId: "repo-1",
      mode: "new",
      branch: "feature/kept",
      base: "a".repeat(40),
      path: "/elsewhere/kept",
    },
  ]);
});

test("the mobile drawer expands pickers in place without horizontal overflow", async () => {
  await page.viewport(390, 844);
  try {
    const longName = "很长的启动配置名称".repeat(6);
    open({
      status: "ready",
      configs: [
        { ...config, name: longName, cmd: ["bash", "-lc", "x".repeat(300)] },
      ],
    });
    const trigger = launchField();
    await expect.element(trigger).toMatchTextContent(longName);
    await trigger.click();
    const choice = option(longName);
    const dialog = page.getByRole("dialog");
    await expect.element(choice).toBeVisible();
    expect(dialog.element().contains(choice.element())).toBe(true);
    for (const element of [
      trigger.element(),
      choice.element(),
      dialog.element(),
    ]) {
      const bounds = element.getBoundingClientRect();
      expect(bounds.left).toBeGreaterThanOrEqual(0);
      expect(bounds.right).toBeLessThanOrEqual(window.innerWidth);
      expect(element.scrollWidth).toBeLessThanOrEqual(element.clientWidth);
    }
    expect(document.documentElement.scrollWidth).toBeLessThanOrEqual(
      window.innerWidth,
    );
    await choice.click();
    await expect.element(choice).not.toBeInTheDocument();
    await expect.element(trigger).toMatchTextContent(longName);
    // Escape from the trigger closes only the expanded panel.
    await trigger.click();
    await expect.element(choice).toBeVisible();
    await userEvent.keyboard("{Escape}");
    await expect.element(choice).not.toBeInTheDocument();
    await expect.element(dialog).toBeVisible();
    await expect.element(trigger).toHaveFocus();
  } finally {
    await page.viewport(1024, 900);
  }
});

test.each(["double-click", "enter"] as const)(
  "%s submission creates only one session while the response is pending",
  async (interaction) => {
    const server = globalThis.fetch;
    const response = Promise.withResolvers<Response>();
    vi.stubGlobal(
      "fetch",
      vi.fn<typeof fetch>((input, init) => {
        const path = new URL(String(input), location.href).pathname;
        if (path === "/api/sessions" && init?.method === "POST") {
          created.push(JSON.parse(String(init.body)));
          return response.promise;
        }
        return server(input, init);
      }),
    );
    open({ status: "ready", configs: [config] });
    await expect.element(submit()).toBeEnabled();
    if (interaction === "double-click")
      await page.elementLocator(submit().element()).dblClick();
    else {
      submit().element().focus();
      await userEvent.keyboard("{Enter}{Enter}{Enter}");
    }
    await vi.waitFor(() => expect(created).toHaveLength(1));
    expect(created).toHaveLength(1);
    response.resolve(
      Response.json({ session, configWarnings: [] }, { status: 201 }),
    );
    await vi.waitFor(() => expect(location.pathname).toBe("/s/sessio23"));
  },
);

test("split creation starts in the pane's directory and returns the chosen direction", async () => {
  localStorage.setItem("webmux.recentDirs", JSON.stringify(["/other-project"]));
  const oncreated = vi.fn();
  open(
    { status: "ready", configs: [config] },
    {
      entry: { kind: "directory", cwd: "/project/subdir", workspace: null },
      split: true,
      oncreated,
    },
  );
  await expect.element(directory()).toHaveValue("/project/subdir");
  await page.getByRole("radio", { name: "上下分屏", exact: true }).click();
  await page
    .getByRole("button", { name: "创建并加入分屏 →", exact: true })
    .click();
  await vi.waitFor(() =>
    expect(oncreated).toHaveBeenCalledWith(session, "vertical"),
  );
  expect(created[0]).toMatchObject({
    cwd: "/project/subdir",
    configId: config.id,
  });
  expect(location.pathname + location.search).toBe(origin);
});

test("worktree management hides launch controls until starting a session is explicitly selected", async () => {
  open(
    { status: "unavailable", error: "offline" },
    {
      repositories: [repository],
      entry: { kind: "new-worktree", repositoryId: "repo-1" },
      intent: "worktree",
    },
  );
  const checkbox = page.getByRole("checkbox", {
    name: "创建后启动会话",
    exact: true,
  });
  await expect.element(checkbox).not.toBeChecked();
  await expect.element(launchField()).not.toBeInTheDocument();
  await page
    .getByLabelText("新分支名称", { exact: true })
    .fill("feature/separate");
  const create = page.getByRole("button", { name: "创建工作树", exact: true });
  await expect.element(create).toBeEnabled();
  await checkbox.click();
  await expect.element(launchField()).toBeVisible();
  await expect.element(submit()).toBeDisabled();
  await checkbox.click();
  await create.click();
  await vi.waitFor(() => expect(worktrees).toHaveLength(1));
  expect(created).toEqual([]);
});

test("creating only a worktree cannot submit another kind of location", async () => {
  const onclose = vi.fn();
  render(NewSessionDialog, {
    launchConfigs: { status: "ready", configs: [config] },
    defaultCwd: "/work",
    repositories: [repository],
    entry: { kind: "new-worktree", repositoryId: "repo-1" },
    intent: "worktree",
    onchange: () => {},
    onworktreecreated: () => {},
    onclose,
  });
  const branch = page.getByLabelText("新分支名称", { exact: true });
  await expect.element(branch).not.toHaveValue("");
  const create = page.getByRole("button", { name: "创建工作树", exact: true });
  await expect.element(create).toBeEnabled();
  await chooseLocation("主工作区 · main /repos/project");
  await expect.element(create).toBeDisabled();
  await expect
    .element(summary())
    .toHaveTextContent("请将位置选为新建工作树，或勾选创建后启动会话");
  await chooseLocation("输入或浏览目录…");
  await directory().fill("/work/typed");
  await userEvent.keyboard("{Enter}");
  await expect.element(create).toBeDisabled();
  expect(onclose).not.toHaveBeenCalled();
  expect(worktrees).toEqual([]);
  await chooseLocation("新建工作树…");
  await expect.element(create).toBeEnabled();
  await create.click();
  await vi.waitFor(() => expect(onclose).toHaveBeenCalledOnce());
  expect(worktrees).toHaveLength(1);
  expect(created).toEqual([]);
});

test.each(["/repos/project/subdir", "/other-project"])(
  "a split keeps the pane's workspace only for its own directory: %s",
  async (cwd) => {
    const oncreated = vi.fn();
    open(
      { status: "ready", configs: [config] },
      {
        entry: {
          kind: "directory",
          cwd: "/repos/project/subdir",
          workspace: {
            repositoryId: repository.id,
            worktreeId: "main",
            worktreePath: repository.anchorPath,
          },
        },
        split: true,
        oncreated,
      },
    );
    await directory().fill(cwd);
    await page
      .getByRole("button", { name: "创建并加入分屏 →", exact: true })
      .click();
    await vi.waitFor(() => expect(created).toHaveLength(1));
    expect(created[0]).toEqual({
      cwd,
      configId: config.id,
      name: "",
      ...(cwd === "/repos/project/subdir"
        ? { workspace: { repositoryId: repository.id, worktreeId: "main" } }
        : {}),
    });
  },
);

test("a first message and the repository instruction go with the new session", async () => {
  const chatConfig = { ...otherConfig, chat: true };
  const withInstruction = { ...repository, initInstruction: "先读 AGENTS.md" };
  open(
    { status: "ready", configs: [chatConfig] },
    {
      repositories: [withInstruction],
      entry: { kind: "worktree", repositoryId: "repo-1", worktreeId: "main" },
    },
  );
  await page.getByRole("radio", { name: "从聊天开始", exact: true }).click();
  await expect.element(page.getByLabelText("使用初始化指令")).toBeChecked();
  await page.getByLabelText("用户指令", { exact: false }).fill("  修复排序 ");
  await submit().click();
  await vi.waitFor(() => expect(created).toHaveLength(1));
  expect(created[0]).toEqual({
    workspace: { repositoryId: "repo-1", worktreeId: "main" },
    configId: "hijklm23",
    name: "",
    view: "chat",
    prompt: "修复排序",
    useInitInstruction: true,
  });
});

test("a terminal start leaves the instruction unticked by default", async () => {
  const withInstruction = { ...repository, initInstruction: "先读 AGENTS.md" };
  open(
    { status: "ready", configs: [config] },
    {
      repositories: [withInstruction],
      entry: { kind: "worktree", repositoryId: "repo-1", worktreeId: "main" },
    },
  );
  await expect.element(page.getByLabelText("使用初始化指令")).not.toBeChecked();
  await submit().click();
  await vi.waitFor(() => expect(created).toHaveLength(1));
  expect(created[0]).not.toHaveProperty("useInitInstruction");
});

test("the instruction is offered only where the repository has one and can be left out", async () => {
  open({ status: "ready", configs: [config] }, { repositories: [repository] });
  await expect
    .element(page.getByLabelText("用户指令", { exact: false }))
    .toBeVisible();
  expect(page.getByLabelText("使用初始化指令").elements()).toHaveLength(0);
  await submit().click();
  await vi.waitFor(() => expect(created).toHaveLength(1));
  expect(created[0]).not.toHaveProperty("prompt");
  expect(created[0]).not.toHaveProperty("useInitInstruction");
});

test("an unticked instruction is not sent", async () => {
  const chatConfig = { ...otherConfig, chat: true };
  const withInstruction = { ...repository, initInstruction: "先读 AGENTS.md" };
  open(
    { status: "ready", configs: [chatConfig] },
    {
      repositories: [withInstruction],
      entry: { kind: "worktree", repositoryId: "repo-1", worktreeId: "main" },
    },
  );
  await page.getByRole("radio", { name: "从聊天开始", exact: true }).click();
  await page.getByLabelText("使用初始化指令").click();
  await submit().click();
  await vi.waitFor(() => expect(created).toHaveLength(1));
  expect(created[0]).not.toHaveProperty("useInitInstruction");
});
