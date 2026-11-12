import { FontSizeSchema } from "../../contracts/schemas";
import { deviceNumberPreference } from "../shared/device-preferences.svelte";

// Screens differ in size, so the font is a per-browser preference and never
// syncs through server settings. Every pane and the settings page share this
// module state; an unset or invalid value falls back to the device default.
export const minFontSize = 8;
export const maxFontSize = 20;

const defaultFontSize = () =>
  matchMedia("(pointer: coarse)").matches ? 15 : 14;
const clampFontSize = (value: number) =>
  Math.max(minFontSize, Math.min(maxFontSize, Math.round(value)));

const size = deviceNumberPreference("fontSize", FontSizeSchema);

export const terminalFont = {
  // Effective size: the saved value or the pointer-based default.
  get size() {
    return size.value ?? defaultFontSize();
  },
  set(value: number) {
    size.set(clampFontSize(value));
  },
  reset: size.reset,
};
