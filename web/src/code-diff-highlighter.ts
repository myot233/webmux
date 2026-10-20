import { WorkerPoolManager } from "@pierre/diffs/worker";
import DiffHighlightWorker from "@pierre/diffs/worker/worker.js?worker";

let pool: WorkerPoolManager | undefined;
let readers = 0;

// Tabs share one bounded highlighting cache and one worker. Closing the last
// diff releases the worker as well as the cached source tokens.
export function acquireDiffHighlighter() {
  pool ??= new WorkerPoolManager(
    {
      // Bundle the library as the Worker entry itself. Its package marks a
      // side-effect-only wrapper import as tree-shakeable in production.
      workerFactory: () => new DiffHighlightWorker(),
      poolSize: 1,
      totalASTLRUCacheSize: 4,
    },
    {
      theme: { light: "pierre-light", dark: "pierre-dark" },
      lineDiffType: "word-alt",
      maxLineDiffLength: 4096,
      tokenizeMaxLineLength: 4096,
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
