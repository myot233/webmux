import { isTerminalFindShortcut } from "../../domains/terminal/terminal-search";

// The terminal column also hosts chat. An absent callback leaves shortcuts to
// the active chat reader (or browser) instead of claiming terminal ownership.
export function terminalFindShortcut(
  node: HTMLElement,
  open: (() => void) | undefined,
) {
  const keydown = (event: KeyboardEvent) => {
    if (!open || !isTerminalFindShortcut(event, navigator.platform)) return;
    event.preventDefault();
    event.stopPropagation();
    open();
  };
  node.addEventListener("keydown", keydown, true);
  return {
    update(next: (() => void) | undefined) {
      open = next;
    },
    destroy: () => node.removeEventListener("keydown", keydown, true),
  };
}
