import { z } from "zod";
import { placeArchived, recordTitle } from "../sessions/archived";
import { homeLayout, worktreeTitle } from "./home-worktrees";
import type {
  ArchivedSession,
  HomeWorktrees,
  Repository,
  SessionInfo,
  Worktree,
} from "../../contracts/schemas";
import { rowStatus } from "../sessions/session-status";

export type TreeNode = {
  key: string;
  level: number;
  parent?: string;
  kind:
    | "all"
    | "repository"
    | "direct"
    | "worktree"
    | "missing"
    | "session"
    | "archived";
  label: string;
  /** Repository or worktree ID; the session for session rows. */
  repositoryId?: string;
  worktreeId?: string;
  session?: SessionInfo;
  /** The archived record of an archived row. */
  record?: ArchivedSession;
  path?: string;
  /** The worktree of a worktree row, for its upstream indicator. */
  worktree?: Worktree;
  /** A linked worktree, which the row's menu can remove. */
  removable?: boolean;
  /** Why a repository is unavailable; "" when it is fine. */
  reason?: string;
  children: TreeNode[];
  /** Live sessions below the node, and how many of them wait. */
  count: number;
  waiting: number;
};

// Newest first by creation, as the home cards: output never reorders rows.
const byCreation = (a: SessionInfo, b: SessionInfo) =>
  Date.parse(b.createdAt) - Date.parse(a.createdAt);

function sessionNodes(
  sessions: readonly SessionInfo[],
  parent: string,
  level: number,
): TreeNode[] {
  return [...sessions].sort(byCreation).map((session) => ({
    key: `s:${session.id}`,
    level,
    parent,
    kind: "session",
    label: session.title,
    session,
    children: [],
    count: 0,
    waiting: 0,
  }));
}

function counted(
  node: Omit<TreeNode, "count" | "waiting">,
  sessions: readonly SessionInfo[],
): TreeNode {
  return {
    ...node,
    count: sessions.length,
    waiting: sessions.filter((s) => rowStatus(s) === "waiting").length,
  };
}

function archivedNodes(
  records: readonly ArchivedSession[],
  parent: string,
  level: number,
): TreeNode[] {
  return records.map((record) => ({
    key: `a:${record.id}`,
    level,
    parent,
    kind: "archived",
    label: recordTitle(record),
    record,
    children: [],
    count: 0,
    waiting: 0,
  }));
}

/**
 * The workbench tree: all repositories, each registered repository with
 * the worktrees its home group shows and their live sessions, then direct
 * sessions. Worktree order is the home page's, independent of output.
 * Counts and waiting numbers cover live sessions only.
 */
export function workbenchTree(
  repositories: readonly Repository[],
  sessions: readonly SessionInfo[],
  home: HomeWorktrees,
  records: readonly ArchivedSession[],
): TreeNode[] {
  // The archived records that fit no existing worktree, such as those of
  // a removed or released one, sit under their repository's unavailable
  // workspaces; a live session wins over a record with its ID.
  const live = new Set(sessions.map((s) => s.id));
  const placement = placeArchived(
    records.filter((record) => !live.has(record.id)),
    repositories,
  );
  const nodes: TreeNode[] = [
    counted(
      { key: "all", level: 1, kind: "all", label: "全部仓库", children: [] },
      sessions,
    ),
  ];
  for (const repo of repositories) {
    const key = `r:${repo.id}`;
    const own = sessions.filter((s) => s.workspace?.repositoryId === repo.id);
    const reason =
      repo.availability === "available" ? "" : repo.error || "仓库当前不可用";
    const children: TreeNode[] = [];
    if (!reason) {
      for (const { tree } of homeLayout(home, repo, own).shown) {
        const treeKey = `w:${tree.id}`;
        const inside = own.filter((s) => s.workspace?.worktreeId === tree.id);
        children.push(
          counted(
            {
              key: treeKey,
              level: 2,
              parent: key,
              kind: "worktree",
              label: worktreeTitle(tree),
              repositoryId: repo.id,
              worktreeId: tree.id,
              path: tree.path,
              removable: tree.kind === "linked",
              worktree: tree,
              children: sessionNodes(inside, treeKey, 3),
            },
            inside,
          ),
        );
      }
      const missing = own.filter(
        (s) => !repo.worktrees.some((t) => t.id === s.workspace?.worktreeId),
      );
      const kept = placement.repositories.get(repo.id)?.missing ?? [];
      if (missing.length || kept.length)
        children.push(
          counted(
            {
              key: `m:${repo.id}`,
              level: 2,
              parent: key,
              kind: "missing",
              label: "已不可用工作区",
              repositoryId: repo.id,
              children: [
                ...sessionNodes(missing, `m:${repo.id}`, 3),
                ...archivedNodes(kept, `m:${repo.id}`, 3),
              ],
            },
            missing,
          ),
        );
    }
    nodes.push(
      counted(
        {
          key,
          level: 1,
          kind: "repository",
          label: repo.name,
          repositoryId: repo.id,
          path: repo.anchorPath,
          reason,
          children,
        },
        own,
      ),
    );
  }
  const direct = sessions.filter(
    (s) =>
      !s.workspace ||
      !repositories.some((r) => r.id === s.workspace?.repositoryId),
  );
  nodes.push(
    counted(
      {
        key: "direct",
        level: 1,
        kind: "direct",
        label: "直接会话",
        children: sessionNodes(direct, "direct", 2),
      },
      direct,
    ),
  );
  return nodes;
}

