import { describe, it, expect } from "vitest";
import { codeReference, nodeId } from "../src/code-reference";
import { CodeDocumentSchema, CodeFileSchema } from "../src/schemas";
const doc = CodeDocumentSchema.parse({
  path: "src/main.go",
  absolutePath: "/repo/src/main.go",
  source: "worktree",
  oid: "",
  contentVersion: "version",
  kind: "text",
  text: "line\n",
  lineCount: 20,
  referenceable: true,
  reason: "",
});
describe("code references", () => {
  it("formats ordinary lines and source versions without submitting input", () => {
    expect(codeReference(doc, 12, 18)).toBe("/repo/src/main.go:12-18 ");
    expect(codeReference({ ...doc, source: "index" }, 2)).toBe(
      "/repo/src/main.go:2（索引版本） ",
    );
    expect(codeReference({ ...doc, source: "head" })).toBe(
      "/repo/src/main.go（HEAD 旧版本） ",
    );
  });
  it("escapes special paths with no raw terminal controls", () => {
    const result = codeReference(
      { ...doc, absolutePath: '/repo/a "\n\r\x1b.go' },
      1,
    );
    expect(result).toBe(
      '"/repo/a \\"\\n\\r\\u001b.go":1 '.replace('\\\\"', '\\"'),
    );
    expect(result).not.toMatch(/[\n\r\x1b]/);
  });
  it("rejects invalid ranges and unrepresentable paths", () => {
    for (const n of [-1, 0, 21, 1.5])
      expect(() => codeReference(doc, n)).toThrow();
    expect(() => codeReference(doc, 5, 2)).toThrow();
    expect(() => codeReference({ ...doc, referenceable: false })).toThrow();
    expect(() => codeReference({ ...doc, kind: "binary" }, 1)).toThrow();
    expect(() => codeReference({ ...doc, source: "empty" })).toThrow();
  });
  it("uses UTF-8 node identifiers and validates server contracts", () => {
    expect(nodeId("你好.go")).toBe("5L2g5aW9Lmdv");
    expect(
      CodeFileSchema.safeParse({
        snapshotId: "x",
        document: { ...doc, lineCount: 20001 },
        checkedAt: "now",
      }).success,
    ).toBe(false);
  });
});
