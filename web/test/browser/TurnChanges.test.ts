import "../../src/styles.css";
import { afterEach, beforeEach, expect, test } from "vitest";
import { page, userEvent } from "vitest/browser";
import { cleanup, render } from "vitest-browser-svelte/pure";
import TurnChanges from "../../src/components/chat/TurnChanges.svelte";
import TurnPatch from "../../src/components/chat/TurnPatch.svelte";
import { seedSettings } from "./settings-fixture";

beforeEach(async () => {
  await seedSettings();
});
afterEach(async () => {
  await cleanup();
  await userEvent.cleanup();
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

test("selecting deletion and addition rows switches the searchable source", async () => {
  render(TurnPatch, {
    patch: {
      type: "diff",
      path: "app.ts",
      oldText: "export const title = 'Old';\n",
      newText: "export const title = 'New';\n",
    },
  });
  const diff = page.getByRole("region", { name: "回合差异：app.ts" });
  await expect.element(diff).toBeVisible();
  await page.getByRole("button", { name: "文件内查找", exact: true }).click();
  const search = page.getByRole("textbox", { name: "查找", exact: true });
  for (const [type, query, label] of [
    ["change-deletion", "Old", "修改前"],
    ["change-addition", "New", "修改后"],
  ] as const) {
    const selector = `[data-line-type="${type}"][data-column-number="1"]`;
    await expect
      .poll(() =>
        document
          .querySelector("diffs-container")
          ?.shadowRoot?.querySelector(selector),
      )
      .toBeTruthy();
    await page
      .elementLocator(
        document
          .querySelector("diffs-container")!
          .shadowRoot!.querySelector(selector)!,
      )
      .click();
    await search.fill(query);
    await userEvent.keyboard("{Enter}");
    await expect
      .element(page.getByRole("status"))
      .toHaveTextContent(`${label} · 1 / 1`);
  }
});

test("a fully deleted file searches its old content by default", async () => {
  render(TurnPatch, {
    patch: {
      type: "diff",
      path: "removed.ts",
      oldText: "export const removed = 'Deleted';\n",
      newText: "",
    },
  });
  await page.getByRole("button", { name: "文件内查找", exact: true }).click();
  const search = page.getByRole("textbox", { name: "查找", exact: true });
  await search.fill("Deleted");
  await userEvent.keyboard("{Enter}");
  await expect
    .element(page.getByRole("status"))
    .toHaveTextContent("修改前 · 1 / 1");
});

test("patch-only and unchanged snapshot diffs render honestly without full-file tab copy", async () => {
  render(TurnChanges, {
    changes: [
      {
        type: "diff",
        path: "patch-only.ts",
        text: "@@ -1 +1 @@\n-oldLine\n+newLine",
      },
      {
        type: "diff",
        path: "same.ts",
        oldText: "same\n",
        newText: "same\n",
      },
    ],
  });
  await page.getByRole("button", { name: "本轮改动 · 2 个文件" }).click();
  const patchFile = page.getByRole("button", { name: /^patch-only\.ts$/ });
  await expect.element(patchFile).not.toHaveTextContent("+0");
  await patchFile.click();
  await expect
    .element(page.getByText(/@@ -1 \+1 @@\s+-oldLine\s+\+newLine/))
    .toBeVisible();
  const sameFile = page.getByRole("button", { name: /^same\.ts/ });
  await sameFile.click();
  const status = page.getByRole("status");
  await expect.element(status).toHaveTextContent("文件内容没有变化");
  await expect.element(status).not.toHaveTextContent("完整文件");
});
