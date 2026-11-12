import { placeArchived } from "../sessions/archived";
import { homeLayout, worktreeTitle } from "./home-worktrees";
import type {
  ArchivedSession,
  HomeWorktrees,
  RemovalPreviewItem,
  Repository,
  SessionInfo,
  Worktree,
} from "../../contracts/schemas";

type WorktreeBatchCategory = "safe" | "blocked" | "protected";
export type WorktreeBatchFilter = "all" | WorktreeBatchCategory;

/** Why a locked registration whose directory is gone is left alone. */
export const LOCKED_STALE_REASON = "已锁定，需在终端解锁后清理";

export interface WorktreeBatchEntry {
  tree: Worktree;
  title: string;
  category: WorktreeBatchCategory;
  pinned: boolean;
  activeSessionCount: number;
  exitedSessionCount: number;
  archivedCount: number;
  reason: string;
}

export interface WorktreeBatchCounts {
  all: number;
  safe: number;
  blocked: number;
  protected: number;
}

export interface BatchPreviewGroups {
  safe: RemovalPreviewItem[];
  blocked: RemovalPreviewItem[];
  unsupported: RemovalPreviewItem[];
}

function insideWorktree(cwd: string, worktreePath: string): boolean {
  if (!cwd || !worktreePath) return false;
  if (cwd === worktreePath) return true;
  const prefix = worktreePath.endsWith("/") ? worktreePath : `${worktreePath}/`;
  return cwd.startsWith(prefix);
}

function classifyWorktree(
  tree: Worktree,
  activeSessionCount: number,
): { category: WorktreeBatchCategory; reason: string } {
  if (tree.kind === "main") {
    return { category: "protected", reason: "主工作区受保护" };
  }
  if (tree.kind !== "linked") {
    return { category: "protected", reason: "Bare 仓库不能在页面删除" };
  }
  if (tree.locked) {
    return {
      category: "protected",
      reason:
        tree.prunable && !tree.available
          ? LOCKED_STALE_REASON
          : tree.lockReason
            ? `已锁定：${tree.lockReason}`
            : "工作树已锁定，请先在终端解锁",
    };
  }
  const missingPrunable = tree.prunable && !tree.available;
  if (!missingPrunable) {
    if (!tree.available || tree.prunable) {
      return { category: "protected", reason: "目录缺失或不可验证" };
    }
    if (tree.dirty === null || tree.error !== "") {
      return {
        category: "protected",
        reason: tree.error ? `状态未知：${tree.error}` : "工作树状态未知",
      };
    }
  }
  const blockers: string[] = [];
  if (tree.dirty === true) blockers.push("未提交修改");
  if (tree.hasUntracked) blockers.push("未跟踪文件");
  if (activeSessionCount > 0) {
    blockers.push(`${activeSessionCount} 个运行中会话`);
  }
  if (blockers.length > 0) {
    return { category: "blocked", reason: blockers.join("、") };
  }
  return {
    category: "safe",
    reason: missingPrunable ? "目录已缺失，可清理登记" : "干净空闲",
  };
}

/**
 * Builds all worktrees of `repo` in stable order (main first, then by path),
 * regardless of whether they are pinned to the home page.
 */
export function buildWorktreeBatchEntries(
  repo: Repository,
  home: HomeWorktrees,
  sessions: readonly SessionInfo[],
  archived: readonly ArchivedSession[],
): WorktreeBatchEntry[] {
  const repoSessions = sessions.filter((s) => s.state === "active");
  const placement = placeArchived([...archived], [repo]).repositories.get(
    repo.id,
  );
  const layout = homeLayout(home, repo, repoSessions, placement?.worktrees);
  const pinnedIds = new Set(
    layout.shown.filter((item) => item.pinned).map((item) => item.tree.id),
  );
  return layout.trees.map((tree) => {
    const matchedSessions = repoSessions.filter(
      (s) =>
        s.workspace?.worktreeId === tree.id || insideWorktree(s.cwd, tree.path),
    );
    const activeSessionCount = matchedSessions.filter(
      (s) => s.status !== "exited",
    ).length;
    const exitedSessionCount = matchedSessions.length - activeSessionCount;
    const archivedCount = archived.filter(
      (rec) =>
        !rec.released &&
        (rec.workspace?.worktreeId === tree.id ||
          insideWorktree(rec.cwd, tree.path)),
    ).length;
    const { category, reason } = classifyWorktree(tree, activeSessionCount);
    return {
      tree,
      title: worktreeTitle(tree),
      category,
      pinned: pinnedIds.has(tree.id),
      activeSessionCount,
      exitedSessionCount,
      archivedCount,
      reason,
    };
  });
}

export function countWorktreeBatchCategories(
  entries: readonly WorktreeBatchEntry[],
): WorktreeBatchCounts {
  const counts: WorktreeBatchCounts = {
    all: entries.length,
    safe: 0,
    blocked: 0,
    protected: 0,
  };
  for (const entry of entries) {
    counts[entry.category]++;
  }
  return counts;
}

/**
 * The stale registrations "清理失效登记" preselects: unlocked linked
 * worktrees Git marks prunable whose directory is gone, and that are safe
 * to clean (no running session). The batch preview checks the directory
 * again on the server; this list only drives the shortcut and its count.
 */
export function staleRegistrations(
  entries: readonly WorktreeBatchEntry[],
): WorktreeBatchEntry[] {
  return entries.filter(
    ({ tree, category }) =>
      category === "safe" &&
      tree.kind === "linked" &&
      tree.prunable &&
      !tree.available &&
      !tree.locked,
  );
}

export function filterWorktreeBatchEntries(
  entries: readonly WorktreeBatchEntry[],
  filter: WorktreeBatchFilter,
  query: string,
): WorktreeBatchEntry[] {
  const term = query.trim().toLowerCase();
  return entries.filter((entry) => {
    if (filter !== "all" && entry.category !== filter) return false;
    if (!term) return true;
    const head = entry.tree.head ?? "";
    const branch = entry.tree.branch ?? "";
    return (
      entry.title.toLowerCase().includes(term) ||
      branch.toLowerCase().includes(term) ||
      head.toLowerCase().includes(term) ||
      entry.tree.path.toLowerCase().includes(term)
    );
  });
}

export function groupBatchPreviewItems(
  items: readonly RemovalPreviewItem[],
): BatchPreviewGroups {
  const groups: BatchPreviewGroups = {
    safe: [],
    blocked: [],
    unsupported: [],
  };
  for (const item of items) {
    if (!item.forceAllowed) groups.unsupported.push(item);
    else if (item.blockers.length > 0) groups.blocked.push(item);
    else groups.safe.push(item);
  }
  return groups;
}
