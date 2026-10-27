import { describe, expect, test } from "vitest";
import { isTerminalCopyShortcut } from "../src/hooks/terminal-input";

const plain = {
  key: "c",
  code: "KeyC",
  ctrlKey: false,
  shiftKey: false,
  altKey: false,
  metaKey: false,
  isComposing: false,
  keyCode: 67,
};

describe("terminal copy shortcut", () => {
  test("Windows and Linux copy on Ctrl+C without extra modifiers", () => {
    expect(isTerminalCopyShortcut({ ...plain, ctrlKey: true }, "Linux")).toBe(
      true,
    );
    expect(isTerminalCopyShortcut({ ...plain, ctrlKey: true }, "Win32")).toBe(
      true,
    );
    expect(
      isTerminalCopyShortcut(
        { ...plain, ctrlKey: true, shiftKey: true },
        "Linux",
      ),
    ).toBe(false);
    expect(
      isTerminalCopyShortcut(
        { ...plain, ctrlKey: true, altKey: true },
        "Linux",
      ),
    ).toBe(false);
    expect(
      isTerminalCopyShortcut(
        { ...plain, ctrlKey: true, metaKey: true },
        "Linux",
      ),
    ).toBe(false);
  });

  test("macOS copies on Cmd+C and leaves Ctrl+C to the terminal", () => {
    expect(
      isTerminalCopyShortcut({ ...plain, metaKey: true }, "MacIntel"),
    ).toBe(true);
    expect(
      isTerminalCopyShortcut({ ...plain, ctrlKey: true }, "MacIntel"),
    ).toBe(false);
    expect(
      isTerminalCopyShortcut(
        { ...plain, metaKey: true, shiftKey: true },
        "MacIntel",
      ),
    ).toBe(false);
  });

  test("non-Latin layouts still copy on the physical C key", () => {
    expect(
      isTerminalCopyShortcut({ ...plain, key: "с", ctrlKey: true }, "Linux"),
    ).toBe(true);
  });

  test("IME composition, stale key codes and other keys never copy", () => {
    expect(
      isTerminalCopyShortcut(
        { ...plain, ctrlKey: true, isComposing: true },
        "Linux",
      ),
    ).toBe(false);
    expect(
      isTerminalCopyShortcut(
        { ...plain, ctrlKey: true, keyCode: 229 },
        "Linux",
      ),
    ).toBe(false);
    expect(
      isTerminalCopyShortcut(
        { ...plain, key: "v", code: "KeyV", ctrlKey: true },
        "Linux",
      ),
    ).toBe(false);
  });
});
