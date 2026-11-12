import { expect, test } from "vitest";
import {
  applySessionEvent,
  unknownWorkspaces,
} from "../src/hooks/sessions/session-list";
import type {
  Repository,
  SessionInfo,
  Worktree,
} from "../src/contracts/schemas";

function session(id: string, second = 0): SessionInfo {
  return {
    id,
    generation: "0123456789abcdef",
    title: id,
    cwd: "/work",
    cmd: ["bash"],
    status: "running",
    exitCode: null,
    listeningPorts: [],
    state: "active",
    views: ["terminal"],
    view: "terminal",
    transition: null,
    agent: null,
    preview: "",
    createdAt: new Date(second * 1000).toISOString(),
    lastActiveAt: new Date().toISOString(),
    cols: 80,
    rows: 24,
    clients: 0,
    workspace: null,
    attention: null,
    reportedCwd: null,
  };
}

test("snapshot order uses creation time and deterministic ID ties without mutating input", () => {
  const input = [
    session("bbbbbbbb"),
    session("cccccccc", 1),
    session("aaaaaaaa"),
  ];
  const result = applySessionEvent([], {
    type: "snapshot",
    sessions: input,
    forwards: [],
  });
  expect(result.map((item) => item.id)).toEqual([
    "cccccccc",
    "aaaaaaaa",
    "bbbbbbbb",
  ]);
  expect(input.map((item) => item.id)).toEqual([
    "bbbbbbbb",
    "cccccccc",
    "aaaaaaaa",
  ]);
});

test("output updates preserve order, untouched row identity and previous snapshots", () => {
  const first = session("aaaaaaaa", 1);
  const second = session("bbbbbbbb");
  const input = [first, second];
  const updated = {
    ...second,
    preview: "new output",
    status: "waiting" as const,
  };
  const result = applySessionEvent(input, {
    type: "updated",
    session: updated,
  });
  expect(result).toEqual([first, updated]);
  expect(result[0]).toBe(first);
  expect(input[1]).toBe(second);
  expect(input[1]!.preview).toBe("");
});

test("creation and an update for an unknown ID insert into creation order", () => {
  let items = [session("aaaaaaaa", 2), session("cccccccc")];
  items = applySessionEvent(items, {
    type: "created",
    session: session("bbbbbbbb", 1),
  });
  items = applySessionEvent(items, {
    type: "updated",
    session: session("dddddddd", 3),
  });
  expect(items.map((item) => item.id)).toEqual([
    "dddddddd",
    "aaaaaaaa",
    "bbbbbbbb",
    "cccccccc",
  ]);
  items = applySessionEvent(items, {
    type: "created",
    session: session("bbbbbbbb", 1),
  });
  expect(items).toHaveLength(4);
});

test("changed creation times and timezone offsets retain chronological order", () => {
  const a = session("aaaaaaaa", 1);
  const b = session("bbbbbbbb");
  const result = applySessionEvent([a, b], {
    type: "updated",
    session: { ...b, createdAt: "1970-01-01T08:00:02+08:00" },
  });
  expect(result.map((item) => item.id)).toEqual([b.id, a.id]);
});

test("removal is idempotent and keeps other session objects intact", () => {
  const a = session("aaaaaaaa");
  const b = session("bbbbbbbb");
  const input = [a, b];
  expect(applySessionEvent(input, { type: "removed", id: "cccccccc" })).toBe(
    input,
  );
  const result = applySessionEvent(input, { type: "removed", id: a.id });
  expect(result).toEqual([b]);
  expect(result[0]).toBe(b);
  expect(input).toEqual([a, b]);
});

function repository(id: string, worktreeIds: string[]): Repository {
  return {
    id,
    name: id,
    commonDir: `/work/${id}/.git`,
    anchorPath: `/work/${id}`,
    availability: "available",
    error: "",
    checkedAt: "2026-10-20T00:00:00Z",
    lastFetchedAt: null,
    initInstruction: "",
    worktrees: worktreeIds.map(
      (worktreeId) => ({ id: worktreeId }) as Worktree,
    ),
  };
}

function inWorkspace(id: string, repositoryId: string, worktreeId: string) {
  return {
    ...session(id),
    workspace: { repositoryId, worktreeId, worktreePath: "/work" },
  };
}

test("unknownWorkspaces names worktrees and repositories the list has not read yet", () => {
  const repositories = [repository("r1", ["w1"])];
  expect(
    unknownWorkspaces(
      [
        inWorkspace("aaaaaaaa", "r1", "w1"),
        inWorkspace("bbbbbbbb", "r1", "w2"),
        inWorkspace("cccccccc", "r1", "w2"),
        inWorkspace("dddddddd", "r2", "w3"),
        session("eeeeeeee"),
      ],
      repositories,
    ),
  ).toEqual(["w:w2", "r:r2"]);
  expect(
    unknownWorkspaces([inWorkspace("aaaaaaaa", "r1", "w1")], repositories),
  ).toEqual([]);
});
