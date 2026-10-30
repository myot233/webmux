import { afterEach, expect, test, vi } from "vitest";
import { page, userEvent } from "vitest/browser";
import { cleanup, render } from "vitest-browser-svelte/pure";
import CodeMarkdownPreview from "./CodeMarkdownPreviewHarness.svelte";
import type { MarkdownImage, MarkdownWorkspace } from "../../src/markdown-link";

afterEach(async () => {
  await cleanup();
  await userEvent.cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  document.documentElement.classList.remove("dark");
});

const preview = () =>
  document.querySelector<HTMLElement>('[aria-label="Markdown 预览"]')!;
// Every request the page makes; previews must never fetch on their own.
function watchFetch() {
  const requests: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>(async (input) => {
      requests.push(String(input));
      return new Response("", { status: 404 });
    }),
  );
  return requests;
}
function workspace(
  image: MarkdownWorkspace["image"] = async () => ({
    ok: false,
    reason: "图片不存在",
  }),
) {
  const opened: { path: string; heading?: string }[] = [];
  let reply = "";
  return {
    opened,
    reply: (value: string) => (reply = value),
    value: {
      async open(path: string, heading?: string) {
        opened.push({ path, heading });
        return reply;
      },
      image,
    } satisfies MarkdownWorkspace,
  };
}
async function png(width: number, height: number) {
  const canvas = new OffscreenCanvas(width, height);
  const context = canvas.getContext("2d")!;
  context.fillStyle = "#3c3";
  context.fillRect(0, 0, width, height);
  return canvas.convertToBlob({ type: "image/png" });
}
const filler = (count: number) =>
  Array.from({ length: count }, (_, i) => `段落 ${i + 1}`).join("\n\n");

test("scripts, frames, external images, unsafe links and author ids stay inert", async () => {
  const requests = watchFetch();
  const markdown = [
    "# Title",
    "<script>window.mdScript = true</scr" + "ipt>",
    '<iframe src="https://example.com/frame"></iframe>',
    '<img src="https://example.com/a.png" alt="远程">',
    "![外部](https://example.com/b.png)",
    '<a href="javascript:alert(1)">危险</a> <a href="data:text/html,x">数据</a> <a href="file:///etc/passwd">文件</a>',
    '<div id="app">作者</div><a name="top">锚</a>',
    '<span data-md-href="../secret.md" data-md-open="x">伪造</span>',
    "[相对](docs/other.md) [网站](https://example.com) [邮件](mailto:a@example.com)",
    '<img src="docs/a.png" alt="相对图片">',
    '<map name="m"><area href="docs/x.md" shape="default" alt="区域"></map>',
  ].join("\n\n");
  render(CodeMarkdownPreview, { text: markdown, path: "README.md" });
  await expect
    .element(page.getByRole("heading", { name: "Title" }))
    .toBeVisible();
  const root = preview();
  expect(root.querySelector("script, iframe, img, svg, map, area")).toBeNull();
  expect("mdScript" in window).toBe(false);
  expect(root.textContent).toContain("图片：远程（https://example.com/a.png）");
  expect(root.textContent).toContain("图片：外部（https://example.com/b.png）");
  for (const name of ["危险", "数据", "文件"]) {
    const link = page.getByText(name).element();
    expect(link.hasAttribute("href")).toBe(false);
    expect(link.hasAttribute("data-md-href")).toBe(false);
  }
  expect(root.querySelector("#app, [name]")).toBeNull();
  expect(document.querySelectorAll("#app").length).toBe(0);
  const forged = page.getByText("伪造").element();
  expect(forged.hasAttribute("data-md-href")).toBe(false);
  expect(forged.hasAttribute("data-md-open")).toBe(false);
  // Relative addresses never reach src/href with their original value.
  for (const element of root.querySelectorAll("[src], [href]"))
    expect(element.getAttribute("src") ?? element.getAttribute("href")).toMatch(
      /^(?:https:|mailto:|blob:)/,
    );
  const site = root.querySelector<HTMLAnchorElement>(
    'a[href="https://example.com"]',
  )!;
  expect(site.target).toBe("_blank");
  expect(site.rel).toBe("noopener noreferrer");
  expect(
    root.querySelector('a[href="mailto:a@example.com"]')?.getAttribute("rel"),
  ).toBe("noopener noreferrer");
  expect(
    page
      .getByText("相对", { exact: true })
      .element()
      .getAttribute("data-md-href"),
  ).toBe("docs/other.md");
  expect(requests.filter((url) => url.includes("example.com"))).toEqual([]);
});

