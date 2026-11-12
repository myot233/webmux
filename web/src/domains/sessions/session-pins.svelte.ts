import { SvelteMap, SvelteSet } from "svelte/reactivity";
import { stored, store } from "../../platform/api";
import { errorMessage } from "../../platform/error";
import { SessionPinsSchema, type SessionInfo } from "../../contracts/schemas";
import {
  observeSettings,
  reloadSettings,
  saveSettings,
  settings,
} from "../settings/settings.svelte";

const key = "sessionPins";
const limit = 50;

let error = $state("");

function savePins(pins: string[]) {
  error = "";
  void saveSettings({ sessionPins: pins }).catch((err: unknown) => {
    error = `无法保存置顶顺序：${errorMessage(err)}`;
  });
}

export const sessionPins = {
  get error() {
    return error;
  },
  get ready() {
    return settings.ready;
  },
  get full() {
    return (settings.values.sessionPins?.length ?? 0) >= limit;
  },
  has(id: string) {
    return settings.values.sessionPins?.includes(id) ?? false;
  },
  connect() {
    let migrating = false;
    return observeSettings(() => {
      if (!settings.ready || migrating) return;
      const raw = stored(key);
      if (!raw) return;
      let legacy: string[];
      try {
        legacy = SessionPinsSchema.parse(JSON.parse(raw));
      } catch {
        store(key, null);
        return;
      }
      const pins = settings.values.sessionPins ?? [];
      const next = [...new SvelteSet([...pins, ...legacy])].slice(0, limit);
      if (next.length === pins.length) {
        store(key, null);
        return;
      }
      migrating = true;
      void saveSettings({ sessionPins: next })
        .then(() => store(key, null))
        .catch(() => undefined)
        .finally(() => (migrating = false));
    });
  },
  toggle(id: string) {
    const pins = settings.values.sessionPins ?? [];
    if (pins.includes(id)) {
      savePins(pins.filter((pin) => pin !== id));
    } else if (pins.length < limit) {
      savePins([id, ...pins]);
    }
  },
  canMove(id: string, offset: -1 | 1, live: readonly SessionInfo[]) {
    const ordered = this.ordered(live);
    const index = ordered.findIndex((session) => session.id === id);
    return index >= 0 && !!ordered[index + offset];
  },
  move(id: string, offset: -1 | 1, live: readonly SessionInfo[]) {
    const ordered = this.ordered(live);
    const index = ordered.findIndex((session) => session.id === id);
    const other = ordered[index + offset];
    if (index < 0 || !other) return;
    const pins = settings.values.sessionPins ?? [];
    savePins(
      pins.map((pin) => (pin === id ? other.id : pin === other.id ? id : pin)),
    );
  },
  refreshMissing(sessions: readonly SessionInfo[]) {
    const pins = settings.values.sessionPins ?? [];
    if (this.ordered(sessions).length < pins.length) void reloadSettings();
  },
  refreshEnded(id: string) {
    if (this.has(id)) void reloadSettings();
  },
  ordered(sessions: readonly SessionInfo[]) {
    const live = new SvelteMap(
      sessions
        .filter(
          (session) =>
            session.state === "active" && session.status !== "exited",
        )
        .map((session) => [session.id, session]),
    );
    return (settings.values.sessionPins ?? []).flatMap(
      (id) => live.get(id) ?? [],
    );
  },
};
