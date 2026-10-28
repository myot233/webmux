import "../../src/styles.css";
import { beforeEach, expect, test, vi } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte/pure";
import Harness from "./ChatViewHarness.svelte";
import {
  chatState,
  installChatFixture,
  snapshot,
  socketAt,
} from "./chat-fixture";

installChatFixture();

const id = "hybrid22";

class FakeXHR {
  static pending: FakeXHR[] = [];
  upload: { onprogress: ((event: ProgressEvent) => void) | null } = {
    onprogress: null,
  };
  status = 0;
  statusText = "";
  response: unknown;
  responseType = "";
  timeout = 0;
  onload: (() => void) | null = null;
  onerror: (() => void) | null = null;
  onabort: (() => void) | null = null;
  ontimeout: (() => void) | null = null;

  open() {}
  setRequestHeader() {}
  send() {
    FakeXHR.pending.push(this);
  }
  abort() {
    this.onabort?.();
  }
}

beforeEach(() => {
  FakeXHR.pending = [];
  vi.stubGlobal("XMLHttpRequest", FakeXHR);
});

async function open() {
  render(Harness, { id });
  const socket = await socketAt(`/ws/s/${id}/chat`);
  socket.frame(snapshot(chatState()));
  await expect
    .element(page.getByRole("textbox", { name: "消息" }))
    .toBeVisible();
  return socket;
}

function choose(files: File[]) {
  const input = page
    .getByLabelText("选择上传文件")
    .element() as HTMLInputElement;
  const transfer = new DataTransfer();
  for (const file of files) transfer.items.add(file);
  input.files = transfer.files;
  input.dispatchEvent(new Event("change", { bubbles: true }));
}

function report(loaded: number, total: number) {
  FakeXHR.pending.at(-1)?.upload.onprogress?.({
    lengthComputable: true,
    loaded,
    total,
  } as ProgressEvent);
}

function finish(path: string) {
  const xhr = FakeXHR.pending.at(-1)!;
  xhr.status = 201;
  xhr.response = { path };
  xhr.onload?.();
}

const draft = () => page.getByLabelText("Draft");
const error = () => page.getByLabelText("Upload error");

test("a chosen image shows progress, disables sending, then inserts its path", async () => {
  await open();
  const input = page.getByRole("textbox", { name: "消息" });
  await input.fill("看看这张图");
  choose([new File(["png"], "shot.png", { type: "image/png" })]);

  await expect
    .element(page.getByRole("status").filter({ hasText: "正在上传 shot.png" }))
    .toBeVisible();
  report(50, 100);
  await expect.element(page.getByText("50%")).toBeVisible();
  await expect
    .element(page.getByRole("button", { name: "发送", exact: true }))
    .toBeDisabled();
  await expect
    .element(page.getByRole("button", { name: "上传文件" }))
    .toBeDisabled();

  finish("/tmp/webmux-upload-1/shot.png");
  await expect
    .poll(() => draft().element().textContent)
    .toBe("看看这张图\n`/tmp/webmux-upload-1/shot.png`\n");
  await expect
    .element(page.getByRole("progressbar", { name: "文件上传进度" }))
    .not.toBeInTheDocument();
  await expect
    .element(page.getByRole("button", { name: "发送", exact: true }))
    .toBeEnabled();
  expect(error().element().textContent).toBe("");
});

test("inserts the path at the moved caret and leaves it after the insert", async () => {
  await open();
  const input = page.getByRole("textbox", { name: "消息" });
  await input.fill("hello world");
  const textarea = input.element() as HTMLTextAreaElement;
  textarea.setSelectionRange(6, 6);
  textarea.dispatchEvent(new Event("select", { bubbles: true }));

  choose([new File(["a"], "mid.txt", { type: "text/plain" })]);
  finish("/tmp/webmux-upload-1/mid.txt");

  const expected = "hello \n`/tmp/webmux-upload-1/mid.txt`\nworld";
  await expect.poll(() => draft().element().textContent).toBe(expected);
  expect(textarea.value).toBe(expected);
  expect(textarea.selectionStart).toBe(expected.indexOf("world"));
});

