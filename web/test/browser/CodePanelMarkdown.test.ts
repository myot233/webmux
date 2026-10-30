import { afterEach, expect, test, vi } from "vitest";
import { page, userEvent } from "vitest/browser";
import { cleanup, render } from "vitest-browser-svelte/pure";
import Harness from "./CodePanelHarness.svelte";
import { nodeId } from "../../src/code-reference";

const now = "2026-10-30T00:00:00Z";
const filler = Array.from({ length: 40 }, (_, i) => `段落 ${i + 1}`).join(
  "\n\n",
);
const texts: Record<string, string> = {
  "README.md": [
    "# 项目",
    "[配置](docs/config.md) [别名](link.md) [示例](./examples/) [缺失](missing.md) [第二节](docs/config.md#第二节)",
    "![图](img/a.png) ![大](img/big.png) ![文本](img/notes.png)",
  ].join("\n\n"),
  "docs/config.md": `# 配置\n\n${filler}\n\n## 第二节\n\n${filler}\n`,
  "img/notes.png": "not an image\n",
};
function document(path: string) {
  const text = texts[path] ?? "";
  const image = path === "img/a.png" || path === "img/big.png";
  return {
    snapshotId: `snapshot:${path}`,
    checkedAt: now,
    document: {
      nodeId: nodeId(path),
      path,
      absolutePath: `/root/${path}`,
      source: "worktree",
      oid: "",
      contentVersion: "v1",
      kind: path === "img/big.png" ? "too-large" : image ? "image" : "text",
      text: image ? "" : text,
      lineCount: image ? 0 : text.split("\n").length,
      referenceable: true,
      reason: "",
      mediaType: image ? "image/png" : "",
      size: path === "img/big.png" ? 30 << 20 : text.length,
      downloadable: true,
    },
  };
}
async function png() {
  const canvas = new OffscreenCanvas(30, 20);
  canvas.getContext("2d")!.fillRect(0, 0, 30, 20);
  return canvas.convertToBlob({ type: "image/png" });
}
// Locate answers by absolute path, as the server does after resolving links.
const locations: Record<string, { path: string; kind: string } | number> = {
  "/root/docs/config.md": { path: "docs/config.md", kind: "file" },
  "/root/link.md": { path: "docs/config.md", kind: "file" },
  "/root/examples": { path: "examples", kind: "directory" },
  "/root/missing.md": 404,
  "/root/img/a.png": { path: "img/a.png", kind: "file" },
  "/root/img/big.png": { path: "img/big.png", kind: "file" },
  "/root/img/notes.png": { path: "img/notes.png", kind: "file" },
};
async function serve() {
  const blob = await png();
  const located: string[] = [];
  const raw: RequestInit[] = [];
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
          children: [
            {
              nodeId: nodeId("README.md"),
              path: "README.md",
              name: "README.md",
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
          contentVersion: "tree",
          checkedAt: now,
        });
      if (url.pathname.endsWith("/locate")) {
        const path = url.searchParams.get("path")!;
        located.push(path);
        const answer = locations[path];
        if (typeof answer !== "object")
          return Response.json({ error: "路径不存在" }, { status: 404 });
        return Response.json({ ...answer, nodeId: nodeId(answer.path) });
      }
      if (url.pathname.endsWith("/file")) {
        const path = atob(
          url.searchParams.get("nodeId")!.replace(/-/g, "+").replace(/_/g, "/"),
        );
        return Response.json(document(path));
      }
      if (url.pathname.endsWith("/raw")) {
        raw.push(init ?? {});
        return new Response(blob, { headers: { "Content-Type": "image/png" } });
      }
      if (url.pathname.endsWith("/check"))
        return Response.json({
          snapshots: {},
          directories: {},
          changes: "unchanged",
          checkedAt: now,
        });
      throw new Error(`Unexpected ${url}`);
    }),
  );
  return { located, raw };
}

afterEach(async () => {
  await cleanup();
  await userEvent.cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  localStorage.removeItem("webmux.token");
});

async function openReadmePreview() {
  await page.getByRole("button", { name: "README.md", exact: true }).click();
  await page.getByRole("button", { name: "预览", exact: true }).click();
  await expect
    .element(page.getByRole("heading", { name: "项目" }))
    .toBeVisible();
}
const visiblePreview = () =>
  [
    ...window.document.querySelectorAll<HTMLElement>(
      '[aria-label="Markdown 预览"]',
    ),
  ].find((element) => element.offsetParent !== null)!;

test("preview links open files and report symlinks and missing targets", async () => {
  const { located } = await serve();
  render(Harness);
  await openReadmePreview();
  await page.getByText("别名").click();
  await expect
    .element(page.getByText("链接目标是符号链接，不在预览中打开"))
    .toBeVisible();
  await page.getByText("缺失").click();
  await expect.element(page.getByText("链接目标不存在")).toBeVisible();
  await page.getByText("示例").click();
  await expect.poll(() => located.at(-1)).toBe("/root/examples");
  await page.getByText("配置", { exact: true }).click();
  await expect
    .element(page.getByRole("tab", { name: /config\.md/ }))
    .toHaveAttribute("aria-selected", "true");
  expect(located).toContain("/root/docs/config.md");
});

test("a path#heading link previews the target file at that heading", async () => {
  await serve();
  render(Harness);
  await openReadmePreview();
  await page.getByText("第二节", { exact: true }).click();
  const heading = page.getByRole("heading", { name: "第二节" });
  await expect.element(heading).toBeVisible();
  await expect
    .poll(() =>
      Math.abs(
        heading.element().getBoundingClientRect().top -
          visiblePreview().getBoundingClientRect().top,
      ),
    )
    .toBeLessThan(2);
});

test("relative images load through file snapshots with the bearer token", async () => {
  localStorage.setItem("webmux.token", "secret");
  const { raw } = await serve();
  render(Harness);
  await openReadmePreview();
  const image = page.getByAltText("图");
  await expect.element(image).toBeVisible();
  expect((image.element() as HTMLImageElement).src).toMatch(/^blob:/);
  expect(raw).toHaveLength(1);
  expect(new Headers(raw[0]!.headers).get("Authorization")).toBe(
    "Bearer secret",
  );
  await expect
    .element(page.getByText("超过 20 MiB 预览上限（img/big.png）"))
    .toBeVisible();
  await expect
    .element(page.getByText("不是可预览的图片（img/notes.png）"))
    .toBeVisible();
  await image.click();
  await expect
    .element(page.getByRole("tab", { name: /a\.png/ }))
    .toHaveAttribute("aria-selected", "true");
});
