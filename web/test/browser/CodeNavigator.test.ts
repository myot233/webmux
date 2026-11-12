import { afterEach, expect, test, vi } from "vitest";
import { page, userEvent } from "vitest/browser";
import { cleanup, render } from "vitest-browser-svelte/pure";
import Harness from "./CodeNavigatorHarness.svelte";
import CodeChangesList from "../../src/components/code/CodeChangesList.svelte";
import { nodeId } from "../../src/domains/code/code-reference";
import type { CodeListing } from "../../src/contracts/schemas";
import type * as Api from "../../src/platform/api";

const uploads = vi.hoisted(() => ({
  calls: [] as { path: string; name: string }[],
  hold: undefined as Promise<void> | undefined,
}));
vi.mock("../../src/platform/api", async (original) => ({
  ...(await original<typeof Api>()),
  uploadForm: async (path: string, file: File) => {
    uploads.calls.push({ path, name: file.name });
    await uploads.hold;
    return { nodes: [{ name: file.name }] };
  },
}));

function listing(paths: string[]): CodeListing {
  return {
    children: paths.map((entry) => {
      const directory = entry.endsWith("/");
      const path = entry.replace(/\/$/, "");
      return {
        nodeId: nodeId(path),
        path,
        name: path.split("/").at(-1)!,
        kind: directory ? "directory" : "file",
        gitStatus: "",
        ignored: false,
        referenceable: !directory,
        mediaType: "",
      };
    }),
    nextCursor: "",
    complete: true,
    limited: false,
    contentVersion: "v1",
    checkedAt: "2026-10-25T00:00:00Z",
  };
}
const context = (writableAvailable = true) => ({
  contextId: "c1",
  rootPath: "/workspace/webmux",
  rootKind: "directory" as const,
  browseAvailable: true,
  diffAvailable: true,
  ignoreAvailable: true,
  reason: "",
  writableAvailable,
});
async function setup(writable = true) {
  const fetchDirectory = vi.fn(async (path: string) =>
    path === "docs" ? listing(["docs/architecture.md"]) : listing([]),
  );
  const changed = vi.fn();
  const view = await render(Harness, {
    props: {
      context: context(writable),
      rootListing: listing(["docs/", "README.md"]),
      fetchDirectory,
      changed,
    },
  });
  return { fetchDirectory, changed, view };
}
const row = (name: string) =>
  page.getByRole("button", { name, exact: true }).element();
const status = page.getByLabelText("拖放状态");
const navigator = page.getByRole("navigation", { name: "文件与修改导航" });
const clipboard = page.getByRole("status", { name: "剪贴板" });
function files(...names: string[]) {
  const transfer = new DataTransfer();
  for (const name of names) transfer.items.add(new File([name], name));
  return transfer;
}
function drag(target: Element, type: string, transfer: DataTransfer) {
  const event = new DragEvent(type, {
    dataTransfer: transfer,
    bubbles: true,
    cancelable: true,
  });
  target.dispatchEvent(event);
  return event;
}
function drop(target: Element, transfer: DataTransfer) {
  drag(target, "dragenter", transfer);
  return drag(target, "drop", transfer);
}
// Chromium ignores dropEffect on a synthetic DataTransfer outside a real
// drag session, so the effect is read from a stand-in transfer.
function dropEffect(target: Element) {
  const transfer = { types: ["Files"], dropEffect: "" };
  const event = new Event("dragover", { bubbles: true, cancelable: true });
  Object.defineProperty(event, "dataTransfer", { value: transfer });
  target.dispatchEvent(event);
  return transfer.dropEffect;
}
afterEach(async () => {
  await cleanup();
  await userEvent.cleanup();
  uploads.calls = [];
  uploads.hold = undefined;
});

