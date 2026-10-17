import { stored, store } from "./api";
import { QuickCommandsSchema, type QuickCommand } from "./schemas";
import { parseQuickCommands } from "./quickCommands";

const key = "quickCommands";

export const quickCommands = $state({
  list: parseQuickCommands(stored(key)),
});

export function saveQuickCommands(list: QuickCommand[]) {
  const data = QuickCommandsSchema.parse(list);
  quickCommands.list = data;
  store(key, JSON.stringify(data));
}

window.addEventListener("storage", (event) => {
  if (event.key === null || event.key === `webmux.${key}`)
    quickCommands.list = parseQuickCommands(stored(key));
});
