import { execFileSync } from "node:child_process";
import { mkdir, writeFile, readFile } from "node:fs/promises";
import { join } from "node:path";
import {
  test,
  expect,
  createShell,
  runCommand,
  expectTerminalLine,
} from "./fixtures";
function git(root: string, ...args: string[]) {
  return execFileSync("git", ["-C", root, ...args], {
    env: {
      ...process.env,
      GIT_AUTHOR_NAME: "E2E",
      GIT_AUTHOR_EMAIL: "e2e@example.com",
      GIT_COMMITTER_NAME: "E2E",
      GIT_COMMITTER_EMAIL: "e2e@example.com",
    },
  }).toString();
}
async function showCode(page: import("@playwright/test").Page) {
  await page
    .locator(".terminal-header")
    .getByRole("button", { name: "代码", exact: true })
    .click();
  await expect(page.getByRole("region", { name: "代码工作区" })).toBeVisible();
}
async function nav(page: import("@playwright/test").Page) {
  const button = page.getByRole("button", { name: "文件导航", exact: true });
  if (await button.isVisible()) await button.click();
}

test("read ordinary files, independent tabs, updates and insert a line without Enter", async ({
  page,
  request,
  workDir,
}, info) => {
  await writeFile(
    join(workDir, "clean.go"),
    "package main\n// unchanged line\nfunc main() {}\n",
  );
  await writeFile(join(workDir, "other.txt"), "independent document\n");
  const id = await createShell(page, "Code shell", workDir);
  await runCommand(page, "printf 'code-pty-ready\\n'");
  await expectTerminalLine(page, "code-pty-ready");
  await page.locator(".xterm-helper-textarea").focus();
  await page.keyboard.type("printf 'reference: %s\\n' ");
  await showCode(page);
  await page.getByRole("button", { name: "clean.go", exact: true }).click();
  await expect(
    page.getByLabel("只读代码").filter({ visible: true }),
  ).toContainText("unchanged line");
  await page
    .locator(".code-reader:visible .cm-lineNumbers .cm-gutterElement")
    .filter({ hasText: /^2$/ })
    .click();
  await expect(
    page.getByLabel("位置预览").filter({ visible: true }),
  ).toHaveText(join(workDir, "clean.go") + ":2 ");
  await page.getByRole("button", { name: "发送位置到终端" }).click();
  if (info.project.name.includes("mobile")) {
    await expect(page.locator(".terminal-host")).toBeVisible();
  } else
    await page
      .locator(".terminal-header")
      .getByRole("button", { name: "折叠代码" })
      .click();
  const buffer = await (await request.get(`/api/sessions/${id}/buffer`)).text();
  expect(buffer).not.toContain(`reference: ${join(workDir, "clean.go")}:2\r\n`);
  await page.locator(".xterm-helper-textarea").focus();
  await page.keyboard.press("Enter");
  await expect
    .poll(
      async () =>
        await (await request.get(`/api/sessions/${id}/buffer`)).text(),
    )
    .toContain(`reference: ${join(workDir, "clean.go")}:2\r\n`);
  await showCode(page);
  await nav(page);
  await page.getByRole("button", { name: "other.txt", exact: true }).click();
  await expect(
    page.getByLabel("只读代码").filter({ visible: true }),
  ).toContainText("independent document");
  await page.getByRole("tab", { name: /clean.go · worktree/ }).click();
  await expect(
    page.getByLabel("只读代码").filter({ visible: true }),
  ).toContainText("unchanged line");
  await writeFile(join(workDir, "clean.go"), "new version\n");
  await expect(
    page.getByRole("status").filter({ hasText: "内容已更新" }),
  ).toBeVisible({ timeout: 12000 });
  await expect(
    page.getByLabel("只读代码").filter({ visible: true }),
  ).toContainText("unchanged line");
  await expect(
    page
      .getByRole("button", { name: "发送位置到终端" })
      .filter({ visible: true }),
  ).toBeDisabled();
  await page
    .getByRole("button", { name: "刷新文档" })
    .filter({ visible: true })
    .click();
  await expect(
    page.getByLabel("只读代码").filter({ visible: true }),
  ).toContainText("new version");
  await expect(
    page.getByLabel("位置预览").filter({ visible: true }),
  ).toHaveText(join(workDir, "clean.go") + " ");
  await page
    .getByRole("button", { name: "文件内查找" })
    .filter({ visible: true })
    .click();
  await expect(page.locator(".cm-search:visible")).toBeVisible();
  expect(await readFile(join(workDir, "clean.go"), "utf8")).toBe(
    "new version\n",
  );
  await expect
    .poll(() =>
      page.evaluate(() => document.documentElement.scrollWidth <= innerWidth),
    )
    .toBe(true);
});

