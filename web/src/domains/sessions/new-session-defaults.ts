import { api, APIError, parseArgs, store } from "../../platform/api";
import type { LaunchConfigsState } from "../settings/launch-configs";
import type { WorktreePreset } from "./worktree-preset";
import {
  CreateSessionFailureSchema,
  CreateSessionRequestSchema,
  CreateSessionResultSchema,
  CreateWorktreeRequestSchema,
  NoContentSchema,
  WorktreeSchema,
  type CreateSessionRequest,
  type LaunchConfig,
  type LaunchConfigSummary,
  type Repository,
  type SessionInfo,
  type SessionView,
  type Worktree,
} from "../../contracts/schemas";

type ChosenLaunchConfig = LaunchConfig | LaunchConfigSummary;

/** The contextual entry the new-session dialog was opened from, if any. */
export type LaunchEntry =
  | { kind: "worktree"; repositoryId: string; worktreeId: string }
  | { kind: "new-worktree"; repositoryId: string; preset?: WorktreePreset }
  | { kind: "directory"; cwd: string; workspace: SessionInfo["workspace"] };

/**
 * Where a new session may start: inside one repository, or inside one of
 * its worktrees (that worktree, or a new one from its branch). Without a
 * scope the dialog offers every repository and any directory.
 */
export type LaunchScope = { repositoryId: string; worktreeId?: string };
/**
 * The dialog's location. A worktree target with an empty `worktreeId` is the
 * unset state: the repository is kept but the user must pick a target.
 */
export type TargetKind = "directory" | "worktree" | "new-worktree";
export type InitialTarget = {
  kind: TargetKind;
  cwd: string;
  repositoryId: string;
  worktreeId: string;
};

export function selectableWorktree(tree: Worktree) {
  return tree.available && tree.kind !== "bare";
}

export function worktreeLabel(tree: Worktree) {
  return `${tree.kind === "main" ? "主工作区 · " : ""}${tree.branch || tree.head?.slice(0, 12) || "无提交"}`;
}

/**
 * The scope of a new session started from the home page: a contextual entry
 * names its own repository or worktree; otherwise the repository (and
 * worktree) the home view is narrowed to. Directory entries, "all" and
 * "direct" views have none.
 */
export function resolveLaunchScope(input: {
  entry?: LaunchEntry;
  home: boolean;
  choice: string | null;
  worktreeId: string | null;
  repositories: readonly Repository[];
}): LaunchScope | undefined {
  const { entry } = input;
  if (entry?.kind === "worktree")
    return { repositoryId: entry.repositoryId, worktreeId: entry.worktreeId };
  if (entry?.kind === "new-worktree")
    return { repositoryId: entry.repositoryId };
  if (entry || !input.home) return undefined;
  const repository = input.repositories.find((r) => r.id === input.choice);
  if (!repository) return undefined;
  const tree = repository.worktrees.find((w) => w.id === input.worktreeId);
  return tree
    ? { repositoryId: repository.id, worktreeId: tree.id }
    : { repositoryId: repository.id };
}

/**
 * Initial location by precedence: an explicit contextual entry, then the home
 * page's selected repository (its available main worktree, else another
 * available non-bare tree, else unset), then the latest recent directory,
 * then the service default.
 */
