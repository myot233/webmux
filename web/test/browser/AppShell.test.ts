import { afterEach, beforeEach, expect, test } from "vitest";
import { page, userEvent } from "vitest/browser";
import { render } from "vitest-browser-svelte/pure";
import { resetAttentionToasts } from "../../src/domains/notifications/attention-toast.svelte";
import { openSession, shellUi } from "../../src/hooks/shared/shell.svelte";
import { createTerminalDrafts } from "../../src/domains/terminal/terminal-draft.svelte";
import {
  commands,
  installChatFixture,
  respond,
  socketAt,
} from "./chat-fixture";
import ShellHarness from "./ShellHarness.svelte";
import {
  chat,
  launchConfig,
  record,
  repository,
  session,
  worktree,
} from "./shell-fixture";

installChatFixture();
beforeEach(() => {
  resetAttentionToasts();
  shellUi.setMode("workbench");
  shellUi.setCollapsed(false);
  history.replaceState(null, "", "/");
  respond(/^GET \/api\/repositories$/, () => Response.json([repository()]));
  respond(/^GET \/api\/launch-configs$/, () => Response.json([launchConfig()]));
  respond(/^GET \/api\/sessions$/, () => Response.json([record()]));
});
afterEach(() => {
  localStorage.removeItem("webmux.shellMode");
  localStorage.removeItem("webmux.sidebarCollapsed");
  localStorage.removeItem("webmux.toastNotifications");
  localStorage.removeItem("webmux.workbenchTree");
  resetAttentionToasts();
  history.replaceState(null, "", "/");
});

const sidebar = () => page.getByRole("complementary", { name: "侧栏" });

async function snapshot(
  sessions = [chat({ id: "chataaaa", title: "修复排序" })],
) {
  const socket = await socketAt("/ws/sessions");
  socket.frame({ type: "snapshot", sessions, forwards: [] } as never);
  return socket;
}

// Must stay first: shellUi reads the saved width once, when it loads.
test("without a saved width the sidebar starts at its default 288px", async () => {
  expect(localStorage.getItem("webmux.sidebarWidth")).toBeNull();
  expect(shellUi.width).toBe(288);
  render(ShellHarness, { drafts: createTerminalDrafts() });
  await snapshot();
  await expect
    .poll(() => sidebar().element().getBoundingClientRect().width)
    .toBe(288);
});

test("the chat button takes a workbench home to quick start", async () => {
  render(ShellHarness, { drafts: createTerminalDrafts() });
  await snapshot();
  const chatMode = sidebar().getByRole("button", { name: "聊天模式" });
  await expect.element(chatMode).toHaveAttribute("aria-pressed", "false");
  await chatMode.click();
  expect(location.pathname).toBe("/new");
  expect(localStorage.getItem("webmux.shellMode")).toBe("chat");
  await expect.element(chatMode).toHaveAttribute("aria-pressed", "true");
  // Quick start puts the caret in its input.
  await expect
    .element(page.getByRole("textbox", { name: "任务描述" }))
    .toHaveFocus();
  await expect
    .element(sidebar().getByRole("list", { name: "聊天历史" }))
    .toBeVisible();
});

test("on a session page switching mode only swaps the sidebar", async () => {
  history.replaceState(null, "", "/s/shellaaa");
  render(ShellHarness, { drafts: createTerminalDrafts() });
  await snapshot([session({ id: "shellaaa", title: "pnpm dev" })]);
  await expect
    .element(sidebar().getByRole("tree", { name: "仓库与会话" }))
    .toBeVisible();
  await sidebar().getByRole("button", { name: "聊天模式" }).click();
  expect(location.pathname).toBe("/s/shellaaa");
  await expect
    .element(sidebar().getByRole("button", { name: "聊天模式" }))
    .toHaveFocus();
  await expect.element(page.getByTestId("session-page")).toBeVisible();
  // A terminal session has no row among the chats; archived records fold below.
  await sidebar()
    .getByRole("button", { name: /已归档/ })
    .click();
  const archived = sidebar().getByRole("list", { name: "已归档聊天" });
  await expect.element(archived).toBeVisible();
  expect(
    archived
      .getByRole("button")
      .elements()
      .filter((row) => row.getAttribute("aria-current") === "page"),
  ).toEqual([]);
});

