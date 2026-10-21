import { defineConfig } from "@playwright/test";
import base from "./playwright.config";

// Measure the embedded production app on a dedicated worker, without other
// tests competing for the same CPU or masking cold startup with Vite HMR.
export default defineConfig({
  ...base,
  testDir: "./test/performance",
  globalSetup: "./test/performance/global-setup.mjs",
  fullyParallel: false,
  workers: 1,
  retries: 0,
  outputDir: "test-results/performance",
  reporter: [
    [process.env.CI ? "github" : "list"],
    ["html", { outputFolder: "playwright-report/performance", open: "never" }],
  ],
});
