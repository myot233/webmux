import { test, expect } from "../e2e/fixtures";
import { observePerformance, performanceReport, settled } from "./performance";
import type { SessionInfo } from "../../src/schemas";

test("4x CPU: a busy session among 120 cards keeps order, filtering and input responsive", async ({
  page,
}, testInfo) => {
  await observePerformance(page);
  const alphabet = "abcdefghijklmnopqrstuvwxyz234567";
  const sessions: SessionInfo[] = Array.from({ length: 120 }, (_, index) => ({
    id: "aaaaaa" + alphabet[Math.floor(index / 32)] + alphabet[index % 32],
    generation: "0123456789abcdef",
    title: `load-${index}`,
    cwd: "/work",
    preset: "shell",
    cmd: ["bash"],
    status: "running",
    exitCode: null,
    preview: `ready-${index}\nstatus`,
    createdAt: new Date(Date.UTC(2026, 9, 24, 0, 0, index)).toISOString(),
    lastActiveAt: new Date().toISOString(),
    cols: 80,
    rows: 24,
    clients: 0,
    workspace: null,
    attention: null,
    reportedCwd: null,
  }));
  let update: (session: SessionInfo) => void = () => {
    throw new Error("Missing list socket");
  };
  await page.routeWebSocket("**/ws/sessions*", (socket) => {
    socket.send(JSON.stringify({ type: "snapshot", sessions }));
    update = (session) =>
      socket.send(JSON.stringify({ type: "updated", session }));
  });
  await page.goto("/");
  await expect(page.locator("[data-session-id]")).toHaveCount(120);
  const order = () =>
    page
      .locator("[data-session-id]")
      .evaluateAll((rows) =>
        rows.map((row) => row.getAttribute("data-session-id")),
      );
  const before = await order();
  await settled(page);
  // This scenario measures steady-state list updates. Cold startup and delayed
  // fonts are covered separately by performance.spec.ts using real sessions.
  await page.evaluate(() => {
    window.webmuxPerformance.shifts = [];
    window.webmuxPerformance.interactions = [];
    window.webmuxPerformance.interactionEvents = [];
    window.webmuxPerformance.longTasks = [];
  });
  const started = performance.now();
  for (let index = 0; index < 60; index++) {
    update({
      ...sessions[31],
      preview: `updated-${index}\nstatus`,
      status: "waiting",
    });
  }
  const row = page.locator(`[data-session-id="${sessions[31].id}"]`);
  await expect(row.getByLabel("终端输出预览")).toContainText("updated-59");
  const updateBurstMs = performance.now() - started;
  expect(await order()).toEqual(before);
  await expect(row).toHaveAttribute("data-status", "waiting");
  const search = page.getByRole("searchbox", { name: "搜索会话" });
  // Real key events carry the same recent-input attribution as manual typing.
  await search.click();
  await page.keyboard.type("load-31");
  await expect(page.locator("[data-session-id]")).toHaveCount(1);
  await expect(
    page.getByRole("link", { name: "load-31", exact: true }),
  ).toBeVisible();
  await search.press("Escape");
  await expect(page.locator("[data-session-id]")).toHaveCount(120);
  expect(await order()).toEqual(before);
  const report = await performanceReport(
    page,
    testInfo,
    "many-session-updates",
    {
      updateBurstMs,
    },
  );
  expect(report.maxInteractionMs).toBeLessThanOrEqual(200);
});
