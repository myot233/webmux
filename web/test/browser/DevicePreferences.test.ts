import { afterEach, expect, test, vi } from "vitest";
import {
  deviceNumberPreference,
  deviceToggle,
} from "../../src/hooks/shared/device-preferences.svelte";
import { FontSizeSchema } from "../../src/contracts/schemas";

afterEach(() => vi.restoreAllMocks());

test.each([
  ["0", false, false],
  ["1", true, true],
  [null, true, false],
  ["unknown", true, false],
])(
  "reads stored toggle %s with either default",
  (raw, defaultOn, defaultOff) => {
    vi.spyOn(Storage.prototype, "getItem").mockReturnValue(raw);
    expect(deviceToggle("voiceInput", true).enabled).toBe(defaultOn);
    expect(deviceToggle("touchGamepad", false).enabled).toBe(defaultOff);
  },
);

test.each([
  ["14.0", 14],
  ["30", undefined],
])("validates the saved font size %s on read", (raw, value) => {
  vi.spyOn(Storage.prototype, "getItem").mockReturnValue(raw);
  expect(deviceNumberPreference("fontSize", FontSizeSchema).value).toBe(value);
});

test("updates device preferences in memory when browser storage fails", () => {
  const unavailable = () => {
    throw new DOMException("Storage disabled", "SecurityError");
  };
  vi.spyOn(Storage.prototype, "getItem").mockImplementation(unavailable);
  const set = vi
    .spyOn(Storage.prototype, "setItem")
    .mockImplementation(unavailable);
  const remove = vi
    .spyOn(Storage.prototype, "removeItem")
    .mockImplementation(unavailable);

  const font = deviceNumberPreference("fontSize", FontSizeSchema);
  expect(font.value).toBeUndefined();
  font.set(17);
  expect(font.value).toBe(17);
  expect(set).toHaveBeenCalledWith("webmux.fontSize", "17");
  font.reset();
  expect(font.value).toBeUndefined();
  expect(remove).toHaveBeenCalledWith("webmux.fontSize");

  const voice = deviceToggle("voiceInput", true);
  expect(voice.enabled).toBe(true);
  voice.enabled = false;
  expect(voice.enabled).toBe(false);
  expect(set).toHaveBeenCalledWith("webmux.voiceInput", "0");
});
