import { afterEach, expect, test, vi } from "vitest";
import { page, userEvent } from "vitest/browser";
import { cleanup, render } from "vitest-browser-svelte/pure";
import ChatHistory from "../../src/components/ChatHistory.svelte";
import {
  chat,
  launchConfig,
  record,
  repository,
  session,
  worktree,
} from "./shell-fixture";

afterEach(async () => {
  await cleanup();
  await userEvent.cleanup();
});

const list = () => page.getByRole("list", { name: "聊天历史" });
const archivedList = () => page.getByRole("list", { name: "已归档聊天" });

test("live chats show newest created first and archived records fold below", async () => {
  const onopen = vi.fn();
  const onresume = vi.fn();
  const working = chat({
    id: "chataaaa",
    title: "工作中的聊天",
    status: "running",
    createdAt: "2026-10-28T03:00:00Z",
    lastActiveAt: "2026-10-28T01:00:00Z",
  });
  render(ChatHistory, {
    sessions: [
      working,
      chat({
        id: "chatbbbb",
        title: "等授权",
        status: "waiting",
        createdAt: "2026-10-28T02:00:00Z",
        lastActiveAt: "2026-10-28T01:00:00Z",
        attention: {
          kind: "permission",
          source: "agent",
          since: "2026-10-28T02:00:00Z",
        },
      }),
      chat({
        id: "chatcccc",
        title: "空闲的聊天",
        createdAt: "2026-10-28T01:00:00Z",
        lastActiveAt: "2026-10-28T01:00:00Z",
      }),
      // Terminal sessions never show here.
      session({ id: "shellaaa", title: "pnpm dev" }),
    ],
    records: [
      record({ id: "chatcccc", title: "旧记录" }),
      record({
        id: "archivea",
        title: "整理文档",
        updatedAt: "2026-10-28T02:30:00Z",
      }),
    ],
    configs: [launchConfig()],
    repositories: [],
    current: "chatbbbb",
    onopen,
    onresume,
    onrename: vi.fn(),
    onclose: vi.fn(),
    onrelease: vi.fn(),
    onremove: vi.fn(),
  });
  const rows = list().getByRole("button", { name: /（/ });
  await expect.element(rows.first()).toBeVisible();
  expect(rows.elements().map((row) => row.getAttribute("aria-label"))).toEqual([
    "工作中的聊天（工作中）",
    "等授权（等待）",
    "空闲的聊天（空闲）",
  ]);
  await expect
    .element(list().getByRole("button", { name: "等授权（等待）" }))
    .toHaveAttribute("aria-current", "page");
  await list().getByRole("button", { name: "工作中的聊天（工作中）" }).click();
  expect(onopen).toHaveBeenCalledWith(working);

  // Archived records are collapsed by default; expanding shows them.
  const trigger = page.getByRole("button", { name: /已归档/ });
  await expect.element(trigger).toHaveAttribute("aria-expanded", "false");
  await trigger.click();
  await expect.element(trigger).toHaveAttribute("aria-expanded", "true");
  await archivedList()
    .getByRole("button", { name: "整理文档（已结束）" })
    .click();
  expect(onresume).toHaveBeenCalledWith(
    expect.objectContaining({ id: "archivea" }),
  );
});

const noop = {
  configs: [launchConfig()],
  repositories: [],
  current: "",
  onopen: vi.fn(),
  onresume: vi.fn(),
  onrename: vi.fn(),
  onclose: vi.fn(),
  onrelease: vi.fn(),
  onremove: vi.fn(),
};
const liveLabels = () =>
  list()
    .getByRole("button", { name: /（/ })
    .elements()
    .map((row) => row.getAttribute("aria-label"));

test("terminal titles keep Nerd Font icons through the UI icon fallback", async () => {
  const title = "π > \uf407 PR614: Review PR 614 for dev testing";
  render(ChatHistory, {
    ...noop,
    sessions: [chat({ id: "chatnfff", title })],
    records: [],
  });
  const label = page.getByText(title);
  await expect.element(label).toBeVisible();
  const family = getComputedStyle(label.element()).fontFamily;
  expect(family).toContain('"WebMux UI Icons"');
  // The icon family must come before the system fallback, or the private-use
  // codepoint could be claimed by a font without the glyph.
  expect(family.indexOf('"WebMux UI Icons"')).toBeLessThan(
    family.indexOf("system-ui"),
  );

  const faces = [...document.fonts].filter(
    (face) => face.family === "WebMux UI Icons",
  );
  expect(faces).toHaveLength(2);
  await Promise.all(faces.map((face) => face.load()));
  expect(faces.map((face) => face.status)).toEqual(["loaded", "loaded"]);
});

