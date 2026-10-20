import { defineConfig } from "@playwright/test";
import { once } from "node:events";
import { createServer } from "node:net";

let previewPort: number;
if (process.env.TEST_WORKER_INDEX === undefined) {
  // Vite treats port 0 as its default. Reserve an OS-assigned loopback port,
  // release it, then require the preview server to bind exactly that port.
  const reservation = createServer();
  reservation.listen(0, "127.0.0.1");
  await once(reservation, "listening");
  const address = reservation.address();
  if (!address || typeof address === "string")
    throw new Error("Failed to reserve a Storybook preview port");
  previewPort = address.port;
  const released = once(reservation, "close");
  reservation.close();
  await released;
  // Playwright loads this config again in each worker; inherit the runner's port.
  process.env.WEBMUX_STORYBOOK_TEST_PORT = String(previewPort);
} else {
  previewPort = Number(process.env.WEBMUX_STORYBOOK_TEST_PORT);
  if (!Number.isInteger(previewPort) || previewPort < 1 || previewPort > 65535)
    throw new Error("Missing Storybook preview port from the test runner");
}
const baseURL = `http://127.0.0.1:${previewPort}`;

const viewports = [
  { name: "narrow", width: 320, height: 568 },
  { name: "mobile", width: 390, height: 844 },
  { name: "tablet", width: 768, height: 800 },
  { name: "desktop", width: 1280, height: 900 },
  { name: "short", width: 667, height: 320 },
  { name: "large-text", width: 320, height: 568, textScale: 2 },
];

export default defineConfig({
  testDir: "./test/storybook",
  workers: 2,
  fullyParallel: true,
  forbidOnly: Boolean(process.env.CI),
  retries: 0,
  timeout: 30_000,
  outputDir: "test-results/storybook",
  reporter: [
    ["list"],
    ["html", { outputFolder: "playwright-report/storybook", open: "never" }],
  ],
  use: {
    baseURL,
    browserName: "chromium",
    headless: true,
    reducedMotion: "reduce",
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
  },
  projects: [
    { name: "contracts", testMatch: "contracts.spec.ts" },
    ...viewports.flatMap(({ name, width, height, textScale }) =>
      (["light", "dark"] as const).map((theme) => ({
        name: `${name}-${theme}`,
        testMatch: ["overflow.spec.ts", "design-system.spec.ts"],
        metadata: { theme, textScale: textScale ?? 1 },
        use: { viewport: { width, height }, colorScheme: theme },
      })),
    ),
  ],
  webServer: {
    command: `node node_modules/vite/bin/vite.js preview --config storybook.serve.config.ts --port ${previewPort}`,
    url: `${baseURL}/index.json`,
    reuseExistingServer: false,
    timeout: 30_000,
  },
});
