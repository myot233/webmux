import type { ResumeChoice, ResumeOutcome } from "./recoverable";
import type {
  ArchivedSession,
  Repository,
  SessionInfo,
  SessionView,
  Worktree,
} from "../../contracts/schemas";

// Archived records are the closed and interrupted sessions. The home
// page shows each one under the group a live session would have: its
// worktree, its repository's unavailable-workspace group, or the direct
// session group of its directory.

export interface RepositoryArchive {
  /** Records by worktree ID, for worktrees in the repository's Git list. */
  worktrees: ReadonlyMap<string, ArchivedSession[]>;
  /** Records whose worktree is no longer in the Git list. */
  missing: ArchivedSession[];
}

export interface ArchivePlacement {
  repositories: ReadonlyMap<string, RepositoryArchive>;
  /** Records outside every registered repository, grouped by `cwd` later. */
  direct: ArchivedSession[];
}

/** What every archived section needs from the home page. */
export interface ArchiveActions {
  now: number;
  /** Filters are active: sections with matches open on their own. */
  filtering: boolean;
  resume: (
    id: string,
    view: SessionView,
    choice?: ResumeChoice,
  ) => Promise<ResumeOutcome>;
  /** Deletes the record and its artifacts; the worktree is left alone. */
  remove: (id: string) => Promise<unknown>;
  /**
   * Opens the worktree removal confirmation and deletes `record` once the
   * worktree is gone. Absent when the record's linked worktree is not
   * removable, so only the record itself can be deleted.
   */
  removeRecordAndWorktree?: (
    record: ArchivedSession,
    worktree: Worktree,
  ) => void;
  /** Opens the release preview of a record's linked worktree. */
  release?: (worktree: Worktree) => void;
  onresumed: (session: SessionInfo) => void;
  /**
   * A released record's worktree was rebuilt and the resume that followed is
   * settled: the repository's worktrees and the records changed. Refreshing
   * may move the record to another group and replace its section, so the
   * page shows `failure`, a failed resume's explanation ("" otherwise).
   */
  onrebuilt?: (failure: string) => void;
}

export const EMPTY_ARCHIVE: RepositoryArchive = {
  worktrees: new Map(),
  missing: [],
};

/** The title an archived record shows: its own, else its agent's name. */
export function recordTitle(record: ArchivedSession) {
  return record.title || record.agent.name;
}

function trimmed(path: string) {
  return path.length > 1 ? path.replace(/\/+$/, "") : path;
}

function within(root: string, path: string) {
  const base = trimmed(root);
  const target = trimmed(path);
  return target === base || target.startsWith(base === "/" ? "/" : `${base}/`);
}

/**
 * Resolves the registered worktree a record belongs to: its recorded
 * worktree association wins; a record without one (written before
 * associations were recorded) joins the registered worktree with the
 * longest path containing its `cwd`.
 */
export function recordWorktree(
  record: ArchivedSession,
  repositories: readonly Repository[],
): Worktree | undefined {
  const recorded = record.workspace;
  if (recorded) {
    const repository = repositories.find(
      (repo) => repo.id === recorded.repositoryId,
    );
    return repository?.worktrees.find(
      (tree) => tree.id === recorded.worktreeId,
    );
  }
  let best: { tree: Worktree; length: number } | undefined;
  for (const repo of repositories)
    for (const tree of repo.worktrees)
      if (
        within(tree.path, record.cwd) &&
        (!best || trimmed(tree.path).length > best.length)
      )
        best = { tree, length: trimmed(tree.path).length };
  return best?.tree;
}

/**
 * The linked worktree whose disk space releasing `record` frees, or
 * undefined when there is none to release: a main/bare checkout, an
 * unavailable or prunable directory, a worktree already gone, or a record
 * already released.
 */
export function releasableRecordWorktree(
  record: ArchivedSession,
  repositories: readonly Repository[],
): Worktree | undefined {
  if (record.released) return undefined;
  const tree = recordWorktree(record, repositories);
  return tree && tree.kind === "linked" && tree.available && !tree.prunable
    ? tree
    : undefined;
}

/**
 * The linked worktree that deleting `record` should remove through the
 * worktree confirmation dialog: the one its space could be released from.
 * Undefined when only the record itself is deleted, including a released
 * record, whose worktree is gone.
 */
export function removableRecordWorktree(
  record: ArchivedSession,
  repositories: readonly Repository[],
): Worktree | undefined {
  return releasableRecordWorktree(record, repositories);
}

/**
 * Places every record exactly once: a recorded worktree association wins;
 * a record without one (written before associations were recorded) joins
 * the registered worktree with the longest path containing its `cwd`; any
 * other record is a direct one. Record order is kept, newest first.
 */
export function placeArchived(
  records: readonly ArchivedSession[],
  repositories: readonly Repository[],
): ArchivePlacement {
  const placed = new Map<
    string,
    { worktrees: Map<string, ArchivedSession[]>; missing: ArchivedSession[] }
  >();
  const direct: ArchivedSession[] = [];
  const repositoryOf = (id: string) => {
    let entry = placed.get(id);
    if (!entry) {
      entry = { worktrees: new Map(), missing: [] };
      placed.set(id, entry);
    }
    return entry;
  };
  const toWorktree = (
    repositoryId: string,
    worktreeId: string,
    record: ArchivedSession,
  ) => {
    const entry = repositoryOf(repositoryId);
    const list = entry.worktrees.get(worktreeId);
    if (list) list.push(record);
    else entry.worktrees.set(worktreeId, [record]);
  };
  for (const record of records) {
    const tree = recordWorktree(record, repositories);
    if (tree) {
      toWorktree(tree.repositoryId, tree.id, record);
      continue;
    }
    const recorded = record.workspace;
    if (
      recorded &&
      repositories.some((repo) => repo.id === recorded.repositoryId)
    ) {
      repositoryOf(recorded.repositoryId).missing.push(record);
      continue;
    }
    direct.push(record);
  }
  return { repositories: placed, direct };
}

/** Every record placed in one repository, worktree by worktree. */
export function repositoryRecords(archive: RepositoryArchive) {
  return [...archive.worktrees.values(), archive.missing].flat();
}

export function matchesArchivedQuery(
  record: ArchivedSession,
  search: string,
  repositoryName?: string,
  branch?: string | null,
) {
  return [
    record.title,
    record.agent.name,
    record.cwd,
    repositoryName,
    branch,
  ].some((value) => value?.toLocaleLowerCase().includes(search));
}
