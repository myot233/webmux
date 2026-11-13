import { afterEach, expect, test, vi } from "vitest";
import { page, userEvent } from "vitest/browser";
import { cleanup, render } from "vitest-browser-svelte/pure";
import SessionSwitcher from "../../src/components/shell/SessionSwitcher.svelte";
import type {
  ChatMessageMatch,
  SessionInfo,
} from "../../src/contracts/schemas";

afterEach(async () => {
  try {
    await cleanup();
    await userEvent.cleanup();
  } finally {
    vi.unstubAllGlobals();
  }
});

function makeSession(overrides: Partial<SessionInfo>): SessionInfo {
  return {
    id: "abcdefgh",
    generation: "0123456789abcdef",
    title: "默认会话",
    cwd: "/work",
    cmd: ["omp"],
    status: "waiting",
    exitCode: null,
    listeningPorts: [],
    state: "active",
    views: ["terminal", "chat"],
    view: "chat",
    transition: null,
    agent: null,
    preview: "",
    createdAt: "2026-10-16T00:00:00Z",
    lastActiveAt: "2026-10-16T00:00:01Z",
    cols: 80,
    rows: 24,
    clients: 0,
    workspace: null,
    attention: null,
    reportedCwd: null,
    ...overrides,
  };
}

test("session title matches stay above message matches and preserve backend message order", async () => {
  const sessions: SessionInfo[] = [
    makeSession({
      id: "sess2222",
      title: "user auth fix",
      view: "terminal",
      views: ["terminal"],
    }),
    makeSession({ id: "sess3333", title: "对话排查", view: "chat" }),
    makeSession({
      id: "sess4444",
      title: "retired user shell",
      status: "exited",
      exitCode: 0,
      view: "terminal",
      views: ["terminal"],
    }),
  ];
  const matches: ChatMessageMatch[] = [
    {
      sessionId: "sess3333",
      messageId: "m-newer",
      role: "agent",
      excerpt: "最新回复提到 user token",
      at: "2026-10-16T00:00:03Z",
    },
    {
      sessionId: "sess3333",
      messageId: "m-older",
      role: "user",
      excerpt: "较早提问提到 user token",
      at: "2026-10-16T00:00:02Z",
    },
  ];
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof globalThis.fetch>(async (input) => {
      const url = String(input);
      if (url.includes("/sessions")) return Response.json(sessions);
      if (url.includes("/chat/search")) return Response.json(matches);
      return Response.json([]);
    }),
  );

  const onselect = vi.fn();
  render(SessionSwitcher, {
    open: true,
    onselect,
    oncancel: () => {},
  });

  const dialog = page.getByRole("dialog", { name: "快速切换会话" });
  const combobox = dialog.getByRole("combobox");
  await combobox.fill("user");

  await expect
    .element(
      dialog.getByRole("option", { name: /Agent · 最新回复提到 user token/ }),
    )
    .toBeVisible();

  await vi.waitFor(() => {
    const options = dialog.getByRole("option").elements();
    expect(
      options.map((el) => el.textContent?.replace(/\s+/g, " ").trim()),
    ).toEqual([
      expect.stringContaining("user auth fix"),
      expect.stringContaining("Agent · 最新回复提到 user token"),
      expect.stringContaining("你 · 较早提问提到 user token"),
      expect.stringContaining("retired user shell"),
    ]);
    expect(options[0]?.getAttribute("aria-selected")).toBe("true");
  });

  await dialog
    .getByRole("option", { name: /Agent · 最新回复提到 user token/ })
    .click();
  await vi.waitFor(() =>
    expect(onselect).toHaveBeenCalledWith("sess3333", "m-newer"),
  );
});
