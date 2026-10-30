import { writeFile } from "node:fs/promises";
import { join } from "node:path";
import type { Page, Request } from "@playwright/test";
import { test, expect, createShell } from "./fixtures";
import { pdfFixture } from "../fixtures/pdf";

async function showCode(page: Page) {
  await page
    .locator(".terminal-header")
    .getByRole("button", { name: "代码", exact: true })
    .click();
  await expect(page.getByRole("region", { name: "代码工作区" })).toBeVisible();
}
async function open(page: Page, name: string) {
  const navigation = page.getByRole("button", {
    name: "文件导航",
    exact: true,
  });
  if (await navigation.isVisible()) await navigation.click();
  await page.getByRole("button", { name, exact: true }).click();
  // Controls of the previous tab stay mounted; act only once this one shows.
  await expect(
    page.getByRole("tab", { name: `${name} · worktree`, exact: true }),
  ).toHaveAttribute("aria-selected", "true");
}
// Below 640px the document bar's controls live in the code menu. Inactive
// tabs stay mounted, so pick the visible document bar's button.
async function togglePreview(page: Page, menuItem: string) {
  if (page.viewportSize()!.width < 640) {
    await page.getByRole("button", { name: "代码菜单", exact: true }).click();
    await page.getByRole("menuitem", { name: menuItem, exact: true }).click();
  } else
    await page
      .getByRole("button", {
        name: menuItem === "返回源码" ? "返回源码" : "预览",
        exact: true,
      })
      .filter({ visible: true })
      .click();
}
const renderer =
  /code-pdf|pdf\.worker|\.bcmap|\.pfb|LiberationSans|openjpeg|jbig2/;
// Service worker requests have no frame; they are never the preview's.
function fromSubframe(page: Page, request: Request) {
  try {
    return request.frame() !== page.mainFrame();
  } catch {
    return false;
  }
}

