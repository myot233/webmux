import { execFileSync } from "node:child_process";
import { mkdir, readFile } from "node:fs/promises";
import { join } from "node:path";
import type { APIRequestContext, Page } from "@playwright/test";
import {
  createShell,
  expect,
  expectConnected,
  headerAction,
  openPaneMenu,
  recordInput,
  test,
} from "./fixtures";

const switcher = (page: Page) =>
  page.getByRole("dialog", { name: "快速切换会话", exact: true });
const trigger = (page: Page) =>
  page.getByRole("button", { name: "快速切换会话", exact: true });

async function shell(
  request: APIRequestContext,
  data: {
    name: string;
    cwd?: string;
    workspace?: { repositoryId: string; worktreeId: string };
  },
) {
  const response = await request.post("/api/sessions", {
    data: { cmd: ["/bin/bash", "--noprofile", "--norc", "-i"], ...data },
  });
  expect(response.status()).toBe(201);
  return (await response.json()).session.id as string;
}

async function repoSession(request: APIRequestContext, workDir: string) {
  const path = join(workDir, "sample-repo");
  await mkdir(path);
  execFileSync("git", ["init", "-q", "-b", "release-candidate", path]);
  execFileSync("git", [
    "-C",
    path,
    "-c",
    "user.name=WebMux QA",
    "-c",
    "user.email=qa@example.invalid",
    "commit",
    "--allow-empty",
    "-qm",
    "Seed",
  ]);
  const response = await request.post("/api/repositories", { data: { path } });
  expect(response.status()).toBe(201);
  const repository = await response.json();
  return shell(request, {
    name: "Release shell",
    workspace: {
      repositoryId: repository.id,
      worktreeId: repository.worktrees[0].id,
    },
  });
}

test("searches beyond the workbench scope and follows live session events", async ({
  page,
  request,
  workDir,
}) => {
  const target = await repoSession(request, workDir);
  await page.addInitScript(() =>
    localStorage.setItem("webmux.homeRepository", "direct"),
  );
  await page.goto("/");
  await expect(page.getByRole("main")).toBeVisible();
  await expect(
    page.getByRole("main").getByRole("link", { name: "Release shell" }),
  ).toHaveCount(0);
  await trigger(page).click();

  const dialog = switcher(page);
  const search = dialog.getByRole("combobox");
  await expect(dialog).toBeInViewport({ ratio: 1 });
  await expect(search).toBeFocused();
  await search.fill("New elsewhere");
  await expect(dialog.getByText(/找不到匹配/)).toBeVisible();
  await shell(request, { name: "New elsewhere", cwd: workDir });
  await expect(
    dialog.getByRole("option", { name: /New elsewhere/ }),
  ).toBeVisible();
  await search.fill("release-candidate");
  await expect(
    dialog.getByRole("option", { name: /Release shell/ }),
  ).toContainText("sample-repo · release-candidate");
  await expect(dialog.getByRole("option")).toHaveCount(1);
  await search.press("ArrowDown");
  await search.press("Enter");
  await expect(page).toHaveURL(new RegExp(`/s/${target}$`));
  await expectConnected(page);
});

test("settings can switch sessions while repository metadata loads", async ({
  page,
  request,
  workDir,
}) => {
  const target = await repoSession(request, workDir);
  const finished = await request.post("/api/sessions", {
    data: {
      name: "Finished task",
      cwd: workDir,
      cmd: ["/bin/sh", "-c", "exit 0"],
    },
  });
  expect(finished.status()).toBe(201);
  await page.goto("/settings");
  const metadata = Promise.withResolvers<void>();
  await page.route(
    "**/api/repositories",
    async (route) => {
      await metadata.promise;
      await route.continue();
    },
    { times: 1 },
  );
  const dialog = switcher(page);
  const search = dialog.getByRole("combobox");
  const result = dialog.getByRole("option", { name: /Release shell/ });
  try {
    await trigger(page).click();
    await expect(
      dialog.getByRole("option", { name: /Finished task.*已退出/ }),
    ).toBeVisible();
    await search.fill("Release shell");
    await expect(result).toBeVisible();
    await search.fill("release-candidate");
    await expect(dialog.getByText("正在读取仓库和分支…")).toBeVisible();
  } finally {
    metadata.resolve();
  }
  await expect(result).toBeVisible();
  await result.click();
  await expect(page).toHaveURL(new RegExp(`/s/${target}$`));
});

test("shortcut preserves Shell input and restores focus when dismissed", async ({
  page,
  workDir,
}) => {
  await createShell(page, "Keep working", workDir);
  const recording = await recordInput(page, workDir);
  const terminal = page.locator(".xterm-helper-textarea");
  const keys = await trigger(page).getAttribute("aria-keyshortcuts");
  expect(keys).toBeTruthy();
  await terminal.press("Control+k");
  await expect.poll(() => readFile(recording, "utf8")).toBe("\x0b");

  await page.keyboard.press(keys!);
  const dialog = switcher(page);
  await expect(dialog).toBeVisible();
  const search = dialog.getByRole("combobox");
  await search.pressSequentially("palette-only");
  await page.keyboard.press("Escape");
  await expect(dialog).toBeHidden();
  await expect(terminal).toBeFocused();
  expect(await readFile(recording, "utf8")).toBe("\x0b");

  await (await headerAction(page, "快捷键")).click();
  await page.keyboard.press(keys!);
  await expect(dialog).toBeHidden();
  await expect(
    page.getByRole("dialog", { name: "快捷键", exact: true }),
  ).toBeVisible();
});

test("a session already in a split activates its pane in place", async ({
  page,
  request,
  workDir,
}) => {
  const original = await createShell(page, "Original pane", workDir);
  const target = await shell(request, { name: "Other pane", cwd: workDir });
  await openPaneMenu(page);
  await page.getByRole("menuitem", { name: "加入已有会话…" }).click();
  await page
    .getByRole("dialog")
    .getByRole("button", { name: /Other pane/ })
    .click();
  const targetPane = page.locator(`[data-session-id="${target}"]`);
  await expect(targetPane).toHaveAttribute("data-active", "true");
  await targetPane
    .getByRole("button", { name: "快速切换会话", exact: true })
    .click();
  await switcher(page)
    .getByRole("option", { name: /Original pane/ })
    .click();
  await expect(page.locator(`[data-session-id="${original}"]`)).toHaveAttribute(
    "data-active",
    "true",
  );
  await expect(page.locator(".split-terminal")).toHaveCount(2);
  await expect(page).toHaveURL(new RegExp(`/s/${original}$`));
});
