import { errorMessage } from "../error";
import { onMount } from "svelte";
import { uploadFile } from "../api";
import type { ChatAttachment } from "../schemas";
import type { Connection } from "../ws";
import { hasTouchInput } from "../touchInput";
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
  uploadFiles(files: File[]): Promise<void>;
  filePaste(event: ClipboardEvent): boolean;
  cancel(): void;
  remove(path: string): void;
  /** Hands the pending attachments to a send and clears them. */
  take(): ChatAttachment[];
  /** Puts a failed send's attachments back in front of newer ones. */
  restore(attachments: ChatAttachment[]): void;
}

interface UploadRun {
  id: string;
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
    const { path } = await uploadFile(
      run.id,
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
        id,
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
    let dragDepth = 0;
    const dragEnter = (event: DragEvent) => {
      if (!event.dataTransfer?.types.includes("Files")) return;
      event.preventDefault();
      dragging = true;
      dragDepth++;
    };
    const dragOver = (event: DragEvent) => {
      if (!event.dataTransfer?.types.includes("Files")) return;
      event.preventDefault();
      event.dataTransfer.dropEffect = progress ? "none" : "copy";
    };
    const dragLeave = () => {
      if (--dragDepth <= 0) {
        dragDepth = 0;
        dragging = false;
      }
    };
    const drop = (event: DragEvent) => {
      if (!event.dataTransfer?.types.includes("Files")) return;
      event.preventDefault();
      event.stopImmediatePropagation();
      dragDepth = 0;
      dragging = false;
      void uploadFiles(Array.from(event.dataTransfer.files));
    };
    host.addEventListener("paste", filePaste, true);
    host.addEventListener("dragenter", dragEnter, true);
    host.addEventListener("dragover", dragOver, true);
    host.addEventListener("dragleave", dragLeave, true);
    host.addEventListener("drop", drop, true);
    return () => {
      document.removeEventListener("visibilitychange", visibilityChanged);
      cancelAnimationFrame(foregroundFrame);
      mounted = false;
      controller?.abort();
      host.removeEventListener("paste", filePaste, true);
      host.removeEventListener("dragenter", dragEnter, true);
      host.removeEventListener("dragover", dragOver, true);
      host.removeEventListener("dragleave", dragLeave, true);
      host.removeEventListener("drop", drop, true);
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

// Chat uploads become attachments of the next message rather than text in
// the draft: the server sends them as ACP content blocks, or as server paths
// when the agent cannot take them.
export function useChatUpload(
  id: string,
  setError: (message: string) => void,
): ChatUploads {
  let progress = $state<UploadProgress | null>(null);
  let attachments = $state<ChatAttachment[]>([]);
  let controller: AbortController | undefined;
  let disposed = false;

  function refusal(files: File[]) {
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
        id,
        files,
        signal: current.signal,
        waiting: false,
        report: (value) => (progress = value),
        insert: async (path, file) => {
          attachments = [
            ...attachments,
            {
              path,
              name: file.name,
              ...(file.type ? { mimeType: file.type } : {}),
              size: file.size,
              kind: Object.hasOwn(imageTypes, file.type) ? "image" : "file",
            },
          ];
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
  });

  return {
    get progress() {
      return progress;
    },
    get attachments() {
      return attachments;
    },
    uploadFiles,
    filePaste(event) {
      return pasteFiles(event, uploadFiles);
    },
    cancel() {
      controller?.abort();
    },
    remove(path) {
      attachments = attachments.filter((item) => item.path !== path);
    },
    take() {
      const taken = attachments;
      attachments = [];
      return taken;
    },
    restore(sent) {
      attachments = [...sent, ...attachments];
    },
  };
}
