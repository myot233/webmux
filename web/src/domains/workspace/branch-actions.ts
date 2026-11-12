// Which sync actions a local branch offers, decided in one place for the
// worktree menus, the worktree details and the repository panel's branches.
// Counts are local: they compare with the remote-tracking refs of the last
// fetch, and the preview recomputes them exactly.
import {
  MAX_BRANCH_DELETE,
  type Branch,
  type Worktree,
} from "../../contracts/schemas";
import { upstreamState } from "./worktree-sync";

export type BranchAction = "fast-forward" | "push" | "publish" | "delete";

/** A flow of the branch operations, opened as a dialog or panel subview. */
export type BranchOperation =
  | { kind: "fast-forward"; branch: string }
  | { kind: "push"; branch: string; publish: boolean }
  | { kind: "delete"; branches: string[]; cleanup: boolean };

export interface BranchActions {
  /** The branch the actions act on; null when there is none. */
  branch: string | null;
  actions: BranchAction[];
  /** Ahead and behind at once: nothing to offer but the terminal. */
  diverged: boolean;
}

const none = (branch: string | null = null): BranchActions => ({
  branch,
  actions: [],
  diverged: false,
});

function isBranch(target: Branch | Worktree): target is Branch {
  return "worktreePaths" in target;
}

/**
 * The actions of a local branch, or of the branch a worktree has checked
 * out. Only a branch row offers deletion, and only when no worktree has it
 * checked out; a detached, bare, missing or prunable worktree offers none.
 */
export function branchActions(target: Branch | Worktree): BranchActions {
  let name: string;
  if (isBranch(target)) name = target.name;
  else {
    if (
      target.kind === "bare" ||
      !target.available ||
      target.prunable ||
      target.detached ||
      target.branch === null ||
      target.head === null
    )
      return none();
    name = target.branch;
  }
  const actions: BranchAction[] = [];
  let diverged = false;
  const state = upstreamState(target);
  switch (state.kind) {
    case "none":
    case "gone":
      actions.push("publish");
      break;
    case "uncounted":
      // The preview tells which one applies.
      actions.push("fast-forward", "push");
      break;
    case "tracked":
      if (state.ahead > 0 && state.behind > 0) diverged = true;
      else if (state.behind > 0) actions.push("fast-forward");
      else if (state.ahead > 0) actions.push("push");
      break;
  }
  if (isBranch(target) && target.worktreePaths.length === 0)
    actions.push("delete");
  return { branch: name, actions, diverged };
}

export const branchActionLabels: Record<BranchAction, string> = {
  "fast-forward": "快进到上游",
  push: "推送…",
  publish: "发布分支…",
  delete: "删除分支…",
};

export const divergedText = "已分叉，需要在终端合并或变基";

/** The flow an action of a branch opens. */
export function branchOperation(
  action: BranchAction,
  branch: string,
): BranchOperation {
  switch (action) {
    case "fast-forward":
      return { kind: "fast-forward", branch };
    case "push":
    case "publish":
      return { kind: "push", branch, publish: action === "publish" };
    case "delete":
      return { kind: "delete", branches: [branch], cleanup: false };
  }
}

/** The title of a flow's dialog or subview. */
export function operationTitle(operation: BranchOperation): string {
  switch (operation.kind) {
    case "fast-forward":
      return "快进到上游";
    case "push":
      return operation.publish ? "发布分支" : "推送分支";
    case "delete":
      return operation.cleanup ? "清理分支" : "删除分支";
  }
}

/**
 * Branches "清理分支" offers: not checked out anywhere, and either their
 * upstream is gone or the default branch already contains them.
 */
export function cleanupCandidates(branches: readonly Branch[]): string[] {
  return branches
    .filter(
      (branch) =>
        branch.worktreePaths.length === 0 &&
        (branch.upstreamGone || branch.mergedIntoDefault),
    )
    .map((branch) => branch.name);
}

/** A cleanup over the limit takes the first branches; the rest wait. */
export function cleanupOperation(candidates: readonly string[]) {
  return {
    kind: "delete",
    branches: candidates.slice(0, MAX_BRANCH_DELETE),
    cleanup: true,
  } satisfies BranchOperation;
}

/** The command that recreates a deleted branch at its commit. */
export const restoreCommand = (branch: string, commit: string) =>
  `git branch ${branch} ${commit}`;
