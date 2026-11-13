import {
  applyFrame,
  emptyChat,
  type ChatModel,
} from "../../src/domains/chat/chat-model";
import {
  ChatFrameSchema,
  CodeChangeSchema,
  type AgentConnector,
  type ChatEvent,
  type ChatRequest,
  type ChatState,
  type AgentCandidate,
  type SessionView,
  type TransitionPhase,
} from "../../src/contracts/schemas";

// Sample data for the hybrid session stories: the chat channel's frames as
// the server sends them (docs/protocol/http-hybrid.md), validated by the
// production schemas and reduced by the production reducer. No ACP process,
// PTY or API request is involved.

export type Agent = "omp" | "opencode" | "claudecode" | "devin";
export type Scenario =
  | "conversation"
  | "long-conversation"
  | "permission"
  | "commands"
  | "cancelled"
  | "compaction"
  | "switching"
  | "awaiting-choice"
  | "terminal"
  | "connector-missing"
  | "connector-recheck"
  | "first-message"
  | "ambiguous"
  | "occupied"
  | "mode-fallback"
  | "recovery-failure";

export const claudeConnector = {
  executable: "claude-agent-acp",
  title: "Claude Code ACP 适配器",
  installCommand: "npm install -g @agentclientprotocol/claude-agent-acp",
  docsUrl: "https://github.com/agentclientprotocol/claude-agent-acp",
} satisfies AgentConnector;

// Server-published profile facts, kept separate from the stable agent ID and
// its executable. Stories consume these exactly as the production UI does.
export const agentMetadata: Record<
  Agent,
  {
    title: string;
    icon: string;
    executable: string;
    connector?: AgentConnector;
    steering: boolean;
    forceSwitchNote: string;
  }
> = {
  omp: {
    title: "omp",
    icon: "pi",
    executable: "omp",
    steering: true,
    forceSwitchNote: "omp 已输出的部分可能丢失",
  },
  opencode: {
    title: "OpenCode",
    icon: "opencode",
    executable: "opencode",
    steering: false,
    forceSwitchNote: "已输出的部分可能丢失；回合不会在后台继续",
  },
  claudecode: {
    title: "Claude Code",
    icon: "claudecode",
    executable: "claude",
    connector: claudeConnector,
    steering: true,
    forceSwitchNote:
      "cancel 只中断当前回合，后台 Bash 或子任务可能继续运行；强制切换会终止连接器并等待受管后代退出",
  },
  devin: {
    title: "Devin CLI",
    icon: "devin",
    executable: "devin",
    steering: false,
    forceSwitchNote: "已输出的部分可能丢失",
  },
};

export const claudeCandidates = [
  {
    agentSessionId: "01a18715-8504-7020-9f3d-257900000000",
    title: "修复首页排序",
    updatedAt: "2026-01-01T11:58:00Z",
  },
  {
    agentSessionId: "01b29826-9615-8131-af4e-368a11111111",
    title: "更新 README",
    updatedAt: "2026-01-01T11:59:00Z",
  },
] satisfies AgentCandidate[];

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

