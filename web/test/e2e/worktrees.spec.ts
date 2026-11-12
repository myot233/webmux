import { execFile } from "node:child_process";
import { promisify } from "node:util";
import { mkdir, writeFile, access, rename, rm } from "node:fs/promises";
import { join } from "node:path";
import type { Page, Locator } from "@playwright/test";
import {
  test,
  expect,
  runCommand,
  expectTerminalLine,
  expectConnected,
  nameSession,
  useCustomCommand,
  useDirectory,
  backToList,
  revealSidebar,
} from "./fixtures";
import { expectHomeWorktrees, repositoryMenu } from "./repository-menu";
import { startE2EServer } from "./server.mjs";

const exec = promisify(execFile);
async function git(path: string, ...args: string[]) {
  return (await exec("git", ["-C", path, ...args])).stdout.trim();
}
async function repository(root: string, name = "project with spaces") {
  const path = join(root, name);
  await mkdir(path);
  await git(path, "init", "-b", "main");
  await writeFile(join(path, ".gitignore"), ".env\n");
  await writeFile(join(path, "tracked.txt"), "original\n");
  await git(path, "add", ".");
  await git(
    path,
    "-c",
    "user.name=WebMux QA",
    "-c",
    "user.email=qa@example.invalid",
    "commit",
    "-m",
    "initial",
  );
  return path;
}
async function register(page: Page, path: string) {
  await revealSidebar(page);
  await page.getByRole("button", { name: "添加仓库", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "添加仓库" });
  await dialog.getByLabel("仓库目录").fill(path);
  await dialog.getByRole("button", { name: "添加仓库", exact: true }).click();
  await expect(dialog).toBeHidden();
}
async function expand(group: Locator) {
  const toggle = group.locator("button[aria-expanded]").first();
  if ((await toggle.getAttribute("aria-expanded")) === "false")
    await toggle.click();
}
async function openWorktreeCreation(page: Page, withSession = true) {
  await repositoryMenu(page, "新建工作树…");
  // The start commit and path are folded until requested.
  await page
    .getByRole("button", { name: "高级：起始提交与存放目录", exact: true })
    .click();
  if (withSession)
    await page
      .getByRole("checkbox", { name: "创建后启动会话", exact: true })
      .check();
}
async function createTree(page: Page, branch: string, name: string) {
  await openWorktreeCreation(page);
  const branchInput = page.getByLabel("新分支名称", { exact: true });
  // The default name arrives asynchronously. Wait before fill selects the
  // input, so a late default cannot be inserted ahead of the intended name.
  await expect(branchInput).not.toHaveValue("");
  await branchInput.fill(branch);
  await expect(branchInput).toHaveValue(branch);
  await expect(
    page.getByRole("combobox", { name: "起始提交", exact: true }),
  ).toContainText("initial");
  const path = await page
    .getByLabel("新工作树完整路径", { exact: true })
    .inputValue();
  await useCustomCommand(page, "/bin/bash --noprofile --norc -i");
  await nameSession(page, name);
  await expect(branchInput).toHaveValue(branch);
  await page.getByRole("button", { name: "创建并进入 →", exact: true }).click();
  await expect(page).toHaveURL(/\/s\/[a-z0-9]+$/);
  await expectConnected(page);
  return { path, id: new URL(page.url()).pathname.split("/").at(-1)! };
}

