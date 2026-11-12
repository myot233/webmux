import { describe, expect, test } from "vitest";
import { osc52CopyText } from "../src/domains/terminal/terminal-clipboard";

function payload(text: string): string {
  const bytes = new TextEncoder().encode(text);
  let binary = "";
  for (const byte of bytes) binary += String.fromCharCode(byte);
  return btoa(binary);
}

describe("OSC 52 application copy", () => {
  test("decodes clipboard, primary, select, and the default target", () => {
    expect(osc52CopyText("c;aGVsbG8=")).toBe("hello");
    expect(osc52CopyText(`c;${payload("中文")}`)).toBe("中文");
    expect(osc52CopyText(`p;${payload("primary")}`)).toBe("primary");
    expect(osc52CopyText(`s;${payload("select")}`)).toBe("select");
    expect(osc52CopyText(`;${payload("default")}`)).toBe("default");
    expect(osc52CopyText(`cp;${payload("both")}`)).toBe("both");
  });

  test("keeps newlines inside the copied text and ignores wrapping in base64", () => {
    expect(osc52CopyText(`c;${payload("中文 tail\nsecond")}`)).toBe(
      "中文 tail\nsecond",
    );
    expect(osc52CopyText("c;aGVs\r\n bG8=")).toBe("hello");
    expect(osc52CopyText("c;aGVsbG8")).toBe("hello");
  });

  test("clears the clipboard for an empty payload", () => {
    expect(osc52CopyText("c;")).toBe("");
    expect(osc52CopyText("c; \n")).toBe("");
  });

  test("ignores queries, cut buffers, and malformed UTF-8 base64", () => {
    expect(osc52CopyText("c;?")).toBeUndefined();
    expect(osc52CopyText(";?")).toBeUndefined();
    expect(osc52CopyText("0;YQ==")).toBeUndefined();
    expect(osc52CopyText("c;!!!!")).toBeUndefined();
    expect(osc52CopyText("c")).toBeUndefined();
    expect(osc52CopyText("c;YQ===")).toBeUndefined();
    // 0xFF is not valid UTF-8.
    expect(osc52CopyText("c;/w==")).toBeUndefined();
  });
});
