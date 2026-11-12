import { spawnSync } from "node:child_process";
import { mkdir } from "node:fs/promises";
import { basename, join } from "node:path";
import type { APIRequestContext, Page } from "@playwright/test";
import { expect, expectTerminalLine, test } from "./fixtures";

// Quick start over the fake agent (internal/acp/testdata/fakeagent), which
// global-setup puts on PATH as claude and claude-agent-acp: its chat side
// answers "echo: text". Each test registers its own temporary repository.

async function repository(
  request: APIRequestContext,
  root: string,
  initInstruction = "",
) {
  await mkdir(root);
  for (const args of [
    ["init", "-b", "main"],
    [
      "-c",
      "user.name=WebMux QA",
      "-c",
      "user.email=qa@example.invalid",
      "commit",
      "--allow-empty",
      "-m",
      "initial",
    ],
  ])
    expect(spawnSync("git", ["-C", root, ...args]).status).toBe(0);
  const response = await request.post("/api/repositories", {
    data: { path: root },
  });
  expect(response.status(), await response.text()).toBe(201);
  const repo = (await response.json()) as { id: string };
  if (initInstruction) {
    const patch = await request.patch(`/api/repositories/${repo.id}`, {
      data: { initInstruction },
    });
    expect(patch.status(), await patch.text()).toBe(200);
  }
  return repo;
}

async function hybridConfig(request: APIRequestContext, name: string) {
  const response = await request.post("/api/launch-configs", {
    data: { name, icon: "claudecode", cmd: ["claude"], env: {}, chat: true },
  });
  expect(response.status(), await response.text()).toBe(201);
}

const phone = (page: Page) => (page.viewportSize()?.width ?? 1280) < 640;

// The sidebar: always shown on desktop, a drawer opened from the top bar on
// phones. The drawer stays open when only the mode changes.
async function sidebar(page: Page) {
  if (phone(page))
    await page.getByRole("button", { name: "打开侧栏" }).first().click();
  return page.getByRole("navigation", { name: "导航" }).last();
}

const conversation = (page: Page) =>
  page.getByRole("list", { name: "对话", exact: true });
const sideTab = (page: Page, view: "chat" | "terminal") =>
  page
    .getByRole("tablist", { name: "会话视图", exact: true })
    .getByRole(
      "tab",
      view === "chat" ? { name: /^聊天/ } : { name: "终端", exact: true },
    );

