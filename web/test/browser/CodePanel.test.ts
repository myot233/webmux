import { afterEach, expect, test, vi } from "vitest";
import { page, userEvent } from "vitest/browser";
import { cleanup, render } from "vitest-browser-svelte/pure";
import Harness from "./CodePanelHarness.svelte";
import { nodeId } from "../../src/domains/code/code-reference";
const now = "2026-10-14T00:00:00Z";
const readerText = () => {
  const host = [
    ...document.querySelectorAll<HTMLElement>("[data-code-reader]"),
  ].find((element) => element.offsetParent !== null);
  return host?.querySelector("diffs-container")?.shadowRoot?.textContent ?? "";
};
function file(n: number, version = "original") {
  return {
    snapshotId: `snapshot-${n}-${version}`,
    checkedAt: now,
    document: {
      nodeId: btoa(`file-${n}.txt`),
      path: `file-${n}.txt`,
      absolutePath: `/root/file-${n}.txt`,
      source: "worktree",
      oid: "",
      contentVersion: version,
      kind: "text",
      text: `${version} file ${n}\n`,
      lineCount: 1,
      referenceable: true,
      reason: "",
      mediaType: "",
      size: `${version} file ${n}\n`.length,
      downloadable: true,
    },
  };
}
let state = "unchanged",
  version = "original",
  checkFailed = false,
  checks = 0;
function serve() {
  state = "unchanged";
  version = "original";
  checkFailed = false;
  checks = 0;
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>(async (input, init) => {
      const url = new URL(String(input), location.href);
      if (url.pathname.endsWith("/context"))
        return Response.json({
          contextId: "root",
          rootPath: "/root",
          rootKind: "directory",
          browseAvailable: true,
          diffAvailable: false,
          ignoreAvailable: false,
          writableAvailable: true,
          reason: "",
        });
      if (url.pathname.endsWith("/tree"))
        return Response.json({
          children: Array.from({ length: 21 }, (_, n) => ({
            nodeId: btoa(`file-${n}.txt`),
            path: `file-${n}.txt`,
            name: `file-${n}.txt`,
            kind: "file",
            gitStatus: "",
            ignored: false,
            referenceable: true,
            mediaType: "",
          })),
          nextCursor: "",
          limited: false,
          complete: true,
          contentVersion: "tree",
          checkedAt: now,
        });
      if (url.pathname.endsWith("/file")) {
        const path = atob(url.searchParams.get("nodeId")!);
        return Response.json(file(Number(path.match(/\d+/)![0]), version));
      }
      if (url.pathname.endsWith("/check")) {
        checks++;
        if (checkFailed)
          return Response.json({ error: "check unavailable" }, { status: 503 });
        const req = JSON.parse(String(init?.body));
        return Response.json({
          snapshots: Object.fromEntries(
            req.snapshotIds.map((id: string) => [id, state]),
          ),
          directories: {},
          changes: "unchanged",
          checkedAt: now,
        });
      }
      throw new Error(`Unexpected ${url}`);
    }),
  );
}
afterEach(async () => {
  await cleanup();
  await userEvent.cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});
test("updates only on refresh, blocks stale references and pauses hidden polling", async () => {
  serve();
  render(Harness);
  await page.getByRole("button", { name: "file-0.txt", exact: true }).click();
  await expect.poll(readerText).toContain("original file 0");
  state = "updated";
  document.dispatchEvent(new Event("visibilitychange"));
  await expect
    .element(
      page.getByText("内容已更新，保留旧内容。刷新文档", { exact: false }),
    )
    .toMatchTextContent(new RegExp("内容已更新"));
  await expect
    .element(page.getByRole("button", { name: "发送位置到终端" }))
    .toBeDisabled();
  await expect.poll(readerText).toContain("original file 0");
  version = "refreshed";
  state = "unchanged";
  await page.getByRole("button", { name: "刷新文档" }).click();
  await expect.poll(readerText).toContain("refreshed file 0");
  await page.getByRole("button", { name: "发送位置到终端" }).click();
  await expect
    .element(page.getByLabelText("Inserted reference"))
    .toMatchTextContent(new RegExp("/root/file-0.txt"));
  checkFailed = true;
  document.dispatchEvent(new Event("visibilitychange"));
  await expect
    .element(page.getByRole("button", { name: "发送位置到终端" }))
    .toBeDisabled();
  await expect.poll(readerText).toContain("refreshed file 0");
  await page.getByRole("button", { name: "Toggle code visibility" }).click();
  const previous = checks;
  document.dispatchEvent(new Event("visibilitychange"));
  await new Promise((resolve) => setTimeout(resolve, 100));
  expect(checks).toBe(previous);
});
test("caps open documents without evicting tabs and reuses existing paths", async () => {
  serve();
  render(Harness);
  for (let n = 0; n < 20; n++) {
    await page
      .getByRole("button", { name: `file-${n}.txt`, exact: true })
      .click();
    await expect
      .element(
        page.getByRole("tab", {
          name: `file-${n}.txt · worktree`,
          exact: true,
        }),
      )
      .toBeInTheDocument();
  }
  await page.getByRole("button", { name: "file-20.txt", exact: true }).click();
  await expect
    .element(page.getByRole("alert"))
    .toMatchTextContent(new RegExp("最多打开 20 个标签"));
  expect(
    document.querySelectorAll(
      '[role="tablist"][aria-label="文件标签"] [role="tab"]',
    ).length,
  ).toBe(20);
  await page.getByRole("button", { name: "file-0.txt", exact: true }).click();
  await expect
    .element(
      page.getByRole("tab", { name: "file-0.txt · worktree", exact: true }),
    )
    .toHaveAttribute("aria-selected", "true");
  expect(
    document.querySelectorAll(
      '[role="tablist"][aria-label="文件标签"] [role="tab"]',
    ).length,
  ).toBe(20);
});

