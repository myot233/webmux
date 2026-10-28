import { describe, expect, test } from "vitest";
import {
  applyFrame,
  diffStats,
  emptyChat,
  groupTimeline,
  resumePoint,
  type ChatModel,
} from "../src/chat-model";
import {
  ChatFrameSchema,
  type ChatEvent,
  type ChatFrame,
} from "../src/schemas";

const at = "2026-10-28T00:00:00Z";
let seq = 0;
type Payload = ChatEvent extends infer E
  ? E extends ChatEvent
    ? Omit<E, "seq" | "at">
    : never
  : never;
const ev = (payload: Payload, n = seq++): ChatEvent =>
  ({ ...payload, seq: n, at }) as ChatEvent;
const events = (items: ChatEvent[], generation = "g1"): ChatFrame => ({
  type: "events",
  generation,
  items,
});
const state = {
  side: "acp",
  dormant: false,
  agent: "omp",
  agentSessionId: "01a",
  eligibility: { available: true },
  turn: { running: false },
  requests: [],
  config: [],
  commands: [],
  plan: null,
  usage: null,
  queue: { paused: false, items: [] },
  steering: true,
  authRequired: false,
  permissionMode: "ask",
  unsupported: [],
  idChange: null,
  recovery: null,
} as const;
const snapshot = (
  generation: string,
  extra: Partial<Extract<ChatFrame, { type: "snapshot" }>> = {},
): ChatFrame =>
  ChatFrameSchema.parse({ type: "snapshot", generation, state, ...extra });

function connect(generation = "g1"): ChatModel {
  seq = 0;
  return applyFrame(emptyChat(), snapshot(generation));
}

