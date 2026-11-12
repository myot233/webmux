import { getContext, setContext } from "svelte";

const key = Symbol("session switcher");
type OpenSwitcher = (invoker: HTMLElement | null) => void;

export function provideSessionSwitcher(open: OpenSwitcher) {
  setContext(key, open);
}

export function findSessionSwitcher() {
  return getContext<OpenSwitcher | undefined>(key);
}

const apple = /Macintosh|iPhone|iPad/.test(navigator.userAgent);
export const sessionSwitcherKeys = apple ? "Meta+K" : "Control+Alt+K";
export const sessionSwitcherKeyLabel = apple ? "⌘ K" : "Ctrl + Alt + K";

export function isSessionSwitcherShortcut(event: KeyboardEvent) {
  return (
    !event.repeat &&
    !event.isComposing &&
    event.keyCode !== 229 &&
    !event.getModifierState("AltGraph") &&
    !event.shiftKey &&
    event.key.toLowerCase() === "k" &&
    (apple
      ? event.metaKey && !event.ctrlKey && !event.altKey
      : event.ctrlKey && event.altKey && !event.metaKey)
  );
}
