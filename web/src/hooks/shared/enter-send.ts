import { hasTouchInput } from "../../platform/touch-input";

/**
 * How a keydown in a message box should send: "steer" for Ctrl/Cmd+Enter,
 * "send" for a plain Enter, null when the key is not a send (Shift+Enter, IME
 * composition, or Enter on a touch keyboard where it starts a new line and
 * the button sends).
 */
export function enterSendIntent(event: KeyboardEvent): "send" | "steer" | null {
  if (event.isComposing || event.key !== "Enter" || event.shiftKey) return null;
  if (event.ctrlKey || event.metaKey) return "steer";
  return hasTouchInput() ? null : "send";
}
