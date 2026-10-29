import { afterEach, expect, test, vi } from "vitest";
import { cleanup, render } from "vitest-browser-svelte/pure";
import SoftKeys from "../../src/components/SoftKeys.svelte";

afterEach(() => cleanup());

test("soft keys keep the focused input by canceling mousedown, not pointerdown", async () => {
  const send = vi.fn();
  const { container } = await render(SoftKeys, {
    send,
    modifiers: { ctrl: false, alt: false, shift: false },
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
    modifiers: { ctrl: false, alt: false, shift: false },
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

test("Shift taps toggle once while a long press locks it", async () => {
  const toggle = vi.fn();
  const lock = vi.fn();
  const { container } = await render(SoftKeys, {
    send: () => {},
    modifiers: { ctrl: false, alt: false, shift: false },
    toggle,
    lock,
    paste: () => {},
    keyboard: () => {},
  });
  const shift = [...container.querySelectorAll("button")].find(
    (button) => button.textContent === "Shift",
  )!;
  const pointer = (type: string) =>
    shift.dispatchEvent(
      new PointerEvent(type, { bubbles: true, cancelable: true }),
    );

  // A short tap keeps the one-shot toggle behavior of the other modifiers.
  pointer("pointerdown");
  pointer("pointerup");
  shift.click();
  expect(toggle).toHaveBeenCalledWith("shift");
  expect(lock).not.toHaveBeenCalled();
  // Releasing must cancel the pending timer, not just hide it.
  await new Promise((resolve) => setTimeout(resolve, 700));
  expect(lock).not.toHaveBeenCalled();

  // Holding past the threshold locks Shift, and the release click is ignored.
  toggle.mockClear();
  pointer("pointerdown");
  await new Promise((resolve) => setTimeout(resolve, 700));
  expect(lock).toHaveBeenCalledWith("shift");
  pointer("pointerup");
  shift.click();
  expect(toggle).not.toHaveBeenCalled();

  // Moving the pointer turns the press into a scroll, not a lock.
  lock.mockClear();
  pointer("pointerdown");
  shift.dispatchEvent(
    new PointerEvent("pointermove", {
      bubbles: true,
      clientX: shift.getBoundingClientRect().left + 20,
      clientY: shift.getBoundingClientRect().top,
    }),
  );
  await new Promise((resolve) => setTimeout(resolve, 700));
  expect(lock).not.toHaveBeenCalled();
  pointer("pointerup");
  shift.click();
  expect(toggle).toHaveBeenCalledWith("shift");
});

test("a locked Shift shows its state and unlock hint", async () => {
  const { container } = await render(SoftKeys, {
    send: () => {},
    modifiers: { ctrl: false, alt: false, shift: true },
    locked: true,
    toggle: () => {},
    paste: () => {},
    keyboard: () => {},
  });
  const shift = [...container.querySelectorAll("button")].find(
    (button) => button.textContent === "Shift",
  );

  expect(shift).toBeTruthy();
  expect(shift!.querySelector("svg")).toBeTruthy();
  expect(shift!.getAttribute("title")).toBe("已锁定，点按解除");
  expect(shift!.getAttribute("aria-pressed")).toBe("true");
});
