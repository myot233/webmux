import { onDestroy, untrack } from "svelte";
import { SvelteSet } from "svelte/reactivity";
import { api } from "../../platform/api";
import { setBrowserChrome } from "../../platform/browser-chrome";
import { answerSwitch, changeView, chatActions } from "../../platform/chat-api";
import { errorMessage } from "../../platform/error";
import {
  buildHybridNotices,
  deriveHybridHeaderInfo,
  eligibilityText,
  switchConfirmations,
  switchSteps,
  switchTarget,
  type HybridAmbiguous,
  type HybridConfirm,
  type HybridView,
  type SwitchProgressPhase,
  type SwitchTarget,
} from "../../domains/sessions/hybrid";
export type { HybridNotice, HybridView } from "../../domains/sessions/hybrid";
export type { HybridSession, HybridTerminal } from "./hybrid-types";
import {
  returnQueuedDraft,
  type HybridSession,
  type HybridTerminal,
} from "./hybrid-types";
import {
  loadRecord,
  sessionConflict,
} from "../../domains/sessions/recoverable";
import {
  SessionInfoSchema,
  type AgentConnector,
  type ArchivedSession,
  type ChatEvent,
  type SessionInfo,
} from "../../contracts/schemas";
import type {
  TerminalDraft,
  TerminalDrafts,
} from "../../domains/terminal/terminal-draft.svelte";
import { createChatSocket } from "../chat/chat-socket.svelte";

// A switch to the terminal stays on the progress until the new run has
// drawn on this page, at most this long; the progress stays at least
// minimumProgress so a fast switch does not flash.
const arrivalTimeout = 10_000;
const minimumProgress = 400;

