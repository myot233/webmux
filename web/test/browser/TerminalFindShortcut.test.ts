import { expect, test, vi } from "vitest";
import { terminalFindShortcut } from "../../src/hooks/terminal/terminal-find-shortcut";

test("macOS shortcut ownership follows terminal/chat view changes", () => {
  const platform = vi
    .spyOn(navigator, "platform", "get")
    .mockReturnValue("MacIntel");
  const column = document.createElement("div");
  const reader = document.createElement("button");
  column.append(reader);
  document.body.append(column);
  const openTerminalFind = vi.fn();
  const readerKeydown = vi.fn();
  reader.addEventListener("keydown", readerKeydown);
  const shortcut = terminalFindShortcut(column, undefined);
  const pressFind = () => {
    const event = new KeyboardEvent("keydown", {
      key: "f",
      metaKey: true,
      bubbles: true,
      cancelable: true,
    });
    reader.dispatchEvent(event);
    return event;
  };
  try {
    expect(pressFind().defaultPrevented).toBe(false);
    expect(readerKeydown).toHaveBeenCalledTimes(1);
    expect(openTerminalFind).not.toHaveBeenCalled();

    shortcut.update(openTerminalFind);
    expect(pressFind().defaultPrevented).toBe(true);
    expect(readerKeydown).toHaveBeenCalledTimes(1);
    expect(openTerminalFind).toHaveBeenCalledTimes(1);

    shortcut.update(undefined);
    expect(pressFind().defaultPrevented).toBe(false);
    expect(readerKeydown).toHaveBeenCalledTimes(2);
    expect(openTerminalFind).toHaveBeenCalledTimes(1);
  } finally {
    shortcut.destroy();
    column.remove();
    platform.mockRestore();
  }
});
