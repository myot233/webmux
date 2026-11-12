import { describe, expect, test } from "vitest";
import {
  applyFrame,
  diffStats,
  emptyChat,
} from "../src/domains/chat/chat-model";
import { ChatFrameSchema, type ChatEvent } from "../src/contracts/schemas";

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
  oldText: string | undefined,
  newText: string | undefined,
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
  return applyFrame(
    emptyChat(),
    ChatFrameSchema.parse(
      JSON.parse(
        JSON.stringify({
          type: "events",
          items: items.map((item, seq) => ({
            ...item,
            seq,
            at: "2026-11-12T00:00:00Z",
          })),
        }),
      ),
    ),
  );
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

  test.each([
    ["rewrites", "keep\nreplacement", { added: 1, removed: 1 }],
    ["restores", "keep\noriginal", { added: 0, removed: 0 }],
  ] as const)(
    "clear then %s composes across omitted empty newText",
    (_, after, stats) => {
      const result = replay([
        start("live"),
        diff("clear", "keep\noriginal", undefined),
        diff("rewrite", "", after),
        end("live"),
      ]);
      const entry = result.entries.at(-1);
      expect(entry).toMatchObject({
        type: "turnEnd",
        changes: [
          { path: "main.ts", oldText: "keep\noriginal", newText: after },
        ],
      });
      expect(entry?.type === "turnEnd" && entry.changes).toHaveLength(1);
      expect(
        entry?.type === "turnEnd" && diffStats(entry.changes![0]!),
      ).toEqual(stats);
    },
  );

  test("unknown and patch-only fragments remain independent", () => {
    const result = replay([
      start("live"),
      diff("unknown-first", undefined, undefined),
      diff("unknown-second", undefined, undefined),
      diff("patch-only", undefined, "middle"),
      diff("known", "middle", "after"),
      diff("clear", "other", undefined),
      diff("unknown-last", undefined, undefined),
      end("live"),
    ]);
    expect(result.entries.at(-1)).toMatchObject({
      changes: [
        { path: "main.ts" },
        { path: "main.ts" },
        { newText: "middle" },
        { oldText: "middle", newText: "after" },
        { oldText: "other", newText: "" },
        { path: "main.ts" },
      ],
    });
    const entry = result.entries.at(-1);
    expect(entry?.type === "turnEnd" && entry.changes).toHaveLength(6);
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
