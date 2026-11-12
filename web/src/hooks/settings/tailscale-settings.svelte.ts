import { errorMessage } from "../../platform/error";
import { onMount } from "svelte";
import { api } from "../../platform/api";
import { LatestRequest } from "../shared/remote-request.svelte";
import {
  TailscaleSettingsUpdateSchema,
  TailscaleStatusSchema,
  type TailscaleSettingsUpdate,
  type TailscaleStatus,
} from "../../contracts/schemas";

// Applying a Tailscale entry runs several CLI calls on the server; poll only
// while the controller reports "starting" so an idle page spawns no probes.
const STARTING_POLL_MS = 1500;
const APPLYING = "已保存，正在应用 Tailscale 设置。";

/** Reactive view of the Tailscale settings page; mutate only via the hook. */
export interface TailscaleSettingsState {
  view: TailscaleStatus | undefined;
  loading: boolean;
  busy: boolean;
  error: string;
  notice: string;
  // Bumps when the form must re-read the server values.
  editVersion: number;
}

export function useTailscaleSettings() {
  const state = $state<TailscaleSettingsState>({
    view: undefined,
    loading: true,
    busy: false,
    error: "",
    notice: "",
    editVersion: 0,
  });
  const requests = new LatestRequest();
  let timer: ReturnType<typeof setTimeout> | undefined;

  // Every response supersedes older ones.
  async function exchange(
    path: string,
    method = "GET",
    body?: unknown,
  ): Promise<TailscaleStatus | undefined> {
    const ticket = requests.begin();
    clearTimeout(timer);
    try {
      const next = await api(
        path,
        TailscaleStatusSchema,
        method,
        body,
        ticket.signal,
      );
      if (!ticket.current()) return undefined;
      state.view = next;
      state.error = "";
      return next;
    } catch (cause) {
      if (ticket.reportable()) state.error = errorMessage(cause);
      throw cause;
    } finally {
      if (ticket.current()) {
        state.loading = false;
        if (state.view?.state === "starting")
          timer = setTimeout(() => void load(), STARTING_POLL_MS);
        else if (state.notice === APPLYING) state.notice = "设置已应用。";
      }
    }
  }

  async function load(refresh = false) {
    const first = !state.view;
    try {
      await exchange(refresh ? "/tailscale?refresh=1" : "/tailscale");
      if (first && state.view) state.editVersion++;
    } catch {
      // exchange already reported the failure.
    }
  }

  async function mutate(method: "PUT" | "DELETE", body?: unknown) {
    if (state.busy || requests.disposed) return false;
    state.busy = true;
    state.notice = "";
    try {
      const next = await exchange("/tailscale/settings", method, body);
      if (!next) return false;
      state.editVersion++;
      state.notice = next.state === "starting" ? APPLYING : "设置已保存。";
      return true;
    } catch {
      return false;
    } finally {
      if (!requests.disposed) state.busy = false;
    }
  }

  onMount(() => {
    void load();
    return () => {
      requests.dispose();
      clearTimeout(timer);
    };
  });

  return {
    state,
    load,
    save: (draft: TailscaleSettingsUpdate) =>
      mutate("PUT", TailscaleSettingsUpdateSchema.parse(draft)),
    reset: () => mutate("DELETE"),
  };
}
