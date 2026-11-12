import { z } from "zod";

// Session views, archives and the chat view's channel
// (internal/contract/views.go and chat.go). Re-exported from schemas.ts, the
// single contract entry point.

const datetime = z.iso.datetime({ offset: true });

// A session has the terminal view, and with a chat-enabled launch config
// also the chat view.
export const SessionViewSchema = z.enum(["terminal", "chat"]);
export type SessionView = z.infer<typeof SessionViewSchema>;
// Ordinary sessions are always active; a session with a chat view becomes
// closed or interrupted and then lives on as an archive record.
export const SessionStateSchema = z.enum(["active", "closed", "interrupted"]);
const TransitionPhaseSchema = z.enum([
  "interrupting",
  "awaiting_choice",
  "cancelling",
  "stopping",
  "starting",
  "loading",
  "rolling_back",
]);
export type TransitionPhase = z.infer<typeof TransitionPhaseSchema>;

// A view change in progress.
export const SessionTransitionSchema = z.object({
  to: SessionViewSchema,
  phase: TransitionPhaseSchema,
});

// The agent behind a session's chat view.
export const SessionAgentSchema = z.object({
  name: z.string(),
  // The agent's display name and its brand glyph, a launch-config icon key.
  title: z.string(),
  icon: z.string(),
  sessionId: z.string().nullable(),
  // Present only while the agent is working (a running turn): the
  // tool call or plan step it is on, one line. Omitted otherwise.
  work: z.string().optional(),
});

// What a closed or interrupted session's archive record adds.
export const SessionArchiveSchema = z.object({
  configMissing: z.boolean(),
  previousAgentSessionIds: z.array(z.string()),
  activity: z.enum(["idle", "turn", "switching"]).optional(),
  queuedCount: z.number().int().nonnegative(),
  interruptedAt: datetime.nullable(),
  updatedAt: datetime,
});

export const ViewChangeResultSchema = z.object({
  view: SessionViewSchema,
  draft: z.string().optional(),
  dropped: z.array(z.string()),
  unsupported: z.array(z.string()),
});
export type ViewChangeResult = z.infer<typeof ViewChangeResultSchema>;

// The worktree a record's session ran in was released to free disk space:
// its original path and the commit it was on, kept so the worktree can be
// rebuilt there before the session resumes. `branch` is omitted when the
// worktree was detached.
export const HybridReleaseSchema = z.object({
  id: z.string(),
  repositoryId: z.string(),
  path: z.string(),
  commit: z.string(),
  branch: z.string().optional(),
  releasedAt: datetime,
});
export type HybridRelease = z.infer<typeof HybridReleaseSchema>;

const AgentCandidateSchema = z.object({
  agentSessionId: z.string(),
  title: z.string(),
  updatedAt: datetime,
});
export type AgentCandidate = z.infer<typeof AgentCandidateSchema>;

// A third-party connector that provides an agent's chat (ACP) side and is
// installed separately; its presence does not mean it is installed.
export const AgentConnectorSchema = z.object({
  executable: z.string(),
  title: z.string(),
  installCommand: z.string(),
  docsUrl: z.string(),
});
export type AgentConnector = z.infer<typeof AgentConnectorSchema>;

// The 409 body of every session, view, input, interrupt, resume and chat
// command; reason is a stable code.
export const SessionConflictSchema = z.object({
  error: z.string(),
  reason: z.string(),
  eligibility: z.string().optional(),
  candidates: z.array(AgentCandidateSchema).optional(),
  connector: AgentConnectorSchema.optional(),
  submission: z.unknown().optional(),
});
export type SessionConflict = z.infer<typeof SessionConflictSchema>;

export const ViewChangeRequestSchema = z
  .object({
    clientRequestId: z.string(),
    to: SessionViewSchema.optional(),
    choice: z.enum(["wait", "force", "cancel"]).optional(),
    agentSessionId: z.string().optional(),
  })
  .strict();

const ToolStatusSchema = z.enum([
  "pending",
  "in_progress",
  "completed",
  "failed",
]);
export type ToolStatus = z.infer<typeof ToolStatusSchema>;

export const ChatAttachmentSchema = z.object({
  path: z.string(),
  name: z.string().optional(),
  mimeType: z.string().optional(),
  size: z.number().int().nonnegative().optional(),
  kind: z.enum(["image", "file"]).optional(),
});
export type ChatAttachment = z.infer<typeof ChatAttachmentSchema>;

const ChatContentSchema = z.object({
  type: z.enum(["text", "diff", "resource_link", "unsupported"]),
  text: z.string().optional(),
  path: z.string().optional(),
  oldText: z.string().optional(),
  newText: z.string().optional(),
  uri: z.string().optional(),
  name: z.string().optional(),
  mimeType: z.string().optional(),
  unsupported: z.string().optional(),
  truncated: z.boolean().optional(),
});
export type ChatContent = z.infer<typeof ChatContentSchema>;

