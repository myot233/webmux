import { errorMessage } from "../error";
import { onMount } from "svelte";
import { SvelteMap } from "svelte/reactivity";
import { uploadFile } from "../api";
import { attachFileDrop } from "./file-drop";
import type { ChatAttachment } from "../schemas";
import type { Connection } from "../ws";
import { hasTouchInput } from "../touch-input";
import type {
  DraftOperation,
  InputDestination,
  TextInsertion,
} from "../terminal-draft.svelte";

// Connection state is only required when the operation still targets the PTY.
interface UploadTerminal {
  readonly exitCode: number | null;
  readonly connection: Connection;
  readonly restoring: boolean;
  focus(): void;
}

export interface UploadInput {
  begin(): DraftOperation;
  destination(operation: DraftOperation): InputDestination;
  insert(text: string, operation: DraftOperation): TextInsertion;
}

export interface UploadProgress {
  name: string;
  percent: number;
  index: number;
  total: number;
  waiting: boolean;
}

export interface ChatUploads {
  readonly progress: UploadProgress | null;
  /** Finished uploads waiting to be sent with the next message. */
  readonly attachments: readonly ChatAttachment[];
  /** Object or data URL for an uploaded image attachment, when available locally. */
  preview?(path: string): string | undefined;
  uploadFiles(files: File[]): Promise<void>;
  filePaste(event: ClipboardEvent): boolean;
  cancel(): void;
  remove(path: string): void;
  /** Hands the pending attachments to a send and clears them. */
  take(): ChatAttachment[];
  /** Puts a failed send's attachments back in front of newer ones. */
  restore(attachments: ChatAttachment[]): void;
}

/** Stores one file on the server and returns where it landed. */
export type UploadSend = (
  file: File,
  progress: (percent: number) => void,
  signal: AbortSignal,
) => Promise<{ path: string }>;

interface UploadRun {
  send: UploadSend;
  files: File[];
  signal: AbortSignal;
  /** Progress shows "等待" between upload and insertion while the PTY settles. */
  waiting: boolean;
  report(progress: UploadProgress): void;
  insert(path: string, file: File): Promise<void>;
}

// Both editors upload files the same way: one at a time, with progress and
// cancellation; only where the returned path goes differs.
async function runUploads(run: UploadRun) {
  for (const [index, file] of run.files.entries()) {
    const current: UploadProgress = {
      name: file.name,
      percent: 0,
      index: index + 1,
      total: run.files.length,
      waiting: false,
    };
    run.report(current);
    const { path } = await run.send(
      file,
      (percent) => run.report({ ...current, percent }),
      run.signal,
    );
    if (run.signal.aborted)
      throw new Error(`文件已上传至 ${path}，但路径未插入：操作已取消`);
    run.report({ ...current, percent: 100, waiting: run.waiting });
    await run.insert(path, file);
  }
}

// File paste is taken over from the editor; plain text keeps its native path.
function pasteFiles(
  event: ClipboardEvent,
  upload: (files: File[]) => Promise<void>,
) {
  const files = Array.from(event.clipboardData?.files ?? []);
  if (!files.length) return false;
  event.preventDefault();
  event.stopImmediatePropagation();
  void upload(files);
  return true;
}

