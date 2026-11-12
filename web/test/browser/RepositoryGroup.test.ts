import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { page, userEvent } from "vitest/browser";
import { cleanup, render } from "vitest-browser-svelte/pure";
import RepositoryGroup from "../../src/components/workspace/RepositoryGroup.svelte";
import type { SessionInfo, Worktree } from "../../src/contracts/schemas";
import { repository, session, worktree } from "./shell-fixture";

beforeEach(async () => {
  await page.viewport(1024, 900);
});

afterEach(async () => {
  await cleanup();
  document.body.style.padding = "";
});

function renderGroup(
  focused: boolean,
  linked: Parameters<typeof worktree>[0] = {},
  sessions: SessionInfo[] = [],
  onswitch: (tree: Worktree) => void = () => {},
) {
  render(RepositoryGroup, {
    repo: repository({
      worktrees: [
        worktree({ id: "main" }),
        worktree({
          id: "linked",
          kind: "linked",
          branch: "wt/feature",
          ...linked,
        }),
      ],
    }),
    sessions,
    filtering: false,
    focused,
    open: true,
    onopenchange: () => {},
    now: Date.now(),
    onstart: () => {},
    ontogglepin: () => {},
    onremove: () => {},
    onswitch,
    onclose: () => {},
  });
}

test("a focused repository lists its worktrees without repeating a heading", async () => {
  await page.viewport(430, 932);
  renderGroup(true);
  await expect
    .element(page.getByRole("region", { name: /^仓库 / }))
    .toBeVisible();
  await expect.element(page.getByText("main", { exact: true })).toBeVisible();
  expect(page.getByRole("heading", { level: 2 }).elements()).toEqual([]);
  expect(document.documentElement.scrollWidth).toBeLessThanOrEqual(
    window.innerWidth,
  );
});

test("an unfocused repository keeps its name as a foldable heading", async () => {
  renderGroup(false);
  const heading = page.getByRole("heading", { level: 2 });
  await expect.element(heading).toBeVisible();
  await expect
    .element(heading.getByRole("button", { expanded: true }))
    .toBeVisible();
});

// A worktree with a session is always on the home page.
const inLinked = session({
  id: "linked01",
  workspace: {
    repositoryId: "repo1",
    worktreeId: "linked",
    worktreePath: "/repos/webmux",
  },
});

test("a worktree ahead of its upstream says so in its title, without a behind count", async () => {
  renderGroup(false, { upstream: "origin/wt/feature", ahead: 3, behind: 0 }, [
    inLinked,
  ]);
  const linked = page.getByRole("region", { name: "wt/feature" });
  await expect
    .element(linked.getByRole("img", { name: "领先上游 3 个提交" }))
    .toHaveTextContent("3");
  expect(linked.getByRole("img", { name: /落后/ }).elements()).toEqual([]);
  // Synced and untracked worktrees show no indicator.
  expect(
    page.getByRole("region", { name: "main" }).getByRole("img").elements(),
  ).toEqual([]);
});

test("a deleted upstream is marked without counts", async () => {
  renderGroup(false, { upstream: "origin/wt/feature", upstreamGone: true }, [
    inLinked,
  ]);
  const linked = page.getByRole("region", { name: "wt/feature" });
  await expect.element(linked.getByText("上游已删除")).toBeVisible();
  expect(linked.getByRole("img").elements()).toEqual([]);
});

test("a worktree's menu switches its branch unless it is prunable", async () => {
  const onswitch = vi.fn();
  renderGroup(
    false,
    { available: false, prunable: true },
    [inLinked],
    onswitch,
  );
  const items = () =>
    page
      .getByRole("menuitem")
      .elements()
      .map((el) => el.textContent?.trim());
  await page.getByRole("button", { name: "工作树菜单 main" }).click();
  expect(items()).toEqual(["切换分支…", "从主页移除"]);
  await page.getByRole("menuitem", { name: "切换分支…" }).click();
  expect(onswitch).toHaveBeenCalledWith(
    expect.objectContaining({ id: "main" }),
  );

  await page.getByRole("button", { name: "工作树菜单 wt/feature" }).click();
  await expect
    .element(page.getByRole("menuitem", { name: "删除 worktree…" }))
    .toBeVisible();
  expect(items()).not.toContain("切换分支…");
  await userEvent.keyboard("{Escape}");
});

test("a worktree's menu follows its branch state and starts the operation", async () => {
  const onoperation = vi.fn();
  render(RepositoryGroup, {
    repo: repository({
      worktrees: [
        worktree({
          id: "main",
          upstream: "origin/main",
          ahead: 0,
          behind: 0,
        }),
        worktree({
          id: "linked",
          kind: "linked",
          branch: "wt/feature",
          upstream: "origin/wt/feature",
          ahead: 0,
          behind: 2,
        }),
        worktree({
          id: "fresh",
          kind: "linked",
          branch: "wt/fresh",
          path: "/repos/webmux.worktrees/fresh",
        }),
      ],
    }),
    // A worktree with a session is on the home page even unpinned.
    sessions: [
      inLinked,
      session({
        id: "fresh001",
        workspace: {
          repositoryId: "repo1",
          worktreeId: "fresh",
          worktreePath: "/repos/webmux.worktrees/fresh",
        },
      }),
    ],
    filtering: false,
    focused: false,
    open: true,
    onopenchange: () => {},
    now: Date.now(),
    onstart: () => {},
    ontogglepin: () => {},
    onremove: () => {},
    onswitch: () => {},
    onclose: () => {},
    onoperation,
  });
  const items = () =>
    page
      .getByRole("menuitem")
      .elements()
      .map((el) => el.textContent?.trim());
  for (const [name, expected] of [
    ["main", ["切换分支…", "从主页移除"]],
    ["wt/feature", ["切换分支…", "快进到上游", "添加到主页", "删除 worktree…"]],
    ["wt/fresh", ["切换分支…", "发布分支…", "添加到主页", "删除 worktree…"]],
  ] as const) {
    await page.getByRole("button", { name: `工作树菜单 ${name}` }).click();
    await expect.element(page.getByRole("menuitem").first()).toBeVisible();
    expect(items()).toEqual(expected);
    await userEvent.keyboard("{Escape}");
    await expect.poll(() => items()).toEqual([]);
  }
  await page.getByRole("button", { name: "工作树菜单 wt/fresh" }).click();
  await page.getByRole("menuitem", { name: "发布分支…" }).click();
  expect(onoperation).toHaveBeenCalledWith({
    kind: "push",
    branch: "wt/fresh",
    publish: true,
  });
});
