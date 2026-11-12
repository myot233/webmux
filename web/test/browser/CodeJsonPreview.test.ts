import { afterEach, expect, test } from "vitest";
import { page, userEvent } from "vitest/browser";
import { cleanup, render } from "vitest-browser-svelte/pure";
import CodeJsonPreview from "../../src/components/code/CodeJsonPreview.svelte";

afterEach(async () => {
  await cleanup();
  await userEvent.cleanup();
});

test("captured expansion and paging restore in a new preview", async () => {
  const text = JSON.stringify({
    items: Array.from({ length: 450 }, (_, index) => index),
    // `deep` is the first level that starts closed.
    nested: { deep: { value: "hidden" } },
  });
  const first = await render(CodeJsonPreview, { text, onsource: () => {} });
  await page.getByRole("button", { name: "再显示 200 项" }).click();
  await expect.element(page.getByText("399:", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: /^deep/ }).click();
  await expect.element(page.getByText('"hidden"')).toBeVisible();
  const captured = first.component.capture();
  await first.unmount();

  await render(CodeJsonPreview, {
    text,
    onsource: () => {},
    restore: captured,
  });
  await expect.element(page.getByText("399:", { exact: true })).toBeVisible();
  await expect
    .element(page.getByText("400:", { exact: true }))
    .not.toBeInTheDocument();
  await expect.element(page.getByText('"hidden"')).toBeVisible();
});