test.each(["file", "tab click", "tab key"] as const)(
  "a delayed file response cannot activate over a newer document choice (%s)",
  async (choice) => {
    serve();
    const server = globalThis.fetch;
    const delayed = Promise.withResolvers<Response>();
    vi.stubGlobal(
      "fetch",
      vi.fn<typeof fetch>((input, init) => {
        const url = new URL(String(input), location.href);
        if (
          url.pathname.endsWith("/file") &&
          atob(url.searchParams.get("nodeId")!) === "file-0.txt"
        )
          return delayed.promise;
        return server(input, init);
      }),
    );
    render(Harness);
    const latestFile = page.getByRole("button", {
      name: "file-1.txt",
      exact: true,
    });
    const latest = page.getByRole("tab", {
      name: "file-1.txt · worktree",
      exact: true,
    });
    if (choice !== "file") await latestFile.click();
    await page.getByRole("button", { name: "file-0.txt", exact: true }).click();
    if (choice === "file") await latestFile.click();
    else if (choice === "tab click") await latest.click();
    else {
      latest.element().focus();
      await userEvent.keyboard("{Enter}");
    }
    await expect.element(latest).toHaveAttribute("aria-selected", "true");
    delayed.resolve(Response.json(file(0)));
    await expect
      .element(
        page.getByRole("tab", { name: "file-0.txt · worktree", exact: true }),
      )
      .toBeInTheDocument();
    await expect.element(latest).toHaveAttribute("aria-selected", "true");
    await expect.poll(readerText).toContain("original file 1");
  },
);

