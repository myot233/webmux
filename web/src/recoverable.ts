import { api, APIError } from "./api";
import { newRequestId } from "./ids";
import {
  HybridConflictSchema,
  HybridRecordSchema,
  HybridRecordsSchema,
  NoContentSchema,
  SessionInfoSchema,
  type HybridCandidate,
  type HybridRecord,
  type SessionInfo,
} from "./schemas";

// Hybrid session records that can be resumed: closed or interrupted sessions
// whose agent conversation lives on in the agent's own store.

export function loadRecoverable(signal?: AbortSignal) {
  return api(
    "/hybrid-sessions?state=recoverable",
    HybridRecordsSchema,
    "GET",
    undefined,
    signal,
  );
}

/**
 * The record of session `id`, the session page's fallback when the session
 * itself is gone. Undefined when there is none: no such record (404) or no
 * readable store (503). Other failures prove nothing and are thrown.
 */
export async function loadRecord(
  id: string,
  signal?: AbortSignal,
): Promise<HybridRecord | undefined> {
  try {
    return await api(
      `/hybrid-sessions/${id}`,
      HybridRecordSchema,
      "GET",
      undefined,
      signal,
    );
  } catch (err) {
    if (err instanceof APIError && (err.status === 404 || err.status === 503))
      return undefined;
    throw err;
  }
}

export type ResumeChoice = {
  /** Picks another agent session after the recorded one went missing. */
  agentSessionId?: string;
  /** Accepts resuming after the launch config (and its env) was deleted. */
  confirmWithoutEnv?: boolean;
};

export type ResumeOutcome =
  | { status: "resumed"; session: SessionInfo }
  | { status: "candidates"; message: string; candidates: HybridCandidate[] }
  | { status: "config_missing"; message: string };

/** Resumes a record on one side; 409s that need a user choice are returned. */
export async function resumeHybrid(
  id: string,
  side: "tui" | "acp",
  choice: ResumeChoice = {},
): Promise<ResumeOutcome> {
  try {
    const session = await api(
      `/sessions/${id}/resume`,
      SessionInfoSchema,
      "POST",
      { clientRequestId: newRequestId(), side, ...choice },
    );
    return { status: "resumed", session };
  } catch (err) {
    if (err instanceof APIError && err.status === 409) {
      const conflict = HybridConflictSchema.safeParse(err.body);
      if (conflict.success && conflict.data.reason === "session_missing")
        return {
          status: "candidates",
          message: conflict.data.error,
          candidates: conflict.data.candidates ?? [],
        };
      if (conflict.success && conflict.data.reason === "config_missing")
        return { status: "config_missing", message: conflict.data.error };
    }
    throw err;
  }
}

export function removeRecord(id: string) {
  return api(`/hybrid-sessions/${id}`, NoContentSchema, "DELETE");
}
