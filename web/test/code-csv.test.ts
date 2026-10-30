import { describe, expect, it } from "vitest";
import { maxTableRows, parseDelimited } from "../src/code-csv";

const parsed = (text: string, delimiter: "," | "\t" = ",") => {
  const table = parseDelimited(text, delimiter);
  if (!table.ok) throw new Error(table.message);
  return table;
};

describe("parseDelimited", () => {
  it("reads quoted delimiters, escaped quotes and multi-line fields", () => {
    const table = parsed(
      'name,note,extra\n"a,b","他说""好""","第一行\n第二行"\nplain,x,y\n',
    );
    expect(table.header).toEqual(["name", "note", "extra"]);
    expect(table.rows).toEqual([
      ["a,b", '他说"好"', "第一行\n第二行"],
      ["plain", "x", "y"],
    ]);
    expect(table.total).toBe(2);
  });

  it("treats CRLF and LF alike and ignores the trailing newline", () => {
    expect(parsed("a,b\r\n1,2\r\n3,4\r\n").rows).toEqual([
      ["1", "2"],
      ["3", "4"],
    ]);
    expect(parsed('a\r\n"x\r\ny"\r\n').rows).toEqual([["x\r\ny"]]);
  });

  it("pads ragged rows to the widest record", () => {
    const table = parsed("a,b\n1\n1,2,3\n");
    expect(table.columns).toBe(3);
    expect(table.header).toEqual(["a", "b", ""]);
    expect(table.rows).toEqual([
      ["1", "", ""],
      ["1", "2", "3"],
    ]);
  });

  it("splits TSV on tabs only", () => {
    expect(parsed("a\tb\nx,y\tz\n", "\t").rows).toEqual([["x,y", "z"]]);
  });

  it("reports the line of an unclosed quote", () => {
    const lines = Array.from({ length: 10 }, (_, i) => `${i},ok`);
    const text = ["a,b", ...lines, '11,"open', "more"].join("\n");
    expect(parseDelimited(text, ",")).toEqual({
      ok: false,
      line: 12,
      message: "第 12 行引号未闭合",
    });
  });

  it("returns an empty table for an empty file", () => {
    expect(parsed("")).toEqual({
      ok: true,
      header: [],
      rows: [],
      total: 0,
      columns: 0,
    });
  });

  it("drops a UTF-8 byte order mark", () => {
    expect(parsed("\uFEFFid,name\n1,x\n").header).toEqual(["id", "name"]);
  });

  it("keeps the first rows and counts the rest", () => {
    const text = [
      "n",
      ...Array.from({ length: 5000 }, (_, i) => String(i)),
    ].join("\n");
    const table = parsed(text);
    expect(table.rows).toHaveLength(maxTableRows);
    expect(table.rows.at(-1)).toEqual([String(maxTableRows - 1)]);
    expect(table.total).toBe(5000);
  });
});
