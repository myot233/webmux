import { deviceToggle } from "./device-preferences.svelte";

// The preference is device-wide, so every mounted pane reads and writes the
// same module state instead of loading its own copy from localStorage.
export const keyboardSizeHold = deviceToggle("keyboardSizeHold", true);
