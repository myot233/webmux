import { execFile } from "node:child_process";
import { promisify } from "node:util";
import { mkdir } from "node:fs/promises";
import { join, resolve } from "node:path";
import type { Page } from "@playwright/test";
import { test, expect } from "./fixtures";
import { repositoryMenu, worktreeMenu } from "./repository-menu";

const exec = promisify(execFile);

// The workbench tree: in the sidebar on desktop, in the drawer on phones.
async function openNavigation(page: Page) {
  const tree = page.getByRole("tree", { name: "仓库与会话" });
  const open = page.getByRole("button", { name: "打开侧栏" });
  await expect(tree.or(open).first()).toBeVisible();
  if (!(await tree.isVisible())) await open.click();
  await expect(tree).toBeVisible();
  return tree;
}

async function selectRepository(page: Page, name: RegExp) {
  const tree = await openNavigation(page);
  await tree.getByRole("treeitem", { name }).click();
  // The phone drawer closes after a pick; let it finish before the next.
  await expect(page.getByRole("dialog", { name: "导航" })).toHaveCount(0);
}

test("repository scope limits the home list, composes with filters and survives navigation", async ({
  page,
  request,
  workDir,
}, testInfo) => {
  const repositories = [];
  for (const name of ["webmux", "api-server", "mobile-app"]) {
    const path = join(workDir, name);
    await mkdir(path);
    await exec("git", ["-C", path, "init", "-b", "main"]);
    await exec("git", [
      "-C",
      path,
      "-c",
      "user.name=WebMux QA",
      "-c",
      "user.email=qa@example.invalid",
      "commit",
      "--allow-empty",
      "-m",
      "initial",
    ]);
    const response = await request.post("/api/repositories", {
      data: { path },
    });
    expect(response.status()).toBe(201);
    repositories.push(await response.json());
  }
  const sessions = [];
  for (const [index, repo] of repositories.entries()) {
    const response = await request.post("/api/sessions", {
      data: {
        cmd: ["/bin/sh", "-c", "printf 'Building project…\\n'; sleep 600"],
        name: ["Terminal reconnect", "API review", "Mobile layout"][index],
        workspace: { repositoryId: repo.id, worktreeId: repo.worktrees[0].id },
      },
    });
    expect(response.status()).toBe(201);
    sessions.push((await response.json()).session);
  }
  const direct = await request.post("/api/sessions", {
    data: {
      cwd: workDir,
      cmd: ["/bin/sh", "-c", "sleep 600"],
      name: "Scratch shell",
    },
  });
  expect(direct.status()).toBe(201);
  await page.goto("/");
  await expect(page.getByLabel("正在加载会话")).toHaveCount(0);
  const evidence = process.env.WEBMUX_SCOPE_EVIDENCE_DIR;
  if (evidence) {
    await mkdir(resolve(evidence), { recursive: true });
    for (const theme of ["light", "dark"]) {
      await page.evaluate(async (theme) => {
        document.documentElement.classList.toggle("dark", theme === "dark");
        await document.fonts.ready;
      }, theme);
      await page.screenshot({
        path: resolve(evidence, `${testInfo.project.name}-${theme}.png`),
        fullPage: true,
        animations: "disabled",
      });
    }
  }

  // The current scope row of the tree (the drawer's tree while it is open).
  const scope = page.locator('[role="treeitem"][aria-current="page"]').last();
  const sidebar = page.getByRole("complementary", { name: "侧栏" });
  const toggle = page.getByRole("button", { name: "打开侧栏" });
  if (testInfo.project.name === "chromium-desktop") {
    await expect(sidebar).toBeVisible();
    const sidebarBox = await sidebar.boundingBox();
    const contentBox = await page.locator("main").boundingBox();
    expect(sidebarBox!.x + sidebarBox!.width).toBeLessThanOrEqual(
      contentBox!.x,
    );
    await expect(toggle).toBeHidden();
  } else {
    await expect(sidebar).toBeHidden();
    await openNavigation(page);
    if (evidence)
      await page.screenshot({
        path: resolve(evidence, "chromium-mobile-navigation.png"),
        fullPage: true,
        animations: "disabled",
      });
    await page.keyboard.press("Escape");
    await expect(page.getByRole("tree", { name: "仓库与会话" })).toBeHidden();
  }
  await expect(page.locator("[data-repository-id]:visible")).toHaveCount(1);
  await expect(page.locator("[data-session-id]:visible")).toHaveCount(1);
  await expect(
    page.getByRole("status").filter({ hasText: "个仓库" }),
  ).toHaveText("1 个仓库 · 1 个会话");
  const repo = page.locator("[data-repository-id]:visible");
  await expect(repo.getByText("主工作区", { exact: true })).toBeHidden();
  await expect(repo.getByText("已跟踪文件干净", { exact: true })).toBeHidden();
  await repositoryMenu(page, "仓库详情…");
  // A closed phone drawer stays in the DOM; only the open dialog counts.
  const dialog = page.locator('[role="dialog"][data-state="open"]');
  await expect(dialog).toContainText("读取于");
  await expect(dialog).toContainText("上次获取");
  // The panel's sections read branches and remotes locally on first view.
  await dialog.getByRole("tab", { name: "分支" }).click();
  await expect(dialog.getByRole("region", { name: "本地分支" })).toBeVisible();
  await dialog.getByRole("tab", { name: "远程" }).click();
  await expect(dialog).toContainText("没有远程");
  await page.keyboard.press("Escape");
  await expect(dialog).toHaveCount(0);
  await repo.getByRole("button", { name: "工作树详情 main" }).click();
  await expect(dialog).toContainText("已跟踪文件干净");
  await expect(dialog.getByRole("button", { name: "计算容量" })).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(dialog).toHaveCount(0);
  // The sidebar's worktree menu opens the same details.
  await worktreeMenu(page, "工作树详情…", "main");
  await expect(dialog).toContainText("已跟踪文件干净");
  await expect(dialog).toContainText("分支与上游");
  await page.keyboard.press("Escape");
  await expect(dialog).toHaveCount(0);
  await expect(page.locator('[aria-label="终端输出预览"]:visible')).toHaveCount(
    1,
  );
  const views = page.getByRole("radiogroup", { name: "会话显示方式" });
  await expect(views.getByRole("radio", { name: "输出预览" })).toHaveAttribute(
    "aria-checked",
    "true",
  );
  await views.getByRole("radio", { name: "紧凑列表" }).click();
  await expect(page.locator('[aria-label="终端输出预览"]:visible')).toHaveCount(
    0,
  );
  await page.reload();
  await expect(views.getByRole("radio", { name: "紧凑列表" })).toHaveAttribute(
    "aria-checked",
    "true",
  );
  await expect(page.locator('[aria-label="终端输出预览"]:visible')).toHaveCount(
    0,
  );
  await selectRepository(page, /^api-server/);
  await expect(scope).toContainText("api-server");
  await expect(page.getByRole("link", { name: "API review" })).toBeVisible();
  await expect(page.getByRole("link", { name: "Scratch shell" })).toHaveCount(
    0,
  );

  const search = page.getByRole("searchbox", { name: "搜索会话" });
  await search.fill("Mobile layout");
  await expect(page.getByText("没有匹配的会话", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "清除筛选", exact: true }).click();
  const filters = page.getByRole("radiogroup", { name: "按会话状态筛选" });
  await filters.getByRole("radio", { name: /已退出/ }).click();
  await expect(page.locator("[data-session-id]:visible")).toHaveCount(0);
  expect(
    (await request.delete(`/api/sessions/${sessions[1].id}`)).status(),
  ).toBe(204);
  await expect(page.getByRole("link", { name: "API review" })).toBeVisible();
  await page.getByRole("button", { name: "清除筛选", exact: true }).click();
  await page.reload();
  await expect(scope).toContainText("api-server");
  await expect(page.locator('[aria-label="终端输出预览"]:visible')).toHaveCount(
    0,
  );
  await expect(page.locator("[data-repository-id]:visible")).toHaveCount(1);
  await page.getByRole("link", { name: "API review" }).click();
  await expect(page).toHaveURL(new RegExp(`/s/${sessions[1].id}$`));
  await page.goto("/");
  await expect(scope).toContainText("api-server");

  // Arrow keys move between rows; left folds an open repository.
  const nav = await openNavigation(page);
  await nav.getByRole("treeitem", { name: /^mobile-app/ }).focus();
  await page.keyboard.press("ArrowLeft");
  await expect(
    nav.getByRole("treeitem", { name: /^mobile-app/ }),
  ).toHaveAttribute("aria-expanded", "false");
  await page.keyboard.press("ArrowDown");
  await expect(nav.getByRole("treeitem", { name: /^直接会话/ })).toBeFocused();
  await page.keyboard.press("Enter");
  if (testInfo.project.name !== "chromium-desktop")
    await expect(nav).toBeHidden();
  await expect(page.locator("[data-repository-id]:visible")).toHaveCount(0);
  await expect(page.getByRole("link", { name: "Scratch shell" })).toBeVisible();
  await expect(
    page.getByRole("status").filter({ hasText: "个仓库" }),
  ).toHaveText("0 个仓库 · 1 个会话");
  await selectRepository(page, /^全部仓库/);
  await expect(page.locator("[data-repository-id]:visible")).toHaveCount(3);
  await expect(page.locator("[data-session-id]:visible")).toHaveCount(4);
  await expect(
    page.getByRole("status").filter({ hasText: "个仓库" }),
  ).toHaveText("3 个仓库 · 4 个会话");
  if (evidence) {
    await page.screenshot({
      path: resolve(evidence, `${testInfo.project.name}-all-compact.png`),
      fullPage: true,
      animations: "disabled",
    });
  }
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);

  await selectRepository(page, /^api-server/);
  expect(
    (await request.delete(`/api/repositories/${repositories[1].id}`)).status(),
  ).toBe(204);
  await page.getByRole("button", { name: "刷新 Git 状态" }).click();
  await expect(scope).toContainText("全部仓库");
  await expect(page.getByRole("link", { name: "API review" })).toBeVisible();
  await page.setViewportSize({ width: 320, height: 700 });
  await page.evaluate(() => {
    document.documentElement.style.fontSize = "200%";
  });
  const overflow = await page.evaluate(() =>
    [...document.querySelectorAll("main *, aside *")]
      .filter((node) => node.getBoundingClientRect().right > innerWidth + 1)
      .map((node) => `${node.tagName}.${node.className}`)
      .slice(0, 8),
  );
  expect(
    await page.evaluate(() => document.documentElement.scrollWidth),
    JSON.stringify(overflow),
  ).toBeLessThanOrEqual(321);
  const drawer = await openNavigation(page);
  await expect(
    drawer.getByRole("treeitem", { name: /^直接会话/ }),
  ).toBeVisible();
  expect(
    await page.evaluate(() => document.documentElement.scrollWidth),
  ).toBeLessThanOrEqual(321);
});

