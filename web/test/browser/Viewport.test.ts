import { afterEach, expect, test } from "vitest";
import { page } from "vitest/browser";
import { cleanup, render } from "vitest-browser-svelte/pure";
import Harness from "./ViewportHarness.svelte";

afterEach(async () => {
  await cleanup();
  window.scrollTo(0, 0);
});

const frames = () =>
  new Promise<void>((resolve) =>
    requestAnimationFrame(() => requestAnimationFrame(() => resolve())),
  );

const shownHeight = () =>
  Number(
    page.getByRole("status", { name: "Viewport height" }).element().textContent,
  );

// Scroll snapping waits for composition to end, but keyboard height changes
// apply immediately: Android keyboards compose every word and can hide
// without ending the composition.
test("composition defers scroll snapping but never the viewport height", async () => {
  await page.viewport(400, 700);
  await render(Harness);
  await expect.poll(shownHeight).toBe(700);
  expect(document.documentElement.style.overflow).toBe("hidden");

  const input = page.getByRole("textbox", { name: "Composition input" });
  const field = input.element() as HTMLInputElement;
  field.focus();
  field.dispatchEvent(
    new CompositionEvent("compositionstart", { bubbles: true, data: "" }),
  );
  window.scrollTo(0, 50);
  window.dispatchEvent(new Event("resize"));
  await frames();
  expect(window.scrollY).toBe(50);

  await page.viewport(400, 420);
  await expect.poll(shownHeight).toBe(420);
  expect(window.scrollY).toBe(50);

  field.dispatchEvent(
    new CompositionEvent("compositionend", { bubbles: true, data: "你" }),
  );
  await expect.poll(() => window.scrollY).toBe(0);

  // An abandoned composition is released by blur.
  field.dispatchEvent(
    new CompositionEvent("compositionstart", { bubbles: true, data: "" }),
  );
  window.scrollTo(0, 50);
  window.dispatchEvent(new Event("resize"));
  await frames();
  expect(window.scrollY).toBe(50);
  field.blur();
  await expect.poll(() => window.scrollY).toBe(0);
});

test("unmount restores the document overflow", async () => {
  document.documentElement.style.overflow = "clip";
  const rendered = await render(Harness);
  expect(document.documentElement.style.overflow).toBe("hidden");
  await rendered.unmount();
  expect(document.documentElement.style.overflow).toBe("clip");
  document.documentElement.style.overflow = "";
});
