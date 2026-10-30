import { afterEach, expect, test, vi } from "vitest";
import { page, userEvent } from "vitest/browser";
import { cleanup, render } from "vitest-browser-svelte/pure";
import Harness from "./CodePanelHarness.svelte";

const now = "2026-10-31T00:00:00Z";
const folder = "/tmp/webmux-artifacts-1000-abcd/code-session";

afterEach(async () => {
  await cleanup();
  await userEvent.cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

function node(path: string) {
  return {
    nodeId: btoa(path),
    path,
    name: path,
    kind: "file",
    gitStatus: "",
    ignored: false,
    referenceable: true,
    mediaType: "",
  };
}

function serve() {
  const log: string[] = [];
  const checks: { contextId: string; snapshotIds: string[] }[] = [];
  let ensureFails = true;
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>(async (input, init) => {
      const url = new URL(String(input), location.href);
      const method = init?.method ?? "GET";
      log.push(`${method} ${url.pathname}${url.search}`);
      if (url.pathname === "/api/sessions/code-session/artifacts") {
        expect(method).toBe("POST");
        if (ensureFails)
          return Response.json(
            { error: "临时文件夹路径被占用" },
            { status: 503 },
          );
        return Response.json({ path: folder });
      }
      const artifacts = url.searchParams.get("contextId") === "art";
      if (url.pathname.endsWith("/context")) {
        const root = url.searchParams.get("root");
        return Response.json(
          root === "artifacts"
            ? {
                contextId: "art",
                rootPath: folder,
                rootKind: "artifacts",
                browseAvailable: true,
                diffAvailable: false,
                ignoreAvailable: false,
                reason: "",
              }
            : {
                contextId: "ws",
                rootPath: "/repo",
                rootKind: "worktree",
                browseAvailable: true,
                diffAvailable: true,
                ignoreAvailable: true,
                reason: "",
              },
        );
      }
      if (url.pathname.endsWith("/tree"))
        return Response.json({
          children: [node(artifacts ? "report.md" : "main.go")],
          nextCursor: "",
          limited: false,
          complete: true,
          contentVersion: artifacts ? "art-tree" : "ws-tree",
          checkedAt: now,
        });
      if (url.pathname.endsWith("/file")) {
        const path = atob(url.searchParams.get("nodeId")!);
        return Response.json({
          snapshotId: `snap-${path}`,
          checkedAt: now,
          document: {
            nodeId: btoa(path),
            path,
            absolutePath: `${artifacts ? folder : "/repo"}/${path}`,
            source: "worktree",
            oid: "",
            contentVersion: "v1",
            kind: "text",
            text: `${path} body\n`,
            lineCount: 1,
            referenceable: true,
            reason: "",
            mediaType: "",
            size: 10,
            downloadable: true,
          },
        });
      }
      if (url.pathname.endsWith("/check")) {
        const req = JSON.parse(String(init?.body));
        checks.push({ contextId: req.contextId, snapshotIds: req.snapshotIds });
        return Response.json({
          snapshots: Object.fromEntries(
            req.snapshotIds.map((id: string) => [id, "unchanged"]),
          ),
          directories: {},
          changes: "unchanged",
          checkedAt: now,
        });
      }
      throw new Error(`Unexpected ${method} ${url}`);
    }),
  );
  return {
    log,
    checks,
    recover() {
      ensureFails = false;
    },
  };
}

test("the artifacts root ensures the folder first, hides changes and keeps tabs of both roots", async () => {
  const server = serve();
  render(Harness);
  const rootTab = (name: string) =>
    page.getByRole("tab", { name, exact: true });
  await expect
    .element(page.getByRole("button", { name: "main.go", exact: true }))
    .toBeVisible();
  await expect.element(rootTab("修改")).toBeVisible();
  await page.getByRole("button", { name: "main.go", exact: true }).click();

  await rootTab("临时文件夹").click();
  await expect
    .element(page.getByRole("alert"))
    .toHaveTextContent("临时文件夹不可用：临时文件夹路径被占用");
  expect(server.log).not.toContain(
    "GET /api/sessions/code-session/code/context?root=artifacts",
  );
  await expect
    .element(page.getByRole("button", { name: "main.go", exact: true }))
    .not.toBeInTheDocument();
  await expect.element(rootTab("修改")).not.toBeInTheDocument();

  server.recover();
  await page.getByRole("button", { name: "重试" }).click();
  await expect
    .element(page.getByRole("button", { name: "report.md", exact: true }))
    .toBeVisible();
  const ensure = server.log.lastIndexOf(
    "POST /api/sessions/code-session/artifacts",
  );
  const context = server.log.indexOf(
    "GET /api/sessions/code-session/code/context?root=artifacts",
  );
  expect(ensure).toBeGreaterThanOrEqual(0);
  expect(context).toBeGreaterThan(ensure);
  await expect
    .element(
      page
        .getByRole("navigation", { name: "文件与修改导航" })
        .getByText(folder, { exact: true }),
    )
    .toBeVisible();
  await expect.element(rootTab("修改")).not.toBeInTheDocument();

  await page.getByRole("button", { name: "report.md", exact: true }).click();
  await expect
    .element(page.getByRole("tab", { name: "report.md · 临时文件夹" }))
    .toHaveAttribute("title", "临时文件夹 · report.md");

  await rootTab("工作区").click();
  await expect
    .element(page.getByRole("button", { name: "main.go", exact: true }))
    .toBeVisible();
  await expect.element(rootTab("修改")).toBeVisible();
  await expect
    .element(page.getByRole("tab", { name: "report.md · 临时文件夹" }))
    .toBeVisible();
  await expect
    .element(page.getByRole("tab", { name: "main.go · worktree" }))
    .toBeVisible();

  // Each root's tabs are checked under the context they were read from.
  server.checks.length = 0;
  await expect
    .poll(() => {
      document.dispatchEvent(new Event("visibilitychange"));
      return server.checks;
    })
    .toEqual(
      expect.arrayContaining([
        { contextId: "ws", snapshotIds: ["snap-main.go"] },
        { contextId: "art", snapshotIds: ["snap-report.md"] },
      ]),
    );
});
