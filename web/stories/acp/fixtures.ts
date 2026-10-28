import { applyFrame, emptyChat, type ChatModel } from "../../src/chat-model";
import {
  ChatFrameSchema,
  ChatStateSchema,
  CodeChangeSchema,
  type ChatEvent,
  type ChatRequest,
  type ChatState,
} from "../../src/schemas";

// Sample data for the hybrid session stories: the chat channel's frames as
// the server sends them (docs/protocol/http-hybrid.md), validated by the
// production schemas and reduced by the production reducer. No ACP process,
// PTY or API request is involved.

export type Agent = "omp" | "opencode";
export type Scenario =
  | "conversation"
  | "permission"
  | "commands"
  | "cancelled"
  | "compaction"
  | "switching"
  | "awaiting-choice"
  | "terminal";

const at = "2026-01-01T11:59:00Z";
type EventBody = ChatEvent extends infer E
  ? E extends unknown
    ? Omit<E, "seq" | "at">
    : never
  : never;

/** Applies events after the model's last sequence, as the channel would. */
export function withEvents(model: ChatModel, items: EventBody[]): ChatModel {
  return applyFrame(
    model,
    ChatFrameSchema.parse({
      type: "events",
      generation: model.generation,
      items: items.map((item, i) => ({ ...item, seq: model.next + i, at })),
    }),
  );
}

const user = (messageId: string, text: string): EventBody => ({
  kind: "message.append",
  message: { messageId, role: "user", text },
});
const agentText = (messageId: string, text: string): EventBody => ({
  kind: "message.append",
  message: { messageId, role: "agent", text },
});
const tool = (
  toolCallId: string,
  kind: string,
  title: string,
  status: "pending" | "in_progress" | "completed" | "failed",
  content: { path: string; oldText: string; newText: string }[] = [],
): EventBody => ({
  kind: "tool.upsert",
  tool: {
    toolCallId,
    kind,
    title,
    status,
    content: content.map((diff) => ({ type: "diff", ...diff })),
  },
});

const lines = (count: number, prefix: string) =>
  Array.from({ length: count }, (_, i) => `${prefix} ${i}`).join("\n");
const homeDiff = {
  path: "web/src/pages/Home.svelte",
  oldText: lines(3, "old"),
  newText: lines(12, "new"),
};
const worktreesDiff = {
  path: "web/src/homeWorktrees.ts",
  oldText: lines(15, "before"),
  newText: lines(30, "after"),
};

const history: EventBody[] = [
  user("u1", "修复首页搜索的排序：置顶的工作区应排在最前。"),
  {
    kind: "message.append",
    message: {
      messageId: "t1",
      role: "thought",
      text: "先找到首页的排序函数，确认置顶状态从哪里来，再补一个回归测试。",
    },
  },
  tool("read-0", "read", "web/src/pages/Home.svelte", "completed"),
  tool("read-1", "read", "web/src/homeWorktrees.ts", "completed"),
  tool("read-2", "read", "web/src/session-list.ts", "completed"),
  tool("search-0", "search", "sortWorkspaces", "completed"),
  agentText(
    "a1",
    "排序只比较了最近活动时间，没有考虑置顶。我先补测试，再修改 `sortWorkspaces`。",
  ),
  tool("edit-1", "edit", "编辑 web/src/homeWorktrees.ts", "completed", [
    worktreesDiff,
  ]),
  tool("edit-2", "edit", "编辑 web/src/pages/Home.svelte", "in_progress", [
    homeDiff,
  ]),
];

const compaction: EventBody[] = [
  {
    kind: "compaction.upsert",
    compaction: {
      id: "c1",
      status: "completed",
      summary:
        "已定位 sortWorkspaces；置顶状态来自 homeWorktrees。计划先写测试再改实现。",
    },
  },
  user("u2", "继续，把 README 里的排序说明也更新一下。"),
  tool("read-3", "read", "README.md", "completed"),
  { kind: "compaction.upsert", compaction: { id: "c2", status: "running" } },
];

export const permissionRequest: ChatRequest = {
  requestId: "perm-1",
  kind: "permission",
  toolCallId: "exec-1",
  options: [
    { optionId: "once", name: "允许一次", kind: "allow_once" },
    { optionId: "always", name: "始终允许", kind: "allow_always" },
    { optionId: "reject", name: "拒绝", kind: "reject_once" },
  ],
};

const configs: Record<Agent, ChatState["config"]> = {
  omp: [
    {
      id: "model",
      name: "模型",
      category: "model",
      type: "select",
      value: "claude-haiku-4-5",
      options: [
        { value: "claude-haiku-4-5", name: "haiku-4-5" },
        { value: "claude-sonnet-4-5", name: "sonnet-4-5" },
      ],
    },
    {
      id: "thinking",
      name: "思考",
      category: "thought_level",
      type: "select",
      value: "off",
      options: ["off", "low", "medium", "high"].map((value) => ({
        value,
        name: value,
      })),
    },
  ],
  opencode: [
    {
      id: "model",
      name: "模型",
      category: "model",
      type: "select",
      value: "deepseek-v4.1-flash",
      options: [
        { value: "deepseek-v4.1-flash", name: "deepseek-v4.1-flash" },
        { value: "deepseek-v4-pro", name: "deepseek-v4-pro" },
      ],
    },
    {
      id: "mode",
      name: "模式",
      category: "mode",
      type: "select",
      value: "build",
      options: [
        { value: "build", name: "build" },
        { value: "plan", name: "plan" },
      ],
    },
  ],
};