const ChatLocationSchema = z.object({
  path: z.string(),
  line: z.number().int().optional(),
});

const ChatToolSchema = z.object({
  toolCallId: z.string().min(1),
  kind: z.string().optional(),
  title: z.string().optional(),
  status: ToolStatusSchema.optional(),
  content: z.array(ChatContentSchema).optional(),
  locations: z.array(ChatLocationSchema).optional(),
});

const ChatPlanSchema = z.object({
  entries: z.array(
    z.object({
      content: z.string(),
      priority: z.string(),
      status: z.enum(["pending", "in_progress", "completed"]),
    }),
  ),
});
export type ChatPlan = z.infer<typeof ChatPlanSchema>;

const ChatCommandSchema = z.object({
  name: z.string(),
  description: z.string(),
  hint: z.string().optional(),
});
export type ChatCommand = z.infer<typeof ChatCommandSchema>;

export const ChatConfigSchema = z.object({
  id: z.string(),
  name: z.string(),
  category: z.string().optional(),
  type: z.enum(["select", "boolean"]),
  value: z.string().optional(),
  enabled: z.boolean().optional(),
  options: z.array(
    z.object({
      value: z.string(),
      name: z.string(),
      description: z.string().optional(),
    }),
  ),
});
export type ChatConfig = z.infer<typeof ChatConfigSchema>;

const ChatUsageSchema = z.object({
  used: z.number().nonnegative(),
  size: z.number().nonnegative(),
  cost: z.number().optional(),
  currency: z.string().optional(),
});
export type ChatUsage = z.infer<typeof ChatUsageSchema>;

const ChatOptionSchema = z.object({
  optionId: z.string(),
  name: z.string(),
  kind: z.enum(["allow_once", "allow_always", "reject_once", "reject_always"]),
});
export type ChatOption = z.infer<typeof ChatOptionSchema>;

const ChatFormFieldSchema = z.object({
  name: z.string(),
  title: z.string().optional(),
  description: z.string().optional(),
  type: z.enum(["string", "enum", "array", "boolean", "unsupported"]),
  required: z.boolean(),
  enum: z.array(z.string()).optional(),
  enumNames: z.array(z.string()).optional(),
  default: z.unknown().optional(),
});
export type ChatFormField = z.infer<typeof ChatFormFieldSchema>;

const ChatRequestSchema = z.object({
  requestId: z.string().min(1),
  kind: z.enum(["permission", "form"]),
  toolCallId: z.string().optional(),
  title: z.string().optional(),
  options: z.array(ChatOptionSchema).optional(),
  form: z
    .object({
      message: z.string(),
      fields: z.array(ChatFormFieldSchema),
      supported: z.boolean(),
    })
    .optional(),
});
export type ChatRequest = z.infer<typeof ChatRequestSchema>;

const StopReasonSchema = z.enum([
  "end_turn",
  "max_tokens",
  "max_turn_requests",
  "refusal",
  "cancelled",
  "error",
  "auth_required",
]);
export type StopReason = z.infer<typeof StopReasonSchema>;

const eventBase = { seq: z.number().int().nonnegative(), at: datetime };

const ChatEventSchema = z.discriminatedUnion("kind", [
  z.object({
    ...eventBase,
    kind: z.literal("message.append"),
    message: z.object({
      messageId: z.string().min(1),
      role: z.enum(["user", "agent", "thought"]),
      text: z.string(),
      truncated: z.boolean().optional(),
      attachments: z.array(ChatAttachmentSchema).optional(),
    }),
  }),
  z.object({
    ...eventBase,
    kind: z.literal("tool.upsert"),
    tool: ChatToolSchema,
  }),
  z.object({ ...eventBase, kind: z.literal("plan.set"), plan: ChatPlanSchema }),
  z.object({
    ...eventBase,
    kind: z.literal("commands.set"),
    commands: z.array(ChatCommandSchema).optional(),
  }),
  z.object({
    ...eventBase,
    kind: z.literal("config.set"),
    config: z.array(ChatConfigSchema).optional(),
  }),
  z.object({
    ...eventBase,
    kind: z.literal("usage.set"),
    usage: ChatUsageSchema,
  }),
  z.object({
    ...eventBase,
    kind: z.literal("info.set"),
    info: z.object({ title: z.string().optional() }),
  }),
  z.object({
    ...eventBase,
    kind: z.literal("compaction.upsert"),
    compaction: z.object({
      id: z.string(),
      status: z.enum(["running", "completed", "failed"]),
      summary: z.string().optional(),
    }),
  }),
  z.object({
    ...eventBase,
    kind: z.literal("notice"),
    notice: z.object({
      level: z.enum(["info", "warning", "error"]),
      text: z.string(),
    }),
  }),
  z.object({
    ...eventBase,
    kind: z.literal("turn.start"),
    turnStart: z.object({
      turnId: z.string(),
      promptId: z.string(),
      queueItemId: z.string().optional(),
    }),
  }),
  z.object({
    ...eventBase,
    kind: z.literal("turn.end"),
    turnEnd: z.object({
      turnId: z.string(),
      stopReason: StopReasonSchema,
      error: z.string().optional(),
    }),
  }),
  z.object({
    ...eventBase,
    kind: z.literal("request.open"),
    request: ChatRequestSchema,
  }),
  z.object({
    ...eventBase,
    kind: z.literal("request.close"),
    resolved: z.object({
      requestId: z.string(),
      outcome: z.enum(["selected", "cancelled", "accept", "decline", "cancel"]),
      optionId: z.string().optional(),
    }),
  }),
  z.object({
    ...eventBase,
    kind: z.literal("marker"),
    marker: z.object({
      kind: z.enum(["loaded", "switched", "truncated", "reconnected"]),
      view: SessionViewSchema.optional(),
    }),
  }),
]);
export type ChatEvent = z.infer<typeof ChatEventSchema>;

