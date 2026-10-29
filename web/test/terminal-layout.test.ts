import { describe, expect, it } from "vitest";
import {
  MAX_PANES,
  layoutGeometry,
  paneIds,
  removePane,
  resizeSplit,
  restoreLayout,
  splitPane,
  type PaneLayout,
} from "../src/terminal-layout";

const first: PaneLayout = { kind: "pane", sessionId: "aaaaaaaa" };
const nested = () =>
  splitPane(
    splitPane(first, "aaaaaaaa", "bbbbbbbb", "horizontal"),
    "bbbbbbbb",
    "cccccccc",
    "vertical",
  );

describe("terminal pane layout", () => {
  it("tiles nested panes without overlap and preserves total dimensions", () => {
    const geometry = layoutGeometry(nested(), 1006, 806);
    expect(geometry.panes).toEqual([
      { sessionId: "aaaaaaaa", x: 0, y: 0, width: 500, height: 806 },
      { sessionId: "bbbbbbbb", x: 506, y: 0, width: 500, height: 400 },
      { sessionId: "cccccccc", x: 506, y: 406, width: 500, height: 400 },
    ]);
    expect(
      layoutGeometry(nested(), 0, 0).panes.every(
        (pane) => pane.width >= 0 && pane.height >= 0,
      ),
    ).toBe(true);
  });

  it("collapses only the removed branch and clamps resize ratios", () => {
    const layout = nested();
    expect(paneIds(removePane(layout, "bbbbbbbb")!)).toEqual([
      "aaaaaaaa",
      "cccccccc",
    ]);
    expect(
      layoutGeometry(removePane(layout, "aaaaaaaa")!, 1006, 806).panes[0]!
        .width,
    ).toBe(1006);
    expect(removePane(first, "aaaaaaaa")).toBeUndefined();
    if (layout.kind !== "split") throw new Error("Expected split");
    const resized = resizeSplit(layout, layout.id, 10);
    expect(resized.kind === "split" && resized.ratio).toBe(0.9);
    expect(resizeSplit(layout, layout.id, -1)).toMatchObject({ ratio: 0.1 });
  });

  it("bounds pane creation and never attaches the same PTY twice", () => {
    let layout: PaneLayout = first;
    for (const letter of "bcdefghi")
      layout = splitPane(layout, "aaaaaaaa", letter.repeat(8), "horizontal");
    expect(paneIds(layout)).toHaveLength(MAX_PANES);
    expect(splitPane(layout, "aaaaaaaa", "zzzzzzzz", "vertical")).toBe(layout);
    expect(splitPane(first, "aaaaaaaa", "aaaaaaaa", "vertical")).toBe(first);
  });

  it("restores valid layouts and rejects corrupt, duplicate and excessive saved trees", () => {
    const layout = nested();
    expect(restoreLayout(JSON.stringify(layout), "dddddddd")).toEqual(layout);
    const invalid = [
      "null",
      "{}",
      "broken",
      JSON.stringify({ ...layout, ratio: 100 }),
      JSON.stringify({
        kind: "split",
        id: "duplicate",
        direction: "horizontal",
        ratio: 0.5,
        first,
        second: first,
      }),
      JSON.stringify({ kind: "pane", sessionId: "invalid" }),
      " ".repeat(4097),
    ];
    let deep: PaneLayout = first;
    for (let i = 0; i < 20; i++)
      deep = {
        kind: "split",
        id: String(i),
        direction: "vertical",
        ratio: 0.5,
        first,
        second: deep,
      };
    invalid.push(JSON.stringify(deep));
    for (const value of invalid)
      expect(restoreLayout(value, "aaaaaaaa")).toEqual(first);
  });
});
