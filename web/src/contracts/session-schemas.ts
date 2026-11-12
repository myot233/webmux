import { z } from "zod";
import {
  ChatAttachmentSchema,
  HybridReleaseSchema,
  SessionAgentSchema,
  SessionArchiveSchema,
  SessionStateSchema,
  SessionTransitionSchema,
  SessionViewSchema,
} from "./chat-schemas";

const WorkspaceLinkSchema = z.object({
  repositoryId: z.string(),
  worktreeId: z.string(),
  worktreePath: z.string(),
});

const AttentionSchema = z.object({
  kind: z.enum(["question", "permission", "done", "error", "notify", "bell"]),
  source: z.enum(["agent", "terminal", "cli"]),
  since: z.iso.datetime({ offset: true }),
  message: z.string().optional(),
});

export const ReportedCwdSchema = z.object({
  path: z.string(),
  host: z.string(),
  remote: z.boolean(),
  since: z.iso.datetime({ offset: true }),
});
export type ReportedCwd = z.infer<typeof ReportedCwdSchema>;

// A TCP port a session's process tree listens on (loopback or wildcard).
const ListeningPortSchema = z.object({
  port: z.number().int().min(1).max(65535),
  process: z.string(),
});
export type ListeningPort = z.infer<typeof ListeningPortSchema>;

const ForwardOptionsSchema = z.strictObject({
  rewriteHost: z.boolean(),
  rewriteOrigin: z.boolean(),
  forwardedHeaders: z.boolean(),
  rewriteResponse: z.boolean(),
  prefix: z.enum(["keep", "strip"]),
});
export type ForwardOptions = z.infer<typeof ForwardOptionsSchema>;
// A forwarded loopback port served by the gateway at `path`.
export const ForwardSchema = z.object({
  port: ListeningPortSchema.shape.port,
  path: z.string().startsWith("/forward/"),
  label: z.string(),
  // ID of the session the port was detected in, or "".
  session: z.string(),
  process: z.string(),
  options: ForwardOptionsSchema,
  status: z.enum(["listening", "unreachable"]),
  createdAt: z.iso.datetime({ offset: true }),
});
export type Forward = z.infer<typeof ForwardSchema>;
export const ForwardsSchema = z.array(ForwardSchema);
export const ForwardTicketSchema = z.object({
  code: z.string().min(1),
  expiresAt: z.iso.datetime({ offset: true }),
});
export const ForwardRequestSchema = z.strictObject({
  port: ListeningPortSchema.shape.port,
  label: z.string().max(64).optional(),
  session: z.string().optional(),
  process: z.string().optional(),
  options: ForwardOptionsSchema.optional(),
});
export type ForwardRequest = z.infer<typeof ForwardRequestSchema>;
export const ForwardUpdateSchema = z.strictObject({
  label: z.string().max(64).optional(),
  options: ForwardOptionsSchema.optional(),
});
export type ForwardUpdate = z.infer<typeof ForwardUpdateSchema>;

// A terminal run's generation: output offsets and submissions are bound to it.
const GenerationSchema = z.string().regex(/^[0-9a-f]{16}$/);
// Required response arrays encode as [] even when service slices are nil.
// Only fields explicitly marked nullable accept null. Commands remain nonempty.
export const SessionInfoSchema = z.object({
  id: z.string().regex(/^[a-z2-7]{8}$/),
  // Empty while a session has no terminal run (on its chat view, or
  // archived).
  generation: GenerationSchema.or(z.literal("")),
  title: z.string(),
  cwd: z.string(),
  cmd: z.array(z.string()).min(1),
  status: z.enum(["running", "waiting", "idle", "exited"]),
  exitCode: z.number().int().nullable(),
  preview: z.string(),
  createdAt: z.iso.datetime({ offset: true }),
  lastActiveAt: z.iso.datetime({ offset: true }),
  // 0 only on an archived session, which has no terminal.
  cols: z.number().int().min(0).max(1000),
  rows: z.number().int().min(0).max(1000),
  clients: z.number().int().nonnegative(),
  workspace: WorkspaceLinkSchema.nullable(),
  attention: AttentionSchema.nullable(),
  reportedCwd: ReportedCwdSchema.nullable(),
  // Saved launch config used at creation; absent for explicit commands.
  configId: z
    .string()
    .regex(/^[a-z2-7]{8}$/)
    .optional(),
  // active for every running session; closed and interrupted sessions are
  // archived, with zero runtime fields.
  state: SessionStateSchema,
  // ["terminal"], or ["terminal", "chat"] for a session with a chat view.
  views: z.array(SessionViewSchema).min(1),
  // The last settled view; during a transition still the one it leaves.
  view: SessionViewSchema,
  transition: SessionTransitionSchema.nullable(),
  // The agent behind the chat view; null without one.
  agent: SessionAgentSchema.nullable(),
  // Only on a closed or interrupted session.
  archive: SessionArchiveSchema.optional(),
  // Set while the record's worktree is released; omitted otherwise.
  released: HybridReleaseSchema.optional(),
  // Ascending ports the session's processes listen on; always [] where the
  // server cannot detect them.
  listeningPorts: z.array(ListeningPortSchema),
  // The session whose process created this one through the local API.
  parentId: z
    .string()
    .regex(/^[a-z2-7]{8}$/)
    .optional(),
  // Caller-defined labels; omitted when empty.
  labels: z.record(z.string(), z.string()).optional(),
});
export type SessionInfo = z.infer<typeof SessionInfoSchema>;
// A closed or interrupted session, as GET /sessions?state=archived lists
// it: only sessions with a chat view are archived.
export const ArchivedSessionSchema = SessionInfoSchema.extend({
  state: z.enum(["closed", "interrupted"]),
  agent: SessionAgentSchema,
  archive: SessionArchiveSchema,
});
export type ArchivedSession = z.infer<typeof ArchivedSessionSchema>;
export const ArchivedSessionsSchema = z.array(ArchivedSessionSchema);
export const PreviewSettingsSchema = z.object({
  lines: z.number().int().min(1).max(50),
  skip: z.number().int().min(0).max(50),
});
export type PreviewSettings = z.infer<typeof PreviewSettingsSchema>;

