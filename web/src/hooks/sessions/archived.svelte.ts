import { APIError } from "../../platform/api";
import { loadRecoverable } from "../../domains/sessions/recoverable";
import type { ArchivedSession } from "../../contracts/schemas";

/** Closed and interrupted sessions the shell's pages share. */
export interface Archived {
  readonly records: readonly ArchivedSession[];
  /**
   * Reloads the records. Resolves to their IDs, or null when they could not
   * be read; a later call supersedes an earlier one still in flight.
   */
  refresh(): Promise<string[] | null>;
}

export function createArchived(): Archived {
  let records = $state.raw<ArchivedSession[]>([]);
  let request = 0;
  return {
    get records() {
      return records;
    },
    async refresh() {
      const current = ++request;
      try {
        const next = await loadRecoverable();
        if (current === request) records = next;
        return next.map((record) => record.id);
      } catch (err) {
        // An unavailable store (503) or a server without hybrid sessions
        // (404) has nothing to recover; a failed request proves nothing.
        if (
          err instanceof APIError &&
          (err.status === 503 || err.status === 404)
        ) {
          if (current === request) records = [];
          return [];
        }
        return null;
      }
    },
  };
}
