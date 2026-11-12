import { afterEach, expect, test, vi } from "vitest";
import { page, userEvent } from "vitest/browser";
import { cleanup, render } from "vitest-browser-svelte/pure";
import ForwardButton from "../../src/components/workspace/ForwardButton.svelte";
import ForwardDialog from "../../src/components/workspace/ForwardDialog.svelte";
import { defaultForwardOptions } from "../../src/domains/workspace/forwards.svelte";
import {
  ForwardRequestSchema,
  ForwardUpdateSchema,
} from "../../src/contracts/schemas";
import { stopSettings } from "../../src/domains/settings/settings.svelte";
import {
  body,
  forward,
  seedForwards,
  session,
  stubFetch,
} from "./forward-fixture";
import { seedSettings } from "./settings-fixture";

afterEach(async () => {
  try {
    await cleanup();
    await userEvent.cleanup();
  } finally {
    stopSettings();
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
    seedForwards([]);
  }
});

const web = session("webwebwe", "web", [{ port: 5173, process: "node" }]);

async function open(sessions = [web], detection = true) {
  await render(ForwardDialog, { sessions, detection, onclose: () => {} });
  return page.getByRole("dialog", { name: "端口转发" });
}
const portField = page.getByRole("textbox", { name: "端口" });
const rows = page.getByRole("region", { name: /已转发/ }).getByRole("listitem");
// Each forward row's visible text, whitespace collapsed.
const rowTexts = () =>
  rows
    .elements()
    .map((row) => (row.textContent ?? "").replace(/\s+/g, " ").trim());

test("invalid and refused ports show an inline error and keep the dialog open", async () => {
  const fetch = stubFetch(() =>
    Response.json(
      { error: "invalid forward: port 7681 is WebMux's own port" },
      { status: 400 },
    ),
  );
  const dialog = await open();
  await portField.fill("70000");
  await expect
    .element(page.getByRole("alert"))
    .toHaveTextContent("请输入 1–65535 的端口号");
  await page.getByRole("button", { name: "添加转发" }).click();
  expect(fetch).not.toHaveBeenCalled();

  await portField.fill("7681");
  await expect.element(page.getByRole("alert")).not.toBeInTheDocument();
  await page.getByRole("button", { name: "添加转发" }).click();
  await expect
    .element(page.getByRole("alert"))
    .toHaveTextContent("invalid forward: port 7681 is WebMux's own port");
  await expect.element(dialog).toBeVisible();
  await expect.element(portField).toHaveValue("7681");
  await expect.element(portField).toHaveAttribute("aria-invalid", "true");
});

test("adding a port sends its compatibility options and lists it", async () => {
  const fetch = stubFetch((_url, init) =>
    Response.json(
      forward(3000, {
        options: ForwardRequestSchema.parse(body(init)).options,
      }),
      { status: 201 },
    ),
  );
  await open([]);
  await expect.element(page.getByText("还没有转发")).toBeVisible();
  await portField.fill("3000");
  await page.getByRole("button", { name: /兼容选项/ }).click();
  await page.getByRole("radio", { name: "保留前缀" }).click();
  await page.getByRole("switch", { name: "改写 Host" }).click();
  await page.getByRole("button", { name: "添加转发" }).click();

  await vi.waitFor(() => expect(fetch).toHaveBeenCalledOnce());
  const [url, init] = fetch.mock.calls[0]!;
  expect(String(url)).toBe("/api/forwards");
  expect(init?.method).toBe("POST");
  expect(body(init)).toEqual({
    port: 3000,
    options: { ...defaultForwardOptions, rewriteHost: false, prefix: "keep" },
  });
  await expect.poll(rowTexts).toEqual(["3000 手动添加 可访问 /forward/3000/"]);
  await expect.element(portField).toHaveValue("");
});

test("each row shows its source, status and path; removal needs no confirmation", async () => {
  seedForwards([
    forward(5173, { session: web.id, process: "node" }),
    forward(8000, { label: "docs", status: "unreachable" }),
  ]);
  const fetch = stubFetch(() => new Response(null, { status: 204 }));
  await open();
  await expect
    .poll(rowTexts)
    .toEqual([
      "5173 web · node 可访问 /forward/5173/",
      "8000 docs 无法连接 /forward/8000/",
    ]);
  await expect
    .element(page.getByRole("link", { name: "在新标签页打开端口 5173" }))
    .toHaveAttribute("href", "/forward/5173/");
  // 5173 is forwarded, so it is no longer a detected port.
  await expect
    .element(page.getByText("会话中没有其他正在监听的端口。"))
    .toBeVisible();

  await page.getByRole("button", { name: "移除端口 5173 的转发" }).click();
  await vi.waitFor(() => expect(fetch).toHaveBeenCalledOnce());
  const [url, init] = fetch.mock.calls[0]!;
  expect(String(url)).toBe("/api/forwards/5173");
  expect(init?.method).toBe("DELETE");
  await expect.poll(rowTexts).toEqual(["8000 docs 无法连接 /forward/8000/"]);
  await expect.element(page.getByRole("alertdialog")).not.toBeInTheDocument();
});

