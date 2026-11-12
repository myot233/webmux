import { z } from "zod";
import { store, stored } from "../../platform/api";
import type { ChatState } from "../../contracts/schemas";

let enabled = $state(stored("nextPromptSuggestions") === "1");
const dismissalKey = "nextPromptDismissals";
const maxDismissals = 100;
const DismissalsSchema = z
  .array(z.tuple([z.string(), z.string()]))
  .max(maxDismissals);

function readDismissals(): [string, string][] {
  try {
    return DismissalsSchema.parse(JSON.parse(stored(dismissalKey, "[]")));
  } catch {
    return [];
  }
}

let dismissals = $state.raw(Object.fromEntries(readDismissals()));

export const nextPromptPreference = {
  get enabled() {
    return enabled;
  },
  set(value: boolean) {
    enabled = value;
    store("nextPromptSuggestions", value ? "1" : "0");
  },
  dismissed(session: string, turn: string) {
    return dismissals[session] === turn;
  },
  dismiss(session: string, turn: string) {
    const current = readDismissals().filter(([id]) => id !== session);
    const next: [string, string][] = [[session, turn], ...current];
    const recent = next.slice(0, maxDismissals);
    dismissals = Object.fromEntries(recent);
    store(dismissalKey, JSON.stringify(recent));
  },
};

export function canSuggestNextPrompt({
  turn,
  plan,
}: Pick<ChatState, "turn" | "plan">) {
  if (turn.unfinished) return false;
  switch (turn.stopReason) {
    case "end_turn":
      return !!plan?.entries.some(
        (entry) => entry.status !== "completed" && entry.content.trim(),
      );
    case "error":
    case "max_tokens":
    case "max_turn_requests":
      return true;
    default:
      return false;
  }
}
