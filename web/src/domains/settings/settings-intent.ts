import type { SettingsValues, QuickCommand } from "../../contracts/schemas";
import { parseQuickCommands } from "./quick-commands";

export const defaultPreview = { lines: 10, skip: 1 };
export const defaultCodeReader = {
  lineNumbers: "show",
  diffOverflow: "scroll",
} as const;
export const emptySettings: SettingsValues = {
  theme: null,
  preview: null,
  codeReader: null,
  quickCommands: null,
  homeWorktrees: null,
  sessionPins: null,
  forceSoftKeys: null,
  ignoreHighPorts: null,
};
export type SettingsIntent = (
  latest: SettingsValues,
) => Partial<SettingsValues>;

// Rebase the user's changed IDs and order, preserving concurrently added IDs.
function commandsIntent(
  before: QuickCommand[],
  after: QuickCommand[],
  latest: QuickCommand[],
): QuickCommand[] {
  const removed = new Set(
    before.filter((x) => !after.some((y) => y.id === x.id)).map((x) => x.id),
  );
  const changed = new Map(
    after
      .filter(
        (x) =>
          JSON.stringify(before.find((y) => y.id === x.id)) !==
          JSON.stringify(x),
      )
      .map((x) => [x.id, x]),
  );
  const result = latest
    .filter((x) => !removed.has(x.id))
    .map((x) => changed.get(x.id) ?? x);
  for (const x of changed.values())
    if (!result.some((y) => y.id === x.id)) result.push(x);
  const reordered =
    before
      .filter((x) => after.some((y) => y.id === x.id))
      .map((x) => x.id)
      .join() !==
    after
      .filter((x) => before.some((y) => y.id === x.id))
      .map((x) => x.id)
      .join();
  if (!reordered) return result;
  const byId = new Map(result.map((x) => [x.id, x]));
  return [
    ...after.flatMap((x) => (byId.has(x.id) ? [byId.get(x.id)!] : [])),
    ...result.filter((x) => !after.some((y) => y.id === x.id)),
  ];
}

// Reapply only the added, removed or reordered pins. IDs pinned by another
// device keep their positions and removed IDs are not resurrected by moves.
function pinsIntent(before: string[], after: string[], latest: string[]) {
  const added = after.filter((id) => !before.includes(id));
  const removed = before.filter((id) => !after.includes(id));
  const remaining = latest.filter(
    (id) => !added.includes(id) && !removed.includes(id),
  );
  const result = added.includes(after[0] ?? "")
    ? [...added, ...remaining]
    : [...remaining, ...added];
  const retained = before.filter((id) => after.includes(id));
  const order = after.filter((id) => retained.includes(id));
  const moved = new Set(retained.filter((id, index) => id !== order[index]));
  if (!moved.size) return result;
  const stillPinned = order.filter(
    (id) => moved.has(id) && result.includes(id),
  );
  let index = 0;
  return result.map((id) =>
    stillPinned.includes(id) ? stillPinned[index++]! : id,
  );
}

export function settingsIntent(
  before: SettingsValues,
  changes: Partial<SettingsValues>,
): SettingsIntent {
  // Snapshot the operation so a reactive caller cannot mutate queued intent.
  const original = structuredClone(before);
  const desired = structuredClone(changes);
  return (latest) => {
    const result = structuredClone(desired);
    for (const key of ["preview", "codeReader"] as const) {
      const target = desired[key];
      if (target === undefined || target === null) continue;
      const defaults = key === "preview" ? defaultPreview : defaultCodeReader;
      const base = original[key] ?? defaults;
      const merged = { ...(latest[key] ?? defaults) };
      for (const [field, value] of Object.entries(target)) {
        if (value !== (base as Record<string, unknown>)[field])
          (merged as Record<string, unknown>)[field] = value;
      }
      Object.assign(result, { [key]: merged });
    }
    if (desired.quickCommands !== undefined && desired.quickCommands !== null) {
      result.quickCommands =
        desired.quickCommands.length === 0
          ? []
          : commandsIntent(
              original.quickCommands ?? parseQuickCommands(""),
              desired.quickCommands,
              latest.quickCommands ?? parseQuickCommands(""),
            );
    }
    if (desired.sessionPins) {
      result.sessionPins = pinsIntent(
        original.sessionPins ?? [],
        desired.sessionPins,
        latest.sessionPins ?? [],
      );
    }
    if (desired.homeWorktrees !== undefined && desired.homeWorktrees !== null) {
      const merged = { ...latest.homeWorktrees };
      const old = original.homeWorktrees ?? {};
      for (const key of new Set([
        ...Object.keys(old),
        ...Object.keys(desired.homeWorktrees),
      ])) {
        const next = desired.homeWorktrees[key];
        if (JSON.stringify(old[key]) === JSON.stringify(next)) continue;
        if (next === undefined) {
          const removed = new Set(old[key] ?? []);
          const retained = (merged[key] ?? []).filter((id) => !removed.has(id));
          if (retained.length) merged[key] = retained;
          else delete merged[key];
          continue;
        }
        const removed = new Set(
          (old[key] ?? []).filter((id) => !next.includes(id)),
        );
        merged[key] = [
          ...new Set([
            ...next,
            ...(merged[key] ?? []).filter((id) => !removed.has(id)),
          ]),
        ];
      }
      result.homeWorktrees = merged;
    }
    return result;
  };
}
