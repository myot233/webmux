import type { FileDiffMetadata } from "@pierre/diffs";
import { api, APIError } from "../../platform/api";
import { DiffReplySchema, type DiffResult } from "./code-diff-model";
import { nodeId } from "./code-reference";
import {
  CodeChangesSchema,
  CodeDiffSchema,
  CodeFileSchema,
  type CodeDiff,
  type CodeFile,
} from "../../contracts/schemas";

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

function buildFileDiffMetadata(
  current: CodeDiff,
  fileDiff: FileDiffMetadata,
): FileDiffMetadata {
  return {
    ...fileDiff,
    name: current.new.path || current.old.path,
    prevName:
      current.old.path !== current.new.path ? current.old.path : undefined,
    type:
      current.old.source === "empty"
        ? "new"
        : current.new.source === "empty"
          ? "deleted"
          : current.old.path !== current.new.path
            ? fileDiff.hunks.length
              ? "rename-changed"
              : "rename-pure"
            : fileDiff.type,
    cacheKey: current.snapshotId,
  };
}

export async function fetchFileComparisonDiff(
  id: string,
  contextId: string,
  file: CodeFile,
  signal: AbortSignal,
  onState: (state: string) => void,
): Promise<CodeDiff> {
  const params = new URLSearchParams({
    contextId,
    nodeId: file.document.nodeId ?? nodeId(file.document.path),
  });
  const list = await api(
    `/sessions/${id}/code/changes?${params}`,
    CodeChangesSchema,
    "GET",
    undefined,
    signal,
  );
  const group = file.document.source !== "worktree" ? "staged" : "unstaged";
  const change = list.entries.find(
    (c) => c.group === group || c.group === "untracked",
  );
  if (!change) throw new Error("此文件没有可用的未提交差异，保留完整阅读");
  const value = await api(
    `/sessions/${id}/code/diff?${new URLSearchParams({ contextId, changeId: change.changeId })}`,
    CodeDiffSchema,
    "GET",
    undefined,
    signal,
  );
  if (signal.aborted) return value;
  const candidate = file.document.source === "head" ? value.old : value.new;
  if (candidate.contentVersion !== file.document.contentVersion) {
    onState("updated");
    throw new Error("比较来源已更新，请刷新完整文件后再切换 Diff");
  }
  return value;
}

export function sourceLabel(source: string) {
  return source === "worktree"
    ? "工作区"
    : source === "index"
      ? "索引版本"
      : source === "head"
        ? "HEAD 版本"
        : source === "empty"
          ? "不存在"
          : "";
}

export async function fetchSnapshotSideFile(
  id: string,
  snapshotId: string,
  which: "old" | "new",
  signal: AbortSignal,
  onState: (state: string) => void,
): Promise<CodeFile> {
  try {
    return await api(
      `/sessions/${id}/code/snapshots/${snapshotId}/file?side=${which}`,
      CodeFileSchema,
      "GET",
      undefined,
      signal,
    );
  } catch (e) {
    if (!signal.aborted && e instanceof APIError && e.status === 410)
      onState("expired");
    throw e;
  }
}

export type CalculatedDiffState = {
  modelSnapshot: string;
  calculation: "ready" | "limited" | "error";
  calculationReason: string;
  fileDiff?: FileDiffMetadata;
};

export async function resolveCalculatedDiff(
  current: CodeDiff,
  signal: AbortSignal,
): Promise<CalculatedDiffState> {
  try {
    const result = await calculateDiff(
      current.snapshotId,
      current.old.text,
      current.new.text,
      signal,
    );
    if (result.kind === "ready") {
      return {
        modelSnapshot: current.snapshotId,
        calculation: "ready",
        calculationReason: "",
        fileDiff: buildFileDiffMetadata(current, result.fileDiff),
      };
    }
    return {
      modelSnapshot: current.snapshotId,
      calculation: result.kind,
      calculationReason: result.reason,
    };
  } catch (error: unknown) {
    return {
      modelSnapshot: current.snapshotId,
      calculation: "error",
      calculationReason:
        error instanceof Error
          ? error.message
          : "差异计算失败，请查看两侧完整文件",
    };
  }
}
