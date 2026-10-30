import { expect, it } from "vitest";
import { previewFormat, previewFormatLabel } from "../src/code-preview";

it("offers previews by case-insensitive extension only", () => {
  expect(previewFormat("docs/README.md")).toBe("markdown");
  expect(previewFormat("report.HTML")).toBe("html");
  expect(previewFormat("index.htm")).toBe("html");
  expect(previewFormat("data.csv")).toBe("csv");
  expect(previewFormat("data.TSV")).toBe("tsv");
  expect(previewFormat("result.json")).toBe("json");
  for (const path of [
    "notes.txt",
    "events.jsonl",
    "page.xhtml",
    "csv",
    "a.json.bak",
  ])
    expect(previewFormat(path)).toBeUndefined();
  expect(previewFormatLabel("csv")).toBe("预览表格");
  expect(previewFormatLabel("json")).toBe("预览 JSON");
});