const ExpandedSchema = z.record(z.string(), z.boolean());
const FolderMemorySchema = z.object({
  open: ExpandedSchema.default({}),
  sessions: z.record(z.string(), z.array(z.string())).default({}),
});
export type FolderMemory = z.infer<typeof FolderMemorySchema>;

/** Every expandable folder's sorted live session IDs. */
function folderLiveSessions(
  nodes: readonly TreeNode[],
): Record<string, string[]> {
  const out: Record<string, string[]> = {};
  const walk = (list: readonly TreeNode[]): string[] => {
    const all: string[] = [];
    for (const node of list) {
      if (node.session) {
        all.push(node.session.id);
        continue;
      }
      if (node.record || node.kind === "all") continue;
      const below = walk(node.children);
      out[node.key] = [...below].sort();
      all.push(...below);
    }
    return all;
  };
  walk(nodes);
  return out;
}

function sameIds(a: readonly string[] | undefined, b: readonly string[]) {
  return (
    a !== undefined &&
    a.length === b.length &&
    a.every((id, index) => id === b[index])
  );
}

export function readFolderMemory(raw: string): FolderMemory {
  try {
    const parsed: unknown = JSON.parse(raw);
    const memory = FolderMemorySchema.safeParse(parsed);
    if (
      memory.success &&
      typeof parsed === "object" &&
      parsed !== null &&
      ("open" in parsed || "sessions" in parsed)
    )
      return memory.data;
    const flat = ExpandedSchema.safeParse(parsed);
    if (flat.success) return { open: flat.data, sessions: {} };
  } catch {
    // Unreadable stored data starts fresh.
  }
  return { open: {}, sessions: {} };
}

/**
 * Reconciles saved folder folds with the current live sessions. Folders
 * whose live sessions changed (or were never recorded) fall back to their
 * default: open down to reachable live sessions, closed without any, and
 * "已不可用工作区" (`m:...`) always closed by default even when a session
 * is archived into it.
 */
export function reconcileFolderMemory(
  memory: FolderMemory,
  nodes: readonly TreeNode[],
): FolderMemory {
  const sessions = folderLiveSessions(nodes);
  let changed = false;
  const open = { ...memory.open };
  for (const [key, ids] of Object.entries(sessions)) {
    if (key in memory.sessions && !sameIds(memory.sessions[key], ids)) {
      changed = true;
      delete open[key];
    } else if (!(key in memory.sessions)) {
      changed = true;
    }
  }
  for (const key of Object.keys(memory.sessions)) {
    if (!(key in sessions)) changed = true;
  }
  return changed ? { open, sessions } : memory;
}

export function defaultNodeOpen(node: TreeNode): boolean {
  if (node.kind === "missing") return false;
  if (node.kind === "repository")
    return node.children.some(
      (child) => child.kind !== "missing" && child.count > 0,
    );
  return node.count > 0;
}
