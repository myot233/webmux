import { describe, expect, it } from "vitest";
import { ConversationCursorSchema } from "../src/schemas";

describe("persistent history cursors", () => {
  it("preserves int64 positions beyond JavaScript's integer precision", () => {
    for (const value of ["0", "1", "9007199254740993", "9223372036854775807"]) {
      expect(ConversationCursorSchema.parse(value)).toBe(value);
    }
  });

  it("rejects ambiguous, malformed and overflowing representations", () => {
    for (const value of [
      "",
      "01",
      "-1",
      "+1",
      "1e3",
      "1.0",
      "0 OR 1=1",
      "9223372036854775808",
    ]) {
      expect(ConversationCursorSchema.safeParse(value).success).toBe(false);
    }
  });
});