test("loads changes beyond 1000 lazily, virtualizes rows, filters and refreshes a late diff", async () => {
  serve();
  const server = globalThis.fetch;
  const entries = Array.from({ length: 1505 }, (_, n) => ({
    nodeId: btoa(`file-${n}.txt`),
    changeId: `change-${n}`,
    group: "untracked",
    oldPath: `file-${n}.txt`,
    newPath: `file-${n}.txt`,
    status: "A",
    oldOid: "",
    newOid: "",
    oldMode: "000000",
    newMode: "100644",
  }));
  let stale = false;
  const changeRequests: URL[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>(async (input, init) => {
      const url = new URL(String(input), location.href);
      if (url.pathname.endsWith("/context")) {
        const response = await server(input, init);
        return Response.json({
          ...(await response.json()),
          diffAvailable: true,
        });
      }
      if (url.pathname.endsWith("/changes")) {
        changeRequests.push(url);
        const cursor = url.searchParams.get("cursor") || "";
        if (stale && cursor)
          return Response.json(
            { error: "修改列表已改变，请刷新列表" },
            { status: 409 },
          );
        const id = url.searchParams.get("nodeId"),
          q = url.searchParams.get("q") || "";
        const group = url.searchParams.get("group");
        const matching = entries.filter(
          (c) =>
            (!id || c.nodeId === id) &&
            (!group || c.group === group) &&
            c.newPath.includes(q),
        );
        const offset = Number(cursor || "0"),
          end = Math.min(offset + 200, matching.length);
        return Response.json({
          entries: matching.slice(offset, end),
          nextCursor: end < matching.length ? String(end) : "",
          total: matching.length,
          contentVersion: stale ? "updated" : "original",
          checkedAt: now,
        });
      }
      if (url.pathname.endsWith("/diff")) {
        const n = Number(url.searchParams.get("changeId")!.split("-")[1]);
        const next = file(n).document;
        return Response.json({
          snapshotId: `diff-${n}`,
          checkedAt: now,
          kind: "text",
          reason: "",
          change: entries[n],
          old: {
            ...next,
            source: "empty",
            text: "",
            lineCount: 0,
            referenceable: false,
            size: 0,
            downloadable: false,
          },
          new: next,
        });
      }
      return server(input, init);
    }),
  );
  render(Harness);
  await page.getByRole("tab", { name: "修改", exact: true }).click();
  await expect
    .element(page.getByText("已加载 200 / 1505 项", { exact: true }))
    .toBeVisible();
  const list = document.querySelector<HTMLElement>(
    '[role="list"][aria-label="修改列表"]',
  )!;
  expect(list.querySelectorAll("button").length).toBeLessThan(50);
  const source = document.querySelector<HTMLSelectElement>(
    'select[aria-label="修改来源"]',
  )!;
  source.value = "staged";
  source.dispatchEvent(new Event("change", { bubbles: true }));
  await expect
    .element(page.getByText("已加载 0 / 0 项", { exact: true }))
    .toBeVisible();
  source.value = "untracked";
  source.dispatchEvent(new Event("change", { bubbles: true }));
  await expect
    .element(page.getByText("已加载 200 / 1505 项", { exact: true }))
    .toBeVisible();
  const calls = changeRequests.length;
  list.querySelector<HTMLButtonElement>("button")!.focus();
  await userEvent.keyboard("{ArrowUp}");
  expect(changeRequests.length).toBe(calls);
  await userEvent.keyboard("{End}");
  await expect
    .element(page.getByText("已加载 400 / 1505 项", { exact: true }))
    .toBeVisible();
  for (const loaded of [600, 800, 1000, 1200, 1400, 1505]) {
    await page
      .getByRole("button", { name: "加载更多修改", exact: true })
      .click();
    await expect
      .element(page.getByText(`已加载 ${loaded} / 1505 项`, { exact: true }))
      .toBeVisible();
  }
  await expect
    .element(page.getByText("已加载 1505 / 1505 项", { exact: true }))
    .toBeVisible();
  list.scrollTop = list.scrollHeight;
  list.dispatchEvent(new Event("scroll"));
  await page
    .getByRole("button", { name: "未跟踪 file-1504.txt", exact: true })
    .click();
  await expect
    .element(page.getByText("original file 1504", { exact: true }))
    .toBeVisible();
  expect(list.querySelectorAll("button").length).toBeLessThan(50);
  await page.getByRole("button", { name: "刷新全部", exact: true }).click();
  await expect
    .poll(() =>
      changeRequests.some(
        (u) => u.searchParams.get("nodeId") === btoa("file-1504.txt"),
      ),
    )
    .toBe(true);
  await page.getByLabelText("筛选修改路径").fill("file-1504.txt");
  await page.getByRole("button", { name: "筛选修改", exact: true }).click();
  await expect
    .element(page.getByText("已加载 1 / 1 项", { exact: true }))
    .toBeVisible();
  await page
    .getByRole("button", { name: "未跟踪 file-1504.txt", exact: true })
    .click();
  await page.getByLabelText("筛选修改路径").fill("");
  await page.getByRole("button", { name: "筛选修改", exact: true }).click();
  await expect
    .element(page.getByText("已加载 200 / 1505 项", { exact: true }))
    .toBeVisible();
  stale = true;
  await page.getByRole("button", { name: "加载更多修改", exact: true }).click();
  await expect
    .element(page.getByRole("alert"))
    .toMatchTextContent("修改列表已改变");
  await expect
    .element(page.getByText("已加载 200 / 1505 项", { exact: true }))
    .toBeVisible();
  await expect
    .element(page.getByRole("button", { name: "加载更多修改", exact: true }))
    .toBeDisabled();
  await page.getByRole("button", { name: "刷新修改列表", exact: true }).click();
  await expect
    .element(page.getByRole("button", { name: "加载更多修改", exact: true }))
    .toBeEnabled();
});

// A same-origin iframe stands in for the pop-out window, closed through the
// window's pagehide. The shell's own frame would load the app at /code/<id>;
// a scripted stand-in performs the frame's handshake and records what the
// opener sends, so a test can speak the frame's side of the channel.
type FakeFrame = Window & {
  port?: MessagePort;
  messages: Record<string, unknown>[];
};
interface FakeWindow {
  readonly window: Window | null;
  readonly source: string;
  readonly frame: FakeFrame | undefined;
  open(): Window;
}
const frameScript =
  `<script>
  window.messages = [];
  addEventListener("message", (event) => {
    window.port = event.ports[0];
    window.port.onmessage = (m) => window.messages.push(m.data);
  });
  parent.postMessage({ type: "webmux-code-popout", version: 1 }, "*");
</scr` + `ipt>`;
function fakeWindow(): FakeWindow {
  let shell: HTMLIFrameElement | undefined;
  let frame: HTMLIFrameElement | undefined;
  let source = "";
  return {
    get window() {
      return shell?.contentWindow ?? null;
    },
    get source() {
      return source;
    },
    get frame() {
      return frame?.contentWindow as FakeFrame | undefined;
    },
    open() {
      shell = document.createElement("iframe");
      shell.title = "popout";
      document.body.append(shell);
      const win = shell.contentWindow!;
      const create = win.document.createElement.bind(win.document);
      win.document.createElement = ((tag: string) => {
        const element = create(tag);
        if (element instanceof win.HTMLIFrameElement) {
          frame = element;
          Object.defineProperty(element, "src", {
            set(value: string) {
              source = value;
              element.srcdoc = frameScript;
            },
          });
        }
        return element;
      }) as typeof win.document.createElement;
      Object.defineProperty(win, "close", {
        value: () => {
          win.dispatchEvent(new Event("pagehide"));
          shell?.remove();
          shell = undefined;
        },
      });
      return win;
    },
  };
}
// Waits for the frame to have received a message of `type` and returns it.
async function received(
  popup: FakeWindow,
  type: string,
): Promise<Record<string, unknown>> {
  let found: Record<string, unknown> | undefined;
  await expect
    .poll(() => {
      found = popup.frame?.messages.find((m) => m.type === type);
      return found;
    })
    .toBeTruthy();
  return found!;
}
function fakePictureInPicture(refuse?: Error) {
  const fake = fakeWindow();
  const api = {
    get window() {
      return fake.window;
    },
    async requestWindow() {
      if (refuse) throw refuse;
      return fake.open();
    },
  };
  vi.stubGlobal("documentPictureInPicture", api);
  return api;
}
// Leaves every other media query to the real browser.
function pointer(fine: boolean) {
  const real = window.matchMedia.bind(window);
  vi.stubGlobal("matchMedia", (query: string) =>
    query === "(pointer: fine)"
      ? Object.defineProperty(real(query), "matches", { value: fine })
      : real(query),
  );
}

