import type { z } from "zod";
import { store, stored } from "../../platform/api";

/** Keep changes reactive even when this browser cannot write to storage. */
export function deviceNumberPreference(key: string, schema: z.ZodType<number>) {
  const saved = stored(key);
  let value = $state(saved ? schema.safeParse(saved).data : undefined);
  return {
    get value() {
      return value;
    },
    set(next: number) {
      value = next;
      store(key, String(next));
    },
    reset(this: void) {
      value = undefined;
      store(key, null);
    },
  };
}

/** The device default applies to missing and unrecognized stored values. */
export function deviceToggle(key: string, enabledByDefault: boolean) {
  const saved = stored(key);
  let enabled = $state(enabledByDefault ? saved !== "0" : saved === "1");
  return {
    get enabled() {
      return enabled;
    },
    set enabled(next: boolean) {
      enabled = next;
      store(key, next ? "1" : "0");
    },
  };
}
