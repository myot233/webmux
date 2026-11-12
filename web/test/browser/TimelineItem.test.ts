import "../../src/styles.css";
import { afterEach, expect, test } from "vitest";
import { page } from "vitest/browser";
import { cleanup, render } from "vitest-browser-svelte/pure";
import TimelineItem from "../../src/components/chat/TimelineItem.svelte";

afterEach(async () => {
  await cleanup();
});

test("a one-line user message bubble has no blank line below the text", async () => {
  render(TimelineItem, {
    entry: {
      type: "message",
      id: "m1",
      role: "user",
      text: "提交并推送",
      truncated: false,
    },
    waiting: [],
    onreview: () => {},
  });
  const text = page.getByRole("paragraph");
  await expect.element(text).toBeVisible();
  const p = text.element() as HTMLElement;
  const bubble = p.parentElement as HTMLElement;
  const style = getComputedStyle(bubble);
  const content =
    bubble.clientHeight -
    parseFloat(style.paddingTop) -
    parseFloat(style.paddingBottom);
  expect(content).toBeCloseTo(p.getBoundingClientRect().height, 0);
});

test("user session references are same-tab links while foreign URLs stay text", async () => {
  const foreign = "[Foreign](https://other.example.test/s/abcd2345)";
  render(TimelineItem, {
    entry: {
      type: "message",
      id: "reference",
      role: "user",
      text: `Compare [Session](/s/abcd2345) with ${foreign}`,
      truncated: false,
    },
    waiting: [],
    onreview: () => {},
  });
  const link = page.getByRole("link", { name: "Session", exact: true });
  await expect.element(link).toHaveAttribute("href", "/s/abcd2345");
  await expect.element(link).not.toHaveAttribute("target");
  await expect
    .element(page.getByRole("paragraph"))
    .toHaveTextContent(`你：Compare Session with ${foreign}`);
  expect(page.getByRole("link").elements()).toHaveLength(1);
});
