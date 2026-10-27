import { writeFile } from "node:fs/promises";
import { join } from "node:path";
import type { Page } from "@playwright/test";
import { expect, expectConnected, test } from "./fixtures";

// Produces output for as long as the indicator should stay on; the file ends it.
const producer = String.raw`
while [ ! -e stop ]; do printf 'tick'; sleep 0.2; done
`;

const iconHref = (page: Page) =>
  page.locator('link[rel="icon"]').getAttribute("href");

test("a running session animates the tab chrome until it exits", async ({
  page,
  request,
  workDir,
}) => {
  let iconRequests = 0;
  page.on("request", (outgoing) => {
    if (new URL(outgoing.url()).pathname === "/icon.svg") iconRequests++;
  });
  const created = await request.post("/api/sessions", {
    data: {
      cwd: workDir,
      cmd: ["/bin/bash", "-c", producer],
      name: "E2E chrome",
    },
  });
  expect(created.status()).toBe(201);
  const { id } = await created.json();

  await page.goto("/");
  await expect(page).toHaveTitle("▶ WebMux · 工作区");
  await expect.poll(() => iconHref(page)).toMatch(/^data:image\/png/);
  const homeFrame = await iconHref(page);
  await expect
    .poll(() => iconHref(page), { timeout: 3000 })
    .not.toBe(homeFrame);

  await page.goto(`/s/${id}`);
  await expectConnected(page);
  await expect(page.locator(".restore-overlay")).toBeHidden();
  await expect(page).toHaveTitle("▶ E2E chrome · WebMux");
  await expect.poll(() => iconHref(page)).toMatch(/^data:image\/png/);
  const terminalFrame = await iconHref(page);
  await expect
    .poll(() => iconHref(page), { timeout: 3000 })
    .not.toBe(terminalFrame);
  // Frames are local data URLs; animation must not refetch the icon.
  const requestsBefore = iconRequests;
  await page.waitForTimeout(1500);
  expect(iconRequests).toBe(requestsBefore);

  await writeFile(join(workDir, "stop"), "");
  await expect(page).toHaveTitle("E2E chrome · WebMux");
  await expect.poll(() => iconHref(page)).toBe("/icon.svg");
});

test("reduced motion keeps a static running marker", async ({
  page,
  request,
  workDir,
}) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  const created = await request.post("/api/sessions", {
    data: {
      cwd: workDir,
      cmd: ["/bin/bash", "-c", producer],
      name: "E2E chrome reduced",
    },
  });
  expect(created.status()).toBe(201);

  await page.goto("/");
  await expect(page).toHaveTitle("▶ WebMux · 工作区");
  await expect.poll(() => iconHref(page)).toMatch(/^data:image\/png/);
  const frame = await iconHref(page);
  await page.waitForTimeout(1200);
  expect(await iconHref(page)).toBe(frame);
});
