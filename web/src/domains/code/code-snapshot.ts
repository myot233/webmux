// Document bodies dominate a code workspace snapshot and rarely change while
// a pop-out window is open. The window sends a body once; later snapshots
// name it by snapshot id and the opener fills it back in.

import type {
  CodeDiff,
  CodeFile,
  CodeWorkspaceSnapshot,
} from "../../contracts/schemas";

type SnapshotTab = CodeWorkspaceSnapshot["tabs"][number];

/** Bodies the window already sent, by snapshot id. */
export class SentBodies {
  #ids = new Set<string>();

  /** Replaces bodies sent before by references and records new ones. */
  reference(snapshot: CodeWorkspaceSnapshot): CodeWorkspaceSnapshot {
    const reference = <T extends CodeFile | CodeDiff>(body: T | undefined) => {
      if (!body) return { body: undefined, ref: undefined };
      if (this.#ids.has(body.snapshotId))
        return { body: undefined, ref: body.snapshotId };
      this.#ids.add(body.snapshotId);
      return { body, ref: undefined };
    };
    return {
      ...snapshot,
      tabs: snapshot.tabs.map((tab): SnapshotTab => {
        const file = reference(tab.file);
        const diff = reference(tab.diff);
        const comparison = reference(tab.comparison);
        return {
          ...tab,
          file: file.body,
          fileRef: file.ref,
          diff: diff.body,
          diffRef: diff.ref,
          comparison: comparison.body,
          comparisonRef: comparison.ref,
        };
      }),
    };
  }
}

/** Bodies the opener received, by snapshot id. */
export class SnapshotBodies {
  #files = new Map<string, CodeFile>();
  #diffs = new Map<string, CodeDiff>();

  /**
   * Fills referenced bodies back in, remembering every body the snapshot
   * carries; a tab whose body is unknown is dropped rather than shown empty.
   */
  resolve(snapshot: CodeWorkspaceSnapshot): CodeWorkspaceSnapshot {
    const files = new Map<string, CodeFile>();
    const diffs = new Map<string, CodeDiff>();
    const file = (body: CodeFile | undefined, ref: string | undefined) => {
      const value = body ?? (ref ? this.#files.get(ref) : undefined);
      if (value) files.set(value.snapshotId, value);
      return value;
    };
    const diff = (body: CodeDiff | undefined, ref: string | undefined) => {
      const value = body ?? (ref ? this.#diffs.get(ref) : undefined);
      if (value) diffs.set(value.snapshotId, value);
      return value;
    };
    const tabs = snapshot.tabs.flatMap((tab): SnapshotTab[] => {
      const next: SnapshotTab = {
        key: tab.key,
        contextId: tab.contextId,
        rootKind: tab.rootKind,
        status: tab.status,
        view: tab.view,
        file: file(tab.file, tab.fileRef),
        diff: diff(tab.diff, tab.diffRef),
        comparison: diff(tab.comparison, tab.comparisonRef),
      };
      const missing =
        (tab.fileRef && !next.file) ||
        (tab.diffRef && !next.diff) ||
        (tab.comparisonRef && !next.comparison);
      return missing || (!next.file && !next.diff) ? [] : [next];
    });
    // Only bodies of open tabs can be referenced again.
    this.#files = files;
    this.#diffs = diffs;
    const active = tabs.some((tab) => tab.key === snapshot.active)
      ? snapshot.active
      : (tabs.at(-1)?.key ?? "");
    return { ...snapshot, tabs, active };
  }
}
