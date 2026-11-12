import { spawnSync } from "node:child_process";
import { mkdir, writeFile } from "node:fs/promises";
import { join } from "node:path";
import type { APIRequestContext, Page } from "@playwright/test";
import { expect, expectConnected, test } from "./fixtures";

// The app shell around the home page, quick start and session pages: the
// sidebar beside a session, its workbench tree, rename, and the phone
// drawer. Sessions run plain shells or the fake agent (see hybrid.spec.ts).

async function repository(request: APIRequestContext, root: string) {
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
  return (await response.json()) as {
    id: string;
    worktrees: { id: string }[];
  };
}

async function shell(
  request: APIRequestContext,
  name: string,
  data: Record<string, unknown>,
) {
  const response = await request.post("/api/sessions", {
    data: { cmd: ["/bin/sh", "-c", "sleep 600"], name, ...data },
  });
  expect(response.status(), await response.text()).toBe(201);
  return (await response.json()).session.id as string;
}

async function chat(request: APIRequestContext, cwd: string, name: string) {
  const config = await request.post("/api/launch-configs", {
    data: { name: "Claude Code", cmd: ["claude"], env: {}, chat: true },
  });
  expect(config.status(), await config.text()).toBe(201);
  const response = await request.post("/api/sessions", {
    data: { cwd, configId: (await config.json()).id, view: "chat", name },
  });
  expect(response.status(), await response.text()).toBe(201);
  return (await response.json()).session.id as string;
}

const tree = (page: Page) => page.getByRole("tree", { name: "仓库与会话" });
const cols = async (request: APIRequestContext, id: string) =>
  (await (await request.get(`/api/sessions/${id}`)).json()).cols as number;