test("the collapsed rail keeps the expand and primary actions, not the mode switch", async () => {
  render(ShellHarness, { drafts: createTerminalDrafts() });
  await snapshot();
  await page.getByRole("button", { name: "收起侧栏" }).click();
  expect(localStorage.getItem("webmux.sidebarCollapsed")).toBe("true");
  const rail = page.getByRole("navigation", { name: "窄栏导航" });
  await expect
    .element(rail.getByRole("button", { name: "展开侧栏" }))
    .toBeVisible();
  await expect
    .element(rail.getByRole("button", { name: "新建会话" }))
    .toBeVisible();
  expect(rail.getByRole("button", { name: "聊天模式" }).elements()).toEqual([]);
  expect(rail.getByRole("button", { name: "工作台模式" }).elements()).toEqual(
    [],
  );
  expect(
    document.querySelector(".app-shell")?.getAttribute("data-collapsed"),
  ).toBe("true");
  await rail.getByRole("button", { name: "展开侧栏" }).click();
  expect(localStorage.getItem("webmux.sidebarCollapsed")).toBe("false");
  await expect
    .element(sidebar().getByRole("button", { name: "聊天模式" }))
    .toBeVisible();
});

test("the sidebar's new-session action opens the shared dialog", async () => {
  render(ShellHarness, { drafts: createTerminalDrafts() });
  await snapshot();
  await sidebar().getByRole("button", { name: "新建会话" }).click();
  await expect
    .element(page.getByRole("dialog", { name: "新建会话" }))
    .toBeVisible();
});

test("opening any session keeps the sidebar's mode", () => {
  for (const mode of ["workbench", "chat"] as const) {
    shellUi.setMode(mode);
    openSession(session({ id: "shellaaa" }));
    expect(shellUi.mode).toBe(mode);
    expect(location.pathname).toBe("/s/shellaaa");
    openSession(chat({ id: "chataaaa" }));
    expect(shellUi.mode).toBe(mode);
    expect(location.pathname).toBe("/s/chataaaa");
  }
});

test("renaming from the sidebar menu uses the shared dialog", async () => {
  shellUi.setMode("chat");
  history.replaceState(null, "", "/s/otherabc");
  respond(/^PATCH \/api\/sessions\/chataaaa$/, () =>
    Response.json(chat({ id: "chataaaa", title: "首页排序" })),
  );
  render(ShellHarness, { drafts: createTerminalDrafts() });
  const socket = await snapshot();
  const chats = sidebar().getByRole("list", { name: "聊天历史" });
  await sidebar()
    .getByRole("button", { name: /已归档/ })
    .click();
  const archived = sidebar().getByRole("list", { name: "已归档聊天" });
  // Archived records resume or delete their record; they have no rename.
  await archived.getByRole("button", { name: "会话操作：整理文档" }).click();
  await expect
    .element(page.getByRole("menuitem", { name: "删除记录" }))
    .toBeVisible();
  expect(page.getByRole("menuitem", { name: "修改标题" }).elements()).toEqual(
    [],
  );
  await userEvent.keyboard("{Escape}");
  await chats.getByRole("button", { name: "会话操作：修复排序" }).click();
  await page.getByRole("menuitem", { name: "修改标题" }).click();
  const dialog = page.getByRole("dialog");
  const field = dialog.getByRole("textbox");
  await expect.element(field).toHaveValue("修复排序");
  await field.fill("首页排序");
  await userEvent.keyboard("{Enter}");
  await expect.element(dialog).not.toBeInTheDocument();
  expect(commands().at(-1)).toMatchObject({
    method: "PATCH",
    path: "/api/sessions/chataaaa",
    body: { name: "首页排序" },
  });
  // The sidebar shows what the list event brings.
  socket.frame({
    type: "updated",
    session: chat({ id: "chataaaa", title: "首页排序" }),
  } as never);
  await expect
    .element(chats.getByRole("button", { name: "首页排序（空闲）" }))
    .toBeVisible();
});

