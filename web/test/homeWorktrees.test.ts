import { describe, expect, test } from "vitest";
import {
  homeLayout,
  type HomeLayout,
  parseHomeWorktrees,
  pruneHomeWorktrees,
  setPinned,
} from "../src/homeWorktrees";
import type { Repository, SessionInfo, Worktree } from "../src/schemas";

const tree = (id: string, kind: Worktree["kind"] = "linked"): Worktree => ({
  id,
  repositoryId: "r1",
  path: `/work/${id}`,
  kind,
  branch: id,
  head: null,
  detached: false,
  locked: false,
  lockReason: "",
  prunable: false,
  available: true,
  dirty: false,
  hasUntracked: false,
  hasIgnored: false,
  error: "",
});
const repo = (
  worktrees: Worktree[],
  overrides: Partial<Repository> = {},
): Repository => ({
  id: "r1",
  name: "project",
  commonDir: "/work/main/.git",
  anchorPath: "/work/main",
  availability: "available",
  error: "",
  checkedAt: "2026-10-20T00:00:00Z",
  worktrees,
  ...overrides,
});
const sessionIn = (worktreeId: string) =>
  ({
    workspace: { repositoryId: "r1", worktreeId, worktreePath: "" },
  }) as SessionInfo;
const shownIds = (layout: HomeLayout) =>
  layout.shown.map(({ tree, pinned }) => `${tree.id}:${pinned}`);

describe("homeLayout", () => {
  const trees = [tree("b"), tree("main", "main"), tree("a")];

  test("an uncurated repository shows only its main worktree", () => {
    const layout = homeLayout({}, repo(trees), []);
    expect(shownIds(layout)).toEqual(["main:true"]);
    expect(layout.unpinned.map((t) => t.id)).toEqual(["a", "b"]);
  });

  test("a worktree with sessions is shown even when not pinned", () => {
    const layout = homeLayout({ r1: [] }, repo(trees), [sessionIn("b")]);
    expect(shownIds(layout)).toEqual(["b:false"]);
    expect(layout.unpinned.map((t) => t.id)).toEqual(["main", "a", "b"]);
  });

  test("an explicit empty choice hides the main worktree too", () => {
    expect(homeLayout({ r1: [] }, repo(trees), []).shown).toEqual([]);
  });
});

describe("setPinned", () => {
  test("first curation starts from the default main pin", () => {
    const home = setPinned(
      {},
      repo([tree("main", "main"), tree("a")]),
      ["a"],
      true,
    );
    expect(home.r1).toEqual(["main", "a"]);
    expect(setPinned(home, repo([]), ["main"], false).r1).toEqual(["a"]);
  });
});

describe("pruneHomeWorktrees", () => {
  test("drops unregistered repositories and worktrees gone from a healthy snapshot", () => {
    const home = { r1: ["a", "gone"], r2: ["x"] };
    expect(pruneHomeWorktrees(home, [repo([tree("a")])])).toEqual({
      r1: ["a"],
    });
  });

  test("keeps pins while the snapshot is unhealthy and returns the same object when unchanged", () => {
    const home = { r1: ["a", "gone"] };
    for (const failed of [
      repo([], { availability: "unavailable" }),
      repo([tree("a")], { error: "git failed" }),
    ])
      expect(pruneHomeWorktrees(home, [failed])).toBe(home);
    expect(pruneHomeWorktrees({ r1: ["a"] }, [repo([tree("a")])])).toEqual({
      r1: ["a"],
    });
  });
});

test("corrupt stored data means nothing was curated", () => {
  expect(parseHomeWorktrees("{not json")).toEqual({});
  expect(parseHomeWorktrees(JSON.stringify({ r1: "a" }))).toEqual({});
});
