import { describe, expect, test } from "vitest";
import {
  applyFrame,
  conversationTurns,
  diffStats,
  emptyChat,
  groupTimeline,
  resumePoint,
  type ChatModel,
} from "../src/domains/chat/chat-model";
import {
  ChatFrameSchema,
  type ChatEvent,
  type ChatFrame,
} from "../src/contracts/schemas";

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
  outdated: [],
  eligibility: { available: true },
  turn: { running: false },
  requests: [],
  config: [],
  commands: [],
  plan: null,
  usage: null,
  queue: { paused: false, items: [] },
  steering: true,
  forceSwitchNote: "",
  authRequired: false,
  permissionMode: "auto",
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

  test("a merged run and the rest of a partly applied run give the same text", () => {
    const message = (text: string) => ({
      messageId: "a1",
      role: "agent" as const,
      text,
    });
    // A new device replays the log: sequences 1-3 arrive as one event.
    let fresh = applyFrame(emptyChat(), snapshot("g1", { from: 1 }));
    fresh = applyFrame(
      fresh,
      events([ev({ kind: "message.append", message: message("Done. ok") }, 3)]),
    );
    // A device that applied the first chunk is sent only the rest, under the
    // sequence of the run's last chunk.
    let partial = applyFrame(emptyChat(), snapshot("g1", { from: 1 }));
    partial = applyFrame(
      partial,
      events([ev({ kind: "message.append", message: message("Do") }, 1)]),
    );
    partial = applyFrame(
      partial,
      events([ev({ kind: "message.append", message: message("ne. ok") }, 3)]),
    );
    for (const m of [fresh, partial]) {
      expect(
        m.entries.map((e) => (e.type === "message" ? e.text : e.type)),
      ).toEqual(["Done. ok"]);
      expect(resumePoint(m)).toEqual({ generation: "g1", seq: 3 });
    }
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

  test("info.set replaces the session title", () => {
    let m = connect();
    m = applyFrame(
      m,
      events([ev({ kind: "info.set", info: { title: "修复排序" } })]),
    );
    expect(m.title).toBe("修复排序");
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
  test("folds thinking and tool calls between replies into one work group", () => {
    let m = connect();
    m = applyFrame(
      m,
      events([
        ev({
          kind: "message.append",
          message: { messageId: "u1", role: "user", text: "go" },
        }),
        ev({
          kind: "message.append",
          message: { messageId: "t1", role: "thought", text: "plan" },
        }),
        ev({
          kind: "tool.upsert",
          tool: { toolCallId: "r1", kind: "read", title: "a" },
        }),
        ev({
          kind: "message.append",
          message: { messageId: "t2", role: "thought", text: "more" },
        }),
        ev({
          kind: "tool.upsert",
          tool: { toolCallId: "e1", kind: "edit", title: "c" },
        }),
        ev({
          kind: "message.append",
          message: { messageId: "a1", role: "agent", text: "x" },
        }),
        ev({
          kind: "tool.upsert",
          tool: { toolCallId: "r3", kind: "read", title: "d" },
        }),
        ev({
          kind: "message.append",
          message: { messageId: "u2", role: "user", text: "again" },
        }),
      ]),
    );
    expect(
      groupTimeline(m.entries).map((e) =>
        e.type === "work" ? e.steps.map((s) => s.id) : e.id,
      ),
    ).toEqual(["u1", ["t1", "r1", "t2", "e1"], "a1", ["r3"], "u2"]);
  });
});

describe("conversation turn navigation", () => {
  test("keeps user and agent messages in order and numbers turns by prompts", () => {
    let m = connect();
    m = applyFrame(
      m,
      events([
        ev({
          kind: "message.append",
          message: { messageId: "a0", role: "agent", text: "Earlier reply" },
        }),
        ev({
          kind: "message.append",
          message: { messageId: "t1", role: "thought", text: "hidden" },
        }),
        ev({
          kind: "tool.upsert",
          tool: { toolCallId: "r1", kind: "read", title: "hidden" },
        }),
        ev({
          kind: "message.append",
          message: {
            messageId: "u1",
            role: "user",
            text: " \n ",
            attachments: [
              {
                path: "/tmp/reference.png",
                name: "reference.png",
                mimeType: "image/png",
                size: 100,
                kind: "image",
              },
            ],
          },
        }),
        ev({
          kind: "message.append",
          message: { messageId: "a1", role: "agent", text: "First reply" },
        }),
        ev({
          kind: "message.append",
          message: { messageId: "a2", role: "agent", text: "Second reply" },
        }),
        ev({
          kind: "message.append",
          message: { messageId: "u2", role: "user", text: " next\n step " },
        }),
      ]),
    );

    expect(conversationTurns(m.entries)).toEqual([
      { id: "a0", role: "agent", turn: 0, summary: "Earlier reply" },
      { id: "u1", role: "user", turn: 1, summary: "用户附件" },
      { id: "a1", role: "agent", turn: 1, summary: "First reply" },
      { id: "a2", role: "agent", turn: 1, summary: "Second reply" },
      { id: "u2", role: "user", turn: 2, summary: "next step" },
    ]);
  });

  test("only adds an empty agent reply once it has visible content", () => {
    let m = connect();
    m = applyFrame(
      m,
      events([
        ev({
          kind: "message.append",
          message: { messageId: "a1", role: "agent", text: " \n " },
        }),
        ev({
          kind: "message.append",
          message: {
            messageId: "cut",
            role: "agent",
            text: "",
            truncated: true,
          },
        }),
      ]),
    );
    expect(conversationTurns(m.entries)).toEqual([
      { id: "cut", role: "agent", turn: 0, summary: "内容已截断" },
    ]);
    m = applyFrame(
      m,
      events([
        ev({
          kind: "message.append",
          message: { messageId: "a1", role: "agent", text: "回复正文" },
        }),
      ]),
    );
    expect(conversationTurns(m.entries)).toEqual([
      { id: "a1", role: "agent", turn: 0, summary: "回复正文" },
      { id: "cut", role: "agent", turn: 0, summary: "内容已截断" },
    ]);
  });

  test("bounds summaries as a streaming message grows", () => {
    let m = connect();
    m = applyFrame(
      m,
      events([
        ev({
          kind: "message.append",
          message: { messageId: "a1", role: "agent", text: "x".repeat(60) },
        }),
        ev({
          kind: "message.append",
          message: { messageId: "a1", role: "agent", text: "y".repeat(60) },
        }),
      ]),
    );

    const [item] = conversationTurns(m.entries);
    expect(item?.summary).toHaveLength(81);
    expect(item?.summary).toBe(`${"x".repeat(60)}${"y".repeat(20)}…`);
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
