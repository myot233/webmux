import { afterEach, expect, test, vi } from "vitest";
import { cleanup, render } from "vitest-browser-svelte/pure";
import SoftKeys from "../../src/components/SoftKeys.svelte";

afterEach(() => cleanup());

test("soft keys keep the focused input by canceling mousedown, not pointerdown", async () => {
  const send = vi.fn();
  const { container } = await render(SoftKeys, {
    send,
    modifiers: { ctrl: false, alt: false },
    toggle: () => {},
    paste: () => {},
    keyboard: () => {},
  });
  const arrow = [...container.querySelectorAll("button")].find(
    (button) => button.textContent === "↑",
  )!;

  // iOS Safari blurs the active input on pointerdown even when canceled, so the
  // event must reach the button's compatibility mousedown.
  const pointer = new PointerEvent("pointerdown", {
    bubbles: true,
    cancelable: true,
  });
  arrow.dispatchEvent(pointer);
  expect(pointer.defaultPrevented).toBe(false);

  const mouse = new MouseEvent("mousedown", {
    bubbles: true,
    cancelable: true,
  });
  arrow.dispatchEvent(mouse);
  expect(mouse.defaultPrevented).toBe(true);

  // Canceling mousedown must not swallow the key action.
  arrow.click();
  expect(send).toHaveBeenCalledWith("\x1b[A");
});

test("symbol keys send their literal characters", async () => {
  const send = vi.fn();
  const { container } = await render(SoftKeys, {
    send,
    modifiers: { ctrl: false, alt: false },
    toggle: () => {},
    paste: () => {},
    keyboard: () => {},
  });
  const buttons = [...container.querySelectorAll("button")];

  for (const symbol of ["/", "@", "#", "!"]) {
    const button = buttons.find((node) => node.textContent === symbol);
    expect(button).toBeTruthy();
    send.mockClear();
    button!.click();
    expect(send).toHaveBeenCalledWith(symbol);
  }
});
