import type { ChatActions } from "../../platform/chat-api";
import type {
  HybridAmbiguous,
  HybridConfirm,
  HybridNotice,
  HybridView,
  SwitchStep,
  SwitchTarget,
} from "../../domains/sessions/hybrid";
import type {
  AgentConnector,
  ArchivedSession,
  SessionInfo,
} from "../../contracts/schemas";
import type {
  TerminalDraft,
  TerminalDrafts,
} from "../../domains/terminal/terminal-draft.svelte";
import type { Connection } from "../../platform/ws";
import type { ChatSocket } from "../chat/chat-socket.svelte";

/** What the hybrid page needs from the session's terminal stream. */
export interface HybridTerminal {
  readonly info: SessionInfo | undefined;
  readonly connection: Connection;
  readonly restoring: boolean;
  /** The run whose output this page has drawn, if any. */
  readonly drawn: string | undefined;
  reconnect(): void;
}

export interface HybridSession {
  /** The session is (or was) a hybrid session. */
  readonly known: boolean;
  readonly view: HybridView;
  readonly chat: ChatSocket;
  readonly actions: ChatActions;
  /** Session info for the header: the terminal's, or read on the chat view. */
  readonly info: SessionInfo | undefined;
  /** The view the session settled on, null while switching or unknown. */
  readonly side: SwitchTarget | null;
  /** The view a running switch goes to, when known. */
  readonly target: SwitchTarget | null;
  readonly steps: SwitchStep[];
  /** Why switching to chat is unavailable now; "" when it is available. */
  readonly chatUnavailable: string;
  readonly confirming: HybridConfirm | null;
  /** The interrupt timed out and the switch waits for a choice (D21). */
  readonly awaitingChoice: boolean;
  readonly answering: boolean;
  readonly notices: HybridNotice[];
  /** The closed or interrupted session, to resume it. */
  readonly recoverable: ArchivedSession | undefined;
  /** Neither the session nor a record exists: its drafts can go. */
  readonly missing: boolean;
  /** The chat side's connector is not installed; undefined once dismissed. */
  readonly connector: AgentConnector | undefined;
  /** A switch to chat waiting for the user to pick the agent session. */
  readonly ambiguous: HybridAmbiguous | null;
  request(to: SwitchTarget): void;
  confirm(): void;
  cancelConfirm(): void;
  choose(choice: "wait" | "force" | "cancel"): Promise<void>;
  /** Switches to chat continuing the picked agent session. */
  pick(agentSessionId: string): void;
  cancelPick(): void;
  dismissConnector(): void;
  dismiss(key: string): void;
}

// D20: the prompts still queued come back to this device's draft only,
// never sent anywhere by themselves.
export function returnQueuedDraft(
  drafts: TerminalDrafts,
  draft: TerminalDraft,
  text: string,
): string {
  if (draft.submission) drafts.addScratch(draft, text);
  else {
    const merged = draft.text ? `${draft.text}\n\n${text}` : text;
    drafts.edit(draft, merged, merged.length);
  }
  drafts.setEnabled(draft, true);
  return "排队的消息已退回到本机草稿，不会自动发送。";
}
