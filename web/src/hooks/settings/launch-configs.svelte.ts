import { api } from "../../platform/api";
import { errorMessage } from "../../platform/error";
import type { LaunchConfigsState } from "../../domains/settings/launch-configs";
import { setLaunchPreviews } from "../../domains/settings/launch-previews.svelte";
import { LaunchConfigsSchema } from "../../contracts/schemas";

/** The saved launch configs, shared by the new-session dialog and quick start. */
export interface LaunchConfigs {
  readonly state: LaunchConfigsState;
  /**
   * Reloads them, e.g. to pick up changes from other devices. `initial`
   * marks the first load, whose failure resets cards to the global preview.
   */
  load(initial?: boolean): Promise<void>;
  dispose(): void;
}

export function createLaunchConfigs(): LaunchConfigs {
  let state = $state.raw<LaunchConfigsState>({ status: "loading" });
  let disposed = false;
  return {
    get state() {
      return state;
    },
    async load(initial = false) {
      if (state.status !== "ready") state = { status: "loading" };
      try {
        const configs = await api("/launch-configs", LaunchConfigsSchema);
        if (disposed) return;
        state = { status: "ready", configs };
        setLaunchPreviews(configs);
      } catch (err) {
        if (disposed) return;
        state = { status: "unavailable", error: errorMessage(err) };
        // Cards keep the last loaded previews while a reload is in flight or
        // fails; only an unavailable store on page load falls back to global.
        if (initial) setLaunchPreviews([]);
      }
    },
    dispose() {
      disposed = true;
    },
  };
}
