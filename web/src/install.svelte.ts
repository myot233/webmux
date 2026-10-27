// Web app installation state. Chromium fires `beforeinstallprompt` once,
// early and only when the page is installable, so the listener is installed
// at startup and the event kept for the settings button.

// Chromium's install prompt; not in the DOM type library.
interface InstallPromptEvent extends Event {
  prompt(): Promise<void>;
  readonly userChoice: Promise<{ outcome: "accepted" | "dismissed" }>;
}

let deferred: InstallPromptEvent | undefined;

export const installation = $state({
  // A kept install prompt can be shown from a click.
  promptable: false,
  // The browser reported this app installed during this page's lifetime.
  installed: false,
});

export function trackInstallation(target: Window = window) {
  target.addEventListener("beforeinstallprompt", (event) => {
    // The browser's own install promotion stays; the kept event only adds
    // an explicit entry in settings.
    deferred = event as InstallPromptEvent;
    installation.promptable = true;
  });
  target.addEventListener("appinstalled", () => {
    deferred = undefined;
    installation.promptable = false;
    installation.installed = true;
  });
}

// Shows the kept prompt. An event prompts only once; Chromium fires a new one
// if the user dismissed it and the page is still installable.
export async function promptInstall(): Promise<
  "accepted" | "dismissed" | "unavailable"
> {
  const event = deferred;
  deferred = undefined;
  installation.promptable = false;
  if (!event) return "unavailable";
  await event.prompt();
  return (await event.userChoice).outcome;
}

export type InstallPlatform = {
  // HTTPS or a loopback address; other origins get no service worker, push
  // or clipboard, and Chromium will not install them.
  secure: boolean;
  // Running as an installed app (display-mode: standalone or iOS Home Screen).
  standalone: boolean;
  ios: boolean;
};

// What the settings card offers. iOS Safari has no install prompt: it adds to
// the Home Screen from the share sheet, even over plain HTTP.
export type InstallAdvice =
  | "standalone"
  | "installed"
  | "prompt"
  | "ios"
  | "browser-menu";

export function adviseInstall(
  platform: InstallPlatform,
  state: { promptable: boolean; installed: boolean },
): InstallAdvice {
  if (platform.standalone) return "standalone";
  if (state.installed) return "installed";
  if (platform.ios) return "ios";
  if (state.promptable && platform.secure) return "prompt";
  return "browser-menu";
}
