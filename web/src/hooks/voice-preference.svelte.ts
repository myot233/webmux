import { deviceToggle } from "./device-preferences.svelte";

// Whether this browser offers the microphone. Microphone access and the
// wish to dictate differ per device, so this is a per-browser preference and
// never syncs through server settings; it only matters once the server has
// speech recognition configured. On unless the user turned it off.
export const voicePreference = deviceToggle("voiceInput", true);
