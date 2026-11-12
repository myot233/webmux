import { describe, expect, it } from "vitest";
import { formatBytes, hostStrain } from "../src/domains/sessions/host-health";

const health = (one: number, cpus = 4) => ({
  ok: true as const,
  sessions: 0,
  load: { one, five: one, fifteen: one, cpus },
});

describe("hostStrain", () => {
  it("is quiet for a fast, idle host", () => {
    expect(hostStrain(200, health(2))).toBe("");
  });
  it("flags latency above 200 ms only", () => {
    expect(hostStrain(201, null)).not.toBe("");
    expect(hostStrain(null, null)).toBe("");
  });
  it("flags one-minute load above half the cores", () => {
    expect(hostStrain(10, health(2.01))).not.toBe("");
    expect(hostStrain(10, health(2))).toBe("");
  });
  it("ignores load when the host reports none", () => {
    expect(hostStrain(10, { ok: true, sessions: 0 })).toBe("");
  });
});

describe("formatBytes", () => {
  it("scales to the largest fitting unit", () => {
    expect(formatBytes(512)).toBe("512 B");
    expect(formatBytes(4 * 1024 ** 3)).toBe("4.0 GB");
  });
});
