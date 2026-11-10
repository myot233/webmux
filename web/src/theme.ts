import { settings, changeSettings, observeSettings } from "./settings.svelte";
import { type Theme } from "./schemas";

const system = matchMedia("(prefers-color-scheme: light)");
const reducedMotion = matchMedia("(prefers-reduced-motion: reduce)");
let choice: Theme | undefined;
let transition: ViewTransition | undefined;
let pendingTheme: Theme | undefined;

function applyTheme() {
  const dark = choice ? choice === "dark" : !system.matches;
  const root = document.documentElement;
  root.classList.toggle("dark", dark);
  const meta = document.querySelector<HTMLMetaElement>(
    'meta[name="theme-color"]',
  );
  if (meta)
    meta.content = getComputedStyle(root)
      .getPropertyValue("--background")
      .trim();
}

export function initializeTheme() {
  observeSettings(() => {
    choice = settings.values.theme ?? undefined;
    applyTheme();
  });
  system.addEventListener("change", () => {
    if (!choice) applyTheme();
  });
}

function commitTheme() {
  if (!pendingTheme) return;
  const theme = pendingTheme;
  pendingTheme = undefined;
  changeSettings({ theme });
}

export function toggleTheme(event: MouseEvent) {
  const root = document.documentElement;
  // Another click can arrive before the browser runs the first update.
  const dark = pendingTheme
    ? pendingTheme === "dark"
    : root.classList.contains("dark");
  pendingTheme = dark ? "light" : "dark";

  const button = event.currentTarget;
  if (
    transition ||
    !document.startViewTransition ||
    reducedMotion.matches ||
    !(button instanceof HTMLElement)
  ) {
    transition?.skipTransition();
    commitTheme();
    return;
  }

  const { left, top, width, height } = button.getBoundingClientRect();
  const x = left + width / 2;
  const y = top + height / 2;
  const radius = Math.hypot(
    Math.max(x, innerWidth - x),
    Math.max(y, innerHeight - y),
  );
  root.style.setProperty("--theme-reveal-origin", `${x}px ${y}px`);
  root.style.setProperty("--theme-reveal-radius", `${radius}px`);
  root.dataset.themeTransition = "";
  transition = document.startViewTransition(commitTheme);
  // A skipped transition still runs the update callback, but its ready promise rejects.
  void transition.ready.catch(() => undefined);
  const cleanup = () => {
    transition = undefined;
    delete root.dataset.themeTransition;
    root.style.removeProperty("--theme-reveal-origin");
    root.style.removeProperty("--theme-reveal-radius");
  };
  void transition.finished.then(cleanup, cleanup);
}