describe("chat reducer", () => {
  test("appends message chunks by messageId and keeps roles apart", () => {
    let m = connect();
    m = applyFrame(
      m,
      events([
        ev({
          kind: "message.append",
          message: { messageId: "u1", role: "user", text: "fix" },
        }),
        ev({
          kind: "message.append",
          message: { messageId: "t1", role: "thought", text: "look" },
        }),
        ev({
          kind: "message.append",
          message: { messageId: "a1", role: "agent", text: "Do" },
        }),
        ev({
          kind: "message.append",
          message: { messageId: "a1", role: "agent", text: "ne." },
        }),
      ]),
    );
    expect(
      m.entries.map((e) => (e.type === "message" ? [e.role, e.text] : e.type)),
    ).toEqual([
      ["user", "fix"],
      ["thought", "look"],
      ["agent", "Done."],
    ]);
    expect(resumePoint(m)).toEqual({ generation: "g1", seq: 3 });
  });

  test("tool updates replace one row; content and status follow the latest patch", () => {
    let m = connect();
    m = applyFrame(
      m,
      events([
        ev({
          kind: "tool.upsert",
          tool: {
            toolCallId: "c1",
            kind: "edit",
            title: "Edit a.ts",
            status: "pending",
          },
        }),
        ev({
          kind: "tool.upsert",
          tool: {
            toolCallId: "c1",
            status: "completed",
            content: [
              {
                type: "diff",
                path: "a.ts",
                oldText: "a\nb\nc",
                newText: "a\nB\nc\nd",
              },
            ],
          },
        }),
      ]),
    );
    expect(m.entries).toHaveLength(1);
    const tool = m.entries[0]!;
    expect(
      tool.type === "tool" && [tool.title, tool.status, tool.partial],
    ).toEqual(["Edit a.ts", "completed", false]);
    expect(tool.type === "tool" && diffStats(tool.content[0]!)).toEqual({
      added: 2,
      removed: 1,
    });
  });

  test("a patch for an unknown tool call creates a partial placeholder", () => {
    let m = connect();
    m = applyFrame(
      m,
      events([
        ev({
          kind: "tool.upsert",
          tool: { toolCallId: "lost", status: "completed" },
        }),
      ]),
    );
    const tool = m.entries[0]!;
    expect(
      tool.type === "tool" && [tool.partial, tool.status, tool.title],
    ).toEqual([true, "completed", ""]);
  });

  test("replayed events below the resume point are not applied twice", () => {
    let m = connect();
    const first = ev({
      kind: "message.append",
      message: { messageId: "a1", role: "agent", text: "x" },
    });
    m = applyFrame(m, events([first]));
    m = applyFrame(m, snapshot("g1", { from: 0 }));
    m = applyFrame(
      m,
      events([
        first,
        ev({
          kind: "message.append",
          message: { messageId: "a1", role: "agent", text: "y" },
        }),
      ]),
    );
    expect(m.entries[0]!.type === "message" && m.entries[0]!.text).toBe("xy");
  });

  test("reset, a new generation or stale events replace or keep the timeline", () => {
    let m = connect();
    m = applyFrame(
      m,
      events([ev({ kind: "notice", notice: { level: "info", text: "n" } })]),
    );
    expect(applyFrame(m, snapshot("g1")).entries).toHaveLength(1);
    expect(
      applyFrame(m, snapshot("g1", { reset: true, from: 7 })).entries,
    ).toEqual([]);
    expect(applyFrame(m, snapshot("g1", { reset: true, from: 7 })).next).toBe(
      7,
    );
    expect(applyFrame(m, snapshot("g2")).entries).toEqual([]);
    expect(
      applyFrame(
        m,
        events(
          [ev({ kind: "notice", notice: { level: "info", text: "old" } })],
          "g0",
        ),
      ),
    ).toBe(m);
  });

  test("a truncated log is reported from the snapshot", () => {
    const m = applyFrame(
      emptyChat(),
      snapshot("g1", { from: 40, truncated: true }),
    );
    expect([m.truncated, m.next]).toEqual([true, 40]);
  });

  test("closing a permission request annotates its tool call", () => {
    let m = connect();
    m = applyFrame(
      m,
      events([
        ev({
          kind: "tool.upsert",
          tool: {
            toolCallId: "c1",
            kind: "execute",
            title: "rm",
            status: "pending",
          },
        }),
        ev({
          kind: "request.open",
          request: {
            requestId: "r1",
            kind: "permission",
            toolCallId: "c1",
            options: [
              { optionId: "once", name: "允许一次", kind: "allow_once" },
              { optionId: "never", name: "拒绝", kind: "reject_once" },
            ],
          },
        }),
        ev({
          kind: "request.close",
          resolved: { requestId: "r1", outcome: "selected", optionId: "once" },
        }),
        ev({
          kind: "turn.end",
          turnEnd: { turnId: "t", stopReason: "cancelled" },
        }),
      ]),
    );
    // The tool row shows which choice answered it, by its label and kind.
    expect(m.entries[0]!.type === "tool" && m.entries[0]!.permission).toEqual({
      outcome: "selected",
      optionId: "once",
      name: "允许一次",
      kind: "allow_once",
    });
    expect(m.entries[1]).toEqual({
      type: "turnEnd",
      id: "t",
      stopReason: "cancelled",
    });
  });
});

describe("timeline grouping", () => {
  test("merges consecutive read-only calls, keeps single reads and edits", () => {
    let m = connect();
    m = applyFrame(
      m,
      events([
        ev({
          kind: "tool.upsert",
          tool: { toolCallId: "r1", kind: "read", title: "a" },
        }),
        ev({
          kind: "tool.upsert",
          tool: { toolCallId: "r2", kind: "search", title: "b" },
        }),
        ev({
          kind: "tool.upsert",
          tool: { toolCallId: "e1", kind: "edit", title: "c" },
        }),
        ev({
          kind: "tool.upsert",
          tool: { toolCallId: "r3", kind: "read", title: "d" },
        }),
      ]),
    );
    expect(
      groupTimeline(m.entries).map((e) =>
        e.type === "reads" ? e.calls.map((c) => c.id) : e.id,
      ),
    ).toEqual([["r1", "r2"], "e1", "r3"]);
  });
});

test("diff stats count lines of new and deleted files", () => {
  expect(diffStats({ type: "diff", newText: "a\nb" })).toEqual({
    added: 2,
    removed: 0,
  });
  expect(diffStats({ type: "diff", oldText: "a\nb", newText: "" })).toEqual({
    added: 0,
    removed: 2,
  });
});
