import { afterEach, expect, test, vi } from "vitest";
import { page, userEvent } from "vitest/browser";
import { cleanup, render } from "vitest-browser-svelte/pure";
import Harness from "./CodeDocumentHarness.svelte";

afterEach(async () => {
  await cleanup();
  await userEvent.cleanup();
  vi.unstubAllGlobals();
});

test("a real Worker computes diff; contexts expand locally and keep old-side references", async () => {
  const fetch = vi.fn(() => {
    throw new Error("context expansion must be local");
  });
  vi.stubGlobal("fetch", fetch);
  render(Harness);
  await expect
    .element(page.getByLabelText("只读代码"))
    .toMatchTextContent("changed revision 1");
  const source = document.querySelector<HTMLSelectElement>(
    'select[aria-label="引用来源"]',
  )!;
  source.value = "old";
  source.dispatchEvent(new Event("change", { bubbles: true }));
  const deleted = Array.from(
    document.querySelectorAll<HTMLElement>(".diff-gutter .cm-gutterElement"),
  ).find((e) => e.textContent === "50 ")!;
  await page.elementLocator(deleted).click();
  await expect
    .element(page.getByLabelText("位置预览"))
    .toHaveTextContent("/repo/code.txt:50（索引版本）");
  await page
    .getByRole("button", { name: "向下 30 行", exact: true })
    .first()
    .click();
  await expect
    .element(page.getByLabelText("只读代码"))
    .toMatchTextContent("ordinary line 1");
  await expect
    .element(page.getByLabelText("位置预览"))
    .toHaveTextContent("/repo/code.txt:50（索引版本）");
  await page.getByRole("button", { name: "展开全部", exact: true }).click();
  expect(fetch).not.toHaveBeenCalled();
  await page.getByRole("button", { name: "Replace snapshot" }).click();
  await expect
    .element(page.getByLabelText("只读代码"))
    .toMatchTextContent("changed revision 2");
  await expect
    .element(page.getByLabelText("位置预览"))
    .toHaveTextContent("/repo/code.txt（索引版本）");
});

test("large deletions stay virtualized and retain the actual old line numbers", async () => {
  render(Harness, { deleted: true });
  await expect
    .element(page.getByLabelText("只读代码"))
    .toMatchTextContent("ordinary line 1");
  expect(document.querySelectorAll(".cm-line").length).toBeLessThan(150);
  expect(document.querySelectorAll(".code-reader *").length).toBeLessThan(1000);
  await page.getByRole("button", { name: "跳转行" }).click();
  await page.getByRole("textbox", { name: /Go to line/ }).fill("19800");
  await userEvent.keyboard("{Enter}");
  await expect
    .element(page.getByLabelText("只读代码"))
    .toMatchTextContent("ordinary line 19800");
  const source = document.querySelector<HTMLSelectElement>(
    'select[aria-label="引用来源"]',
  )!;
  source.value = "old";
  source.dispatchEvent(new Event("change", { bubbles: true }));
  const target = Array.from(
    document.querySelectorAll<HTMLElement>(".diff-gutter .cm-gutterElement"),
  ).find((e) => e.textContent === "19800 ")!;
  await page.elementLocator(target).click();
  await expect
    .element(page.getByLabelText("位置预览"))
    .toHaveTextContent("/repo/code.txt:19800（索引版本）");
  expect(document.querySelectorAll(".cm-line").length).toBeLessThan(150);
});

test("replacing or hiding a document cancels its Worker and ignores late results", async () => {
  class WorkerStub {
    static instances: WorkerStub[] = [];
    data?: { snapshotId: string };
    onmessage?: (event: { data: unknown }) => void;
    terminate = vi.fn();
    constructor() {
      WorkerStub.instances.push(this);
    }
    postMessage(data: { snapshotId: string }) {
      this.data = data;
    }
    reply(text: string) {
      this.onmessage?.({
        data: {
          snapshotId: this.data!.snapshotId,
          result: {
            kind: "ready",
            lines: [{ kind: "add", oldLine: 0, newLine: 1, text }],
          },
        },
      });
    }
  }
  vi.stubGlobal("Worker", WorkerStub);
  render(Harness);
  await expect.poll(() => WorkerStub.instances.length).toBe(1);
  const obsolete = WorkerStub.instances[0];
  await page.getByRole("button", { name: "Replace snapshot" }).click();
  await expect.poll(() => WorkerStub.instances.length).toBe(2);
  expect(obsolete.terminate).toHaveBeenCalledTimes(1);
  WorkerStub.instances[1].reply("new snapshot result");
  await expect
    .element(page.getByLabelText("只读代码"))
    .toMatchTextContent("new snapshot result");
  obsolete.reply("obsolete snapshot result");
  await expect
    .element(page.getByLabelText("只读代码"))
    .not.toMatchTextContent("obsolete snapshot result");
  await page.getByRole("button", { name: "Replace snapshot" }).click();
  await expect.poll(() => WorkerStub.instances.length).toBe(3);
  await page.getByRole("button", { name: "Toggle active" }).click();
  expect(WorkerStub.instances[2].terminate).toHaveBeenCalledTimes(1);
  await page.getByRole("button", { name: "Toggle active" }).click();
  await expect.poll(() => WorkerStub.instances.length).toBe(4);
  WorkerStub.instances[3].reply("resumed snapshot result");
  await expect
    .element(page.getByLabelText("只读代码"))
    .toMatchTextContent("resumed snapshot result");
  await page.getByRole("button", { name: "Replace snapshot" }).click();
  await expect.poll(() => WorkerStub.instances.length).toBe(5);
  const limited = WorkerStub.instances[4];
  limited.onmessage?.({
    data: {
      snapshotId: limited.data!.snapshotId,
      result: { kind: "limited", reason: "计算受限" },
    },
  });
  await expect.element(page.getByRole("status")).toHaveTextContent("计算受限");
  await expect
    .element(page.getByRole("button", { name: "旧侧完整文件" }))
    .toBeEnabled();
  await page.getByRole("button", { name: "重试差异计算" }).click();
  await expect.poll(() => WorkerStub.instances.length).toBe(6);
  WorkerStub.instances[5].reply("retried result");
  await expect
    .element(page.getByLabelText("只读代码"))
    .toMatchTextContent("retried result");
});
