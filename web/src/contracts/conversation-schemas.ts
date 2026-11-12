import { z } from "zod";

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
