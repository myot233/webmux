import type {
  PendingUpdate,
  Repository,
  SessionInfo,
} from "../../contracts/schemas";

// Server-derived values the workspace page needs to draw its first frame.
export interface HomeSetup {
  cwd: string;
  hostname: string;
  pendingUpdate: PendingUpdate | undefined;
  gitReady: boolean;
  registryReady: boolean;
  capabilityError: string;
  // Whether the server detects the ports sessions listen on.
  portDetection: boolean;
  // Whether the server has a [voice] ASR command configured.
  voice?: boolean;
}

// The last data the workspace page showed, kept in memory across route changes
// so returning from a terminal renders it immediately and refreshes behind it.
// Over a slow link the session snapshot, settings and Git discovery each take
// a round trip; none of them should hold the page behind a skeleton again.
// Nothing is persisted: a reload starts empty and `clearHomeCache` drops it
// when authentication is lost.
let repositories: Repository[] | undefined;
let sessions: SessionInfo[] | undefined;
let setup: HomeSetup | undefined;

export const homeCache = {
  repositories: () => repositories,
  sessions: () => sessions,
  setup: () => setup,
  rememberRepositories(value: Repository[]): void {
    repositories = value;
  },
  rememberSessions(value: SessionInfo[]): void {
    sessions = value;
  },
  rememberSetup(value: HomeSetup): void {
    setup = value;
  },
};

export function clearHomeCache(): void {
  repositories = undefined;
  sessions = undefined;
  setup = undefined;
}
