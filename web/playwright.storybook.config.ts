import { defineConfig } from "@playwright/test";

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
    baseURL: "http://127.0.0.1:6007",
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
        testMatch: "overflow.spec.ts",
        metadata: { theme, textScale: textScale ?? 1 },
        use: { viewport: { width, height }, colorScheme: theme },
      })),
    ),
  ],
  webServer: {
    command:
      "node node_modules/vite/bin/vite.js preview --config storybook.serve.config.ts",
    url: "http://127.0.0.1:6007/index.json",
    reuseExistingServer: false,
    timeout: 30_000,
  },
});