const commands: ChatState["commands"] = [
  { name: "compact", description: "压缩上下文，保留摘要" },
  { name: "model", description: "切换模型", hint: "模型名称" },
  { name: "plan", description: "进入计划模式" },
  { name: "review", description: "审阅当前改动", hint: "文件或提交" },
  { name: "commit", description: "生成提交说明并提交" },
];

const plan = (done: boolean): ChatState["plan"] => ({
  entries: [
    ["定位排序函数", "completed"],
    ["写回归测试", "completed"],
    ["修改实现", done ? "completed" : "in_progress"],
    ["运行测试", done ? "completed" : "pending"],
  ].map(([content, status]) => ({
    content: content!,
    priority: "medium",
    status: status as "pending" | "in_progress" | "completed",
  })),
});

const queued = [
  { id: "q1", text: "然后把 README 里的排序说明也更新一下", createdAt: at },
  { id: "q2", text: "最后运行一次 make test-fast", createdAt: at },
];

export const sessionIds: Record<Agent, { current: string; previous: string }> =
  {
    omp: { current: "01a17e26c4b84f0e9d2a", previous: "01a17e25a9f04c11b7e3" },
    opencode: { current: "ses_7f3c2a91d0e4b5", previous: "ses_6e1b0f82c3d7a4" },
  };

/** The chat channel of one scenario: its timeline and its latest state. */
export function chatFixture(scenario: Scenario, agent: Agent) {
  const running = !["commands", "cancelled", "terminal"].includes(scenario);
  const events = [...history];
  if (scenario === "compaction") events.push(...compaction);
  if (scenario === "permission")
    events.push(
      tool(
        "exec-1",
        "execute",
        "pnpm --dir web exec vitest run test/homeWorktrees.test.ts",
        "pending",
      ),
      { kind: "request.open", request: permissionRequest },
    );
  if (scenario === "commands" || scenario === "cancelled") {
    events.push(
      tool(
        "edit-2",
        "edit",
        "编辑 web/src/pages/Home.svelte",
        scenario === "cancelled" ? "failed" : "completed",
      ),
    );
    if (scenario === "commands")
      events.push(
        agentText(
          "a2",
          "已修复排序并补充测试，12 个用例全部通过。置顶的工作区现在始终排在最前。",
        ),
      );
    events.push({
      kind: "turn.end",
      turnEnd: {
        turnId: "turn-1",
        stopReason: scenario === "cancelled" ? "cancelled" : "end_turn",
      },
    });
  }
  const start = applyFrame(emptyChat(), {
    type: "snapshot",
    generation: "story",
    from: 0,
    state: ChatStateSchema.parse(baseState(agent)),
  });
  const model = withEvents(start, events);
  const phase =
    scenario === "switching"
      ? "stopping"
      : scenario === "awaiting-choice"
        ? "awaiting_choice"
        : undefined;
  const ids = sessionIds[agent];
  const state = ChatStateSchema.parse({
    ...baseState(agent),
    side:
      scenario === "terminal" || scenario === "awaiting-choice"
        ? "tui"
        : scenario === "switching"
          ? "switching"
          : "acp",
    phase,
    agentSessionId: ids.current,
    turn: { running: running && !phase },
    requests: scenario === "permission" ? [permissionRequest] : [],
    plan: plan(scenario === "commands"),
    queue: {
      paused: scenario === "cancelled",
      items:
        scenario === "conversation" || scenario === "cancelled" ? queued : [],
    },
    idChange:
      scenario === "terminal" ? { from: ids.previous, to: ids.current } : null,
  } satisfies ChatState);
  return { model, state };
}

function baseState(agent: Agent): ChatState {
  return {
    side: "acp",
    dormant: false,
    agent,
    agentVersion: agent === "omp" ? "13.4.0" : "1.2.3",
    agentSessionId: sessionIds[agent].current,
    eligibility: { available: true },
    turn: { running: false },
    requests: [],
    config: configs[agent],
    commands,
    plan: null,
    usage: { used: 26_800, size: 200_000, cost: 0.07, currency: "USD" },
    queue: { paused: false, items: [] },
    steering: agent === "omp",
    authRequired: false,
    unsupported: [],
    idChange: null,
    recovery: null,
  };
}

export const terminalLines = [
  " omp  claude-haiku-4-5 · webmux",
  "",
  "› 修复首页搜索的排序：置顶的工作区应排在最前。",
  "",
  "  ✓ Read web/src/pages/Home.svelte",
  "  ✓ Read web/src/homeWorktrees.ts",
  "  ✓ Edit web/src/homeWorktrees.ts (+30 −15)",
  "  ✓ Edit web/src/pages/Home.svelte (+12 −3)",
  "",
  "  排序已修复，测试通过。",
  "",
  "› /new",
  "",
  "  New session started.",
  "",
  "› 继续：更新 README 中的排序说明",
  "  ● Editing README.md …",
  "",
].join("\n");

// The two files the chat changed, for the code panel.
export const codeChanges = [
  ["web/src/pages/Home.svelte", "M"],
  ["web/src/homeWorktrees.ts", "M"],
].map(([path, status], i) =>
  CodeChangeSchema.parse({
    nodeId: `acp-change-${i}`,
    changeId: `acp-change-${i}`,
    group: "unstaged",
    oldPath: path,
    newPath: path,
    status,
    oldOid: "a".repeat(40),
    newOid: "b".repeat(40),
    oldMode: "100644",
    newMode: "100644",
  }),
);
