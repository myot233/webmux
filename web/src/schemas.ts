import { z } from "zod";
import {
  AgentConnectorSchema,
  ChatAttachmentSchema,
  ChatConfigSchema,
  ChatPermissionModeSchema,
  HybridReleaseSchema,
  SessionAgentSchema,
  SessionArchiveSchema,
  SessionStateSchema,
  SessionTransitionSchema,
  SessionViewSchema,
} from "./chat-schemas";

export * from "./chat-schemas";

export const WorkspaceLinkSchema = z.object({
  repositoryId: z.string(),
  worktreeId: z.string(),
  worktreePath: z.string(),
});

export const AttentionSchema = z.object({
  kind: z.enum(["question", "permission", "done", "error", "notify", "bell"]),
  source: z.enum(["agent", "terminal", "cli"]),
  since: z.iso.datetime({ offset: true }),
  message: z.string().optional(),
});
export type Attention = z.infer<typeof AttentionSchema>;

export const ReportedCwdSchema = z.object({
  path: z.string(),
  host: z.string(),
  remote: z.boolean(),
  since: z.iso.datetime({ offset: true }),
});
export type ReportedCwd = z.infer<typeof ReportedCwdSchema>;

// A TCP port a session's process tree listens on (loopback or wildcard).
export const ListeningPortSchema = z.object({
  port: z.number().int().min(1).max(65535),
  process: z.string(),
});
export type ListeningPort = z.infer<typeof ListeningPortSchema>;