test("previews HTML, CSV and JSON statically and PDF on demand", async ({
  page,
  workDir,
}) => {
  await writeFile(
    join(workDir, "report.html"),
    `<!doctype html><html><head>
<style>.title { color: rgb(0, 128, 0); }</style>
<link rel="stylesheet" href="a.css">
<meta http-equiv="refresh" content="0;url=https://html-leak.invalid/refresh">
</head><body>
<h1 class="title">构建报告</h1>
<img alt="icon" src="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='4' height='4'/%3E">
<img alt="remote" src="https://html-leak.invalid/x.png">
<script>fetch("https://html-leak.invalid/script")</script>
<form action="/api/sessions" method="post"><button>提交</button></form>
<a href="https://html-leak.invalid/link">外链</a>
</body></html>
`,
  );
  await writeFile(
    join(workDir, "data.csv"),
    'name,note\n"a,b","他说""好"""\nplain,"第一行\n第二行"\n',
  );
  await writeFile(
    join(workDir, "result.json"),
    '{"id": 12345678901234567890, "ratio": 1.10, "items": [1, 2, 3]}\n',
  );
  await writeFile(
    join(workDir, "manual.pdf"),
    pdfFixture({
      pages: 30,
      internalTarget: 30,
      externalUrl: "https://example.com/",
      script: true,
    }),
  );
  const requests: Request[] = [];
  page.on("request", (request) => requests.push(request));
  await createShell(page, "Document preview", workDir);
  await showCode(page);

  // HTML: a static, sandboxed frame that issues no requests at all.
  await open(page, "report.html");
  await expect(
    page.locator("[data-code-reader]:visible .cm-content"),
  ).toContainText("构建报告");
  const before = requests.length;
  await togglePreview(page, "预览 HTML");
  await expect(
    page.getByText("静态预览，脚本与外部资源已禁用").filter({ visible: true }),
  ).toBeVisible();
  const frame = page.frameLocator('iframe[title="HTML 静态预览"]');
  await expect(frame.getByRole("heading", { name: "构建报告" })).toHaveCSS(
    "color",
    "rgb(0, 128, 0)",
  );
  await expect(frame.getByRole("img", { name: "icon" })).toBeVisible();
  await frame.getByRole("button", { name: "提交" }).click();
  await frame.getByText("外链").click();
  // An absent request has no event to await; give a refresh, script or
  // submission time to fire before checking that none did.
  await page.waitForTimeout(500);
  const fromPreview = requests.slice(before).filter(
    (request) =>
      // Inline data: images are decoded locally, not fetched.
      !request.url().startsWith("data:") &&
      (fromSubframe(page, request) ||
        /html-leak\.invalid|\/a\.css$/.test(request.url()) ||
        (request.method() === "POST" &&
          request.url().endsWith("/api/sessions"))),
  );
  expect(fromPreview.map((request) => request.url())).toEqual([]);
  await togglePreview(page, "返回源码");
  await expect(
    page.locator("[data-code-reader]:visible .cm-content"),
  ).toBeVisible();

  // CSV: quoted fields and multi-line cells in a table.
  await open(page, "data.csv");
  await togglePreview(page, "预览表格");
  const table = page.getByRole("table", { name: "表格预览" });
  await expect(table.getByRole("columnheader", { name: "note" })).toBeVisible();
  await expect(table.getByRole("cell", { name: "a,b" })).toBeVisible();
  await expect(table.getByRole("cell", { name: '他说"好"' })).toBeVisible();
  await expect(
    table.getByRole("cell", { name: /第一行\s*第二行/ }),
  ).toBeVisible();

  // JSON: a collapsible tree that keeps the number text.
  await open(page, "result.json");
  await togglePreview(page, "预览 JSON");
  const tree = page.getByLabel("JSON 预览");
  await expect(tree).toContainText("12345678901234567890");
  await expect(tree).toContainText("1.10");
  await expect(tree).toContainText("3 项");

  // Nothing so far loaded the PDF renderer.
  expect(requests.filter((request) => renderer.test(request.url()))).toEqual(
    [],
  );

  // PDF: opens straight into the preview with bundled CJK resources.
  await open(page, "manual.pdf");
  const viewer = page.getByRole("group", { name: "PDF 预览" });
  await expect(viewer).toBeVisible();
  await expect(page.getByText("/ 30", { exact: true })).toBeVisible();
  const firstPage = viewer.getByRole("region", { name: "第 1 页" });
  await expect(firstPage.locator(".textLayer")).toContainText("中文测试");
  const loaded = requests.filter((request) => renderer.test(request.url()));
  expect(loaded.some((request) => /\.bcmap/.test(request.url()))).toBe(true);
  expect(
    loaded.every(
      (request) => new URL(request.url()).origin === new URL(page.url()).origin,
    ),
  ).toBe(true);
  const external = firstPage.getByRole("link", {
    name: "打开链接 https://example.com/",
  });
  await expect(external).toHaveAttribute("target", "_blank");
  await expect(external).toHaveAttribute("rel", "noopener noreferrer");
  await firstPage.getByRole("link", { name: "跳到第 30 页" }).click();
  await expect(page.getByLabel("页码")).toHaveValue("30");
  await expect(
    viewer.getByRole("region", { name: "第 30 页" }).locator(".textLayer"),
  ).toContainText("第 30 页");
  await page.getByLabel("页码").fill("5");
  await page.getByLabel("页码").press("Enter");
  await expect(
    viewer.getByRole("region", { name: "第 5 页" }).locator(".textLayer"),
  ).toContainText("中文测试");
  // Only pages near the viewport hold a canvas.
  await expect
    .poll(() => viewer.locator("canvas").count())
    .toBeLessThanOrEqual(9);
  expect(
    await viewer
      .getByRole("region", { name: "第 30 页" })
      .locator("canvas")
      .count(),
  ).toBe(0);
  expect(await page.evaluate(() => "pdfScript" in window)).toBe(false);
});
