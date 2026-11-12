import { CodeFontSizeSchema } from "../../contracts/schemas";
import { deviceNumberPreference } from "../shared/device-preferences.svelte";

// Screens differ in size, so the code font is a per-browser preference and
// never syncs through server settings. Every code reader and the settings page
// share this module state; an unset or invalid value falls back to 13px.
export const minCodeFontSize = 10;
export const maxCodeFontSize = 24;
export const defaultCodeFontSize = 13;

const clampCodeFontSize = (value: number) =>
  Math.max(minCodeFontSize, Math.min(maxCodeFontSize, Math.round(value)));

const codeLineHeightFor = (fontSize: number) =>
  Math.round((clampCodeFontSize(fontSize) * 22) / defaultCodeFontSize);

const size = deviceNumberPreference("codeFontSize", CodeFontSizeSchema);

export const codeFont = {
  get size() {
    return size.value ?? defaultCodeFontSize;
  },
  get lineHeight() {
    return codeLineHeightFor(this.size);
  },
  get itemMetrics() {
    return { lineHeight: this.lineHeight };
  },
  set(value: number) {
    size.set(clampCodeFontSize(value));
  },
  reset: size.reset,
};