test("touch-only devices without Picture-in-Picture get no pop-out control", async () => {
  serve();
  vi.stubGlobal("documentPictureInPicture", undefined);
  pointer(false);
  render(Harness);
  await expect
    .element(page.getByRole("button", { name: "file-0.txt", exact: true }))
    .toBeVisible();
  expect(
    page.getByRole("button", { name: "在独立窗口中显示代码" }).elements(),
  ).toHaveLength(0);
});

// Opens file-0 and file-1, then pops out into an ordinary popup.
async function popOutWithTabs(beforePopOut?: () => void) {
  serve();
  vi.stubGlobal("documentPictureInPicture", undefined);
  pointer(true);
  const popup = fakeWindow();
  const open = vi.spyOn(window, "open").mockImplementation(() => popup.open());
  render(Harness);
  await page.getByRole("button", { name: "file-0.txt", exact: true }).click();
  await page.getByRole("button", { name: "file-1.txt", exact: true }).click();
  beforePopOut?.();
  await page
    .getByRole("button", { name: "在独立窗口中显示代码", exact: true })
    .click();
  return { popup, open };
}
type Snapshot = { active: string; tabs: { key: string }[] };

test("desktop browsers without Picture-in-Picture pop out into a framed popup and take the state back", async () => {
  const { popup, open } = await popOutWithTabs();
  expect(open).toHaveBeenCalledWith(
    "",
    "_blank",
    expect.stringMatching(/^popup,width=\d+,height=\d+$/),
  );
  const init = await received(popup, "init");
  expect(popup.source).toBe("/code/code-session");
  const snapshot = init.snapshot as Snapshot;
  expect(snapshot.tabs.map((tab) => tab.key)).toHaveLength(2);
  expect(snapshot.active).toBe(snapshot.tabs[1]!.key);
  // The opener keeps its workspace until the frame shows its own.
  expect(document.querySelector(".code-panel")).not.toBeNull();
  popup.frame!.port!.postMessage({ type: "ready" });
  await expect.element(page.getByText("代码已在独立窗口中显示")).toBeVisible();
  expect(document.querySelector(".code-panel")).toBeNull();

  const reads = () =>
    vi
      .mocked(fetch)
      .mock.calls.filter(([input]) => String(input).includes("/file?")).length;
  const before = reads();
  await page.getByRole("button", { name: "收回到此处" }).click();
  const collect = await received(popup, "collect");
  popup.frame!.port!.postMessage({
    type: "snapshot",
    requestId: collect.requestId,
    snapshot: { ...snapshot, active: snapshot.tabs[0]!.key },
  });
  await expect
    .element(page.getByRole("tab", { name: /^file-0\.txt/ }))
    .toHaveAttribute("aria-selected", "true");
  await expect.poll(readerText).toContain("original file 0");
  expect(popup.window).toBeNull();
  // Restored tabs keep the content they were handed.
  expect(reads()).toBe(before);
});

test("a window closed by the user returns its last reported state", async () => {
  const { popup } = await popOutWithTabs();
  const snapshot = (await received(popup, "init")).snapshot as Snapshot;
  popup.frame!.port!.postMessage({ type: "ready" });
  await expect.element(page.getByText("代码已在独立窗口中显示")).toBeVisible();
  popup.frame!.port!.postMessage({
    type: "snapshot",
    snapshot: { ...snapshot, active: snapshot.tabs[0]!.key },
  });
  // Malformed or unknown messages are dropped without effect.
  popup.frame!.port!.postMessage({ type: "snapshot", snapshot: { bad: 1 } });
  popup.frame!.port!.postMessage({ type: "unknown" });
  await expect.poll(() => popup.frame?.messages.length).toBeGreaterThan(0);
  popup.window!.close();
  await expect
    .element(page.getByRole("tab", { name: /^file-0\.txt/ }))
    .toHaveAttribute("aria-selected", "true");
});

