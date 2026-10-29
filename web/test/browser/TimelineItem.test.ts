import "../../src/styles.css";
import { expect, test } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte/pure";
import TimelineItem from "../../src/components/chat/TimelineItem.svelte";

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
