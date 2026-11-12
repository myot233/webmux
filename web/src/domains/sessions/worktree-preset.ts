/**
 * Initial values of the new-worktree fields; later edits work as usual.
 * "existing" selects `branch`; "new" starts from `startBranch` (a local or
 * remote-tracking branch) with the new branch named `name`.
 */
export type WorktreePreset = {
  mode: "new" | "existing";
  branch?: string;
  startBranch?: string;
  /** Exact saved commit, retained until the user changes the starting branch. */
  base?: string;
  name?: string;
};