export function resolveInitialTarget(input: {
  entry?: LaunchEntry;
  scope?: LaunchScope;
  selectedRepositoryId?: string;
  repositories: readonly Repository[];
  recent: readonly string[];
  defaultCwd: string;
}): InitialTarget {
  const fallbackCwd = input.recent[0] || input.defaultCwd;
  const { entry, scope } = input;
  if (scope?.worktreeId) {
    const tree = input.repositories
      .find((repo) => repo.id === scope.repositoryId)
      ?.worktrees.find((item) => item.id === scope.worktreeId);
    return {
      kind: tree && selectableWorktree(tree) ? "worktree" : "new-worktree",
      cwd: fallbackCwd,
      repositoryId: scope.repositoryId,
      worktreeId: tree && selectableWorktree(tree) ? tree.id : "",
    };
  }
  if (entry?.kind === "directory")
    return {
      kind: "directory",
      cwd: entry.cwd || fallbackCwd,
      repositoryId: entry.workspace?.repositoryId ?? "",
      worktreeId: "",
    };
  if (entry)
    return {
      kind: entry.kind,
      cwd: fallbackCwd,
      repositoryId: entry.repositoryId,
      worktreeId: entry.kind === "worktree" ? entry.worktreeId : "",
    };
  const repository = input.repositories.find(
    (repo) => repo.id === input.selectedRepositoryId,
  );
  if (!repository)
    return {
      kind: "directory",
      cwd: fallbackCwd,
      repositoryId: "",
      worktreeId: "",
    };
  const trees = repository.worktrees;
  const tree =
    trees.find((item) => item.kind === "main" && selectableWorktree(item)) ??
    trees.find(selectableWorktree);
  return {
    kind: "worktree",
    cwd: fallbackCwd,
    repositoryId: repository.id,
    worktreeId: tree?.id ?? "",
  };
}

export type LaunchSource = "config" | "custom" | "unset";
export type InitialLaunch = { source: LaunchSource; configId: string };

/**
 * Initial command from the remembered `lastLaunch` (`config:<ID>` or
 * `custom`). A remembered config that no longer exists is never replaced by
 * another one; without a memory the first config is used, else a custom
 * command. Returns null while the remembered or default config is loading.
 */
export function resolveInitialLaunch(
  lastLaunch: string,
  launchConfigs: LaunchConfigsState,
): InitialLaunch | null {
  if (lastLaunch === "custom") return { source: "custom", configId: "" };
  const remembered =
    lastLaunch.startsWith("config:") && lastLaunch.length > 7
      ? lastLaunch.slice(7)
      : "";
  if (launchConfigs.status === "loading") return null;
  const configs = launchConfigs.status === "ready" ? launchConfigs.configs : [];
  if (remembered)
    return configs.some((config) => config.id === remembered)
      ? { source: "config", configId: remembered }
      : { source: "unset", configId: "" };
  const first = configs[0]?.id;
  return first
    ? { source: "config", configId: first }
    : { source: "custom", configId: "" };
}

/** The last path segment, used as a short directory name. */
export function directoryName(path: string) {
  return path.replace(/\/+$/, "").split("/").pop() || "/";
}

// The view a config starts on, as stored; values before the view names
// were "acp" and "tui".
export function storedView(value: string): SessionView {
  return value === "chat" || value === "acp" ? "chat" : "terminal";
}

export function buildNewSessionSummary(input: {
  treeTargetMissing: boolean;
  targetReady: boolean;
  treeOnly: boolean;
  kind: TargetKind;
  locationText: string;
  commandReady: boolean;
  source: LaunchSource;
  chosenConfig: ChosenLaunchConfig | undefined;
  command: string;
}): string {
  if (input.treeTargetMissing)
    return "请将位置选为新建工作树，或勾选创建后启动会话";
  if (!input.targetReady)
    return input.kind === "new-worktree" ? "请填写新工作树信息" : "请选择位置";
  if (input.treeOnly) return `将创建 ${input.locationText}`;
  if (!input.commandReady)
    return input.source === "custom"
      ? "请输入可执行程序及参数"
      : "请选择启动方式";
  const launch =
    input.source === "config"
      ? (input.chosenConfig?.name ?? "")
      : input.command.trim();
  return `将在 ${input.locationText} 运行 ${launch}`;
}

type SessionBuildFields = {
  kind: TargetKind;
  repositoryId: string;
  worktreeId: string;
  branchMode: "new" | "existing";
  branch: string;
  base: string;
  newPath: string;
  cwd: string;
  entryDirectory: { cwd: string; workspace: SessionInfo["workspace"] } | null;
  prompt: string;
  hasInstruction: boolean;
  useInstruction: boolean;
  chosenConfig: ChosenLaunchConfig | undefined;
  name: string;
  startSide: SessionView;
};

