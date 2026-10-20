import { mkdir, writeFile } from "node:fs/promises";
import { resolve } from "node:path";
import { test, expect } from "./fixtures";

test("workspace UI audit: touch targets, overflow and session discovery", async ({
  page,
  request,
  workDir,
}, testInfo) => {
  test.skip(testInfo.project.name !== "chromium-desktop");
  const names = [
    "修复终端断线后的重连",
    "Review authentication changes",
    "long_session_name_without_spaces_".repeat(5),
  ];
  for (const name of names) {
    const response = await request.post("/api/sessions", {
      data: { cwd: workDir, preset: "shell", name },
    });
    expect(response.status()).toBe(201);
  }
  await page.goto("/");
  await expect(page.locator(".session-row")).toHaveCount(3);
  const measurements = [];
  const directory = process.env.WEBMUX_UI_AUDIT_DIR
    ? resolve(process.env.WEBMUX_UI_AUDIT_DIR)
    : testInfo.outputPath("audit");
  await mkdir(directory, { recursive: true });
  for (const theme of ["light", "dark"]) {
    for (const width of [320, 390, 1280]) {
      await page.setViewportSize({ width, height: 900 });
      await page.evaluate(async (theme) => {
        document.documentElement.classList.toggle("dark", theme === "dark");
        await document.fonts.ready;
      }, theme);
      const measurement = await page.evaluate(() => {
        const controls = [
          ...document.querySelectorAll<HTMLElement>(
            "header a, header button, main button",
          ),
        ].filter((element) => element.getClientRects().length);
        return {
          overflow: document.documentElement.scrollWidth > innerWidth + 1,
          smallTargets:
            innerWidth >= 640
              ? []
              : controls
                  .filter((element) => {
                    const box = element.getBoundingClientRect();
                    // Topbar controls use the same compact size as the terminal.
                    const minimum = element.closest("header") ? 28 : 44;
                    return box.height < minimum || box.width < minimum;
                  })
                  .map(
                    (element) =>
                      element.getAttribute("aria-label") ||
                      element.textContent?.trim(),
                  ),
          searchAvailable: Boolean(
            document.querySelector('input[aria-label="搜索会话"]'),
          ),
          statusFilterAvailable: Boolean(
            document.querySelector('[aria-label="按会话状态筛选"]'),
          ),
        };
      });
      measurements.push({ theme, width, ...measurement });
      if (width !== 320)
        await page.screenshot({
          path: resolve(directory, `${theme}-${width}.png`),
          fullPage: true,
          animations: "disabled",
        });
    }
  }
  const violations = measurements.reduce(
    (sum, row) =>
      sum +
      Number(row.overflow) +
      row.smallTargets.length +
      Number(!row.searchAvailable) +
      Number(!row.statusFilterAvailable),
    0,
  );
  await writeFile(
    resolve(directory, "metrics.json"),
    JSON.stringify({ violations, measurements }, null, 2),
  );
  console.log(`UI_AUDIT_VIOLATIONS=${violations}`);
  expect(violations, JSON.stringify(measurements, null, 2)).toBe(0);
});
