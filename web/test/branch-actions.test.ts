import { describe, expect, it } from "vitest";
import {
  branchActions,
  branchOperation,
  cleanupCandidates,
  cleanupOperation,
  operationTitle,
  restoreCommand,
} from "../src/domains/workspace/branch-actions";
import type { Branch, Worktree } from "../src/contracts/schemas";

function branch(patch: Partial<Branch> = {}): Branch {
  return {
    name: "feat/a",
    head: "a".repeat(40),
    worktreePaths: [],
    subject: "",
    committedAt: "2026-10-28T00:00:00Z",
    upstream: "origin/feat/a",
    ahead: 0,
    behind: 0,
    upstreamGone: false,
    mergedIntoDefault: false,
    ...patch,
  };
}

function tree(patch: Partial<Worktree> = {}): Worktree {
  return {
    id: "w1",
    repositoryId: "r1",
    path: "/repo.worktrees/a",
    kind: "linked",
    branch: "feat/a",
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
    upstream: "origin/feat/a",
    ahead: 0,
    behind: 0,
    upstreamGone: false,
    lastCommit: null,
    changes: { staged: 0, unstaged: 0, untracked: 0, conflicted: 0 },
    operation: null,
    ...patch,
  };
}

describe("branchActions", () => {
  // design §6: one row per state, for a worktree and a branch row (which
  // adds deletion when nothing has it checked out).
  const rows: [string, Partial<Branch>, string[], boolean][] = [
    ["behind only", { ahead: 0, behind: 3 }, ["fast-forward"], false],
    ["ahead only", { ahead: 2, behind: 0 }, ["push"], false],
    [
      "no upstream",
      { upstream: null, ahead: null, behind: null },
      ["publish"],
      false,
    ],
    [
      "upstream gone",
      { upstreamGone: true, ahead: null, behind: null },
      ["publish"],
      false,
    ],
    ["diverged", { ahead: 1, behind: 2 }, [], true],
    [
      "uncounted",
      { ahead: null, behind: null },
      ["fast-forward", "push"],
      false,
    ],
    ["in sync", { ahead: 0, behind: 0 }, [], false],
  ];
  for (const [name, state, actions, diverged] of rows) {
    it(`${name}: a worktree's branch`, () => {
      expect(branchActions(tree(state))).toEqual({
        branch: "feat/a",
        actions,
        diverged,
      });
    });
    it(`${name}: a branch row adds deletion when not checked out`, () => {
      expect(branchActions(branch(state))).toEqual({
        branch: "feat/a",
        actions: [...actions, "delete"],
        diverged,
      });
      expect(
        branchActions(branch({ ...state, worktreePaths: ["/repo"] })).actions,
      ).toEqual(actions);
    });
  }

  const none: [string, Partial<Worktree>][] = [
    ["detached", { detached: true, branch: null }],
    ["bare", { kind: "bare", branch: null, head: null }],
    ["unavailable", { available: false, changes: null }],
    ["prunable", { prunable: true, available: false }],
    ["without commits", { head: null }],
  ];
  for (const [name, patch] of none)
    it(`a ${name} worktree offers nothing`, () => {
      // Even a state that would offer an action elsewhere.
      expect(branchActions(tree({ ahead: 0, behind: 3, ...patch }))).toEqual({
        branch: null,
        actions: [],
        diverged: false,
      });
    });
});

describe("branch operations", () => {
  it("maps actions to flows and titles", () => {
    expect(branchOperation("fast-forward", "b")).toEqual({
      kind: "fast-forward",
      branch: "b",
    });
    expect(operationTitle(branchOperation("push", "b"))).toBe("推送分支");
    expect(operationTitle(branchOperation("publish", "b"))).toBe("发布分支");
    expect(branchOperation("delete", "b")).toEqual({
      kind: "delete",
      branches: ["b"],
      cleanup: false,
    });
  });

  it("offers cleanup for unchecked-out gone or merged branches", () => {
    const candidates = cleanupCandidates([
      branch({ name: "merged", mergedIntoDefault: true }),
      branch({ name: "gone", upstreamGone: true }),
      branch({
        name: "checked-out",
        mergedIntoDefault: true,
        worktreePaths: ["/repo"],
      }),
      branch({ name: "open" }),
    ]);
    expect(candidates).toEqual(["merged", "gone"]);
    const many = Array.from({ length: 60 }, (_, i) => `b${i}`);
    expect(cleanupOperation(many).branches).toHaveLength(50);
  });

  it("builds the restore command", () => {
    expect(restoreCommand("feat/old", "abc")).toBe("git branch feat/old abc");
  });
});
