import { settings, saveSettings } from "./settings.svelte";
import {
  QuickCommandsSchema,
  type QuickCommand,
} from "../../contracts/schemas";
import { parseQuickCommands, quickCommandsNameError } from "./quick-commands";

export const quickCommands = {
  get list() {
    return settings.values.quickCommands ?? parseQuickCommands("");
  },
};
/** Saves a list after the strict name check the snapshot schema skips. */
export async function saveQuickCommands(list: QuickCommand[]) {
  const problem = quickCommandsNameError(list);
  if (problem) throw new Error(problem);
  await saveSettings({ quickCommands: QuickCommandsSchema.parse(list) });
}
