import { expect, test, vi } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte/pure";
import QuickStartAgentToolbar from "../../src/components/sessions/QuickStartAgentToolbar.svelte";
import { launchConfig } from "./shell-fixture";

test("mouse wheels reach the last option and return without trapping zoom or edge scrolling", async () => {
  await page.viewport(768, 800);
  const agent = launchConfig();
  const change = vi.fn();
  render(QuickStartAgentToolbar, {
    agents: [agent],
    agent,
    startSide: "chat",
    chat: true,
    permissionMode: "default",
    locked: false,
    probing: false,
    sending: false,
    options: Array.from({ length: 12 }, (_, index) => ({
      id: `option-${index}`,
      name: `配置 ${index}`,
      type: "select" as const,
      value: "default",
      options: [{ value: "default", name: `选项 ${index}` }],
    })),
    onagentchange: change,
    onsidechange: change,
    onmodechange: change,
    onoptionchange: change,
  });
  const toolbar = page.getByRole("toolbar", { name: "启动选项" });
  const node = toolbar.element() as HTMLElement;
  expect(node.scrollWidth).toBeGreaterThan(node.clientWidth);
  await toolbar.wheel({ delta: { y: 500 } });
  await expect.poll(() => node.scrollLeft).toBeGreaterThan(0);
  await toolbar.wheel({ delta: { y: 10000 } });
  await expect
    .poll(() => node.scrollLeft)
    .toBe(node.scrollWidth - node.clientWidth);
  const edge = new WheelEvent("wheel", { deltaY: 100, cancelable: true });
  node.dispatchEvent(edge);
  expect(edge.defaultPrevented).toBe(false);
  const zoom = new WheelEvent("wheel", {
    deltaY: -100,
    ctrlKey: true,
    cancelable: true,
  });
  node.dispatchEvent(zoom);
  expect(zoom.defaultPrevented).toBe(false);
  expect(node.scrollLeft).toBe(node.scrollWidth - node.clientWidth);
  await toolbar.wheel({ delta: { y: -10000 } });
  await expect.poll(() => node.scrollLeft).toBe(0);
  await toolbar.wheel({ delta: { x: 150 } });
  await expect.poll(() => node.scrollLeft).toBeGreaterThan(0);
});
