import { computeDiff } from "./code-diff";

self.onmessage = (
  event: MessageEvent<{ snapshotId: string; old: string; new: string }>,
) => {
  const { snapshotId, old, new: next } = event.data;
  self.postMessage({ snapshotId, result: computeDiff(old, next) });
};
