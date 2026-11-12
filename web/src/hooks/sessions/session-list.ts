import type { z } from "zod";
import {
  matchesArchivedQuery,
  placeArchived,
  repositoryRecords,
} from "../../domains/sessions/archived";
import {
  homeLayout,
  worktreeTitle,
} from "../../domains/workspace/home-worktrees";
import type {
  ArchivedSession,
  Repository,
  SessionEventSchema,
  SessionInfo,
  Worktree,
} from "../../contracts/schemas";
import { matchesSessionQuery } from "../../domains/sessions/session-location";
import {
  matchesStatusFilter,
  type StatusFilter,
} from "../../domains/sessions/session-status";

function compare(a: SessionInfo, b: SessionInfo) {
  return (
    Date.parse(b.createdAt) - Date.parse(a.createdAt) ||
    a.id.localeCompare(b.id)
  );
}

/** Keep creation order without sorting the full list on every output update. */
export function applySessionEvent(
  sessions: SessionInfo[],
  event: z.infer<typeof SessionEventSchema>,
): SessionInfo[] {
  if (event.type === "snapshot") return [...event.sessions].sort(compare);
  // Forwards live in forwards.svelte.ts; the session list is unchanged.
  if (event.type === "forwards") return sessions;
  const id = event.type === "removed" ? event.id : event.session.id;
  const index = sessions.findIndex((session) => session.id === id);
  if (event.type === "removed") {
    if (index === -1) return sessions;
    return sessions.filter((session) => session.id !== id);
  }
  const next = [...sessions];
  if (index !== -1) {
    if (sessions[index]!.createdAt === event.session.createdAt) {
      next[index] = event.session;
      return next;
    }
    next.splice(index, 1);
  }
  const position = next.findIndex(
    (session) => compare(event.session, session) < 0,
  );
  next.splice(position === -1 ? next.length : position, 0, event.session);
  return next;
}

/**
 * The repositories and worktrees that live sessions name but the repository
 * list does not contain yet. A session started elsewhere (`webmux run
 * --new-worktree`, another browser) creates its worktree after this page
 * read the list, and `/ws/sessions` carries no workspace events, so the
 * sidebar would show it under "已不可用工作区" until something refreshed.
 */
export function unknownWorkspaces(
  sessions: readonly SessionInfo[],
  repositories: readonly Repository[],
): string[] {
  const unknown = new Set<string>();
  for (const { workspace } of sessions) {
    if (!workspace) continue;
    const repository = repositories.find(
      (r) => r.id === workspace.repositoryId,
    );
    if (!repository) unknown.add(`r:${workspace.repositoryId}`);
    else if (!repository.worktrees.some((t) => t.id === workspace.worktreeId))
      unknown.add(`w:${workspace.worktreeId}`);
  }
  return [...unknown];
}

export function filterVisibleSessions(
  scopedSessions: readonly SessionInfo[],
  repositories: readonly Repository[],
  statusFilter: StatusFilter,
  search: string,
): SessionInfo[] {
  return scopedSessions.filter((session) => {
    if (!matchesStatusFilter(session.status, statusFilter)) return false;
    if (!search) return true;
    const repository = repositories.find(
      (repo) => repo.id === session.workspace?.repositoryId,
    );
    const tree = repository?.worktrees.find(
      (worktree) => worktree.id === session.workspace?.worktreeId,
    );
    return matchesSessionQuery(session, search, repository?.name, tree?.branch);
  });
}

// Archived records the filters keep: the archived status or none, and the
// search over their titles, agent, directory, repository and branch.
export function filterVisibleArchiveIds(
  recoverable: readonly ArchivedSession[],
  repositories: readonly Repository[],
  filtering: boolean,
  statusFilter: StatusFilter,
  search: string,
): Set<string> | undefined {
  if (!filtering) return undefined;
  if (statusFilter !== "all" && statusFilter !== "archived")
    return new Set<string>();
  return new Set(
    recoverable
      .filter((record) => {
        if (!search) return true;
        const repository = repositories.find(
          (repo) => repo.id === record.workspace?.repositoryId,
        );
        const tree = repository?.worktrees.find(
          (worktree) => worktree.id === record.workspace?.worktreeId,
        );
        return matchesArchivedQuery(
          record,
          search,
          repository?.name,
          tree?.branch,
        );
      })
      .map((record) => record.id),
  );
}

