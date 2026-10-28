import { onDestroy } from "svelte";
import { SvelteSet } from "svelte/reactivity";
import { api } from "../api";
import { setBrowserChrome } from "../browser-chrome";
import {
  answerSwitch,
  chatActions,
  switchSide,
  type ChatActions,
} from "../chat-api";
import { errorMessage } from "../error";
import {
  eligibilityText,
  shortId,
  switchSteps,
  switchTarget,
  type SwitchStep,
  type SwitchTarget,
} from "../hybrid";
import { loadRecord } from "../recoverable";
import {
  SessionInfoSchema,
  type ChatEvent,
  type ChatState,
  type HybridRecord,
  type SessionInfo,
} from "../schemas";
import type { TerminalDraft, TerminalDrafts } from "../terminal-draft.svelte";
import type { Connection } from "../ws";
import { createChatSocket, type ChatSocket } from "./chat-socket.svelte";

/** What the hybrid page needs from the session's terminal stream. */
export interface HybridTerminal {
  readonly info: SessionInfo | undefined;
  readonly connection: Connection;
  reconnect(): void;
}

/**
 * What the pane shows: the terminal (also for ordinary sessions), the chat
 * view, a switch in progress, or the resume choices of a session that is
 * gone but recoverable.
 */
export type HybridView = "terminal" | "chat" | "switching" | "recovery";

/** A dismissible page strip. */
export type HybridNotice = {
  key: string;
  tone: "info" | "warning" | "danger";
  text: string;
};

/** A switch that waits for the user to confirm interrupting a turn. */
type HybridConfirm = { to: SwitchTarget; title: string; body: string };

export interface HybridSession {
  /** The session is (or was) a hybrid session. */
  readonly known: boolean;
  readonly view: HybridView;
  readonly chat: ChatSocket;
  readonly actions: ChatActions;
  /** Session info for the header: the terminal's, or read on the chat side. */
  readonly info: SessionInfo | undefined;
  /** The side the session settled on, null while switching or unknown. */
  readonly side: SwitchTarget | null;
  /** The side a running switch goes to, when known. */
  readonly target: SwitchTarget | null;
  readonly steps: SwitchStep[];
  /** Why switching to chat is unavailable now; "" when it is available. */
  readonly chatUnavailable: string;
  readonly confirming: HybridConfirm | null;
  /** The interrupt timed out and the switch waits for a choice (D21). */
  readonly awaitingChoice: boolean;
  readonly answering: boolean;
  readonly notices: HybridNotice[];
  /** The record of a closed or interrupted session, to resume it. */
  readonly recoverable: HybridRecord | undefined;
  /** Neither the session nor a record exists: its drafts can go. */
  readonly missing: boolean;
  request(to: SwitchTarget): void;
  confirm(): void;
  cancelConfirm(): void;
  choose(choice: "wait" | "force" | "cancel"): Promise<void>;
  dismiss(key: string): void;
}

const confirmations: Record<SwitchTarget, Omit<HybridConfirm, "to">> = {
  tui: {
    title: "切换到终端？",
    body: "当前回合会被中断，已输出的内容会保留。",
  },
  acp: {
    title: "切换到聊天？",
    body: "TUI 正在执行回合：会先发送中断键，等它停下后再退出 TUI。",
  },
};

