import { WorkerPoolManager } from "@pierre/diffs/worker";
import DiffHighlightWorker from "@pierre/diffs/worker/worker.js?worker";
import { codeThemes, codeTokenizeLimit } from "./code-pierre";

let pool: WorkerPoolManager | undefined;
let readers = 0;

// Tabs share one bounded highlighting cache and one worker. Closing the last
// reader releases the worker as well as the cached source tokens.
export function acquireCodeHighlighter() {
  pool ??= new WorkerPoolManager(
    {
      // Bundle the library as the Worker entry itself. Its package marks a
      // side-effect-only wrapper import as tree-shakeable in production.
      workerFactory: () => new DiffHighlightWorker(),
      poolSize: 1,
      totalASTLRUCacheSize: 4,
    },
    {
      theme: codeThemes,
      lineDiffType: "word-alt",
      maxLineDiffLength: codeTokenizeLimit,
      tokenizeMaxLineLength: codeTokenizeLimit,
    },
  );
  readers++;
  return {
    pool,
    release() {
      if (--readers === 0) {
        pool?.terminate();
        pool = undefined;
      }
    },
  };
}