export function resolveScopedArchive(
  selectedRepository: string,
  selectedWorktree: Worktree | undefined,
  recoverable: readonly ArchivedSession[],
  placement: ReturnType<typeof placeArchived>,
): readonly ArchivedSession[] {
  if (selectedRepository === "all") return recoverable;
  if (selectedRepository === "direct") return placement.direct;
  const entry = placement.repositories.get(selectedRepository);
  if (selectedWorktree)
    return [...(entry?.worktrees.get(selectedWorktree.id) ?? [])];
  return entry ? repositoryRecords(entry) : [];
}

export function resolveScopedSessions(
  selectedRepository: string,
  selectedWorktree: Worktree | undefined,
  list: readonly SessionInfo[],
  direct: readonly SessionInfo[],
): readonly SessionInfo[] {
  if (selectedRepository === "all") return list;
  if (selectedRepository === "direct") return direct;
  return list.filter(
    (session) =>
      session.workspace?.repositoryId === selectedRepository &&
      (!selectedWorktree ||
        session.workspace.worktreeId === selectedWorktree.id),
  );
}

export function deriveRepositoryExpanded(
  repositories: readonly Repository[],
  expansionOverrides: Readonly<Record<string, boolean>>,
  sessions: readonly SessionInfo[],
  placement: ReturnType<typeof placeArchived>,
): Record<string, boolean> {
  return Object.fromEntries(
    repositories.map((repo) => [
      repo.id,
      expansionOverrides[repo.id] ??
        (sessions.some((s) => s.workspace?.repositoryId === repo.id) ||
          placement.repositories.has(repo.id)),
    ]),
  );
}

export function reconcileSelectedRepository(input: {
  loading: boolean;
  initializing: boolean;
  repositoriesSettled: boolean;
  repositoryError: string;
  repositoryChoice: string | null;
  selectedRepository: string;
  selectedWorktreeId: string | null;
  repositories: readonly Repository[];
  list: readonly SessionInfo[];
  select: (repositoryId: string) => void;
}) {
  if (
    input.loading ||
    input.initializing ||
    !input.repositoriesSettled ||
    input.repositoryError
  )
    return;
  if (!input.repositoryChoice) {
    input.select(
      input.repositories.length > 1
        ? ((
            input.repositories.find((repo) =>
              input.list.some(
                (session) => session.workspace?.repositoryId === repo.id,
              ),
            ) ?? input.repositories[0]
          )?.id ?? "all")
        : "all",
    );
  } else if (
    input.repositoryChoice !== "all" &&
    input.repositoryChoice !== "direct" &&
    !input.repositories.some((repo) => repo.id === input.repositoryChoice)
  ) {
    input.select("all");
  } else if (
    input.selectedWorktreeId &&
    input.selectedRepository !== "all" &&
    input.selectedRepository !== "direct"
  ) {
    const repo = input.repositories.find(
      (r) => r.id === input.selectedRepository,
    );
    if (
      repo &&
      repo.availability === "available" &&
      !repo.error &&
      !repo.worktrees.some((t) => t.id === input.selectedWorktreeId)
    ) {
      input.select(input.selectedRepository);
    }
  }
}

export function resolveScopeLabel(
  selectedRepository: string,
  selectedWorktree: Worktree | undefined,
  repositories: readonly Repository[],
): string {
  const repoName = repositories.find(
    (repo) => repo.id === selectedRepository,
  )?.name;
  if (selectedWorktree) {
    return `${repoName} · ${worktreeTitle(selectedWorktree)}`;
  }
  if (selectedRepository === "direct") return "直接会话";
  return repoName ?? "工作区";
}