const AttentionModeSchema = z.enum(["all", "urgent", "off"]);
export type AttentionMode = z.infer<typeof AttentionModeSchema>;

export const AttentionSettingsSchema = z.object({
  mode: AttentionModeSchema,
  programStatus: z.boolean(),
  agentProtocol: z.boolean(),
  notify: z.boolean(),
  bell: z.boolean(),
  minTurnSeconds: z.number().int().min(0).max(300),
  doneDelaySeconds: z.number().int().min(0).max(60),
});
export type AttentionSettings = z.infer<typeof AttentionSettingsSchema>;

export const SessionEventSchema = z.discriminatedUnion("type", [
  z.object({
    type: z.literal("snapshot"),
    sessions: z.array(SessionInfoSchema),
    forwards: ForwardsSchema,
  }),
  z.object({ type: z.literal("created"), session: SessionInfoSchema }),
  z.object({ type: z.literal("updated"), session: SessionInfoSchema }),
  z.object({ type: z.literal("removed"), id: SessionInfoSchema.shape.id }),
  z.object({ type: z.literal("forwards"), forwards: ForwardsSchema }),
]);
export const ExitSchema = z.object({ exitCode: z.number().int() });
// Terminal stream frames 0x04–0x06; see docs/protocol/websocket.md.
const OffsetSchema = z.number().int().nonnegative();
export const StreamPositionSchema = z.object({
  generation: GenerationSchema,
  mode: z.enum(["reset", "append"]),
  start: OffsetSchema,
  end: OffsetSchema,
  truncated: z.boolean(),
});
export type StreamPosition = z.infer<typeof StreamPositionSchema>;
export const SubmissionIdSchema = z.string().regex(/^[A-Za-z0-9_-]{8,64}$/);
export const SubmissionStatusSchema = z.object({
  id: SubmissionIdSchema,
  state: z.enum(["received", "written", "partial", "failed", "missing"]),
  error: z.string().optional(),
});
export const ControlStateSchema = z.object({
  controller: z
    .object({
      clientId: z.string(),
      label: z.string(),
      since: z.iso.datetime({ offset: true }),
    })
    .nullable(),
  // A terminal is always at least one cell in each direction.
  cols: z.number().int().min(1).max(1000),
  rows: z.number().int().min(1).max(1000),
  // The program enabled DEC mode 2031 (color scheme change reports).
  colorSchemeReports: z.boolean(),
});
export type ControlState = z.infer<typeof ControlStateSchema>;
// Client-side progress of one submission. "saved" exists only in this
// browser; "unknown" means the acknowledgement was lost and not yet resolved.
const SubmissionProgressSchema = z.enum([
  "saved",
  "sending",
  "received",
  "written",
  "partial",
  "failed",
  "missing",
  "unknown",
]);
export type SubmissionProgress = z.infer<typeof SubmissionProgressSchema>;
export const StoredDraftSchema = z.object({
  v: z.literal(1),
  sessionId: SessionInfoSchema.shape.id,
  generation: GenerationSchema.nullable(),
  text: z.string(),
  // Pending uploaded attachments waiting in the chat composer.
  attachments: z.array(ChatAttachmentSchema).optional(),
  savedAt: z.number().int().nonnegative(),
  submission: z
    .object({
      id: SubmissionIdSchema,
      state: SubmissionProgressSchema,
      at: z.number().int().nonnegative(),
    })
    .nullable(),
});
export type StoredDraft = z.infer<typeof StoredDraftSchema>;
const ScratchEntrySchema = z.object({
  id: z.string().regex(/^[A-Za-z0-9_-]{8,64}$/),
  text: z.string(),
  createdAt: z.number().int().nonnegative(),
  updatedAt: z.number().int().nonnegative(),
});
export type ScratchEntry = z.infer<typeof ScratchEntrySchema>;
export const StoredScratchSchema = z
  .object({
    v: z.literal(1),
    sessionId: SessionInfoSchema.shape.id,
    generation: GenerationSchema.nullable(),
    entries: z.array(ScratchEntrySchema),
    savedAt: z.number().int().nonnegative(),
  })
  .refine(
    (scratch) =>
      new Set(scratch.entries.map((entry) => entry.id)).size ===
      scratch.entries.length,
    "暂存 ID 不能重复",
  );
export type StoredScratch = z.infer<typeof StoredScratchSchema>;
export const HealthSchema = z.object({
  ok: z.literal(true),
  sessions: z.number().int().nonnegative(),
  load: z
    .object({
      one: z.number().nonnegative(),
      five: z.number().nonnegative(),
      fifteen: z.number().nonnegative(),
      cpus: z.number().int().positive(),
    })
    .optional(),
  memory: z
    .object({
      total: z.number().nonnegative(),
      available: z.number().nonnegative(),
    })
    .optional(),
});
export type Health = z.infer<typeof HealthSchema>;
export const WorkspaceCapabilitySchema = z.object({
  gitAvailable: z.boolean(),
  registryAvailable: z.boolean(),
  gitError: z.string(),
  registryError: z.string(),
});

export const SessionIdSchema = SessionInfoSchema.shape.id;
