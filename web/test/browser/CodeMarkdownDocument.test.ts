import { seedSettings } from "./settings-fixture";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { page, userEvent } from "vitest/browser";
import { cleanup, render } from "vitest-browser-svelte/pure";
import Harness from "./CodeMarkdownDocumentHarness.svelte";

beforeEach(async () => {
  await seedSettings();
});

afterEach(async () => {
  await cleanup();
  await userEvent.cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

const filler = Array.from({ length: 30 }, (_, i) => `段落 ${i + 1}`).join(
  "\n\n",
);
const outlined = [
  "# 指南",
  filler,
  "## 第一节",
  filler,
  "### 细节",
  filler,
  "## 第三节",
  filler,
].join("\n\n");
const preview = () =>
  document.querySelector<HTMLElement>('[aria-label="Markdown 预览"]')!;
const offset = (name: string) =>
  page.getByRole("heading", { name }).element().getBoundingClientRect().top -
  preview().getBoundingClientRect().top;

test("the outline lists headings by level and scrolls the preview", async () => {
  render(Harness, { text: outlined });
  const outline = page.getByRole("button", { name: "目录" });
  await expect
    .element(page.getByRole("button", { name: "预览", exact: true }))
    .toBeVisible();
  await expect.element(outline).not.toBeInTheDocument();
  await page.getByRole("button", { name: "预览", exact: true }).click();
  await outline.click();
  const items = page.getByRole("menuitem");
  await expect.element(items.first()).toBeVisible();
  expect(items.elements().map((item) => item.textContent?.trim())).toEqual([
    "指南",
    "第一节",
    "细节",
    "第三节",
  ]);
  const before = location.href;
  await page.getByRole("menuitem", { name: "第三节" }).click();
  await expect.element(page.getByRole("menu")).not.toBeInTheDocument();
  await expect.poll(() => Math.abs(offset("第三节"))).toBeLessThan(2);
  expect(location.href).toBe(before);
});

test("documents with fewer than two headings have no outline", async () => {
  render(Harness, { text: `# 唯一\n\n${filler}` });
  await page.getByRole("button", { name: "预览", exact: true }).click();
  await expect
    .element(page.getByRole("heading", { name: "唯一" }))
    .toBeVisible();
  await expect
    .element(page.getByRole("button", { name: "目录" }))
    .not.toBeInTheDocument();
  await page.getByRole("button", { name: "代码菜单" }).click();
  await expect
    .element(page.getByRole("menuitem", { name: "返回源码" }))
    .toBeVisible();
  await expect
    .element(page.getByRole("menuitem", { name: "目录" }))
    .not.toBeInTheDocument();
});

test("the narrow code menu offers the outline", async () => {
  render(Harness, { text: outlined });
  await page.getByRole("button", { name: "代码菜单" }).click();
  await page.getByRole("menuitem", { name: "预览 Markdown" }).click();
  await expect
    .element(page.getByRole("heading", { name: "指南" }))
    .toBeVisible();
  await page.getByRole("button", { name: "代码菜单" }).click();
  await page.getByRole("menuitem", { name: "目录" }).click();
  await page.getByRole("menuitem", { name: "第三节" }).click();
  await expect.element(page.getByRole("menu")).not.toBeInTheDocument();
  await expect.poll(() => Math.abs(offset("第三节"))).toBeLessThan(2);
});

test("a heading request from another file's link previews and scrolls once", async () => {
  render(Harness, { text: outlined });
  await expect.element(page.getByLabelText("只读代码")).toBeVisible();
  await page.getByRole("button", { name: "Request heading" }).click();
  await expect
    .element(page.getByLabelText("Pending heading"))
    .toHaveTextContent("none");
  await expect.element(page.getByLabelText("Markdown 预览")).toBeVisible();
  await expect.poll(() => Math.abs(offset("第三节"))).toBeLessThan(2);
});