test("repository → two worktrees → shared PTYs → guarded force removal retains branch and replay", async ({
  page,
  request,
  workDir,
}) => {
  const root = await repository(workDir);
  await page.goto("/");
  await register(page, root);
  const repos = await (await request.get("/api/repositories")).json();
  const repo = page.locator(`[data-repository-id="${repos[0].id}"]`);
  await expect(repo).toBeVisible();
  await register(page, root);
  expect(await (await request.get("/api/repositories")).json()).toHaveLength(1);
  const first = await createTree(page, "feature/alpha", "Alpha agent");
  await runCommand(page, "printf '\\n%s%s\\n' 'worktree-' 'replay-proof'");
  await expectTerminalLine(page, "worktree-replay-proof");
  await page.reload();
  await expectTerminalLine(page, "worktree-replay-proof");
  await page.goto("/");
  const second = await createTree(page, "feature/beta", "Beta agent");
  await page.goto("/");
  await expand(repo);
  const trees = (
    await (
      await request.get(`/api/repositories/${repos[0].id}/worktrees`)
    ).json()
  ).worktrees;
  const alpha = trees.find(
    (tree: { path: string }) => tree.path === first.path,
  );
  const alphaGroup = page.locator(`[data-worktree-id="${alpha.id}"]`);
  await alphaGroup
    .getByRole("button", { name: "新建会话", exact: true })
    .click();
  await useCustomCommand(page, "/bin/bash --noprofile --norc -i");
  await nameSession(page, "Alpha helper");
  await page.getByRole("button", { name: "创建并进入 →", exact: true }).click();
  await expect(page).toHaveURL(/\/s\/[a-z0-9]+$/);
  const helperId = new URL(page.url()).pathname.split("/").at(-1)!;
  await runCommand(page, "printf '\\n%s%s\\n' 'independent-' 'helper'");
  await expectTerminalLine(page, "independent-helper");
  expect(
    await (await request.get(`/api/sessions/${first.id}/buffer`)).text(),
  ).not.toContain("independent-helper");
  await writeFile(join(first.path, "tracked.txt"), "changed\n");
  await writeFile(join(first.path, "untracked.txt"), "untracked\n");
  await writeFile(join(first.path, ".env"), "SECRET=local\n");
  await page.goto("/");
  await expand(repo);
  await expect(page.locator("[data-session-link]")).toHaveCount(3);
  const route = `/api/repositories/${repos[0].id}/worktrees/${alpha.id}`;
  const preview = await (
    await request.post(route + "/removal-preview", { data: {} })
  ).json();
  expect(
    (
      await request.post(route + "/remove", {
        data: { confirmationId: preview.confirmationId, mode: "safe" },
      })
    ).status(),
  ).toBe(409);
  await alphaGroup.getByRole("button", { name: /工作树菜单/ }).click();
  await page
    .getByRole("menuitem", { name: "删除 worktree…", exact: true })
    .click();
  await expect(page.getByText("安全删除被阻止", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "取消", exact: true }).click();
  await access(first.path);
  expect(
    (await (await request.get(`/api/sessions/${first.id}`)).json()).status,
  ).not.toBe("exited");
  await alphaGroup.getByRole("button", { name: /工作树菜单/ }).click();
  await page
    .getByRole("menuitem", { name: "删除 worktree…", exact: true })
    .click();
  await page
    .getByRole("button", { name: "进入强制删除确认", exact: true })
    .click();
  const force = page.getByRole("button", {
    name: "关闭会话并强制删除",
    exact: true,
  });
  await expect(force).toBeDisabled();
  await expect(page.getByLabel("输入完整路径以确认删除")).toHaveCount(0);
  await page.getByRole("checkbox").check();
  await force.click();
  await expect(page.getByRole("dialog")).toBeHidden();
  await expect(alphaGroup).toHaveCount(0);
  await expect(repo.getByText("已不可用工作区", { exact: true })).toBeVisible();
  for (const id of [first.id, helperId])
    expect(
      (await (await request.get(`/api/sessions/${id}`)).json()).status,
    ).toBe("exited");
  expect(
    (await (await request.get(`/api/sessions/${second.id}`)).json()).status,
  ).not.toBe("exited");
  expect(await git(root, "branch", "--list", "feature/alpha")).toContain(
    "feature/alpha",
  );
  await expect(access(first.path)).rejects.toThrow();
  expect(
    await (await request.get(`/api/sessions/${first.id}/buffer`)).text(),
  ).toContain("\r\nworktree-replay-proof\r\n");
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
});