test("a state handed over while the tree was still loading reads it again", async () => {
  const { popup } = await popOutWithTabs();
  const snapshot = (await received(popup, "init")).snapshot as Snapshot;
  popup.frame!.port!.postMessage({ type: "ready" });
  await expect.element(page.getByText("代码已在独立窗口中显示")).toBeVisible();
  popup.frame!.port!.postMessage({
    type: "snapshot",
    snapshot: { ...snapshot, listing: undefined, rootListing: undefined },
  });
  await expect.poll(() => popup.frame?.messages.length).toBeGreaterThan(0);
  popup.window!.close();
  await expect
    .element(page.getByRole("button", { name: "file-0.txt", exact: true }))
    .toBeVisible();
});

test("the window sends each document once and keeps update markers", async () => {
  const { popup } = await popOutWithTabs();
  const init = (await received(popup, "init")).snapshot as Snapshot & {
    tabs: { key: string; status: string; file?: { snapshotId: string } }[];
  };
  popup.frame!.port!.postMessage({ type: "ready" });
  await expect.element(page.getByText("代码已在独立窗口中显示")).toBeVisible();
  // Later snapshots name bodies the opener already holds.
  popup.frame!.port!.postMessage({
    type: "snapshot",
    snapshot: {
      ...init,
      active: init.tabs[0]!.key,
      tabs: init.tabs.map((tab) => ({
        ...tab,
        file: undefined,
        fileRef: tab.file!.snapshotId,
        status: tab.key === init.tabs[0]!.key ? "updated" : tab.status,
      })),
    },
  });
  await expect.poll(() => popup.frame?.messages.length).toBeGreaterThan(0);
  popup.window!.close();
  await expect
    .element(page.getByRole("tab", { name: /^file-0\.txt · worktree ●$/ }))
    .toHaveAttribute("aria-selected", "true");
  await expect.poll(readerText).toContain("original file 0");
  await expect
    .element(page.getByRole("tab", { name: /^file-1\.txt/ }))
    .toBeInTheDocument();
});

test("requests to show a file while popped out go to the window, once", async () => {
  const { popup } = await popOutWithTabs();
  await received(popup, "init");
  popup.frame!.port!.postMessage({ type: "ready" });
  await expect.element(page.getByText("代码已在独立窗口中显示")).toBeVisible();
  expect(popup.frame!.messages.filter((m) => m.type === "reveal")).toHaveLength(
    0,
  );
  await page.getByRole("button", { name: "Reveal file 3 lines" }).click();
  expect(await received(popup, "reveal")).toEqual({
    type: "reveal",
    reveal: {
      path: "file-3.txt",
      mode: "file",
      line: 2,
      endLine: 4,
      key: 1,
    },
  });
});

