import { TextSettingSchema } from "../contracts/schemas";

const hostIdPattern = /^[A-Za-z0-9_-]{1,64}$/;

/**
 * Browser-local UI and input preferences shared across hosts on the same device.
 * Every other key defaults to host-scoped isolation when a `hostId` is active.
 */
export const deviceScopedKeys: ReadonlySet<string> = new Set([
  "shellMode",
  "sidebarCollapsed",
  "sidebarWidth",
  "homeSessionView",
  "quickStartSide",
  "codePanelShown",
  "codePanelSize",
  "sizeLocked",
  "wsCompression",
  "toastNotifications",
  "fontSize",
  "codeFontSize",
  "keyboardSizeHold",
  "touchGamepad",
  "voiceInput",
]);

let currentHostId: string | null = null;
const hostListeners = new Set<(hostId: string | null) => void>();

/** Validates a non-default host identifier so keys never collide with flat names. */
export function validateHostId(hostId: string): string {
  if (!hostIdPattern.test(hostId)) {
    throw new Error(`Invalid hostId "${hostId}"`);
  }
  return hostId;
}

/** Returns the active host namespace, or null in default same-origin mode. */
export function activeHostId(): string | null {
  return currentHostId;
}

/** Sets the active host namespace (or null for default same-origin mode). */
export function setActiveHostId(hostId: string | null): void {
  const normalized = hostId === null ? null : validateHostId(hostId);
  if (normalized === currentHostId) return;
  currentHostId = normalized;
  for (const listener of hostListeners) listener(currentHostId);
}

/** Subscribes to active host namespace changes. */
export function onActiveHostChange(
  listener: (hostId: string | null) => void,
): () => void {
  hostListeners.add(listener);
  return () => {
    hostListeners.delete(listener);
  };
}

/**
 * Storage prefix for host-scoped records (`"webmux."` in default same-origin
 * mode, `"webmux.host.<hostId>."` when a host namespace is active).
 */
export function hostNamespacePrefix(
  hostId: string | null = currentHostId,
): string {
  return hostId === null ? "webmux." : `webmux.host.${validateHostId(hostId)}.`;
}

/** Full localStorage key for an explicitly host-scoped logical key. */
export function hostStorageKey(
  key: string,
  hostId: string | null = currentHostId,
): string {
  return hostNamespacePrefix(hostId) + key;
}

/** Full localStorage key accounting for device-scoped vs host-scoped keys. */
export function resolveStorageKey(
  key: string,
  hostId: string | null = currentHostId,
): string {
  return deviceScopedKeys.has(key)
    ? "webmux." + key
    : hostStorageKey(key, hostId);
}

/** Reads a local setting (host-scoped unless listed in `deviceScopedKeys`). */
export const stored = (key: string, fallback = ""): string => {
  try {
    return TextSettingSchema.parse(
      localStorage.getItem(resolveStorageKey(key)) ?? fallback,
    );
  } catch {
    return fallback;
  }
};

/** Writes a local setting, or removes its key when the value is null. */
export function store(key: string, value: string | null): void {
  const storageKey = resolveStorageKey(key);
  try {
    if (value === null) localStorage.removeItem(storageKey);
    else localStorage.setItem(storageKey, value);
  } catch {
    /* private-mode storage is optional */
  }
}

/**
 * Reports whether `localStorage` is reachable and currently has no entry for
 * `key` in the active scope. Returns false when storage itself throws.
 */
export function isStoredMissing(key: string): boolean {
  const storageKey = resolveStorageKey(key);
  try {
    return localStorage.getItem(storageKey) === null;
  } catch {
    return false;
  }
}

/**
 * Removes every localStorage entry belonging to `hostId` (credentials, caches,
 * workspace memories, drafts and scratch notes) without touching other hosts,
 * default same-origin root keys, or device preferences.
 */
export function clearHostStorage(
  hostId: string,
  target?: Storage | null,
): void {
  const prefix = hostNamespacePrefix(validateHostId(hostId));
  let area: Storage | null;
  try {
    area = target === undefined ? localStorage : target;
  } catch {
    return;
  }
  if (!area) return;
  try {
    const keysToRemove: string[] = [];
    for (let i = 0; i < area.length; i++) {
      const key = area.key(i);
      if (key?.startsWith(prefix)) keysToRemove.push(key);
    }
    for (const key of keysToRemove) area.removeItem(key);
  } catch {
    /* private-mode storage is optional */
  }
}