test.describe("desktop sidebar", () => {
  test.skip(({ isMobile }) => isMobile, "the desktop sidebar");

  test("collapsing the sidebar widens the terminal and keeps it in the viewport", async ({
    page,
    request,
    workDir,
  }) => {
    await page.setViewportSize({ width: 1280, height: 720 });
    const id = await shell(request, "Columns", {
      cwd: workDir,
      cmd: ["/bin/bash", "--noprofile", "--norc", "-i"],
    });
    await page.goto(`/s/${id}`);
    await expectConnected(page);
    const fits = () =>
      page.evaluate(() => {
        const box = document
          .querySelector(".terminal-page")!
          .getBoundingClientRect();
        return box.right <= innerWidth + 0.5 && box.left > 0;
      });
    expect(await fits()).toBe(true);
    await expect.poll(() => cols(request, id)).toBeGreaterThan(0);
    const wide = await cols(request, id);
    await page.getByRole("button", { name: "收起侧栏" }).click();
    await expect.poll(() => cols(request, id)).toBeGreaterThan(wide);
    expect(await fits()).toBe(true);
    await page.reload();
    await expect(page.getByRole("button", { name: "展开侧栏" })).toBeVisible();
    await page.getByRole("button", { name: "展开侧栏" }).click();
    await expect.poll(() => cols(request, id)).toBe(wide);
  });

  test("the tree jumps to worktrees, opens sessions by their side and renames them", async ({
    page,
    request,
    workDir,
  }) => {
    test.slow();
    const repo = await repository(request, join(workDir, "tree-repo"));
    const main = repo.worktrees[0]!.id;
    const terminal = await shell(request, "pnpm dev", {
      cwd: join(workDir, "tree-repo"),
      workspace: { repositoryId: repo.id, worktreeId: main },
    });
    const agent = await chat(request, join(workDir, "tree-repo"), "修复排序");
    await page.goto(`/s/${terminal}`);
    // The current session's repository and worktree open on their own.
    await expect(
      tree(page).getByRole("treeitem", { name: /^pnpm dev（/ }),
    ).toHaveAttribute("aria-current", "page");

    // A terminal session keeps the workbench and shows its terminal.
    await tree(page).getByRole("treeitem", { name: /^main/ }).click();
    // The home view narrows to that worktree's sessions.
    await expect(page).toHaveURL(/\/$/);
    await expect(
      page.getByRole("main").getByRole("heading", { level: 1 }),
    ).toHaveText("tree-repo · main");
    await expect(page.getByRole("link", { name: "pnpm dev" })).toBeVisible();
    await expect(page.locator("[data-worktree-id]:visible")).toHaveCount(1);
    // The worktree stays open down to its live session row on the home view.
    await expect(
      tree(page).getByRole("treeitem", { name: /^main/ }),
    ).toHaveAttribute("aria-expanded", "true");
    await tree(page)
      .getByRole("treeitem", { name: /^pnpm dev（/ })
      .click();
    await expect(page).toHaveURL(new RegExp(`/s/${terminal}$`));
    await expect(
      page.getByRole("button", { name: "工作台模式" }),
    ).toHaveAttribute("aria-pressed", "true");

    // A chat-side hybrid session keeps the workbench; switching to chat
    // mode only swaps the sidebar.
    await page.goto("/");
    await tree(page).locator('[data-tree-key="all"]').click();
    await page.getByRole("link", { name: "修复排序" }).click();
    await expect(page).toHaveURL(new RegExp(`/s/${agent}$`));
    await expect(
      page.getByRole("button", { name: "工作台模式" }),
    ).toHaveAttribute("aria-pressed", "true");
    await page.getByRole("button", { name: "聊天模式" }).click();
    await expect(page).toHaveURL(new RegExp(`/s/${agent}$`));
    const chats = page.getByRole("list", { name: "聊天历史" });
    await expect(chats.locator('[aria-current="page"]')).toHaveCount(1);

    // Renamed from the sidebar, the title follows everywhere.
    await chats.getByRole("button", { name: "会话操作：修复排序" }).click();
    await page.getByRole("menuitem", { name: "修改标题" }).click();
    const dialog = page.getByRole("dialog");
    await dialog.getByRole("textbox").fill("首页排序");
    await page.keyboard.press("Enter");
    await expect(
      chats.getByRole("button", { name: /^首页排序（/ }),
    ).toBeVisible();
    await expect(page.locator(".terminal-header")).toContainText("首页排序");
    await chats.getByRole("button", { name: "会话操作：首页排序" }).click();
    await page.getByRole("menuitem", { name: "修改标题" }).click();
    await dialog.getByRole("textbox").fill("");
    await dialog.getByRole("button", { name: "保存" }).click();
    await expect(
      chats.getByRole("button", { name: /^首页排序（/ }),
    ).toHaveCount(0);

    // The workbench repository menu opens the new-worktree dialog in place.
    await page.getByRole("button", { name: "工作台模式" }).click();
    await expect(page).toHaveURL(new RegExp(`/s/${agent}$`));
    await tree(page)
      .getByRole("treeitem", { name: /^tree-repo，/ })
      .hover();
    await tree(page)
      .getByRole("button", { name: "仓库操作：tree-repo" })
      .click();
    await page.getByRole("menuitem", { name: "新建工作树…" }).click();
    const create = page.getByRole("dialog");
    await expect(create).toBeVisible();
    await create.getByRole("button", { name: /^创建工作树/ }).click();
    await expect(create).toBeHidden();
    await expect(page).toHaveURL(new RegExp(`/s/${agent}$`));
    await expect(
      tree(page).getByRole("treeitem", { name: /^wt\// }),
    ).toHaveCount(1);
  });

  test("the code panel keeps its state, width and tabs across session switches", async ({
    page,
    request,
    workDir,
  }) => {
    test.slow();
    await page.setViewportSize({ width: 1440, height: 800 });
    const root = join(workDir, "panel-repo");
    const repo = await repository(request, root);
    await writeFile(join(root, "note.txt"), "kept open\n");
    const workspace = {
      repositoryId: repo.id,
      worktreeId: repo.worktrees[0]!.id,
    };
    const first = await shell(request, "panel one", { cwd: root, workspace });
    await shell(request, "panel two", { cwd: root, workspace });
    const config = await request.post("/api/launch-configs", {
      data: { name: "Claude Code", cmd: ["claude"], env: {}, chat: true },
    });
    expect(config.status(), await config.text()).toBe(201);
    const configId = (await config.json()).id as string;
    for (const name of ["面板甲", "面板乙"]) {
      const response = await request.post("/api/sessions", {
        data: { cwd: root, configId, view: "chat", name },
      });
      expect(response.status(), await response.text()).toBe(201);
    }
    const code = page.getByRole("region", { name: "代码工作区" });
    const toggle = page
      .locator(".terminal-header")
      .getByRole("button", { name: /^(代码|折叠代码)$/ });
    const width = async () =>
      Math.round((await page.locator(".code-pane").boundingBox())!.width);

    await page.goto(`/s/${first}`);
    await expectConnected(page);
    await toggle.click();
    await expect(code).toBeVisible();
    await code.getByRole("button", { name: "note.txt", exact: true }).click();
    const note = code.getByRole("tab", { name: /^note\.txt · / });
    await expect(note).toBeVisible();
    const before = await width();
    await page.getByRole("separator", { name: "调整终端与代码宽度" }).focus();
    for (let i = 0; i < 3; i++) await page.keyboard.press("ArrowLeft");
    await expect.poll(width).toBeGreaterThan(before + 20);
    const wide = await width();

    // Workbench mode: the next session opens with the same panel.
    await tree(page)
      .getByRole("treeitem", { name: /^panel two（/ })
      .click();
    await expect(page).not.toHaveURL(new RegExp(`/s/${first}$`));
    await expect(code).toBeVisible();
    await expect.poll(width).toBe(wide);
    // Open files belong to their own session.
    await expect(note).toHaveCount(0);

    // Chat mode keeps it too, open or closed.
    await page.getByRole("button", { name: "聊天模式" }).click();
    const chats = page.getByRole("list", { name: "聊天历史" });
    await chats.getByRole("button", { name: /^面板甲（/ }).click();
    await expect(page.locator(".terminal-header")).toContainText("面板甲");
    await expect(code).toBeVisible();
    await expect.poll(width).toBe(wide);
    await toggle.click();
    await expect(code).toBeHidden();
    await chats.getByRole("button", { name: /^面板乙（/ }).click();
    await expect(page.locator(".terminal-header")).toContainText("面板乙");
    await expect(code).toBeHidden();

    // Back on the first session, its files are open again.
    await page.getByRole("button", { name: "工作台模式" }).click();
    await tree(page)
      .getByRole("treeitem", { name: /^panel one（/ })
      .click();
    await expect(page).toHaveURL(new RegExp(`/s/${first}$`));
    await toggle.click();
    await expect(note).toHaveAttribute("aria-selected", "true");
    await expect(code.locator("[data-code-reader]:visible")).toContainText(
      "kept open",
    );
  });
});

test.describe("phone drawer", () => {
  test.skip(({ isMobile }) => !isMobile, "the phone drawer");

  for (const size of [
    { width: 390, height: 844 },
    { width: 320, height: 568 },
  ])
    test(`the session top bar stays one line and opens the drawer at ${size.width}px`, async ({
      page,
      request,
      workDir,
    }) => {
      await page.setViewportSize(size);
      const id = await shell(request, "demo-agent", { cwd: workDir });
      await page.goto(`/s/${id}`);
      const header = page.locator(".terminal-header").first();
      await expect(header).toBeVisible();
      expect((await header.boundingBox())!.height).toBeLessThanOrEqual(48);
      expect(
        await page.evaluate(() => document.documentElement.scrollWidth),
      ).toBeLessThanOrEqual(size.width + 1);
      const open = header.getByRole("button", { name: "打开侧栏" });
      await open.click();
      const drawer = page.getByRole("dialog", { name: "导航" });
      await expect(drawer).toBeVisible();
      await expect(drawer.locator(":focus")).toHaveCount(1);
      await page.keyboard.press("Escape");
      await expect(drawer).toBeHidden();
      await expect(open).toBeFocused();
      // "全部仓库" in the drawer returns to the workbench home.
      await open.click();
      await drawer.locator('[data-tree-key="all"]').click();
      await expect(drawer).toBeHidden();
      await expect(page).toHaveURL(/\/$/);
    });
});

// Required guard: at every width, on the home page, with either stored
// collapse state, the sidebar (full or rail) or its open button is reachable.
test.describe("sidebar is always reachable", () => {
  for (const collapsed of ["false", "true"])
    test(`a sidebar or its open button is visible at every width (collapsed=${collapsed})`, async ({
      page,
    }) => {
      await page.addInitScript((value) => {
        localStorage.setItem("webmux.sidebarCollapsed", value);
      }, collapsed);
      await page.goto("/");
      for (const width of [320, 390, 639, 640, 768, 1023, 1024, 1280]) {
        await page.setViewportSize({ width, height: 800 });
        const reachable = page
          .getByRole("complementary", { name: "侧栏" })
          .or(page.getByRole("navigation", { name: "窄栏导航" }))
          .or(page.getByRole("button", { name: "打开侧栏" }));
        await expect(
          reachable.first(),
          `no sidebar or open button at ${width}px`,
        ).toBeVisible();
        // Page content never hides under a sidebar that is not shown.
        const pad = await page.evaluate(() => {
          const main = document.querySelector(".app-shell-main");
          return main ? parseFloat(getComputedStyle(main).paddingLeft) : 0;
        });
        if (width < 640) expect(pad, `padding at ${width}px`).toBe(0);
      }
    });
});

// Required guard: the sidebar's top row (collapse button, brand, mode
// switch) never overlaps, even at the narrowest sidebar and a larger font.
test.describe("sidebar header never overlaps", () => {
  for (const fontSize of ["100%", "150%"])
    test(`collapse button, brand and mode switch stay apart at the narrowest width (font ${fontSize})`, async ({
      page,
    }) => {
      await page.addInitScript(() => {
        localStorage.setItem("webmux.sidebarCollapsed", "false");
        localStorage.setItem("webmux.sidebarWidth", "1");
      });
      await page.setViewportSize({ width: 1280, height: 800 });
      await page.goto("/");
      await page.evaluate((size) => {
        document.documentElement.style.fontSize = size;
      }, fontSize);
      const sidebar = page.getByRole("complementary", { name: "侧栏" });
      const nav = sidebar.getByRole("navigation", { name: "导航" });
      await expect(nav).toBeVisible();
      const boxes = [];
      for (const part of [
        nav.getByRole("button", { name: "收起侧栏" }),
        nav.locator(".brand"),
        nav.getByRole("group", { name: "模式" }),
      ]) {
        await expect(part).toBeVisible();
        boxes.push((await part.boundingBox())!);
      }
      for (let i = 0; i < boxes.length - 1; i++)
        expect(
          boxes[i].x + boxes[i].width,
          `part ${i} overlaps part ${i + 1}`,
        ).toBeLessThanOrEqual(boxes[i + 1].x + 0.5);
      const side = (await sidebar.boundingBox())!;
      const last = boxes[2];
      expect(last.x + last.width).toBeLessThanOrEqual(side.x + side.width);
    });
});

test("session pins and their order sync to another browser", async ({
  page,
  browser,
  request,
  workDir,
}, info) => {
  const id = await shell(request, "Pinned demo", { cwd: workDir });
  const second = await shell(request, "Second demo", { cwd: workDir });
  const mobile = info.project.use.isMobile;
  const drawer = (target: Page) =>
    target.getByRole("button", { name: "打开侧栏", exact: true }).click();
  const region = (target: Page) =>
    target.getByRole("region", { name: "已置顶会话", exact: true });
  async function expandDirect() {
    const expand = tree(page).getByRole("button", {
      name: "展开直接会话",
      exact: true,
    });
    if (await expand.isVisible()) await expand.click();
  }
  async function pin(title: string) {
    await expandDirect();
    const row = tree(page).getByRole("treeitem", {
      name: new RegExp(`^${title}`),
    });
    if (!mobile) await row.hover();
    await tree(page)
      .getByRole("button", { name: `会话操作：${title}`, exact: true })
      .click();
    await page.getByRole("menuitem", { name: "置顶会话", exact: true }).click();
    await expect(row).toHaveCount(0);
  }
  async function pinnedMenu(title: string) {
    if (!mobile) await region(page).getByText(title, { exact: true }).hover();
    await region(page)
      .getByRole("button", { name: `会话操作：置顶 · ${title}`, exact: true })
      .click();
  }
  const order = (target: Page, titles: string[]) =>
    expect(region(target).getByRole("listitem")).toHaveText(
      titles.map((title) => new RegExp(title)),
    );

  await page.goto(`/s/${id}`);
  if (mobile) await drawer(page);
  await pin("Pinned demo");
  await pin("Second demo");
  await order(page, ["Second demo", "Pinned demo"]);
  const other = await browser.newContext({
    viewport: page.viewportSize() ?? undefined,
    isMobile: mobile,
    hasTouch: info.project.use.hasTouch,
  });
  try {
    const device = await other.newPage();
    await device.goto(new URL(`/s/${id}`, page.url()).href);
    if (mobile) await drawer(device);
    await order(device, ["Second demo", "Pinned demo"]);

    await pinnedMenu("Pinned demo");
    await expect(
      page.getByRole("menuitem", { name: "下移置顶" }),
    ).toBeDisabled();
    await page.getByRole("menuitem", { name: "上移置顶" }).click();
    await order(page, ["Pinned demo", "Second demo"]);
    await order(device, ["Pinned demo", "Second demo"]);

    await page.reload();
    await page
      .getByRole("button", { name: "快速切换会话", exact: true })
      .click();
    const dialog = page.getByRole("dialog", {
      name: "快速切换会话",
      exact: true,
    });
    await expect(dialog.getByRole("option").first()).toContainText(
      "Pinned demo",
    );
    await page.keyboard.press("Escape");
    if (mobile) await drawer(page);

    await pinnedMenu("Second demo");
    await page.getByRole("menuitem", { name: "取消置顶", exact: true }).click();
    await order(device, ["Pinned demo"]);
    await expandDirect();
    await expect(
      tree(page).getByRole("treeitem", { name: /^Second demo/ }),
    ).toBeVisible();
    await pin("Second demo");
    expect((await request.delete(`/api/sessions/${id}`)).ok()).toBe(true);
    await order(device, ["Second demo"]);
    await expect
      .poll(async () => {
        const response = await request.get("/api/settings");
        return (await response.json()).values.sessionPins;
      })
      .toEqual([second]);
  } finally {
    await other.close();
  }
});
