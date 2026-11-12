import { seedSettings } from "./settings-fixture";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { page, userEvent } from "vitest/browser";
import { cleanup, render } from "vitest-browser-svelte/pure";
import type { FileDiffMetadata } from "@pierre/diffs";
import Harness from "./CodeDiffReaderHarness.svelte";
import { computeDiff } from "../../src/domains/code/code-diff";

beforeEach(async () => {
  await seedSettings();
});
afterEach(async () => {
  await cleanup();
  await userEvent.cleanup();
  vi.unstubAllGlobals();
});

const shadow = () => document.querySelector("diffs-container")?.shadowRoot;
// Rows of the new side: unified rows carry new line numbers except deletions.
const row = (line: number) =>
  shadow()?.querySelector<HTMLElement>(
    `[data-line="${line}"]:not([data-line-type="change-deletion"])`,
  );

test("captured diff state restores selection, expansion, search and position", async () => {
  const lines = Array.from({ length: 400 }, (_, i) => `ordinary line ${i + 1}`);
  const changed = lines.map((line, i) =>
    i === 49 || i === 299 ? `changed line ${i + 1}` : line,
  );
  const result = computeDiff(
    lines.join("\n") + "\n",
    changed.join("\n") + "\n",
  );
  if (result.kind !== "ready") throw new Error(result.reason);
  const fileDiff = { ...result.fileDiff, name: "code.txt" } as FileDiffMetadata;
  const first = await render(Harness, { fileDiff });
  await expect
    .poll(() => row(50)?.textContent, { timeout: 10000 })
    .toContain("changed line 50");

  await page.getByRole("button", { name: "文件内查找" }).click();
  await page.getByRole("textbox", { name: "查找" }).fill("changed");
  await page.getByRole("button", { name: "下一个匹配", exact: true }).click();

  // Reveal unmodified lines between the two hunks.
  const separator = shadow()!.querySelector('[data-expand-index="1"]')!;
  await page
    .elementLocator(separator)
    .getByRole("button", { name: "向下展开 30 行" })
    .click();
  const [region] = first.component.capture().expanded;
  expect(region).toBeDefined();
  expect(region!.index).toBe(1);
  expect(region!.fromStart + region!.fromEnd).toBe(30);
  // The first revealed line, in either direction.
  const revealed = region!.fromStart > 0 ? 54 : 297 - 30;
  await expect
    .poll(() => row(revealed)?.textContent)
    .toContain(`ordinary line ${revealed}`);

  await page
    .elementLocator(
      shadow()!.querySelector(
        '[data-line-type="change-addition"][data-column-number="50"]',
      )!,
    )
    .click();
  await expect
    .element(page.getByLabelText("Selected lines"))
    .toHaveTextContent("50-50");

  // Bring the revealed line to the top edge.
  const host = document.querySelector<HTMLElement>(".code-diff-reader")!;
  host.scrollTop +=
    row(revealed)!.getBoundingClientRect().top -
    host.getBoundingClientRect().top;
  await expect
    .poll(() => first.component.capture().top)
    .toEqual({ line: revealed, side: "additions" });
  const captured = first.component.capture();
  expect(captured).toMatchObject({
    selected: { start: 50, end: 50, side: "additions" },
    search: { search: "changed" },
    panel: "search",
    cursor: { matches: [50, 300], index: 0 },
  });
  await first.unmount();

  // A hidden tab waits for layout; capture returns the pending state.
  const second = await render(Harness, {
    fileDiff,
    restore: captured,
    hidden: true,
  });
  expect(second.component.capture()).toEqual(captured);
  await second.rerender({ hidden: false });
  const restoredHost =
    document.querySelector<HTMLElement>(".code-diff-reader")!;
  // The revealed line is rendered at the top edge again.
  await expect
    .poll(
      () =>
        (row(revealed)?.getBoundingClientRect().top ?? -1e6) -
        restoredHost.getBoundingClientRect().top,
      { timeout: 10000 },
    )
    .toBeCloseTo(0, 0);
  expect(row(revealed)?.textContent).toContain(`ordinary line ${revealed}`);
  expect(second.component.capture()).toEqual(captured);
  await expect
    .element(page.getByLabelText("Selected lines"))
    .toHaveTextContent("50-50");
  await expect
    .element(page.getByRole("textbox", { name: "查找" }))
    .toHaveValue("changed");
  await expect
    .poll(() =>
      [...document.querySelectorAll('[role="status"]')].map((element) =>
        element.textContent?.replace(/\s+/g, " ").trim(),
      ),
    )
    .toContain("修改后 · 1 / 2");
});