test("live rows keep their place when a later chat gets output, changes status or is renamed", async () => {
  const first = chat({
    id: "chataaaa",
    title: "较新",
    createdAt: "2026-10-28T03:00:00Z",
  });
  const second = chat({
    id: "chatbbbb",
    title: "居中",
    createdAt: "2026-10-28T02:00:00Z",
  });
  const third = chat({
    id: "chatcccc",
    title: "较旧",
    createdAt: "2026-10-28T01:00:00Z",
  });
  const view = await render(ChatHistory, {
    ...noop,
    sessions: [first, second, third],
    records: [],
  });
  await expect
    .element(list().getByRole("button", { name: /（/ }).first())
    .toBeVisible();
  const initial = ["较新（空闲）", "居中（空闲）", "较旧（空闲）"];
  expect(liveLabels()).toEqual(initial);

  // The oldest chat produces output and is the newest by activity.
  await view.rerender({
    sessions: [
      first,
      second,
      { ...third, lastActiveAt: "2026-10-28T09:00:00Z" },
    ],
  });
  expect(liveLabels()).toEqual(initial);

  // A status change and a rename do not move the row either.
  await view.rerender({
    sessions: [
      first,
      second,
      {
        ...third,
        title: "改名后的旧聊天",
        status: "running",
        lastActiveAt: "2026-10-28T09:30:00Z",
      },
    ],
  });
  await expect
    .element(list().getByRole("button", { name: "改名后的旧聊天（工作中）" }))
    .toBeVisible();
  expect(liveLabels()).toEqual([
    "较新（空闲）",
    "居中（空闲）",
    "改名后的旧聊天（工作中）",
  ]);

  // A chat created later appears at the top.
  await view.rerender({
    sessions: [
      first,
      second,
      third,
      chat({
        id: "chatdddd",
        title: "最新",
        createdAt: "2026-10-28T04:00:00Z",
      }),
    ],
  });
  await expect
    .element(list().getByRole("button", { name: "最新（空闲）" }))
    .toBeVisible();
  expect(liveLabels()[0]).toBe("最新（空闲）");
});

test("rows with equal creation or update time are ordered by ID", async () => {
  const created = "2026-10-28T03:00:00Z";
  render(ChatHistory, {
    ...noop,
    sessions: [
      chat({ id: "chatbbbb", title: "乙", createdAt: created }),
      chat({ id: "chataaaa", title: "甲", createdAt: created }),
    ],
    records: [
      record({ id: "archiveb", title: "归档乙" }, { updatedAt: created }),
      record({ id: "archivea", title: "归档甲" }, { updatedAt: created }),
      record(
        { id: "archivez", title: "归档较新" },
        { updatedAt: "2026-10-28T04:00:00Z" },
      ),
    ],
  });
  await expect
    .element(list().getByRole("button", { name: /（/ }).first())
    .toBeVisible();
  expect(liveLabels()).toEqual(["甲（空闲）", "乙（空闲）"]);
  await page.getByRole("button", { name: /已归档/ }).click();
  const rows = archivedList().getByRole("button", { name: /（/ });
  await expect.element(rows.first()).toBeVisible();
  expect(rows.elements().map((row) => row.getAttribute("aria-label"))).toEqual([
    "归档较新（已结束）",
    "归档甲（已结束）",
    "归档乙（已结束）",
  ]);
});

test("archived rows mount only while expanded, a page at a time", async () => {
  const newest = Date.parse("2026-10-28T12:00:00Z");
  const records = Array.from({ length: 25 }, (_, index) =>
    record(
      { id: `archiv${String(index).padStart(2, "0")}`, title: `记录 ${index}` },
      { updatedAt: new Date(newest - index * 60_000).toISOString() },
    ),
  );
  render(ChatHistory, { ...noop, sessions: [], records });
  // A closed section renders no rows at all, however long the history is.
  expect(archivedList().elements()).toEqual([]);
  await page.getByRole("button", { name: /已归档/ }).click();
  const rows = archivedList().getByRole("button", { name: /（/ });
  await expect.element(rows.first()).toBeVisible();
  expect(rows.elements()).toHaveLength(20);
  await page.getByRole("button", { name: "显示更多（还有 5 条）" }).click();
  await expect
    .element(archivedList().getByRole("button", { name: "记录 24（已结束）" }))
    .toBeVisible();
  expect(rows.elements()).toHaveLength(25);
  // Folding again drops the rows instead of keeping them hidden.
  await page.getByRole("button", { name: /已归档/ }).click();
  expect(archivedList().elements()).toEqual([]);
});

