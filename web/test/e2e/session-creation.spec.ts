import { execFile } from "node:child_process";
import { promisify } from "node:util";
import { mkdir } from "node:fs/promises";
import { join, resolve } from "node:path";
import type { APIRequestContext, Page } from "@playwright/test";
import { test, expect } from "./fixtures";

const exec = promisify(execFile);

async function register(request: APIRequestContext, path: string) {
  await mkdir(path);
  await exec("git", ["-C", path, "init", "-b", "main"]);
  await exec("git", [
    "-C",
    path,
    "-c",
    "user.name=QA",
    "-c",
    "user.email=qa@example.invalid",
    "commit",
    "--allow-empty",
    "-m",
    "initial",
  ]);
  const response = await request.post("/api/repositories", { data: { path } });
  expect(response.status()).toBe(201);
  return response.json();
}

async function selectScope(page: Page, id: string) {
  await expect(page.locator('[data-repository-scope="all"]')).toHaveCount(1);
  const navigation = page.getByRole("navigation", { name: "仓库导航" });
  if (!(await navigation.isVisible()))
    await page.getByRole("button", { name: /仓库导航：/ }).click();
  await page.locator(`[data-repository-scope="${id}"]`).click();
}

test("global and worktree creation use the selected repository instead of an unrelated recent directory", async ({
  page,
  request,
  workDir,
}) => {
  await register(request, join(workDir, "first"));
  const second = await register(request, join(workDir, "second"));
  await page.addInitScript(
    (path) => localStorage.setItem("webmux.recentDirs", JSON.stringify([path])),
    workDir,
  );
  await page.goto("/");
  await selectScope(page, second.id);
  await page
    .locator("header")
    .getByRole("button", { name: "＋ 新建会话", exact: true })
    .click();
  await expect(
    page.getByRole("tab", { name: "已有工作树", exact: true }),
  ).toHaveAttribute("aria-selected", "true");
  await expect(
    page.getByRole("combobox", { name: "仓库", exact: true }),
  ).toContainText("second");
  await expect(
    page.getByRole("combobox", { name: "工作树", exact: true }),
  ).toContainText("main");
  await page.getByRole("button", { name: "取消", exact: true }).click();
  expect(await (await request.get("/api/sessions")).json()).toEqual([]);
  const group = page.locator(`[data-worktree-id="${second.worktrees[0].id}"]`);
  await group.getByRole("button", { name: "新建会话", exact: true }).click();
  await expect(
    page.getByRole("combobox", { name: "仓库", exact: true }),
  ).toContainText("second");
  await page.getByRole("tab", { name: "手动填写", exact: true }).click();
  await page.getByLabel("自定义启动命令", { exact: true }).fill("/bin/sh -i");
  await page.getByRole("button", { name: "创建并进入 →", exact: true }).click();
  await expect(page).toHaveURL(/\/s\/[a-z0-9]+$/);
  const sessions = await (await request.get("/api/sessions")).json();
  expect(sessions).toHaveLength(1);
  expect(sessions[0]).toMatchObject({
    cwd: second.anchorPath,
    workspace: { repositoryId: second.id, worktreeId: second.worktrees[0].id },
  });
});

test("worktree management creates only a worktree even when launch configs are unavailable", async ({
  page,
  request,
  workDir,
}) => {
  const repo = await register(request, join(workDir, "project"));
  await page.route("**/api/launch-configs", (route) =>
    route.fulfill({ status: 503, json: { error: "launch store offline" } }),
  );
  await page.goto("/");
  await expect(
    page.getByRole("button", { name: "新建 worktree", exact: true }),
  ).toHaveCount(0);
  await page
    .getByRole("button", { name: "仓库菜单 project", exact: true })
    .click();
  await page
    .getByRole("menuitem", { name: "新建工作树…", exact: true })
    .click();
  await expect(
    page.getByRole("checkbox", { name: "创建后启动会话", exact: true }),
  ).not.toBeChecked();
  await expect(
    page.getByRole("tab", { name: "手动填写", exact: true }),
  ).toHaveCount(0);
  const branch = page.getByLabel("新分支名称", { exact: true });
  await expect(branch).not.toHaveValue("");
  await branch.fill("feature/prepared-only");
  await page.getByRole("button", { name: "创建工作树", exact: true }).click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await expect(page).toHaveURL(/\/$/);
  expect(await (await request.get("/api/sessions")).json()).toEqual([]);
  const repositories = await (await request.get("/api/repositories")).json();
  expect(
    repositories.find((item: { id: string }) => item.id === repo.id).worktrees,
  ).toHaveLength(2);
});

