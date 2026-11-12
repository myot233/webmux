import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { createKeyboardSizeHold } from "../src/hooks/terminal/keyboard-size-hold";

function setup(canHold = true, keyboardStays?: () => boolean) {
  const events: string[] = [];
  const hold = createKeyboardSizeHold({
    canHold: () => canHold,
    keyboardStays,
    onActive: (active) => events.push(active ? "hold" : "unhold"),
    release: () => events.push("release"),
  });
  const target = new EventTarget();
  const blur = () => target.dispatchEvent(new Event("focusout"));
  return { hold, events, target, blur };
}

describe("keyboard size hold", () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  test("button close releases once, after the keyboard is gone and settled", () => {
    const { hold, events, target, blur } = setup();
    hold.begin(target);
    hold.update("visible");
    expect(events).toEqual(["hold"]);
    // The button blurs first; the keyboard is still animating closed.
    blur();
    vi.advanceTimersByTime(500);
    expect(events).toEqual(["hold"]);
    hold.update("hidden");
    vi.advanceTimersByTime(150);
    hold.update("hidden"); // the height still moves
    vi.advanceTimersByTime(150);
    expect(events).toEqual(["hold"]);
    vi.advanceTimersByTime(50);
    expect(events).toEqual(["hold", "unhold", "release"]);
    blur();
    vi.advanceTimersByTime(5000);
    expect(events).toEqual(["hold", "unhold", "release"]);
  });

  test("a system dismissal that keeps focus ends the hold", () => {
    const { hold, events, target } = setup();
    hold.begin(target);
    hold.update("visible");
    hold.update("hidden");
    vi.advanceTimersByTime(200);
    expect(events).toEqual(["hold", "unhold", "release"]);
  });

  test("an unclassifiable viewport during the hold is not a dismissal", () => {
    const { hold, events, target } = setup();
    hold.begin(target);
    hold.update("visible");
    // Rotation with the keyboard open resets the detection baseline.
    hold.update("unknown");
    vi.advanceTimersByTime(5000);
    expect(events).toEqual(["hold"]);
    hold.update("hidden");
    vi.advanceTimersByTime(200);
    expect(events).toEqual(["hold", "unhold", "release"]);
  });

  test("a keyboard that never appears releases after the wait", () => {
    const { hold, events, target } = setup();
    hold.begin(target);
    vi.advanceTimersByTime(1499);
    expect(events).toEqual(["hold"]);
    vi.advanceTimersByTime(1 + 200);
    expect(events).toEqual(["hold", "unhold", "release"]);
  });

  test("a detection stuck on visible still releases after the close limit", () => {
    const { hold, events, target, blur } = setup();
    hold.begin(target);
    hold.update("visible");
    blur();
    vi.advanceTimersByTime(999);
    expect(events).toEqual(["hold"]);
    vi.advanceTimersByTime(1);
    expect(events).toEqual(["hold", "unhold", "release"]);
  });

  test("focus moving to another field keeps the hold until the keyboard hides", () => {
    const { hold, events, target, blur } = setup(true, () => true);
    hold.begin(target);
    hold.update("visible");
    // The paste dialog or find bar takes focus while the IME stays up.
    blur();
    vi.advanceTimersByTime(5000);
    expect(events).toEqual(["hold"]);
    hold.update("hidden");
    vi.advanceTimersByTime(200);
    expect(events).toEqual(["hold", "unhold", "release"]);
  });

  test("a late focus move into another field outlasts the close limit", () => {
    let stays = false;
    const { hold, events, target, blur } = setup(true, () => stays);
    hold.begin(target);
    hold.update("visible");
    // Tapping the paste button drops focus before the dialog focuses its field.
    blur();
    stays = true;
    vi.advanceTimersByTime(5000);
    expect(events).toEqual(["hold"]);
    hold.update("hidden");
    vi.advanceTimersByTime(200);
    expect(events).toEqual(["hold", "unhold", "release"]);
  });

  test("an unknown signal while closing waits for the keyboard to hide", () => {
    const { hold, events, target, blur } = setup();
    hold.begin(target);
    hold.update("visible");
    blur();
    // A rotation rebuilds the detection baseline during the close animation.
    hold.update("unknown");
    vi.advanceTimersByTime(200);
    expect(events).toEqual(["hold"]);
    hold.update("hidden");
    vi.advanceTimersByTime(200);
    expect(events).toEqual(["hold", "unhold", "release"]);
  });

  test("a moving viewport keeps the closing hold past the first limit", () => {
    const { hold, events, target, blur } = setup();
    hold.begin(target);
    hold.update("visible");
    blur();
    hold.update("unknown");
    vi.advanceTimersByTime(900);
    hold.update("unknown");
    vi.advanceTimersByTime(900);
    expect(events).toEqual(["hold"]);
    hold.update("hidden");
    vi.advanceTimersByTime(200);
    expect(events).toEqual(["hold", "unhold", "release"]);
  });

  test("reopening after a dismissal holds again", () => {
    const { hold, events, target } = setup();
    hold.begin(target);
    hold.update("visible");
    hold.update("hidden");
    vi.advanceTimersByTime(200);
    hold.begin(target);
    hold.update("visible");
    vi.advanceTimersByTime(5000);
    expect(events).toEqual(["hold", "unhold", "release", "hold"]);
  });

  test("cancel releases now; dispose stays silent", () => {
    const first = setup();
    first.hold.begin(first.target);
    first.hold.cancel();
    expect(first.events).toEqual(["hold", "unhold", "release"]);
    first.hold.cancel();
    expect(first.events).toHaveLength(3);

    const second = setup();
    second.hold.begin(second.target);
    second.hold.dispose();
    vi.advanceTimersByTime(5000);
    second.blur();
    expect(second.events).toEqual(["hold"]);
  });

  test("does nothing when holding is not allowed", () => {
    const { hold, events, target } = setup(false);
    hold.begin(target);
    hold.update("visible");
    vi.advanceTimersByTime(5000);
    expect(events).toEqual([]);
  });
});
