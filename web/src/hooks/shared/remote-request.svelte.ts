import { onMount } from "svelte";
import type { z } from "zod";
import { api } from "../../platform/api";
import { errorMessage } from "../../platform/error";

/** One request issued through {@link LatestRequest}. */
export interface RequestTicket {
  signal: AbortSignal;
  /** The owner is alive and this request has not been invalidated. */
  current(): boolean;
  /** A failure of this request should still be shown to the user. */
  reportable(): boolean;
}

/**
 * Serializes a page's requests: each new request aborts the one it replaces,
 * and a ticket lets late replies of superseded requests be dropped.
 */
export class LatestRequest {
  #inflight: AbortController | undefined;
  #disposed = false;

  get disposed() {
    return this.#disposed;
  }

  begin(): RequestTicket {
    const previous = this.#inflight;
    const controller = (this.#inflight = new AbortController());
    previous?.abort();
    const current = () => !this.#disposed && this.#inflight === controller;
    return {
      signal: controller.signal,
      current,
      reportable: () => current() && !controller.signal.aborted,
    };
  }

  /** Invalidates the current request while allowing the owner to start another. */
  cancel() {
    const previous = this.#inflight;
    this.#inflight = undefined;
    previous?.abort();
  }

  dispose() {
    this.#disposed = true;
    this.cancel();
  }
}

/** Reads path once after mount; the request is aborted on unmount. */
export function useMountedRead<T>(path: string, schema: z.ZodType<T>) {
  const state = $state<{ value: T | undefined; error: string }>({
    value: undefined,
    error: "",
  });
  onMount(() => {
    const controller = new AbortController();
    void api(path, schema, "GET", undefined, controller.signal)
      .then((value) => {
        state.value = value;
      })
      .catch((cause: unknown) => {
        if (!controller.signal.aborted) state.error = errorMessage(cause);
      });
    return () => controller.abort();
  });
  return state;
}
