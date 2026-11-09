import { store, stored } from "./api";
import type { StopReason } from "./schemas";

let enabled = $state(stored("nextPromptSuggestions") === "1");

export const nextPromptPreference = {
  get enabled() {
    return enabled;
  },
  set(value: boolean) {
    enabled = value;
    store("nextPromptSuggestions", value ? "1" : "0");
  },
};

export function nextPrompt(reason: StopReason | undefined) {
  switch (reason) {
    case "end_turn":
      return "请总结本轮结果，并说明我该如何验证。";
    case "error":
      return "请定位刚才的错误，并说明如何修复和验证。";
    case "max_tokens":
    case "max_turn_requests":
      return "请从刚才停止的地方继续。";
    default:
      return "";
  }
}