test("failed agent start retains new worktree and retry does not create again; unregister keeps terminal", async ({
  page,
  request,
  workDir,
}) => {
  const root = await repository(workDir);
  await page.goto("/");
  await register(page, root);
  await openWorktreeCreation(page);
  const branchInput = page.getByLabel("新分支名称", { exact: true });
  await expect(branchInput).not.toHaveValue("");
  await branchInput.fill("retry-agent");
  await expect(branchInput).toHaveValue("retry-agent");
  await useCustomCommand(page, "/webmux-missing-agent");
  await expect(branchInput).toHaveValue("retry-agent");
  await page.getByRole("button", { name: "创建并进入 →", exact: true }).click();
  await expect(page.getByRole("alert")).toContainText("webmux-missing-agent");
  await expect(
    page.getByText(
      "工作树已创建。再次提交只启动会话，不会重复创建分支或目录。",
    ),
  ).toBeVisible();
  await page
    .getByLabel("自定义启动命令", { exact: true })
    .fill("/bin/bash --noprofile --norc -i");
  await nameSession(page, "Retained shell");
  await page.getByRole("button", { name: "创建并进入 →", exact: true }).click();
  await expect(page).toHaveURL(/\/s\/[a-z0-9]+$/);
  const sessionURL = page.url();
  await page.goto("/");
  const repos = await (await request.get("/api/repositories")).json();
  expect(repos[0].worktrees).toHaveLength(2);
  await repositoryMenu(page, "仓库详情…");
  await page.getByRole("button", { name: "从列表移除仓库…" }).click();
  await page.getByRole("button", { name: "确认移除登记" }).click();
  await expect(
    page.getByRole("region", { name: "直接会话", exact: true }),
  ).toBeVisible();
  await page.goto(sessionURL);
  await expect(page).toHaveURL(sessionURL);
  await runCommand(page, "printf '\\n%s%s\\n' 'still-' 'running'");
  await expectTerminalLine(page, "still-running");
  expect(await git(root, "branch", "--list", "retry-agent")).toContain(
    "retry-agent",
  );
});

test("external changes refresh and restart retains registry but not sessions", async ({
  page,
  request,
  workDir,
  server,
}) => {
  const root = await repository(workDir);
  await page.goto("/");
  await register(page, root);
  const before = await (await request.get("/api/repositories")).json();
  const external = join(workDir, "external worktree");
  await git(root, "worktree", "add", "-b", "external", external);
  await page.getByRole("button", { name: "刷新 Git 状态" }).click();
  const repo = page.locator(`[data-repository-id="${before[0].id}"]`);
  await expand(repo);
  // Externally created worktrees stay off the home page until chosen.
  await expectHomeWorktrees(page, 1, 2);
  await expect(repo.getByText("external", { exact: true })).toHaveCount(0);
  await repositoryMenu(page, "主页显示…");
  // The dialog (a drawer on phones) must stay inside the viewport, even on a
  // 320×568 phone.
  // A closed phone drawer stays in the DOM, so pick the open dialog.
  const picker = page.locator('[role="dialog"][data-state="open"]');
  await expect(picker).toBeVisible();
  const viewport = page.viewportSize()!;
  // Wait out the slide-in before measuring.
  await expect(async () => {
    const box = (await picker.boundingBox())!;
    expect(box.y).toBeGreaterThanOrEqual(0);
    expect(box.y + box.height).toBeLessThanOrEqual(viewport.height);
    expect(box.x).toBeGreaterThanOrEqual(0);
    expect(box.x + box.width).toBeLessThanOrEqual(viewport.width);
  }).toPass();
  await page.getByRole("option", { name: "external", exact: true }).click();
  await page.keyboard.press("Escape");
  await expect(repo.getByText("external", { exact: true })).toBeVisible();
  const session = (
    await (
      await request.post("/api/sessions", {
        data: {
          cwd: external,
          cmd: ["/bin/bash", "--noprofile", "--norc", "-i"],
        },
      })
    ).json()
  ).session;
  expect(session.workspace.repositoryId).toBe(before[0].id);
  await server.restart();
  expect(await (await request.get("/api/sessions")).json()).toEqual([]);
  const after = await (await request.get("/api/repositories")).json();
  expect(after[0].id).toBe(before[0].id);
  expect(
    after[0].worktrees.map((tree: { path: string }) => tree.path),
  ).toContain(external);
  await page.reload();
  await expect(repo).toBeVisible();
  await expand(repo);
  // The home-page choice survives the restart; removing it keeps the worktree.
  const externalGroup = repo.locator(
    `[data-worktree-id="${after[0].worktrees.find((tree: { path: string }) => tree.path === external).id}"]`,
  );
  await expect(externalGroup).toBeVisible();
  await externalGroup.getByRole("button", { name: /工作树菜单/ }).click();
  await page.getByRole("menuitem", { name: "从主页移除", exact: true }).click();
  await expect(externalGroup).toHaveCount(0);
  await access(external);
});