// The hybrid side of the session page: the chat channel, where the session
// is, switching (with the D21 choice and the confirmation before a running
// turn is interrupted), switch notices, and the record fallback once the
// session is gone. Ordinary sessions only ever see view "terminal".
export function useHybrid(
  id: string,
  terminal: HybridTerminal,
  drafts: TerminalDrafts,
  draft: TerminalDraft,
  manageChrome = true,
): HybridSession {
  const chat = createChatSocket(id, live);
  const actions = chatActions(id);
  onDestroy(() => {
    chat.dispose();
    clearTimeout(lingerTimer);
  });

  let record = $state.raw<ArchivedSession>();
  let lookup = $state<"none" | "found" | "absent" | "failed">("none");
  // The session as last read: from a terminal frame while the stream is
  // attached, otherwise from the server whenever the chat channel or the
  // stream reports a change. The latest of the two wins.
  let current = $state.raw<SessionInfo>();
  // Whether `current` was read from the server rather than the stream.
  let fromServer = $state(false);
  let requesting = $state<SwitchTarget | null>(null);
  let confirming = $state<HybridConfirm | null>(null);
  let answering = $state(false);
  let error = $state("");
  let notice = $state("");
  let dropped = $state.raw<string[]>([]);
  const dismissed = new SvelteSet<string>();
  let ambiguous = $state.raw<HybridAmbiguous | null>(null);
  // The connector a switch to chat reported missing.
  let switchConnector = $state.raw<AgentConnector>();
  // The install banner was closed on this page view.
  let connectorDismissed = $state(false);
  // The view before the current switch, for a switch first seen mid-way.
  let settled: SwitchTarget | undefined;
  // The session read that last reattached the terminal; one try per read.
  let reattachedFor: SessionInfo | undefined;

  const state = $derived(chat.state);
  // The session's streams exist: its terminal reported a chat view, or moved.
  const streaming = $derived(
    !!terminal.info?.views.includes("chat") || terminal.connection === "moved",
  );
  const known = $derived(streaming || !!state || !!record);
  const where = $derived(current?.view);
  const phase = $derived(current?.transition?.phase);
  const side = $derived<SwitchTarget | null>(phase ? null : (where ?? null));
  // A switch to the terminal seen on this page: the progress stays up until
  // the reattached stream drew the new run (terminalDrawn below).
  let arriving = $state(false);
  // The run on screen when the switch began; the new run must differ.
  let leftRun: string | undefined;
  // A switch keeps its progress on screen for minimumProgress.
  let lingering = $state(false);
  let lingerTimer: ReturnType<typeof setTimeout> | undefined;
  // Where the switch on screen goes, kept while its progress lingers.
  let movingTo = $state<SwitchTarget | null>(null);
  const moving = $derived(
    !!phase || (!!requesting && where !== requesting) || arriving,
  );
  const view = $derived.by<HybridView>(() => {
    if (!known) return "terminal";
    if (terminal.connection === "missing" && !state)
      return record ? "recovery" : "terminal";
    if (moving || lingering) return "switching";
    return where === "terminal" ? "terminal" : "chat";
  });
  const target = $derived(
    view === "switching"
      ? (requesting ??
          (phase
            ? (current?.transition?.to ?? switchTarget(phase, settled))
            : arriving
              ? "terminal"
              : movingTo))
      : requesting,
  );
  const terminalDrawn = $derived.by(() => {
    const run = terminal.info?.generation;
    if (terminal.info?.status === "exited") return true;
    return (
      terminal.connection === "connected" &&
      !terminal.restoring &&
      !!run &&
      run !== leftRun &&
      terminal.drawn === run
    );
  });
  const ended = $derived(
    terminal.connection === "missing" ||
      chat.connection === "missing" ||
      chat.connection === "exited" ||
      (current?.state ?? "active") !== "active" ||
      !!state?.recovery,
  );
  const info = $derived(
    deriveHybridHeaderInfo(where, fromServer, current, terminal.info, state),
  );
  // While the stream is not attached, the session's view, transition and
  // header are read from the server on every change the chat channel or the
  // stream reports. The model's title re-reads the session so the server's
  // name precedence still applies.
  const readKey = $derived.by(() => {
    if (!known || terminal.connection === "connected") return undefined;
    return [state, chat.model.title, terminal.connection] as const;
  });

  $effect(() => {
    if (streaming) chat.open();
  });
  // Every terminal frame is the newest description of the session.
  $effect(() => {
    const next = terminal.info;
    if (!next) return;
    untrack(() => {
      current = next;
      fromServer = false;
    });
  });
  $effect(() => {
    if (side) settled = side;
  });
  // Each switch keeps its progress up for at least minimumProgress.
  $effect(() => {
    if (!moving) return;
    untrack(() => {
      lingering = true;
      clearTimeout(lingerTimer);
      lingerTimer = setTimeout(() => (lingering = false), minimumProgress);
    });
  });
  $effect(() => {
    if (view === "switching" && target) movingTo = target;
  });
  // A switch to the terminal, by this device or another, holds the progress
  // until the new run has drawn here; a switch that stays on the chat side,
  // a session that ended, or arrivalTimeout lets it go.
  $effect(() => {
    const toTerminal =
      (!!phase && target === "terminal") ||
      (requesting === "terminal" && where !== "terminal");
    if (arriving || !toTerminal) return;
    leftRun = untrack(() => terminal.info?.generation);
    arriving = true;
  });
  $effect(() => {
    if (!arriving) return;
    if (ended) arriving = false;
    else if (!phase && !requesting) {
      if (where === "chat") arriving = false;
      else if (where === "terminal" && terminalDrawn) arriving = false;
    }
  });
  $effect(() => {
    if (!arriving) return;
    const timer = setTimeout(() => (arriving = false), arrivalTimeout);
    return () => clearTimeout(timer);
  });
  // One read at a time: changes during a read ask for one more afterwards.
  let reading = false;
  let readAgain = false;
  let disposed = false;
  onDestroy(() => (disposed = true));
  function read() {
    if (reading) {
      readAgain = true;
      return;
    }
    reading = true;
    api(`/sessions/${id}`, SessionInfoSchema, "GET")
      .then((value) => {
        if (disposed) return;
        current = value;
        fromServer = true;
      })
      .catch(() => {
        // The next change reads it again; the page keeps the last one.
      })
      .finally(() => {
        reading = false;
        if (readAgain && !disposed) {
          readAgain = false;
          read();
        }
      });
  }
  $effect(() => {
    if (readKey === undefined) return;
    untrack(read);
  });
  $effect(() => {
    if (!manageChrome || view !== "chat" || !info) return;
    setBrowserChrome(`${info.title} · WebMux`, info.status === "running");
  });
  // Back on the terminal view, the stream that 4409 closed attaches again.
  $effect(() => {
    if (
      where !== "terminal" ||
      phase ||
      terminal.connection !== "moved" ||
      !fromServer ||
      !current ||
      reattachedFor === current
    )
      return;
    reattachedFor = current;
    terminal.reconnect();
  });
  $effect(() => {
    if (!ended) return;
    const controller = new AbortController();
    loadRecord(id, controller.signal).then(
      (found) => {
        record = found;
        lookup = found ? "found" : "absent";
      },
      () => {
        if (!controller.signal.aborted) lookup = "failed";
      },
    );
    return () => controller.abort();
  });

  // Notices that reach this device while it does not show the timeline,
  // such as prompts returned to the device that switched.
  function live(events: ChatEvent[]) {
    if (view === "chat") return;
    for (const event of events)
      if (event.kind === "notice" && event.notice.level !== "error")
        notice = event.notice.text;
  }

  async function run(to: SwitchTarget, agentSessionId?: string) {
    requesting = to;
    error = "";
    dropped = [];
    ambiguous = null;
    for (const key of dismissed)
      if (key.startsWith("failure:")) dismissed.delete(key);
    try {
      const result = await changeView(id, to, agentSessionId);
      if (result.draft) notice = returnQueuedDraft(drafts, draft, result.draft);
      dropped = result.dropped;
    } catch (err) {
      const conflict = sessionConflict(err);
      if (conflict?.reason === "session_ambiguous" && conflict.candidates)
        ambiguous = {
          message: conflict.error,
          candidates: conflict.candidates,
        };
      else if (conflict?.reason === "connector_missing" && conflict.connector) {
        switchConnector = conflict.connector;
        connectorDismissed = false;
      } else error = errorMessage(err);
    } finally {
      requesting = null;
    }
  }

  const progressPhase = $derived.by<SwitchProgressPhase | undefined>(() => {
    if (view !== "switching") return undefined;
    if (phase) return phase;
    if (where === "terminal" && target === "terminal") return "arriving";
    if (where === "chat" && target === "chat") return "loading";
    return "requested";
  });
  const steps = $derived(
    progressPhase ? switchSteps(target, progressPhase) : [],
  );
  const chatUnavailable = $derived(
    side !== "terminal"
      ? ""
      : state
        ? eligibilityText(state.eligibility)
        : "正在连接聊天通道",
  );
  const connector = $derived.by(() => {
    if (connectorDismissed) return undefined;
    const eligibility = state?.eligibility;
    if (side === "terminal" && eligibility?.reason === "connector_missing")
      return eligibility.connector ?? switchConnector;
    return switchConnector;
  });

  const notices = $derived(
    buildHybridNotices(state, where, error, notice, dropped, dismissed),
  );

  return {
    get known() {
      return known;
    },
    get view() {
      return view;
    },
    chat,
    actions,
    get info() {
      return info;
    },
    get side() {
      return side;
    },
    get target() {
      return target;
    },
    get steps() {
      return steps;
    },
    get chatUnavailable() {
      return chatUnavailable;
    },
    get confirming() {
      return confirming;
    },
    get awaitingChoice() {
      return phase === "awaiting_choice";
    },
    get answering() {
      return answering;
    },
    get notices() {
      return notices;
    },
    get recoverable() {
      return record;
    },
    get missing() {
      return terminal.connection === "missing" && lookup === "absent";
    },
    get connector() {
      return connector;
    },
    get ambiguous() {
      return ambiguous;
    },
    request(to) {
      if (requesting || view === "switching" || !side || to === side) return;
      notice = "";
      if (to === "chat" && chatUnavailable) {
        notice = `现在不能切到聊天：${chatUnavailable}`;
        return;
      }
      const running =
        to === "terminal"
          ? !!state?.turn.running
          : terminal.info?.status === "running";
      if (running) confirming = { to, ...switchConfirmations[to] };
      else void run(to);
    },
    confirm() {
      const pending = confirming;
      confirming = null;
      if (pending) void run(pending.to);
    },
    cancelConfirm() {
      confirming = null;
    },
    async choose(choice) {
      answering = true;
      try {
        await answerSwitch(id, choice);
      } catch (err) {
        error = errorMessage(err);
      } finally {
        answering = false;
      }
    },
    pick(agentSessionId) {
      if (requesting || !agentSessionId) return;
      void run("chat", agentSessionId);
    },
    cancelPick() {
      ambiguous = null;
    },
    dismissConnector() {
      connectorDismissed = true;
    },
    dismiss(key) {
      if (key.startsWith("notice:")) notice = "";
      else if (key.startsWith("dropped:")) dropped = [];
      else {
        if (key.startsWith("failure:")) error = "";
        dismissed.add(key);
      }
    },
  };
}
