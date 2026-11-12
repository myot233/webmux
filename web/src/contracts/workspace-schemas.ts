import { z } from "zod";
import {
  ChatAttachmentSchema,
  ChatConfigSchema,
  ChatPermissionModeSchema,
  SessionViewSchema,
} from "./chat-schemas";
import { ForwardSchema, SessionInfoSchema } from "./session-schemas";

// One commit: a worktree's last commit or a start point offered for a new
// worktree.
export const CommitSchema = z.object({
  hash: z.string(),
  subject: z.string(),
  author: z.string(),
  committedAt: z.string(),
});
export type Commit = z.infer<typeof CommitSchema>;
// Change counts of a worktree's status; one entry may be both staged and
// unstaged.
const WorktreeChangesSchema = z.object({
  staged: z.number().int().nonnegative(),
  unstaged: z.number().int().nonnegative(),
  untracked: z.number().int().nonnegative(),
  conflicted: z.number().int().nonnegative(),
});
export type WorktreeChanges = z.infer<typeof WorktreeChangesSchema>;
const WorktreeOperationSchema = z.enum([
  "merge",
  "rebase",
  "cherry-pick",
  "revert",
  "bisect",
]);
export type WorktreeOperation = z.infer<typeof WorktreeOperationSchema>;
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
  // Null without an upstream; ahead and behind are also null when the
  // upstream is gone or the counts were not computed.
  upstream: z.string().nullable(),
  ahead: z.number().int().nonnegative().nullable(),
  behind: z.number().int().nonnegative().nullable(),
  upstreamGone: z.boolean(),
  lastCommit: CommitSchema.nullable(),
  // Null when the status could not be read.
  changes: WorktreeChangesSchema.nullable(),
  operation: WorktreeOperationSchema.nullable(),
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
  // Newest FETCH_HEAD change in the repository; approximate, null if never.
  lastFetchedAt: z.iso.datetime({ offset: true }).nullable(),
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

// Server-synchronized choice of worktrees shown on the home page, keyed by
// repository ID. A repository without an entry was never curated.
export const HomeWorktreesSchema = z.record(
  z.string().min(1),
  z.array(z.string().min(1)).max(1000),
);
export type HomeWorktrees = z.infer<typeof HomeWorktreesSchema>;
const UpstreamFields = {
  upstream: z.string().nullable(),
  ahead: z.number().int().nonnegative().nullable(),
  behind: z.number().int().nonnegative().nullable(),
  upstreamGone: z.boolean(),
};
const BranchSchema = z.object({
  name: z.string(),
  head: z.string(),
  worktreePaths: z.array(z.string()),
  subject: z.string(),
  committedAt: z.string(),
  ...UpstreamFields,
  // The main worktree's HEAD contains the branch; false for a bare or
  // unborn main worktree.
  mergedIntoDefault: z.boolean(),
});
export type Branch = z.infer<typeof BranchSchema>;
const BranchBaseSchema = z.object({
  name: z.string(),
  head: z.string(),
  subject: z.string(),
  kind: z.enum(["local", "remote"]),
  committedAt: z.string(),
});
export type BranchBase = z.infer<typeof BranchBaseSchema>;
export const BranchesSchema = z.object({
  branches: z.array(BranchSchema),
  bases: z.array(BranchBaseSchema),
  defaultBase: z.string().nullable(),
  defaultBaseSubject: z.string(),
  defaultBranch: z.string().nullable(),
  defaultParent: z.string(),
});
export type Branches = z.infer<typeof BranchesSchema>;
export const CommitsSchema = z.object({
  commits: z.array(CommitSchema),
});
// GET /repositories/:id/remotes; URLs are redacted by the server.
const RemoteSchema = z.object({
  name: z.string(),
  fetchUrl: z.string(),
  pushUrl: z.string(),
});
export type Remote = z.infer<typeof RemoteSchema>;
export const RemotesSchema = z.object({ remotes: z.array(RemoteSchema) });
export type Remotes = z.infer<typeof RemotesSchema>;
// POST /repositories/:id/fetch: one result per remote, sorted by name, and
// the repository read again afterwards. `message` is "" for "ok" and
// otherwise explains the failure with credentials redacted.
export const FetchResultSchema = z.object({
  remote: z.string(),
  status: z.enum(["ok", "auth", "network", "timeout", "rejected", "error"]),
  message: z.string(),
});
export type FetchResult = z.infer<typeof FetchResultSchema>;
export const FetchRemotesResponseSchema = z.object({
  results: z.array(FetchResultSchema),
  repository: RepositorySchema,
});
export type FetchRemotesResponse = z.infer<typeof FetchRemotesResponseSchema>;
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
const PushCheckSchema = z.object({
  state: z.enum(["pushed", "unpushed", "noRemote", "unknown"]),
  branches: z.array(z.string()),
  more: z.number().int().nonnegative(),
});
export type PushCheck = z.infer<typeof PushCheckSchema>;

const RemovalPreviewItemSchema = z.object({
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
const BatchRemoveItemResultSchema = z.object({
  worktreeId: z.string(),
  path: z.string(),
  branch: z.string().optional(),
  status: z.enum(["removed", "skipped", "failed"]),
  error: z.string().optional(),
});
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
const RebuildModeSchema = z.enum(["reuse", "branch", "detached"]);
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

// Launch configs are server-stored launch commands. Summaries (list) carry only
// env variable names; the full config with values is fetched per id for edits.
// A null preview inherits the browser's global preview setting.
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
const WorkspaceReferenceSchema = z.object({
  repositoryId: z.string(),
  worktreeId: z.string(),
});
// POST /sessions asks for a new worktree first: a new branch from `base`
// (an omitted branch is generated together with the path), or an existing
// branch checked out.
const NewWorktreeSchema = z.object({
  repositoryId: z.string().min(1),
  mode: z.enum(["new", "existing"]),
  branch: z.string().optional(),
  base: z.string().optional(),
  path: z.string().optional(),
});
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