test("without Git an isolated real service still starts and runs a direct shell", async ({
  page,
}) => {
  const isolated = await startE2EServer({ noGit: true });
  try {
    await page.goto(isolated.baseURL);
    const closedDrawer = await revealSidebar(page);
    await expect(
      page.getByRole("button", { name: "添加仓库", exact: true }),
    ).toBeDisabled();
    if (closedDrawer) await page.keyboard.press("Escape");
    const config = await (await fetch(isolated.apiURL + "/api/config")).json();
    expect(config.workspace.gitAvailable).toBe(false);
    expect(config).not.toHaveProperty("token");
    await page
      .getByRole("main")
      .getByRole("button", { name: "＋ 新建会话", exact: true })
      .first()
      .click();
    await useCustomCommand(page, "/bin/bash --noprofile --norc -i");
    await page
      .getByRole("button", { name: "创建并进入 →", exact: true })
      .click();
    await expect(page).toHaveURL(/\/s\/[a-z0-9]+$/);
    await runCommand(page, "printf '\\n%s%s\\n' 'no-git-' 'shell-proof'");
    await expectTerminalLine(page, "no-git-shell-proof");
  } finally {
    await isolated.close();
  }
});

test("multiple repositories keep every session visible through local failure, collapse and long paths", async ({
  page,
  request,
  workDir,
}) => {
  const root = await repository(
    workDir,
    "long-project-" + "segment-".repeat(16),
  );
  const other = await repository(workDir, "second-project");
  const registered = [];
  for (const path of [root, other]) {
    const response = await request.post("/api/repositories", {
      data: { path },
    });
    expect(response.status()).toBe(201);
    registered.push(await response.json());
  }
  const entries = [];
  for (const [cwd, name] of [
    [root, "First shell"],
    [root, "Shared shell"],
    [other, "Second shell"],
    [workDir, "Direct shell"],
  ]) {
    const response = await request.post("/api/sessions", {
      data: { cwd, cmd: ["/bin/bash", "--noprofile", "--norc", "-i"], name },
    });
    expect(response.status()).toBe(201);
    entries.push((await response.json()).session);
  }
  await page.goto("/");
  const tree = page.getByRole("tree", { name: "仓库与会话" });
  const open = page.getByRole("button", { name: "打开侧栏" });
  await expect(tree.or(open).first()).toBeVisible();
  if (!(await tree.isVisible()))
    await page.getByRole("button", { name: "打开侧栏" }).click();
  await tree.locator('[data-tree-key="all"]').click();
  await expect(page.locator("[data-session-link]")).toHaveCount(4);
  for (const item of entries)
    await expect(
      page.getByRole("link", { name: item.title, exact: true }),
    ).toHaveCount(1);
  const firstGroup = page.locator(`[data-repository-id="${registered[0].id}"]`);
  await expect(page.locator("[data-repository-id]")).toHaveCount(2);
  const order = await page
    .locator("[data-repository-id]")
    .evaluateAll((nodes) =>
      nodes.map((node) => node.getAttribute("data-repository-id")),
    );
  const toggle = firstGroup.locator("button[aria-expanded]").first();
  await toggle.click();
  await expect(toggle).toHaveAttribute("aria-expanded", "false");
  await page.getByRole("button", { name: "刷新 Git 状态" }).click();
  await expect(
    page.getByRole("button", { name: "刷新 Git 状态" }),
  ).toHaveAttribute("aria-busy", "false");
  await expect(toggle).toHaveAttribute("aria-expanded", "false");
  expect(
    await page
      .locator("[data-repository-id]")
      .evaluateAll((nodes) =>
        nodes.map((node) => node.getAttribute("data-repository-id")),
      ),
  ).toEqual(order);
  await toggle.focus();
  await page.keyboard.press("Enter");
  await expect(toggle).toHaveAttribute("aria-expanded", "true");
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  await rename(other, other + "-moved");
  await page.getByRole("button", { name: "刷新 Git 状态" }).click();
  const failed = page.locator(`[data-repository-id="${registered[1].id}"]`);
  await expect(
    failed.getByText("已不可用工作区", { exact: true }),
  ).toBeVisible();
  await expect(page.locator("[data-session-link]")).toHaveCount(4);
  await repositoryMenu(page, "仓库详情…", registered[1].name);
  await page.getByRole("button", { name: "从列表移除仓库…" }).click();
  await page.getByRole("button", { name: "确认移除登记" }).click();
  await expect(failed).toHaveCount(0);
  await expect(
    page
      .getByRole("region", { name: "直接会话", exact: true })
      .getByRole("link", { name: "Second shell", exact: true }),
  ).toBeVisible();
  await revealSidebar(page);
  await page.getByRole("button", { name: "添加仓库", exact: true }).click();
  const addDialog = page.getByRole("dialog", { name: "添加仓库" });
  await addDialog.getByLabel("仓库目录").fill(workDir);
  await addDialog
    .getByRole("button", { name: "添加仓库", exact: true })
    .click();
  await expect(addDialog.getByRole("alert")).toBeVisible();
  await expect(access(join(workDir, ".git"))).rejects.toThrow();
  await addDialog.getByRole("button", { name: "取消", exact: true }).click();
});

