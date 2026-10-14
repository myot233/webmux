import { afterEach, expect, test, vi } from "vitest";
import { page, userEvent } from "vitest/browser";
import { cleanup, render } from "vitest-browser-svelte/pure";
import Harness from "./CodePanelHarness.svelte";
const now = "2026-10-14T00:00:00Z";
function file(n: number, version = "original") {
  return {
    snapshotId: `snapshot-${n}-${version}`,
    checkedAt: now,
    document: {
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
  await expect
    .element(page.getByLabelText("只读代码"))
    .toMatchTextContent(new RegExp("original file 0"));
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
  await expect
    .element(page.getByLabelText("只读代码"))
    .toMatchTextContent(new RegExp("original file 0"));
  version = "refreshed";
  state = "unchanged";
  await page.getByRole("button", { name: "刷新文档" }).click();
  await expect
    .element(page.getByLabelText("只读代码"))
    .toMatchTextContent(new RegExp("refreshed file 0"));
  await page.getByRole("button", { name: "发送位置到终端" }).click();
  await expect
    .element(page.getByLabelText("Inserted reference"))
    .toMatchTextContent(new RegExp("/root/file-0.txt"));
  checkFailed = true;
  document.dispatchEvent(new Event("visibilitychange"));
  await expect
    .element(page.getByRole("button", { name: "发送位置到终端" }))
    .toBeDisabled();
  await expect
    .element(page.getByLabelText("只读代码"))
    .toMatchTextContent(new RegExp("refreshed file 0"));
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

test("a delayed file response cannot activate over a newer document choice", async () => {
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
  await page.getByRole("button", { name: "file-0.txt", exact: true }).click();
  await page.getByRole("button", { name: "file-1.txt", exact: true }).click();
  const latest = page.getByRole("tab", {
    name: "file-1.txt · worktree",
    exact: true,
  });
  await expect.element(latest).toHaveAttribute("aria-selected", "true");
  delayed.resolve(Response.json(file(0)));
  await expect
    .element(
      page.getByRole("tab", { name: "file-0.txt · worktree", exact: true }),
    )
    .toBeInTheDocument();
  await expect.element(latest).toHaveAttribute("aria-selected", "true");
  await expect
    .element(page.getByRole("tabpanel").getByLabelText("只读代码"))
    .toMatchTextContent("original file 1");
});
