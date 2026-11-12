import { z } from "zod";
import { store, stored } from "../../platform/api";
import type { CodeWorkspaceSnapshot } from "../../contracts/schemas";

// The session page remounts for every session, so the code panel beside it
// would start closed, at the default width and without its tabs each time.
// Whether it is open and how wide it is carry over to the next session as a
// per-browser preference, in chat and workbench mode alike; split panes read
// it when they mount and do not follow one another live. What the panel
// shows belongs to its own session and lives only as long as the page.
const shownKey = "codePanelShown";
const sizeKey = "codePanelSize";
const SizeSchema = z.coerce.number().min(25).max(75);
const snapshots = new Map<string, CodeWorkspaceSnapshot>();

export const codePanelState = {
  shown: () => stored(shownKey) === "1",
  setShown(value: boolean) {
    store(shownKey, value ? "1" : "0");
  },
  /** The code pane's share of the horizontal split, in percent. */
  size: () => SizeSchema.safeParse(stored(sizeKey, "x")).data ?? 50,
  setSize(value: number) {
    if (SizeSchema.safeParse(value).success) store(sizeKey, String(value));
  },
  /** Open tabs, navigation and reading positions a session left behind. */
  snapshot: (session: string) => snapshots.get(session),
  keep(session: string, snapshot: CodeWorkspaceSnapshot) {
    snapshots.set(session, snapshot);
  },
};
