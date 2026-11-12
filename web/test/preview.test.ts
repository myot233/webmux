import { describe, expect, test } from "vitest";
import { selectPreview } from "../src/domains/workspace/preview";

const output = Array.from({ length: 12 }, (_, index) => `line ${index + 1}`);

describe("session preview selection", () => {
  test("skips two footer rows without reducing the ten output rows", () => {
    expect(
      selectPreview([...output, "status bar", "prompt"].join("\n"), {
        lines: 10,
        skip: 2,
      }),
    ).toBe(output.slice(2).join("\n"));
  });

  test("zero skip includes the final row", () => {
    expect(selectPreview("first\nsecond\nlast", { lines: 2, skip: 0 })).toBe(
      "second\nlast",
    );
  });

  test("short output does not wrap negative indices into hidden footer rows", () => {
    expect(
      selectPreview("content\nstatus\nprompt", { lines: 10, skip: 2 }),
    ).toBe("content");
    expect(selectPreview("status\nprompt", { lines: 10, skip: 2 })).toBe("");
    expect(selectPreview("prompt", { lines: 10, skip: 2 })).toBe("");
    expect(selectPreview("", { lines: 10, skip: 0 })).toBe("");
  });
});
