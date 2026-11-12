import { describe, expect, test } from "vitest";
import {
  placeArchived,
  recordWorktree,
  releasableRecordWorktree,
  removableRecordWorktree,
  repositoryRecords,
} from "../src/domains/sessions/archived";
import type {
  ArchivedSession,
  Repository,
  Worktree,
} from "../src/contracts/schemas";

const tree = (id: string, path: string): Worktree => ({
  id,
  repositoryId: "r1",
  path,
  kind: "linked",
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
  upstream: null,
  ahead: null,
  behind: null,
  upstreamGone: false,
  lastCommit: null,
  changes: { staged: 0, unstaged: 0, untracked: 0, conflicted: 0 },
  operation: null,
});
const repository: Repository = {
  id: "r1",
  name: "project",
  commonDir: "/work/main/.git",
  anchorPath: "/work/main",
  availability: "available",
  error: "",
  checkedAt: "2026-10-20T00:00:00Z",
  lastFetchedAt: null,
  initInstruction: "",
  worktrees: [tree("main", "/work/main"), tree("nested", "/work/main/nested")],
};
const record = (
  id: string,
  patch: Partial<ArchivedSession>,
): ArchivedSession => ({
  id,
  generation: "",
  title: id,
  cwd: "/elsewhere",
  cmd: ["omp"],
  status: "exited",
  exitCode: null,
  preview: "",
  createdAt: "2026-10-28T00:00:00Z",
  lastActiveAt: "2026-10-28T00:00:00Z",
  cols: 0,
  rows: 0,
  clients: 0,
  workspace: null,
  attention: null,
  reportedCwd: null,
  configId: "cfgcfgcf",
  state: "closed",
  views: ["terminal", "chat"],
  view: "terminal",
  transition: null,
  agent: { name: "omp", title: "omp", icon: "pi", sessionId: null },
  archive: {
    configMissing: false,
    previousAgentSessionIds: [],
    queuedCount: 0,
    interruptedAt: null,
    updatedAt: "2026-10-28T00:00:00Z",
  },
  listeningPorts: [],
  ...patch,
});
const ids = (records: readonly ArchivedSession[] = []) =>
  records.map((r) => r.id);

describe("placeArchived", () => {
  const records = [
    // The recorded association wins over the directory.
    record("recorded", {
      cwd: "/work/main/nested",
      workspace: {
        repositoryId: "r1",
        worktreeId: "main",
        worktreePath: "/work/main/nested",
      },
    }),
    record("gone", {
      cwd: "/work/gone",
      workspace: {
        repositoryId: "r1",
        worktreeId: "gone",
        worktreePath: "/work/gone",
      },
    }),
    // Legacy records join the deepest worktree containing their directory.
    record("deep", { cwd: "/work/main/nested/src/" }),
    record("root", { cwd: "/work/main" }),
    // A sibling path with the same prefix is not inside the worktree.
    record("sibling", { cwd: "/work/main-copy" }),
    record("removed", {
      cwd: "/work/main",
      workspace: {
        repositoryId: "r9",
        worktreeId: "main",
        worktreePath: "/work/main",
      },
    }),
  ];

  test("places every record exactly once", () => {
    const placed = placeArchived(records, [repository]);
    const entry = placed.repositories.get("r1")!;
    expect(ids(entry.worktrees.get("main"))).toEqual(["recorded", "root"]);
    expect(ids(entry.worktrees.get("nested"))).toEqual(["deep"]);
    expect(ids(entry.missing)).toEqual(["gone"]);
    expect(ids(placed.direct)).toEqual(["sibling", "removed"]);
    expect(repositoryRecords(entry).length + placed.direct.length).toBe(
      records.length,
    );
  });

  test("without repositories, as when Git cannot be read, nothing is lost", () => {
    const placed = placeArchived(records, []);
    expect(placed.repositories.size).toBe(0);
    expect(ids(placed.direct)).toEqual(ids(records));
  });

  test("resolves removable linked worktrees for archived records", () => {
    const repos: Repository[] = [
      {
        ...repository,
        worktrees: [
          { ...tree("main", "/work/main"), kind: "main" },
          tree("nested", "/work/main/nested"),
          { ...tree("prunable", "/work/prunable"), prunable: true },
        ],
      },
    ];
    expect(recordWorktree(records[0]!, repos)?.id).toBe("main");
    expect(removableRecordWorktree(records[0]!, repos)).toBeUndefined();
    expect(removableRecordWorktree(records[1]!, repos)).toBeUndefined();
    expect(removableRecordWorktree(records[2]!, repos)?.id).toBe("nested");
    expect(
      removableRecordWorktree(
        record("stale", {
          cwd: "/work/prunable",
          workspace: {
            repositoryId: "r1",
            worktreeId: "prunable",
            worktreePath: "/work/prunable",
          },
        }),
        repos,
      ),
    ).toBeUndefined();
  });

  test("offers releasing only linked worktrees of records not yet released", () => {
    const repos: Repository[] = [
      {
        ...repository,
        worktrees: [
          { ...tree("main", "/work/main"), kind: "main" },
          tree("feature", "/work/feature"),
          { ...tree("prunable", "/work/prunable"), prunable: true },
          { ...tree("offline", "/work/offline"), available: false },
        ],
      },
    ];
    const at = (worktreeId: string, patch: Partial<ArchivedSession> = {}) =>
      record(worktreeId, {
        cwd: `/work/${worktreeId}`,
        workspace: {
          repositoryId: "r1",
          worktreeId,
          worktreePath: `/work/${worktreeId}`,
        },
        ...patch,
      });
    expect(releasableRecordWorktree(at("feature"), repos)?.id).toBe("feature");
    // The main checkout, a prunable or unavailable directory, a worktree no
    // longer in the Git list and a direct directory never offer it.
    for (const id of ["main", "prunable", "offline", "gone"])
      expect(releasableRecordWorktree(at(id), repos), id).toBeUndefined();
    expect(
      releasableRecordWorktree(record("direct", { cwd: "/tmp" }), repos),
    ).toBeUndefined();
    // A released record has nothing left to release, and deleting it only
    // deletes the record, even when a worktree is back at its path.
    const released = at("feature", {
      released: {
        id: "rel1",
        repositoryId: "r1",
        path: "/work/feature",
        commit: "c".repeat(40),
        branch: "feature",
        releasedAt: "2026-10-28T00:00:00Z",
      },
    });
    expect(releasableRecordWorktree(released, repos)).toBeUndefined();
    expect(removableRecordWorktree(released, repos)).toBeUndefined();
  });
});
