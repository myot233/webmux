import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { store as apiStore, stored as apiStored } from "../src/platform/api";
import { quickStartMemory } from "../src/hooks/sessions/quick-start";
import {
  activeHostId,
  clearHostStorage,
  deviceScopedKeys,
  hostNamespacePrefix,
  hostStorageKey,
  isStoredMissing,
  onActiveHostChange,
  resolveStorageKey,
  setActiveHostId,
  store,
  stored,
  validateHostId,
} from "../src/platform/storage";

class MemoryStorage implements Storage {
  data = new Map<string, string>();
  get length() {
    return this.data.size;
  }
  clear() {
    this.data.clear();
  }
  getItem(key: string) {
    return this.data.get(key) ?? null;
  }
  key(index: number) {
    return [...this.data.keys()][index] ?? null;
  }
  removeItem(key: string) {
    this.data.delete(key);
  }
  setItem(key: string, value: string) {
    this.data.set(key, value);
  }
}

describe("host-scoped browser storage", () => {
  let memory: MemoryStorage;

  beforeEach(() => {
    memory = new MemoryStorage();
    vi.stubGlobal("localStorage", memory);
    setActiveHostId(null);
  });

  afterEach(() => {
    setActiveHostId(null);
    vi.unstubAllGlobals();
  });

  test("preserves exact flat webmux.* keys in default same-origin mode", () => {
    memory.setItem("webmux.token", "legacy-token");
    memory.setItem("webmux.settingsCache", '{"revision":1}');
    memory.setItem("webmux.fontSize", "15");

    expect(activeHostId()).toBeNull();
    expect(hostNamespacePrefix()).toBe("webmux.");
    expect(apiStored("token")).toBe("legacy-token");
    expect(stored("settingsCache")).toBe('{"revision":1}');
    expect(stored("fontSize")).toBe("15");

    apiStore("recentDirs", '["/work"]');
    expect(memory.getItem("webmux.recentDirs")).toBe('["/work"]');
    apiStore("recentDirs", null);
    expect(memory.getItem("webmux.recentDirs")).toBeNull();
  });

  test("isolates host-scoped keys per hostId while sharing device preferences", () => {
    const observed: Array<string | null> = [];
    const unsubscribe = onActiveHostChange((id) => observed.push(id));

    setActiveHostId("host-a");
    store("token", "token-a");
    store("homeRepository", "repo-a");
    store("hybridStartSide:cfg1", "chat");
    store("fontSize", "16");
    store("sidebarWidth", "280");

    setActiveHostId("host-b");
    expect(stored("token")).toBe("");
    expect(stored("homeRepository")).toBe("");
    expect(stored("hybridStartSide:cfg1")).toBe("");
    expect(stored("fontSize")).toBe("16");
    expect(stored("sidebarWidth")).toBe("280");

    store("token", "token-b");
    store("homeRepository", "repo-b");

    expect(memory.getItem("webmux.host.host-a.token")).toBe("token-a");
    expect(memory.getItem("webmux.host.host-b.token")).toBe("token-b");
    expect(memory.getItem("webmux.fontSize")).toBe("16");
    expect(hostStorageKey("diffReview", "host-a")).toBe(
      "webmux.host.host-a.diffReview",
    );
    expect(resolveStorageKey("fontSize", "host-a")).toBe("webmux.fontSize");
    expect(deviceScopedKeys.has("wsCompression")).toBe(true);
    expect(deviceScopedKeys.has("token")).toBe(false);

    unsubscribe();
    setActiveHostId(null);
    expect(observed).toEqual(["host-a", "host-b"]);
  });

  test("rejects malformed hostId values before touching storage", () => {
    for (const invalid of [
      "",
      " ",
      "host.a",
      "host/a",
      "host a",
      "host:a",
      "x".repeat(65),
    ]) {
      expect(() => validateHostId(invalid)).toThrow();
      expect(() => setActiveHostId(invalid)).toThrow();
      expect(() => clearHostStorage(invalid, memory)).toThrow();
    }
    expect(memory.length).toBe(0);
  });

  test("clears one host namespace without touching sibling hosts or device settings", () => {
    memory.setItem("webmux.token", "root-token");
    memory.setItem("webmux.fontSize", "15");
    memory.setItem("webmux.host.host-1.token", "t1");
    memory.setItem("webmux.host.host-1.draft.s1", '{"text":"hi"}');
    memory.setItem("webmux.host.host-1.scratch.s1", '{"entries":[]}');
    memory.setItem("webmux.host.host-10.token", "t10");

    clearHostStorage("host-1");

    expect(memory.getItem("webmux.host.host-1.token")).toBeNull();
    expect(memory.getItem("webmux.host.host-1.draft.s1")).toBeNull();
    expect(memory.getItem("webmux.host.host-1.scratch.s1")).toBeNull();
    expect(memory.getItem("webmux.host.host-10.token")).toBe("t10");
    expect(memory.getItem("webmux.token")).toBe("root-token");
    expect(memory.getItem("webmux.fontSize")).toBe("15");
  });

  test("degrades gracefully when localStorage throws", () => {
    vi.stubGlobal("localStorage", {
      getItem() {
        throw new Error("SecurityError");
      },
      setItem() {
        throw new Error("QuotaExceededError");
      },
      removeItem() {
        throw new Error("SecurityError");
      },
      get length() {
        throw new Error("SecurityError");
      },
    });

    setActiveHostId("host-a");
    expect(stored("token", "fallback")).toBe("fallback");
    expect(isStoredMissing("quick-start-draft")).toBe(false);
    expect(() => store("token", "secret")).not.toThrow();
    expect(() => store("token", null)).not.toThrow();
    expect(() => clearHostStorage("host-a")).not.toThrow();
  });

  test("scopes quickStartMemory drafts and in-memory fallbacks per hostId", () => {
    setActiveHostId("host-a");
    quickStartMemory.saveDraft("host A task");
    setActiveHostId("host-b");
    expect(quickStartMemory.draft()).toBe("");
    quickStartMemory.saveDraft("host B task");

    expect(memory.getItem("webmux.host.host-a.quick-start-draft")).toContain(
      "host A task",
    );
    expect(memory.getItem("webmux.host.host-b.quick-start-draft")).toContain(
      "host B task",
    );

    // Clearing host-a's entry externally resets only host-a's memory fallback.
    memory.removeItem("webmux.host.host-a.quick-start-draft");
    setActiveHostId("host-a");
    expect(quickStartMemory.draft()).toBe("");
    setActiveHostId("host-b");
    expect(quickStartMemory.draft()).toBe("host B task");
    quickStartMemory.clearDraft();
    expect(memory.getItem("webmux.host.host-b.quick-start-draft")).toBeNull();
  });
});
