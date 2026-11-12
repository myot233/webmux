import { api } from "./api";
import { newRequestId } from "./ids";
import {
  ChatNextPromptRequestSchema,
  ChatNextPromptSchema,
  ChatPermissionModeRequestSchema,
  NoContentSchema,
  SessionInputResultSchema,
  ViewChangeRequestSchema,
  ViewChangeResultSchema,
  type ChatAttachment,
  type ChatPermissionMode,
  type SessionView,
} from "../contracts/schemas";

// Commands of a session's chat view (docs/protocol). Every request
// carries a fresh clientRequestId, which the server runs at most once. The
// outcome reaches every device as chat events and state, so callers only
// report failures.

export type RequestAnswer =
  | { outcome: "selected"; optionId: string }
  | { outcome: "cancelled" }
  | { outcome: "accept"; content: Record<string, string | boolean | string[]> }
  | { outcome: "decline" }
  | { outcome: "cancel" };

export type ConfigValue = { value: string } | { enabled: boolean };

export interface ChatActions {
  send(
    text: string,
    mode: "auto" | "steer" | "refuse" | "now",
    attachments?: ChatAttachment[],
  ): Promise<unknown>;
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
  const session = `/sessions/${id}`;
  const base = `${session}/chat`;
  const command = (path: string, method: string, body: object = {}) =>
    api(`${base}/${path}`, NoContentSchema, method, {
      clientRequestId: newRequestId(),
      ...body,
    });
  const item = (itemId: string) => `queue/${encodeURIComponent(itemId)}`;
  return {
    // Input goes to the session's current view; the chat view takes it as
    // a prompt.
    send: (text, mode, attachments) =>
      api(`${session}/input`, SessionInputResultSchema, "POST", {
        clientRequestId: newRequestId(),
        text,
        mode,
        ...(attachments?.length
          ? { attachments: attachments.map(({ path }) => ({ path })) }
          : {}),
      }),
    // On the chat view an interrupt cancels the running turn.
    cancel: () =>
      api(`${session}/interrupt`, NoContentSchema, "POST", {
        clientRequestId: newRequestId(),
      }),
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
 * Moves session `id` to view `to`. Resolves once the change finished; while
 * it waits for the user's choice (D21) another request answers it.
 * `agentSessionId` picks one of a session_ambiguous conflict's candidates.
 */
export function changeView(
  id: string,
  to: SessionView,
  agentSessionId?: string,
) {
  return api(
    `/sessions/${id}/view`,
    ViewChangeResultSchema,
    "POST",
    { clientRequestId: newRequestId(), to, agentSessionId },
    undefined,
    ViewChangeRequestSchema,
  );
}

/** Answers a view change waiting for the user after the interrupt timed out. */
export function answerSwitch(id: string, choice: "wait" | "force" | "cancel") {
  return api(
    `/sessions/${id}/view`,
    ViewChangeResultSchema,
    "POST",
    { clientRequestId: newRequestId(), choice },
    undefined,
    ViewChangeRequestSchema,
  );
}

export function requestNextPrompt(
  id: string,
  turnId: string,
  endedAt: string,
  signal: AbortSignal,
) {
  return api(
    `/sessions/${id}/chat/next-prompt`,
    ChatNextPromptSchema,
    "POST",
    { turnId, endedAt },
    signal,
    ChatNextPromptRequestSchema,
  );
}