function buildCreateSessionRequest(
  input: SessionBuildFields & { cmd: string[] | undefined },
): CreateSessionRequest {
  const workspace =
    input.entryDirectory?.workspace && input.cwd === input.entryDirectory.cwd
      ? input.entryDirectory.workspace
      : null;
  const target =
    input.kind === "new-worktree"
      ? {
          newWorktree: {
            repositoryId: input.repositoryId,
            mode: input.branchMode,
            branch: input.branch,
            ...(input.branchMode === "new" ? { base: input.base } : {}),
            ...(input.newPath ? { path: input.newPath } : {}),
          },
        }
      : input.kind === "directory"
        ? {
            cwd: input.cwd,
            ...(workspace
              ? {
                  workspace: {
                    repositoryId: workspace.repositoryId,
                    worktreeId: workspace.worktreeId,
                  },
                }
              : {}),
          }
        : {
            workspace: {
              repositoryId: input.repositoryId,
              worktreeId: input.worktreeId,
            },
          };
  const first = {
    ...(input.prompt.trim() ? { prompt: input.prompt.trim() } : {}),
    ...(input.hasInstruction && input.useInstruction
      ? { useInitInstruction: true }
      : {}),
  };
  return input.chosenConfig
    ? {
        ...target,
        configId: input.chosenConfig.id,
        name: input.name,
        ...(input.chosenConfig.chat ? { view: input.startSide } : {}),
        ...first,
      }
    : { ...target, cmd: input.cmd, name: input.name, ...first };
}

export async function submitNewSessionRequest(
  input: SessionBuildFields & {
    onlyTree: boolean;
    source: LaunchSource;
    command: string;
    makeDirectory: boolean;
    recent: readonly string[];
    adoptTree: (tree: Worktree) => void;
  },
): Promise<{ kind: "tree" } | { kind: "session"; session: SessionInfo }> {
  // Paint the pending state before a fast local response can immediately
  // start loading and mounting the terminal in the same interaction.
  await new Promise<void>((resolve) =>
    requestAnimationFrame(() => requestAnimationFrame(() => resolve())),
  );
  const custom = input.source === "custom";
  const cmd = custom && !input.onlyTree ? parseArgs(input.command) : undefined;
  if (custom && !input.onlyTree && (!cmd?.length || !cmd[0]))
    throw new Error("请输入可执行程序及参数");
  // Only a worktree: create it on its own. With a session, the create
  // request makes the worktree first.
  if (input.kind === "new-worktree" && input.onlyTree) {
    const tree = await api(
      `/repositories/${input.repositoryId}/worktrees`,
      WorktreeSchema,
      "POST",
      {
        path: input.newPath,
        mode: input.branchMode,
        branch: input.branch,
        ...(input.branchMode === "new" ? { base: input.base } : {}),
      },
      undefined,
      CreateWorktreeRequestSchema,
    );
    input.adoptTree(tree);
    return { kind: "tree" };
  }
  const payload = buildCreateSessionRequest({ ...input, cmd });
  if (input.makeDirectory)
    await api("/fs/mkdir", NoContentSchema, "POST", { path: input.cwd });
  let result;
  try {
    result = await api(
      "/sessions",
      CreateSessionResultSchema,
      "POST",
      payload,
      undefined,
      CreateSessionRequestSchema,
    );
  } catch (err) {
    // A worktree the request made stays; a retry starts the session in
    // it instead of making another.
    const failure =
      err instanceof APIError
        ? CreateSessionFailureSchema.safeParse(err.body)
        : undefined;
    if (failure?.success && failure.data.worktree)
      input.adoptTree(failure.data.worktree);
    throw err;
  }
  if (result.worktree) input.adoptTree(result.worktree);
  const session = result.session;
  const recent = [
    session.cwd,
    ...input.recent.filter((p) => p !== session.cwd),
  ].slice(0, 10);
  store("recentDirs", JSON.stringify(recent));
  store(
    "lastLaunch",
    input.chosenConfig ? `config:${input.chosenConfig.id}` : "custom",
  );
  if (input.chosenConfig?.chat) {
    store("hybridStartSide", input.startSide);
    store("hybridStartSide:" + input.chosenConfig.id, input.startSide);
  }
  if (custom) store("customCommand", input.command);
  return { kind: "session", session };
}