test("one send creates a worktree and starts the chat; the result is an ordinary hybrid session", async ({
  page,
  request,
  workDir,
}, info) => {
  test.slow();
  await page.setViewportSize(
    info.project.use.isMobile
      ? { width: 390, height: 844 }
      : { width: 1280, height: 720 },
  );
  const root = join(workDir, "qs-repo");
  const repo = await repository(request, root, "先读 AGENTS.md");
  await hybridConfig(request, "Claude Code");

  await page.goto("/");
  const nav = await sidebar(page);
  await nav.getByRole("button", { name: "聊天模式" }).click();
  await expect(page).toHaveURL(/\/new$/);
  await expect(nav.getByRole("button", { name: "聊天模式" })).toHaveAttribute(
    "aria-pressed",
    "true",
  );
  if (phone(page)) {
    // Only the mode changed, so the drawer is still open.
    await expect(nav.getByRole("list", { name: "聊天历史" })).toHaveCount(0);
    await expect(nav.getByText("还没有聊天")).toBeVisible();
    await page.keyboard.press("Escape");
  }

  const toolbar = page.getByRole("toolbar", { name: "仓库与分支" });
  const input = page.getByRole("textbox", { name: "任务描述" });
  if (phone(page)) {
    await expect(toolbar).toBeHidden();
    await input.click();
  }
  await expect(
    toolbar.getByRole("button", { name: "仓库：qs-repo" }),
  ).toBeVisible();
  await expect(
    toolbar.getByRole("button", { name: "基准分支：main" }),
  ).toBeVisible();
  await expect(
    page.getByRole("checkbox", { name: "使用初始化指令" }),
  ).toBeChecked();
  await input.fill("修复首页排序");
  await page.getByRole("button", { name: "发送", exact: true }).click();

  await expect(page).toHaveURL(/\/s\/[a-z2-7]{8}$/, { timeout: 30_000 });
  const id = page.url().split("/").at(-1)!;
  // The first message is the instruction, a blank line and the task.
  await expect(conversation(page)).toContainText("先读 AGENTS.md");
  await expect(conversation(page)).toContainText("修复首页排序");
  await expect(conversation(page)).toContainText("echo: 先读 AGENTS.md");
  // A new linked worktree holds the session.
  const listed = await (
    await request.get(`/api/repositories/${repo.id}/worktrees`)
  ).json();
  expect(listed.worktrees).toHaveLength(2);
  const session = await (await request.get(`/api/sessions/${id}`)).json();
  expect(session.view).toBe("chat");
  expect(session.workspace.worktreeId).not.toBe(listed.worktrees[0].id);
  // The chat shows where it runs above the input, read-only (expanded on focus on phones).
  const where = page.getByRole("group", { name: "会话位置" });
  if (phone(page)) {
    await expect(where).toBeHidden();
    await page.getByRole("textbox", { name: "消息" }).click();
  }
  await expect(where).toContainText("qs-repo");
  await expect(where).toContainText(/wt\/[a-z]+-[a-z]+/);
  await expect(where.getByRole("button")).toHaveCount(0);

  // The chat history lists it as the current chat.
  const chats = (await sidebar(page)).getByRole("list", { name: "聊天历史" });
  await expect(chats.locator('[aria-current="page"]')).toHaveCount(1);
  if (phone(page)) await page.keyboard.press("Escape");

  // Echoed text can arrive while the first turn is still running. Like
  // hybrid.spec.ts's chatTurn, wait for completion before the idle switch;
  // an active turn correctly asks for interruption confirmation instead.
  await expect(
    page.getByRole("status").filter({ hasText: "正在工作…" }),
  ).toHaveCount(0);
  await sideTab(page, "terminal").click();
  await expect(sideTab(page, "terminal")).toHaveAttribute(
    "aria-selected",
    "true",
  );
  await expectTerminalLine(page, "> 先读 AGENTS.md");
  await expect(sideTab(page, "chat")).toHaveAccessibleName("聊天", {
    timeout: 20_000,
  });
  await sideTab(page, "chat").click();
  await expect(sideTab(page, "chat")).toHaveAttribute("aria-selected", "true");
  await expect(conversation(page)).toContainText("echo: 先读 AGENTS.md");

  // Archived, it stays in the chat history and resumes from there.
  await page.getByRole("button", { name: "会话菜单", exact: true }).click();
  await page
    .getByRole("menuitem", { name: "停止会话并归档", exact: true })
    .click();
  await page
    .getByRole("alertdialog")
    .getByRole("button", { name: "停止并归档", exact: true })
    .click();
  await expect(page).toHaveURL(/\/new$/);
  const navAfterArchive = await sidebar(page);
  await navAfterArchive.getByRole("button", { name: /^已归档/ }).click();
  const history = navAfterArchive.getByRole("list", { name: "已归档聊天" });
  const archived = history.getByRole("button", { name: /（已结束）$/ });
  await expect(archived).toHaveCount(1);
  // Deleting the record and its linked worktree opens the worktree
  // confirmation dialog; deleting only the record would keep the worktree.
  await history.getByRole("button", { name: /^会话操作：/ }).click();
  await page
    .getByRole("menuitem", { name: "删除记录和工作树", exact: true })
    .click();
  await expect(
    page.getByRole("dialog", { name: "删除 worktree" }),
  ).toBeVisible();
  await page
    .getByRole("dialog", { name: "删除 worktree" })
    .getByRole("button", { name: "取消", exact: true })
    .click();
  await expect(
    page.getByRole("dialog", { name: "删除 worktree" }),
  ).toBeHidden();
  // The workbench home shows the same record in its worktree group.
  if (phone(page)) await page.keyboard.press("Escape");
  const nav2 = await sidebar(page);
  await nav2.getByRole("button", { name: "工作台模式" }).click();
  await expect(page).toHaveURL(/\/$/);
  if (phone(page)) await page.keyboard.press("Escape");
  await expect(
    page.getByRole("region", { name: /的已归档会话$/ }).first(),
  ).toBeVisible();
  // Back in chat mode, the record resumes on its chat side.
  const nav3 = await sidebar(page);
  await nav3.getByRole("button", { name: "聊天模式" }).click();
  await nav3.getByRole("button", { name: /^已归档/ }).click();
  await nav3
    .getByRole("list", { name: "已归档聊天" })
    .getByRole("button", { name: /（已结束）$/ })
    .click();
  await expect(page).toHaveURL(new RegExp(`/s/${id}$`));
  await expect(conversation(page)).toContainText("修复首页排序");
  expect(basename(root)).toBe("qs-repo");
});

