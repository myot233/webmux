import { describe, expect, test } from "vitest";
import {
  countSessions,
  hasUrgentAttention,
  isActive,
  matchesStatusFilter,
  sessionStatusTone,
  statusSummary,
  type SessionState,
} from "../src/domains/sessions/session-status";

const of = (...statuses: SessionState[]) =>
  statuses.map((status) => ({ status }));

describe("session status grouping", () => {
  test("treats running and idle as active, waiting and exited as not", () => {
    expect(isActive("running")).toBe(true);
    expect(isActive("idle")).toBe(true);
    expect(isActive("waiting")).toBe(false);
    expect(isActive("exited")).toBe(false);
  });

  test("counts an empty list as all zero", () => {
    expect(countSessions([])).toEqual({ active: 0, waiting: 0, exited: 0 });
    expect(statusSummary([])).toBe("运行 0 · 等待 0 · 退出 0");
  });

  test("puts each status in exactly one bucket", () => {
    expect(countSessions(of("running"))).toEqual({
      active: 1,
      waiting: 0,
      exited: 0,
    });
    expect(countSessions(of("idle"))).toEqual({
      active: 1,
      waiting: 0,
      exited: 0,
    });
    expect(countSessions(of("waiting"))).toEqual({
      active: 0,
      waiting: 1,
      exited: 0,
    });
    expect(countSessions(of("exited"))).toEqual({
      active: 0,
      waiting: 0,
      exited: 1,
    });
  });

  test("summarises a mixed list in the home-page wording", () => {
    const list = of("running", "idle", "waiting", "exited", "exited", "idle");
    expect(countSessions(list)).toEqual({ active: 3, waiting: 1, exited: 2 });
    expect(statusSummary(list)).toBe("运行 3 · 等待 1 · 退出 2");
  });
});

describe("session status filter", () => {
  const statuses: SessionState[] = ["running", "idle", "waiting", "exited"];

  test("all matches every status", () => {
    for (const status of statuses)
      expect(matchesStatusFilter(status, "all")).toBe(true);
  });

  test("active matches the same statuses counted as running", () => {
    expect(statuses.filter((s) => matchesStatusFilter(s, "active"))).toEqual([
      "running",
      "idle",
    ]);
  });

  test("waiting and exited match only themselves", () => {
    expect(statuses.filter((s) => matchesStatusFilter(s, "waiting"))).toEqual([
      "waiting",
    ]);
    expect(statuses.filter((s) => matchesStatusFilter(s, "exited"))).toEqual([
      "exited",
    ]);
  });
});

describe("session status tone", () => {
  const since = "2026-10-16T00:00:02Z";

  test("marks question, permission and error attention as urgent destructive", () => {
    for (const kind of ["question", "permission", "error"] as const) {
      const attention = { kind, source: "agent" as const, since };
      expect(hasUrgentAttention({ attention })).toBe(true);
      expect(
        sessionStatusTone({ status: "waiting", exitCode: null, attention }),
      ).toBe("destructive");
    }
  });

  test("keeps non-blocking attention and heuristic waiting on warning", () => {
    for (const kind of ["done", "notify", "bell"] as const) {
      const attention = { kind, source: "agent" as const, since };
      expect(hasUrgentAttention({ attention })).toBe(false);
      expect(
        sessionStatusTone({ status: "waiting", exitCode: null, attention }),
      ).toBe("warning");
    }
    expect(hasUrgentAttention({ attention: null })).toBe(false);
    expect(
      sessionStatusTone({ status: "waiting", exitCode: null, attention: null }),
    ).toBe("warning");
  });

  test("distinguishes running, idle, clean exit and failed exit", () => {
    expect(
      sessionStatusTone({ status: "running", exitCode: null, attention: null }),
    ).toBe("success");
    expect(
      sessionStatusTone({ status: "idle", exitCode: null, attention: null }),
    ).toBe("secondary");
    expect(
      sessionStatusTone({ status: "exited", exitCode: 0, attention: null }),
    ).toBe("secondary");
    expect(
      sessionStatusTone({ status: "exited", exitCode: null, attention: null }),
    ).toBe("secondary");
    expect(
      sessionStatusTone({ status: "exited", exitCode: 1, attention: null }),
    ).toBe("destructive");
  });
});