test("a rejected name stays in the dialog and the row keeps its title", async () => {
  shellUi.setMode("chat");
  history.replaceState(null, "", "/s/otherabc");
  respond(/^PATCH \/api\/sessions\/chataaaa$/, () =>
    Response.json({ error: "名称不能包含控制字符" }, { status: 400 }),
  );
  render(ShellHarness, { drafts: createTerminalDrafts() });
  await snapshot();
  const chats = sidebar().getByRole("list", { name: "聊天历史" });
  await chats.getByRole("button", { name: "会话操作：修复排序" }).click();
  await page.getByRole("menuitem", { name: "修改标题" }).click();
  await page.getByRole("dialog").getByRole("textbox").fill("坏\u0007名");
  await page.getByRole("dialog").getByRole("button", { name: "保存" }).click();
  await expect
    .element(page.getByRole("dialog").getByText("名称不能包含控制字符"))
    .toBeVisible();
  await expect
    .element(chats.getByRole("button", { name: "修复排序（空闲）" }))
    .toBeVisible();
});

test("an archived chat resumes on its chat side, or says why it cannot", async () => {
  shellUi.setMode("chat");
  history.replaceState(null, "", "/s/otherabc");
  respond(/^POST \/api\/sessions\/archivea\/resume$/, () =>
    Response.json(
      { error: "工作目录 /repos/webmux 已不存在" },
      { status: 409 },
    ),
  );
  render(ShellHarness, { drafts: createTerminalDrafts() });
  await snapshot();
  await sidebar()
    .getByRole("button", { name: /已归档/ })
    .click();
  const archived = sidebar().getByRole("list", { name: "已归档聊天" });
  await archived.getByRole("button", { name: "整理文档（已结束）" }).click();
  await expect
    .element(sidebar().getByRole("alert"))
    .toMatchTextContent(
      /无法恢复“整理文档”：工作目录 \/repos\/webmux 已不存在/,
    );
  expect(location.pathname).toBe("/s/otherabc");
  expect(commands().at(-1)).toMatchObject({
    path: "/api/sessions/archivea/resume",
    body: { view: "chat" },
  });

  respond(/^POST \/api\/sessions\/archivea\/resume$/, () =>
    Response.json(chat({ id: "archivea", title: "整理文档" })),
  );
  await archived.getByRole("button", { name: "整理文档（已结束）" }).click();
  await expect.poll(() => location.pathname).toBe("/s/archivea");
});

test("the sidebar menu archives a live chat and deletes an archived record", async () => {
  shellUi.setMode("chat");
  history.replaceState(null, "", "/s/otherabc");
  respond(
    /^DELETE \/api\/sessions\/chataaaa$/,
    () => new Response(null, { status: 204 }),
  );
  respond(
    /^DELETE \/api\/sessions\/archivea$/,
    () => new Response(null, { status: 204 }),
  );
  render(ShellHarness, { drafts: createTerminalDrafts() });
  await snapshot();
  const chats = sidebar().getByRole("list", { name: "聊天历史" });
  await chats.getByRole("button", { name: "会话操作：修复排序" }).click();
  await page.getByRole("menuitem", { name: "停止会话并归档" }).click();
  await page
    .getByRole("alertdialog")
    .getByRole("button", { name: "停止并归档", exact: true })
    .click();
  await expect
    .poll(() =>
      commands().some(
        (c) => c.method === "DELETE" && c.path === "/api/sessions/chataaaa",
      ),
    )
    .toBe(true);

  await sidebar()
    .getByRole("button", { name: /已归档/ })
    .click();
  const archived = sidebar().getByRole("list", { name: "已归档聊天" });
  await archived.getByRole("button", { name: "会话操作：整理文档" }).click();
  await page.getByRole("menuitem", { name: "删除记录" }).click();
  await page
    .getByRole("alertdialog")
    .getByRole("button", { name: "确认" })
    .click();
  await expect
    .poll(() =>
      commands().some(
        (c) =>
          c.method === "DELETE" &&
          c.path === "/api/sessions/archivea" &&
          c.query === "?forget=1",
      ),
    )
    .toBe(true);
});

