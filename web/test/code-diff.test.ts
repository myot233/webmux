import { describe, it, expect } from "vitest";
import { computeDiff } from "../src/code-diff";
import {
  collapseDiff,
  expandDiff,
  type CodeLine,
  DiffReplySchema,
} from "../src/code-diff-model";

function ready(old: string, next: string): CodeLine[] {
  const result = computeDiff(old, next);
  expect(result.kind).toBe("ready");
  if (result.kind !== "ready") throw new Error(result.reason);
  return result.lines;
}

describe("browser text diff", () => {
  it("maps additions, deletions and ordinary lines to both original sources", () => {
    expect(ready("same\nold\nend\n", "same\nnew\nextra\nend\n")).toEqual([
      { kind: "context", oldLine: 1, newLine: 1, text: "same" },
      { kind: "delete", oldLine: 2, newLine: 0, text: "old" },
      { kind: "add", oldLine: 0, newLine: 2, text: "new" },
      { kind: "add", oldLine: 0, newLine: 3, text: "extra" },
      { kind: "context", oldLine: 3, newLine: 4, text: "end" },
    ]);
    expect(ready("", "new\n")).toEqual([
      { kind: "add", oldLine: 0, newLine: 1, text: "new" },
    ]);
    expect(ready("old\n", "")).toEqual([
      { kind: "delete", oldLine: 1, newLine: 0, text: "old" },
    ]);
    expect(ready("", "")).toEqual([]);
  });
  it("handles CRLF, blank lines and final newline changes without inventing source lines", () => {
    expect(
      ready("a\r\n\r\nb\r\n", "a\n\nb\n").map((l) => [
        l.kind,
        l.oldLine,
        l.newLine,
        l.text,
      ]),
    ).toEqual([
      ["context", 1, 1, "a"],
      ["context", 2, 2, ""],
      ["context", 3, 3, "b"],
    ]);
    const rows = ready("a\nlast", "a\nlast\n");
    expect(rows.filter((l) => l.kind === "marker")).toEqual([
      {
        kind: "marker",
        oldLine: 0,
        newLine: 0,
        text: "\\ No newline at end of file",
      },
    ]);
    expect(
      rows
        .filter((l) => l.kind !== "marker")
        .map((l) => [l.kind, l.oldLine, l.newLine]),
    ).toEqual([
      ["context", 1, 1],
      ["delete", 2, 0],
      ["add", 0, 2],
    ]);
    expect(
      ready("last", "next").filter((l) => l.kind === "marker"),
    ).toHaveLength(2);
  });
  it("expands head, middle and tail contexts locally without duplicates or changed line numbers", () => {
    const old =
      Array.from({ length: 200 }, (_, i) => `line ${i + 1}`).join("\n") + "\n";
    const rows = ready(
      old,
      old
        .replace("line 100\n", "changed 100\n")
        .replace("line 150\n", "changed 150\n"),
    );
    let blocks = collapseDiff(rows);
    expect(blocks.filter((b) => b.kind === "gap").map((b) => b.count)).toEqual([
      96, 43, 47,
    ]);
    const head = blocks[0];
    blocks = expandDiff(rows, blocks, head.gapId, "up");
    expect(blocks[0].count).toBe(66);
    expect(blocks[1].lines[0].oldLine).toBe(67);
    blocks = expandDiff(rows, blocks, blocks[0].gapId, "down");
    expect(blocks[0].lines[0].oldLine).toBe(1);
    const middle = blocks.find((b) => b.kind === "gap" && b.count === 43)!;
    blocks = expandDiff(rows, blocks, middle.gapId, "all");
    expect(
      blocks.find((b) => b.start === middle.start)?.lines.map((l) => l.oldLine),
    ).toEqual(Array.from({ length: 43 }, (_, i) => 104 + i));
    blocks = expandDiff(rows, blocks, "all", "all");
    expect(blocks).toHaveLength(1);
    expect(blocks[0].lines).toEqual(rows);
    expect(rows.filter((l) => l.newLine).map((l) => l.newLine)).toEqual(
      Array.from({ length: 200 }, (_, i) => i + 1),
    );
    expect(rows.filter((l) => l.oldLine).map((l) => l.oldLine)).toEqual(
      Array.from({ length: 200 }, (_, i) => i + 1),
    );
    expect(expandDiff(rows, blocks, head.gapId, "all")).toEqual(blocks);
  });
  it("supports the full 20000-line bound and reports limits instead of a truncated diff", () => {
    const old =
      Array.from({ length: 20000 }, (_, i) => `line ${i}`).join("\n") + "\n";
    expect(ready(old, "").filter((l) => l.oldLine)).toHaveLength(20000);
    expect(computeDiff(old + "extra\n", "").kind).toBe("limited");
    expect(computeDiff("界".repeat(750000), "").kind).toBe("limited");
    // A negative budget expires immediately at the algorithm's next deadline
    // check, independent of the test machine's speed.
    const result = computeDiff(old, old.replaceAll("line", "different"), -1);
    expect(result.kind).toBe("limited");
  });
  it("validates Worker results before they reach the reader", () => {
    expect(
      DiffReplySchema.safeParse({
        snapshotId: "s",
        result: {
          kind: "ready",
          lines: [{ kind: "add", oldLine: 0, newLine: -1, text: "x" }],
        },
      }).success,
    ).toBe(false);
  });
  it("keeps both source projections intact for empty, repeated, Unicode and control-looking lines", () => {
    const samples = [
      "",
      "\n",
      "\n\n",
      "a",
      "a\n",
      "a\r\n",
      "a\r",
      "a\rb\n",
      "same\nsame\nlast",
      "界\n🙂\n",
      "<script>x</script>\n\x1b[31m",
    ];
    const split = (text: string) =>
      text
        ? text
            .replace(/\n$/, "")
            .split("\n")
            .map((l) => l.replace(/\r$/, ""))
        : [];
    for (const old of samples)
      for (const next of samples) {
        const rows = ready(old, next);
        const a = rows.filter((l) => l.oldLine),
          b = rows.filter((l) => l.newLine);
        expect(a.map((l) => l.text)).toEqual(split(old));
        expect(b.map((l) => l.text)).toEqual(split(next));
        expect(a.map((l) => l.oldLine)).toEqual(a.map((_, i) => i + 1));
        expect(b.map((l) => l.newLine)).toEqual(b.map((_, i) => i + 1));
      }
  });
});