const user = (
  messageId: string,
  text: string,
  attachments?: ChatState["queue"]["items"][number]["attachments"],
): EventBody => ({
  kind: "message.append",
  message: { messageId, role: "user", text, attachments },
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
  locations?: { path: string; line?: number }[],
): EventBody => ({
  kind: "tool.upsert",
  tool: {
    toolCallId,
    kind,
    title,
    status,
    content: content.map((diff) => ({ type: "diff", ...diff })),
    ...(locations ? { locations } : {}),
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
  { kind: "turn.start", turnStart: { turnId: "turn-1", promptId: "u1" } },
  user("u1", "修复首页搜索的排序：置顶的工作区应排在最前。", [
    {
      path: "/tmp/webmux-upload-story/home-order.png",
      name: "home-order.png",
      mimeType: "image/png",
      size: 48_213,
      kind: "image",
    },
    {
      path: "/tmp/webmux-upload-story/notes.txt",
      name: "notes.txt",
      mimeType: "text/plain",
      size: 1_204,
      kind: "file",
    },
  ]),
  {
    kind: "message.append",
    message: {
      messageId: "t1",
      role: "thought",
      text: "先找到首页的排序函数，确认置顶状态从哪里来，再补一个回归测试。",
    },
  },
  tool(
    "read-0",
    "read",
    "web/src/pages/Home.svelte",
    "completed",
    [],
    [{ path: "web/src/pages/Home.svelte", line: 48 }],
  ),
  tool(
    "read-1",
    "read",
    "web/src/homeWorktrees.ts",
    "completed",
    [],
    [{ path: "web/src/homeWorktrees.ts", line: 15 }],
  ),
  tool("read-2", "read", "web/src/hooks/sessions/session-list.ts", "completed"),
  tool("search-0", "search", "sortWorkspaces", "completed"),
  agentText(
    "a1",
    "排序只比较了 [homeWorktrees.ts](web/src/homeWorktrees.ts#L15-L30) 中的最近活动时间，没有考虑 [Home.svelte](file:///home/developer/webmux/web/src/pages/Home.svelte#L48) 的置顶状态。我先补测试，再修改 [`sortWorkspaces`](web/src/homeWorktrees.ts:15)。",
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

export const configs: Record<Agent, ChatState["config"]> = {
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
  claudecode: [
    {
      id: "model",
      name: "模型",
      category: "model",
      type: "select",
      value: "sonnet",
      options: [
        { value: "sonnet", name: "Sonnet", description: "均衡的速度与推理" },
        { value: "opus", name: "Opus", description: "深度思考与复杂任务" },
        { value: "haiku", name: "Haiku", description: "快速响应" },
      ],
    },
    {
      id: "effort",
      name: "Effort",
      category: "thought_level",
      type: "select",
      value: "high",
      options: ["low", "medium", "high"].map((value) => ({
        value,
        name: value,
      })),
    },
    {
      id: "mode",
      name: "模式",
      category: "mode",
      type: "select",
      value: "auto",
      options: [
        "default",
        "acceptEdits",
        "plan",
        "auto",
        "bypassPermissions",
      ].map((value) => ({ value, name: value })),
    },
  ],
  // Devin's catalog as `devin acp` advertises it, after the launch command's
  // --permission-mode dangerous seed selects Bypass Permissions.
  devin: [
    {
      id: "mode",
      name: "Session Mode",
      category: "mode",
      type: "select",
      value: "bypass",
      options: [
        {
          value: "accept-edits",
          name: "Code",
          description: "Write and edit code",
        },
        {
          value: "smart",
          name: "Smart",
          description: "Auto-approve actions the model judges safe",
        },
        {
          value: "ask",
          name: "Ask",
          description: "Answer questions without code changes",
        },
        {
          value: "plan",
          name: "Plan",
          description: "Plan changes before implementing",
        },
        {
          value: "bypass",
          name: "Bypass Permissions",
          description:
            "Auto-approve all tool calls unless your organization's policy requires approval",
        },
      ],
    },
    {
      id: "model",
      name: "Model",
      category: "model",
      type: "select",
      value: "swe-2-high",
      options: [
        { value: "adaptive", name: "Adaptive" },
        { value: "swe-2-high", name: "SWE-2 High" },
        { value: "swe-2-flash-medium", name: "SWE-2 Flash" },
        { value: "claude-fable-5-2-medium", name: "Claude Fable" },
      ],
    },
    {
      id: "thought_level",
      name: "Thinking Level",
      category: "thought_level",
      type: "select",
      value: "medium",
      options: ["low", "medium", "high"].map((value) => ({
        value,
        name: value,
      })),
    },
    {
      id: "speed",
      name: "Speed",
      category: "speed",
      type: "select",
      value: "standard",
      options: [
        { value: "standard", name: "Standard" },
        { value: "fast", name: "Fast" },
      ],
    },
  ],
};

const commonCommands: ChatState["commands"] = [
  { name: "compact", description: "压缩上下文，保留摘要" },
  { name: "model", description: "切换模型", hint: "模型名称" },
  { name: "plan", description: "进入计划模式" },
  { name: "review", description: "审阅当前改动", hint: "文件或提交" },
  { name: "commit", description: "生成提交说明并提交" },
];

// This is the list after the server profile removes authentication commands.
// The chat UI only renders the published list; it has no agent-specific filter.
const commands: Record<Agent, ChatState["commands"]> = {
  omp: commonCommands,
  opencode: commonCommands,
  devin: commonCommands,
  claudecode: [
    ...commonCommands,
    { name: "context", description: "查看当前上下文" },
    { name: "init", description: "初始化项目说明" },
  ],
};

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
  {
    id: "q1",
    text: "然后把 README 里的排序说明也更新一下",
    createdAt: at,
    attachments: [
      {
        path: "/tmp/webmux-upload-story/readme-diff.txt",
        name: "readme-diff.txt",
        mimeType: "text/plain",
        size: 840,
        kind: "file" as const,
      },
    ],
  },
  { id: "q2", text: "最后运行一次 make test-fast", createdAt: at },
];

export const sessionIds: Record<Agent, { current: string; previous: string }> =
  {
    omp: { current: "01a17e26c4b84f0e9d2a", previous: "01a17e25a9f04c11b7e3" },
    opencode: { current: "ses_7f3c2a91d0e4b5", previous: "ses_6e1b0f82c3d7a4" },
    claudecode: {
      current: "01a18715-8504-7020-9f3d-257900000000",
      previous: "019f8765-4321-7090-8a1b-250000000000",
    },
    devin: { current: "freckle-seeder", previous: "amber-falcon" },
  };

/** The chat channel of one scenario: its timeline and its latest state. */
export function chatFixture(scenario: Scenario, agent: Agent) {
  const terminalScenarios: Scenario[] = [
    "terminal",
    "awaiting-choice",
    "connector-recheck",
    "first-message",
    "ambiguous",
    "occupied",
  ];
  const idleScenarios: Scenario[] = [
    "commands",
    "cancelled",
    "long-conversation",
    "connector-missing",
    "recovery-failure",
    ...terminalScenarios,
  ];
  const running = !idleScenarios.includes(scenario);
  const events = [...history];
  if (scenario === "long-conversation") {
    for (let index = 0; index < 8; index++) {
      events.push(
        user(`long-u${index}`, `第 ${index + 2} 轮：继续完善排序和测试。`),
        {
          kind: "message.append",
          message: {
            messageId: `long-t${index}`,
            role: "thought",
            text: "检查已有实现和回归测试。",
          },
        },
        tool(
          `long-r${index}`,
          "read",
          `检查第 ${index + 2} 轮改动`,
          "completed",
        ),
        agentText(
          `long-a${index}`,
          Array.from(
            { length: 6 },
            (_, line) =>
              `第 ${index + 2} 轮回复，段落 ${line + 1}：已保留用户消息与工作步骤的顺序。`,
          ).join("\n\n"),
        ),
        agentText(`long-empty${index}`, ""),
        agentText(`long-space${index}`, " \n "),
      );
    }
  }
  if (scenario === "compaction") events.push(...compaction);
  if (scenario === "mode-fallback")
    events.push({
      kind: "notice",
      notice: {
        level: "warning",
        text: "当前模型不支持 auto，实际权限模式已回退到 acceptEdits",
      },
    });
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
        tool("edit-3", "edit", "编辑 Home.svelte 的另一片段", "completed", [
          {
            path: homeDiff.path,
            oldText: "const pinnedFirst = false;\n",
            newText: "const pinnedFirst = true;\n",
          },
        ]),
        tool("delete-1", "delete", "移除旧排序模块", "completed", [
          {
            path: "web/src/legacy-sort.ts",
            oldText: "export const legacySort = true;\n",
            newText: "",
          },
        ]),
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
  const start = applyFrame(
    emptyChat(),
    ChatFrameSchema.parse({
      type: "snapshot",
      generation: "story",
      from: 0,
      state: baseState(agent),
    }),
  );
  const model = withEvents(start, events);
  const phase: TransitionPhase | undefined =
    scenario === "switching"
      ? "stopping"
      : scenario === "awaiting-choice"
        ? "awaiting_choice"
        : undefined;
  const ids = sessionIds[agent];
  const eligibility: ChatState["eligibility"] =
    scenario === "first-message"
      ? { available: false, reason: "no_session_id" }
      : scenario === "occupied"
        ? { available: false, reason: "occupied" }
        : scenario === "connector-recheck"
          ? {
              available: false,
              reason: "connector_missing",
              connector: agentMetadata[agent].connector,
            }
          : { available: true };
  const config = configs[agent].map((option) =>
    scenario === "mode-fallback" && option.id === "mode"
      ? { ...option, value: "acceptEdits" }
      : option,
  );
  // Where the session is, as its SessionInfo says: the view it settled on
  // (or leaves, during a switch) and the switch's phase.
  const view: SessionView = terminalScenarios.includes(scenario)
    ? "terminal"
    : "chat";
  const state = ChatFrameSchema.parse({
    type: "state",
    state: {
      ...baseState(agent),
      eligibility,
      config,
      turn: running
        ? {
            running: true,
            startedAt: new Date(Date.now() - 65_000).toISOString(),
          }
        : scenario === "commands" || scenario === "cancelled"
          ? {
              running: false,
              stopReason: scenario === "cancelled" ? "cancelled" : "end_turn",
              startedAt: "2026-01-01T11:57:54Z",
              endedAt: at,
            }
          : { running: false },
      requests: scenario === "permission" ? [permissionRequest] : [],
      plan: plan(scenario === "commands"),
      queue: {
        paused: scenario === "cancelled",
        items:
          scenario === "conversation" || scenario === "cancelled" ? queued : [],
      },
      idChange:
        scenario === "terminal"
          ? { from: ids.previous, to: ids.current }
          : null,
      connectorMissing:
        scenario === "connector-missing"
          ? (agentMetadata[agent].connector ?? codexConnector)
          : undefined,
      switchError:
        scenario === "recovery-failure"
          ? "恢复会话失败：无法加载保存的 agent 会话"
          : undefined,
      recovery:
        scenario === "recovery-failure" ? { state: "interrupted" } : null,
    } satisfies ChatState,
  });
  if (state.type !== "state") throw new Error("expected state frame");
  return {
    model,
    state: state.state,
    view,
    phase,
    agentSessionId: ids.current,
  };
}

// The third-party connector Codex CLI's chat view runs through.
const codexConnector = {
  executable: "codex-acp",
  title: "Codex ACP 适配器",
  installCommand: "npm install -g @agentclientprotocol/codex-acp",
  docsUrl: "https://github.com/agentclientprotocol/codex-acp",
};

function baseState(agent: Agent): ChatState {
  return {
    outdated: [],
    eligibility: { available: true },
    turn: { running: false },
    requests: [],
    config: configs[agent],
    commands: commands[agent],
    plan: null,
    usage: { used: 26_800, size: 200_000, cost: 0.07, currency: "USD" },
    queue: { paused: false, items: [] },
    steering: agentMetadata[agent].steering,
    forceSwitchNote: agentMetadata[agent].forceSwitchNote,
    promptCapabilities: { image: true, embeddedContext: true },
    permissionMode: "auto",
    authRequired: false,
    unsupported: [],
    idChange: null,
    recovery: null,
  };
}

export const terminalLines = (agent: Agent) =>
  [
    ` ${agentMetadata[agent].title} · webmux`,
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