// The hybrid side of the session page: the chat channel, where the session
// is, switching (with the D21 choice and the confirmation before a running
// turn is interrupted), switch notices, and the record fallback once the
// session is gone. Ordinary sessions only ever see view "terminal".
export function useHybrid(
  id: string,
  terminal: HybridTerminal,
  drafts: TerminalDrafts,
  draft: TerminalDraft,
): HybridSession {
  const chat = createChatSocket(id, live);
  const actions = chatActions(id);
  onDestroy(() => chat.dispose());

  let record = $state.raw<HybridRecord>();
  let lookup = $state<"none" | "found" | "absent" | "failed">("none");
  let restInfo = $state.raw<SessionInfo>();
  let requesting = $state<SwitchTarget | null>(null);
  let confirming = $state<HybridConfirm | null>(null);
  let answering = $state(false);
  let error = $state("");
  let notice = $state("");
  let dropped = $state.raw<string[]>([]);
  const dismissed = new SvelteSet<string>();
  // The side before the current switch, for a switch first seen mid-way.
  let settled: SwitchTarget | undefined;
  // The state that last reattached the terminal; one try per state.
  let reattachedFor: ChatState | undefined;

  const state = $derived(chat.state);
  // The session's streams exist: its terminal reported it hybrid, or moved.
  const streaming = $derived(
    !!terminal.info?.hybrid || terminal.connection === "moved",
  );
  const known = $derived(streaming || !!state || !!record);
  const where = $derived(state?.side ?? terminal.info?.hybrid?.side);
  const phase = $derived(state ? state.phase : terminal.info?.hybrid?.phase);
  const side = $derived<SwitchTarget | null>(
    phase ? null : where === "acp" || where === "tui" ? where : null,
  );
  const recoverable = $derived(
    record && record.state !== "active" ? record : undefined,
  );
  const view = $derived.by<HybridView>(() => {
    if (!known) return "terminal";
    if (terminal.connection === "missing" && !state)
      return recoverable ? "recovery" : "terminal";
    if (where === "switching" || phase) return "switching";
    return where === "tui" ? "terminal" : "chat";
  });
  const target = $derived(
    view === "switching"
      ? (requesting ?? switchTarget(phase, settled))
      : requesting,
  );
  const ended = $derived(
    terminal.connection === "missing" ||
      chat.connection === "missing" ||
      chat.connection === "exited" ||
      (terminal.info?.hybrid?.state ?? "active") !== "active" ||
      !!state?.recovery,
  );
  const info = $derived.by(() => {
    if (where === "tui" || !restInfo) return terminal.info ?? restInfo;
    // Read once per change below; the turn itself is newer than that read.
    const running = state?.turn.running ?? false;
    const status = running
      ? "running"
      : state?.requests.length
        ? "waiting"
        : restInfo.status === "running"
          ? "idle"
          : restInfo.status;
    return { ...restInfo, status } satisfies SessionInfo;
  });
  // What the header shows of a session off its terminal side.
  const infoKey = $derived(
    state && where !== "tui"
      ? [
          state.side,
          state.phase ?? "",
          state.dormant,
          state.turn.running,
          state.requests.length,
        ].join()
      : "",
  );

  $effect(() => {
    if (streaming) chat.open();
  });
  $effect(() => {
    if (side) settled = side;
  });
  $effect(() => {
    if (!infoKey) return;
    const controller = new AbortController();
    api(
      `/sessions/${id}`,
      SessionInfoSchema,
      "GET",
      undefined,
      controller.signal,
    )
      .then((value) => (restInfo = value))
      .catch(() => {
        // The next change reads it again; the page keeps the last one.
      });
    return () => controller.abort();
  });
  $effect(() => {
    if (view !== "chat" || !info) return;
    setBrowserChrome(`${info.title} · WebMux`, info.status === "running");
  });
  // Back on the terminal side, the stream that 4409 closed attaches again.
  $effect(() => {
    if (
      where !== "tui" ||
      phase ||
      terminal.connection !== "moved" ||
      !state ||
      reattachedFor === state
    )
      return;
    reattachedFor = state;
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

  // D20: the prompts still queued come back to this device's draft only,
  // never sent anywhere by themselves.
  function returnDraft(text: string) {
    if (draft.submission) drafts.addScratch(draft, text);
    else {
      const merged = draft.text ? `${draft.text}\n\n${text}` : text;
      drafts.edit(draft, merged, merged.length);
    }
    drafts.setEnabled(draft, true);
    notice = "排队的消息已退回到本机草稿，不会自动发送。";
  }

  async function run(to: SwitchTarget) {
    requesting = to;
    error = "";
    dropped = [];
    for (const key of dismissed)
      if (key.startsWith("failure:")) dismissed.delete(key);
    try {
      const result = await switchSide(id, to);
      if (result.draft) returnDraft(result.draft);
      dropped = result.dropped;
    } catch (err) {
      error = errorMessage(err);
    } finally {
      requesting = null;
    }
  }

  const steps = $derived(phase ? switchSteps(target, phase) : []);
  const chatUnavailable = $derived(
    side !== "tui"
      ? ""
      : state
        ? eligibilityText(state.eligibility)
        : "正在连接聊天通道",
  );

  const notices = $derived.by(() => {
    const list: HybridNotice[] = [];
    const failure = state?.switchError || error;
    if (failure)
      list.push({ key: `failure:${failure}`, tone: "danger", text: failure });
    if (notice)
      list.push({ key: `notice:${notice}`, tone: "info", text: notice });
    if (dropped.length)
      list.push({
        key: `dropped:${dropped.join(" ")}`,
        tone: "warning",
        text: `以下参数在恢复时未保留：${dropped.join(" ")}`,
      });
    const change = state?.idChange;
    if (change && where === "tui")
      list.push({
        key: `id:${change.from}>${change.to}`,
        tone: "info",
        text: `TUI 已切换到新会话 ${shortId(change.to)}（上一个：${shortId(change.from)}），切到聊天时接手新会话`,
      });
    if (state?.queueLost)
      list.push({
        key: "queue-lost",
        tone: "warning",
        text: `排队的消息已丢失（${state.queueLost} 条），浏览器中的草稿不受影响`,
      });
    return list.filter((item) => !dismissed.has(item.key));
  });

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
      return recoverable;
    },
    get missing() {
      return terminal.connection === "missing" && lookup === "absent";
    },
    request(to) {
      if (requesting || view === "switching" || !side || to === side) return;
      notice = "";
      if (to === "acp" && chatUnavailable) {
        notice = `现在不能切到聊天：${chatUnavailable}`;
        return;
      }
      const running =
        to === "tui"
          ? !!state?.turn.running
          : terminal.info?.status === "running";
      if (running) confirming = { to, ...confirmations[to] };
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