test("upstream refresh selects remote commits and falls back to a creatable local base", async ({
  page,
  request,
  workDir,
}) => {
  const root = await repository(workDir);
  const localHead = await git(root, "rev-parse", "main");
  const remote = join(workDir, "remote.git");
  await git(root, "clone", "--bare", root, remote);
  await git(root, "remote", "add", "origin", remote);
  await git(root, "push", "-u", "origin", "main");
  const other = join(workDir, "upstream clone");
  await git(root, "clone", remote, other);
  const advance = async (subject: string) => {
    await writeFile(join(other, "remote.txt"), subject + "\n");
    await git(other, "add", "remote.txt");
    await git(
      other,
      "-c",
      "user.name=WebMux QA",
      "-c",
      "user.email=qa@example.invalid",
      "commit",
      "-m",
      subject,
    );
    await git(other, "push", "origin", "main");
    return git(other, "rev-parse", "HEAD");
  };
  const remoteOne = await advance("remote one");
  await page.goto("/");
  await register(page, root);
  await openWorktreeCreation(page, false);
  const base = page.getByRole("combobox", { name: "起始提交", exact: true });
  const short = (oid: string) => oid.slice(0, 7);
  await expect(base).toContainText(short(remoteOne));
  await expect(base).toContainText("remote one");
  await expect(
    page.getByRole("status", { name: "上游刷新状态" }),
  ).toContainText("远程最新 · origin/main · remote one");

  const remoteTwo = await advance("remote two");
  await page.getByRole("button", { name: "刷新上游", exact: true }).click();
  await expect(base).toContainText(short(remoteTwo));
  await expect(
    page.getByRole("status", { name: "上游刷新状态" }),
  ).toContainText("remote two");
  // The fetched tip leads the list, followed by the older history.
  await base.click();
  const choices = page.getByRole("listbox").getByRole("option");
  await expect(choices.first()).toHaveAccessibleName(
    new RegExp(`^remote two · ${short(remoteTwo)} · 远程最新`),
  );
  await expect(choices.filter({ hasText: "remote one" }).first()).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(choices.first()).toBeHidden();

  await git(root, "remote", "set-url", "origin", join(workDir, "missing.git"));
  await page.getByRole("button", { name: "刷新上游", exact: true }).click();
  await expect(base).toContainText(short(localHead));
  await expect(
    page.getByRole("status", { name: "上游刷新警告" }),
  ).toContainText("保留本地基线");
  await page
    .getByLabel("新分支名称", { exact: true })
    .fill("feature/from-local");
  const path = await page
    .getByLabel("新工作树完整路径", { exact: true })
    .inputValue();
  await page.getByRole("button", { name: "创建工作树", exact: true }).click();
  await expect(page.getByRole("dialog")).toBeHidden();
  const repo = (await (await request.get("/api/repositories")).json())[0];
  const created = repo.worktrees.find(
    (item: { path: string }) => item.path === path,
  );
  expect(created).toMatchObject({
    branch: "feature/from-local",
    head: localHead,
  });
});