test("phone drawer: the chat history switches sessions and closes", async ({
  page,
  request,
  workDir,
}, info) => {
  test.skip(!info.project.use.isMobile, "the drawer is the phone sidebar");
  await page.setViewportSize({ width: 390, height: 844 });
  await hybridConfig(request, "Claude Code");
  const configs = await (await request.get("/api/launch-configs")).json();
  const ids: string[] = [];
  for (const name of ["第一个聊天", "第二个聊天"]) {
    const response = await request.post("/api/sessions", {
      data: { cwd: workDir, configId: configs[0].id, view: "chat", name },
    });
    expect(response.status(), await response.text()).toBe(201);
    ids.push((await response.json()).session.id);
  }
  await page.goto(`/s/${ids[0]}`);
  const open = page.getByRole("button", { name: "打开侧栏" });
  await open.click();
  const drawer = page.getByRole("dialog", { name: "导航" });
  await expect(drawer).toBeVisible();
  await drawer.getByRole("button", { name: "聊天模式" }).click();
  // Only the mode changed: the drawer stays open on the chat history.
  await expect(drawer).toBeVisible();
  await drawer.getByRole("button", { name: /^第二个聊天/ }).click();
  await expect(drawer).toBeHidden();
  await expect(page).toHaveURL(new RegExp(`/s/${ids[1]}$`));
  // The top bar stays on one line.
  const header = page.locator(".terminal-header").first();
  expect((await header.boundingBox())!.height).toBeLessThanOrEqual(48);
});

test("the quick command menu inserts a preset as written and the server fills the placeholders", async ({
  page,
  request,
  workDir,
}) => {
  test.slow();
  await repository(request, join(workDir, "qs-quick-repo"));
  await hybridConfig(request, "Claude Code");

  await page.goto("/new");
  const input = page.getByRole("textbox", { name: "任务描述" });
  await input.fill("读手册：");
  await page.getByRole("button", { name: "快捷指令", exact: true }).click();
  await page
    .getByRole("menuitem", { name: /WebMux 工具/ })
    .first()
    .click();
  // The page does not expand it: the placeholder is in the box, unsent.
  await expect(input).toHaveValue("读手册：{{webmux}}");
  await expect(page).toHaveURL(/\/new$/);
  await input.press("End");
  await input.pressSequentially(" 文件放 {{dir}}");
  await page.getByRole("button", { name: "发送", exact: true }).click();

  await expect(page).toHaveURL(/\/s\/[a-z2-7]{8}$/, { timeout: 30_000 });
  // The reply echoes the whole message over several paragraphs.
  await expect(conversation(page)).toContainText("llms.txt");
  await expect(conversation(page)).not.toContainText("{{");
});

test("picking an existing worktree starts the chat directly in it without creating a new worktree", async ({
  page,
  request,
  workDir,
}, info) => {
  test.slow();
  await page.setViewportSize(
    info.project.use.isMobile
      ? { width: 390, height: 844 }
      : { width: 1280, height: 720 },
  );
  const root = join(workDir, "qs-existing-repo");
  const repo = await repository(request, root);
  await hybridConfig(request, "Claude Code");

  await page.goto("/new");
  const toolbar = page.getByRole("toolbar", { name: "仓库与分支" });
  const input = page.getByRole("textbox", { name: "任务描述" });
  if (phone(page)) {
    await input.click();
  }
  await toolbar.getByRole("button", { name: "基准分支：main" }).click();
  const modeGroup = page.getByRole("radiogroup", { name: "工作树方式" });
  await modeGroup.getByRole("radio", { name: "已有工作树" }).click();
  await page
    .getByRole("listbox", { name: "工作树列表" })
    .getByRole("option", { name: /主工作区 · main/ })
    .click();
  await expect(
    toolbar.getByRole("button", { name: "工作树：主工作区 · main" }),
  ).toBeVisible();
  await input.fill("run in main worktree");
  await page.getByRole("button", { name: "发送", exact: true }).click();

  await expect(page).toHaveURL(/\/s\/[a-z2-7]{8}$/, { timeout: 30_000 });
  const id = page.url().split("/").at(-1)!;
  await expect(conversation(page)).toContainText("echo: run in main worktree");
  const listed = await (
    await request.get(`/api/repositories/${repo.id}/worktrees`)
  ).json();
  expect(listed.worktrees).toHaveLength(1);
  const session = await (await request.get(`/api/sessions/${id}`)).json();
  expect(session.workspace.worktreeId).toBe(listed.worktrees[0].id);
});
