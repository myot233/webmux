import { diffLines } from "diff";
import type { CodeLine, DiffResult } from "./code-diff-model";

export const DIFF_BUDGET_MS = 250;

function lines(text: string): string[] {
  if (!text) return [];
  const result = text.split("\n");
  if (text.endsWith("\n")) result.pop();
  return result.map((line) => (line.endsWith("\r") ? line.slice(0, -1) : line));
}

// Only the Worker imports this module in the app. The texts remain untouched;
// jsdiff's counts map displayed rows back to each captured source document.
export function computeDiff(
  oldText: string,
  newText: string,
  budget = DIFF_BUDGET_MS,
): DiffResult {
  const old = lines(oldText),
    next = lines(newText);
  if (
    old.length > 20000 ||
    next.length > 20000 ||
    new TextEncoder().encode(oldText).length > 2 * 1024 * 1024 ||
    new TextEncoder().encode(newText).length > 2 * 1024 * 1024
  ) {
    return {
      kind: "limited",
      reason: "内容超过 2 MiB 或 20000 行，请查看两侧完整文件",
    };
  }
  const changes = diffLines(oldText, newText, {
    stripTrailingCr: true,
    timeout: budget,
  });
  if (!changes)
    return {
      kind: "limited",
      reason: "差异计算超过时间预算，请查看两侧完整文件",
    };
  const result: CodeLine[] = [];
  let a = 0,
    b = 0;
  for (const change of changes) {
    for (let i = 0; i < change.count; i++) {
      const kind = change.added ? "add" : change.removed ? "delete" : "context";
      const text = kind === "add" ? next[b] : old[a];
      const oldLine = kind === "add" ? 0 : ++a;
      const newLine = kind === "delete" ? 0 : ++b;
      result.push({ kind, oldLine, newLine, text });
      if (
        (oldLine === old.length && oldText && !oldText.endsWith("\n")) ||
        (newLine === next.length && newText && !newText.endsWith("\n"))
      ) {
        result.push({
          kind: "marker",
          oldLine: 0,
          newLine: 0,
          text: "\\ No newline at end of file",
        });
      }
    }
  }
  return { kind: "ready", lines: result };
}
