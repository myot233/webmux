import { deviceToggle } from "../shared/device-preferences.svelte";

// Device-local preference for the floating touch gamepad overlay.
// Every mounted terminal pane shares the same reactive module state.
export const touchGamepad = deviceToggle("touchGamepad", false);
