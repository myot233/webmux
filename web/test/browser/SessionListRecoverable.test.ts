import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { page, userEvent } from "vitest/browser";
import { cleanup, render } from "vitest-browser-svelte/pure";
import SessionList from "../../src/pages/SessionList.svelte";
import { createTerminalDrafts } from "../../src/terminal-draft.svelte";
import type { HybridRecord, SessionInfo } from "../../src/schemas";

const record: HybridRecord = {
  id: "hybrid22",
  name: "修复排序",
  configId: "cfgomp01",
  configMissing: false,
  agent: "omp",
  cwd: "/work",
  cmd: ["omp"],
  side: "tui",
  agentSessionId: "01a",
  previousIds: [],
  state: "closed",
  queuedCount: 0,
  createdAt: "2026-10-28T00:00:00Z",
  updatedAt: "2026-10-28T00:00:00Z",
  interruptedAt: null,
};
const active: SessionInfo = {
  id: "hybrid33",
  generation: "0123456789abcdef",
  title: "进行中",
  cwd: "/work",
  cmd: ["omp"],
  status: "idle",
  exitCode: null,
  preview: "",
  createdAt: "2026-10-28T00:00:00Z",
  lastActiveAt: "2026-10-28T00:00:00Z",
  cols: 80,
  rows: 24,
  clients: 0,
  workspace: null,
  attention: null,
  reportedCwd: null,
  hybrid: {
    side: "acp",
    dormant: false,
    state: "active",
    agent: "omp",
    agentSessionId: "01b",
  },
};

let records: HybridRecord[] = [];
const sockets: FakeSocket[] = [];

class FakeSocket {
  static readonly OPEN = 1;
  readyState = 0;
  bufferedAmount = 0;
  binaryType = "blob";
  onopen?: () => void;
  onmessage?: (event: { data: string }) => void;
  onclose?: (event: { code: number }) => void;
  onerror?: () => void;
  constructor(readonly url: string) {
    sockets.push(this);
  }
  send() {}
  close() {}
  open() {
    this.readyState = 1;
    this.onopen?.();
  }
  message(event: unknown) {
    this.onmessage?.({ data: JSON.stringify(event) });
  }
}

beforeEach(() => {
  records = [record];
  sockets.length = 0;
  vi.stubGlobal("WebSocket", FakeSocket);
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>(async (input, init) => {
      const url = new URL(String(input), location.href);
      const method = init?.method ?? "GET";
      if (url.pathname === "/api/hybrid-sessions" && method === "GET")
        return Response.json(records);
      if (
        url.pathname === "/api/hybrid-sessions/hybrid22" &&
        method === "DELETE"
      ) {
        records = [];
        return new Response(null, { status: 204 });
      }
      // The rest of the home page degrades on its own; not under test.
      return Response.json({ error: "not found" }, { status: 404 });
    }),
  );
});

afterEach(async () => {
  try {
    await cleanup();
    await userEvent.cleanup();
  } finally {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  }
});

function draftsWith(...ids: string[]) {
  const drafts = createTerminalDrafts();
  const entries = ids.map((id) => {
    const entry = drafts.get(id);
    drafts.setEnabled(entry, true);
    drafts.edit(entry, `draft of ${id}`);
    return entry;
  });
  return { drafts, entries };
}

async function connected() {
  await vi.waitFor(() => expect(sockets.length).toBeGreaterThan(0));
  const socket = sockets.find((s) => s.url.endsWith("/ws/sessions"))!;
  socket.open();
  return socket;
}

test("a snapshot keeps drafts of recoverable hybrid sessions and drops the rest", async () => {
  const {
    drafts,
    entries: [recoverable, gone],
  } = draftsWith("hybrid22", "gone0000");
  render(SessionList, { drafts });
  const socket = await connected();
  socket.message({ type: "snapshot", sessions: [] });
  await vi.waitFor(() => expect(drafts.valid(gone!)).toBe(false));
  expect(drafts.valid(recoverable!)).toBe(true);
  await expect
    .element(page.getByRole("region", { name: "可恢复" }))
    .toMatchTextContent(/可恢复\s*1/);
});

test("closing a hybrid session keeps its draft; removing the record deletes it", async () => {
  records = [];
  const {
    drafts,
    entries: [closed],
  } = draftsWith("hybrid33");
  render(SessionList, { drafts });
  const socket = await connected();
  socket.message({ type: "snapshot", sessions: [active] });
  records = [{ ...record, id: "hybrid33", name: "进行中" }];
  socket.message({ type: "removed", id: "hybrid33" });
  const group = page.getByRole("region", { name: "可恢复" });
  await expect.element(group).toMatchTextContent(/可恢复\s*1/);
  expect(drafts.valid(closed!)).toBe(true);

  await group.getByRole("button", { name: /可恢复/ }).click();
  await group.getByRole("button", { name: "移除" }).click();
  // The fetch stub only knows how to delete hybrid22; answer for this one.
  vi.mocked(fetch).mockImplementationOnce(async () => {
    records = [];
    return new Response(null, { status: 204 });
  });
  await page
    .getByRole("alertdialog")
    .getByRole("button", { name: "确认" })
    .click();
  await vi.waitFor(() => expect(drafts.valid(closed!)).toBe(false));
});