test("chat messages keep images, relative links and headings inert", async () => {
  const requests = watchFetch();
  render(CodeMarkdownPreview, {
    text: '## 标题\n\n![截图](docs/a.png)\n\n[说明](README.md)\n\n<img src="docs/b.png">\n\n<div id="chat-app">作者</div><a name="top">锚</a>',
    variant: "message",
  });
  await expect
    .element(page.getByRole("heading", { name: "标题" }))
    .toBeVisible();
  const heading = page.getByRole("heading", { name: "标题" }).element();
  expect(heading.hasAttribute("id")).toBe(false);
  await expect
    .element(page.getByText("图片：截图（docs/a.png）"))
    .toBeVisible();
  const link = page.getByText("说明").element();
  expect(link.hasAttribute("href")).toBe(false);
  expect(link.hasAttribute("data-md-href")).toBe(false);
  expect(
    document.querySelector(
      "img, [data-md-src], [data-md-href], #chat-app, a[name]",
    ),
  ).toBeNull();
  expect(requests).toEqual([]);
});

test("relative links open workspace paths and report failures in place", async () => {
  watchFetch();
  const space = workspace();
  render(CodeMarkdownPreview, {
    text: [
      "[配置](config.md) [根说明](../../README.md) [示例](./examples/)",
      "[部署](/docs/deploy.md?x=1#systemd) [外部](../../../../etc/passwd)",
      "[缺失](missing.md)",
    ].join("\n\n"),
    path: "docs/guide/intro.md",
    workspace: space.value,
  });
  const before = location.href;
  await page.getByText("配置").click();
  await page.getByText("根说明").click({ modifiers: ["ControlOrMeta"] });
  await page.getByText("示例").click({ modifiers: ["Shift"] });
  await page.getByText("部署").click();
  expect(space.opened).toEqual([
    { path: "docs/guide/config.md", heading: "" },
    { path: "README.md", heading: "" },
    { path: "docs/guide/examples", heading: "" },
    { path: "docs/deploy.md", heading: "systemd" },
  ]);
  await page.getByText("外部").click();
  await expect.element(page.getByText("链接目标不在工作区内")).toBeVisible();
  expect(space.opened).toHaveLength(4);
  space.reply("链接目标不存在");
  await page.getByText("缺失").click();
  await expect.element(page.getByText("链接目标不存在")).toBeVisible();
  // Keyboard activation follows the same path.
  space.reply("");
  (page.getByText("配置").element() as HTMLElement).focus();
  await userEvent.keyboard("{Enter}");
  expect(space.opened.at(-1)).toEqual({
    path: "docs/guide/config.md",
    heading: "",
  });
  expect(location.href).toBe(before);
});

test("heading anchors scroll inside the preview without touching the address", async () => {
  watchFetch();
  const text = [
    "[第二个安装](#安装-1) [缺少](#nowhere) [自身](intro.md#用法)",
    "## 安装",
    filler(40),
    "## 安装",
    filler(40),
    "## 用法",
    filler(40),
  ].join("\n\n");
  render(CodeMarkdownPreview, { text, path: "docs/intro.md" });
  const headings = page.getByRole("heading", { name: "安装" });
  await expect.element(headings.first()).toBeVisible();
  expect(headings.elements().map((element) => element.id)).toEqual([
    "md-h-安装",
    "md-h-安装-1",
  ]);
  const before = location.href;
  await page.getByText("第二个安装").click();
  const top = () =>
    headings.elements()[1]!.getBoundingClientRect().top -
    preview().getBoundingClientRect().top;
  await expect.poll(top).toBeLessThan(2);
  expect(top()).toBeGreaterThan(-2);
  expect(location.href).toBe(before);
  await page.getByText("自身").click();
  await expect
    .poll(
      () =>
        page
          .getByRole("heading", { name: "用法" })
          .element()
          .getBoundingClientRect().top - preview().getBoundingClientRect().top,
    )
    .toBeLessThan(2);
  await page.getByText("缺少").click();
  await expect.element(page.getByText("未找到标题“nowhere”")).toBeVisible();
  expect(location.href).toBe(before);
});

