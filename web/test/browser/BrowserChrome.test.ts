// Tab title and favicon ownership across running, waiting and reset states.
import { afterEach, expect, test } from "vitest";
import { resetBrowserChrome, setBrowserChrome } from "../../src/browser-chrome";

const icon = () => document.querySelector<HTMLLinkElement>('link[rel="icon"]')!;
const iconHref = () => icon().getAttribute("href");

afterEach(() => {
  resetBrowserChrome();
  document.title = "WebMux";
});

test("a running page marks the title and cycles local favicon frames", async () => {
  setBrowserChrome("WebMux · 工作区", true);
  expect(document.title).toBe("▶ WebMux · 工作区");
  await expect.poll(iconHref).toMatch(/^data:image\/png;base64,/);
  expect(icon().type).toBe("image/png");
  const first = iconHref();
  await expect.poll(iconHref, { timeout: 3000 }).not.toBe(first);
  expect(iconHref()).toMatch(/^data:image\/png;base64,/);
});

test("status refreshes do not postpone the next frame", async () => {
  setBrowserChrome("WebMux · 工作区", true);
  await expect.poll(iconHref).toMatch(/^data:image\/png/);
  const seen = new Set([iconHref()]);
  // The server republishes state about every 500 ms while output arrives.
  for (let index = 0; index < 5; index++) {
    await new Promise((resolve) => setTimeout(resolve, 450));
    setBrowserChrome("WebMux · 工作区", true);
    seen.add(iconHref());
  }
  expect(seen.size).toBeGreaterThan(1);
});

test("leaving the running state restores the static icon and title", async () => {
  setBrowserChrome("fixture · WebMux", true);
  await expect.poll(iconHref).toMatch(/^data:image\/png/);
  setBrowserChrome("fixture · WebMux", false);
  expect(document.title).toBe("fixture · WebMux");
  expect(iconHref()).toBe("/icon.svg");
  expect(icon().type).toBe("image/svg+xml");
});

test("reset stops animation and clears only the marker it added", async () => {
  setBrowserChrome("fixture · WebMux", true);
  await expect.poll(iconHref).toMatch(/^data:image\/png/);
  resetBrowserChrome();
  expect(document.title).toBe("fixture · WebMux");
  expect(iconHref()).toBe("/icon.svg");
  expect(icon().type).toBe("image/svg+xml");

  document.title = "WebMux · 设置";
  setBrowserChrome("WebMux · 设置", true);
  document.title = "WebMux · Tailscale";
  resetBrowserChrome();
  expect(document.title).toBe("WebMux · Tailscale");
  expect(iconHref()).toBe("/icon.svg");
  await new Promise((resolve) => setTimeout(resolve, 1200));
  expect(iconHref()).toBe("/icon.svg");
});
