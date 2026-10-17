import { describe, expect, test } from "vitest";
import {
  commandLabel,
  commandPreview,
  normalizeCommand,
  parseQuickCommands,
} from "../src/quickCommands";
import { QuickCommandsSchema, type QuickCommand } from "../src/schemas";

const command = (overrides: Partial<QuickCommand> = {}): QuickCommand => ({
  id: "a1",
  name: "",
  command: "git status",
  enter: false,
  ...overrides,
});

describe("normalizeCommand", () => {
  test("drops every trailing newline so only `enter` submits", () => {
    expect(normalizeCommand("git status\n\n")).toBe("git status");
    expect(normalizeCommand("git status\r\n\r\n")).toBe("git status");
  });

  test("unifies CRLF and CR without touching leading or inner whitespace", () => {
    expect(normalizeCommand("  first\r\n\tsecond\rthird  ")).toBe(
      "  first\n\tsecond\nthird  ",
    );
  });
});

describe("QuickCommandsSchema", () => {
  test("rejects blank bodies and over-long fields", () => {
    expect(
      QuickCommandsSchema.safeParse([command({ command: " \n " })]).success,
    ).toBe(false);
    expect(
      QuickCommandsSchema.safeParse([command({ command: "x".repeat(4097) })])
        .success,
    ).toBe(false);
    expect(
      QuickCommandsSchema.safeParse([command({ name: "n".repeat(41) })])
        .success,
    ).toBe(false);
  });

  test("caps the list at 50 commands", () => {
    const list = (count: number) =>
      Array.from({ length: count }, (_, index) => command({ id: `c${index}` }));
    expect(QuickCommandsSchema.safeParse(list(50)).success).toBe(true);
    expect(QuickCommandsSchema.safeParse(list(51)).success).toBe(false);
  });
});

describe("parseQuickCommands", () => {
  test("offers the /resume and /new presets when nothing was ever saved", () => {
    expect(parseQuickCommands("")).toEqual([
      { id: "preset-resume", name: "", command: "/resume", enter: true },
      { id: "preset-new", name: "", command: "/new", enter: true },
    ]);
  });

  test("a saved empty list stays empty instead of restoring presets", () => {
    expect(parseQuickCommands("[]")).toEqual([]);
  });

  test("keeps saved order", () => {
    const saved = [command({ id: "b" }), command({ id: "a" })];
    expect(parseQuickCommands(JSON.stringify(saved))).toEqual(saved);
  });

  test("corrupt or off-contract data degrades to an empty list", () => {
    expect(parseQuickCommands("{not json")).toEqual([]);
    expect(parseQuickCommands(JSON.stringify([{ id: "x" }]))).toEqual([]);
  });

  test("duplicate IDs invalidate the list before keyed rendering", () => {
    const saved = [command(), command({ command: "git diff" })];
    expect(parseQuickCommands(JSON.stringify(saved))).toEqual([]);
  });
});

describe("labels", () => {
  test("unnamed commands use the first non-blank body line", () => {
    const multi = command({ command: "\n  first line\nsecond line" });
    expect(commandLabel(multi)).toBe("first line");
    expect(commandPreview(multi)).toBe("first line …");
    expect(commandLabel(command({ name: "状态" }))).toBe("状态");
    expect(commandPreview(command())).toBe("git status");
  });
});
