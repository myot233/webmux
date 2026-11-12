import type { Health } from "../../contracts/schemas";

/** A health round trip slower than this marks the host as strained. */
export const SLOW_LATENCY_MS = 200;
/** One-minute load above this share of the cores marks the host as busy. */
export const BUSY_LOAD_RATIO = 0.5;

/** Why the host looks strained, or "" when it does not. */
export function hostStrain(latency: number | null, health: Health | null) {
  if (latency !== null && latency > SLOW_LATENCY_MS) return "网络延时偏高";
  const load = health?.load;
  if (load && load.one > load.cpus * BUSY_LOAD_RATIO) return "系统负载偏高";
  return "";
}

const units = ["B", "KB", "MB", "GB", "TB"];

/** Bytes as a short binary-unit size such as "3.9 GB". */
export function formatBytes(bytes: number) {
  let value = bytes;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit++;
  }
  return `${unit === 0 ? value : value.toFixed(1)} ${units[unit]}`;
}
