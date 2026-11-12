import { expect, test } from "vitest";
import {
  resolveInitialLaunch,
  resolveInitialTarget,
  resolveLaunchScope,
} from "../src/domains/sessions/new-session-defaults";
import type { LaunchConfigsState } from "../src/domains/settings/launch-configs";
import type { Repository, Worktree } from "../src/contracts/schemas";

function tree(
  repositoryId: string,
  id: string,
  extra: Partial<Worktree> = {},
): Worktree {
  return {
    id,
    repositoryId,
    path: `/repos/${repositoryId}/${id}`,
    kind: "linked",
    branch: id,
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
    ...extra,
  };
}
function repo(id: string, worktrees: Worktree[]): Repository {
  return {
    id,
    name: id,
    commonDir: `/repos/${id}/.git`,
    anchorPath: `/repos/${id}`,
    availability: "available",
    error: "",
    checkedAt: "2026-11-04T00:00:00Z",
    lastFetchedAt: null,
    initInstruction: "",
    worktrees,
  };
}
const a = repo("a", [
  tree("a", "feature"),
  tree("a", "main", { kind: "main" }),
]);
const b = repo("b", [tree("b", "main", { kind: "main" })]);
const base = {
  repositories: [a, b],
  recent: ["/repos/b/main/src"],
  defaultCwd: "/home",
};

test("the selected repository's main worktree wins over a recent directory of another project", () => {
  expect(resolveInitialTarget({ ...base, selectedRepositoryId: "a" })).toEqual({
    kind: "worktree",
    cwd: "/repos/b/main/src",
    repositoryId: "a",
    worktreeId: "main",
  });
});

test("an unavailable main worktree falls back to another usable tree, skipping bare ones", () => {
  const repository = repo("a", [
    tree("a", "main", { kind: "main", available: false }),
    tree("a", "bare", { kind: "bare" }),
    tree("a", "feature"),
  ]);
  expect(
    resolveInitialTarget({
      ...base,
      repositories: [repository],
      selectedRepositoryId: "a",
    }).worktreeId,
  ).toBe("feature");
});

test("a repository without usable targets stays selected but unset", () => {
  const repository = repo("a", [
    tree("a", "main", { kind: "main", available: false }),
    tree("a", "bare", { kind: "bare" }),
  ]);
  expect(
    resolveInitialTarget({
      ...base,
      repositories: [repository],
      selectedRepositoryId: "a",
    }),
  ).toMatchObject({ kind: "worktree", repositoryId: "a", worktreeId: "" });
});

test("all-repositories and direct views use the recent directory, then the default", () => {
  for (const selectedRepositoryId of ["all", "direct", ""])
    expect(resolveInitialTarget({ ...base, selectedRepositoryId })).toEqual({
      kind: "directory",
      cwd: "/repos/b/main/src",
      repositoryId: "",
      worktreeId: "",
    });
  expect(resolveInitialTarget({ ...base, recent: [] }).cwd).toBe("/home");
});

test("a contextual entry overrides the selected repository", () => {
  expect(
    resolveInitialTarget({
      ...base,
      selectedRepositoryId: "a",
      entry: { kind: "worktree", repositoryId: "b", worktreeId: "main" },
    }),
  ).toMatchObject({ kind: "worktree", repositoryId: "b", worktreeId: "main" });
  expect(
    resolveInitialTarget({
      ...base,
      selectedRepositoryId: "a",
      entry: { kind: "new-worktree", repositoryId: "b" },
    }),
  ).toMatchObject({ kind: "new-worktree", repositoryId: "b" });
  expect(
    resolveInitialTarget({
      ...base,
      selectedRepositoryId: "a",
      entry: {
        kind: "directory",
        cwd: "/repos/a/main/pkg",
        workspace: {
          repositoryId: "a",
          worktreeId: "main",
          worktreePath: "/repos/a/main",
        },
      },
    }),
  ).toEqual({
    kind: "directory",
    cwd: "/repos/a/main/pkg",
    repositoryId: "a",
    worktreeId: "",
  });
});

const configs = (...ids: string[]): LaunchConfigsState => ({
  status: "ready",
  configs: ids.map((id) => ({
    id,
    name: id,
    cmd: [id],
    envNames: [],
    chatOptions: [],
    chat: false,
    chatSupport: {
      supported: false,
      supportedAgents: [],
      reason: "unknown_agent",
    },
    createdAt: "2026-11-04T00:00:00Z",
    updatedAt: "2026-11-04T00:00:00Z",
  })),
});

test("a remembered config is reused only while it exists, never replaced", () => {
  expect(resolveInitialLaunch("config:two", configs("one", "two"))).toEqual({
    source: "config",
    configId: "two",
  });
  expect(resolveInitialLaunch("config:gone", configs("one"))).toEqual({
    source: "unset",
    configId: "",
  });
  expect(
    resolveInitialLaunch("config:two", { status: "unavailable", error: "x" }),
  ).toEqual({ source: "unset", configId: "" });
});

test("without a memory the first config is used, else a custom command", () => {
  expect(resolveInitialLaunch("", configs("one", "two"))).toEqual({
    source: "config",
    configId: "one",
  });
  expect(resolveInitialLaunch("config:", configs())).toEqual({
    source: "custom",
    configId: "",
  });
  expect(
    resolveInitialLaunch("", { status: "unavailable", error: "x" }),
  ).toEqual({ source: "custom", configId: "" });
});

test("a remembered custom command applies before configs load; configs wait", () => {
  const loading: LaunchConfigsState = { status: "loading" };
  expect(resolveInitialLaunch("custom", loading)).toEqual({
    source: "custom",
    configId: "",
  });
  expect(resolveInitialLaunch("config:one", loading)).toBeNull();
  expect(resolveInitialLaunch("", loading)).toBeNull();
});

test("a worktree scope starts in that worktree; an unusable one asks for a new worktree", () => {
  const scope = { repositoryId: "a", worktreeId: "feature" };
  expect(resolveInitialTarget({ ...base, scope })).toMatchObject({
    kind: "worktree",
    repositoryId: "a",
    worktreeId: "feature",
  });
  expect(
    resolveInitialTarget({
      ...base,
      scope: { repositoryId: "a", worktreeId: "gone" },
    }),
  ).toMatchObject({ kind: "new-worktree", repositoryId: "a", worktreeId: "" });
});

test("the launch scope follows the home view or the contextual entry", () => {
  const home = { home: true, repositories: [a, b], worktreeId: null };
  expect(resolveLaunchScope({ ...home, choice: "a" })).toEqual({
    repositoryId: "a",
  });
  expect(
    resolveLaunchScope({ ...home, choice: "a", worktreeId: "feature" }),
  ).toEqual({ repositoryId: "a", worktreeId: "feature" });
  // A worktree of another repository narrows nothing.
  expect(
    resolveLaunchScope({ ...home, choice: "b", worktreeId: "feature" }),
  ).toEqual({ repositoryId: "b" });
  for (const choice of ["all", "direct", null, "gone"])
    expect(resolveLaunchScope({ ...home, choice })).toBeUndefined();
  expect(
    resolveLaunchScope({ ...home, choice: "a", home: false }),
  ).toBeUndefined();
  expect(
    resolveLaunchScope({
      ...home,
      choice: "all",
      entry: { kind: "worktree", repositoryId: "b", worktreeId: "main" },
    }),
  ).toEqual({ repositoryId: "b", worktreeId: "main" });
  expect(
    resolveLaunchScope({
      ...home,
      choice: "a",
      entry: { kind: "directory", cwd: "/x", workspace: null },
    }),
  ).toBeUndefined();
});
