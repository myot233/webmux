import { z } from "zod";
import { SessionInfoSchema } from "./session-schemas";
import { ChatAttachmentSchema } from "./chat-schemas";

// Persistent cursors stay decimal strings, independent of chat generation/seq.
export const ConversationCursorSchema = z
  .string()
  .refine(
    (value) =>
      /^(0|[1-9][0-9]{0,18})$/.test(value) &&
      BigInt(value) <= 9223372036854775807n,
    "Invalid persistent history cursor",
  );

export const ConversationActivitySchema = z.object({
  exportable: z.boolean(),
  exclusionReason: z.string(),
  activityId: z.string().min(1),
  conversationId: z.string().min(1),
  providerThreadId: z.string().min(1),
  runId: z.string(),
  position: ConversationCursorSchema,
  runOrdinal: ConversationCursorSchema,
  kind: z.enum([
    "message",
    "command",
    "tool",
    "error",
    "interrupt",
    "file_change",
    "plan",
    "compaction",
    "turn_start",
    "turn_end",
    "permission",
    "marker",
  ]),
  role: z.enum(["", "user", "agent", "system"]),
  status: z.string(),
  text: z.string(),
  source: z.enum(["local", "live", "replay"]),
  providerItemId: z.string(),
  byteLength: ConversationCursorSchema,
  nextTextOffset: ConversationCursorSchema,
  truncated: z.boolean(),
  createdAt: z.iso.datetime({ offset: true }),
  attachments: z.array(
    z.object({
      name: z.string(),
      mimeType: z.string(),
      reference: z.string(),
      size: z.number().int().min(-1),
    }),
  ),
});

export const ConversationActivitiesSchema = z.object({
  items: z.array(ConversationActivitySchema),
  nextPosition: ConversationCursorSchema,
  gaps: z.array(
    z.object({
      id: z.string(),
      providerThreadId: z.string(),
      reason: z.string(),
      createdAt: z.iso.datetime({ offset: true }),
    }),
  ),
  reconcileRequired: z.boolean(),
  historyOrigin: z.enum(["unknown", "bounded_import", "complete"]),
});

export const HandoffLocationSchema = z.object({
  cwd: z.string(),
  repositoryId: z.string(),
  worktreeId: z.string(),
  newWorktreeBase: z.string(),
  newWorktreeBranch: z.string(),
});

export const HandoffPreviewRequestSchema = z.object({
  clientRequestId: z.string().regex(/^[a-zA-Z0-9_-]{8,64}$/),
  configId: z.string().min(1),
  targetProviderThreadId: z.string().min(1).optional(),
  agentConfig: z.record(z.string(), z.string()).optional(),
  userText: z.string(),
  fresh: z.boolean().optional(),
  fork: z.boolean().optional(),
  location: HandoffLocationSchema.partial().optional(),
  attachments: z.array(ChatAttachmentSchema).optional(),
});

export const HandoffEditRequestSchema = z
  .object({
    revision: z.number().int().positive(),
    text: z.string().optional(),
    userText: z.string().optional(),
  })
  .refine((value) => value.text !== undefined || value.userText !== undefined);

