import { expect, test, type Locator, type Page } from "@playwright/test";
import { overflowViolations } from "./overflow";

const viewports = [
  { name: "narrow", width: 320, height: 568 },
  { name: "mobile", width: 390, height: 844 },
  { name: "tablet", width: 768, height: 800 },
  { name: "desktop", width: 1280, height: 900 },
  { name: "short", width: 667, height: 320 },
  { name: "large-text", width: 320, height: 568, textScale: 2 },
];

type Layout = {
  name: string;
  viewport: { width: number; height: number };
  theme: "light" | "dark";
  textScale: number;
};

const layouts: Layout[] = viewports.flatMap(
  ({ name, width, height, textScale }) =>
    (["light", "dark"] as const).map((theme) => ({
      name: `${name}-${theme}`,
      viewport: { width, height },
      theme,
      textScale: textScale ?? 1,
    })),
);

export async function expectNoOverflow(page: Page, name: string) {
  await expect.soft
    .poll(() => overflowViolations(page), { message: name, timeout: 1_500 })
    .toEqual([]);
}

/**
 * Asserts the dialog lies within the viewport. Its height follows dvh,
 * which can settle a frame after a viewport change on a loaded CI runner.
 */
export async function expectDialogFits(
  page: Page,
  dialog: Locator,
  name: string,
) {
  const viewport = page.viewportSize()!.height;
  await expect.soft
    .poll(
      async () => {
        const bounds = await dialog.boundingBox();
        return (
          bounds !== null &&
          bounds.y >= -1 &&
          bounds.y + bounds.height <= viewport + 1
        );
      },
      { message: name, timeout: 2_000 },
    )
    .toBe(true);
}

export async function checkEachLayout(
  page: Page,
  check: (layout: Layout) => Promise<void>,
) {
  for (const layout of layouts) {
    await test.step(layout.name, async () => {
      await page.setViewportSize(layout.viewport);
      await page.emulateMedia({ colorScheme: layout.theme });
      await page.evaluate(
        async ({ dark, scale }) => {
          document.documentElement.classList.toggle("dark", dark);
          document.documentElement.style.fontSize = `${scale * 100}%`;
          await document.fonts.ready;
          await new Promise<void>((resolve) =>
            requestAnimationFrame(() => requestAnimationFrame(() => resolve())),
          );
        },
        { dark: layout.theme === "dark", scale: layout.textScale },
      );
      try {
        await check(layout);
      } catch (error) {
        expect
          .soft(
            error instanceof Error
              ? `${error.name}: ${error.message}`
              : String(error),
            layout.name,
          )
          .toBe("");
      } finally {
        if (layout.textScale !== 1) {
          await page.evaluate(() => {
            document.documentElement.style.fontSize = "100%";
          });
        }
      }
    });
  }
}