const linkedTree = worktree({
  id: "wt1",
  path: "/repos/webmux.worktrees/wt1",
  kind: "linked",
  branch: "wt/fix-sort",
});
const linkedChat = record({
  id: "archivew",
  title: "分支聊天",
  cwd: linkedTree.path,
  workspace: {
    repositoryId: "repo1",
    worktreeId: "wt1",
    worktreePath: linkedTree.path,
  },
});
function serveLinkedWorktreeChat() {
  respond(/^GET \/api\/repositories$/, () =>
    Response.json([repository({ worktrees: [worktree(), linkedTree] })]),
  );
  respond(/^GET \/api\/sessions$/, () => Response.json([linkedChat]));
  respond(
    /^POST \/api\/repositories\/repo1\/worktrees\/wt1\/removal-preview$/,
    () =>
      Response.json({
        confirmationId: "conf1234",
        expiresAt: "2026-10-28T01:00:00Z",
        worktree: linkedTree,
        sessions: [],
        archived: [linkedChat],
        commit: "c".repeat(40),
        push: { state: "unpushed", branches: [], more: 0 },
        blockers: [],
        warnings: [],
        forceAllowed: true,
      }),
  );
  respond(
    /^POST \/api\/repositories\/repo1\/worktrees\/wt1\/remove$/,
    () => new Response(null, { status: 204 }),
  );
  respond(
    /^DELETE \/api\/sessions\/archivew$/,
    () => new Response(null, { status: 204 }),
  );
}
async function openLinkedChatArchive() {
  render(ShellHarness, { drafts: createTerminalDrafts() });
  await snapshot();
  await sidebar()
    .getByRole("button", { name: /已归档/ })
    .click();
  const archived = sidebar().getByRole("list", { name: "已归档聊天" });
  await archived.getByRole("button", { name: "会话操作：分支聊天" }).click();
  return archived;
}

test("deleting an archived record leaves its linked worktree in place", async () => {
  shellUi.setMode("chat");
  history.replaceState(null, "", "/s/otherabc");
  serveLinkedWorktreeChat();
  await openLinkedChatArchive();
  await page.getByRole("menuitem", { name: "删除记录" }).click();
  const dialog = page.getByRole("alertdialog");
  await expect
    .element(dialog)
    .toMatchTextContent(/工作树与 agent 自己保存的会话不受影响/);
  await dialog.getByRole("button", { name: "确认" }).click();
  await expect
    .poll(() =>
      commands().some(
        (c) =>
          c.method === "DELETE" &&
          c.path === "/api/sessions/archivew" &&
          c.query === "?forget=1",
      ),
    )
    .toBe(true);
  expect(
    commands().some(
      (c) =>
        c.method === "POST" &&
        c.path === "/api/repositories/repo1/worktrees/wt1/removal-preview",
    ),
  ).toBe(false);
});

