import { mergeConfig } from "vite";
import { defineConfig } from "vitest/config";
import { defineBrowserCommand, playwright } from "@vitest/browser-playwright";
import viteConfig from "./vite.config";

// Reuse the production compiler, Tailwind and aliases, but never its API proxy.
const { server: _applicationServer, ...sharedConfig } = viteConfig;

// nativeTouchGesture accepts CSS-pixel points in the test iframe's viewport.
// The first point starts one real Chromium finger, subsequent points move it,
// and the command releases it (or cancels when requested). CDP coordinates are
// translated to the top-level page; emulation and the session are always
// released, including on failure.
const nativeTouchGesture = defineBrowserCommand(
  async (
    { page, frame },
    points: { x: number; y: number }[],
    finish: "end" | "cancel" = "end",
    holdMilliseconds = 0,
  ) => {
    const testFrame = await frame();
    const bounds = await (await testFrame.frameElement()).boundingBox();
    if (!bounds || !points.length)
      throw new Error("Missing touch gesture target");
    const viewport = await testFrame.evaluate(() => ({
      width: innerWidth,
      height: innerHeight,
    }));
    const cdp = await page.context().newCDPSession(page);
    try {
      await cdp.send("Emulation.setTouchEmulationEnabled", {
        enabled: true,
        maxTouchPoints: 2,
      });
      for (const [index, point] of points.entries()) {
        await cdp.send("Input.dispatchTouchEvent", {
          type: index === 0 ? "touchStart" : "touchMove",
          touchPoints: [
            {
              x: bounds.x + (point.x * bounds.width) / viewport.width,
              y: bounds.y + (point.y * bounds.height) / viewport.height,
              id: 1,
            },
          ],
        });
        if (index === 0 && holdMilliseconds > 0) {
          const { promise, resolve } = Promise.withResolvers<void>();
          setTimeout(resolve, holdMilliseconds);
          await promise;
        }
        // Real frames give Chrome time to recognize a native pan/cancel instead
        // of collapsing a whole swipe into one synchronous DOM event burst.
        await testFrame.evaluate(
          () =>
            new Promise<void>((resolve) =>
              requestAnimationFrame(() =>
                requestAnimationFrame(() => resolve()),
              ),
            ),
        );
      }
      await cdp.send("Input.dispatchTouchEvent", {
        type: finish === "cancel" ? "touchCancel" : "touchEnd",
        touchPoints: [],
      });
      await testFrame.evaluate(
        () =>
          new Promise<void>((resolve) =>
            requestAnimationFrame(() => requestAnimationFrame(() => resolve())),
          ),
      );
    } finally {
      await cdp.send("Emulation.setTouchEmulationEnabled", { enabled: false });
      await cdp.detach();
    }
  },
);

export default mergeConfig(
  sharedConfig,
  defineConfig({
    optimizeDeps: {
      include: ["@xterm/xterm", "@xterm/addon-fit", "@xterm/addon-webgl"],
    },
    test: {
      include: ["test/browser/**/*.test.ts"],
      setupFiles: ["./test/browser/setup.ts"],
      browser: {
        enabled: true,
        headless: true,
        provider: playwright(),
        instances: [{ browser: "chromium" }],
        viewport: { width: 1024, height: 900 },
        commands: { nativeTouchGesture },
      },
    },
  }),
);