test("an unknown saved repository falls back to the full home list", async ({
  page,
  request,
  workDir,
}) => {
  await page.addInitScript(() =>
    localStorage.setItem("webmux.homeRepository", "removed-repository"),
  );
  const response = await request.post("/api/sessions", {
    data: {
      cwd: workDir,
      cmd: ["/bin/sh", "-c", "sleep 600"],
      name: "Available shell",
    },
  });
  expect(response.status()).toBe(201);
  await page.goto("/");
  await expect(
    page.getByRole("link", { name: "Available shell" }),
  ).toBeVisible();
  // The tree keeps only the full view and direct sessions.
  const tree = await openNavigation(page);
  await expect(
    tree.getByRole("treeitem", { name: /^全部仓库/ }),
  ).toHaveAttribute("aria-current", "page");
});

test("repository refresh failure keeps direct sessions reachable and preserves the saved choice", async ({
  page,
  request,
  workDir,
}) => {
  await page.addInitScript(() =>
    localStorage.setItem("webmux.homeRepository", "saved-repository"),
  );
  let requests = 0;
  await page.route("**/api/repositories", async (route) => {
    requests += 1;
    // Hold the retry so the assertion observes the in-flight refresh, when the
    // previous failure notice is already cleared but no new list exists yet.
    if (requests > 1) await new Promise((resolve) => setTimeout(resolve, 500));
    await route.fulfill({
      status: 503,
      json: { error: "Registry unavailable" },
    });
  });
  const response = await request.post("/api/sessions", {
    data: {
      cwd: workDir,
      cmd: ["/bin/sh", "-c", "sleep 600"],
      name: "Fallback shell",
    },
  });
  expect(response.status()).toBe(201);
  await page.goto("/");
  await expect(
    page.getByRole("link", { name: "Fallback shell" }),
  ).toBeVisible();
  await expect(page.getByRole("alert")).toContainText("Registry unavailable");
  expect(
    await page.evaluate(() => localStorage.getItem("webmux.homeRepository")),
  ).toBe("saved-repository");
  await page.evaluate(() => window.dispatchEvent(new Event("pageshow")));
  await expect(page.getByRole("alert")).toHaveCount(0);
  expect(
    await page.evaluate(() => localStorage.getItem("webmux.homeRepository")),
  ).toBe("saved-repository");
  await expect(page.getByRole("alert")).toContainText("Registry unavailable");
  expect(
    await page.evaluate(() => localStorage.getItem("webmux.homeRepository")),
  ).toBe("saved-repository");
});
