import { z } from "zod";
import { AgentConnectorSchema, ChatConfigSchema } from "./chat-schemas";
import {
  AttentionSettingsSchema,
  PreviewSettingsSchema,
  SessionInfoSchema,
} from "./session-schemas";
import {
  CommitSchema,
  FetchResultSchema,
  RepositorySchema,
  WorktreeSchema,
} from "./workspace-schemas";

// One blocker or warning of a preview; `path`, when set, is the worktree a
// blocker refers to, such as the one that has the target branch checked out.
const IssueSchema = z.object({
  code: z.string(),
  message: z.string(),
  path: z.string().optional(),
});
export type Issue = z.infer<typeof IssueSchema>;

// What a worktree switches to: "local" takes `branch`; "remote" takes `ref`
// ("origin/fix/b") and creates the local branch `name` (default: `ref`
// without its remote) tracking it; "new" takes `name` and `startPoint` and
// sets no upstream; "detach" takes `commit`.
const SwitchTargetSchema = z.discriminatedUnion("kind", [
  z.object({ kind: z.literal("local"), branch: z.string().min(1) }).strict(),
  z
    .object({
      kind: z.literal("remote"),
      ref: z.string().min(1),
      name: z.string().min(1).optional(),
    })
    .strict(),
  z
    .object({
      kind: z.literal("new"),
      name: z.string().min(1),
      startPoint: z.string().min(1),
    })
    .strict(),
  z.object({ kind: z.literal("detach"), commit: z.string().min(1) }).strict(),
]);
export type SwitchTarget = z.infer<typeof SwitchTargetSchema>;

export const SwitchPreviewRequestSchema = z
  .object({ target: SwitchTargetSchema })
  .strict();

// A preview with blockers carries no confirmation: `confirmationId` is ""
// and `expiresAt` null. `sessions` are the running sessions in the
// worktree; a switch never closes them.
export const SwitchPreviewSchema = z.object({
  confirmationId: z.string(),
  expiresAt: z.iso.datetime({ offset: true }).nullable(),
  worktree: WorktreeSchema,
  target: z.object({
    kind: z.enum(["local", "remote", "new", "detach"]),
    // The branch the worktree ends on; null for a detached target.
    branch: z.string().nullable(),
    commit: z.string(),
    subject: z.string(),
    createsBranch: z.boolean(),
    // The remote-tracking branch a remote target tracks.
    upstream: z.string().nullable(),
  }),
  sessions: z.array(SessionInfoSchema),
  blockers: z.array(IssueSchema),
  warnings: z.array(IssueSchema),
});
export type SwitchPreview = z.infer<typeof SwitchPreviewSchema>;

// Confirms a previewed switch; `acknowledgeSessions` is required when the
// preview listed sessions. The answer is the refreshed worktree.
export const SwitchRequestSchema = z
  .object({
    confirmationId: z.string().min(1),
    acknowledgeSessions: z.boolean(),
  })
  .strict();
export type SwitchRequest = z.infer<typeof SwitchRequestSchema>;

// ---- Branch sync actions (add-branch-sync-actions) ----
// Fast-forward, push and delete of local branches: a preview, then a
// confirmed execution answering with the repository read again. A preview
// with blockers carries no confirmation (`confirmationId` "", `expiresAt`
// null); an execution whose previewed state changed answers 409.
// Blockers and warnings are the switch's IssueSchema.
// The first commits of a range (at most 20, newest first); `total` counts
// the whole range.
const CommitListSchema = z.object({
  total: z.number().int().nonnegative(),
  items: z.array(CommitSchema),
});
export type CommitList = z.infer<typeof CommitListSchema>;

const PreviewConfirmation = {
  confirmationId: z.string(),
  expiresAt: z.iso.datetime({ offset: true }).nullable(),
};

export const MAX_BRANCH_DELETE = 50;

export const FastForwardPreviewRequestSchema = z
  .object({ branch: z.string().min(1) })
  .strict();

