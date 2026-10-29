import { execFile } from "node:child_process";
import { promisify } from "node:util";
import { mkdir } from "node:fs/promises";
import { join, resolve } from "node:path";
import type { Page } from "@playwright/test";
import { test, expect } from "./fixtures";

const exec = promisify(execFile);

async function openNavigation(page: Page) {
  const navigation = page.getByRole("navigation", { name: "仓库导航" });
  await expect(page.locator('[data-repository-scope="all"]')).toHaveCount(1);
  if (!(await navigation.isVisible()))
    await page.getByRole("button", { name: /仓库导航：/ }).click();
  await expect(navigation).toBeVisible();
  return navigation;
}

async function selectRepository(page: Page, name: RegExp) {
  const navigation = await openNavigation(page);
  await navigation.getByRole("button", { name }).click();
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
    sessions.push(await response.json());
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

  const scope = page.locator('[data-repository-scope][aria-pressed="true"]');
  const navigation = page.getByRole("navigation", { name: "仓库导航" });
  const toggle = page.getByRole("button", { name: /仓库导航：/ });
  if (testInfo.project.name === "chromium-desktop") {
    await expect(navigation).toBeVisible();
    const sidebarBox = await navigation.boundingBox();
    const contentBox = await page.locator("main").boundingBox();
    expect(sidebarBox!.x + sidebarBox!.width).toBeLessThanOrEqual(
      contentBox!.x,
    );
    await expect(toggle).toBeHidden();
  } else {
    await expect(navigation).toBeHidden();
    await expect(toggle).toHaveAttribute("aria-expanded", "false");
    await openNavigation(page);
    if (evidence)
      await page.screenshot({
        path: resolve(evidence, "chromium-mobile-navigation.png"),
        fullPage: true,
        animations: "disabled",
      });
    await toggle.click();
    await expect(navigation).toBeHidden();
  }
  await expect(page.locator("[data-repository-id]:visible")).toHaveCount(1);
  await expect(page.locator("[data-session-id]:visible")).toHaveCount(1);
  await expect(
    page.getByRole("status").filter({ hasText: "个仓库" }),
  ).toHaveText("1 个仓库 · 1 个会话");
  const repo = page.locator("[data-repository-id]:visible");
  await expect(repo.locator('[data-slot="repository-details"]')).toHaveCount(0);
  await expect(repo.getByText("主工作区", { exact: true })).toBeHidden();
  await expect(repo.getByText("已跟踪文件干净", { exact: true })).toBeHidden();
  await repo.getByRole("button", { name: /仓库菜单/ }).click();
  await page.getByRole("menuitem", { name: "仓库详情", exact: true }).click();
  await expect(repo.locator('[data-slot="repository-details"]')).toContainText(
    "Git 读取时间",
  );
  await repo.getByRole("button", { name: /仓库菜单/ }).click();
  await page
    .getByRole("menuitem", { name: "收起仓库详情", exact: true })
    .click();
  await expect(repo.locator('[data-slot="repository-details"]')).toHaveCount(0);
  await repo.getByRole("button", { name: "工作树详情 main" }).click();
  await expect(repo.getByText("已跟踪文件干净", { exact: true })).toBeVisible();
  await repo.getByRole("button", { name: "工作树详情 main" }).click();
  await expect(repo.getByText("已跟踪文件干净", { exact: true })).toBeHidden();
  await expect(page.locator('[aria-label="终端输出预览"]:visible')).toHaveCount(
    0,
  );
  const views = page.getByRole("group", { name: "会话显示方式" });
  await expect(views.getByRole("button", { name: "紧凑列表" })).toHaveAttribute(
    "aria-pressed",
    "true",
  );
  await views.getByRole("button", { name: "输出预览" }).click();
  await expect(page.locator('[aria-label="终端输出预览"]:visible')).toHaveCount(
    1,
  );
  await page.reload();
  await expect(views.getByRole("button", { name: "输出预览" })).toHaveAttribute(
    "aria-pressed",
    "true",
  );
  await expect(page.locator('[aria-label="终端输出预览"]:visible')).toHaveCount(
    1,
  );
  await views.getByRole("button", { name: "紧凑列表" }).click();
  await selectRepository(page, /^仓库 api-server$/);
  await expect(scope).toContainText("api-server");
  await expect(page.getByRole("link", { name: "API review" })).toBeVisible();
  await expect(page.getByRole("link", { name: "Scratch shell" })).toHaveCount(
    0,
  );

  const search = page.getByRole("searchbox", { name: "搜索会话" });
  await search.fill("Mobile layout");
  await expect(page.getByText("没有匹配的会话", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "清除筛选", exact: true }).click();
  const filters = page.getByRole("group", { name: "按会话状态筛选" });
  await filters.getByRole("button", { name: /已退出/ }).click();
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

  const nav = await openNavigation(page);
  await nav.getByRole("button", { name: /^仓库 api-server$/ }).focus();
  await page.keyboard.press("Tab");
  await expect(
    nav.getByRole("button", { name: /^仓库 mobile-app$/ }),
  ).toBeFocused();
  await page.keyboard.press("Tab");
  await expect(nav.getByRole("button", { name: /直接会话/ })).toBeFocused();
  await page.keyboard.press("Enter");
  if (testInfo.project.name === "chromium-mobile") {
    await expect(navigation).toBeHidden();
    await expect(toggle).toBeFocused();
  }
  await expect(page.locator("[data-repository-id]:visible")).toHaveCount(0);
  await expect(page.getByRole("link", { name: "Scratch shell" })).toBeVisible();
  await expect(
    page.getByRole("status").filter({ hasText: "个仓库" }),
  ).toHaveText("0 个仓库 · 1 个会话");
  await selectRepository(page, /全部仓库/);
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

  await selectRepository(page, /^仓库 api-server$/);
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
  await openNavigation(page);
  await expect(
    navigation.getByRole("button", { name: /直接会话/ }),
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
  await expect(page.getByRole("navigation", { name: "仓库导航" })).toHaveCount(
    0,
  );
});

test("repository refresh failure keeps direct sessions reachable and preserves the saved choice", async ({
  page,
  request,
  workDir,
}) => {
  await page.addInitScript(() =>
    localStorage.setItem("webmux.homeRepository", "saved-repository"),
  );
  await page.route("**/api/repositories", (route) =>
    route.fulfill({ status: 503, json: { error: "Registry unavailable" } }),
  );
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
});
