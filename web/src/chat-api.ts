import { api } from "./api";
import { newRequestId } from "./ids";
import {
  ChatPermissionModeRequestSchema,
  ChatPromptResultSchema,
  HybridSwitchResultSchema,
  NoContentSchema,
  type ChatPermissionMode,
} from "./schemas";

// Commands of a hybrid session (docs/protocol/http-hybrid.md). Every request
// carries a fresh clientRequestId, which the server runs at most once. The
// outcome reaches every device as chat events and state, so callers only
// report failures.

export type RequestAnswer =
  | { outcome: "selected"; optionId: string }
  | { outcome: "cancelled" }
  | { outcome: "accept"; content: Record<string, string | boolean> }
  | { outcome: "decline" }
  | { outcome: "cancel" };

export type ConfigValue = { value: string } | { enabled: boolean };

export interface ChatActions {
  send(text: string, mode: "auto" | "steer"): Promise<unknown>;
  cancel(): Promise<unknown>;
  editQueued(itemId: string, text: string): Promise<unknown>;
  moveQueuedUp(itemId: string): Promise<unknown>;
  removeQueued(itemId: string): Promise<unknown>;
  sendQueuedNow(itemId: string): Promise<unknown>;
  resumeQueue(): Promise<unknown>;
  clearQueue(): Promise<unknown>;
  answer(requestId: string, answer: RequestAnswer): Promise<unknown>;
  setConfig(configId: string, value: ConfigValue): Promise<unknown>;
  setPermissionMode(mode: ChatPermissionMode): Promise<unknown>;
}

/** The chat commands of session `id`. */
export function chatActions(id: string): ChatActions {
  const base = `/sessions/${id}/chat`;
  const command = (path: string, method: string, body: object = {}) =>
    api(`${base}/${path}`, NoContentSchema, method, {
      clientRequestId: newRequestId(),
      ...body,
    });
  const item = (itemId: string) => `queue/${encodeURIComponent(itemId)}`;
  return {
    send: (text, mode) =>
      api(`${base}/prompts`, ChatPromptResultSchema, "POST", {
        clientRequestId: newRequestId(),
        text,
        mode,
      }),
    cancel: () => command("cancel", "POST"),
    editQueued: (itemId, text) => command(item(itemId), "PATCH", { text }),
    moveQueuedUp: (itemId) => command(item(itemId), "PATCH", { move: "up" }),
    removeQueued: (itemId) => command(item(itemId), "DELETE"),
    sendQueuedNow: (itemId) => command(`${item(itemId)}/send-now`, "POST"),
    resumeQueue: () => command("queue/resume", "POST"),
    clearQueue: () => command("queue/clear", "POST"),
    answer: (requestId, answer) =>
      command(`requests/${encodeURIComponent(requestId)}`, "POST", answer),
    setConfig: (configId, value) =>
      command(`config/${encodeURIComponent(configId)}`, "PUT", value),
    setPermissionMode: (mode) =>
      api(
        `${base}/permission-mode`,
        NoContentSchema,
        "PUT",
        { clientRequestId: newRequestId(), mode },
        undefined,
        ChatPermissionModeRequestSchema,
      ),
  };
}

/**
 * Moves session `id` to side `to`. Resolves once the switch finished; while
 * it waits for the user's choice (D21) another request answers it.
 */
export function switchSide(id: string, to: "acp" | "tui") {
  return api(`/sessions/${id}/switch`, HybridSwitchResultSchema, "POST", {
    clientRequestId: newRequestId(),
    to,
  });
}

/** Answers a switch waiting for the user after the interrupt timed out. */
export function answerSwitch(id: string, choice: "wait" | "force" | "cancel") {
  return api(`/sessions/${id}/switch`, HybridSwitchResultSchema, "POST", {
    clientRequestId: newRequestId(),
    choice,
  });
}
