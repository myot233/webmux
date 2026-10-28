import "../../src/styles.css";
import { beforeEach, expect, test, vi } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte/pure";
import Harness from "./ChatViewHarness.svelte";
import {
  chatState,
  commands,
  installChatFixture,
  respond,
  snapshot,
  socketAt,
} from "./chat-fixture";
import type { ChatState } from "../../src/schemas";

installChatFixture();

const id = "hybrid22";
const prompts = `/api/sessions/${id}/chat/prompts`;

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

async function open(patch: Partial<ChatState> = {}) {
  render(Harness, { id });
  const socket = await socketAt(`/ws/s/${id}/chat`);
  socket.frame(snapshot(chatState(patch)));
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
const chips = () => page.getByRole("list", { name: "待发送附件" });
const chipNames = () =>
  Array.from(
    document.querySelectorAll('[aria-label="待发送附件"] li'),
    (item) => item.textContent?.trim(),
  );
const sendButton = () =>
  page.getByRole("button", { name: "发送", exact: true });
const sentPrompts = () =>
  commands().filter((request) => request.path === prompts);

test("a chosen image uploads with progress, then waits as a chip and is sent with the message", async () => {
  respond(/^POST .*\/chat\/prompts$/, () => Response.json({ queued: false }));
  await open();
  const input = page.getByRole("textbox", { name: "消息" });
  await input.fill("看看这张图");
  choose([new File(["png"], "shot.png", { type: "image/png" })]);

  await expect
    .element(page.getByRole("status").filter({ hasText: "正在上传 shot.png" }))
    .toBeVisible();
  report(50, 100);
  await expect.element(page.getByText("50%")).toBeVisible();
  await expect.element(sendButton()).toBeDisabled();
  await expect
    .element(page.getByRole("button", { name: "上传文件" }))
    .toBeDisabled();

  finish("/tmp/webmux-upload-1/shot.png");
  await expect.element(chips()).toBeVisible();
  expect(chipNames()).toEqual(["shot.png"]);
  // The draft text is not touched: the file travels as an attachment.
  expect(draft().element().textContent).toBe("看看这张图");
  await expect
    .element(page.getByRole("progressbar", { name: "文件上传进度" }))
    .not.toBeInTheDocument();
  expect(error().element().textContent).toBe("");

  await sendButton().click();
  await expect.poll(() => sentPrompts().length).toBe(1);
  expect(sentPrompts()[0]?.body).toMatchObject({
    text: "看看这张图",
    mode: "auto",
    attachments: [{ path: "/tmp/webmux-upload-1/shot.png" }],
  });
  await expect.element(chips()).not.toBeInTheDocument();
});

test("a message of attachments alone can be sent", async () => {
  respond(/^POST .*\/chat\/prompts$/, () => Response.json({ queued: false }));
  await open();
  await expect.element(sendButton()).toBeDisabled();
  choose([new File(["a"], "notes.txt", { type: "text/plain" })]);
  finish("/tmp/webmux-upload-1/notes.txt");
  await expect.element(sendButton()).toBeEnabled();

  await sendButton().click();
  await expect.poll(() => sentPrompts().length).toBe(1);
  expect(sentPrompts()[0]?.body).toMatchObject({
    text: "",
    attachments: [{ path: "/tmp/webmux-upload-1/notes.txt" }],
  });
});

test("a failed send gives the text and attachments back", async () => {
  respond(/^POST .*\/chat\/prompts$/, () =>
    Response.json({ error: "附件路径无效" }, { status: 400 }),
  );
  await open();
  await page.getByRole("textbox", { name: "消息" }).fill("再试一次");
  choose([new File(["a"], "a.txt", { type: "text/plain" })]);
  finish("/tmp/webmux-upload-1/a.txt");
  await expect.element(chips()).toBeVisible();

  await sendButton().click();
  await expect.poll(() => sentPrompts().length).toBe(1);
  await expect.poll(() => draft().element().textContent).toBe("再试一次");
  await expect.element(chips()).toBeVisible();
  expect(chipNames()).toEqual(["a.txt"]);
});

test("a chip can be removed before sending", async () => {
  await open();
  choose([
    new File(["a"], "a.txt", { type: "text/plain" }),
    new File(["b"], "b.txt", { type: "text/plain" }),
  ]);
  finish("/tmp/webmux-upload-1/a.txt");
  await expect
    .element(page.getByRole("status").filter({ hasText: "（2/2）" }))
    .toBeVisible();
  finish("/tmp/webmux-upload-1/b.txt");
  await expect.poll(chipNames).toEqual(["a.txt", "b.txt"]);

  await page.getByRole("button", { name: "移除附件 a.txt" }).click();
  await expect.poll(chipNames).toEqual(["b.txt"]);
});

test("an image chip warns when the agent cannot take images", async () => {
  await open();
  choose([new File(["png"], "shot.png", { type: "image/png" })]);
  finish("/tmp/webmux-upload-1/shot.png");
  await expect
    .element(page.getByText("该 agent 不支持图片，图片将按服务器路径发送。"))
    .toBeVisible();
});

test("an image chip does not warn when the agent takes images", async () => {
  await open({ promptCapabilities: { image: true, embeddedContext: false } });
  choose([new File(["png"], "shot.png", { type: "image/png" })]);
  finish("/tmp/webmux-upload-1/shot.png");
  await expect.element(chips()).toBeVisible();
  await expect
    .element(page.getByText("该 agent 不支持图片，图片将按服务器路径发送。"))
    .not.toBeInTheDocument();
});

test("uploads beyond the per-message limits are refused before any bytes move", async () => {
  await open();
  choose(
    Array.from(
      { length: 9 },
      (_, i) => new File(["a"], `f${i}.txt`, { type: "text/plain" }),
    ),
  );
  await expect
    .poll(() => error().element().textContent)
    .toBe("每条消息最多 8 个附件。");
  choose([
    new File([new Uint8Array((10 << 20) + 1)], "big.bin", {
      type: "application/octet-stream",
    }),
  ]);
  await expect
    .poll(() => error().element().textContent)
    .toBe("big.bin 超过 10 MiB。");
  expect(FakeXHR.pending).toHaveLength(0);
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
  await expect.poll(chipNames).toEqual(["drop.png"]);
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
  await expect.poll(chipNames).toEqual(["paste.png"]);
  expect(draft().element().textContent).toBe("");
});

test("cancelling clears progress and adds no attachment", async () => {
  await open();
  choose([new File(["a"], "a.txt", { type: "text/plain" })]);
  await expect
    .element(page.getByRole("status").filter({ hasText: "正在上传 a.txt" }))
    .toBeVisible();

  await page.getByRole("button", { name: "取消" }).click();
  await expect
    .element(page.getByRole("progressbar", { name: "文件上传进度" }))
    .not.toBeInTheDocument();
  await expect.element(chips()).not.toBeInTheDocument();
  expect(draft().element().textContent).toBe("");
  await expect.poll(() => error().element().textContent).toBe("上传已取消");
});
