import { QuickCommandsSchema, type QuickCommand } from "./schemas";

export const QUICK_COMMANDS_LIMIT = 50;

const presets: readonly QuickCommand[] = [
  { id: "preset-resume", name: "", command: "/resume", enter: true },
  { id: "preset-new", name: "", command: "/new", enter: true },
];

/**
 * Parses the stored list. An empty string means the browser never saved a
 * list, so the presets apply; unreadable data degrades to an empty list.
 */
export function parseQuickCommands(raw: string): QuickCommand[] {
  if (!raw) return presets.map((cmd) => ({ ...cmd }));
  try {
    const result = QuickCommandsSchema.safeParse(JSON.parse(raw));
    return result.success ? result.data : [];
  } catch {
    return [];
  }
}

/** Unifies line endings and drops trailing newlines; submission is `enter`'s job. */
export function normalizeCommand(text: string) {
  return text.replace(/\r\n?/g, "\n").replace(/\n+$/, "");
}

/** First non-blank line of the body, with a trailing ellipsis for multi-line bodies. */
export function commandPreview(cmd: QuickCommand) {
  const body = cmd.command
    .split("\n")
    .map((line) => line.trim())
    .filter(Boolean);
  return body.length > 1 ? `${body[0]} …` : (body[0] ?? "");
}

export function commandLabel(cmd: QuickCommand) {
  return (
    cmd.name ||
    (cmd.command
      .split("\n")
      .map((line) => line.trim())
      .find(Boolean) ??
      "")
  );
}

export function newQuickCommandId() {
  const bytes = crypto.getRandomValues(new Uint8Array(8));
  return Array.from(bytes, (byte) => byte.toString(16).padStart(2, "0")).join(
    "",
  );
}
