import { mkdir, writeFile } from "node:fs/promises";
import { join } from "node:path";
import type { Page } from "@playwright/test";
import { test, expect, createShell } from "./fixtures";

// Below 640px the document bar's controls live in the code menu.
async function previewMarkdown(page: Page) {
  if (page.viewportSize()!.width < 640) {
    await page.getByRole("button", { name: "代码菜单", exact: true }).click();
    await page.getByRole("menuitem", { name: "预览 Markdown" }).click();
  } else
    await page
      .getByRole("button", { name: "预览", exact: true })
      .filter({ visible: true })
      .click();
}

test("Markdown preview opens relative links, finds headings and loads workspace images", async ({
  page,
  workDir,
}) => {
  const png = Buffer.from(
    await page.evaluate(async () => {
      const canvas = new OffscreenCanvas(48, 24);
      const context = canvas.getContext("2d")!;
      context.fillStyle = "#36c";
      context.fillRect(0, 0, 48, 24);
      const blob = await canvas.convertToBlob({ type: "image/png" });
      return [...new Uint8Array(await blob.arrayBuffer())];
    }),
  );
  await mkdir(join(workDir, "docs/img"), { recursive: true });
  await writeFile(join(workDir, "docs/img/home.png"), png);
  await writeFile(
    join(workDir, "docs/img/logo.svg"),
    '<svg xmlns="http://www.w3.org/2000/svg" width="40" height="40"><circle cx="20" cy="20" r="18" fill="teal"/></svg>\n',
  );
  const filler = Array.from({ length: 60 }, (_, i) => `段落 ${i + 1}`).join(
    "\n\n",
  );
  await writeFile(
    join(workDir, "docs/deploy.md"),
    `# 部署\n\n${filler}\n\n## systemd\n\n${filler}\n`,
  );
  await writeFile(
    join(workDir, "README.md"),
    [
      "# 项目",
      "![首页](docs/img/home.png)",
      '<img src="docs/img/logo.svg" width="80" alt="标志">',
      "![外部](https://markdown-leak.invalid/x.png)",
      "[部署](docs/deploy.md#systemd) [缺失](docs/missing.md)",
    ].join("\n\n"),
  );
  // Any request for the document's images outside the authenticated raw
  // endpoint is a leak: relative addresses resolved by the browser against
  // the app route, or external hosts. The app's own favicon is not.
  const leaks: string[] = [];
  page.on("request", (request) => {
    const url = new URL(request.url());
    if (url.hostname.endsWith(".invalid") || url.pathname.includes("/img/"))
      leaks.push(request.url());
  });
  await createShell(page, "Markdown code", workDir);
  await page
    .locator(".terminal-header")
    .getByRole("button", { name: "代码", exact: true })
    .click();
  await page.getByRole("button", { name: "README.md", exact: true }).click();
  await previewMarkdown(page);
  const preview = page
    .getByRole("region", { name: "Markdown 预览" })
    .filter({ visible: true });
  const home = preview.getByAltText("首页");
  await expect(home).toBeVisible();
  await expect
    .poll(() => home.evaluate((image: HTMLImageElement) => image.naturalWidth))
    .toBe(48);
  expect(await home.getAttribute("src")).toMatch(/^blob:/);
  const logo = preview.getByAltText("标志");
  await expect(logo).toBeVisible();
  expect(await logo.getAttribute("width")).toBe("80");
  await expect(
    preview.getByText("图片：外部（https://markdown-leak.invalid/x.png）"),
  ).toBeVisible();

  await preview.getByText("缺失").click();
  await expect(page.getByText("链接目标不存在")).toBeVisible();
  const before = page.url();
  await preview.getByText("部署", { exact: true }).click();
  const heading = page
    .getByRole("heading", { name: "systemd" })
    .filter({ visible: true });
  await expect(heading).toBeVisible();
  const deploy = page
    .getByRole("region", { name: "Markdown 预览" })
    .filter({ visible: true });
  await expect
    .poll(async () => {
      const [target, box] = await Promise.all([
        heading.boundingBox(),
        deploy.boundingBox(),
      ]);
      return Math.abs(target!.y - box!.y);
    })
    .toBeLessThan(2);
  expect(page.url()).toBe(before);
  expect(leaks).toEqual([]);
});
