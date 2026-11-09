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
  const send = vi.fn();
  const toggle = vi.fn();
  const lock = vi.fn();
  const { container } = await render(SoftKeys, {
    send,
    modifiers: { ctrl: false, alt: false, shift: false },
    toggle,
    lock,
    paste: () => {},
    keyboard: () => {},
  });
  const buttons = [...container.querySelectorAll("button")];
  const shift = buttons.find((button) => button.textContent === "Shift")!;
  const tab = buttons.find((button) => button.textContent === "Tab")!;
  const pointer = (type: string, pointerType = "touch") =>
    shift.dispatchEvent(
      new PointerEvent(type, {
        bubbles: true,
        cancelable: true,
        isPrimary: true,
        pointerType,
      }),
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

  // A mouse hold also locks Shift. Only its release click is ignored.
  toggle.mockClear();
  pointer("pointerdown", "mouse");
  await new Promise((resolve) => setTimeout(resolve, 700));
  expect(lock).toHaveBeenCalledWith("shift");
  pointer("pointerup", "mouse");
  shift.click();
  expect(toggle).not.toHaveBeenCalled();
  tab.click();
  expect(send).toHaveBeenCalledWith("\t");

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

test("soft keys include Enter and deduplicate gamepad keys when gamepad mode is active", async () => {
  const send = vi.fn();
  const view = await render(SoftKeys, {
    send,
    modifiers: { ctrl: false, alt: false, shift: false },
    gamepad: false,
    toggle: () => {},
    paste: () => {},
    keyboard: () => {},
  });

  const labels = () =>
    [...view.container.querySelectorAll(".soft-row button")]
      .map((button) => button.textContent?.trim())
      .filter(Boolean);

  expect(labels()).toContain("↵");
  expect(labels()).toContain("Esc");
  expect(labels()).toContain("Tab");
  expect(labels()).toContain("Ctrl");
  expect(labels()).toContain("↑");
  expect(labels()).toContain("Ctrl+C");
  expect(view.container.querySelector('[aria-label="终端摇杆"]')).toBeNull();

  const enterButton = [
    ...view.container.querySelectorAll(".soft-row button"),
  ].find((button) => button.textContent === "↵")!;
  enterButton.click();
  expect(send).toHaveBeenCalledWith("\r");

  await view.rerender({
    send,
    modifiers: { ctrl: false, alt: false, shift: false },
    gamepad: true,
    toggle: () => {},
    paste: () => {},
    keyboard: () => {},
  });

  expect(
    view.container.querySelector('.soft-keys > [aria-label="终端摇杆"]'),
  ).not.toBeNull();

  for (const removed of ["Esc", "Tab", "↵", "↑", "↓", "←", "→", "Ctrl+C"]) {
    expect(labels()).not.toContain(removed);
  }
  for (const kept of [
    "/",
    "Ctrl",
    "Alt",
    "Shift",
    "Home",
    "End",
    "PgUp",
    "PgDn",
    "Ctrl+D",
    "Ctrl+Z",
    "Ctrl+L",
    "Ctrl+R",
  ]) {
    expect(labels()).toContain(kept);
  }
});