export function isCommandReady(
  source: LaunchSource,
  chosenConfig: ChosenLaunchConfig | undefined,
  command: string,
): boolean {
  if (source === "config") return !!chosenConfig;
  if (source !== "custom") return false;
  try {
    const argv = parseArgs(command);
    return !!argv.length && !!argv[0];
  } catch {
    return false;
  }
}

export function resolveFallbackAfterWorktreeRemoved(
  repositories: readonly Repository[],
  kind: TargetKind,
  worktreeId: string,
  removedTree: Worktree,
): { kind: TargetKind; worktreeId: string } {
  if (kind !== "worktree" || worktreeId !== removedTree.id)
    return { kind, worktreeId };
  const main = repositories
    .find((r) => r.id === removedTree.repositoryId)
    ?.worktrees.find((w) => w.kind === "main");
  return main
    ? { kind: "worktree", worktreeId: main.id }
    : { kind: "directory", worktreeId };
}

export function isDirectoryNotFoundError(err: unknown): boolean {
  return (
    err instanceof APIError &&
    err.status === 400 &&
    err.message.startsWith("directory:") &&
    err.message.includes("no such file or directory")
  );
}

export function formatLocationSummaryText(
  kind: TargetKind,
  cwd: string,
  repository: Repository | undefined,
  worktree: Worktree | undefined,
  branch: string,
): string {
  if (kind === "directory") return cwd.trim();
  if (kind === "worktree")
    return worktree
      ? `${repository?.name ?? ""} · ${worktreeLabel(worktree)}`
      : "";
  return `${repository?.name ?? ""} 的新工作树 ${branch}`;
}

export function resolveInstructionRepository(
  kind: TargetKind,
  cwd: string,
  entryDirectory: { cwd: string; workspace: SessionInfo["workspace"] } | null,
  repositories: readonly Repository[],
  repository: Repository | undefined,
): Repository | undefined {
  if (kind !== "directory") return repository;
  const id =
    entryDirectory?.workspace && cwd === entryDirectory.cwd
      ? entryDirectory.workspace.repositoryId
      : "";
  return repositories.find((r) => r.id === id);
}

export function validateNewSessionSubmit(
  onlyTree: boolean,
  kind: TargetKind,
  commandReady: boolean,
): string | null {
  if (onlyTree && kind !== "new-worktree")
    return "请将位置选为新建工作树，或勾选创建后启动会话。";
  if (!onlyTree && !commandReady)
    return "请选择有效启动配置，或输入可执行程序及参数。";
  return null;
}

export function isLocationTargetReady(
  treeTargetMissing: boolean,
  kind: TargetKind,
  cwd: string,
  worktree: Worktree | undefined,
  repository: Repository | undefined,
  worktreeReady: boolean,
): boolean {
  if (treeTargetMissing) return false;
  if (kind === "directory") return !!cwd.trim();
  if (kind === "worktree") return !!worktree && selectableWorktree(worktree);
  return !!repository && worktreeReady;
}

export function formatSubmitButtonLabel(
  busy: boolean,
  treeOnly: boolean,
  split: boolean,
): string {
  if (busy) return treeOnly ? "正在创建…" : "正在启动…";
  if (treeOnly) return "创建工作树";
  return split ? "创建并加入分屏 →" : "创建并进入 →";
}

export function resolvePreferredStartBranch(
  fieldsRepository: Repository | undefined,
  scopeWorktreeId: string | undefined,
): string | undefined {
  return (
    fieldsRepository?.worktrees.find((t) => t.id === scopeWorktreeId)?.branch ??
    undefined
  );
}

export function resolveEntryPreset(
  entry: LaunchEntry | undefined,
  repositoryId: string,
): WorktreePreset | undefined {
  return entry?.kind === "new-worktree" && entry.repositoryId === repositoryId
    ? entry.preset
    : undefined;
}
