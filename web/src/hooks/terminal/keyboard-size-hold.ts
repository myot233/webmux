import type { KeyboardVisibility } from "../shared/viewport.svelte";

// Opening the soft keyboard from the keyboard button holds the PTY grid: the
// host shrinks by the keyboard height and would otherwise refit and redraw
// full-screen programs on every open and close. Any close signal — the button,
// a system dismissal (Back, IME collapse key, iOS Done) or a lost focus — ends
// the hold only once the keyboard is gone and the viewport height has settled,
// so the closing animation cannot leak an intermediate size to the PTY.
const NO_KEYBOARD_MS = 1500;
const SETTLE_MS = 200;
const CLOSE_LIMIT_MS = 1000;

type Phase = "idle" | "holding" | "shown" | "closing";

export interface KeyboardSizeHold {
  // Starts a hold for an editable about to be focused.
  begin(target: EventTarget): void;
  // Called whenever the page's keyboard detection or visible viewport height
  // changes; while closing, each call restarts the settle delay.
  update(keyboard: KeyboardVisibility): void;
  // Ends a hold now and refits (the preference was turned off).
  cancel(): void;
  // Drops timers and listeners without notifying.
  dispose(): void;
}

export function createKeyboardSizeHold(options: {
  canHold: () => boolean;
  onActive: (active: boolean) => void;
  // Called after a hold ends, to fit the grid to the settled host.
  release: () => void;
  // True while focus sits in an editable field, which keeps the IME up even
  // when it is not the field that started the hold (the paste dialog, find bar).
  keyboardStays?: () => boolean;
}): KeyboardSizeHold {
  let phase: Phase = "idle";
  let keyboard: KeyboardVisibility = "hidden";
  let target: EventTarget | undefined;
  let waitTimer: ReturnType<typeof setTimeout> | undefined;
  let settleTimer: ReturnType<typeof setTimeout> | undefined;
  let limitTimer: ReturnType<typeof setTimeout> | undefined;

  const clear = () => {
    clearTimeout(waitTimer);
    clearTimeout(settleTimer);
    clearTimeout(limitTimer);
    waitTimer = settleTimer = limitTimer = undefined;
    target?.removeEventListener("focusout", close);
    target = undefined;
  };
  const finish = () => {
    clear();
    phase = "idle";
    options.onActive(false);
    options.release();
  };
  const settle = () => {
    clearTimeout(settleTimer);
    settleTimer = setTimeout(() => {
      // Unknown is not gone: a rotation or pinch during the closing animation
      // rebuilds the detection baseline and must wait for the real dismissal.
      // A focused field also keeps the IME up, so a visible keyboard there is
      // not stuck detection.
      if (keyboard === "hidden") finish();
    }, SETTLE_MS);
    // The ceiling only applies once the signal stops changing; every viewport
    // update restarts it so a keyboard animation cannot be cut short.
    clearTimeout(limitTimer);
    limitTimer = setTimeout(function limit() {
      if (keyboard === "visible" && options.keyboardStays?.()) {
        // Focus is still in a field, so the keyboard may legitimately stay up
        // (pasting from the dialog); wait for the real dismissal. Unknown is
        // still bounded here: it is not a dismissal, but it is also not proof
        // that the keyboard remains up.
        limitTimer = setTimeout(limit, CLOSE_LIMIT_MS);
        return;
      }
      finish();
    }, CLOSE_LIMIT_MS);
  };
  function close(event?: Event) {
    if (phase === "idle" || phase === "closing") return;
    // Focus moving to another field leaves the keyboard up: stay shown.
    if (event && options.keyboardStays?.()) return;
    phase = "closing";
    clearTimeout(waitTimer);
    target?.removeEventListener("focusout", close);
    target = undefined;
    settle();
  }

  return {
    begin(next) {
      if (!options.canHold()) return;
      const wasIdle = phase === "idle";
      clear();
      phase = keyboard === "visible" ? "shown" : "holding";
      target = next;
      target.addEventListener("focusout", close);
      if (phase === "holding") waitTimer = setTimeout(close, NO_KEYBOARD_MS);
      if (wasIdle) options.onActive(true);
    },
    update(nextKeyboard) {
      keyboard = nextKeyboard;
      if (phase === "holding" && keyboard === "visible") {
        phase = "shown";
        clearTimeout(waitTimer);
      } else if (phase === "shown" && keyboard === "hidden") {
        // A system dismissal that keeps focus: Back, gesture, IME collapse.
        close();
      } else if (phase === "closing") {
        settle();
      }
    },
    cancel() {
      if (phase !== "idle") finish();
    },
    dispose() {
      clear();
      phase = "idle";
    },
  };
}
