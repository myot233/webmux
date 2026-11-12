import { getContext, setContext } from "svelte";
import { z } from "zod";
import { navigate, store, stored } from "../../platform/api";
import type { ArchiveSignal } from "../../domains/sessions/archive-signal";
import type { BranchOperation } from "../../domains/workspace/branch-actions";
import type { Archived } from "../sessions/archived.svelte";
import type { LaunchConfigs } from "../settings/launch-configs.svelte";
import type { Repositories } from "../workspace/repositories.svelte";
import type { ServerSetup } from "../workspace/server-setup.svelte";
import type { SessionList } from "../sessions/session-list.svelte";
import type { LaunchEntry } from "../../domains/sessions/new-session-defaults";
import type {
  ArchivedSession,
  Repository,
  SessionInfo,
  Worktree,
} from "../../contracts/schemas";
import type { TerminalDrafts } from "../../domains/terminal/terminal-draft.svelte";

// The app shell around the home page, quick start and session pages: which
// mode the sidebar shows, its width, and whether it is collapsed (desktop)
// or open as a drawer (phones). The sidebar is only a way to navigate:
// opening a session never changes its mode. Mode, width and collapse
// survive reloads on this device.

export type ShellMode = "chat" | "workbench";
/** The sections of the repository panel. */
export type RepositoryTab = "overview" | "worktrees" | "branches" | "remotes";
const ModeSchema = z.enum(["chat", "workbench"]);
const CollapsedSchema = z.enum(["true", "false"]);

// First use, or an invalid stored value, starts in the workbench.
let mode = $state<ShellMode>(
  ModeSchema.safeParse(stored("shellMode")).data ?? "workbench",
);
let collapsed = $state(
  CollapsedSchema.safeParse(stored("sidebarCollapsed")).data === "true",
);
/** The full sidebar's width bounds in CSS pixels. */
export const sidebarWidthMin = 256;
export const sidebarWidthMax = 480;
const sidebarWidthDefault = 288;
function clampWidth(width: number) {
  return Math.round(
    Math.max(sidebarWidthMin, Math.min(sidebarWidthMax, width)),
  );
}
// stored() answers "" for a missing key; only a saved pixel width counts.
const storedWidth = z
  .string()
  .regex(/^\d{1,4}$/)
  .transform(Number)
  .safeParse(stored("sidebarWidth")).data;
let width = $state(
  storedWidth === undefined ? sidebarWidthDefault : clampWidth(storedWidth),
);
let drawerOpen = $state(false);

export const shellUi = {
  get mode(): ShellMode {
    return mode;
  },
  /** Remembers the mode; the sidebar follows, the page stays. */
  setMode(next: ShellMode) {
    mode = next;
    store("shellMode", next);
  },
  get collapsed() {
    return collapsed;
  },
  setCollapsed(next: boolean) {
    collapsed = next;
    store("sidebarCollapsed", String(next));
  },
  get width() {
    return width;
  },
  /** Sets the full sidebar's width, kept within its bounds. */
  setWidth(next: number) {
    width = clampWidth(next);
    store("sidebarWidth", String(width));
  },
  get drawerOpen() {
    return drawerOpen;
  },
  set drawerOpen(next: boolean) {
    drawerOpen = next;
  },
  /** The current mode's home page. */
  get home() {
    return mode === "chat" ? "/new" : "/";
  },
};

/**
 * Opening `/` from outside the shell (a fresh load, the settings page)
 * returns to the remembered mode: chat mode replaces the entry with `/new`,
 * so going back never lands on the workbench home first.
 */
export function enterHome() {
  if (location.pathname === "/" && mode === "chat")
    history.replaceState(
      history.state,
      "",
      "/new" + location.search + location.hash,
    );
}

/** The shell's shared data and the actions its sidebar and pages offer. */
export interface Shell {
  readonly drafts: TerminalDrafts;
  readonly sessions: SessionList;
  readonly repositories: Repositories;
  readonly archived: Archived;
  /** The user archived a session from the shell; open pages may leave it. */
  readonly archiveSignal: ArchiveSignal;
  readonly setup: ServerSetup;
  readonly launchConfigs: LaunchConfigs;
  /** Opens the new-session dialog, optionally at a contextual entry. */
  start(entry?: LaunchEntry): void;
  /** Opens the shared rename dialog for a live session. */
  rename(session: SessionInfo): void;
  /** Opens the shared close, archive or remove dialog for a session. */
  close(session: SessionInfo): void;
  /**
   * Opens the shared worktree removal confirmation dialog. `forgetRecords`
   * are archived records the caller deletes with the worktree; `onremoved`
   * runs after a successful removal, before the dialog closes.
   */
  removeWorktree(
    worktree: Worktree,
    options?: {
      onremoved?: () => void;
      forgetRecords?: readonly ArchivedSession[];
    },
  ): void;
  /** Opens the choice of which worktrees a repository's home page lists. */
  chooseHomeWorktrees(repository: Repository): void;
  /**
   * Opens a repository's panel at a section (the overview by default) and
   * reads the repositories again, as the home page does.
   */
  repositoryDetails(repository: Repository, tab?: RepositoryTab): void;
  /** Opens a worktree's details outside the repository panel. */
  worktreeDetails(worktree: Worktree): void;
  /**
   * Opens the switch of a worktree's branch outside the repository panel,
   * which shows the same flow as one of its subviews.
   */
  switchBranch(worktree: Worktree): void;
  /** Opens the shared release preview of an archived record's worktree. */
  releaseWorktree(worktree: Worktree): void;
  /**
   * Opens a branch operation (fast-forward, push, delete) outside the
   * repository panel, which shows the same flow as one of its subviews.
   */
  branchOperation(repositoryId: string, operation: BranchOperation): void;
  /** Opens the port-forwarding dialog. */
  forward(): void;
  /** Opens the add-repository dialog. */
  addRepository(): void;
  /**
   * The repository the home view is narrowed to (or the only one), whose
   * settings the sidebar footer opens; undefined elsewhere.
   */
  readonly settingsRepository: Repository | undefined;
  /** Opens the settings of `settingsRepository`. */
  openRepositorySettings(): void;
}

const key = Symbol("shell");
export function setShell(shell: Shell) {
  setContext(key, shell);
}
/** The shell, or undefined for a page rendered outside it (pop-outs, tests). */
export function findShell(): Shell | undefined {
  return getContext<Shell | undefined>(key);
}
export function getShell(): Shell {
  const shell = getContext<Shell | undefined>(key);
  if (!shell) throw new Error("页面必须位于应用外壳中");
  return shell;
}

/** Opens a session from the sidebar or a home card; the mode stays. */
export function openSession(session: Pick<SessionInfo, "id">) {
  drawerOpen = false;
  navigate(`/s/${session.id}`);
}

/** Opens a page of the shell (a home page or a scope) and closes the drawer. */
export function openPage(path: string) {
  drawerOpen = false;
  if (location.pathname + location.hash === path) return;
  navigate(path);
}