test("revealing a file location opens file mode, selects its line range and resolves repo-root or literal-colon fallbacks", async () => {
  serve();
  const server = globalThis.fetch;
  const multiline = (name: string) => ({
    snapshotId: `snapshot-${name}`,
    checkedAt: now,
    document: {
      nodeId: btoa(name),
      path: name,
      absolutePath: `/root/${name}`,
      source: "worktree" as const,
      oid: "",
      contentVersion: "v1",
      kind: "text" as const,
      text: ["one", "two", "three", "four", "five", ""].join("\n"),
      lineCount: 5,
      referenceable: true,
      reason: "",
      mediaType: "",
      size: 23,
      downloadable: true,
    },
  });
  // Once set, the cwd has its own src/app.ts shadowing the repo-root one.
  let shadowed = false;
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>(async (input, init) => {
      const url = new URL(String(input), location.href);
      if (url.pathname.endsWith("/context")) {
        const response = await server(input, init);
        return Response.json({
          ...(await response.json()),
          diffAvailable: true,
        });
      }
      if (url.pathname.endsWith("/changes")) {
        const id = url.searchParams.get("nodeId");
        const all = [
          {
            nodeId: btoa("file-3.txt"),
            changeId: "change-3",
            group: "unstaged",
            oldPath: "file-3.txt",
            newPath: "file-3.txt",
            status: "M",
            oldOid: "",
            newOid: "",
            oldMode: "100644",
            newMode: "100644",
          },
        ];
        const entries = id ? all.filter((entry) => entry.nodeId === id) : all;
        return Response.json({
          entries,
          nextCursor: "",
          total: entries.length,
          contentVersion: "changes",
          checkedAt: now,
        });
      }
      if (url.pathname.endsWith("/locate")) {
        const requested = url.searchParams.get("path") ?? "";
        if (requested === "file-3.txt")
          return Response.json({
            nodeId: btoa("file-3.txt"),
            path: "file-3.txt",
            name: "file-3.txt",
            kind: "file",
            gitStatus: "M",
            ignored: false,
            referenceable: true,
            mediaType: "",
          });
        if (requested === "src/app.ts" && shadowed)
          return Response.json({
            nodeId: btoa("sub/src/app.ts"),
            path: "sub/src/app.ts",
            name: "app.ts",
            kind: "file",
            gitStatus: "",
            ignored: false,
            referenceable: true,
            mediaType: "",
          });
        if (requested === "/root/src/app.ts")
          return Response.json({
            nodeId: btoa("src/app.ts"),
            path: "src/app.ts",
            name: "app.ts",
            kind: "file",
            gitStatus: "",
            ignored: false,
            referenceable: true,
            mediaType: "",
          });
        if (requested === "literal:1")
          return Response.json({
            nodeId: btoa("literal:1"),
            path: "literal:1",
            name: "literal:1",
            kind: "file",
            gitStatus: "",
            ignored: false,
            referenceable: true,
            mediaType: "",
          });
        return Response.json({ error: "not found" }, { status: 404 });
      }
      if (url.pathname.endsWith("/file")) {
        const name = atob(url.searchParams.get("nodeId")!);
        return Response.json(multiline(name));
      }
      return server(input, init);
    }),
  );

  render(Harness, { sessionCwd: "/root/sub" });
  await page.getByRole("button", { name: "file-0.txt", exact: true }).click();

  // Even when file-3.txt has an unstaged diff, mode: "file" opens file view and selects lines 2-4.
  await page.getByRole("button", { name: "Reveal file 3 lines" }).click();
  const sendRange = page.getByRole("button", {
    name: "发送位置到终端（2-4 行）",
  });
  await expect.element(sendRange).toBeEnabled();
  await sendRange.click();
  await expect
    .element(page.getByLabelText("Inserted reference"))
    .toMatchTextContent(/file-3\.txt.*2.*4/);

  // Relative path from repo root resolves via /root/src/app.ts when cwd is /root/sub.
  await page.getByRole("button", { name: "Reveal repo relative" }).click();
  await expect
    .element(page.getByRole("button", { name: "发送位置到终端（3 行）" }))
    .toBeEnabled();

  // Literal colon filename falls back to rawPath ("literal:1") without applying the suffix line.
  await page.getByRole("button", { name: "Reveal raw path" }).click();
  await expect
    .element(
      page.getByRole("tab", { name: "literal:1 · worktree", exact: true }),
    )
    .toHaveAttribute("aria-selected", "true");

  // A same-named file under the cwd wins over the repo-root one.
  shadowed = true;
  await page.getByRole("button", { name: "Reveal repo relative" }).click();
  await expect
    .element(
      page.getByRole("tab", { name: "sub/src/app.ts · worktree", exact: true }),
    )
    .toHaveAttribute("aria-selected", "true");
});

test("the frame inserts references through the opener and the opener closes it on leave", async () => {
  const { popup } = await popOutWithTabs();
  await received(popup, "init");
  popup.frame!.port!.postMessage({ type: "ready" });
  await expect.element(page.getByText("代码已在独立窗口中显示")).toBeVisible();
  popup.frame!.port!.postMessage({
    type: "paste",
    requestId: 7,
    text: "@file-1.txt",
  });
  const result = await received(popup, "pasteResult");
  expect(result).toEqual({
    type: "pasteResult",
    requestId: 7,
    result: "terminal",
  });
  await expect
    .element(page.getByLabelText("Inserted reference"))
    .toHaveTextContent("@file-1.txt");
  // Reloading or leaving the opener must not strand the popup.
  window.dispatchEvent(new Event("pagehide"));
  expect(popup.window).toBeNull();
});

test("a frame that never gets ready closes and leaves the workspace in place", async () => {
  const { popup } = await popOutWithTabs(() =>
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] }),
  );
  try {
    await received(popup, "init");
    vi.advanceTimersByTime(10_000);
  } finally {
    vi.useRealTimers();
  }
  await expect
    .element(page.getByRole("alert"))
    .toMatchTextContent("独立窗口加载超时");
  expect(popup.window).toBeNull();
  await expect
    .element(page.getByRole("tab", { name: /^file-1\.txt/ }))
    .toHaveAttribute("aria-selected", "true");
});

test("a blocked popup keeps the panel in place and says why", async () => {
  serve();
  vi.stubGlobal("documentPictureInPicture", undefined);
  pointer(true);
  vi.spyOn(window, "open").mockReturnValue(null);
  render(Harness);
  await page
    .getByRole("button", { name: "在独立窗口中显示代码", exact: true })
    .click();
  await expect
    .element(page.getByRole("alert"))
    .toMatchTextContent("弹出窗口被浏览器拦截");
  expect(page.getByText("代码已在独立窗口中显示").elements()).toHaveLength(0);
  expect(document.querySelector(".code-panel")).not.toBeNull();
});

test("a refused pop-out window keeps the panel in place and says why", async () => {
  serve();
  fakePictureInPicture(
    new DOMException(
      "Document PiP requires user activation",
      "NotAllowedError",
    ),
  );
  render(Harness);
  await page
    .getByRole("button", { name: "在独立窗口中显示代码", exact: true })
    .click();
  await expect
    .element(page.getByRole("alert"))
    .toMatchTextContent("Document PiP requires user activation");
  expect(page.getByText("代码已在独立窗口中显示").elements()).toHaveLength(0);
  await page.getByRole("button", { name: "file-0.txt", exact: true }).click();
  await expect.poll(readerText).toContain("original file 0");
});

