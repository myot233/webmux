import { expect, type Page, type TestInfo } from "@playwright/test";
import { writeFile } from "node:fs/promises";

type Shift = {
  value: number;
  time: number;
  recentInput: boolean;
  sources: string[];
};

type Metrics = {
  shifts: Shift[];
  interactions: number[];
  longTasks: number[];
};

declare global {
  interface Window {
    webmuxPerformance: Metrics;
  }
}

/** Install before navigation so cold startup and late fonts are included. */
export async function observePerformance(page: Page) {
  const cdp = await page.context().newCDPSession(page);
  await cdp.send("Emulation.setCPUThrottlingRate", { rate: 4 });
  await page.addInitScript(() => {
    window.webmuxPerformance = {
      shifts: [],
      interactions: [],
      longTasks: [],
    };
    new PerformanceObserver((list) => {
      for (const entry of list.getEntries()) {
        const shift = entry as PerformanceEntry & {
          value: number;
          hadRecentInput: boolean;
          sources: { node?: Node }[];
        };
        window.webmuxPerformance.shifts.push({
          value: shift.value,
          time: shift.startTime,
          recentInput: shift.hadRecentInput,
          sources: shift.sources.map(({ node }) =>
            node instanceof Element
              ? node.getAttribute("data-slot") ||
                node.getAttribute("class")?.slice(0, 120) ||
                node.nodeName
              : node?.nodeName || "removed node",
          ),
        });
      }
    }).observe({ type: "layout-shift", buffered: true });
    new PerformanceObserver((list) => {
      for (const entry of list.getEntries()) {
        const event = entry as PerformanceEntry & { interactionId: number };
        if (event.interactionId)
          window.webmuxPerformance.interactions.push(event.duration);
      }
    }).observe({ type: "event", buffered: true, durationThreshold: 16 });
    new PerformanceObserver((list) => {
      window.webmuxPerformance.longTasks.push(
        ...list.getEntries().map((entry) => entry.duration),
      );
    }).observe({ type: "longtask", buffered: true });
  });
  return cdp;
}

export async function settled(page: Page) {
  await page.evaluate(async () => {
    await document.fonts.ready;
    await new Promise<void>((resolve) =>
      requestAnimationFrame(() => requestAnimationFrame(() => resolve())),
    );
  });
  // Event Timing entries are delivered after the interaction's next paint.
  await page.waitForTimeout(100);
}

export async function performanceReport(
  page: Page,
  testInfo: TestInfo,
  phase: string,
  timings: Record<string, number> = {},
) {
  await settled(page);
  const metrics = await page.evaluate(() => window.webmuxPerformance);
  const shifts = metrics.shifts.filter((shift) => !shift.recentInput);
  let cls = 0;
  let session = 0;
  let first: number | undefined;
  let previous = 0;
  for (const shift of shifts) {
    if (
      first === undefined ||
      shift.time - previous > 1000 ||
      shift.time - first > 5000
    ) {
      session = 0;
      first = shift.time;
    }
    session += shift.value;
    cls = Math.max(cls, session);
    previous = shift.time;
  }
  const report = {
    phase,
    project: testInfo.project.name,
    cpuSlowdown: 4,
    cls,
    // Also catch small shifts in separate windows instead of rounding to zero.
    totalUnexpectedShift: shifts.reduce((sum, shift) => sum + shift.value, 0),
    maxInteractionMs: Math.max(0, ...metrics.interactions),
    maxLongTaskMs: Math.max(0, ...metrics.longTasks),
    timings,
    ...metrics,
  };
  await writeFile(
    testInfo.outputPath(`${phase}.json`),
    JSON.stringify(report, null, 2),
  );
  await testInfo.attach(`${phase}.json`, {
    body: JSON.stringify(report, null, 2),
    contentType: "application/json",
  });
  console.log(JSON.stringify(report));
  expect
    .soft(report.totalUnexpectedShift, `${phase}: unexpected layout shifts`)
    .toBe(0);
  expect
    .soft(report.maxInteractionMs, `${phase}: interaction to next paint`)
    .toBeLessThanOrEqual(200);
  return report;
}
