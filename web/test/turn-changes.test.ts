import { describe, expect, test } from "vitest";
import { applyFrame, emptyChat } from "../src/domains/chat/chat-model";
import type { ChatEvent } from "../src/contracts/schemas";

type EventBody = ChatEvent extends infer E
  ? E extends ChatEvent
    ? Omit<E, "seq" | "at">
    : never
  : never;
const start = (turnId: string): EventBody => ({
  kind: "turn.start",
  turnStart: { turnId, promptId: turnId },
});
const end = (turnId: string): EventBody => ({
  kind: "turn.end",
  turnEnd: { turnId, stopReason: "end_turn" },
});
const diff = (
  toolCallId: string,
  oldText: string,
  newText: string,
  truncated = false,
): EventBody => ({
  kind: "tool.upsert",
  tool: {
    toolCallId,
    status: "completed",
    content: [{ type: "diff", path: "main.ts", oldText, newText, truncated }],
  },
});
function replay(items: EventBody[]) {
  return applyFrame(emptyChat(), {
    type: "events",
    items: items.map((item, seq) => ({
      ...item,
      seq,
      at: "2026-11-12T00:00:00Z",
    })),
  });
}

describe("turn changes", () => {
  test("preserves independent fragments and composes in content-update order", () => {
    const result = replay([
      diff("historical", "old", "history"),
      { kind: "marker", marker: { kind: "loaded" } },
      start("live"),
      {
        kind: "tool.upsert",
        tool: { toolCallId: "first", status: "pending" },
      },
      diff("second", "before", "middle"),
      diff("first", "middle", "after"),
      diff("separate", "different range", "another edit"),
      end("live"),
    ]);
    expect(result.entries.at(-1)).toMatchObject({
      type: "turnEnd",
      changes: [
        { path: "main.ts", oldText: "before", newText: "after" },
        {
          path: "main.ts",
          oldText: "different range",
          newText: "another edit",
        },
      ],
    });
  });

  test("cold history without a turn start never creates a turn summary", () => {
    const result = replay([
      diff("historical", "before", "after"),
      end("historical"),
      { kind: "marker", marker: { kind: "loaded" } },
      start("live"),
      end("live"),
    ]);
    expect(result.entries.filter((entry) => entry.type === "turnEnd")).toEqual([
      { type: "turnEnd", id: "historical", stopReason: "end_turn" },
      { type: "turnEnd", id: "live", stopReason: "end_turn" },
    ]);
  });

  test("a later empty turn cannot inherit edits from the previous turn", () => {
    const result = replay([
      start("first"),
      diff("edit", "before", "after"),
      end("first"),
      start("second"),
      end("second"),
    ]);
    expect(result.entries.at(-1)).toEqual({
      type: "turnEnd",
      id: "second",
      stopReason: "end_turn",
    });
  });

  test("status-only completion keeps content but failed tools are excluded", () => {
    const result = replay([
      start("live"),
      diff("ok", "before", "after"),
      {
        kind: "tool.upsert",
        tool: { toolCallId: "ok", status: "in_progress" },
      },
      diff("failed", "failed before", "failed after"),
      { kind: "tool.upsert", tool: { toolCallId: "failed", status: "failed" } },
      { kind: "tool.upsert", tool: { toolCallId: "ok", status: "completed" } },
      end("live"),
    ]);
    expect(result.entries.at(-1)).toMatchObject({
      changes: [{ oldText: "before", newText: "after" }],
    });
  });

  test("a truncated fragment cannot silently compose with a later edit", () => {
    const result = replay([
      start("live"),
      diff("first", "before", "middle", true),
      diff("second", "middle", "after"),
      end("live"),
    ]);
    expect(result.entries.at(-1)).toMatchObject({
      changes: [
        { oldText: "before", newText: "middle", truncated: true },
        { oldText: "middle", newText: "after" },
      ],
    });
  });
});