test("drop targets the folder row, a file row's parent, or the root elsewhere", async () => {
  const { changed } = await setup();
  await expect.element(page.getByText("README.md")).toBeVisible();
  const docs = row("docs");
  const transfer = files("a.txt");
  drag(docs, "dragenter", transfer);
  drag(docs, "dragover", transfer);
  await expect.element(status).toHaveTextContent("松开以上传到 docs");
  await expect
    .element(page.getByRole("button", { name: "docs", exact: true }))
    .toHaveAttribute("data-drop-target", "true");
  expect(dropEffect(docs)).toBe("copy");
  drag(docs, "drop", transfer);
  await expect.element(status).toHaveTextContent("已上传 a.txt");
  expect(uploads.calls).toEqual([
    {
      path: `/sessions/s1/code/upload?contextId=c1&nodeId=${encodeURIComponent(nodeId("docs"))}`,
      name: "a.txt",
    },
  ]);
  expect(changed).toHaveBeenCalledWith([]);
  await expect
    .element(page.getByRole("button", { name: "docs", exact: true }))
    .not.toHaveAttribute("data-drop-target");

  await page.getByRole("button", { name: "docs", exact: true }).click();
  const file = row("architecture.md");
  drag(file, "dragenter", files("b.txt"));
  await expect.element(status).toHaveTextContent("松开以上传到 docs");
  drag(file, "drop", files("b.txt"));
  await expect.element(status).toHaveTextContent("已上传 b.txt");
  expect(uploads.calls[1]?.path).toContain(
    `nodeId=${encodeURIComponent(nodeId("docs"))}`,
  );

  const readme = row("README.md");
  drag(readme, "dragenter", files("c.txt"));
  await expect.element(status).toHaveTextContent("松开以上传到 webmux");
  await expect
    .element(page.getByRole("button", { name: "webmux", exact: true }))
    .toHaveAttribute("data-drop-target", "true");
  drag(readme, "drop", files("c.txt"));
  await expect.element(status).toHaveTextContent("已上传 c.txt");

  drop(page.getByTestId("document").element(), files("d.txt"));
  await expect.element(status).toHaveTextContent("已上传 d.txt");
  expect(uploads.calls.slice(2).map((call) => call.path)).toEqual([
    "/sessions/s1/code/upload?contextId=c1&nodeId=",
    "/sessions/s1/code/upload?contextId=c1&nodeId=",
  ]);
});

test("drags without files are left alone", async () => {
  await setup();
  await expect.element(page.getByText("README.md")).toBeVisible();
  const transfer = new DataTransfer();
  transfer.setData("text/plain", "hello");
  const event = drop(row("docs"), transfer);
  expect(event.defaultPrevented).toBe(false);
  await expect.element(status).toHaveTextContent("");
  expect(uploads.calls).toEqual([]);
});

test("a read-only root refuses drops", async () => {
  await setup(false);
  await expect.element(page.getByText("README.md")).toBeVisible();
  const transfer = files("a.txt");
  drag(row("docs"), "dragenter", transfer);
  drag(row("docs"), "dragover", transfer);
  expect(dropEffect(row("docs"))).toBe("none");
  await expect.element(status).toHaveTextContent("当前目录只读，无法上传");
  drag(row("docs"), "drop", transfer);
  await expect
    .element(page.getByRole("alert"))
    .toHaveTextContent("当前目录只读，无法上传");
  expect(uploads.calls).toEqual([]);
});

test("a drop containing a folder uploads nothing", async () => {
  await setup();
  await expect.element(page.getByText("README.md")).toBeVisible();
  const event = new Event("drop", { bubbles: true, cancelable: true });
  Object.defineProperty(event, "dataTransfer", {
    value: {
      types: ["Files"],
      items: [
        { webkitGetAsEntry: () => ({ isDirectory: true }) },
        { webkitGetAsEntry: () => ({ isDirectory: false }) },
      ],
      files: [new File(["x"], "x.txt"), new File(["y"], "folder")],
    },
  });
  row("docs").dispatchEvent(event);
  await expect
    .element(page.getByRole("alert"))
    .toHaveTextContent("不支持上传文件夹");
  await expect.element(status).toHaveTextContent("不支持上传文件夹");
  expect(uploads.calls).toEqual([]);
});

