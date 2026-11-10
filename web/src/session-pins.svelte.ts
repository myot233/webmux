import { SvelteSet } from "svelte/reactivity";
import { z } from "zod";
import { store, stored } from "./api";
import type { SessionInfo } from "./schemas";

const key = "sessionPins";
const limit = 50;
const PinsSchema = z.array(z.string().regex(/^[a-z2-7]{8}$/)).max(limit);

function read(fallback: string[] = []): string[] {
  try {
    return PinsSchema.parse(JSON.parse(stored(key, JSON.stringify(fallback))));
  } catch {
    return fallback;
  }
}

let pins = $state.raw(read());

function save(next: string[]) {
  pins = next;
  store(key, JSON.stringify(next));
}

const pinnable = (session: SessionInfo) =>
  session.state === "active" && session.status !== "exited";

export const sessionPins = {
  has(id: string) {
    return pins.includes(id);
  },
  connect() {
    pins = read(pins);
    function sync(event: StorageEvent) {
      if (
        event.storageArea === localStorage &&
        (event.key === "webmux." + key || event.key === null)
      )
        pins = read();
    }
    window.addEventListener("storage", sync);
    return () => window.removeEventListener("storage", sync);
  },
  toggle(id: string) {
    const current = read(pins);
    save(
      current.includes(id)
        ? current.filter((pin) => pin !== id)
        : [id, ...current].slice(0, limit),
    );
  },
  reconcile(sessions: readonly SessionInfo[]) {
    const current = read(pins);
    const live = new SvelteSet(
      sessions.filter(pinnable).map((session) => session.id),
    );
    const next = current.filter((pin) => live.has(pin));
    if (next.length !== current.length) save(next);
  },
  remove(id: string) {
    const current = read(pins);
    if (current.includes(id)) save(current.filter((pin) => pin !== id));
  },
  ordered(sessions: readonly SessionInfo[]) {
    return pins.flatMap((id) => {
      const session = sessions.find((item) => item.id === id && pinnable(item));
      return session ? [session] : [];
    });
  },
};