test("checks all loaded tree branches within the server's 30-directory limit", async () => {
  serve();
  const server = globalThis.fetch;
  const names = Array.from({ length: 31 }, (_, n) => `folder-${n}`);
  const batches: string[][] = [];
  const loaded = new Set<string>();
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>(async (input, init) => {
      const url = new URL(String(input), location.href);
      if (url.pathname.endsWith("/tree")) {
        const path = atob(url.searchParams.get("nodeId") ?? "");
        loaded.add(path);
        return Response.json({
          children: path
            ? []
            : names.map((name) => ({
                nodeId: btoa(name),
                path: name,
                name,
                kind: "directory",
                gitStatus: "",
                ignored: false,
                referenceable: false,
                mediaType: "",
              })),
          nextCursor: "",
          limited: false,
          complete: true,
          contentVersion: "tree",
          checkedAt: now,
        });
      }
      if (url.pathname.endsWith("/check")) {
        const body = JSON.parse(String(init?.body));
        const directories = Object.keys(body.directories);
        batches.push(directories);
        if (directories.length > 30)
          return Response.json(
            { error: "too many directories" },
            { status: 400 },
          );
      }
      return server(input, init);
    }),
  );
  await render(Harness);
  for (const name of names)
    await page.getByRole("button", { name, exact: true }).click();
  await expect.poll(() => loaded.size).toBe(32);
  batches.length = 0;
  document.dispatchEvent(new Event("visibilitychange"));
  await expect.poll(() => new Set(batches.flat()).size).toBe(32);
  expect(batches.every((batch) => batch.length <= 30)).toBe(true);
});

test("collapsing during a path search cannot replace the root with search hits", async () => {
  serve();
  const server = globalThis.fetch;
  const delayed = Promise.withResolvers<Response>();
  let searched = false;
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>((input, init) => {
      if (new URL(String(input), location.href).pathname.endsWith("/files")) {
        searched = true;
        return delayed.promise;
      }
      return server(input, init);
    }),
  );
  render(Harness);
  await page.getByRole("button", { name: "file-0.txt", exact: true }).query();
  await page.getByRole("button", { name: "切换路径搜索" }).click();
  await page.getByRole("textbox", { name: "搜索文件路径" }).fill("nested");
  await page.getByRole("button", { name: "搜索路径", exact: true }).click();
  await expect.poll(() => searched).toBe(true);
  await expect
    .element(page.getByRole("button", { name: "刷新文件树", exact: true }))
    .toBeDisabled();
  await page.getByRole("button", { name: "全部折叠", exact: true }).click();
  delayed.resolve(
    Response.json({
      children: [
        {
          nodeId: btoa("nested/hit.txt"),
          path: "nested/hit.txt",
          name: "hit.txt",
          kind: "file",
          gitStatus: "",
          ignored: false,
          referenceable: true,
          mediaType: "",
        },
      ],
      nextCursor: "",
      limited: false,
      complete: true,
      contentVersion: "search",
      checkedAt: now,
    }),
  );
  await expect
    .element(page.getByRole("button", { name: "刷新文件树", exact: true }))
    .toBeEnabled();
  await expect
    .element(page.getByRole("button", { name: "file-0.txt", exact: true }))
    .toBeVisible();
  await expect
    .element(page.getByRole("button", { name: "hit.txt", exact: true }))
    .not.toBeInTheDocument();
});

function serveExplorer() {
  serve();
  const server = globalThis.fetch;
  let revision = "v1";
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>(async (input, init) => {
      const url = new URL(String(input), location.href);
      if (url.pathname.endsWith("/context")) {
        const response = await server(input, init);
        return Response.json({
          ...(await response.json()),
          diffAvailable: true,
          ignoreAvailable: true,
          writableAvailable: true,
        });
      }
      if (url.pathname.endsWith("/changes"))
        return Response.json({
          entries: [],
          nextCursor: "",
          total: 0,
          contentVersion: "changes",
          checkedAt: now,
          limited: false,
          complete: true,
        });
      if (url.pathname.endsWith("/tree")) {
        const path = atob(url.searchParams.get("nodeId") ?? "");
        const names = path ? [`docs/${revision}.txt`] : ["docs"];
        return Response.json({
          children: names.map((name) => ({
            nodeId: btoa(name),
            path: name,
            name: name.split("/").at(-1),
            kind: path ? "file" : "directory",
            gitStatus: "",
            ignored: false,
            referenceable: !!path,
            mediaType: "",
          })),
          nextCursor: "",
          limited: false,
          complete: true,
          contentVersion: path ? revision : "root",
          checkedAt: now,
        });
      }
      return server(input, init);
    }),
  );
  return {
    refresh: () => {
      revision = "v2";
    },
  };
}

