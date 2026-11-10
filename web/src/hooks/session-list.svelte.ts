import {
  observeAttentionRemoved,
  observeAttentionSession,
  observeAttentionSnapshot,
} from "../attention-toast.svelte";
import { connectSessions } from "../forwards.svelte";
import { homeCache } from "../home-cache";
import type { SessionInfo } from "../schemas";
import { sessionPins } from "../session-pins.svelte";
import { applySessionEvent } from "../session-list";
import type { TerminalDrafts } from "../terminal-draft.svelte";
import type { Connection } from "../ws";
import type { Archived } from "./archived.svelte";

/** The live `/ws/sessions` list the shell's pages share. */
export interface SessionList {
  readonly sessions: readonly SessionInfo[];
  /** No snapshot has arrived yet and nothing was cached. */
  readonly loading: boolean;
  readonly connection: Connection;
  /** The connection's last error; "" while none. */
  readonly error: string;
  close(): void;
}

export function createSessionList({
  drafts,
  archived,
  onreconnect,
}: {
  drafts: TerminalDrafts;
  archived: Archived;
  /** The connection came back after dropping. */
  onreconnect: () => void;
}): SessionList {
  // The previous list renders at once and is refreshed behind the page.
  const cached = homeCache.sessions();
  if (cached) observeAttentionSnapshot(cached);
  let sessions = $state.raw<SessionInfo[]>(cached ?? []);
  let loading = $state(!cached);
  let connection = $state<Connection>("connecting");
  let error = $state("");
  const socket = connectSessions(
    (event) => {
      if (event.type === "forwards") return;
      // A session with a chat view leaves the list when it is closed or
      // interrupted, and lives on as an archived session.
      const removedHybrid =
        event.type === "removed" &&
        !!sessions
          .find((session) => session.id === event.id)
          ?.views.includes("chat");
      sessions = applySessionEvent(sessions, event);
      homeCache.rememberSessions(sessions);
      if (event.type === "snapshot") {
        observeAttentionSnapshot(event.sessions);
        sessionPins.refreshMissing(event.sessions);
        const ids = event.sessions.map((session) => session.id);
        // Recoverable hybrid sessions still own their drafts; without
        // their IDs, leave saved drafts alone rather than guess.
        void archived.refresh().then((records) => {
          if (records) drafts.reconcile([...ids, ...records]);
        });
        loading = false;
      } else if (event.type === "removed") {
        observeAttentionRemoved(event.id);
        sessionPins.refreshEnded(event.id);
        if (removedHybrid) void archived.refresh();
        else drafts.remove(event.id);
      } else {
        observeAttentionSession(event.session);
        if (
          event.session.status === "exited" ||
          event.session.state !== "active"
        )
          sessionPins.refreshEnded(event.session.id);
        if (
          event.session.views.includes("chat") &&
          (event.type === "created" || event.session.state !== "active")
        ) {
          // A resumed shell or one that just closed or was interrupted.
          void archived.refresh();
        }
      }
    },
    (state) => {
      const reconnected =
        state === "connected" && connection === "reconnecting";
      connection = state;
      if (reconnected) onreconnect();
    },
    (message) => {
      error = message;
    },
  );
  return {
    get sessions() {
      return sessions;
    },
    get loading() {
      return loading;
    },
    get connection() {
      return connection;
    },
    get error() {
      return error;
    },
    close() {
      socket.close();
    },
  };
}
