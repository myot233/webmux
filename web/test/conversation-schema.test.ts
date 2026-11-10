import { describe, expect, it } from "vitest";
import {
  ConversationCursorSchema,
  HandoffPreviewRequestSchema,
  HandoffEditRequestSchema,
} from "../src/schemas";

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

describe("handoff preview inputs", () => {
  it("keeps editable context separate from the current request", () => {
    expect(
      HandoffPreviewRequestSchema.parse({
        clientRequestId: "preview-123",
        configId: "cfg",
        userText: "new task",
      }).userText,
    ).toBe("new task");
    expect(
      HandoffEditRequestSchema.parse({ revision: 2, text: "manual history" }),
    ).toEqual({ revision: 2, text: "manual history" });
  });
  it("rejects invalid optimistic revisions and missing command IDs", () => {
    expect(
      HandoffEditRequestSchema.safeParse({ revision: 0, text: "text" }).success,
    ).toBe(false);
    expect(
      HandoffPreviewRequestSchema.safeParse({
        configId: "cfg",
        userText: "task",
      }).success,
    ).toBe(false);
  });
});