test("frozen Git diff, ignored navigation and complete source documents", async ({
  page,
  workDir,
}) => {
  git(workDir, "init", "-q");
  const original =
    Array.from({ length: 120 }, (_, i) => `ordinary line ${i + 1}`).join("\n") +
    "\n";
  await writeFile(join(workDir, "code.txt"), original);
  await writeFile(join(workDir, ".gitignore"), ".env\nnode_modules/\n");
  await writeFile(join(workDir, ".env"), "hidden default");
  await mkdir(join(workDir, "node_modules"));
  await writeFile(join(workDir, "node_modules/dependency"), "ignored");
  git(workDir, "add", ".");
  git(workDir, "commit", "-qm", "initial");
  const indexed = original.replace("ordinary line 50\n", "indexed line 50\n");
  await writeFile(join(workDir, "code.txt"), indexed);
  git(workDir, "add", "code.txt");
  await writeFile(
    join(workDir, "code.txt"),
    indexed.replace("ordinary line 80\n", "working line 80\n"),
  );
  await writeFile(join(workDir, "untracked.txt"), "untracked content\n");
  await createShell(page, "Git code", workDir);
  await showCode(page);
  await expect(
    page.getByRole("button", { name: ".env", exact: true }),
  ).toHaveCount(0);
  await page.getByLabel("显示忽略项").check();
  await expect(page.getByRole("button", { name: /.env · 忽略/ })).toBeVisible();
  await page.getByRole("tab", { name: "修改", exact: true }).click();
  await page
    .getByRole("button", { name: "M code.txt", exact: true })
    .first()
    .click();
  await expect(
    page.getByLabel("只读代码").filter({ visible: true }),
  ).toContainText("working line 80");
  await writeFile(join(workDir, "code.txt"), "external replacement\n");
  await page
    .getByRole("button", { name: "展开全部", exact: true })
    .first()
    .click();
  await expect(
    page.getByLabel("只读代码").filter({ visible: true }),
  ).toContainText("ordinary line 1");
  await expect(
    page.getByLabel("只读代码").filter({ visible: true }),
  ).not.toContainText("external replacement");
  await page.getByRole("button", { name: "旧侧完整文件" }).click();
  await page
    .getByRole("button", { name: "跳转行" })
    .filter({ visible: true })
    .click();
  await page.locator(".cm-goto-line:visible input").fill("50");
  await page.locator(".cm-goto-line:visible input").press("Enter");
  await expect(
    page.getByLabel("只读代码").filter({ visible: true }),
  ).toContainText("indexed line 50");
  await expect(
    page.getByLabel("只读代码").filter({ visible: true }),
  ).not.toContainText("working line 80");
  await expect(
    page.getByLabel("位置预览").filter({ visible: true }),
  ).toContainText("（索引版本）");
  await nav(page);
  await page
    .getByRole("button", { name: "M code.txt", exact: true })
    .last()
    .click();
  await expect(
    page.getByLabel("只读代码").filter({ visible: true }),
  ).toContainText("indexed line 50");
  await page.getByRole("button", { name: "旧侧完整文件" }).click();
  await expect(
    page.getByLabel("位置预览").filter({ visible: true }),
  ).toContainText("（HEAD 旧版本）");
});

test("main and linked worktrees bind their own roots and reject cross-session snapshots", async ({
  page,
  request,
  workDir,
}) => {
  const main = join(workDir, "main"),
    linked = join(workDir, "linked");
  await mkdir(main);
  git(main, "init", "-q");
  await mkdir(join(main, "src"));
  await writeFile(join(main, "src/clean.txt"), "main source\n");
  git(main, "add", ".");
  git(main, "commit", "-qm", "main");
  git(main, "worktree", "add", "-qb", "linked", linked);
  await writeFile(join(linked, "src/clean.txt"), "linked source\n");
  const first = await createShell(page, "Main root", join(main, "src"));
  await showCode(page);
  await page.getByLabel("搜索文件路径").fill("clean");
  await page.getByRole("button", { name: "搜索路径", exact: true }).click();
  await page
    .getByRole("button", { name: "src/clean.txt", exact: true })
    .click();
  await expect(
    page.getByLabel("只读代码").filter({ visible: true }),
  ).toContainText("main source");
  const context = await (
    await request.get(`/api/sessions/${first}/code/context`)
  ).json();
  expect(context.rootPath).toBe(main);
  const file = await (
    await request.get(
      `/api/sessions/${first}/code/file?${new URLSearchParams({ contextId: context.contextId, nodeId: Buffer.from("src/clean.txt").toString("base64url") })}`,
    )
  ).json();
  const second = await createShell(page, "Linked root", join(linked, "src"));
  await showCode(page);
  await page.getByLabel("搜索文件路径").fill("clean");
  await page.getByRole("button", { name: "搜索路径", exact: true }).click();
  await page
    .getByRole("button", { name: "src/clean.txt", exact: false })
    .click();
  await expect(
    page.getByLabel("只读代码").filter({ visible: true }),
  ).toContainText("linked source");
  await expect(
    page.getByLabel("位置预览").filter({ visible: true }),
  ).toHaveText(join(linked, "src/clean.txt") + " ");
  expect(
    (
      await request.get(
        `/api/sessions/${second}/code/snapshots/${file.snapshotId}/file?side=new`,
      )
    ).status(),
  ).toBe(400);
  await expect(page.locator(".xterm")).toHaveCount(1);
});
