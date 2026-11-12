import { store, stored } from "../../platform/api";
import {
  attentionKey,
  buildAttentionToast,
  cleanAttentionTitle,
  defaultToastDurationMs,
  maxVisibleToasts,
  type AttentionToast,
} from "./attention-toast";
import type { SessionInfo } from "../../contracts/schemas";

function readEnabled(): boolean {
  return stored("toastNotifications", "1") !== "0";
}

let enabled = $state(readEnabled());
let items = $state.raw<AttentionToast[]>([]);
let baseline = false;
let durationMs = defaultToastDurationMs;
let seen: Record<string, string> = {};
let timers: Record<string, ReturnType<typeof setTimeout>> = {};

function clearTimer(sessionId: string) {
  const timer = timers[sessionId];
  if (timer !== undefined) {
    clearTimeout(timer);
    delete timers[sessionId];
  }
}

function scheduleDismiss(sessionId: string) {
  clearTimer(sessionId);
  if (durationMs <= 0) return;
  timers[sessionId] = setTimeout(() => {
    delete timers[sessionId];
    dismissAttentionToast(sessionId);
  }, durationMs);
}

function removeToast(sessionId: string) {
  clearTimer(sessionId);
  if (items.some((item) => item.sessionId === sessionId)) {
    items = items.filter((item) => item.sessionId !== sessionId);
  }
}

function upsertToast(toast: AttentionToast) {
  const rest = items.filter((item) => item.sessionId !== toast.sessionId);
  const next = [toast, ...rest];
  for (const dropped of next.slice(maxVisibleToasts)) {
    clearTimer(dropped.sessionId);
  }
  items = next.slice(0, maxVisibleToasts);
  scheduleDismiss(toast.sessionId);
}

function observeSession(session: SessionInfo) {
  const attention = session.status === "exited" ? null : session.attention;
  if (!attention) {
    delete seen[session.id];
    removeToast(session.id);
    return;
  }
  const key = attentionKey(attention);
  if (seen[session.id] === key) {
    // Keep the visible title fresh if the session was renamed while its toast is up.
    const cleaned = cleanAttentionTitle(session.title);
    if (
      items.some(
        (item) => item.sessionId === session.id && item.title !== cleaned,
      )
    ) {
      items = items.map((item) =>
        item.sessionId === session.id ? { ...item, title: cleaned } : item,
      );
    }
    return;
  }
  seen[session.id] = key;
  if (!enabled) return;
  upsertToast(buildAttentionToast(session, attention));
}

export const attentionToasts = {
  get enabled(): boolean {
    return enabled;
  },
  get list(): readonly AttentionToast[] {
    return items;
  },
};

export function setToastNotificationsEnabled(next: boolean): void {
  enabled = next;
  store("toastNotifications", next ? "1" : "0");
  if (!next) {
    for (const timer of Object.values(timers)) clearTimeout(timer);
    timers = {};
    items = [];
  }
}

/**
 * Seeds the baseline when none has been recorded yet; on reconnect snapshots,
 * diffs against the remembered baseline so alerts raised while disconnected
 * still surface once.
 */
export function observeAttentionSnapshot(
  sessions: readonly SessionInfo[],
): void {
  enabled = readEnabled();
  const activeIds: Record<string, true> = {};
  if (!baseline) {
    baseline = true;
    seen = {};
    for (const session of sessions) {
      activeIds[session.id] = true;
      if (session.status !== "exited" && session.attention) {
        seen[session.id] = attentionKey(session.attention);
      }
    }
    return;
  }
  for (const session of sessions) {
    activeIds[session.id] = true;
    observeSession(session);
  }
  for (const id of Object.keys(seen)) {
    if (!activeIds[id]) delete seen[id];
  }
  for (const item of items) {
    if (!activeIds[item.sessionId]) removeToast(item.sessionId);
  }
}

export function observeAttentionSession(session: SessionInfo): void {
  enabled = readEnabled();
  baseline = true;
  observeSession(session);
}

export function observeAttentionRemoved(sessionId: string): void {
  delete seen[sessionId];
  removeToast(sessionId);
}

export function dismissAttentionToast(sessionId: string): void {
  removeToast(sessionId);
}

export function pauseAttentionToast(sessionId: string): void {
  clearTimer(sessionId);
}

export function resumeAttentionToast(sessionId: string): void {
  if (items.some((item) => item.sessionId === sessionId)) {
    scheduleDismiss(sessionId);
  }
}

export function resetAttentionToasts(
  nextDurationMs = defaultToastDurationMs,
): void {
  for (const timer of Object.values(timers)) clearTimeout(timer);
  timers = {};
  seen = {};
  items = [];
  baseline = false;
  durationMs = nextDurationMs;
  enabled = readEnabled();
}