test("deleting an archived record and its linked worktree removes both", async () => {
  shellUi.setMode("chat");
  history.replaceState(null, "", "/s/otherabc");
  serveLinkedWorktreeChat();
  await openLinkedChatArchive();
  await page.getByRole("menuitem", { name: "删除记录和工作树" }).click();
  const dialog = page.getByRole("dialog", { name: "删除 worktree" });
  await expect.element(dialog).toBeVisible();
  await expect.element(dialog).toMatchTextContent(/wt\/fix-sort/);
  await expect.element(dialog).toMatchTextContent(/所选聊天记录会一并删除/);
  await expect
    .element(dialog)
    .toMatchTextContent(/将删除的已归档聊天.*分支聊天/);
  expect(dialog.getByText("保留的已归档会话").elements()).toEqual([]);
  expect(dialog.getByText("尚未推送").elements()).toEqual([]);
  await dialog.getByRole("button", { name: "确认安全删除" }).click();
  await expect
    .poll(() =>
      commands().some(
        (c) =>
          c.method === "POST" &&
          c.path === "/api/repositories/repo1/worktrees/wt1/remove",
      ),
    )
    .toBe(true);
  await expect
    .poll(() =>
      commands().some(
        (c) =>
          c.method === "DELETE" &&
          c.path === "/api/sessions/archivew" &&
          c.query === "?forget=1",
      ),
    )
    .toBe(true);
});

const branchTree = worktree({
  id: "wt1",
  path: "/repos/webmux.worktrees/wt1",
  kind: "linked",
  branch: "wt/fix-sort",
  head: "c".repeat(40),
});
const branchChat = record({
  id: "archivew",
  title: "分支聊天",
  cwd: branchTree.path,
  workspace: {
    repositoryId: "repo1",
    worktreeId: "wt1",
    worktreePath: branchTree.path,
  },
});
const releasedChat = record({
  ...branchChat,
  released: {
    id: "rel1",
    repositoryId: "repo1",
    path: branchTree.path,
    commit: "c".repeat(40),
    branch: "wt/fix-sort",
    releasedAt: "2026-10-28T00:00:00Z",
  },
});

