import { afterEach, expect, test, vi } from "vitest";
import { page } from "vitest/browser";
import { cleanup, render } from "vitest-browser-svelte/pure";
import Harness from "./TerminalViewportHarness.svelte";

afterEach(cleanup);

test("touch gamepad renders only when enabled and controlling, supports slide-to-steer and repeat, and deduplicates bottom soft keys", async () => {
  const send = vi.fn();
  const toggle = vi.fn();
  const view = await render(Harness, {
    connection: "connected",
    restoring: false,
    controlling: false,
    touchGamepad: true,
    send,
    toggle,
  });

  // Not controlling: gamepad is hidden and bottom row keeps full keys.
  expect(document.querySelector('[aria-label="终端摇杆"]')).toBeNull();
  await expect
    .element(page.getByRole("button", { name: "展开摇杆" }))
    .toBeDisabled();
  expect(
    [...document.querySelectorAll(".soft-row button")].some(
      (b) => b.textContent === "Esc",
    ),
  ).toBe(true);

  // Claiming control adds the gamepad to the soft-key bar and trims duplicate keys.
  await view.rerender({
    connection: "connected",
    restoring: false,
    controlling: true,
    touchGamepad: true,
    send,
    toggle,
  });
  const gamepad = document.querySelector<HTMLElement>(
    '[aria-label="终端摇杆"]',
  );
  expect(gamepad).not.toBeNull();
  expect(gamepad!.closest(".soft-keys")).not.toBeNull();
  expect(getComputedStyle(gamepad!).position).toBe("static");
  const terminalArea = document.querySelector(".terminal-area")!;
  expect(gamepad!.getBoundingClientRect().top).toBeGreaterThanOrEqual(
    terminalArea.getBoundingClientRect().bottom,
  );
  for (const group of gamepad!.children) {
    expect(getComputedStyle(group).borderTopWidth).toBe("0px");
    expect(getComputedStyle(group).boxShadow).toBe("none");
  }
  expect(
    [...document.querySelectorAll(".soft-row button")].some(
      (b) => b.textContent === "Esc",
    ),
  ).toBe(false);

  const key = (name: string) =>
    gamepad!.querySelector<HTMLButtonElement>(`[data-gamepad-key="${name}"]`)!;

  key("esc").click();
  expect(send).toHaveBeenCalledWith("\x1b");
  key("tab").click();
  expect(send).toHaveBeenCalledWith("\t");
  key("ctrl-c").click();
  expect(send).toHaveBeenCalledWith("\x03");
  key("enter").click();
  expect(send).toHaveBeenCalledWith("\r");
  key("space").click();
  expect(send).toHaveBeenCalledWith(" ");

  // Slide-to-steer on the D-Pad: press Up, then drag right past the threshold.
  send.mockClear();
  const up = key("up");
  up.dispatchEvent(
    new PointerEvent("pointerdown", {
      bubbles: true,
      cancelable: true,
      pointerId: 1,
      button: 0,
      clientX: 100,
      clientY: 100,
    }),
  );
  expect(send).toHaveBeenLastCalledWith("\x1b[A");
  up.dispatchEvent(
    new PointerEvent("pointermove", {
      bubbles: true,
      cancelable: true,
      pointerId: 1,
      clientX: 130,
      clientY: 100,
    }),
  );
  expect(send).toHaveBeenLastCalledWith("\x1b[C");
  up.dispatchEvent(
    new PointerEvent("pointerup", {
      bubbles: true,
      cancelable: true,
      pointerId: 1,
    }),
  );

  // Hold auto-repeat on the D-Pad.
  send.mockClear();
  up.dispatchEvent(
    new PointerEvent("pointerdown", {
      bubbles: true,
      cancelable: true,
      pointerId: 2,
      button: 0,
      clientX: 100,
      clientY: 100,
    }),
  );
  expect(send).toHaveBeenCalledTimes(1);
  expect(send).toHaveBeenLastCalledWith("\x1b[A");
  await new Promise((resolve) => setTimeout(resolve, 480));
  expect(send.mock.calls.length).toBeGreaterThanOrEqual(2);
  up.dispatchEvent(
    new PointerEvent("pointerup", {
      bubbles: true,
      cancelable: true,
      pointerId: 2,
    }),
  );
});

test("soft-key action expands and collapses the gamepad", async () => {
  const setTouchGamepad = vi.fn();
  const view = await render(Harness, {
    connection: "connected",
    controlling: true,
    touchGamepad: false,
    setTouchGamepad,
  });

  const expand = page.getByRole("button", { name: "展开摇杆" });
  expect(expand.element().closest(".soft-row")).not.toBeNull();
  expect(expand.element().closest(".soft-trailing")).toBeNull();
  await expect.element(expand).toHaveAttribute("aria-expanded", "false");
  await expand.click();
  expect(setTouchGamepad).toHaveBeenCalledWith(true);

  await view.rerender({
    connection: "connected",
    controlling: true,
    touchGamepad: true,
    setTouchGamepad,
  });
  const collapse = page.getByRole("button", { name: "收起摇杆" });
  await expect.element(collapse).toHaveAttribute("aria-expanded", "true");
  await collapse.click();
  expect(setTouchGamepad).toHaveBeenCalledWith(false);
});

test.each(["connecting", "reconnecting"] as const)(
  "a synced %s terminal shows a non-blocking restoration hint",
  async (connection) => {
    const view = await render(Harness, { connection, restoring: false });
    const hint = page.getByText("正在恢复…");
    await expect.element(hint).toBeVisible();
    // The hint floats over retained output without covering or intercepting it.
    expect(hint.element().closest(".pointer-events-none")).not.toBeNull();
    expect(document.querySelector(".restore-overlay")).toBeNull();

    await view.rerender({ connection: "connected", restoring: false });
    await expect.element(hint).not.toBeInTheDocument();
  },
);

test("restoration keeps its blocking overlay labels", async () => {
  const view = await render(Harness, {
    connection: "connecting",
    restoring: true,
  });
  await expect.element(page.getByText("正在连接…")).toBeVisible();
  expect(document.querySelector(".restore-overlay")).not.toBeNull();

  await view.rerender({ connection: "connected", restoring: true });
  await expect.element(page.getByText("正在恢复…")).toBeVisible();

  await view.rerender({ connection: "connected", restoring: false });
  expect(document.querySelector(".restore-overlay")).toBeNull();
});
