import { afterEach, describe, expect, it, vi } from "vitest";
import {
  dismissUpdate,
  dismissedUpdate,
  homeUpdate,
  restartMethod,
} from "../src/platform/pending-update";
import { ConfigSchema } from "../src/contracts/schemas";

const baseConfig = {
  defaultCwd: "/home/me",
  portDetection: true,
  workspace: {
    gitAvailable: true,
    gitError: "",
    registryAvailable: true,
    registryError: "",
  },
};

function memoryStorage() {
  const values: Record<string, string> = {};
  vi.stubGlobal("localStorage", {
    getItem: (key: string) => values[key] ?? null,
    setItem: (key: string, value: string) => {
      values[key] = value;
    },
  });
  return values;
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("pending update", () => {
  it("shows nothing when an older server omits the field", () => {
    memoryStorage();
    const config = ConfigSchema.parse(baseConfig);
    expect(config.pendingUpdate).toBeUndefined();
    expect(homeUpdate(config.pendingUpdate, dismissedUpdate())).toBeUndefined();
  });

  it("gives the restart method of each launcher", () => {
    expect(restartMethod("systemd")).toEqual({
      command: "systemctl --user restart webmux",
    });
    expect(restartMethod("launchd")).toEqual({
      command: "webmux service restart",
    });
    expect(restartMethod("windows-tray")).toEqual({
      text: "在托盘菜单中重启",
    });
    expect(restartMethod("manual")).toEqual({ text: "停止后重新运行 webmux" });
  });

  it("treats an unknown launcher as a manual start", () => {
    const config = ConfigSchema.parse({
      ...baseConfig,
      pendingUpdate: { installedVersion: "0.5.0", launcher: "future" },
    });
    expect(config.pendingUpdate).toEqual({
      installedVersion: "0.5.0",
      launcher: "manual",
    });
  });

  it("shows a newer version again after a dismissal", () => {
    const values = memoryStorage();
    const first = ConfigSchema.parse({
      ...baseConfig,
      pendingUpdate: { installedVersion: "0.5.0", launcher: "systemd" },
    }).pendingUpdate;
    expect(homeUpdate(first, dismissedUpdate())).toEqual(first);

    dismissUpdate("0.5.0");
    expect(values["webmux.dismissedUpdate"]).toBe("0.5.0");
    expect(homeUpdate(first, dismissedUpdate())).toBeUndefined();

    const next = { installedVersion: "0.6.0", launcher: "systemd" } as const;
    expect(homeUpdate(next, dismissedUpdate())).toEqual(next);
  });

  it("ignores an invalid stored dismissal", () => {
    const values = memoryStorage();
    values["webmux.dismissedUpdate"] = "x".repeat(65);
    expect(dismissedUpdate()).toBe("");
  });
});
