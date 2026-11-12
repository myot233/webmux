import { api, APIError } from "../../platform/api";
import { newRequestId } from "../../platform/ids";
import {
  ArchivedSessionSchema,
  ArchivedSessionsSchema,
  EmptyRequestSchema,
  NoContentSchema,
  RebuildPreviewSchema,
  RebuildRequestSchema,
  ReleasePreviewSchema,
  ReleaseResultSchema,
  ReleaseWorktreeRequestSchema,
  SessionConflictSchema,
  SessionInfoSchema,
  WorktreeSchema,
  WorktreeUsageSchema,
  type AgentCandidate,
  type AgentConnector,
  type ArchivedSession,
  type RebuildPreview,
  type SessionConflict,
  type SessionInfo,
  type SessionView,
  type Worktree,
} from "../../contracts/schemas";

/** The session conflict a 409 carries, or undefined for any other failure. */
export function sessionConflict(err: unknown): SessionConflict | undefined {
  if (!(err instanceof APIError) || err.status !== 409) return undefined;
  const conflict = SessionConflictSchema.safeParse(err.body);
  return conflict.success ? conflict.data : undefined;
}

// Archived sessions that can be resumed: closed or interrupted sessions
// whose agent conversation lives on in the agent's own store.

export function loadRecoverable(signal?: AbortSignal) {
  return api(
    "/sessions?state=archived",
    ArchivedSessionsSchema,
    "GET",
    undefined,
    signal,
  );
}

/**
 * Archived session `id`, the session page's fallback when the running
 * session is gone. Undefined when there is none: no such session (404), no
 * readable store (503), or a session that is still active. Other failures
 * prove nothing and are thrown.
 */
export async function loadRecord(
  id: string,
  signal?: AbortSignal,
): Promise<ArchivedSession | undefined> {
  let session: SessionInfo;
  try {
    session = await api(
      `/sessions/${id}`,
      SessionInfoSchema,
      "GET",
      undefined,
      signal,
    );
  } catch (err) {
    if (err instanceof APIError && (err.status === 404 || err.status === 503))
      return undefined;
    throw err;
  }
  const archived = ArchivedSessionSchema.safeParse(session);
  return archived.success ? archived.data : undefined;
}

export type ResumeChoice = {
  /** Picks another agent session after the recorded one went missing. */
  agentSessionId?: string;
  /** Accepts resuming after the launch config (and its env) was deleted. */
  confirmWithoutEnv?: boolean;
};

export type ResumeOutcome =
  | { status: "resumed"; session: SessionInfo }
  | { status: "candidates"; message: string; candidates: AgentCandidate[] }
  | { status: "config_missing"; message: string }
  | { status: "connector_missing"; message: string; connector: AgentConnector };

/** Resumes an archived session on one view; 409s that need a user choice are returned. */
export async function resumeSession(
  id: string,
  view: SessionView,
  choice: ResumeChoice = {},
): Promise<ResumeOutcome> {
  try {
    const session = await api(
      `/sessions/${id}/resume`,
      SessionInfoSchema,
      "POST",
      { clientRequestId: newRequestId(), view, ...choice },
    );
    return { status: "resumed", session };
  } catch (err) {
    const conflict = sessionConflict(err);
    if (conflict?.reason === "session_missing")
      return {
        status: "candidates",
        message: conflict.error,
        candidates: conflict.candidates ?? [],
      };
    if (conflict?.reason === "config_missing")
      return { status: "config_missing", message: conflict.error };
    if (conflict?.reason === "connector_missing" && conflict.connector)
      return {
        status: "connector_missing",
        message: conflict.error,
        connector: conflict.connector,
      };
    throw err;
  }
}

/** Deletes an archived session's record and its temporary folder. */
export function removeRecord(id: string) {
  return api(`/sessions/${id}?forget=1`, NoContentSchema, "DELETE");
}

function worktreeRoute(worktree: Worktree) {
  return `/repositories/${worktree.repositoryId}/worktrees/${worktree.id}`;
}

/** Measures the disk space of `worktree`'s directory; walks it on demand. */
export function measureWorktree(worktree: Worktree, signal?: AbortSignal) {
  return api(
    worktreeRoute(worktree) + "/usage",
    WorktreeUsageSchema,
    "POST",
    {},
    signal,
    EmptyRequestSchema,
  );
}

/**
 * Reads what releasing `worktree` would remove and keep: its blockers, the
 * archived records that stay, and the ignored files deleted with it.
 */
export function previewRelease(worktree: Worktree) {
  return api(
    worktreeRoute(worktree) + "/release-preview",
    ReleasePreviewSchema,
    "POST",
    {},
    undefined,
    EmptyRequestSchema,
  );
}

/** Releases the worktree a preview confirmed; its records stay archived. */
export function releaseWorktree(worktree: Worktree, confirmationId: string) {
  return api(
    worktreeRoute(worktree) + "/release",
    ReleaseResultSchema,
    "POST",
    { confirmationId },
    undefined,
    ReleaseWorktreeRequestSchema,
  );
}

/** Reads how the released worktree of record `id` would be rebuilt. */
export function previewRebuild(id: string) {
  return api(
    `/hybrid-sessions/${id}/rebuild-preview`,
    RebuildPreviewSchema,
    "POST",
    {},
    undefined,
    EmptyRequestSchema,
  );
}

/**
 * Rebuilds the released worktree of record `id` the way `preview` showed;
 * the server refuses when the repository has changed since.
 */
export function rebuildWorktree(id: string, preview: RebuildPreview) {
  return api(
    `/hybrid-sessions/${id}/rebuild`,
    WorktreeSchema,
    "POST",
    {
      releaseId: preview.releaseId,
      mode: preview.mode,
      commit: preview.commit,
    },
    undefined,
    RebuildRequestSchema,
  );
}