export function hasVisibleArchivedInRepository(
  placement: ReturnType<typeof placeArchived>,
  id: string,
  visibleArchiveIds: Set<string> | undefined,
): boolean {
  const entry = placement.repositories.get(id);
  return Boolean(
    entry &&
    repositoryRecords(entry).some(
      (record) => !visibleArchiveIds || visibleArchiveIds.has(record.id),
    ),
  );
}

export function isSelectedWorktreePinned(
  homeValue: Parameters<typeof homeLayout>[0],
  repositories: readonly Repository[],
  selectedRepository: string,
  selectedWorktree: Worktree | undefined,
): boolean {
  const repo = repositories.find((r) => r.id === selectedRepository);
  return Boolean(
    repo &&
    selectedWorktree &&
    homeLayout(homeValue, repo, []).shown.some(
      (entry) => entry.tree.id === selectedWorktree.id,
    ),
  );
}

export function isRepositoryGroupOpen(input: {
  repoId: string;
  filtering: boolean;
  visible: readonly SessionInfo[];
  hasArchived: boolean;
  filterExpanded: Readonly<Record<string, boolean>>;
  selectedRepository: string;
  focusedExpanded: Readonly<Record<string, boolean>>;
  expanded: Readonly<Record<string, boolean>>;
}): boolean {
  if (input.filtering) {
    return (
      (input.visible.some(
        (session) => session.workspace?.repositoryId === input.repoId,
      ) ||
        input.hasArchived) &&
      (input.filterExpanded[input.repoId] ?? true)
    );
  }
  if (input.selectedRepository === input.repoId) {
    return input.focusedExpanded[input.repoId] ?? true;
  }
  return input.expanded[input.repoId] ?? false;
}

export function isSessionListEmpty(input: {
  filtering: boolean;
  visibleCount: number;
  archivedMatches: number;
  repositoriesCount: number;
  listCount: number;
  recoverableCount: number;
  selectedRepository: string;
  directCount: number;
  directArchiveCount: number;
}): boolean {
  return (
    (input.filtering && !input.visibleCount && !input.archivedMatches) ||
    (!input.repositoriesCount && !input.listCount && !input.recoverableCount) ||
    (input.selectedRepository === "direct" &&
      !input.directCount &&
      !input.directArchiveCount)
  );
}

export function mountSessionListPolling(
  refreshRepositories: () => void,
  onTick: (now: number) => void,
  onDispose: () => void,
) {
  refreshRepositories();
  const visible = () => {
    if (!document.hidden) {
      onTick(Date.now());
      refreshRepositories();
    }
  };
  document.addEventListener("visibilitychange", visible);
  window.addEventListener("pageshow", visible);
  const clock = setInterval(() => onTick(Date.now()), 30000);
  return () => {
    onDispose();
    clearInterval(clock);
    document.removeEventListener("visibilitychange", visible);
    window.removeEventListener("pageshow", visible);
  };
}

export async function removeRecordAfterWorktree(
  record: ArchivedSession | null,
  removeRecordFn: (id: string) => Promise<void>,
  removeDraft: (id: string) => void,
  refreshArchive: () => Promise<unknown>,
  setRecordError: (msg: string) => void,
  clearRemovingRecord: () => void,
  formatError: (err: unknown) => string,
) {
  if (!record) return;
  setRecordError("");
  try {
    await removeRecordFn(record.id);
    removeDraft(record.id);
  } catch (err) {
    setRecordError(
      `工作树已删除，但无法删除“${record.title}”的记录：${formatError(err)}`,
    );
  } finally {
    clearRemovingRecord();
    await refreshArchive();
  }
}

export function resolveSelectedRepository(
  repositoryChoice: string | null,
  repositories: readonly Repository[],
): string {
  return repositoryChoice &&
    (repositoryChoice === "direct" ||
      repositories.some((repo) => repo.id === repositoryChoice))
    ? repositoryChoice
    : "all";
}

export function filterDirectSessions(
  list: readonly SessionInfo[],
  repositories: readonly Repository[],
): SessionInfo[] {
  return list.filter(
    (s) =>
      !s.workspace ||
      !repositories.some((r) => r.id === s.workspace?.repositoryId),
  );
}
