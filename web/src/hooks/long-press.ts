import type { Attachment } from "svelte/attachments";

const holdMs = 500;
const moveTolerance = 8;
// How long after the finger lifts a compatibility click may still arrive.
const ghostClickMs = 800;

// A hold ends when the pointer moves far enough to become a scroll.
export function createLongPress() {
  let timer: ReturnType<typeof setTimeout> | undefined;
  let origin: { x: number; y: number } | undefined;
  const cancel = () => {
    clearTimeout(timer);
    timer = undefined;
    origin = undefined;
  };
  return {
    get pending() {
      return timer !== undefined;
    },
    start(event: PointerEvent, press: () => void) {
      cancel();
      origin = { x: event.clientX, y: event.clientY };
      timer = setTimeout(() => {
        timer = undefined;
        origin = undefined;
        press();
      }, holdMs);
    },
    move: (event: PointerEvent) => {
      if (
        origin &&
        Math.hypot(event.clientX - origin.x, event.clientY - origin.y) >=
          moveTolerance
      )
        cancel();
    },
    cancel,
  };
}

/**
 * Calls press after a touch or pen is held on the element, or with a
 * selector on the matching descendant where the press started (one listener
 * for a whole list, unaffected by its rows re-rendering). iOS Safari fires
 * no contextmenu event for a long press, so touch context actions need their
 * own gesture.
 *
 * The click that ends a long press is swallowed anywhere in the document:
 * whatever press opened (a sheet sliding up under the finger) must not
 * receive it. The browser's own long-press menu (Android) is suppressed
 * while a press is held.
 */
export function longPress(
  press: (element: HTMLElement) => void,
  selector?: string,
): Attachment<HTMLElement> {
  return (element) => {
    const document = element.ownerDocument;
    const hold = createLongPress();
    let fired = false;
    let ghostTimer: number | undefined;
    const swallow = (event: MouseEvent) => {
      event.preventDefault();
      event.stopImmediatePropagation();
      release();
    };
    const release = () => {
      clearTimeout(ghostTimer);
      document.removeEventListener("click", swallow, true);
    };
    const down = (event: PointerEvent) => {
      if (event.pointerType === "mouse" || !event.isPrimary) return;
      const target = selector
        ? (event.target as Element | null)?.closest<HTMLElement>(selector)
        : element;
      if (!target || !element.contains(target)) return;
      release();
      fired = false;
      hold.start(event, () => {
        fired = true;
        navigator.vibrate?.(10);
        press(target);
      });
    };
    const up = () => {
      hold.cancel();
      if (!fired) return;
      fired = false;
      document.addEventListener("click", swallow, true);
      ghostTimer = window.setTimeout(release, ghostClickMs);
    };
    const menu = (event: Event) => {
      if (hold.pending || fired) event.preventDefault();
    };
    element.addEventListener("pointerdown", down);
    element.addEventListener("pointermove", hold.move);
    element.addEventListener("pointercancel", hold.cancel);
    element.addEventListener("contextmenu", menu);
    // The finger may lift over the sheet that press opened, outside element.
    document.addEventListener("pointerup", up, true);
    return () => {
      hold.cancel();
      release();
      element.removeEventListener("pointerdown", down);
      element.removeEventListener("pointermove", hold.move);
      element.removeEventListener("pointercancel", hold.cancel);
      element.removeEventListener("contextmenu", menu);
      document.removeEventListener("pointerup", up, true);
    };
  };
}
