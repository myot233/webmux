import { afterEach, expect, test } from "vitest";
import { page } from "vitest/browser";
import { cleanup, render } from "vitest-browser-svelte/pure";
import SessionRow from "../../src/components/SessionRow.svelte";
import type { SessionInfo } from "../../src/schemas";

afterEach(() => cleanup());

const base: SessionInfo = {
  id: "abcdefgh",
  generation: "0123456789abcdef",
  title: "omp agent",
  cwd: "/work",
  preset: "omp",
  cmd: ["omp"],
  status: "waiting",
  exitCode: null,
  preview: "⠋ Working",
  createdAt: "2026-10-16T00:00:00Z",
  lastActiveAt: "2026-10-16T00:00:01Z",
  cols: 80,
  rows: 24,
  clients: 0,
  workspace: null,
  attention: null,
};

test("an agent permission request is labelled precisely", async () => {
  render(SessionRow, {
    session: {
      ...base,
      attention: {
        kind: "permission",
        source: "agent",
        since: "2026-10-16T00:00:02Z",
      },
    },
    now: Date.parse("2026-10-16T00:00:03Z"),
    onclose: () => {},
  });
  await expect.element(page.getByText("等待授权")).toBeVisible();
  expect(page.getByText("可能在等你").elements()).toHaveLength(0);
  expect(
    document.querySelector(
      '[data-status="waiting"] [data-slot="session-status"][data-tone="warning"]',
    ),
  ).not.toBeNull();
});

test("heuristic waiting keeps the cautious label", async () => {
  render(SessionRow, {
    session: base,
    now: Date.parse("2026-10-16T00:00:03Z"),
    onclose: () => {},
  });
  await expect.element(page.getByText("可能在等你")).toBeVisible();
});