// ask: every permission request waits for the user; auto: the server
// approves it (forms always wait).
export const ChatPermissionModeSchema = z.enum(["ask", "auto"]);
export type ChatPermissionMode = z.infer<typeof ChatPermissionModeSchema>;

export const ChatPermissionModeRequestSchema = z
  .object({ clientRequestId: z.string(), mode: ChatPermissionModeSchema })
  .strict();

const ChatEligibilitySchema = z.object({
  available: z.boolean(),
  reason: z.string().optional(),
  connector: AgentConnectorSchema.optional(),
});
export type ChatEligibility = z.infer<typeof ChatEligibilitySchema>;

const ChatStateSchema = z.object({
  // Components running below their verified versions.
  outdated: z.array(
    z.object({ name: z.string(), version: z.string(), verified: z.string() }),
  ),
  // The chat side could not start because its connector is not installed.
  connectorMissing: AgentConnectorSchema.optional(),
  eligibility: ChatEligibilitySchema,
  turn: z.object({
    running: z.boolean(),
    unfinished: z.boolean().optional(),
    turnId: z.string().optional(),
    stopReason: StopReasonSchema.optional(),
    startedAt: datetime.optional(),
    endedAt: datetime.optional(),
  }),
  requests: z.array(ChatRequestSchema),
  config: z.array(ChatConfigSchema),
  commands: z.array(ChatCommandSchema),
  plan: ChatPlanSchema.nullable(),
  usage: ChatUsageSchema.nullable(),
  queue: z.object({
    paused: z.boolean(),
    items: z.array(
      z.object({
        id: z.string(),
        text: z.string(),
        createdAt: datetime,
        attachments: z.array(ChatAttachmentSchema).optional(),
      }),
    ),
  }),
  steering: z.boolean(),
  authRequired: z.boolean(),
  // What forcing a switch during a terminal turn loses, per agent.
  forceSwitchNote: z.string(),
  promptCapabilities: z
    .object({ image: z.boolean(), embeddedContext: z.boolean() })
    .default({ image: false, embeddedContext: false }),
  permissionMode: ChatPermissionModeSchema,
  switchError: z.string().optional(),
  unsupported: z.array(z.string()),
  idChange: z.object({ from: z.string(), to: z.string() }).nullable(),
  queueLost: z.number().int().nonnegative().optional(),
  recovery: z.object({ state: SessionStateSchema }).nullable(),
});
export type ChatState = z.infer<typeof ChatStateSchema>;

export const ChatFrameSchema = z.discriminatedUnion("type", [
  z.object({
    type: z.literal("snapshot"),
    generation: z.string().optional(),
    from: z.number().int().nonnegative().optional(),
    next: z.number().int().nonnegative().optional(),
    reset: z.boolean().optional(),
    truncated: z.boolean().optional(),
    state: ChatStateSchema,
  }),
  z.object({
    type: z.literal("events"),
    generation: z.string().optional(),
    from: z.number().int().nonnegative().optional(),
    items: z.array(ChatEventSchema).optional(),
  }),
  z.object({ type: z.literal("state"), state: ChatStateSchema }),
]);
export type ChatFrame = z.infer<typeof ChatFrameSchema>;

// The 200 body of POST /sessions/:id/input. On the chat view a sent prompt
// names its turn and a queued one its queue item and 1-based position; the
// terminal view sets only `view`.
export const SessionInputResultSchema = z.object({
  view: SessionViewSchema,
  queued: z.boolean().optional(),
  itemId: z.string().optional(),
  position: z.number().int().positive().optional(),
  turnId: z.string().optional(),
  promptId: z.string().optional(),
});

export const ChatNextPromptRequestSchema = z.object({
  turnId: z.string().min(1),
  endedAt: datetime,
});

export const ChatNextPromptSchema = z.object({ text: z.string().max(480) });
