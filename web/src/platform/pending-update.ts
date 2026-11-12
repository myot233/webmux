import { z } from "zod";
import { store, stored } from "./api";
import type { PendingUpdate } from "../contracts/schemas";

// How the user restarts WebMux for the recorded launcher. Restarting is never
// offered as a button: it ends every session.
export type RestartMethod = { command: string } | { text: string };

export function restartMethod(
  launcher: PendingUpdate["launcher"],
): RestartMethod {
  switch (launcher) {
    case "systemd":
      return { command: "systemctl --user restart webmux" };
    case "launchd":
      return { command: "webmux service restart" };
    case "windows-tray":
      return { text: "在托盘菜单中重启" };
    case "manual":
      return { text: "停止后重新运行 webmux" };
  }
}

// The home page notice dismissal is device-local and remembers one version,
// so a newer installed version shows the notice again.
const dismissedKey = "dismissedUpdate";
const DismissedUpdateSchema = z.string().min(1).max(64);

export function dismissedUpdate(): string {
  return DismissedUpdateSchema.safeParse(stored(dismissedKey)).data ?? "";
}

export function dismissUpdate(version: string) {
  store(dismissedKey, version);
}

// The home page notice: none without a pending update or once its version
// was dismissed. The settings page always shows a pending update.
export function homeUpdate(
  update: PendingUpdate | undefined,
  dismissed: string,
): PendingUpdate | undefined {
  return update && update.installedVersion !== dismissed ? update : undefined;
}
