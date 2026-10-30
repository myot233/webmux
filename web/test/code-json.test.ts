import { describe, expect, it } from "vitest";
import { maxJsonDepth, parseJson, type JsonNode } from "../src/code-json";

const value = (text: string): JsonNode => {
  const result = parseJson(text);
  if (!result.ok) throw new Error(`${result.line}:${result.column}`);
  return result.value;
};
const failure = (text: string) => {
  const result = parseJson(text);
  if (result.ok) throw new Error("parsed");
  return { line: result.line, column: result.column, detail: result.detail };
};

describe("parseJson", () => {
  it("keeps the source text of numbers", () => {
    expect(
      value('{"id": 12345678901234567890, "ratio": 1.10, "e": -2E+08}'),
    ).toEqual({
      line: 1,
      kind: "object",
      entries: [
        {
          key: "id",
          value: { line: 1, kind: "number", raw: "12345678901234567890" },
        },
        { key: "ratio", value: { line: 1, kind: "number", raw: "1.10" } },
        { key: "e", value: { line: 1, kind: "number", raw: "-2E+08" } },
      ],
    });
  });

  it("decodes escapes including Unicode surrogate pairs", () => {
    expect(value(String.raw`"a\"\\\/\b\f\n\r\t\u4e2d\uD83D\uDE00"`)).toEqual({
      line: 1,
      kind: "string",
      value: 'a"\\/\b\f\n\r\t中😀',
    });
  });

  it("records the line of each value and keeps duplicate keys", () => {
    const node = value('{\n  "a": [true, false, null],\n  "a": {}\n}');
    expect(
      node.kind === "object" && node.entries.map((e) => e.value.line),
    ).toEqual([2, 3]);
  });

  it("parses deep nesting up to the limit", () => {
    let node = value("[".repeat(maxJsonDepth) + "]".repeat(maxJsonDepth));
    let depth = 1;
    while (node.kind === "array" && node.items[0]) {
      node = node.items[0];
      depth++;
    }
    expect(depth).toBe(maxJsonDepth);
    expect(
      failure("[".repeat(maxJsonDepth + 1) + "]".repeat(maxJsonDepth + 1)),
    ).toEqual({
      line: 1,
      column: maxJsonDepth + 1,
      detail: `嵌套超过 ${maxJsonDepth} 层`,
    });
  });

  it("reports the line and column of syntax errors", () => {
    expect(failure('{\n  "a": 1\n  "b": 2\n}')).toEqual({
      line: 3,
      column: 3,
      detail: "缺少逗号或右花括号",
    });
    expect(failure("[1, 2,]")).toEqual({
      line: 1,
      column: 7,
      detail: "无法识别的值",
    });
    expect(failure('{"a" 1}')).toEqual({
      line: 1,
      column: 6,
      detail: "缺少冒号",
    });
    expect(failure("{1: 2}")).toEqual({
      line: 1,
      column: 2,
      detail: "缺少属性名",
    });
    expect(failure('"open')).toEqual({
      line: 1,
      column: 6,
      detail: "字符串未闭合",
    });
    expect(failure('"a\nb"')).toEqual({
      line: 1,
      column: 3,
      detail: "字符串中含未转义的控制字符",
    });
    expect(failure('"\\x"')).toEqual({
      line: 1,
      column: 2,
      detail: "无效的转义字符",
    });
    expect(failure('"\\u12G4"')).toEqual({
      line: 1,
      column: 2,
      detail: "无效的 Unicode 转义",
    });
    expect(failure("01")).toEqual({
      line: 1,
      column: 2,
      detail: "值之后有多余内容",
    });
    expect(failure("[1 2]")).toEqual({
      line: 1,
      column: 4,
      detail: "缺少逗号或右方括号",
    });
    expect(failure("")).toEqual({ line: 1, column: 1, detail: "内容意外结束" });
    expect(failure("\n\n  nope")).toEqual({
      line: 3,
      column: 3,
      detail: "无法识别的值",
    });
  });

  it("ignores a byte order mark and surrounding whitespace", () => {
    expect(value("\uFEFF \t\r\n 42 \n")).toEqual({
      line: 2,
      kind: "number",
      raw: "42",
    });
  });
});