export const HandoffPreviewSchema = z.object({
  sourceLocation: HandoffLocationSchema,
  id: z.string().min(1),
  sourceConversationId: z.string().min(1),
  targetConversationId: z.string().min(1),
  targetProviderThreadId: z.string().min(1),
  configId: z.string().min(1),
  agentName: z.string(),
  agentConfig: z.record(z.string(), z.string()),
  userText: z.string(),
  fresh: z.boolean(),
  fork: z.boolean(),
  location: HandoffLocationSchema,
  attachments: z.array(ChatAttachmentSchema),
  strategy: z.enum([
    "full_thread_summary",
    "delta_since_target_last_seen",
    "manual_context",
  ]),
  state: z.enum([
    "preparing",
    "ready",
    "over_budget",
    "stale",
    "superseded",
    "delivering",
    "delivered",
    "unknown",
    "failed",
  ]),
  sourcePosition: ConversationCursorSchema,
  sourceRunOrdinal: ConversationCursorSchema,
  fromRunOrdinal: ConversationCursorSchema,
  toRunOrdinal: ConversationCursorSchema,
  revision: z.number().int().positive(),
  historyOrigin: z.enum(["unknown", "bounded_import", "complete"]),
  reconcileRequired: z.boolean(),
  stale: z.boolean(),
  manual: z.boolean(),
  renderedText: z.string(),
  selectedItemIds: z.array(z.string()),
  omittedItemIds: z.array(z.string()),
  omittedItems: z.array(
    z.object({
      activityId: z.string(),
      reason: z.enum(["budget", "manual_context"]),
    }),
  ),
  selectedCount: z.number().int().nonnegative(),
  omittedCount: z.number().int().nonnegative(),
  selectionTruncated: z.boolean(),
  budget: z.object({
    window: z.number().int().nonnegative(),
    nativeUsed: z.number().int().nonnegative(),
    current: z.number().int().nonnegative(),
    reserve: z.number().int().nonnegative(),
    history: z.number().int().nonnegative(),
    used: z.number().int().nonnegative(),
    source: z.enum(["estimated", "provider"]),
    overBudget: z.boolean(),
  }),
  nativeAvailable: z.boolean(),
  historyToolAvailable: z.boolean(),
});

export const HandoffSelectionSchema = z.object({
  sourceLocation: HandoffLocationSchema,
  sourceConversationId: z.string().min(1),
  sourcePosition: ConversationCursorSchema,
  revision: z.number().int().positive(),
  historyOrigin: z.enum(["unknown", "bounded_import", "complete"]),
  reconcileRequired: z.boolean(),
  manual: z.boolean(),
  selectedCount: z.number().int().nonnegative(),
  omittedCount: z.number().int().nonnegative(),
  items: z
    .array(
      z.object({
        activityId: z.string().min(1),
        position: ConversationCursorSchema,
        selected: z.boolean(),
        reason: z.enum(["", "budget", "manual_context"]),
      }),
    )
    .max(100),
  nextPosition: ConversationCursorSchema,
  done: z.boolean(),
});

export const ConversationMessageRequestSchema = z.object({
  clientRequestId: z.string().regex(/^[A-Za-z0-9_-]{8,64}$/),
  handoffId: z.string().min(1),
  revision: z.number().int().positive(),
  text: z.string(),
  configId: z.string().min(1),
  agentConfig: z.record(z.string(), z.string()),
  location: HandoffLocationSchema,
  attachments: z.array(ChatAttachmentSchema),
  retry: z.boolean(),
});

export const HandoffDeliverySchema = z.object({
  clientRequestId: z.string(),
  retry: z.boolean(),
  session: SessionInfoSchema.nullable(),
  executionId: z.string().min(1),
  deliveryId: z.string().min(1),
  handoffId: z.string().min(1),
  sourceConversationId: z.string().min(1),
  targetConversationId: z.string().min(1),
  targetProviderThreadId: z.string().min(1),
  sessionId: z.string().min(1),
  status: z.enum(["pending", "inline", "injected", "unknown", "failed"]),
  historyStatus: z.enum([
    "none",
    "pending",
    "injected",
    "unsupported",
    "unknown",
  ]),
  promptStatus: z.enum(["pending", "consumed", "unknown", "failed"]),
  phase: z.enum(["reserved", "worktree", "session", "ready", "sending"]),
  step: z.enum(["precheck", "worktree", "session", "config", "prompt"]),
  errorClass: z.string(),
  worktreeId: z.string(),
  worktreePath: z.string(),
  nativeContinue: z.boolean(),
  replacementHandoffId: z.string(),
  updatedAt: z.string(),
});

export const HandoffDraftSchema = z.object({
  source: z.object({
    id: z.string(),
    cwd: z.string(),
    repositoryId: z.string(),
    worktreeId: z.string(),
  }),
  preview: HandoffPreviewSchema.nullable(),
  delivery: HandoffDeliverySchema.nullable(),
});
export type HandoffPreview = z.infer<typeof HandoffPreviewSchema>;
export type HandoffDelivery = z.infer<typeof HandoffDeliverySchema>;
export type HandoffDraft = z.infer<typeof HandoffDraftSchema>;
export type HandoffLocation = z.infer<typeof HandoffLocationSchema>;
