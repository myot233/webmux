import { z } from "zod";

// Hybrid sessions and their chat channel (internal/contract/hybrid.go and
// chat.go). Re-exported from schemas.ts, the single contract entry point.

const datetime = z.iso.datetime({ offset: true });

export const HybridSideSchema = z.enum(["tui", "acp", "switching"]);
export type HybridSide = z.infer<typeof HybridSideSchema>;
export const HybridStateSchema = z.enum(["active", "closed", "interrupted"]);
export type HybridState = z.infer<typeof HybridStateSchema>;
export const HybridPhaseSchema = z.enum([
  "interrupting",
  "awaiting_choice",
  "cancelling",
  "stopping",
  "starting",
  "loading",
  "rolling_back",
]);
export type HybridPhase = z.infer<typeof HybridPhaseSchema>;

export const SessionHybridSchema = z.object({
  side: HybridSideSchema,
  phase: HybridPhaseSchema.optional(),
  dormant: z.boolean(),
  state: HybridStateSchema,
  agent: z.string(),
  agentSessionId: z.string().nullable(),
});
export type SessionHybrid = z.infer<typeof SessionHybridSchema>;

export const HybridSwitchResultSchema = z.object({
  side: HybridSideSchema,
  draft: z.string().optional(),
  dropped: z.array(z.string()),
  unsupported: z.array(z.string()),
});
export type HybridSwitchResult = z.infer<typeof HybridSwitchResultSchema>;

export const HybridRecordSchema = z.object({
  id: z.string(),
  name: z.string(),
  configId: z.string(),
  configMissing: z.boolean(),
  agent: z.string(),
  cwd: z.string(),
  cmd: z.array(z.string()),
  side: HybridSideSchema,
  agentSessionId: z.string().nullable(),
  previousIds: z.array(z.string()),
  state: HybridStateSchema,
  activity: z.enum(["idle", "turn", "switching"]).optional(),
  queuedCount: z.number().int().nonnegative(),
  createdAt: datetime,
  updatedAt: datetime,
  interruptedAt: datetime.nullable(),
});
export type HybridRecord = z.infer<typeof HybridRecordSchema>;
export const HybridRecordsSchema = z.array(HybridRecordSchema);

export const HybridCandidateSchema = z.object({
  agentSessionId: z.string(),
  title: z.string(),
  updatedAt: datetime,
});
export type HybridCandidate = z.infer<typeof HybridCandidateSchema>;

export const HybridConflictSchema = z.object({
  error: z.string(),
  reason: z.string(),
  eligibility: z.string().optional(),
  candidates: z.array(HybridCandidateSchema).optional(),
});
export type HybridConflict = z.infer<typeof HybridConflictSchema>;

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

export const ChatContentSchema = z.object({
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

export const ChatToolSchema = z.object({
  toolCallId: z.string().min(1),
  kind: z.string().optional(),
  title: z.string().optional(),
  status: ToolStatusSchema.optional(),
  content: z.array(ChatContentSchema).optional(),
  locations: z.array(ChatLocationSchema).optional(),
});
export type ChatTool = z.infer<typeof ChatToolSchema>;

export const ChatPlanSchema = z.object({
  entries: z.array(
    z.object({
      content: z.string(),
      priority: z.string(),
      status: z.enum(["pending", "in_progress", "completed"]),
    }),
  ),
});
export type ChatPlan = z.infer<typeof ChatPlanSchema>;

export const ChatCommandSchema = z.object({
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

export const ChatUsageSchema = z.object({
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
  type: z.enum(["string", "enum", "boolean", "unsupported"]),
  required: z.boolean(),
  enum: z.array(z.string()).optional(),
  enumNames: z.array(z.string()).optional(),
  default: z.unknown().optional(),
});
export type ChatFormField = z.infer<typeof ChatFormFieldSchema>;

export const ChatRequestSchema = z.object({
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

export const StopReasonSchema = z.enum([
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

export const ChatEventSchema = z.discriminatedUnion("kind", [
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
    turnStart: z.object({ turnId: z.string(), promptId: z.string() }),
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
      kind: z.enum([
        "loaded",
        "switched",
        "truncated",
        "reconnected",
        "resumed",
      ]),
      side: HybridSideSchema.optional(),
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

export const ChatStateSchema = z.object({
  side: HybridSideSchema,
  phase: HybridPhaseSchema.optional(),
  dormant: z.boolean(),
  agent: z.string(),
  agentVersion: z.string().optional(),
  verifiedBelow: z.string().optional(),
  agentSessionId: z.string().nullable(),
  eligibility: z.object({
    available: z.boolean(),
    reason: z.string().optional(),
  }),
  turn: z.object({
    running: z.boolean(),
    turnId: z.string().optional(),
    stopReason: StopReasonSchema.optional(),
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
  promptCapabilities: z
    .object({ image: z.boolean(), embeddedContext: z.boolean() })
    .default({ image: false, embeddedContext: false }),
  permissionMode: ChatPermissionModeSchema,
  switchError: z.string().optional(),
  unsupported: z.array(z.string()),
  idChange: z.object({ from: z.string(), to: z.string() }).nullable(),
  queueLost: z.number().int().nonnegative().optional(),
  recovery: z.object({ state: HybridStateSchema }).nullable(),
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

export const ChatPromptResultSchema = z.object({
  queued: z.boolean(),
  itemId: z.string().optional(),
});
export type ChatPromptResult = z.infer<typeof ChatPromptResultSchema>;