// All upload sources share the same operation identity and quoted-path insertion.
export function useUpload(
  getHost: () => HTMLElement,
  id: string,
  terminal: UploadTerminal,
  setError: (message: string) => void,
  input: UploadInput,
) {
  let progress = $state<UploadProgress | null>(null);
  let dragging = $state(false);
  let foreground = $state(!document.hidden);
  let controller: AbortController | undefined;
  let waiter = $state<{
    resolve: () => void;
    reject: (error: Error) => void;
    operation: DraftOperation;
  } | null>(null);
  let mounted = false;

  $effect(() => {
    const current = waiter;
    if (!current) return;
    const target = input.destination(current.operation);
    if (
      target === "failed" ||
      terminal.exitCode !== null ||
      terminal.connection === "exited" ||
      terminal.connection === "missing"
    ) {
      waiter = null;
      current.reject(new Error("终端会话已退出或不存在"));
    } else if (
      target === "draft" ||
      (foreground && terminal.connection === "connected" && !terminal.restoring)
    ) {
      waiter = null;
      current.resolve();
    }
  });

  async function waitForDestination(
    signal: AbortSignal,
    operation: DraftOperation,
  ) {
    if (signal.aborted) throw new DOMException("操作已取消", "AbortError");
    const { promise, resolve, reject } = Promise.withResolvers<void>();
    const abort = () => reject(new DOMException("操作已取消", "AbortError"));
    const timeout = window.setTimeout(
      () => reject(new Error("等待终端恢复超时")),
      30_000,
    );
    signal.addEventListener("abort", abort, { once: true });
    waiter = { resolve, reject, operation };
    try {
      await promise;
    } finally {
      clearTimeout(timeout);
      signal.removeEventListener("abort", abort);
      waiter = null;
    }
  }

  async function uploadFiles(files: File[], operation = input.begin()) {
    if (!files.length) return;
    if (!mounted || input.destination(operation) === "failed") return;
    if (progress) {
      setError("正在上传，请等待当前上传完成。");
      return;
    }
    if (
      terminal.exitCode !== null ||
      terminal.connection === "exited" ||
      terminal.connection === "missing"
    ) {
      setError("终端会话已退出或不存在，无法上传文件。");
      return;
    }
    setError("");
    const current = new AbortController();
    controller = current;
    let pendingPath = "";
    const inserted = { terminal: false };
    try {
      await runUploads({
        send: (file, percent, signal) => uploadFile(id, file, percent, signal),
        files,
        signal: current.signal,
        waiting: true,
        report: (value) => (progress = value),
        insert: async (path) => {
          pendingPath = path;
          try {
            await waitForDestination(current.signal, operation);
            if (!mounted || current.signal.aborted)
              throw new DOMException("操作已取消", "AbortError");
            // Quote arbitrary filenames safely. Uploading never runs a command.
            const target: TextInsertion = input.insert(
              "'" + path.replaceAll("'", "'\\''") + "' ",
              operation,
            );
            if (target === "failed") throw new Error("输入目标未就绪");
            inserted.terminal = target === "terminal";
            pendingPath = "";
          } catch (err) {
            throw new Error(
              `文件已上传至 ${path}，但路径未插入：${errorMessage(err)}`,
            );
          }
        },
      });
      if (
        inserted.terminal &&
        input.destination(operation) === "terminal" &&
        !hasTouchInput()
      )
        terminal.focus();
    } catch (err) {
      if (mounted || pendingPath) setError(errorMessage(err));
    } finally {
      controller = undefined;
      if (mounted) progress = null;
    }
  }

  // Returns whether the paste carried files and was taken over as an upload.
  function filePaste(event: ClipboardEvent) {
    return pasteFiles(event, uploadFiles);
  }

  onMount(() => {
    mounted = true;
    // bind:this is cleared before onMount teardown; retain the listener owner.
    const host = getHost();
    let foregroundFrame = 0;
    const visibilityChanged = () => {
      cancelAnimationFrame(foregroundFrame);
      foreground = false;
      // Let foreground reconnect handlers invalidate the old socket first.
      if (!document.hidden)
        foregroundFrame = requestAnimationFrame(() => {
          foreground = !document.hidden;
        });
    };
    document.addEventListener("visibilitychange", visibilityChanged);
    // Capture before xterm consumes paste/drop events; text still uses xterm.
    const detachDrop = attachFileDrop(host, {
      exclusive: true,
      onEnter: () => {
        dragging = true;
      },
      onOver: (transfer) => {
        transfer.dropEffect = progress ? "none" : "copy";
      },
      onLeave: () => {
        dragging = false;
      },
      onDrop: (transfer) => {
        dragging = false;
        void uploadFiles(Array.from(transfer.files));
      },
    });
    host.addEventListener("paste", filePaste, true);
    return () => {
      document.removeEventListener("visibilitychange", visibilityChanged);
      cancelAnimationFrame(foregroundFrame);
      mounted = false;
      controller?.abort();
      host.removeEventListener("paste", filePaste, true);
      detachDrop();
    };
  });

  return {
    get progress() {
      return progress;
    },
    get dragging() {
      return dragging;
    },
    uploadFiles,
    filePaste,
    cancel() {
      controller?.abort();
    },
  };
}