export const ForwardOptionsSchema = z.strictObject({
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
// GET /sessions/:id/details: metadata gathered by the server for scripts.
export const SessionDetailsSchema = z.object({
  session: SessionInfoSchema,
  link: z.string().nullable(),
  repository: z
    .object({ id: z.string(), name: z.string(), path: z.string() })
    .nullable(),
  worktree: z
    .object({
      id: z.string(),
      branch: z.string().nullable(),
      path: z.string(),
      main: z.boolean(),
    })
    .nullable(),
  launchConfig: z
    .object({
      id: z.string(),
      name: z.string(),
      icon: z.string().optional(),
      chat: z.boolean(),
    })
    .nullable(),
  launchConfigMissing: z.boolean().optional(),
  artifactsDir: z.string().nullable(),
  forwards: z.array(ForwardSchema),
  parentId: z.string().optional(),
  labels: z.record(z.string(), z.string()),
  children: z.array(z.string()),
});
export const WorkspaceReferenceSchema = z.object({
  repositoryId: z.string(),
  worktreeId: z.string(),
});
// POST /sessions asks for a new worktree first: a new branch from `base`
// (an omitted branch is generated together with the path), or an existing
// branch checked out.
export const NewWorktreeSchema = z.object({
  repositoryId: z.string().min(1),
  mode: z.enum(["new", "existing"]),
  branch: z.string().optional(),
  base: z.string().optional(),
  path: z.string().optional(),
});
export type NewWorktree = z.infer<typeof NewWorktreeSchema>;
// POST /sessions, the only way to create a session. The location is one of
// `cwd`, `workspace` and `newWorktree`; the command `configId` or
// `cmd`/`env`.
export const CreateSessionRequestSchema = z
  .object({
    // Dedupes retries for ten minutes; omitted never dedupes.
    clientRequestId: z.string().min(1).optional(),
    cwd: z.string().optional(),
    workspace: WorkspaceReferenceSchema.optional(),
    newWorktree: NewWorktreeSchema.optional(),
    cmd: z.array(z.string()).optional(),
    env: z.record(z.string(), z.string()).optional(),
    configId: z.string().optional(),
    name: z.string().optional(),
    // Labels set on the new session.
    labels: z.record(z.string(), z.string()).optional(),
    // Only for a launch config with the chat view; the server answers 400
    // otherwise.
    view: SessionViewSchema.optional(),
    // The user's first message, sent once the session is up.
    prompt: z.string().optional(),
    // Puts the repository's initialization instruction before it; needs
    // `workspace` or `newWorktree`.
    useInitInstruction: z.boolean().optional(),
    // Fills `{{dir}}` and `{{webmux}}` in the prompt once the session
    // exists; otherwise the prompt is sent as written.
    expandPrompt: z.boolean().optional(),
    // Files returned by POST /uploads, sent with the prompt (on the
    // terminal view their adopted server paths are appended).
    attachments: z.array(ChatAttachmentSchema).optional(),
    // Chat view only: agent option id to value (booleans "true"/"false")
    // and the permission mode.
    agentConfig: z.record(z.string(), z.string()).optional(),
    permissionMode: ChatPermissionModeSchema.optional(),
    // Initial terminal size; both or neither. The first browser to control
    // the session resizes it to its own window.
    cols: z.number().int().min(1).max(1000).optional(),
    rows: z.number().int().min(1).max(1000).optional(),
  })
  .refine(
    (body) =>
      !body.configId ||
      (!body.cmd?.length && Object.keys(body.env ?? {}).length === 0),
    { message: "configId 不能与 cmd 或 env 同时提供" },
  )
  .refine(
    (body) =>
      !body.useInitInstruction || !!body.workspace || !!body.newWorktree,
    { message: "useInitInstruction 需要 workspace 或 newWorktree" },
  )
  .refine((body) => !body.view || !!body.configId, {
    message: "view 只能与带聊天视图的 configId 一起提供",
  })
  .refine(
    (body) =>
      body.view === "chat" || (!body.agentConfig && !body.permissionMode),
    { message: "agentConfig 与 permissionMode 只用于聊天视图" },
  );
export type CreateSessionRequest = z.infer<typeof CreateSessionRequestSchema>;
export const PreviewSettingsSchema = z.object({
  lines: z.number().int().min(1).max(50),
  skip: z.number().int().min(0).max(50),
});
export type PreviewSettings = z.infer<typeof PreviewSettingsSchema>;

// Whether a command can have a chat view, computed by the server from its
// built-in agent profiles; reason is a stable code.
export const ChatSupportSchema = z.object({
  supported: z.boolean(),
  // Display names of every agent the chat view supports.
  supportedAgents: z.array(z.string()),
  agent: z.string().optional(),
  reason: z
    .enum([
      "unknown_agent",
      "unsupported_form",
      "session_store",
      "directory_arg",
      "non_interactive",
      "session_arg",
    ])
    .optional(),
  connector: AgentConnectorSchema.optional(),
});
export type ChatSupport = z.infer<typeof ChatSupportSchema>;

// Launch configs are server-stored launch commands. Summaries (list) carry only
// env variable names; the full config with values is fetched per id for edits.
// A null preview inherits the browser's global preview setting.
const launchConfigFields = {
  id: z.string().regex(/^[a-z2-7]{8}$/),
  name: z.string().min(1),
  icon: z
    .string()
    .regex(/^[a-z][a-z0-9-]{0,63}$/)
    .optional(),
  cmd: z.array(z.string()).min(1),
  preview: PreviewSettingsSchema.nullable().optional(),
  chat: z.boolean(),
  chatSupport: ChatSupportSchema,
  createdAt: z.iso.datetime({ offset: true }),
  updatedAt: z.iso.datetime({ offset: true }),
};
export const LaunchConfigSummarySchema = z.object({
  ...launchConfigFields,
  envNames: z.array(z.string()),
  // The chat options the config's latest chat side reported, with their
  // last values; [] until one has started.
  chatOptions: z.array(ChatConfigSchema),
});
export type LaunchConfigSummary = z.infer<typeof LaunchConfigSummarySchema>;
export const LaunchConfigSchema = z.object({
  ...launchConfigFields,
  env: z.record(z.string(), z.string()),
});
export type LaunchConfig = z.infer<typeof LaunchConfigSchema>;
export const LaunchConfigsSchema = z
  .array(LaunchConfigSummarySchema)
  .max(100)
  .refine(
    (configs) =>
      new Set(configs.map((config) => config.id)).size === configs.length,
    "启动配置 ID 不能重复",
  );
export const LaunchPresetSchema = z.object({
  name: launchConfigFields.name,
  icon: launchConfigFields.icon,
  cmd: launchConfigFields.cmd,
  envNames: z.array(z.string()),
  chat: z.boolean(),
});
export type LaunchPreset = z.infer<typeof LaunchPresetSchema>;
export const LaunchPresetsSchema = z.array(LaunchPresetSchema);

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
export type SubmissionStatus = z.infer<typeof SubmissionStatusSchema>;
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
export const SubmissionProgressSchema = z.enum([
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
export const ScratchEntrySchema = z.object({
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
export const TunnelInfoSchema = z.object({
  enabled: z.boolean(),
  state: z.enum(["off", "starting", "ready", "failed"]),
  named: z.boolean(),
  hostname: z.string(),
  publicUrl: z.string().optional(),
});
export type TunnelInfo = z.infer<typeof TunnelInfoSchema>;

export const TunnelSettingsSchema = z.object({
  enabled: z.boolean(),
  mode: z.enum(["quick", "named"]),
  hostname: z.string(),
  tokenConfigured: z.boolean(),
  source: z.enum(["web", "config"]),
  state: TunnelInfoSchema.shape.state,
  publicUrl: z.string().optional(),
  error: z.string().optional(),
  canEnable: z.boolean(),
  enableBlockedReason: z.string().optional(),
  storageAvailable: z.boolean(),
  storageError: z.string().optional(),
});
export type TunnelSettings = z.infer<typeof TunnelSettingsSchema>;
export const TunnelSettingsUpdateSchema = z.object({
  mode: z.enum(["quick", "named"]),
  hostname: z.string(),
  token: z.string().optional(),
});
export type TunnelSettingsUpdate = z.infer<typeof TunnelSettingsUpdateSchema>;
export const TailscaleEntrySchema = z.object({
  host: z.string(),
  port: z.number().int().min(0).max(65535),
  public: z.boolean(),
});
export type TailscaleEntry = z.infer<typeof TailscaleEntrySchema>;
export const TailscaleWSLSchema = z.object({
  networkingMode: z.enum(["mirrored", "nat", "unknown"]),
  windowsDetected: z.boolean(),
  windowsRunning: z.boolean(),
  windowsDnsName: z.string().optional(),
});
export const TailscaleStatusSchema = z.object({
  provider: z.enum(["linux", "wsl-native", "windows", "none"]),
  configuredProvider: z.enum(["auto", "linux", "windows"]),
  backendState: z.enum([
    "running",
    "stopped",
    "needs-login",
    "unavailable",
    "unknown",
  ]),
  dnsName: z.string().optional(),
  httpsReady: z.boolean(),
  funnelAvailable: z.enum(["available", "unavailable", "unknown"]),
  mode: z.enum(["off", "serve", "funnel"]),
  port: z.number().int().min(0).max(65535),
  source: z.enum(["config", "web"]),
  storageAvailable: z.boolean(),
  storageError: z.string().optional(),
  canEnable: z.boolean(),
  enableBlockedReason: z.string().optional(),
  state: TunnelInfoSchema.shape.state,
  publicUrl: z.string().optional(),
  blockedReason: z.string().optional(),
  error: z.string().optional(),
  entries: z.array(TailscaleEntrySchema),
  wsl: TailscaleWSLSchema.optional(),
});
export type TailscaleStatus = z.infer<typeof TailscaleStatusSchema>;
export const TailscaleFunnelPorts = [443, 8443, 10000] as const;
export const TailscaleSettingsUpdateSchema = z
  .object({
    mode: TailscaleStatusSchema.shape.mode,
    port: TailscaleStatusSchema.shape.port,
    provider: TailscaleStatusSchema.shape.configuredProvider,
    confirmFunnel: z.boolean().optional(),
  })
  .strict()
  .refine(
    (draft) =>
      draft.mode !== "funnel" ||
      draft.port === 0 ||
      (TailscaleFunnelPorts as readonly number[]).includes(draft.port),
    { message: "Funnel 只支持自动、443、8443 或 10000 端口" },
  );
export type TailscaleSettingsUpdate = z.infer<
  typeof TailscaleSettingsUpdateSchema
>;
// The web-editable [voice] recognizer; env values are credentials for the
// recognition service.
export const VoiceSettingsSchema = z.object({
  cmd: z.array(z.string()),
  env: z.record(z.string(), z.string()),
  enabled: z.boolean(),
  source: z.enum(["config", "web"]),
  storageAvailable: z.boolean(),
  storageError: z.string().optional(),
});
export type VoiceSettings = z.infer<typeof VoiceSettingsSchema>;
export const VoiceSettingsUpdateSchema = z
  .object({
    cmd: VoiceSettingsSchema.shape.cmd,
    env: VoiceSettingsSchema.shape.env,
  })
  .strict();
export type VoiceSettingsUpdate = z.infer<typeof VoiceSettingsUpdateSchema>;
// An installed version that takes effect after a restart. An unknown future
// launcher falls back to the manual restart method.
const PendingUpdateSchema = z.object({
  installedVersion: z.string().min(1).max(64),
  launcher: z
    .enum(["systemd", "launchd", "windows-tray", "manual"])
    .catch("manual"),
});
export type PendingUpdate = z.infer<typeof PendingUpdateSchema>;
export const ConfigSchema = z.object({
  defaultCwd: z.string(),
  // Older servers do not send the editor's default startup shell.
  defaultShell: z.enum(["bash", "zsh", ""]).default(""),
  // Older servers do not send this display-only field.
  hostname: z.string().default(""),
  workspace: WorkspaceCapabilitySchema,
  // Older servers do not send the runtime tunnel state.
  tunnel: TunnelInfoSchema.optional(),
  // Present only while a different version awaits a restart; older servers
  // never send it.
  pendingUpdate: PendingUpdateSchema.optional(),
  // Whether sessions report listening ports on the server's platform.
  portDetection: z.boolean(),
  // Whether external speech recognition is configured on the server.
  voice: z.boolean().default(false),
});

export const VoiceClientMessageSchema = z.object({
  type: z.literal("stop"),
});
export type VoiceClientMessage = z.infer<typeof VoiceClientMessageSchema>;

export const VoiceServerEventSchema = z.discriminatedUnion("type", [
  z.object({ type: z.literal("ready") }),
  z.object({ type: z.literal("partial"), text: z.string().default("") }),
  z.object({ type: z.literal("final"), text: z.string().default("") }),
  z.object({ type: z.literal("error"), error: z.string() }),
]);
export type VoiceServerEvent = z.infer<typeof VoiceServerEventSchema>;
export const WorktreeSchema = z.object({
  id: z.string(),
  repositoryId: z.string(),
  path: z.string(),
  kind: z.enum(["main", "linked", "bare"]),
  branch: z.string().nullable(),
  head: z.string().nullable(),
  detached: z.boolean(),
  locked: z.boolean(),
  lockReason: z.string(),
  prunable: z.boolean(),
  available: z.boolean(),
  dirty: z.boolean().nullable(),
  hasUntracked: z.boolean(),
  hasIgnored: z.boolean(),
  error: z.string(),
});
export type Worktree = z.infer<typeof WorktreeSchema>;
// The longest repository init instruction the server accepts, in characters.
export const INIT_INSTRUCTION_MAX = 4000;
export const RepositorySchema = z.object({
  id: z.string(),
  name: z.string(),
  commonDir: z.string(),
  anchorPath: z.string(),
  availability: z.enum(["available", "unavailable"]),
  error: z.string(),
  checkedAt: z.iso.datetime({ offset: true }),
  // Quick start puts it before the first message; "" when unset.
  initInstruction: z.string(),
  worktrees: z.array(WorktreeSchema),
});
export type Repository = z.infer<typeof RepositorySchema>;
export const RepositoriesSchema = z.array(RepositorySchema);
export const RegisterRepositoryRequestSchema = z.object({
  path: z.string().min(1),
});
export const UpdateRepositoryRequestSchema = z.object({
  initInstruction: z.string().max(INIT_INSTRUCTION_MAX),
});
// The agent options a launch config's agent offers before any chat of it,
// read through a throwaway session in the repository
// (POST /launch-configs/:id/options).
export const LaunchOptionsRequestSchema = z.object({
  repositoryId: z.string().min(1),
});
export const LaunchOptionsSchema = z.array(ChatConfigSchema);
// The 201 body of POST /sessions; `worktree` only when the request created
// one, `configWarnings` the agent options the agent did not take.
export const CreateSessionResultSchema = z.object({
  session: SessionInfoSchema,
  worktree: WorktreeSchema.optional(),
  configWarnings: z.array(z.string()),
});
export type CreateSessionResult = z.infer<typeof CreateSessionResultSchema>;
// The body of a failed POST /sessions: the step that failed and what earlier
// steps left behind, which is never removed automatically.
export const CreateSessionFailureSchema = z.object({
  error: z.string(),
  step: z.enum(["precheck", "worktree", "session", "config", "prompt"]),
  reason: z.string().optional(),
  eligibility: z.string().optional(),
  connector: AgentConnectorSchema.optional(),
  worktree: WorktreeSchema.optional(),
  sessionId: z.string().optional(),
});
export type CreateSessionFailure = z.infer<typeof CreateSessionFailureSchema>;

// Server-synchronized choice of worktrees shown on the home page, keyed by
// repository ID. A repository without an entry was never curated.
export const HomeWorktreesSchema = z.record(
  z.string().min(1),
  z.array(z.string().min(1)).max(1000),
);
export type HomeWorktrees = z.infer<typeof HomeWorktreesSchema>;
export const BranchesSchema = z.object({
  branches: z.array(
    z.object({
      name: z.string(),
      head: z.string(),
      worktreePaths: z.array(z.string()),
    }),
  ),
  bases: z.array(
    z.object({ name: z.string(), head: z.string(), subject: z.string() }),
  ),
  defaultBase: z.string().nullable(),
  defaultBaseSubject: z.string(),
  defaultBranch: z.string().nullable(),
  defaultParent: z.string(),
});
export const CommitsSchema = z.object({
  commits: z.array(
    z.object({
      hash: z.string(),
      subject: z.string(),
      author: z.string(),
      committedAt: z.string(),
    }),
  ),
});
export const RefreshBranchBaseRequestSchema = z.object({
  branch: z.string().min(1),
});
export const RefreshBranchBaseResponseSchema = z.object({
  branch: z.string(),
  base: z.string(),
  baseSubject: z.string(),
  baseSource: z.enum(["local", "remote"]),
  upstream: z.string(),
  upstreamHead: z.string(),
  fetchWarning: z.string(),
});
export const CreateWorktreeRequestSchema = z.object({
  path: z.string(),
  mode: z.enum(["new", "existing"]),
  branch: z.string(),
  base: z.string().optional(),
});
// Whether a commit is contained in a remote-tracking ref of its repository,
// read from local refs only. `unpushed`: remotes exist and none contains it;
// `noRemote`: none is configured; `unknown`: no commit or the check failed.
// `branches` name at most a few containing branches ("origin/main"), `more`
// counts the rest.
export const PushCheckSchema = z.object({
  state: z.enum(["pushed", "unpushed", "noRemote", "unknown"]),
  branches: z.array(z.string()),
  more: z.number().int().nonnegative(),
});
export type PushCheck = z.infer<typeof PushCheckSchema>;

export const RemovalPreviewItemSchema = z.object({
  worktree: WorktreeSchema,
  sessions: z.array(SessionInfoSchema),
  // Closed and interrupted sessions in the worktree; a successful removal
  // keeps them, marked released at `commit`, so they can be rebuilt and
  // resumed. With an empty `commit` (HEAD does not resolve) they stay
  // unchanged and cannot be rebuilt.
  archived: z.array(SessionInfoSchema),
  // Full OID the removal keeps; "" when HEAD does not resolve.
  commit: z.string(),
  push: PushCheckSchema,
  blockers: z.array(z.string()),
  warnings: z.array(z.string()),
  forceAllowed: z.boolean(),
});
export type RemovalPreviewItem = z.infer<typeof RemovalPreviewItemSchema>;

export const RemovalPreviewSchema = RemovalPreviewItemSchema.extend({
  confirmationId: z.string(),
  expiresAt: z.iso.datetime({ offset: true }),
});
export type RemovalPreview = z.infer<typeof RemovalPreviewSchema>;
export const EmptyRequestSchema = z.object({}).strict();
export const RemoveWorktreeRequestSchema = z.object({
  confirmationId: z.string(),
  mode: z.enum(["safe", "force"]),
  confirmPath: z.string().optional(),
  closeSessions: z.boolean().optional(),
});
export const MAX_BATCH_WORKTREES = 50;
export const BatchRemovalPreviewRequestSchema = z
  .object({
    worktreeIds: z.array(z.string().min(1)).min(1).max(MAX_BATCH_WORKTREES),
  })
  .strict();
export type BatchRemovalPreviewRequest = z.infer<
  typeof BatchRemovalPreviewRequestSchema
>;
export const BatchRemovalPreviewSchema = z.object({
  confirmationId: z.string(),
  expiresAt: z.iso.datetime({ offset: true }),
  items: z.array(RemovalPreviewItemSchema),
});
export type BatchRemovalPreview = z.infer<typeof BatchRemovalPreviewSchema>;
export const BatchRemoveWorktreesRequestSchema = z
  .object({
    confirmationId: z.string().min(1),
    includeForce: z.boolean(),
    closeSessions: z.boolean(),
  })
  .strict();
export type BatchRemoveWorktreesRequest = z.infer<
  typeof BatchRemoveWorktreesRequestSchema
>;
export const BatchRemoveItemResultSchema = z.object({
  worktreeId: z.string(),
  path: z.string(),
  branch: z.string().optional(),
  status: z.enum(["removed", "skipped", "failed"]),
  error: z.string().optional(),
});
export type BatchRemoveItemResult = z.infer<typeof BatchRemoveItemResultSchema>;
export const BatchRemoveResultSchema = z.object({
  results: z.array(BatchRemoveItemResultSchema),
});
export type BatchRemoveResult = z.infer<typeof BatchRemoveResultSchema>;
// Releasing a linked worktree's disk space keeps its archived records and
// the commit it was on. Ignored entries are top-level paths relative to the
// worktree, directories ending in "/"; at most 50 are listed and
// `ignoredMore` counts the rest. `sizeComplete` is false when sizing ran out
// of time or failed, so the byte counts are lower bounds.
export const ReleasePreviewSchema = z.object({
  confirmationId: z.string(),
  expiresAt: z.iso.datetime({ offset: true }),
  worktree: WorktreeSchema,
  // Full OID; "" when HEAD does not resolve, which is also a blocker.
  commit: z.string(),
  push: PushCheckSchema,
  // The records that stay archived and are marked released.
  archived: z.array(SessionInfoSchema),
  ignored: z.array(
    z.object({
      path: z.string(),
      bytes: z.number().int().nonnegative().optional(),
    }),
  ),
  ignoredMore: z.number().int().nonnegative(),
  ignoredBytes: z.number().int().nonnegative().optional(),
  sizeComplete: z.boolean(),
  blockers: z.array(z.string()),
});
export type ReleasePreview = z.infer<typeof ReleasePreviewSchema>;
// The disk space of a worktree's directory. `complete` is false when the
// count ran out of budget or skipped an unreadable directory, so `bytes` and
// `entries` are lower bounds.
export const WorktreeUsageSchema = z.object({
  bytes: z.number().int().nonnegative(),
  entries: z.number().int().nonnegative(),
  complete: z.boolean(),
});
export type WorktreeUsage = z.infer<typeof WorktreeUsageSchema>;
export const ReleaseWorktreeRequestSchema = z.object({
  confirmationId: z.string(),
});
export const ReleaseResultSchema = z.object({
  releaseId: z.string(),
  recordIds: z.array(z.string()),
});
// How a released record's worktree comes back at its original path: the
// worktree already there, its branch, or the saved commit detached. A
// detached rebuild of a record that had a branch says why.
export const RebuildModeSchema = z.enum(["reuse", "branch", "detached"]);
export const RebuildPreviewSchema = z.object({
  releaseId: z.string(),
  path: z.string(),
  commit: z.string(),
  branch: z.string().optional(),
  mode: RebuildModeSchema,
  reason: z
    .enum(["branchMissing", "branchMoved", "branchCheckedOut"])
    .optional(),
  blockers: z.array(z.string()),
});
export type RebuildPreview = z.infer<typeof RebuildPreviewSchema>;
export const RebuildRequestSchema = z.object({
  releaseId: z.string(),
  mode: RebuildModeSchema,
  commit: z.string(),
});

export const DirectoriesSchema = z.array(z.string());
// `reason` accompanies push 409s; other errors omit it.
export const ErrorSchema = z.object({
  error: z.string(),
  reason: z.string().optional(),
  notPersisted: z.boolean().optional(),
});
export const UploadSchema = z.object({ path: z.string().startsWith("/") });
export const ArtifactsFolderSchema = z.object({
  path: z.string().startsWith("/"),
});
// POST /sessions/:id/agent-guide: the brief {{webmux}} expands to.
export const AgentGuidePromptSchema = z.object({
  prompt: z.string().min(1),
  guideUrl: z.string(),
  dir: z.string().min(1),
  bin: z.string().nullable(),
  socket: z.string().nullable(),
});
export const RecentDirsSchema = z
  .array(z.string())
  .transform((dirs) => dirs.slice(0, 10));
export const FontSizeSchema = z.coerce.number().int().min(8).max(20);
export const CodeFontSizeSchema = z.coerce.number().int().min(10).max(24);
export const ThemeSchema = z.enum(["light", "dark"]);
export type Theme = z.infer<typeof ThemeSchema>;
export const TextSettingSchema = z.string();
export const QuickCommandSchema = z.object({
  id: z.string().min(1).max(64),
  name: z.string().max(40),
  command: z
    .string()
    .max(4096)
    .refine((value) => value.trim() !== "", "指令不能为空"),
  enter: z.boolean(),
});
export type QuickCommand = z.infer<typeof QuickCommandSchema>;
export const QuickCommandsSchema = z
  .array(QuickCommandSchema)
  .max(50)
  .refine(
    (commands) =>
      new Set(commands.map((cmd) => cmd.id)).size === commands.length,
    "快捷指令 ID 不能重复",
  );
export const NoContentSchema = z.undefined();
export const PushReasonSchema = z.enum([
  "disabled",
  "storage",
  "quick_tunnel",
  "unsupported_origin",
]);
export type PushReason = z.infer<typeof PushReasonSchema>;
export const PushStatusSchema = z.object({
  enabled: z.boolean(),
  available: z.boolean(),
  reason: PushReasonSchema.optional(),
  // Base64url uncompressed P-256 VAPID key, present only when available.
  publicKey: z.string().optional(),
  subscriptions: z.number().int().min(0),
});
export type PushStatus = z.infer<typeof PushStatusSchema>;
export const PushSubscriptionResultSchema = z.object({ id: z.string() });
export const PushSubscriptionRequestSchema = z.object({
  endpoint: z.string(),
  expirationTime: z.number().nullable().optional(),
  keys: z.object({ p256dh: z.string(), auth: z.string() }),
});

// A test notification goes to every device; failures name only the push
// service host, and removed lists subscriptions the service reported expired.
export const PushTestResultSchema = z.object({
  devices: z.number().int().min(0),
  accepted: z.number().int().min(0),
  removed: z.array(z.string()),
  errors: z.array(z.string()),
});
export type PushTestResult = z.infer<typeof PushTestResultSchema>;

const CodeSourceSchema = z.enum(["worktree", "index", "head", "empty"]);
export const CodeContextSchema = z.object({
  contextId: z.string(),
  rootPath: z.string(),
  rootKind: z.enum(["directory", "worktree", "artifacts"]),
  browseAvailable: z.boolean(),
  diffAvailable: z.boolean(),
  ignoreAvailable: z.boolean(),
  reason: z.string(),
  writableAvailable: z.boolean(),
});
export const CodeNodeSchema = z.object({
  nodeId: z.string(),
  path: z.string(),
  name: z.string(),
  kind: z.enum(["file", "directory", "symlink", "boundary", "special"]),
  gitStatus: z.string(),
  ignored: z.boolean(),
  referenceable: z.boolean(),
  mediaType: z.string(),
  linkTarget: z.string().optional(),
});
export const CodeListingSchema = z.object({
  children: z.array(CodeNodeSchema),
  nextCursor: z.string(),
  limited: z.boolean(),
  complete: z.boolean(),
  contentVersion: z.string(),
  checkedAt: z.string(),
});
export type CodeNode = z.infer<typeof CodeNodeSchema>;
// The nodes a file operation created, renamed or moved, under final names.
export const CodeMutationSchema = z.object({
  nodes: z.array(CodeNodeSchema),
});
export const CodeUsageSchema = z.object({
  entries: z.number().int().nonnegative(),
  bytes: z.number().int().nonnegative(),
  limited: z.boolean(),
});
// File operation bodies are validated before any request is sent.
export const CodeNameSchema = z
  .string()
  .min(1)
  .refine((name) => new TextEncoder().encode(name).length <= 255, "名称过长")
  .refine((name) => name !== "." && name !== "..", "名称无效")
  .refine((name) => name.toLowerCase() !== ".git", ".git 为保留名称")
  .refine((name) => !/[/\\]/.test(name), "名称不能包含 / 或 \\")
  // \p{Cc} is the control set the server's unicode.IsControl refuses.
  .refine((name) => !/\p{Cc}/u.test(name), "名称不能包含控制字符");
export const CodeRenameRequestSchema = z.strictObject({
  contextId: z.string().min(1),
  nodeId: z.string().min(1),
  name: CodeNameSchema,
});
export const CodeMkdirRequestSchema = z.strictObject({
  contextId: z.string().min(1),
  parentNodeId: z.string(),
  name: CodeNameSchema,
});
export const CodeCopyRequestSchema = z.strictObject({
  contextId: z.string().min(1),
  nodeIds: z.array(z.string().min(1)).min(1).max(100),
  targetNodeId: z.string(),
  mode: z.enum(["copy", "move"]),
});
export const CodeLocationSchema = z.object({
  nodeId: z.string(),
  path: z.string(),
  kind: z.enum(["file", "directory"]),
});
export const CodeDocumentSchema = z.object({
  nodeId: z.string(),
  path: z.string(),
  absolutePath: z.string(),
  source: CodeSourceSchema,
  oid: z.string(),
  contentVersion: z.string(),
  kind: z.enum([
    "text",
    "binary",
    "unsupported",
    "too-large",
    "symlink",
    "image",
    "pdf",
  ]),
  text: z.string(),
  lineCount: z.number().int().nonnegative().max(20000),
  referenceable: z.boolean(),
  reason: z.string(),
  mediaType: z.string(),
  size: z.number().int().nonnegative(),
  downloadable: z.boolean(),
});
export const CodeFileSchema = z.object({
  snapshotId: z.string(),
  document: CodeDocumentSchema,
  checkedAt: z.string(),
});
export const CodeChangeSchema = z.object({
  nodeId: z.string(),
  changeId: z.string(),
  group: z.enum(["unstaged", "staged", "untracked"]),
  oldPath: z.string(),
  newPath: z.string(),
  status: z.string(),
  oldOid: z.string(),
  newOid: z.string(),
  oldMode: z.string(),
  newMode: z.string(),
});
export const CodeChangesSchema = z.object({
  nextCursor: z.string(),
  total: z.number().int().nonnegative(),
  entries: z.array(CodeChangeSchema).max(200),
  contentVersion: z.string(),
  checkedAt: z.string(),
});
export const CodeDiffSchema = z.object({
  snapshotId: z.string(),
  old: CodeDocumentSchema,
  new: CodeDocumentSchema,
  kind: z.enum(["text", "metadata", "image"]),
  reason: z.string(),
  checkedAt: z.string(),
  change: CodeChangeSchema,
});
const CodeStateSchema = z.enum([
  "updated",
  "unchanged",
  "missing",
  "expired",
  "unknown",
]);
export const CodeCheckSchema = z.object({
  snapshots: z.record(z.string(), CodeStateSchema),
  directories: z.record(z.string(), CodeStateSchema),
  changes: CodeStateSchema,
  checkedAt: z.string(),
});
export type CodeDocument = z.infer<typeof CodeDocumentSchema>;
export type CodeFile = z.infer<typeof CodeFileSchema>;
export type CodeDiff = z.infer<typeof CodeDiffSchema>;
export type CodeChange = z.infer<typeof CodeChangeSchema>;
export type CodeListing = z.infer<typeof CodeListingSchema>;

// Reading state of one code workspace, handed between the opener and the
// pop-out window. Positions anchor on content (lines, pages, headings), so a
// window of another size shows the same place.
const CodeSearchSpecSchema = z.object({
  search: z.string(),
  caseSensitive: z.boolean(),
  regexp: z.boolean(),
  wholeWord: z.boolean(),
});
const CodeLineSideSchema = z.enum(["deletions", "additions"]);
// Search panel and match cursor shared by the file and diff readers.
const CodeFindStateSchema = z.object({
  search: CodeSearchSpecSchema,
  panel: z.enum(["search", "goto", "zoom"]).optional(),
  cursor: z
    .object({
      matches: z.array(z.number().int()),
      index: z.number().int(),
    })
    .optional(),
});
export const CodeReaderStateSchema = CodeFindStateSchema.extend({
  // First visible line and how far (0-1 of its height) it is scrolled past.
  line: z.number().int().min(1),
  offset: z.number().min(0).max(1),
  // The selected line range, or null when nothing is selected.
  selection: z
    .object({
      start: z.number().int().min(1),
      end: z.number().int().min(1),
    })
    .nullable(),
});
export const CodeDiffReaderStateSchema = CodeFindStateSchema.extend({
  selected: z
    .object({
      start: z.number().int(),
      end: z.number().int(),
      side: CodeLineSideSchema.optional(),
      endSide: CodeLineSideSchema.optional(),
    })
    .nullable(),
  // Unmodified lines revealed per hunk separator, as the library counts them.
  expanded: z.array(
    z.object({
      index: z.number().int().nonnegative(),
      fromStart: z.number().int().nonnegative(),
      fromEnd: z.number().int().nonnegative(),
    }),
  ),
  top: z
    .object({ line: z.number().int().min(1), side: CodeLineSideSchema })
    .optional(),
});
export const CodeMarkdownStateSchema = z.object({
  // Slug of the last heading at or above the top edge, "" when none.
  heading: z.string(),
  // Pixels past that heading, or the scroll ratio (0-1) without a heading.
  offset: z.number(),
});
export const CodeImageStateSchema = z.object({
  fitted: z.boolean(),
  scale: z.number().positive(),
  // Image point (0-1 of its natural size) at the viewport centre.
  centerX: z.number(),
  centerY: z.number(),
});
export const CodePdfStateSchema = z.object({
  fitted: z.boolean(),
  zoom: z.number().positive(),
  page: z.number().int().min(1),
  // How far (0-1 of the page height) the top edge is into `page`.
  offset: z.number().min(0).max(1),
});
export const CodeJsonStateSchema = z.object({
  opened: z.array(z.tuple([z.string(), z.boolean()])),
  shown: z.array(z.tuple([z.string(), z.number().int().nonnegative()])),
});
export const CodeDocumentViewStateSchema = z.object({
  mode: z.enum(["file", "diff"]),
  side: z.enum(["old", "new"]),
  start: z.number().int().optional(),
  end: z.number().int().optional(),
  svgView: z.enum(["preview", "source"]),
  formatView: z.enum(["preview", "source"]),
  reader: CodeReaderStateSchema.optional(),
  diff: CodeDiffReaderStateSchema.optional(),
  markdown: CodeMarkdownStateSchema.optional(),
  image: CodeImageStateSchema.optional(),
  pdf: CodePdfStateSchema.optional(),
  json: CodeJsonStateSchema.optional(),
});
const CodeFileTargetSchema = CodeNodeSchema.pick({
  nodeId: true,
  path: true,
  name: true,
  kind: true,
});
export const CodeNavigatorStateSchema = z.object({
  rootOpen: z.boolean(),
  // Expanded folders and the listings they had loaded, by path.
  expanded: z.array(z.string()),
  branches: z.array(z.tuple([z.string(), CodeListingSchema])),
  // Index of the first visible row of the changes list.
  changesTop: z.number().int().nonnegative(),
  clipboard: z
    .object({
      contextId: z.string(),
      nodes: z.array(CodeFileTargetSchema),
      mode: z.enum(["copy", "move"]),
    })
    .optional(),
});
export const CodeWorkspaceSnapshotSchema = z.object({
  version: z.literal(1),
  root: z.enum(["workspace", "artifacts"]),
  context: CodeContextSchema.optional(),
  listing: CodeListingSchema.optional(),
  rootListing: CodeListingSchema.optional(),
  rootIncludeIgnored: z.boolean(),
  directoryVersions: z.record(z.string(), z.string()),
  navIdentity: z.string(),
  navStatus: z.string(),
  changes: CodeChangesSchema.optional(),
  changeQuery: z.string(),
  changeSearch: z.string(),
  changeGroup: z.string(),
  includeIgnored: z.boolean(),
  directory: z.string(),
  query: z.string(),
  nav: z.string(),
  navOpen: z.boolean(),
  navWidth: z.number(),
  active: z.string(),
  tabs: z
    .array(
      z.object({
        key: z.string(),
        contextId: z.string(),
        rootKind: CodeContextSchema.shape.rootKind,
        file: CodeFileSchema.optional(),
        diff: CodeDiffSchema.optional(),
        comparison: CodeDiffSchema.optional(),
        // Between windows, a body the receiver already holds is sent as its
        // snapshot id instead.
        fileRef: z.string().optional(),
        diffRef: z.string().optional(),
        comparisonRef: z.string().optional(),
        status: z.string(),
        view: CodeDocumentViewStateSchema.optional(),
      }),
    )
    .max(20),
  navigator: CodeNavigatorStateSchema.optional(),
});
export type CodeReaderState = z.infer<typeof CodeReaderStateSchema>;
export type CodeDiffReaderState = z.infer<typeof CodeDiffReaderStateSchema>;
export type CodeMarkdownState = z.infer<typeof CodeMarkdownStateSchema>;
export type CodeImageState = z.infer<typeof CodeImageStateSchema>;
export type CodePdfState = z.infer<typeof CodePdfStateSchema>;
export type CodeJsonState = z.infer<typeof CodeJsonStateSchema>;
export type CodeDocumentViewState = z.infer<typeof CodeDocumentViewStateSchema>;
export type CodeNavigatorState = z.infer<typeof CodeNavigatorStateSchema>;
export type CodeWorkspaceSnapshot = z.infer<typeof CodeWorkspaceSnapshotSchema>;

// What the code workspace needs from the terminal page that hosts it.
export const CodeHostStateSchema = z.object({
  draftEnabled: z.boolean(),
  connection: z.string(),
  sessionCwd: z.string(),
  reportedCwd: ReportedCwdSchema.nullable(),
});
export type CodeHostState = z.infer<typeof CodeHostStateSchema>;
const CodeInsertionSchema = z.enum(["draft", "terminal", "failed"]);
// The first message a pop-out frame accepts; it carries the channel port.
export const CodePopoutHandshakeSchema = z.object({
  type: z.literal("webmux-code-popout"),
  version: z.literal(1),
});
export const codePopoutHandshake: z.infer<typeof CodePopoutHandshakeSchema> = {
  type: "webmux-code-popout",
  version: 1,
};
// A code location another view asks the code workspace to open.
export const CodeRevealSchema = z.object({
  path: z.string(),
  key: z.number(),
  mode: z.enum(["diff", "file"]).optional(),
  rawPath: z.string().optional(),
  fromUri: z.boolean().optional(),
  line: z.number().int().positive().optional(),
  endLine: z.number().int().positive().optional(),
  column: z.number().int().positive().optional(),
  heading: z.string().optional(),
});
export type CodeReveal = z.infer<typeof CodeRevealSchema>;
// Opener to pop-out frame, over the handshake's port.
export const CodePopoutInboundSchema = z.discriminatedUnion("type", [
  z.object({
    type: z.literal("init"),
    snapshot: CodeWorkspaceSnapshotSchema.nullable(),
    host: CodeHostStateSchema,
  }),
  z.object({ type: z.literal("host"), host: CodeHostStateSchema }),
  z.object({
    type: z.literal("reveal"),
    reveal: CodeRevealSchema,
  }),
  z.object({
    type: z.literal("pasteResult"),
    requestId: z.number().int(),
    result: CodeInsertionSchema,
  }),
  z.object({ type: z.literal("collect"), requestId: z.number().int() }),
]);
// Pop-out frame to opener.
export const CodePopoutOutboundSchema = z.discriminatedUnion("type", [
  z.object({ type: z.literal("ready") }),
  z.object({
    type: z.literal("paste"),
    requestId: z.number().int(),
    text: z.string(),
  }),
  z.object({
    type: z.literal("snapshot"),
    requestId: z.number().int().optional(),
    snapshot: CodeWorkspaceSnapshotSchema,
  }),
  z.object({ type: z.literal("close") }),
]);
export type CodePopoutInbound = z.infer<typeof CodePopoutInboundSchema>;
export type CodePopoutOutbound = z.infer<typeof CodePopoutOutboundSchema>;

export const SessionPinsSchema = z
  .array(SessionInfoSchema.shape.id)
  .max(50)
  .refine((ids) => new Set(ids).size === ids.length);

// User preferences shared by every authenticated client of this server.
export const SettingsValuesSchema = z.strictObject({
  theme: ThemeSchema.nullable(),
  preview: PreviewSettingsSchema.nullable(),
  codeReader: z
    .strictObject({
      lineNumbers: z.enum(["show", "hide"]),
      diffOverflow: z.enum(["scroll", "wrap"]),
    })
    .nullable(),
  quickCommands: QuickCommandsSchema.nullable(),
  homeWorktrees: HomeWorktreesSchema.refine((value) => {
    const entries = Object.entries(value);
    return (
      entries.length <= 200 &&
      entries.reduce((n, [, ids]) => n + ids.length, 0) <= 2000 &&
      entries.every(
        ([id, ids]) =>
          id.length <= 128 &&
          ids.every((x) => x.length <= 128) &&
          new Set(ids).size === ids.length,
      )
    );
  }).nullable(),
  sessionPins: SessionPinsSchema.nullable().optional(),
  forceSoftKeys: z.boolean().nullable(),
  ignoreHighPorts: z.boolean().nullable(),
});
export type SettingsValues = z.infer<typeof SettingsValuesSchema>;
export const SettingsSnapshotSchema = z.strictObject({
  revision: z.number().int().min(0).max(Number.MAX_SAFE_INTEGER),
  updatedAt: z.union([z.literal(""), z.iso.datetime({ offset: true })]),
  values: SettingsValuesSchema,
});
export type SettingsSnapshot = z.infer<typeof SettingsSnapshotSchema>;
export const SettingsChangesSchema = SettingsValuesSchema.partial();
export const SettingsEventSchema = z.discriminatedUnion("type", [
  z.strictObject({
    type: z.literal("settings"),
    settings: SettingsSnapshotSchema,
  }),
  z.strictObject({
    type: z.literal("settings.update"),
    revision: z.number().int().positive().max(Number.MAX_SAFE_INTEGER),
    updatedAt: z.iso.datetime({ offset: true }),
    changes: SettingsChangesSchema,
  }),
]);
