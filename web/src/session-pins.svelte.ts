import { z } from "zod";
import { store, stored } from "./api";
import type { SessionInfo } from "./schemas";

const key = "sessionPins";
const limit = 50;
const PinsSchema = z.array(z.string().regex(/^[a-z2-7]{8}$/)).max(limit);

function read(): string[] {
  try {
    return PinsSchema.parse(JSON.parse(stored(key)));
  } catch {
    return [];
  }
}

let pins = $state.raw(read());

export const sessionPins = {
  has(id: string) {
    return pins.includes(id);
  },
  toggle(id: string) {
    pins = pins.includes(id)
      ? pins.filter((pin) => pin !== id)
      : [id, ...pins].slice(0, limit);
    store(key, JSON.stringify(pins));
  },
  ordered(sessions: readonly SessionInfo[]) {
    return pins.flatMap((id) => {
      const session = sessions.find(
        (item) =>
          item.id === id && item.state === "active" && item.status !== "exited",
      );
      return session ? [session] : [];
    });
  },
};