// Mirrors the server's per-message bounds so an upload that could never be
// sent is refused before any bytes move; the server still enforces them.
const maxAttachments = 8;
const maxAttachmentBytes = 10 << 20;
const maxAttachmentTotalBytes = 20 << 20;
// The kinds the server sends as native image blocks; the server sniffs the
// stored bytes again and its answer is what the timeline shows.
const imageTypes: Record<string, true> = {
  "image/png": true,
  "image/jpeg": true,
  "image/gif": true,
  "image/webp": true,
};

export interface ChatAttachmentStore {
  get(): ChatAttachment[];
  set(next: ChatAttachment[]): void;
}

// Chat uploads become attachments of the next message rather than text in
// the draft: the server sends them as ACP content blocks, or as server paths
// when the agent cannot take them. send stores each file: in a session's
// uploads, or in the quick start staging folder before a session exists.
export function useChatUpload(
  send: UploadSend,
  setError: (message: string) => void,
  store?: ChatAttachmentStore,
): ChatUploads {
  let progress = $state<UploadProgress | null>(null);
  let localAttachments = $state<ChatAttachment[]>([]);
  const previews = new SvelteMap<string, string>();
  let controller: AbortController | undefined;
  let disposed = false;

  const currentAttachments = () => store?.get() ?? localAttachments;
  const writeAttachments = (next: ChatAttachment[]) => {
    if (store) store.set(next);
    else localAttachments = next;
  };

  function refusal(files: File[]) {
    const attachments = currentAttachments();
    if (attachments.length + files.length > maxAttachments)
      return `每条消息最多 ${maxAttachments} 个附件。`;
    const oversized = files.find((file) => file.size > maxAttachmentBytes);
    if (oversized)
      return `${oversized.name} 超过 ${maxAttachmentBytes >> 20} MiB。`;
    const total =
      attachments.reduce((sum, item) => sum + (item.size ?? 0), 0) +
      files.reduce((sum, file) => sum + file.size, 0);
    if (total > maxAttachmentTotalBytes)
      return `附件合计超过 ${maxAttachmentTotalBytes >> 20} MiB。`;
    return "";
  }

  async function uploadFiles(files: File[]) {
    if (!files.length) return;
    if (progress) {
      setError("正在上传，请等待当前上传完成。");
      return;
    }
    const refused = refusal(files);
    setError(refused);
    if (refused) return;
    const current = new AbortController();
    controller = current;
    try {
      await runUploads({
        send,
        files,
        signal: current.signal,
        waiting: false,
        report: (value) => (progress = value),
        insert: async (path, file) => {
          const isImage = Object.hasOwn(imageTypes, file.type);
          if (isImage) {
            const existing = previews.get(path);
            if (existing) URL.revokeObjectURL(existing);
            previews.set(path, URL.createObjectURL(file));
          }
          writeAttachments([
            ...currentAttachments(),
            {
              path,
              name: file.name,
              ...(file.type ? { mimeType: file.type } : {}),
              size: file.size,
              kind: isImage ? "image" : "file",
            },
          ]);
        },
      });
    } catch (err) {
      if (!disposed) setError(errorMessage(err));
    } finally {
      controller = undefined;
      progress = null;
    }
  }

  onMount(() => () => {
    disposed = true;
    controller?.abort();
    for (const url of previews.values()) URL.revokeObjectURL(url);
    previews.clear();
  });

  return {
    get progress() {
      return progress;
    },
    get attachments() {
      return currentAttachments();
    },
    preview(path) {
      return previews.get(path);
    },
    uploadFiles,
    filePaste(event) {
      return pasteFiles(event, uploadFiles);
    },
    cancel() {
      controller?.abort();
    },
    remove(path) {
      const url = previews.get(path);
      if (url) {
        URL.revokeObjectURL(url);
        previews.delete(path);
      }
      writeAttachments(
        currentAttachments().filter((item) => item.path !== path),
      );
    },
    take() {
      const taken = [...currentAttachments()];
      writeAttachments([]);
      return taken;
    },
    restore(sent) {
      writeAttachments([...sent, ...currentAttachments()]);
    },
  };
}