test("the sidebar releases an archived chat's worktree after a preview and marks the record", async () => {
  shellUi.setMode("chat");
  history.replaceState(null, "", "/s/otherabc");
  respond(/^GET \/api\/repositories$/, () =>
    Response.json([repository({ worktrees: [worktree(), branchTree] })]),
  );
  respond(/^GET \/api\/sessions$/, () => Response.json([branchChat]));
  respond(
    /^POST \/api\/repositories\/repo1\/worktrees\/wt1\/release-preview$/,
    () =>
      Response.json({
        confirmationId: "conf1234",
        expiresAt: "2026-10-28T01:00:00Z",
        worktree: branchTree,
        commit: "c".repeat(40),
        push: { state: "unpushed", branches: [], more: 0 },
        archived: [branchChat],
        ignored: [
          { path: "node_modules/", bytes: 3 * 1024 * 1024 },
          { path: ".env" },
        ],
        ignoredMore: 2,
        ignoredBytes: 3 * 1024 * 1024,
        sizeComplete: false,
        blockers: [],
      }),
  );
  respond(/^POST \/api\/repositories\/repo1\/worktrees\/wt1\/release$/, () => {
    respond(/^GET \/api\/repositories$/, () =>
      Response.json([repository({ worktrees: [worktree()] })]),
    );
    respond(/^GET \/api\/sessions$/, () => Response.json([releasedChat]));
    return Response.json({ releaseId: "rel1", recordIds: ["archivew"] });
  });
  render(ShellHarness, { drafts: createTerminalDrafts() });
  await snapshot();
  await sidebar()
    .getByRole("button", { name: /已归档/ })
    .click();
  const archived = sidebar().getByRole("list", { name: "已归档聊天" });
  await archived.getByRole("button", { name: "会话操作：分支聊天" }).click();
  await page.getByRole("menuitem", { name: "释放空间" }).click();
  const dialog = page.getByRole("dialog", { name: "释放空间" });
  // Deleted ignored files are warned about, not listed.
  await expect.element(dialog).not.toMatchTextContent(/node_modules\//);
  await expect.element(dialog).not.toMatchTextContent(/随目录删除的忽略文件/);
  await expect.element(dialog).not.toMatchTextContent(/另有 2 项/);
  await expect.element(dialog).toMatchTextContent(/重建工作树也无法找回/);
  await expect
    .element(dialog)
    .toMatchTextContent(/尚未推送到任何远程.*请先推送/);
  // An unpushed commit warns, never blocks.
  await dialog.getByRole("button", { name: "释放空间" }).click();
  await expect
    .element(
      archived.getByRole("button", { name: "分支聊天（已释放空间，已结束）" }),
    )
    .toBeVisible();
  expect(commands().at(-1)).toMatchObject({
    path: "/api/repositories/repo1/worktrees/wt1/release",
    body: { confirmationId: "conf1234" },
  });
  const releasedRow = archived.getByRole("button", {
    name: "分支聊天（已释放空间，已结束）",
  });
  expect(releasedRow.element().textContent).not.toContain("已释放空间");
  expect(
    releasedRow
      .element()
      .querySelector("[data-status]")
      ?.getAttribute("data-status"),
  ).toBe("released");
  // Released, the record offers no second release.
  await archived.getByRole("button", { name: "会话操作：分支聊天" }).click();
  await expect
    .element(page.getByRole("menuitem", { name: "删除记录" }))
    .toBeVisible();
  expect(page.getByRole("menuitem", { name: "释放空间" }).elements()).toEqual(
    [],
  );
});

test("a released archived chat rebuilds its worktree after confirmation before resuming", async () => {
  shellUi.setMode("chat");
  history.replaceState(null, "", "/s/otherabc");
  respond(/^GET \/api\/sessions$/, () => Response.json([releasedChat]));
  respond(/^POST \/api\/hybrid-sessions\/archivew\/rebuild-preview$/, () =>
    Response.json({
      releaseId: "rel1",
      path: branchTree.path,
      commit: "c".repeat(40),
      branch: "wt/fix-sort",
      mode: "detached",
      reason: "branchMissing",
      blockers: [],
    }),
  );
  respond(/^POST \/api\/hybrid-sessions\/archivew\/rebuild$/, () =>
    Response.json({ ...branchTree, branch: null, detached: true }),
  );
  respond(/^POST \/api\/sessions\/archivew\/resume$/, () =>
    Response.json(chat({ id: "archivew", title: "分支聊天" })),
  );
  render(ShellHarness, { drafts: createTerminalDrafts() });
  await snapshot();
  await sidebar()
    .getByRole("button", { name: /已归档/ })
    .click();
  const row = sidebar()
    .getByRole("list", { name: "已归档聊天" })
    .getByRole("button", { name: "分支聊天（已释放空间，已结束）" });

  // Cancelling leaves the record released and resumes nothing.
  await row.click();
  const dialog = page.getByRole("dialog", { name: "重建工作树后恢复" });
  await expect
    .element(dialog)
    .toMatchTextContent(/分支 wt\/fix-sort 已被删除.*detached/);
  await dialog.getByRole("button", { name: "取消" }).click();
  await expect.element(dialog).not.toBeInTheDocument();
  const released = () => commands().filter((c) => c.path.includes("archivew"));
  expect(released().map((c) => c.path)).toEqual([
    "/api/hybrid-sessions/archivew/rebuild-preview",
  ]);
  expect(location.pathname).toBe("/s/otherabc");

  await row.click();
  await dialog.getByRole("button", { name: "重建并恢复" }).click();
  await expect.poll(() => location.pathname).toBe("/s/archivew");
  expect(released().slice(-2)).toMatchObject([
    {
      path: "/api/hybrid-sessions/archivew/rebuild",
      body: { releaseId: "rel1", mode: "detached", commit: "c".repeat(40) },
    },
    { path: "/api/sessions/archivew/resume", body: { view: "chat" } },
  ]);
});

test("the workbench tree lists a released record under 已不可用工作区 and rebuilds it on click", async () => {
  shellUi.setMode("workbench");
  history.replaceState(null, "", "/s/otherabc");
  respond(/^GET \/api\/repositories$/, () =>
    Response.json([repository({ worktrees: [worktree()] })]),
  );
  respond(/^GET \/api\/sessions$/, () => Response.json([releasedChat]));
  respond(/^POST \/api\/hybrid-sessions\/archivew\/rebuild-preview$/, () =>
    Response.json({
      releaseId: "rel1",
      path: branchTree.path,
      commit: "c".repeat(40),
      branch: "wt/fix-sort",
      mode: "branch",
      blockers: [],
    }),
  );
  respond(/^POST \/api\/hybrid-sessions\/archivew\/rebuild$/, () =>
    Response.json(branchTree),
  );
  respond(/^POST \/api\/sessions\/archivew\/resume$/, () =>
    Response.json(chat({ id: "archivew", title: "分支聊天" })),
  );
  render(ShellHarness, { drafts: createTerminalDrafts() });
  await snapshot();
  const tree = sidebar().getByRole("tree", { name: "仓库与会话" });
  const repo = tree.getByRole("treeitem", { name: "webmux" });
  await expect.element(repo).toHaveAttribute("aria-expanded", "false");
  await tree.getByRole("button", { name: "展开webmux" }).click();
  // Only live sessions count: the record alone adds no number.
  const group = tree.getByRole("treeitem", { name: "已不可用工作区" });
  await expect.element(group).toHaveAttribute("aria-expanded", "false");
  await tree.getByRole("button", { name: "展开已不可用工作区" }).click();
  const row = tree.getByRole("treeitem", {
    name: "分支聊天（已释放空间，已结束）",
  });
  await row.click();
  const dialog = page.getByRole("dialog", { name: "重建工作树后恢复" });
  await dialog.getByRole("button", { name: "重建并恢复" }).click();
  await expect.poll(() => location.pathname).toBe("/s/archivew");
});

test("a session on a worktree this page has not read re-reads the repositories once", async () => {
  const fresh = worktree({
    id: "wt9",
    kind: "linked",
    path: "/repos/webmux-new",
    branch: "fresh-branch",
  });
  let reads = 0;
  respond(/^GET \/api\/repositories$/, () => {
    reads += 1;
    return Response.json([
      repository({ worktrees: reads > 1 ? [worktree(), fresh] : [worktree()] }),
    ]);
  });
  render(ShellHarness, { drafts: createTerminalDrafts() });
  await expect.poll(() => reads).toBe(1);
  await snapshot([
    session({
      id: "elsewher",
      title: "别处创建",
      workspace: {
        repositoryId: "repo1",
        worktreeId: "wt9",
        worktreePath: fresh.path,
      },
    }),
  ]);
  await expect.poll(() => reads).toBe(2);
  await expect
    .element(sidebar().getByText("fresh-branch").first())
    .toBeVisible();
  expect(sidebar().getByText("已不可用工作区").elements()).toEqual([]);
  // The worktree is known now, so the settled list asks for nothing more.
  expect(reads).toBe(2);
});

test("shows a top-right toast when a session raises attention and navigates on click", async () => {
  render(ShellHarness, { drafts: createTerminalDrafts() });
  const socket = await snapshot([
    chat({ id: "chataaaa", title: "修复排序" }),
    session({ id: "shellbbb", title: "数据库迁移" }),
  ]);

  socket.frame({
    type: "updated",
    session: session({
      id: "shellbbb",
      title: "数据库迁移",
      status: "waiting",
      attention: {
        kind: "permission",
        source: "agent",
        since: "2026-11-05T12:00:00Z",
      },
    }),
  } as never);

  const region = page.getByRole("region", { name: "会话通知" });
  await expect.element(region).toBeVisible();
  const openButton = region.getByRole("button", {
    name: "打开会话 数据库迁移：等待授权 · Agent 请求执行工具操作",
  });
  await expect.element(openButton).toBeVisible();
  await openButton.click();
  expect(location.pathname).toBe("/s/shellbbb");
  await expect.element(region).not.toBeInTheDocument();
});
