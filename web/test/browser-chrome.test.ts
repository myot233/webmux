import { expect, test } from "vitest";
import { chromeTitle } from "../src/browser-chrome";

test("running tabs keep the title and add the marker", () => {
  expect(chromeTitle("WebMux · 工作区", false)).toBe("WebMux · 工作区");
  expect(chromeTitle("WebMux · 工作区", true)).toBe("▶ WebMux · 工作区");
  expect(chromeTitle("fixture · WebMux", true)).toBe("▶ fixture · WebMux");
});
