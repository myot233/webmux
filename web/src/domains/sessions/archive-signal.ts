/**
 * Tells open pages that the user just stopped a session (a close, or a stop
 * and archive) from the shell, so a page showing it can leave instead of
 * offering to resume what was deliberately ended. An event, not state: a
 * session resumed later under the same ID must not be treated as stopped.
 */
export interface ArchiveSignal {
  /** Reports that the user stopped session `id`. */
  emit(id: string): void;
  /** Listens for archived sessions; returns the function that stops it. */
  subscribe(listener: (id: string) => void): () => void;
}

export function createArchiveSignal(): ArchiveSignal {
  const listeners = new Set<(id: string) => void>();
  return {
    emit(id) {
      for (const listener of listeners) listener(id);
    },
    subscribe(listener) {
      listeners.add(listener);
      return () => {
        listeners.delete(listener);
      };
    },
  };
}