test("expanded branches survive hiding navigation, changing tabs and ignore scope", async () => {
  serveExplorer();
  render(Harness);
  await page.getByRole("button", { name: "docs", exact: true }).click();
  const child = page.getByRole("button", {
    name: "v1.txt",
    exact: true,
    includeHidden: true,
  });
  await expect.element(child).toBeVisible();
  await page.getByRole("button", { name: "收起导航", exact: true }).click();
  await expect.element(child).not.toBeVisible();
  await page.getByRole("button", { name: "文件导航", exact: true }).click();
  await expect.element(child).toBeVisible();
  await page.getByRole("tab", { name: "修改", exact: true }).click();
  await expect.element(child).not.toBeVisible();
  await page.getByRole("tab", { name: "文件", exact: true }).click();
  await expect.element(child).toBeVisible();
  await page.getByRole("button", { name: "文件导航选项", exact: true }).click();
  await page.getByRole("menuitemcheckbox", { name: "显示忽略项" }).click();
  await expect.element(child).toBeVisible();
  await expect
    .element(page.getByRole("button", { name: "docs", exact: true }))
    .toHaveAttribute("aria-expanded", "true");
});

test("a directory check from before refresh cannot restore a stale update warning", async () => {
  const explorer = serveExplorer();
  const server = globalThis.fetch;
  const delayed = Promise.withResolvers<Response>();
  let checking = false;
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>((input, init) => {
      const url = new URL(String(input), location.href);
      if (
        url.pathname.endsWith("/check") &&
        JSON.parse(String(init?.body)).directories[nodeId("docs")] === "v1"
      ) {
        checking = true;
        return delayed.promise;
      }
      return server(input, init);
    }),
  );
  render(Harness);
  await page.getByRole("button", { name: "docs", exact: true }).click();
  await expect
    .element(page.getByRole("button", { name: "v1.txt", exact: true }))
    .toBeVisible();
  document.dispatchEvent(new Event("visibilitychange"));
  await expect.poll(() => checking).toBe(true);
  explorer.refresh();
  await page.getByRole("button", { name: "刷新文件树", exact: true }).click();
  await expect
    .element(page.getByRole("button", { name: "v2.txt", exact: true }))
    .toBeVisible();
  delayed.resolve(
    Response.json({
      snapshots: {},
      directories: { [nodeId("docs")]: "updated" },
      changes: "unchanged",
      checkedAt: now,
    }),
  );
  const rendered = Promise.withResolvers<void>();
  requestAnimationFrame(() => requestAnimationFrame(() => rendered.resolve()));
  await rendered.promise;
  await expect
    .element(page.getByText("目录已更新或检测失败", { exact: true }))
    .not.toBeInTheDocument();
});

test("file tabs support roving focus, arrow keys and stable panel relationships", async () => {
  serve();
  render(Harness);
  for (const n of [0, 1, 2])
    await page
      .getByRole("button", { name: `file-${n}.txt`, exact: true })
      .click();
  const tab = (n: number) =>
    page.getByRole("tab", { name: `file-${n}.txt · worktree`, exact: true });
  await expect.element(tab(2)).toHaveAttribute("aria-selected", "true");
  tab(2).element().focus();
  await userEvent.keyboard("{ArrowLeft}");
  await expect.element(tab(1)).toHaveFocus();
  await expect.element(tab(1)).toHaveAttribute("aria-selected", "true");
  await userEvent.keyboard("{ArrowRight}");
  await expect.element(tab(2)).toHaveFocus();
  await userEvent.keyboard("{ArrowRight}");
  await expect.element(tab(0)).toHaveFocus();
  await userEvent.keyboard("{End}");
  await expect.element(tab(2)).toHaveFocus();
  await userEvent.keyboard("{Home}");
  await expect.element(tab(0)).toHaveFocus();
  tab(2).element().focus();
  await expect.element(tab(2)).toHaveAttribute("aria-selected", "true");
  tab(0).element().focus();
  await expect.element(tab(0)).toHaveAttribute("aria-selected", "true");
  const before = [
    ...document.querySelectorAll<HTMLElement>(
      '[aria-label="文件标签"] [role="tab"]',
    ),
  ].map((element) => ({
    id: element.id,
    controls: element.getAttribute("aria-controls"),
  }));
  expect(new Set(before.map((item) => item.id)).size).toBe(3);
  for (const item of before) {
    expect(
      document.getElementById(item.controls!)?.getAttribute("aria-labelledby"),
    ).toBe(item.id);
  }
  await page.getByRole("button", { name: "刷新全部", exact: true }).click();
  await expect.element(tab(0)).toHaveAttribute("id", before[0]!.id);
  await expect
    .element(
      page.elementLocator(document.querySelector('[aria-label="文件标签"]')!),
    )
    .toMatchTextContent(/file-2/);
  expect(
    [
      ...document.querySelectorAll('[aria-label="文件标签"] [role="tab"]'),
    ].filter((element) => element.getAttribute("tabindex") === "0"),
  ).toHaveLength(1);
});
