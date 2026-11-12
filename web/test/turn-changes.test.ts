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

  test("creation followed by editing reports the final new file, not intermediate churn", () => {
    const result = replay([
      start("live"),
      diff("create", undefined, "keep\noriginal"),
      diff("edit", "keep\noriginal", "keep\nreplacement"),
      end("live"),
    ]);
    const entry = result.entries.at(-1);
    expect(entry).toMatchObject({
      type: "turnEnd",
      changes: [{ path: "main.ts", oldText: "", newText: "keep\nreplacement" }],
    });
    expect(entry?.type === "turnEnd" && entry.changes).toHaveLength(1);
    expect(entry?.type === "turnEnd" && diffStats(entry.changes![0]!)).toEqual({
      added: 2,
      removed: 0,
    });
  });

  test.each([true, false])(
    "a truncated %s new-file fragment remains independent",
    (first) => {
      const result = replay([
        start("live"),
        diff("create", undefined, "keep\noriginal", first),
        diff("edit", "keep\noriginal", "keep\nreplacement", !first),
        end("live"),
      ]);
      const entry = result.entries.at(-1);
      expect(entry?.type === "turnEnd" && entry.changes).toHaveLength(2);
      expect(entry).toMatchObject({
        changes: [
          { newText: "keep\noriginal" },
          { oldText: "keep\noriginal", newText: "keep\nreplacement" },
        ],
      });
    },
  );

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
      {
        kind: "tool.upsert",
        tool: {
          toolCallId: "patch-only",
          status: "completed",
          content: [
            {
              type: "diff",
              path: "main.ts",
              text: "@@ -1 +1 @@\n-before\n+middle",
            },
          ],
        },
      },
      diff("known", "middle", "after"),
      diff("clear", "other", undefined),
      diff("unknown-last", undefined, undefined),
      end("live"),
    ]);
    expect(result.entries.at(-1)).toMatchObject({
      changes: [
        { path: "main.ts" },
        { path: "main.ts" },
        { text: "@@ -1 +1 @@\n-before\n+middle" },
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

  test("late live-tool diffs update the original card without becoming the next turn's changes", () => {
    const result = replay([
      start("first"),
      {
        kind: "tool.upsert",
        tool: {
          toolCallId: "background",
          title: "Background edit",
          kind: "edit",
          status: "in_progress",
        },
      },
      end("first"),
      start("second"),
      diff("background", "before", "after"),
      end("second"),
    ]);
    expect(result.entries.filter((entry) => entry.type === "turnEnd")).toEqual([
      { type: "turnEnd", id: "first", stopReason: "end_turn" },
      { type: "turnEnd", id: "second", stopReason: "end_turn" },
    ]);
    expect(result.entries[0]).toMatchObject({
      type: "tool",
      id: "background",
      status: "completed",
      content: [
        { type: "diff", path: "main.ts", oldText: "before", newText: "after" },
      ],
    });
  });

  test("an explicitly restarted tool ID belongs to its new turn", () => {
    const result = replay([
      start("first"),
      diff("reused", "before", "middle"),
      end("first"),
      start("second"),
      {
        kind: "tool.upsert",
        tool: { toolCallId: "reused", status: "in_progress" },
      },
      diff("reused", "middle", "after"),
      end("second"),
    ]);
    expect(
      result.entries.filter((entry) => entry.type === "turnEnd"),
    ).toMatchObject([
      {
        type: "turnEnd",
        id: "first",
        stopReason: "end_turn",
        changes: [
          {
            type: "diff",
            path: "main.ts",
            oldText: "before",
            newText: "middle",
          },
        ],
      },
      {
        type: "turnEnd",
        id: "second",
        stopReason: "end_turn",
        changes: [
          {
            type: "diff",
            path: "main.ts",
            oldText: "middle",
            newText: "after",
          },
        ],
      },
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