// Moves `branch` from `from` to its upstream's tip `to`; `worktree` has it
// checked out (null for none) and `sessions` are that worktree's running
// sessions, which keep running.
export const FastForwardPreviewSchema = z.object({
  ...PreviewConfirmation,
  branch: z.string(),
  from: z.string(),
  to: z.string(),
  upstream: z.string(),
  commits: CommitListSchema,
  worktree: WorktreeSchema.nullable(),
  sessions: z.array(SessionInfoSchema),
  blockers: z.array(IssueSchema),
  warnings: z.array(IssueSchema),
});
export type FastForwardPreview = z.infer<typeof FastForwardPreviewSchema>;

export const FastForwardRequestSchema = z
  .object({
    confirmationId: z.string().min(1),
    acknowledgeSessions: z.boolean(),
  })
  .strict();

export const FastForwardResultSchema = z.object({
  repository: RepositorySchema,
});
export type FastForwardResult = z.infer<typeof FastForwardResultSchema>;

// `remote` and `target` only apply to publishing a branch without an
// existing upstream.
export const PushPreviewRequestSchema = z
  .object({
    branch: z.string().min(1),
    remote: z.string().min(1).optional(),
    target: z.string().min(1).optional(),
  })
  .strict();
export type PushPreviewRequest = z.infer<typeof PushPreviewRequestSchema>;

// Pushes `commit` to branch `target` of `remote`, whose push URL is
// redacted; `publish` marks a branch without an upstream, which gets the
// target as its upstream.
export const PushPreviewSchema = z.object({
  ...PreviewConfirmation,
  branch: z.string(),
  commit: z.string(),
  remote: z.string(),
  pushUrl: z.string(),
  target: z.string(),
  publish: z.boolean(),
  commits: CommitListSchema,
  blockers: z.array(IssueSchema),
  warnings: z.array(IssueSchema),
});
export type PushPreview = z.infer<typeof PushPreviewSchema>;

export const PushRequestSchema = z
  .object({ confirmationId: z.string().min(1) })
  .strict();

// Also a refused push answers 200; `message` explains it, redacted. The
// statuses are those of a fetch.
export const PushResultSchema = z.object({
  status: FetchResultSchema.shape.status,
  message: z.string(),
  repository: RepositorySchema,
});
export type PushResult = z.infer<typeof PushResultSchema>;

export const BranchDeletePreviewRequestSchema = z
  .object({
    branches: z.array(z.string().min(1)).min(1).max(MAX_BRANCH_DELETE),
  })
  .strict();

// "contained": other refs keep all its commits; "unmerged": `unmerged`
// lists the commits only it keeps. A blocked item cannot be deleted.
const BranchDeleteItemSchema = z.object({
  branch: z.string(),
  commit: z.string(),
  group: z.enum(["contained", "unmerged"]),
  unmerged: CommitListSchema,
  blockers: z.array(IssueSchema),
});
export type BranchDeleteItem = z.infer<typeof BranchDeleteItemSchema>;

export const BranchDeletePreviewSchema = z.object({
  ...PreviewConfirmation,
  items: z.array(BranchDeleteItemSchema),
});
export type BranchDeletePreview = z.infer<typeof BranchDeletePreviewSchema>;

// The unmerged group is deleted only with `acknowledgeUnmerged`.
export const BranchDeleteRequestSchema = z
  .object({
    confirmationId: z.string().min(1),
    acknowledgeUnmerged: z.boolean(),
  })
  .strict();

const BranchDeleteItemResultSchema = z.object({
  branch: z.string(),
  status: z.enum(["deleted", "skipped", "failed"]),
  commit: z.string(),
  message: z.string(),
});

export const BranchDeleteResultSchema = z.object({
  results: z.array(BranchDeleteItemResultSchema),
  repository: RepositorySchema,
});
export type BranchDeleteResult = z.infer<typeof BranchDeleteResultSchema>;

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
  attention: AttentionSettingsSchema.nullable().optional(),
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
const LaunchPresetSchema = z.object({
  name: launchConfigFields.name,
  icon: launchConfigFields.icon,
  cmd: launchConfigFields.cmd,
  envNames: z.array(z.string()),
  chat: z.boolean(),
});
export type LaunchPreset = z.infer<typeof LaunchPresetSchema>;
export const LaunchPresetsSchema = z.array(LaunchPresetSchema);
