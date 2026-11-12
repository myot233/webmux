// Route matching for the single-page app. It only looks at the pathname, so
// pages that read `location.search` or `#hash` keep their query intact.
export type Route =
  | { name: "terminal"; id: string }
  | { name: "code"; id: string }
  | { name: "handoff"; sourceId: string }
  | { name: "settings" }
  | { name: "settings-tailscale" }
  | { name: "settings-tunnel" }
  // Exchanges a ticket for the port-forward gateway cookie, then leaves.
  | { name: "forward-auth" }
  // Chat mode's home: write a task and start an agent in a new worktree.
  | { name: "quick-start" }
  | { name: "sessions" };

// Unknown paths fall back to the session list; there is no 404 page.
export function matchRoute(pathname: string): Route {
  const session = /^\/s\/([a-z2-7]{8})$/.exec(pathname);
  if (session?.[1]) return { name: "terminal", id: session[1] };
  const handoff = /^\/handoff\/([a-f0-9]{32})$/.exec(pathname);
  if (handoff?.[1]) return { name: "handoff", sourceId: handoff[1] };
  const code = /^\/code\/([a-z2-7]{8})$/.exec(pathname);
  if (code?.[1]) return { name: "code", id: code[1] };
  if (pathname === "/settings/tailscale") return { name: "settings-tailscale" };
  if (pathname === "/settings/tunnel") return { name: "settings-tunnel" };
  if (pathname === "/forward-auth") return { name: "forward-auth" };
  if (pathname === "/new") return { name: "quick-start" };
  if (pathname === "/settings") return { name: "settings" };
  return { name: "sessions" };
}

// The gateway address `/forward-auth?next=` returns to, or null. Like the
// server, it admits only paths under /forward/, so the page never sends a
// ticket toward another origin.
export function forwardAuthNext(search: string): string | null {
  const next = new URLSearchParams(search).get("next");
  if (!next?.startsWith("/forward/") || /\/\/|[\\\r\n]/.test(next)) return null;
  return next;
}