test("relative images load as object URLs and failures explain themselves", async () => {
  watchFetch();
  const blob = await png(40, 20);
  const created = vi.spyOn(URL, "createObjectURL");
  const reasons: Record<string, MarkdownImage> = {
    "docs/a.png": { ok: false, reason: "图片不存在" },
    "docs/b.png": { ok: false, reason: "不是可预览的图片" },
    "docs/c.png": { ok: false, reason: "超过 20 MiB 预览上限" },
  };
  const space = workspace(
    async (path) => reasons[path] ?? { ok: true, blob, path },
  );
  render(CodeMarkdownPreview, {
    text: [
      "![首页](img/home.png)",
      '<img src="img/logo.svg" width="80" alt="标志">',
      "![a](a.png) ![b](b.png) ![c](c.png)",
      "![越界](../../x.png)",
      "正文仍然显示",
    ].join("\n\n"),
    path: "docs/README.md",
    workspace: space.value,
  });
  await expect.element(page.getByAltText("首页")).toBeVisible();
  const logo = page.getByAltText("标志").element() as HTMLImageElement;
  expect(logo.getAttribute("width")).toBe("80");
  expect(logo.src).toMatch(/^blob:/);
  expect(created).toHaveBeenCalledTimes(2);
  await expect.element(page.getByText("图片不存在（a.png）")).toBeVisible();
  await expect
    .element(page.getByText("不是可预览的图片（b.png）"))
    .toBeVisible();
  await expect
    .element(page.getByText("超过 20 MiB 预览上限（c.png）"))
    .toBeVisible();
  await expect
    .element(page.getByText("图片不在工作区内（../../x.png）"))
    .toBeVisible();
  await expect.element(page.getByText("正文仍然显示")).toBeVisible();
  await page.getByAltText("首页").click();
  expect(space.opened.at(-1)).toEqual({
    path: "docs/img/home.png",
    heading: undefined,
  });
});

test("busy transfers can be retried", async () => {
  watchFetch();
  const blob = await png(10, 10);
  let busy = true;
  const space = workspace(async (path) =>
    busy
      ? { ok: false, reason: "图片传输繁忙，可重试", retry: true }
      : { ok: true, blob, path },
  );
  render(CodeMarkdownPreview, {
    text: "![图](a.png)",
    path: "README.md",
    workspace: space.value,
  });
  await expect
    .element(page.getByText("图片传输繁忙，可重试（a.png）"))
    .toBeVisible();
  busy = false;
  await page.getByRole("button", { name: "重试加载图片" }).click();
  await expect.element(page.getByAltText("图")).toBeVisible();
});

test("image reads run two at a time, stop at fifty and are cancelled when hidden", async () => {
  watchFetch();
  const blob = await png(4, 4);
  const pending: { resolve: (value: MarkdownImage) => void; path: string }[] =
    [];
  const signals: AbortSignal[] = [];
  let running = 0;
  let peak = 0;
  const space = workspace((path, signal) => {
    signals.push(signal);
    running++;
    peak = Math.max(peak, running);
    const { promise, resolve } = Promise.withResolvers<MarkdownImage>();
    pending.push({
      path,
      resolve: (value) => {
        running--;
        resolve(value);
      },
    });
    return promise;
  });
  const revoked = vi.spyOn(URL, "revokeObjectURL");
  const text = Array.from({ length: 52 }, (_, i) => `![p${i}](p${i}.png)`).join(
    " ",
  );
  const view = await render(CodeMarkdownPreview, {
    text,
    path: "README.md",
    workspace: space.value,
  });
  await expect.poll(() => pending.length).toBe(2);
  pending[0]!.resolve({ ok: true, blob, path: pending[0]!.path });
  pending[1]!.resolve({ ok: true, blob, path: pending[1]!.path });
  await expect.poll(() => pending.length).toBe(4);
  expect(peak).toBe(2);
  await expect.element(page.getByAltText("p0")).toBeVisible();
  expect(preview().textContent).toContain("图片过多，未加载（p50.png）");
  expect(preview().textContent).toContain("图片过多，未加载（p51.png）");
  await view.rerender({ active: false });
  expect(signals.every((signal) => signal.aborted)).toBe(true);
  expect(revoked).toHaveBeenCalledTimes(2);
  expect(preview().querySelector("img")).toBeNull();
  expect(peak).toBe(2);
});

test("fenced code is highlighted by language, unknown languages stay plain", async () => {
  watchFetch();
  render(CodeMarkdownPreview, {
    text: [
      "```ts",
      'const a: string = "x";',
      "```",
      "```go",
      'func main() { println("x") }',
      "```",
      "```bash",
      'echo "x" && exit 0',
      "```",
      "```nosuchlanguage",
      "plain text",
      "```",
      "```html",
      "<img src=x onerror=alert(1)>",
      "```",
    ].join("\n"),
    path: "README.md",
  });
  const blocks = () => [...preview().querySelectorAll("pre > code")];
  await expect
    .poll(() => blocks().map((code) => !!code.querySelector("span[class]")))
    .toEqual([true, true, true, false, true]);
  expect(blocks()[3]!.children.length).toBe(0);
  expect(blocks()[3]!.textContent).toBe("plain text\n");
  const html = blocks()[4]!;
  expect(html.querySelector("img")).toBeNull();
  expect(html.textContent).toBe("<img src=x onerror=alert(1)>\n");
  const keyword = [...blocks()[0]!.querySelectorAll("span")].find(
    (span) => span.textContent === "const",
  )!;
  const light = getComputedStyle(keyword).color;
  document.documentElement.classList.add("dark");
  expect(getComputedStyle(keyword).color).not.toBe(light);
});
