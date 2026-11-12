import { execFile } from "node:child_process";
import { promisify } from "node:util";
import { mkdir, readFile, writeFile } from "node:fs/promises";
import { join } from "node:path";
import type { APIRequestContext, Page } from "@playwright/test";
import {
  test,
  expect,
  createShell,
  expectTerminalLine,
  runCommand,
  viewSwitch,
} from "./fixtures";
import { unfoldRepository, worktreeMenu } from "./repository-menu";

const exec = promisify(execFile);
async function git(path: string, ...args: string[]) {
  return (
    await exec("git", [
      "-C",
      path,
      "-c",
      "user.name=WebMux QA",
      "-c",
      "user.email=qa@example.invalid",
      ...args,
    ])
  ).stdout.trim();
}

// A registered repository on main, with a second branch whose notes.txt
// says which branch it is on.
async function repository(
  request: APIRequestContext,
  root: string,
  name: string,
  branch: string,
) {
  const path = join(root, name);
  await mkdir(path);
  await git(path, "init", "-b", "main");
  await writeFile(join(path, "notes.txt"), "main version\n");
  await git(path, "add", ".");
  await git(path, "commit", "-m", "initial");
  await git(path, "switch", "-c", branch);
  await writeFile(join(path, "notes.txt"), `${branch} version\n`);
  await git(path, "commit", "-am", `on ${branch}`);
  await git(path, "switch", "main");
  const response = await request.post("/api/repositories", {
    data: { path },
  });
  expect(response.status()).toBe(201);
  return path;
}

// Opens the switch of the main worktree from the sidebar and previews the
// branch chosen in its list.
async function previewSwitch(page: Page, repositoryName: string, to: string) {
  await unfoldRepository(page, repositoryName);
  await worktreeMenu(page, "切换分支…", "main");
  const dialog = page.getByRole("dialog", { name: "切换分支：main" });
  await dialog.getByRole("button", { name: "目标分支", exact: true }).click();
  await page.getByRole("option", { name: to, exact: true }).click();
  await expect(dialog.getByRole("region", { name: "切换预览" })).toContainText(
    to,
  );
  return dialog;
}

async function visibleCode(page: Page) {
  return (
    await page
      .locator("[data-code-reader]:visible [data-content]")
      .allTextContents()
  ).join("\n");
}

test("the sidebar switches a clean worktree to another branch", async ({
  page,
  request,
  workDir,
}) => {
  const root = await repository(request, workDir, "switch-clean", "feat/a");
  await page.goto("/");
  const dialog = await previewSwitch(page, "switch-clean", "feat/a");
  await dialog.getByRole("button", { name: "切换", exact: true }).click();
  await expect(dialog).toHaveCount(0);
  expect(await git(root, "branch", "--show-current")).toBe("feat/a");
  expect(await readFile(join(root, "notes.txt"), "utf8")).toBe(
    "feat/a version\n",
  );
  // The worktree keeps its row, now named by the new branch.
  await worktreeMenu(page, "工作树详情…", "feat/a");
  await expect(page.getByRole("dialog", { name: "feat/a" })).toBeVisible();
});

test("uncommitted changes turn the switch into a new worktree for the branch", async ({
  page,
  request,
  workDir,
}) => {
  const root = await repository(request, workDir, "switch-dirty", "feat/b");
  await writeFile(join(root, "notes.txt"), "uncommitted\n");
  await page.goto("/");
  const dialog = await previewSwitch(page, "switch-dirty", "feat/b");
  await expect(
    dialog.getByText("有未提交的修改，请先提交或贮藏"),
  ).toBeVisible();
  await expect(
    dialog.getByRole("button", { name: "切换", exact: true }),
  ).toBeDisabled();
  await dialog.getByRole("button", { name: "改为在新工作树打开" }).click();
  await expect(dialog).toHaveCount(0);

  const create = page.getByRole("dialog", { name: "新建工作树" });
  await expect(
    create.getByRole("radio", { name: "使用已有分支", exact: true }),
  ).toHaveAttribute("aria-checked", "true");
  await expect(
    create.getByRole("button", { name: "已有本地分支", exact: true }),
  ).toContainText("feat/b");
  await create.getByRole("button", { name: "创建工作树", exact: true }).click();
  await expect(create).toHaveCount(0);
  expect(await git(root, "worktree", "list", "--porcelain")).toContain(
    `worktree ${root}.worktrees/b\nHEAD`,
  );
  // The original worktree and its changes are untouched.
  expect(await git(root, "branch", "--show-current")).toBe("main");
  expect(await readFile(join(root, "notes.txt"), "utf8")).toBe("uncommitted\n");
});

test("a running shell is acknowledged, keeps running and its code view follows the switch", async ({
  page,
  request,
  workDir,
}) => {
  const root = await repository(request, workDir, "switch-busy", "feat/c");
  const id = await createShell(page, "Switch shell", root);
  // The code view is open on a file before the switch.
  await viewSwitch(page.locator(".terminal-header"), "代码").click();
  await expect(page.getByRole("region", { name: "代码工作区" })).toBeVisible();
  await page.getByRole("button", { name: "notes.txt", exact: true }).click();
  await expect.poll(() => visibleCode(page)).toContain("main version");

  const dialog = await previewSwitch(page, "switch-busy", "feat/c");
  await expect(dialog).toContainText("Switch shell");
  const confirm = dialog.getByRole("button", { name: "切换", exact: true });
  await expect(confirm).toBeDisabled();
  await dialog.getByRole("checkbox", { name: /我已了解/ }).check();
  await confirm.click();
  await expect(dialog).toHaveCount(0);
  expect(await git(root, "branch", "--show-current")).toBe("feat/c");

  // The tab notices the change through the code view's own check; no other
  // refresh path is involved.
  await expect(
    page.getByRole("status").filter({ hasText: "内容已更新", visible: true }),
  ).toBeVisible({ timeout: 12000 });
  await page
    .getByRole("button", { name: "刷新文档" })
    .filter({ visible: true })
    .click();
  await expect.poll(() => visibleCode(page)).toContain("feat/c version");

  // The shell was neither closed nor restarted and sees the new branch.
  const info = await (await request.get(`/api/sessions/${id}`)).json();
  expect(info).toMatchObject({ state: "active" });
  const terminal = viewSwitch(page.locator(".terminal-header"), "终端");
  if (await terminal.isVisible()) await terminal.click();
  await runCommand(page, "printf 'on-%s\\n' \"$(git branch --show-current)\"");
  await expectTerminalLine(page, "on-feat/c");
});
