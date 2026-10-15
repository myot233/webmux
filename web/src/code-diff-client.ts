import { DiffReplySchema, type DiffResult } from "./code-diff-model";

// One job per active document. Terminating the Worker cancels synchronous
// jsdiff work too, and prevents obsolete results from reaching the document.
export function calculateDiff(
  snapshotId: string,
  old: string,
  next: string,
  signal: AbortSignal,
): Promise<DiffResult> {
  return new Promise((resolve, reject) => {
    if (signal.aborted) {
      reject(new DOMException("Diff cancelled", "AbortError"));
      return;
    }
    let worker: Worker;
    try {
      worker = new Worker(new URL("./code-diff.worker.ts", import.meta.url), {
        type: "module",
      });
    } catch {
      reject(new Error("无法启动差异计算，请查看两侧完整文件"));
      return;
    }
    let settled = false;
    const finish = (result?: DiffResult, error?: unknown) => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      signal.removeEventListener("abort", abort);
      worker.onmessage = null;
      worker.onerror = null;
      worker.onmessageerror = null;
      worker.terminate();
      if (error) reject(error);
      else resolve(result!);
    };
    const abort = () =>
      finish(undefined, new DOMException("Diff cancelled", "AbortError"));
    const timer = setTimeout(
      () =>
        finish({ kind: "limited", reason: "差异计算超时，请查看两侧完整文件" }),
      10000,
    );
    signal.addEventListener("abort", abort, { once: true });
    worker.onmessage = (event: MessageEvent<unknown>) => {
      const reply = DiffReplySchema.safeParse(event.data);
      if (!reply.success || reply.data.snapshotId !== snapshotId) {
        finish(undefined, new Error("差异结果与当前快照不匹配，请重试"));
        return;
      }
      finish(reply.data.result);
    };
    worker.onerror = (event) => {
      event.preventDefault();
      finish(undefined, new Error("差异计算失败，请查看两侧完整文件"));
    };
    worker.onmessageerror = () =>
      finish(undefined, new Error("无法读取差异结果，请重试"));
    try {
      worker.postMessage({ snapshotId, old, new: next });
    } catch (error) {
      finish(undefined, error);
    }
  });
}
