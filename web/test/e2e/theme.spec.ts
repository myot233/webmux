import { type Page } from "@playwright/test";
import { test, expect, revealSidebar } from "./fixtures";

async function openThemeControl(page: Page) {
  await page.goto("/");
  await page.waitForFunction(() =>
    localStorage.getItem("webmux.settingsCache"),
  );
  await revealSidebar(page);
  return page.getByRole("button", { name: /^切换为[黑白]色主题$/ });
}

test("both themes reveal from the button center and keep keyboard focus", async ({
  page,
  request,
}) => {
  await page.emulateMedia({
    colorScheme: "light",
    reducedMotion: "no-preference",
  });
  const button = await openThemeControl(page);
  const root = page.locator("html");

  await button.click({ trial: true });
  for (const theme of ["dark", "light"]) {
    const box = await button.boundingBox();
    if (!box) throw new Error("Theme button is not visible");
    const { x, y, width, height } = box;
    const [origin] = await Promise.all([
      page.waitForFunction(() => {
        const active = document
          .getAnimations()
          .some(
            (animation) =>
              animation instanceof CSSAnimation &&
              animation.animationName === "theme-reveal",
          );
        return (
          active &&
          document.documentElement.style.getPropertyValue(
            "--theme-reveal-origin",
          )
        );
      }),
      button.press("Enter"),
    ]);
    expect(await origin.jsonValue()).toBe(
      `${x + width / 2}px ${y + height / 2}px`,
    );
    await expect(button).toHaveAccessibleName(
      theme === "dark" ? "切换为白色主题" : "切换为黑色主题",
    );
    await expect(button).toBeFocused();
    await expect(root).not.toHaveAttribute("data-theme-transition");
    await expect
      .poll(async () => {
        const response = await request.get("/api/settings");
        return (await response.json()).values.theme;
      })
      .toBe(theme);
  }

  await button.evaluate((button: HTMLButtonElement) => {
    button.click();
    button.click();
  });
  await expect(root).not.toHaveAttribute("data-theme-transition");
  await expect(button).toHaveAccessibleName("切换为黑色主题");
});

for (const fallback of ["reduced motion", "unsupported browser"]) {
  test(`theme switches without animation with ${fallback}`, async ({
    page,
  }) => {
    await page.emulateMedia({
      colorScheme: "light",
      reducedMotion: fallback === "reduced motion" ? "reduce" : "no-preference",
    });
    if (fallback === "unsupported browser") {
      await page.addInitScript(() => {
        Object.defineProperty(document, "startViewTransition", {
          value: undefined,
        });
      });
    }

    const button = await openThemeControl(page);
    const result = await button.evaluate((button: HTMLButtonElement) => {
      button.click();
      return {
        dark: document.documentElement.classList.contains("dark"),
        animated: document.documentElement.hasAttribute(
          "data-theme-transition",
        ),
      };
    });
    expect(result).toEqual({ dark: true, animated: false });
  });
}
