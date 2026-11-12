import { onMount } from "svelte";
import { api } from "../../platform/api";
import { errorMessage } from "../../platform/error";
import {
  VoiceSettingsSchema,
  VoiceSettingsUpdateSchema,
  type VoiceSettings,
  type VoiceSettingsUpdate,
} from "../../contracts/schemas";

/** Reactive view of the voice recognizer settings; mutate via the hook. */
export interface VoiceSettingsState {
  view: VoiceSettings | undefined;
  busy: boolean;
  error: string;
  notice: string;
  // Bumps when the form must re-read the server values.
  editVersion: number;
}

export function useVoiceSettings() {
  const state = $state<VoiceSettingsState>({
    view: undefined,
    busy: false,
    error: "",
    notice: "",
    editVersion: 0,
  });
  let mounted = true;

  // One request at a time; a late reply after unmount is dropped.
  async function run(method: string, body?: unknown) {
    state.busy = true;
    state.notice = "";
    try {
      const next = await api(
        "/voice/settings",
        VoiceSettingsSchema,
        method,
        body,
      );
      if (!mounted) return false;
      state.view = next;
      state.error = "";
      state.editVersion++;
      return true;
    } catch (cause) {
      if (mounted) state.error = errorMessage(cause);
      return false;
    } finally {
      if (mounted) state.busy = false;
    }
  }

  async function mutate(method: "PUT" | "DELETE", body?: unknown) {
    if (state.busy) return;
    if (await run(method, body))
      state.notice =
        method === "PUT"
          ? "已保存，下一次录音起生效。"
          : "已恢复为配置文件中的设置。";
  }

  onMount(() => {
    void run("GET");
    return () => {
      mounted = false;
    };
  });

  return {
    state,
    save: (draft: VoiceSettingsUpdate) =>
      mutate("PUT", VoiceSettingsUpdateSchema.parse(draft)),
    reset: () => mutate("DELETE"),
  };
}
