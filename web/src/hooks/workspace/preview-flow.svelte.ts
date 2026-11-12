import { isStale } from "../../domains/workspace/branch-operations";
import { errorMessage } from "../../platform/error";
import { LatestRequest } from "../shared/remote-request.svelte";

/**
 * The state of a preview-then-confirm flow. Only the latest preview counts:
 * a newer one aborts the request it replaces and late replies are dropped.
 * A failed execution previews again, since a confirmation is used up and
 * the state may have moved; a 409 says what changed.
 */
export class PreviewFlow<P> {
  preview = $state.raw<P | null>(null);
  loading = $state(false);
  running = $state(false);
  /** The preview could not be read. */
  error = $state("");
  /** The previewed state changed before the execution. */
  stale = $state("");
  /** The execution failed for another reason. */
  failure = $state("");
  #requests = new LatestRequest();
  #load: (signal: AbortSignal) => Promise<P>;

  constructor(load: (signal: AbortSignal) => Promise<P>) {
    this.#load = load;
  }

  async refresh() {
    const request = this.#requests.begin();
    this.loading = true;
    this.error = "";
    try {
      const preview = await this.#load(request.signal);
      if (request.current()) this.preview = preview;
    } catch (err) {
      if (request.reportable()) {
        this.preview = null;
        this.error = errorMessage(err);
      }
    } finally {
      if (request.current()) this.loading = false;
    }
  }

  /** Drops the shown preview, for example while its inputs are edited. */
  invalidate() {
    this.#requests.cancel();
    this.preview = null;
    this.loading = false;
    this.error = "";
  }

  /** Runs the confirmed execution; undefined when it failed. */
  async run<R>(execute: () => Promise<R>): Promise<R | undefined> {
    this.#requests.cancel();
    this.loading = false;
    this.running = true;
    this.stale = "";
    this.failure = "";
    try {
      return await execute();
    } catch (err) {
      if (isStale(err)) this.stale = errorMessage(err);
      else this.failure = errorMessage(err);
      void this.refresh();
      return undefined;
    } finally {
      this.running = false;
    }
  }

  dispose() {
    this.#requests.dispose();
  }
}
