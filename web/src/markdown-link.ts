import { APIError } from "./api";

/**
 * A link or image address inside a previewed Markdown file, resolved against
 * the document's root-relative path. `path` is root-relative ("" is the root).
 */
export type MarkdownReference =
  | { kind: "anchor"; fragment: string }
  | { kind: "path"; path: string; fragment: string }
  | { kind: "outside" }
  | { kind: "invalid" };

/** Workspace access the code panel gives a Markdown preview. */
export type MarkdownWorkspace = {
  /** Opens a file tab or reveals a directory; resolves to a failure reason, "" on success. */
  open(path: string, heading?: string): Promise<string>;
  /** Reads a root-relative image through the snapshot raw endpoint. */
  image(path: string, signal: AbortSignal): Promise<MarkdownImage>;
};
export type MarkdownImage =
  | { ok: true; blob: Blob; path: string }
  | { ok: false; reason: string; retry?: boolean };

/** Raised when a path resolves through a symbolic link. */
export class SymlinkTargetError extends Error {}

/**
 * True for addresses without a scheme or authority, which the preview resolves
 * inside the workspace instead of handing to the browser.
 */
export const isWorkspaceReference = (value: string) =>
  value !== "" && !/^[a-z][a-z\d+.-]*:/i.test(value) && !value.startsWith("//");

function decode(value: string) {
  try {
    return decodeURIComponent(value);
  } catch {
    return undefined;
  }
}

/**
 * Normalizes a relative address with POSIX rules: the query is ignored, the
 * path is percent-decoded before `.` and `..` apply (so `%2e%2e` climbs),
 * repeated and trailing slashes collapse, and a leading `/` starts at the
 * workspace root. Climbing above the root is rejected here; the server still
 * checks the final path.
 */
export function resolveMarkdownReference(
  documentPath: string,
  reference: string,
): MarkdownReference {
  const hash = reference.indexOf("#");
  const beforeHash = hash < 0 ? reference : reference.slice(0, hash);
  const rawFragment = hash < 0 ? "" : reference.slice(hash + 1);
  const fragment = decode(rawFragment) ?? rawFragment;
  const query = beforeHash.indexOf("?");
  const rawPath = query < 0 ? beforeHash : beforeHash.slice(0, query);
  if (!rawPath)
    return fragment ? { kind: "anchor", fragment } : { kind: "invalid" };
  const decoded = decode(rawPath);
  if (decoded === undefined || decoded.includes("\0"))
    return { kind: "invalid" };
  const segments = decoded.startsWith("/")
    ? []
    : documentPath.split("/").slice(0, -1);
  for (const segment of decoded.split("/")) {
    if (segment === "" || segment === ".") continue;
    if (segment !== "..") segments.push(segment);
    else if (segments.pop() === undefined) return { kind: "outside" };
  }
  return { kind: "path", path: segments.filter(Boolean).join("/"), fragment };
}

/** Explains why a link target or image could not be resolved or read. */
export function targetFailure(subject: "链接目标" | "图片", error: unknown) {
  if (error instanceof SymlinkTargetError)
    return `${subject}是符号链接，不在预览中打开`;
  if (error instanceof APIError)
    switch (error.status) {
      case 403:
        return `${subject}不在工作区内`;
      case 404:
        return `${subject}不存在`;
      case 409:
      case 410:
        return `${subject}内容已变化，请刷新`;
      case 429:
        return `${subject}传输繁忙，可重试`;
      default:
        return `${subject}不可读：${error.message}`;
    }
  if (error instanceof TypeError) return `${subject}不可读：网络连接异常`;
  return `${subject}不可读：${error instanceof Error ? error.message : String(error)}`;
}
