import { onMount } from "svelte";
import { api } from "../../platform/api";
import { errorMessage } from "../../platform/error";
import { refreshP2PAfterSettingsChange } from "../../platform/p2p.svelte";
import {
  P2PSettingsUpdateSchema,
  P2PStatusSchema,
  type P2PSettingsUpdate,
  type P2PStatus,
} from "../../contracts/schemas";
import { LatestRequest } from "../shared/remote-request.svelte";

const PORTMAP_POLL_MS = 2000;

export interface P2PSettingsController {
  status: P2PStatus | undefined;
  saving: boolean;
  failure: string;
  feedback: string;
  revision: number;
}

export function useP2PSettings() {
  const ui = $state<P2PSettingsController>({
    status: undefined,
    saving: false,
    failure: "",
    feedback: "",
    revision: 0,
  });
  const requests = new LatestRequest();
  let pollTimer: ReturnType<typeof setTimeout> | undefined;

  function schedulePortMapPoll(next: P2PStatus | undefined) {
    clearTimeout(pollTimer);
    if (next?.portMap.state === "probing" && !requests.disposed) {
      pollTimer = setTimeout(() => void refresh(), PORTMAP_POLL_MS);
    }
  }

  async function refresh() {
    const ticket = requests.begin();
    try {
      const next = await api(
        "/p2p",
        P2PStatusSchema,
        "GET",
        undefined,
        ticket.signal,
      );
      if (!ticket.current()) return;
      if (!ui.status) ui.revision++;
      ui.status = next;
      ui.failure = "";
      refreshP2PAfterSettingsChange(next);
      schedulePortMapPoll(next);
    } catch (cause) {
      if (ticket.reportable()) ui.failure = errorMessage(cause);
    }
  }

  async function applyChange(
    method: "PUT" | "DELETE",
    payload?: P2PSettingsUpdate,
  ) {
    if (ui.saving || requests.disposed) return false;
    ui.saving = true;
    ui.failure = "";
    ui.feedback = "";
    const ticket = requests.begin();
    try {
      const next = await api(
        "/p2p/settings",
        P2PStatusSchema,
        method,
        payload,
        ticket.signal,
      );
      if (requests.disposed) return false;
      ui.status = next;
      ui.revision++;
      refreshP2PAfterSettingsChange(next);
      schedulePortMapPoll(next);
      ui.feedback =
        method === "DELETE" ? "已恢复配置文件默认。" : "P2P 设置已保存。";
      return true;
    } catch (cause) {
      if (!requests.disposed) ui.failure = errorMessage(cause);
      return false;
    } finally {
      if (!requests.disposed) ui.saving = false;
    }
  }

  onMount(() => {
    void refresh();
    return () => {
      requests.dispose();
      clearTimeout(pollTimer);
    };
  });

  return {
    ui,
    refresh,
    save: (draft: P2PSettingsUpdate) =>
      applyChange("PUT", P2PSettingsUpdateSchema.parse(draft)),
    reset: () => applyChange("DELETE"),
  };
}
