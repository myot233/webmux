import { z } from "zod";

export const WorkspaceLinkSchema = z.object({
  repositoryId: z.string(),
  worktreeId: z.string(),
  worktreePath: z.string(),
});

export const SessionInfoSchema = z.object({
  id: z.string().regex(/^[a-z2-7]{8}$/),
  title: z.string(),
  cwd: z.string(),
  preset: z.string(),
  cmd: z.array(z.string()).min(1),
  status: z.enum(["running", "waiting", "idle", "exited"]),
  exitCode: z.number().int().nullable(),
  lastLine: z.string(),
  createdAt: z.iso.datetime({ offset: true }),
  lastActiveAt: z.iso.datetime({ offset: true }),
  cols: z.number().int().min(1).max(1000),
  rows: z.number().int().min(1).max(1000),
  clients: z.number().int().nonnegative(),
  workspace: WorkspaceLinkSchema.nullable(),
});
export type SessionInfo = z.infer<typeof SessionInfoSchema>;

export const PresetsSchema = z.array(
  z.object({
    name: z.string(),
    label: z.string(),
    cmd: z.array(z.string()).min(1),
    allowArgs: z.boolean(),
  }),
);
export type Preset = z.infer<typeof PresetsSchema>[number];

export const SessionEventSchema = z.discriminatedUnion("type", [
  z.object({
    type: z.literal("snapshot"),
    sessions: z.array(SessionInfoSchema),
  }),
  z.object({ type: z.literal("created"), session: SessionInfoSchema }),
  z.object({ type: z.literal("updated"), session: SessionInfoSchema }),
  z.object({ type: z.literal("removed"), id: SessionInfoSchema.shape.id }),
]);
export const ExitSchema = z.object({ exitCode: z.number().int() });
export const HealthSchema = z.object({
  ok: z.literal(true),
  sessions: z.number().int().nonnegative(),
});
export const WorkspaceCapabilitySchema = z.object({
  gitAvailable: z.boolean(),
  registryAvailable: z.boolean(),
  gitError: z.string(),
  registryError: z.string(),
});
export const ConfigSchema = z.object({
  defaultCwd: z.string(),
  workspace: WorkspaceCapabilitySchema,
});
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
export const RepositorySchema = z.object({
  id: z.string(),
  name: z.string(),
  commonDir: z.string(),
  anchorPath: z.string(),
  availability: z.enum(["available", "unavailable"]),
  error: z.string(),
  checkedAt: z.iso.datetime({ offset: true }),
  worktrees: z.array(WorktreeSchema),
});
export type Repository = z.infer<typeof RepositorySchema>;
export const RepositoriesSchema = z.array(RepositorySchema);
export const BranchesSchema = z.object({
  branches: z.array(
    z.object({
      name: z.string(),
      head: z.string(),
      worktreePaths: z.array(z.string()),
    }),
  ),
  bases: z.array(z.object({ name: z.string(), head: z.string() })),
  defaultBase: z.string().nullable(),
  defaultParent: z.string(),
});
export const RemovalPreviewSchema = z.object({
  confirmationId: z.string(),
  expiresAt: z.iso.datetime({ offset: true }),
  worktree: WorktreeSchema,
  sessions: z.array(SessionInfoSchema),
  blockers: z.array(z.string()),
  forceAllowed: z.boolean(),
});
export type RemovalPreview = z.infer<typeof RemovalPreviewSchema>;
export const DirectoriesSchema = z.array(z.string());
export const ErrorSchema = z.object({ error: z.string() });
export const RecentDirsSchema = z
  .array(z.string())
  .transform((dirs) => dirs.slice(0, 10));
export const FontSizeSchema = z.coerce.number().int().min(12).max(20);
export const ThemeSchema = z.enum(["light", "dark"]);
export type Theme = z.infer<typeof ThemeSchema>;
export const TextSettingSchema = z.string();
export const NoContentSchema = z.undefined();
