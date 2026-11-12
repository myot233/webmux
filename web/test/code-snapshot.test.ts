import { describe, expect, test } from "vitest";
import { SentBodies, SnapshotBodies } from "../src/domains/code/code-snapshot";
import type { CodeFile, CodeWorkspaceSnapshot } from "../src/contracts/schemas";

function file(id: string): CodeFile {
  return {
    snapshotId: id,
    checkedAt: "",
    document: {
      nodeId: id,
      path: `${id}.txt`,
      absolutePath: `/root/${id}.txt`,
      source: "worktree",
      oid: "",
      contentVersion: id,
      kind: "text",
      text: `${id}\n`,
      lineCount: 1,
      referenceable: true,
      reason: "",
      mediaType: "",
      size: 3,
      downloadable: true,
    },
  };
}
function snapshot(...ids: string[]): CodeWorkspaceSnapshot {
  return {
    version: 1,
    root: "workspace",
    rootIncludeIgnored: false,
    directoryVersions: {},
    navIdentity: "",
    navStatus: "",
    changeQuery: "",
    changeSearch: "",
    changeGroup: "",
    includeIgnored: false,
    directory: "",
    query: "",
    nav: "files",
    navOpen: true,
    navWidth: 260,
    active: ids.at(-1) ?? "",
    tabs: ids.map((id) => ({
      key: id,
      contextId: "root",
      rootKind: "directory",
      file: file(id),
      status: "unchanged",
    })),
  };
}
const byReference = (value: CodeWorkspaceSnapshot, key: string) => ({
  ...value,
  tabs: value.tabs.map((tab) =>
    tab.key === key ? { ...tab, file: undefined, fileRef: key } : tab,
  ),
});

describe("snapshot bodies between windows", () => {
  test("a body is sent once and resolved from what the opener holds", () => {
    const sent = new SentBodies();
    const opener = new SnapshotBodies();
    opener.resolve(sent.reference(snapshot("a")));
    const second = sent.reference(snapshot("a", "b"));
    expect(second.tabs[0]).toMatchObject({ file: undefined, fileRef: "a" });
    expect(second.tabs[1]!.file?.snapshotId).toBe("b");
    const resolved = opener.resolve(second);
    expect(resolved.tabs.map((tab) => tab.file?.document.text)).toEqual([
      "a\n",
      "b\n",
    ]);
  });

  test("a tab whose body the opener never got is dropped, not shown empty", () => {
    const resolved = new SnapshotBodies().resolve(
      byReference(snapshot("a", "b"), "b"),
    );
    expect(resolved.tabs.map((tab) => tab.key)).toEqual(["a"]);
    expect(resolved.active).toBe("a");
  });

  test("bodies of closed tabs are forgotten", () => {
    const opener = new SnapshotBodies();
    opener.resolve(snapshot("a", "b"));
    opener.resolve(snapshot("b"));
    expect(opener.resolve(byReference(snapshot("a"), "a")).tabs).toHaveLength(
      0,
    );
  });
});