test("settings save the label and options of one forward", async () => {
  seedForwards([forward(5173)]);
  const fetch = stubFetch((_url, init) =>
    Response.json({
      ...forward(5173),
      ...ForwardUpdateSchema.parse(body(init)),
    }),
  );
  await open();
  const settings = page.getByRole("button", { name: "端口 5173 的设置" });
  await settings.click();
  await expect.element(settings).toHaveAttribute("aria-expanded", "true");
  // The default strips the prefix; keeping it asks for the server's base.
  const form = page.getByRole("form", { name: "端口 5173 的设置" });
  await expect
    .element(
      form.getByText("后转发，适合只使用相对路径的服务", { exact: false }),
    )
    .toBeVisible();
  await page.getByRole("textbox", { name: "名称（可选）" }).fill(" 前端 ");
  await form.getByRole("radio", { name: "保留前缀" }).click();
  await expect
    .element(form.getByText("--base /forward/5173/", { exact: false }))
    .toBeVisible();
  await page
    .getByRole("form", { name: "端口 5173 的设置" })
    .getByRole("switch", { name: "改写重定向和 Cookie" })
    .click();
  await page.getByRole("button", { name: "保存" }).click();

  await vi.waitFor(() => expect(fetch).toHaveBeenCalledOnce());
  const [url, init] = fetch.mock.calls[0]!;
  expect(String(url)).toBe("/api/forwards/5173");
  expect(init?.method).toBe("PATCH");
  expect(body(init)).toEqual({
    label: "前端",
    options: {
      ...defaultForwardOptions,
      rewriteResponse: false,
      prefix: "keep",
    },
  });
  await expect
    .element(page.getByRole("form", { name: "端口 5173 的设置" }))
    .not.toBeInTheDocument();
  await expect.poll(rowTexts).toEqual(["5173 前端 可访问 /forward/5173/"]);
});

test("copy link writes the full address of the forward", async () => {
  seedForwards([forward(5173)]);
  const write = vi
    .spyOn(navigator.clipboard, "writeText")
    .mockResolvedValue(undefined);
  await open();
  const copy = page.getByRole("button", { name: "复制端口 5173 的链接" });
  await copy.click();
  expect(write).toHaveBeenCalledWith(`${location.origin}/forward/5173/`);
  await expect.element(copy).toHaveAttribute("title", "已复制");
});

test("detected ports forward with their session and process", async () => {
  const fetch = stubFetch(() =>
    Response.json(forward(5173, { session: web.id, process: "node" }), {
      status: 201,
    }),
  );
  await open();
  const detected = page
    .getByRole("region", { name: "检测到的端口" })
    .getByRole("listitem");
  await expect
    .poll(() => detected.elements().map((item) => item.textContent?.trim()))
    .toEqual([expect.stringMatching(/^5173\s+node · web\s+转发$/)]);
  await page.getByRole("button", { name: "转发端口 5173" }).click();
  await vi.waitFor(() => expect(fetch).toHaveBeenCalledOnce());
  expect(body(fetch.mock.calls[0]![1])).toEqual({
    port: 5173,
    session: web.id,
    process: "node",
  });
  await expect
    .poll(rowTexts)
    .toEqual(["5173 web · node 可访问 /forward/5173/"]);
  await expect.element(detected).not.toBeInTheDocument();
});

test("a platform without detection says so", async () => {
  await open([], false);
  await expect
    .element(page.getByText("当前平台不支持自动检测", { exact: false }))
    .toBeVisible();
});

test("detected ports hide 30000 and above unless ignoreHighPorts is false; manual add still accepts them", async () => {
  await seedSettings();
  const high = session("highport", "rpc", [
    { port: 29999, process: "vite" },
    { port: 30000, process: "node" },
    { port: 42000, process: "go" },
  ]);
  await open([high]);
  const detected = page
    .getByRole("region", { name: "检测到的端口" })
    .getByRole("listitem");
  await expect
    .poll(() => detected.elements().map((item) => item.textContent?.trim()))
    .toEqual([expect.stringMatching(/^29999\s+vite · rpc\s+转发$/)]);

  await seedSettings({ ignoreHighPorts: false });
  await expect
    .poll(() => detected.elements().map((item) => item.textContent?.trim()))
    .toEqual([
      expect.stringMatching(/^29999\s+vite · rpc\s+转发$/),
      expect.stringMatching(/^30000\s+node · rpc\s+转发$/),
      expect.stringMatching(/^42000\s+go · rpc\s+转发$/),
    ]);
});

test("the header button names its forward count", async () => {
  const onclick = vi.fn();
  await render(ForwardButton, { onclick });
  const button = page.getByRole("button", { name: "端口转发", exact: true });
  await expect.element(button).toHaveAttribute("title", "端口转发");
  seedForwards([forward(5173), forward(8000)]);
  const counted = page.getByRole("button", { name: "端口转发（2 个）" });
  await expect.element(counted).toHaveTextContent("2");
  await expect.element(counted).toHaveAttribute("title", "端口转发（2 个）");
  await counted.click();
  expect(onclick).toHaveBeenCalledOnce();
});