test("archived section opens automatically when viewing or resuming an archived record", async () => {
  render(ChatHistory, {
    sessions: [],
    records: [
      record({
        id: "archivea",
        title: "整理文档",
        updatedAt: "2026-10-28T02:30:00Z",
      }),
    ],
    configs: [launchConfig()],
    repositories: [],
    current: "archivea",
    onopen: vi.fn(),
    onresume: vi.fn(),
    onrename: vi.fn(),
    onclose: vi.fn(),
    onrelease: vi.fn(),
    onremove: vi.fn(),
  });
  const trigger = page.getByRole("button", { name: /已归档/ });
  await expect.element(trigger).toHaveAttribute("aria-expanded", "true");
  await expect
    .element(archivedList().getByRole("button", { name: "整理文档（已结束）" }))
    .toHaveAttribute("aria-current", "page");
});

test("archived rows offer releasing a linked worktree and mark released records", async () => {
  const linked = worktree({
    id: "wt1",
    path: "/repos/webmux.worktrees/wt1",
    kind: "linked",
    branch: "wt/fix-sort",
  });
  const onrelease = vi.fn();
  const onremoveworktree = vi.fn();
  const onresume = vi.fn();
  const released = record({
    id: "archiver",
    title: "已释放的聊天",
    cwd: "/repos/webmux.worktrees/old",
    released: {
      id: "rel1",
      repositoryId: "repo1",
      path: "/repos/webmux.worktrees/old",
      commit: "d".repeat(40),
      releasedAt: "2026-10-28T00:00:00Z",
    },
  });
  render(ChatHistory, {
    sessions: [],
    records: [
      record({
        id: "archivew",
        title: "分支聊天",
        cwd: linked.path,
        workspace: {
          repositoryId: "repo1",
          worktreeId: "wt1",
          worktreePath: linked.path,
        },
        updatedAt: "2026-10-28T02:00:00Z",
      }),
      record({ id: "archivem", title: "主工作区聊天" }),
      released,
    ],
    configs: [launchConfig()],
    repositories: [repository({ worktrees: [worktree(), linked] })],
    current: "",
    onopen: vi.fn(),
    onresume,
    onrename: vi.fn(),
    onclose: vi.fn(),
    onrelease,
    onremove: vi.fn(),
    onremoveworktree,
  });
  await page.getByRole("button", { name: /已归档/ }).click();
  const menuItems = async (title: string) => {
    await archivedList()
      .getByRole("button", { name: `会话操作：${title}` })
      .click();
    await expect.element(page.getByRole("menu")).toBeVisible();
    const items = page
      .getByRole("menuitem")
      .elements()
      .map((item) => item.textContent?.trim());
    await userEvent.keyboard("{Escape}");
    return items;
  };
  expect(await menuItems("分支聊天")).toEqual([
    "恢复聊天",
    "释放空间",
    "删除记录",
    "删除记录和工作树",
  ]);
  expect(await menuItems("主工作区聊天")).toEqual(["恢复聊天", "删除记录"]);
  expect(await menuItems("已释放的聊天")).toEqual(["恢复聊天", "删除记录"]);

  await archivedList()
    .getByRole("button", { name: "会话操作：分支聊天" })
    .click();
  await page.getByRole("menuitem", { name: "释放空间" }).click();
  expect(onrelease).toHaveBeenCalledWith(linked);

  await archivedList()
    .getByRole("button", { name: "会话操作：分支聊天" })
    .click();
  await page.getByRole("menuitem", { name: "删除记录和工作树" }).click();
  expect(onremoveworktree).toHaveBeenCalledWith(
    expect.objectContaining({ id: "archivew" }),
    linked,
  );

  // The released row carries no text badge: the hollow triangle marks it,
  // and its path and commit stay on the row's hover text. It still resumes
  // through the caller, which confirms the rebuild first.
  const row = archivedList().getByRole("button", {
    name: "已释放的聊天（已释放空间，已结束）",
  });
  expect(row.element().textContent).not.toContain("已释放空间");
  expect(row.element().getAttribute("title")).toContain(
    `已释放空间：/repos/webmux.worktrees/old · ${"d".repeat(12)}`,
  );
  expect(
    row.element().querySelector("[data-status]")?.getAttribute("data-status"),
  ).toBe("released");
  expect(
    archivedList()
      .getByRole("button", { name: "主工作区聊天（已结束）" })
      .element()
      .querySelector("[data-status]")
      ?.getAttribute("data-status"),
  ).toBe("ended");
  await row.click();
  expect(onresume).toHaveBeenCalledWith(released);
});