test("existing branch creation, safe removal and stale destructive confirmation require fresh consent", async ({
  page,
  request,
  workDir,
}) => {
  const root = await repository(workDir);
  await git(root, "branch", "spare");
  await page.goto("/");
  await register(page, root);
  await openWorktreeCreation(page, false);
  await page.getByRole("radio", { name: "使用已有分支", exact: true }).click();
  await page.getByRole("button", { name: "已有本地分支", exact: true }).click();
  await page.getByRole("combobox", { name: "已有本地分支" }).fill("spare");
  await page.getByRole("option", { name: "spare", exact: true }).click();
  await expect(page.getByLabel("新工作树完整路径")).toHaveValue(/\/spare$/);
  const path = await page.getByLabel("新工作树完整路径").inputValue();
  await page.getByRole("button", { name: "创建工作树", exact: true }).click();
  await expect(page.getByRole("dialog")).toBeHidden();
  const repo = (await (await request.get("/api/repositories")).json())[0];
  const tree = repo.worktrees.find(
    (item: { path: string }) => item.path === path,
  );
  const group = page.locator(`[data-worktree-id="${tree.id}"]`);
  await expand(page.locator(`[data-repository-id="${repo.id}"]`));
  await group.getByRole("button", { name: /工作树菜单/ }).click();
  await page
    .getByRole("menuitem", { name: "删除 worktree…", exact: true })
    .click();
  await expect(
    page.getByRole("button", { name: "确认安全删除", exact: true }),
  ).toBeVisible();
  const started = (
    await (
      await request.post("/api/sessions", {
        data: {
          cwd: path,
          cmd: ["/bin/bash", "--noprofile", "--norc", "-i"],
          name: "New arrival",
        },
      })
    ).json()
  ).session;
  await page.getByRole("button", { name: "确认安全删除", exact: true }).click();
  await expect(page.getByRole("dialog").getByRole("alert")).toContainText(
    "重新",
  );
  expect(
    (await (await request.get(`/api/sessions/${started.id}`)).json()).status,
  ).not.toBe("exited");
  await access(path);
  await request.delete(`/api/sessions/${started.id}`);
  await expect
    .poll(
      async () =>
        (await (await request.get(`/api/sessions/${started.id}`)).json())
          .status,
    )
    .toBe("exited");
  await page.getByRole("button", { name: "重新读取预览", exact: true }).click();
  await page.getByRole("button", { name: "确认安全删除", exact: true }).click();
  await expect(page.getByRole("dialog")).toBeHidden();
  await expect(access(path)).rejects.toThrow();
  expect(await git(root, "branch", "--list", "spare")).toContain("spare");
});

