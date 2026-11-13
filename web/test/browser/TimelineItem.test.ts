import "../../src/styles.css";
import { afterEach, expect, test, vi } from "vitest";
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

test("agent markdown session references stay in-app links instead of code locations", async () => {
  const onopenlocation = vi.fn();
  render(TimelineItem, {
    entry: {
      type: "message",
      id: "agent-reference",
      role: "agent",
      text: `See [Build](/s/abcd2345), [Same](${location.origin}/s/bcde2345), [Foreign](https://other.example.test/s/abcd2345) and [Code](src/App.svelte:12).`,
      truncated: false,
    },
    waiting: [],
    onreview: () => {},
    onopenlocation,
  });
  const build = page.getByRole("link", { name: "Build", exact: true });
  const same = page.getByRole("link", { name: "Same", exact: true });
  const foreign = page.getByRole("link", { name: "Foreign", exact: true });
  const code = page.getByRole("link", { name: "Code", exact: true });
  await expect.element(build).toHaveAttribute("href", "/s/abcd2345");
  await expect.element(build).not.toHaveAttribute("target");
  await expect.element(build).not.toHaveAttribute("data-md-href");
  await expect.element(same).toHaveAttribute("href", "/s/bcde2345");
  await expect.element(same).not.toHaveAttribute("target");
  await expect
    .element(foreign)
    .toHaveAttribute("href", "https://other.example.test/s/abcd2345");
  await expect.element(foreign).toHaveAttribute("target", "_blank");
  await expect
    .element(code)
    .toHaveAttribute("data-md-href", "src/App.svelte:12");
  await code.click();
  expect(onopenlocation).toHaveBeenCalledTimes(1);
});
