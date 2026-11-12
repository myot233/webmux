import "../../src/styles.css";
import { afterEach, beforeEach, expect, test } from "vitest";
import { page } from "vitest/browser";
import { cleanup, render } from "vitest-browser-svelte/pure";
import TurnChanges from "../../src/components/chat/TurnChanges.svelte";
import TurnPatch from "../../src/components/chat/TurnPatch.svelte";
import { seedSettings } from "./settings-fixture";

beforeEach(async () => {
  await seedSettings();
});
afterEach(async () => {
  await cleanup();
});

test("one file exposes every independent edit and toggles closed again", async () => {
  render(TurnChanges, {
    changes: [
      {
        type: "diff",
        path: "app.ts",
        oldText: "export const title = 'Old';\n",
        newText: "export const title = 'New';\n",
      },
      {
        type: "diff",
        path: "app.ts",
        oldText: "export const footer = 'Before';\n",
        newText: "export const footer = 'After';\n",
      },
    ],
  });
  await page.getByRole("button", { name: "本轮改动 · 1 个文件" }).click();
  const file = page.getByRole("button", { name: /^app\.ts/ });
  await expect.element(file).toMatchTextContent("新增 2 行，删除 2 行");
  await file.click();
  await expect.element(file).toHaveAttribute("aria-pressed", "true");
  const diffs = page.getByRole("region", { name: "回合差异：app.ts" });
  await expect.element(diffs.nth(0)).toBeVisible();
  await expect.element(diffs.nth(1)).toBeVisible();
  const renderedText = (index: number) =>
    diffs.nth(index).element().querySelector("diffs-container")?.shadowRoot
      ?.textContent;
  await expect
    .poll(() => renderedText(0))
    .toContain("export const title = 'Old';");
  await expect
    .poll(() => renderedText(0))
    .toContain("export const title = 'New';");
  await expect
    .poll(() => renderedText(1))
    .toContain("export const footer = 'Before';");
  await expect
    .poll(() => renderedText(1))
    .toContain("export const footer = 'After';");
  await file.click();
  await expect.element(file).toHaveAttribute("aria-pressed", "false");
  await expect.element(diffs).not.toBeInTheDocument();
});

test("report truncation and bounded computation have separate explanations", async () => {
  render(TurnPatch, {
    patch: {
      type: "diff",
      path: "large.ts",
      oldText: "before\n".repeat(20_001),
      newText: "after\n",
      truncated: true,
    },
  });
  await expect
    .element(page.getByText("Agent 报告的改动内容已截断。", { exact: true }))
    .toBeVisible();
  await expect
    .element(page.getByRole("status"))
    .toHaveTextContent("内容超过 2 MiB 或 20000 行");
  await expect
    .element(page.getByRole("region", { name: "回合差异：large.ts" }))
    .not.toBeInTheDocument();
});