test("late Git response cannot replace a newer refresh", async ({
  page,
  request,
  workDir,
}) => {
  const root = await repository(workDir);
  await request.post("/api/repositories", { data: { path: root } });
  const old = await (await request.get("/api/repositories")).json();
  await page.goto("/");
  await expect(page.locator("[data-repository-id]")).toHaveCount(1);
  let release!: () => void;
  const delayed = new Promise<void>((resolve) => {
    release = resolve;
  });
  let captured!: () => void;
  const capture = new Promise<void>((resolve) => {
    captured = resolve;
  });
  let intercepted = false;
  await page.route("**/api/repositories", async (route) => {
    if (intercepted) {
      await route.continue();
      return;
    }
    intercepted = true;
    captured();
    await delayed;
    await route.fulfill({ status: 200, json: old });
  });
  await page.getByRole("button", { name: "刷新 Git 状态" }).click();
  await capture;
  const external = join(workDir, "newer-snapshot");
  await git(root, "worktree", "add", "-b", "newer-snapshot", external);
  await page.evaluate(() =>
    document.dispatchEvent(new Event("visibilitychange")),
  );
  const repo = page.locator("[data-repository-id]");
  await expand(repo);
  await expectHomeWorktrees(page, 1, 2);
  const lateResponse = page.waitForResponse((response) =>
    response.url().endsWith("/api/repositories"),
  );
  release();
  await (await lateResponse).finished();
  await page.evaluate(
    () =>
      new Promise<void>((resolve) =>
        requestAnimationFrame(() => requestAnimationFrame(() => resolve())),
      ),
  );
  await expect(
    page.getByRole("button", { name: "刷新 Git 状态" }),
  ).toHaveAttribute("aria-busy", "false");
  await expectHomeWorktrees(page, 1, 2);
});

test("returning home keeps the cached snapshot until a slow Git refresh finishes", async ({
  page,
  request,
  workDir,
}) => {
  const root = await repository(workDir);
  await request.post("/api/repositories", { data: { path: root } });
  const [repo] = await (await request.get("/api/repositories")).json();
  const created = await request.post("/api/sessions", {
    data: {
      name: "Cached grouping",
      cmd: ["/bin/sh", "-c", "sleep 600"],
      workspace: { repositoryId: repo.id, worktreeId: repo.worktrees[0].id },
    },
  });
  expect(created.status()).toBe(201);
  const { id } = (await created.json()).session;
  await page.goto("/");
  const group = page.locator(`[data-repository-id="${repo.id}"]`);
  await expect(group).toHaveCount(1);
  await page
    .getByRole("link", { name: "Cached grouping", exact: true })
    .click();
  await expect(page).toHaveURL(new RegExp(`/s/${id}$`));
  await expect(page.locator(".xterm-helper-textarea")).toBeAttached();
  const external = join(workDir, "cached-newer");
  await git(root, "worktree", "add", "-b", "cached-newer", external);
  const held = Promise.withResolvers<void>();
  await page.route("**/api/repositories", async (route) => {
    await held.promise;
    await route.continue();
  });
  try {
    await backToList(page);
    // The cached snapshot keeps the session grouped under its repository
    // while the delayed refresh is still in flight.
    await expect(group).toHaveCount(1);
    await expect(
      group.getByRole("link", { name: "Cached grouping", exact: true }),
    ).toBeVisible();
    await expect(page.getByText(/Git 状态(?:刷新失败|读取失败)/)).toHaveCount(
      0,
    );
    await expectHomeWorktrees(page, 1, 1);
    const refreshed = page.waitForResponse(
      (response) =>
        response.url().endsWith("/api/repositories") && response.ok(),
    );
    held.resolve();
    await (await refreshed).finished();
    await expectHomeWorktrees(page, 1, 2);
  } finally {
    held.resolve();
  }
});

