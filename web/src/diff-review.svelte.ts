import { z } from "zod";
import { store, stored } from "./api";
import type { CodeChange } from "./schemas";

const key = "diffReview";
const limit = 500;
const MarksSchema = z.array(z.tuple([z.string(), z.string()])).max(limit);

function read(fallback: Record<string, string> = {}) {
  try {
    return Object.fromEntries(
      MarksSchema.parse(
        JSON.parse(stored(key, JSON.stringify(Object.entries(fallback)))),
      ),
    );
  } catch {
    return {} as Record<string, string>;
  }
}

let marks = $state.raw(read());
const markKey = (scope: string, change: CodeChange) =>
  JSON.stringify([scope, change.group, change.nodeId]);

export const diffReview = {
  has(scope: string, change: CodeChange) {
    return (
      !!change.reviewVersion &&
      marks[markKey(scope, change)] === change.reviewVersion
    );
  },
  connect() {
    marks = read(marks);
    function sync(event: StorageEvent) {
      if (
        event.storageArea === localStorage &&
        (event.key === "webmux." + key || event.key === null)
      )
        marks = read();
    }
    window.addEventListener("storage", sync);
    return () => window.removeEventListener("storage", sync);
  },
  toggle(scope: string, change: CodeChange) {
    if (!change.reviewVersion) return;
    const id = markKey(scope, change);
    const next = read(marks);
    const alreadyReviewed = next[id] === change.reviewVersion;
    delete next[id];
    if (!alreadyReviewed) next[id] = change.reviewVersion;
    const recent = Object.entries(next).slice(-limit);
    marks = Object.fromEntries(recent);
    store(key, JSON.stringify(recent));
  },
};