test("a drop during an upload is refused and the upload continues", async () => {
  await setup();
  await expect.element(page.getByText("README.md")).toBeVisible();
  const { promise, resolve } = Promise.withResolvers<void>();
  uploads.hold = promise;
  drop(row("docs"), files("a.txt"));
  await expect.element(page.getByLabelText("文件上传状态")).toBeVisible();
  const transfer = files("b.txt");
  expect(dropEffect(row("docs"))).toBe("none");
  drag(row("docs"), "drop", transfer);
  await expect.element(status).toHaveTextContent("请等待当前上传完成");
  expect(uploads.calls.map((call) => call.name)).toEqual(["a.txt"]);
  resolve();
  await expect.element(navigator.getByText("已上传 a.txt")).toBeVisible();
});

test("the menu upload uses the same request as a drop", async () => {
  await setup();
  await page.getByRole("button", { name: "文件操作 docs" }).click();
  await page.getByRole("menuitem", { name: "上传到此目录" }).click();
  await page
    .getByLabelText("选择上传到此目录的文件")
    .upload(new File(["m"], "m.txt"));
  await expect.element(navigator.getByText("已上传 m.txt")).toBeVisible();
  drop(row("docs"), files("n.txt"));
  await expect.element(status).toHaveTextContent("已上传 n.txt");
  expect(uploads.calls[0]?.path).toBe(uploads.calls[1]?.path);
});

test("captured folders, listings and clipboard restore without tree requests", async () => {
  const first = await setup();
  await page.getByRole("button", { name: "docs", exact: true }).click();
  await expect.element(page.getByText("architecture.md")).toBeVisible();
  await page.getByRole("button", { name: "文件操作 README.md" }).click();
  await page.getByRole("menuitem", { name: "复制" }).click();
  await expect.element(clipboard).toBeVisible();
  expect(clipboard.element().textContent).toMatch(/已复制\s+README\.md/);
  const state = first.view.component.capture();
  expect(state).toMatchObject({
    rootOpen: true,
    expanded: ["docs"],
    changesTop: 0,
    clipboard: { contextId: "c1", mode: "copy" },
  });
  await cleanup();

  const fetchDirectory = vi.fn(async () => listing([]));
  const second = await render(Harness, {
    props: {
      context: context(),
      rootListing: listing(["docs/", "README.md"]),
      fetchDirectory,
      restore: structuredClone(state),
    },
  });
  await expect.element(page.getByText("architecture.md")).toBeVisible();
  await expect.element(clipboard).toBeVisible();
  expect(clipboard.element().textContent).toMatch(/已复制\s+README\.md/);
  expect(fetchDirectory).not.toHaveBeenCalled();
  expect(second.component.capture()).toEqual(state);
});

test("the changes list restores its first row once a hidden mount gets layout", async () => {
  const host = document.createElement("div");
  host.style.cssText = "display:none;height:200px;flex-direction:column";
  document.body.append(host);
  const entries = Array.from({ length: 60 }, (_, n) => ({
    nodeId: `n${n}`,
    changeId: `change-${n}`,
    group: "unstaged" as const,
    oldPath: `file-${n}.ts`,
    newPath: `file-${n}.ts`,
    status: "M",
    oldOid: "",
    newOid: "",
    oldMode: "",
    newMode: "",
  }));
  const view = await render(CodeChangesList, {
    target: host,
    props: {
      entries,
      total: entries.length,
      nextCursor: "",
      version: "v1",
      pending: false,
      enabled: true,
      onmore: () => {},
      onopen: () => {},
      restore: 20,
    },
  });
  expect(view.component.capture()).toBe(20);
  host.style.display = "flex";
  const list = page.getByRole("list", { name: "修改列表" });
  await expect.poll(() => list.element().scrollTop).toBe(20 * 32);
  expect(view.component.capture()).toBe(20);
  host.style.display = "none";
  expect(view.component.capture()).toBe(20);
  host.remove();
});
