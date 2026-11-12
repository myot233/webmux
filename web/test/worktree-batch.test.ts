import { describe, expect, test } from "vitest";
import type {
  ArchivedSession,
  RemovalPreviewItem,
  Repository,
  SessionInfo,
  Worktree,
} from "../src/contracts/schemas";
import {
  buildWorktreeBatchEntries,
  countWorktreeBatchCategories,
  filterWorktreeBatchEntries,
  groupBatchPreviewItems,
  LOCKED_STALE_REASON,
  staleRegistrations,
} from "../src/domains/workspace/worktree-batch";

const makeTree = (id: string, patch: Partial<Worktree> = {}): Worktree => ({
  id,
  repositoryId: "repo1",
  path: `/work/${id}`,
  kind: "linked",
  branch: `wt/${id}`,
  head: "a".repeat(40),
  detached: false,
  locked: false,
  lockReason: "",
  prunable: false,
  available: true,
  dirty: false,
  hasUntracked: false,
  hasIgnored: false,
  error: "",
  upstream: null,
  ahead: null,
  behind: null,
  upstreamGone: false,
  lastCommit: null,
  changes: { staged: 0, unstaged: 0, untracked: 0, conflicted: 0 },
  operation: null,
  ...patch,
});

const makeRepo = (worktrees: Worktree[]): Repository => ({
  id: "repo1",
  name: "webmux",
  commonDir: "/work/main/.git",
  anchorPath: "/work/main",
  availability: "available",
  error: "",
  checkedAt: "2026-11-06T00:00:00Z",
  lastFetchedAt: null,
  initInstruction: "",
  worktrees,
});

const makeSession = (
  id: string,
  cwd: string,
  status: SessionInfo["status"] = "idle",
  worktreeId?: string,
): SessionInfo =>
  ({
    id,
    title: id,
    cwd,
    state: "active",
    status,
    workspace: worktreeId
      ? { repositoryId: "repo1", worktreeId, worktreePath: cwd }
      : null,
  }) as SessionInfo;

const makeArchived = (
  id: string,
  cwd: string,
  released = false,
): ArchivedSession =>
  ({
    id,
    title: id,
    cwd,
    state: "closed",
    workspace: null,
    released: released
      ? {
          id: "rel1",
          repositoryId: "repo1",
          path: cwd,
          commit: "b".repeat(40),
          branch: "wt/old",
          releasedAt: "2026-11-06T00:00:00Z",
        }
      : null,
  }) as ArchivedSession;

describe("worktreeBatch", () => {
  const main = makeTree("main", {
    kind: "main",
    branch: "main",
    path: "/work/main",
  });
  const unpinnedClean = makeTree("clean-idle", { hasIgnored: true });
  const prunableMissing = makeTree("gone-reg", {
    prunable: true,
    available: false,
    dirty: null,
  });
  const dirty = makeTree("dirty-wt", { dirty: true, hasUntracked: true });
  const active = makeTree("active-wt");
  const locked = makeTree("locked-wt", {
    locked: true,
    lockReason: "deployment",
  });
  const unknown = makeTree("unknown-wt", { dirty: null, error: "git timeout" });

  test("lists all repository worktrees including unpinned idle ones and classifies categories", () => {
    const repo = makeRepo([
      dirty,
      unpinnedClean,
      main,
      prunableMissing,
      active,
      locked,
      unknown,
    ]);
    const entries = buildWorktreeBatchEntries(
      repo,
      {}, // Default curation: only main is pinned
      [
        makeSession("s-live", "/work/active-wt/subdir", "running"),
        makeSession("s-exited", "/work/clean-idle", "exited", "clean-idle"),
      ],
      [
        makeArchived("a-kept", "/work/clean-idle/pkg"),
        makeArchived("a-already-released", "/work/clean-idle", true),
      ],
    );

    expect(
      entries.map((e) => `${e.tree.id}:${e.category}:${e.pinned}`),
    ).toEqual([
      "main:protected:true",
      "active-wt:blocked:false",
      "clean-idle:safe:false",
      "dirty-wt:blocked:false",
      "gone-reg:safe:false",
      "locked-wt:protected:false",
      "unknown-wt:protected:false",
    ]);

    const cleanEntry = entries.find((e) => e.tree.id === "clean-idle")!;
    expect(cleanEntry.activeSessionCount).toBe(0);
    expect(cleanEntry.exitedSessionCount).toBe(1);
    expect(cleanEntry.archivedCount).toBe(1);

    const activeEntry = entries.find((e) => e.tree.id === "active-wt")!;
    expect(activeEntry.activeSessionCount).toBe(1);

    expect(countWorktreeBatchCategories(entries)).toEqual({
      all: 7,
      safe: 2,
      blocked: 2,
      protected: 3,
    });
  });

  test("filters by category and case-insensitive search query", () => {
    const repo = makeRepo([main, unpinnedClean, prunableMissing, dirty]);
    const entries = buildWorktreeBatchEntries(repo, {}, [], []);

    expect(
      filterWorktreeBatchEntries(entries, "safe", "").map((e) => e.tree.id),
    ).toEqual(["clean-idle", "gone-reg"]);
    expect(
      filterWorktreeBatchEntries(entries, "all", "GONE-REG").map(
        (e) => e.tree.id,
      ),
    ).toEqual(["gone-reg"]);
    expect(
      filterWorktreeBatchEntries(entries, "blocked", "clean").map(
        (e) => e.tree.id,
      ),
    ).toEqual([]);
  });

  test("groups preview items into safe, blocked, and unsupported", () => {
    const makeItem = (
      id: string,
      forceAllowed: boolean,
      blockers: string[],
    ): RemovalPreviewItem => ({
      worktree: makeTree(id),
      sessions: [],
      archived: [],
      commit: "a".repeat(40),
      push: { state: "noRemote", branches: [], more: 0 },
      blockers,
      warnings: [],
      forceAllowed,
    });

    const groups = groupBatchPreviewItems([
      makeItem("main", false, ["主工作区不能删除"]),
      makeItem("clean", true, []),
      makeItem("dirty", true, ["存在未提交修改"]),
    ]);
    expect(groups.safe.map((i) => i.worktree.id)).toEqual(["clean"]);
    expect(groups.blocked.map((i) => i.worktree.id)).toEqual(["dirty"]);
    expect(groups.unsupported.map((i) => i.worktree.id)).toEqual(["main"]);
  });

  test("stale registrations are the unlocked, idle linked worktrees whose directory is gone", () => {
    const lockedGone = makeTree("locked-gone", {
      locked: true,
      prunable: true,
      available: false,
      dirty: null,
    });
    const busyGone = makeTree("busy-gone", {
      prunable: true,
      available: false,
      dirty: null,
    });
    const present = makeTree("present", { prunable: true });
    const entries = buildWorktreeBatchEntries(
      makeRepo([main, prunableMissing, lockedGone, busyGone, present, dirty]),
      {},
      [makeSession("s1", busyGone.path, "running", busyGone.id)],
      [],
    );
    expect(staleRegistrations(entries).map((e) => e.tree.id)).toEqual([
      "gone-reg",
    ]);
    expect(entries.find((e) => e.tree.id === "locked-gone")?.reason).toBe(
      LOCKED_STALE_REASON,
    );
  });
});