test("replaces the current selection with the uploaded path", async () => {
  await open();
  const input = page.getByRole("textbox", { name: "消息" });
  await input.fill("replace me");
  const textarea = input.element() as HTMLTextAreaElement;
  const length = textarea.value.length;
  textarea.setSelectionRange(0, length);
  textarea.dispatchEvent(new Event("select", { bubbles: true }));

  choose([new File(["a"], "pick.txt", { type: "text/plain" })]);
  finish("/tmp/webmux-upload-1/pick.txt");

  const expected = "`/tmp/webmux-upload-1/pick.txt`\n";
  await expect.poll(() => draft().element().textContent).toBe(expected);
  expect(textarea.selectionStart).toBe(expected.length);
});

test("multiple files upload in order and insert one path per line", async () => {
  await open();
  choose([
    new File(["a"], "a.txt", { type: "text/plain" }),
    new File(["b"], "b.txt", { type: "text/plain" }),
  ]);
  await expect
    .element(page.getByRole("status").filter({ hasText: "（1/2）" }))
    .toBeVisible();
  finish("/tmp/webmux-upload-1/a.txt");
  await expect
    .poll(() => draft().element().textContent)
    .toBe("`/tmp/webmux-upload-1/a.txt`\n");
  await expect
    .element(page.getByRole("status").filter({ hasText: "（2/2）" }))
    .toBeVisible();
  finish("/tmp/webmux-upload-1/b.txt");
  await expect
    .poll(() => draft().element().textContent)
    .toBe("`/tmp/webmux-upload-1/a.txt`\n`/tmp/webmux-upload-1/b.txt`\n");
});

test("dropping a file on the chat view uploads it after showing the hint", async () => {
  await open();
  const view = document.querySelector<HTMLElement>("[data-chat-view]")!;
  const transfer = new DataTransfer();
  transfer.items.add(new File(["png"], "drop.png", { type: "image/png" }));

  const enter = new DragEvent("dragenter", {
    bubbles: true,
    cancelable: true,
    dataTransfer: transfer,
  });
  view.dispatchEvent(enter);
  expect(enter.defaultPrevented).toBe(true);
  await expect
    .element(page.getByText("松开以上传文件到聊天", { exact: true }))
    .toBeVisible();

  view.dispatchEvent(
    new DragEvent("drop", {
      bubbles: true,
      cancelable: true,
      dataTransfer: transfer,
    }),
  );
  await expect
    .element(page.getByRole("status").filter({ hasText: "正在上传 drop.png" }))
    .toBeVisible();
  finish("/tmp/webmux-upload-1/drop.png");
  await expect
    .poll(() => draft().element().textContent)
    .toBe("`/tmp/webmux-upload-1/drop.png`\n");
  await expect
    .element(page.getByText("松开以上传文件到聊天", { exact: true }))
    .not.toBeInTheDocument();
});

test("pasting a file uploads it instead of inserting text", async () => {
  await open();
  const textarea = page.getByRole("textbox", { name: "消息" }).element();
  const transfer = new DataTransfer();
  transfer.items.add(new File(["png"], "paste.png", { type: "image/png" }));
  transfer.setData("text/plain", "DO_NOT_INSERT");
  const paste = new ClipboardEvent("paste", {
    clipboardData: transfer,
    bubbles: true,
    cancelable: true,
  });
  textarea.dispatchEvent(paste);
  expect(paste.defaultPrevented).toBe(true);
  finish("/tmp/webmux-upload-1/paste.png");
  await expect
    .poll(() => draft().element().textContent)
    .toBe("`/tmp/webmux-upload-1/paste.png`\n");
});

test("cancelling clears progress and leaves the draft untouched", async () => {
  await open();
  choose([new File(["a"], "a.txt", { type: "text/plain" })]);
  await expect
    .element(page.getByRole("status").filter({ hasText: "正在上传 a.txt" }))
    .toBeVisible();

  await page.getByRole("button", { name: "取消" }).click();
  await expect
    .element(page.getByRole("progressbar", { name: "文件上传进度" }))
    .not.toBeInTheDocument();
  expect(draft().element().textContent).toBe("");
  await expect.poll(() => error().element().textContent).toBe("上传已取消");
});