test("direct launch still offers explicit directory creation without registering a repository", async ({
  page,
  request,
  workDir,
}) => {
  const path = join(workDir, "new direct directory");
  await page.goto("/");
  await page
    .getByRole("main")
    .getByRole("button", { name: "＋ 新建会话", exact: true })
    .first()
    .click();
  await useDirectory(page, path);
  await useCustomCommand(page, "/bin/bash --noprofile --norc -i");
  await page.getByRole("button", { name: "创建并进入 →", exact: true }).click();
  await expect(page.getByRole("alertdialog")).toBeVisible();
  await page
    .getByRole("alertdialog")
    .getByRole("button", { name: "取消", exact: true })
    .click();
  await expect(access(path)).rejects.toThrow();
  await page.getByRole("button", { name: "创建并进入 →", exact: true }).click();
  await page
    .getByRole("button", { name: "创建目录并启动", exact: true })
    .click();
  await expect(page).toHaveURL(/\/s\/[a-z0-9]+$/);
  const sessions = await (await request.get("/api/sessions")).json();
  expect(sessions).toHaveLength(1);
  expect(sessions[0]).toMatchObject({ cwd: path, workspace: null });
  expect(await (await request.get("/api/repositories")).json()).toEqual([]);
});

test("batch worktree management cleans unpinned and prunable worktrees while skipping blocked ones and keeping branches", async ({
  page,
  workDir,
}) => {
  const root = await repository(workDir, "batch-repo");
  const cleanPath = join(workDir, "batch-clean");
  const stalePath = join(workDir, "batch-stale");
  const dirtyPath = join(workDir, "batch-dirty");
  await git(root, "worktree", "add", "-b", "wt/batch-clean", cleanPath);
  await git(root, "worktree", "add", "-b", "wt/batch-stale", stalePath);
  await git(root, "worktree", "add", "-b", "wt/batch-dirty", dirtyPath);
  await rm(stalePath, { recursive: true, force: true });
  await writeFile(join(dirtyPath, "tracked.txt"), "uncommitted edit\n");

  await page.goto("/");
  await register(page, root);
  const group = page.locator("[data-repository-id]");
  await expect(group).toHaveCount(1);

  // "管理工作树…" opens the repository panel at its worktrees section, where
  // batch cleanup is a selection mode of the list.
  await repositoryMenu(page, "管理工作树…");
  const dialog = page.getByRole("dialog", { name: "batch-repo" });
  await expect(dialog).toBeVisible();
  await expect(dialog.getByRole("tab", { name: "工作树" })).toHaveAttribute(
    "aria-selected",
    "true",
  );
  await expect(dialog).toContainText("Git 登记待清理");
  await expect(dialog).toContainText("未添加到主页");
  await dialog.getByRole("button", { name: "批量清理", exact: true }).click();
  await expect(dialog.getByRole("checkbox", { name: "main" })).toBeDisabled();

  await dialog.getByRole("button", { name: "选中全部可安全清理（2）" }).click();
  await dialog.getByRole("checkbox", { name: "wt/batch-dirty" }).click();
  await dialog.getByRole("button", { name: "下一步：预览删除 (3)" }).click();

  await expect(
    dialog.getByRole("heading", { name: "确认批量清理" }),
  ).toBeVisible();
  await expect(dialog).toContainText("可安全清理 2 个");
  await expect(dialog).toContainText("有阻塞 1 个");
  await dialog.getByRole("button", { name: "确认安全删除 (2)" }).click();
  await expect(dialog.getByText("已删除 2 个工作树。")).toBeVisible();

  await expect(access(cleanPath)).rejects.toThrow();
  await access(dirtyPath);
  expect(
    await git(root, "show-ref", "--verify", "refs/heads/wt/batch-clean"),
  ).toContain("refs/heads/wt/batch-clean");
  expect(
    await git(root, "show-ref", "--verify", "refs/heads/wt/batch-stale"),
  ).toContain("refs/heads/wt/batch-stale");
  const porcelain = await git(root, "worktree", "list", "--porcelain");
  expect(porcelain).not.toContain(cleanPath);
  expect(porcelain).not.toContain(stalePath);
  expect(porcelain).toContain(dirtyPath);
});
