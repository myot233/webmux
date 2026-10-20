import type { CodeDocument } from "./schemas";

export function codeReference(doc: CodeDocument, start?: number, end?: number) {
  if (!doc.referenceable || doc.source === "empty")
    throw new Error("此路径无法无损引用");
  if (
    start !== undefined &&
    (!Number.isInteger(start) ||
      start < 1 ||
      start > doc.lineCount ||
      !Number.isInteger(end ?? start) ||
      (end ?? start) < start ||
      (end ?? start) > doc.lineCount ||
      doc.kind !== "text")
  )
    throw new Error("无效行范围");
  // Keep control characters in the existing reversible display notation. Its
  // double quotes still need to suppress shell substitutions.
  // oxlint-disable-next-line no-control-regex -- Escape control bytes before sending paths to a terminal.
  const path = /[\x00-\x1f\x7f-\x9f\u2028\u2029]/.test(doc.absolutePath)
    ? JSON.stringify(doc.absolutePath).replace(
        /[\x7f-\x9f\u2028\u2029$`]/g,
        (c) =>
          c === "$" || c === "`"
            ? "\\" + c
            : "\\u" + c.charCodeAt(0).toString(16).padStart(4, "0"),
      )
    : /^[A-Za-z0-9_./:@%+=,-]+$/.test(doc.absolutePath)
      ? doc.absolutePath
      : "'" + doc.absolutePath.replaceAll("'", "'\\''") + "'";
  const range =
    start === undefined
      ? ""
      : `:${start}${end !== undefined && end !== start ? `-${end}` : ""}`;
  const source =
    doc.source === "index"
      ? "（索引版本）"
      : doc.source === "head"
        ? "（HEAD 旧版本）"
        : "";
  return `${path}${range}${source} `;
}
export function nodeId(path: string) {
  const bytes = new TextEncoder().encode(path);
  return btoa(String.fromCharCode(...bytes))
    .replaceAll("+", "-")
    .replaceAll("/", "_")
    .replaceAll("=", "");
}