test("full split creation uses saved configs and keeps the launch subdirectory and original terminal", async ({
  page,
  request,
  workDir,
}) => {
  const repo = await register(request, join(workDir, "project"));
  const cwd = join(repo.anchorPath, "packages", "app");
  await mkdir(cwd, { recursive: true });
  const response = await request.post("/api/sessions", {
    data: { cwd, cmd: ["/bin/sh", "-i"], name: "Original" },
  });
  const original = await response.json();
  expect(original.workspace.repositoryId).toBe(repo.id);
  const configResponse = await request.post("/api/launch-configs", {
    data: {
      name: "Project shell",
      cmd: ["/bin/sh", "-i"],
      env: { SPLIT_TEST: "saved" },
    },
  });
  expect(configResponse.status()).toBe(201);
  const config = await configResponse.json();
  await page.addInitScript(
    (path) => localStorage.setItem("webmux.recentDirs", JSON.stringify([path])),
    workDir,
  );
  await page.goto(`/s/${original.id}`);
  const pane = page.locator(`[data-session-id="${original.id}"]`);
  await expect(pane.locator(".xterm-helper-textarea")).toBeAttached();
  await pane
    .locator(".xterm")
    .evaluate((element) => element.setAttribute("data-retained", "yes"));
  const open = async () => {
    await pane.getByRole("button", { name: "分屏", exact: true }).click();
    await page
      .getByRole("menuitem", { name: "新建会话…", exact: true })
      .click();
    await expect(page.getByLabel("工作目录", { exact: true })).toHaveValue(cwd);
  };
  await open();
  await page.getByRole("button", { name: "取消", exact: true }).click();
  expect(await (await request.get("/api/sessions")).json()).toHaveLength(1);
  await open();
  await page.getByRole("tab", { name: "预先输入", exact: true }).click();
  await page
    .getByRole("combobox", { name: "选择启动配置", exact: true })
    .click();
  await page
    .getByRole("option", { name: "Project shell", exact: true })
    .click();
  await page.getByRole("tab", { name: "上下分屏", exact: true }).click();
  await page
    .getByRole("button", { name: "创建并加入分屏 →", exact: true })
    .click();
  await expect(page.locator(".split-terminal")).toHaveCount(2);
  await expect(page).toHaveURL(new RegExp(`/s/${original.id}$`));
  await expect(pane.locator(".xterm")).toHaveAttribute("data-retained", "yes");
  const sessions = await (await request.get("/api/sessions")).json();
  expect(
    sessions.find((item: { id: string }) => item.id !== original.id),
  ).toMatchObject({
    cwd,
    configId: config.id,
    workspace: { repositoryId: repo.id },
  });
  await page
    .locator(".split-terminal")
    .last()
    .getByRole("button", { name: "分屏", exact: true })
    .click();
  await page.getByRole("menuitem", { name: "左右分屏 · 新建 Shell" }).click();
  await expect(page.locator(".split-terminal")).toHaveCount(3);
  const all = await (await request.get("/api/sessions")).json();
  expect(all.every((item: { cwd: string }) => item.cwd === cwd)).toBe(true);
});

test("split creation retains a new worktree across repository changes after command failure", async ({
  page,
  request,
  workDir,
}, testInfo) => {
  const repo = await register(request, join(workDir, "project"));
  await register(request, join(workDir, "other"));
  const response = await request.post("/api/sessions", {
    data: { cwd: repo.anchorPath, cmd: ["/bin/sh", "-i"], name: "Original" },
  });
  const original = await response.json();
  await page.goto(`/s/${original.id}`);
  await page.getByRole("button", { name: "分屏", exact: true }).click();
  await page.getByRole("menuitem", { name: "新建会话…", exact: true }).click();
  await page.getByRole("tab", { name: "新工作树", exact: true }).click();
  const repository = page.getByRole("combobox", { name: "仓库", exact: true });
  await repository.click();
  await page.getByRole("option", { name: "project", exact: true }).click();
  const branch = page.getByLabel("新分支名称", { exact: true });
  await expect(branch).not.toHaveValue("");
  await branch.fill("feature/retry-target");
  const pathInput = page.getByLabel("新工作树完整路径", { exact: true });
  await expect(pathInput).toHaveValue(/\/retry-target$/);
  const path = await pathInput.inputValue();
  await page.getByRole("tab", { name: "手动填写", exact: true }).click();
  await page
    .getByLabel("自定义启动命令", { exact: true })
    .fill("/webmux-missing-agent");
  await page
    .getByRole("button", { name: "创建并加入分屏 →", exact: true })
    .click();
  await expect(page.getByRole("alert")).toContainText("webmux-missing-agent");
  await expect(
    page.getByRole("tab", { name: "已有工作树", exact: true }),
  ).toHaveAttribute("aria-selected", "true");
  await repository.click();
  await page.getByRole("option", { name: "other", exact: true }).click();
  await repository.click();
  await page.getByRole("option", { name: "project", exact: true }).click();
  await page.getByRole("combobox", { name: "工作树", exact: true }).click();
  const retained = page.getByRole("option", {
    name: "feature/retry-target",
    exact: true,
  });
  await expect(retained).toBeVisible();
  const evidence = process.env.WEBMUX_SESSION_RECOVERY_EVIDENCE_DIR;
  if (evidence) {
    await mkdir(resolve(evidence), { recursive: true });
    for (const theme of ["light", "dark"]) {
      await page.evaluate(async (theme) => {
        document.documentElement.classList.toggle("dark", theme === "dark");
        await document.fonts.ready;
      }, theme);
      await page.screenshot({
        path: resolve(
          evidence,
          `recovery-${testInfo.project.name}-${theme}.jpg`,
        ),
        type: "jpeg",
        quality: 75,
        scale: "css",
        fullPage: true,
        animations: "disabled",
      });
    }
  }
  await retained.click();
  await page.getByLabel("自定义启动命令", { exact: true }).fill("/bin/sh -i");
  await page
    .getByRole("button", { name: "创建并加入分屏 →", exact: true })
    .click();
  await expect(page.locator(".split-terminal")).toHaveCount(2);
  await expect(page).toHaveURL(new RegExp(`/s/${original.id}$`));
  const sessions = await (await request.get("/api/sessions")).json();
  expect(sessions).toHaveLength(2);
  expect(
    sessions.find((item: { id: string }) => item.id !== original.id),
  ).toMatchObject({ cwd: path, workspace: { repositoryId: repo.id } });
  const repositories = await (await request.get("/api/repositories")).json();
  expect(
    repositories.find((item: { id: string }) => item.id === repo.id).worktrees,
  ).toHaveLength(2);
});
