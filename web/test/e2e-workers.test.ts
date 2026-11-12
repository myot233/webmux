import { describe, expect, test } from "vitest";
import { resolveE2EWorkers } from "../playwright.config";

describe("E2E workers", () => {
  test("scales local workers with available parallelism up to eight", () => {
    expect(resolveE2EWorkers({}, 32)).toBe(8);
    expect(resolveE2EWorkers({}, 4)).toBe(4);
    expect(resolveE2EWorkers({}, 1)).toBe(1);
  });

  test("uses two workers on CI", () => {
    expect(resolveE2EWorkers({ CI: "1" }, 8)).toBe(2);
    expect(resolveE2EWorkers({ CI: "1" }, 1)).toBe(2);
  });

  test("honours WEBMUX_E2E_WORKERS everywhere", () => {
    expect(resolveE2EWorkers({ WEBMUX_E2E_WORKERS: "3" }, 8)).toBe(3);
    expect(resolveE2EWorkers({ CI: "1", WEBMUX_E2E_WORKERS: "1" }, 8)).toBe(1);
  });

  test.each(["0", "1.5", "two"])(
    "rejects invalid WEBMUX_E2E_WORKERS %s",
    (value) => {
      expect(() => resolveE2EWorkers({ WEBMUX_E2E_WORKERS: value })).toThrow(
        "positive integer",
      );
    },
  );
});
