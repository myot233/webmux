import { z } from "zod";
import { store, stored } from "./api";
import type { ChatState } from "./schemas";

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

export function nextPrompt({ turn, plan }: Pick<ChatState, "turn" | "plan">) {
  if (turn.unfinished) return "";
  const step = plan?.entries
    .find((entry) => entry.status !== "completed")
    ?.content.trim()
    .slice(0, 160);
  switch (turn.stopReason) {
    case "end_turn":
      return step ? `请继续完成计划中的「${step}」。` : "";
    case "error":
      return step
        ? `请定位「${step}」遇到的错误，并说明如何验证修复。`
        : "请定位刚才的错误，并说明如何验证修复。";
    case "max_tokens":
    case "max_turn_requests":
      return step
        ? `请接着处理计划中的「${step}」。`
        : "请从刚才停止的地方继续。";
    default:
      return "";
  }
}
