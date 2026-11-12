import { APIError } from "../../platform/api";
import type { CodeReveal } from "../../contracts/schemas";

/** A code file, directory, line range or heading target parsed from chat output. */
export type ChatCodeLink = Omit<CodeReveal, "key" | "mode">;

/** True for external links that open in a new browser tab. */
export const isExternalLink = (value: string) =>
  /^(?:https?:|mailto:)/i.test(value.trim());

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

function splitReference(reference: string) {
  const hash = reference.indexOf("#");
  const beforeHash = hash < 0 ? reference : reference.slice(0, hash);
  const rawFragment = hash < 0 ? "" : reference.slice(hash + 1);
  const fragment = decode(rawFragment) ?? rawFragment;
  const query = beforeHash.indexOf("?");
  const rawPath = query < 0 ? beforeHash : beforeHash.slice(0, query);
  return { rawPath, fragment };
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
  const { rawPath, fragment } = splitReference(reference);
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

const disallowedBareNames = new Set([
  "about",
  "blob",
  "data",
  "ftp",
  "ftps",
  "geo",
  "git",
  "http",
  "https",
  "intent",
  "irc",
  "javascript",
  "localhost",
  "magnet",
  "mailto",
  "news",
  "nntp",
  "sip",
  "sips",
  "sms",
  "ssh",
  "tel",
  "urn",
  "vbscript",
  "web",
  "ws",
  "wss",
  "xmpp",
]);

function parseLineFragment(fragment: string): {
  line: number;
  endLine?: number;
  column?: number;
} | null {
  const match =
    /^(?:l(\d+)(?:(?:c|:)(\d+))?(?:-(?:l)?(\d+)(?:(?:c|:)\d+)?)?|(\d+)(?:-(\d+))?)$/i.exec(
      fragment,
    );
  if (!match) return null;
  const rawStart = Number(match[1] ?? match[4]);
  const rawCol = match[2] !== undefined ? Number(match[2]) : undefined;
  const rawEnd =
    match[3] !== undefined
      ? Number(match[3])
      : match[5] !== undefined
        ? Number(match[5])
        : undefined;
  if (!Number.isSafeInteger(rawStart) || rawStart < 1) return null;
  const column =
    rawCol !== undefined && Number.isSafeInteger(rawCol) && rawCol >= 1
      ? rawCol
      : undefined;
  if (rawEnd !== undefined) {
    if (!Number.isSafeInteger(rawEnd) || rawEnd < 1) return null;
    const line = Math.min(rawStart, rawEnd);
    const endLine = Math.max(rawStart, rawEnd);
    return {
      line,
      ...(endLine > line ? { endLine } : {}),
      ...(column !== undefined ? { column } : {}),
    };
  }
  return {
    line: rawStart,
    ...(column !== undefined ? { column } : {}),
  };
}

function stripLineSuffix(fullPath: string): {
  path: string;
  rawPath?: string;
  line?: number;
  endLine?: number;
  column?: number;
} {
  const match = /^(.*?):(\d+)(?:(?::(\d+))?(?:-(\d+)(?::\d+)?)?)$/.exec(
    fullPath,
  );
  if (!match || !match[1] || match[1].endsWith("/")) return { path: fullPath };
  const rawStart = Number(match[2]);
  const rawCol = match[3] !== undefined ? Number(match[3]) : undefined;
  const rawEnd = match[4] !== undefined ? Number(match[4]) : undefined;
  if (!Number.isSafeInteger(rawStart) || rawStart < 1)
    return { path: fullPath };
  if (rawEnd !== undefined && (!Number.isSafeInteger(rawEnd) || rawEnd < 1))
    return { path: fullPath };
  const line = rawEnd !== undefined ? Math.min(rawStart, rawEnd) : rawStart;
  const endLine =
    rawEnd !== undefined && Math.max(rawStart, rawEnd) > line
      ? Math.max(rawStart, rawEnd)
      : undefined;
  const column =
    rawCol !== undefined && Number.isSafeInteger(rawCol) && rawCol >= 1
      ? rawCol
      : undefined;
  return {
    path: match[1],
    rawPath: fullPath,
    line,
    ...(endLine !== undefined ? { endLine } : {}),
    ...(column !== undefined ? { column } : {}),
  };
}

function buildCodeLink(
  decodedPath: string,
  fragment: string,
  fromUri?: boolean,
): ChatCodeLink {
  const base = fromUri ? { fromUri: true as const } : {};
  const fragLine = fragment ? parseLineFragment(fragment) : null;
  if (fragLine) return { path: decodedPath, ...base, ...fragLine };
  if (fragment) return { path: decodedPath, ...base, heading: fragment };
  return { ...stripLineSuffix(decodedPath), ...base };
}

/**
 * Parses a link address in agent chat output (`file://` URI, absolute path or
 * relative path, with optional `#L10-L20`, `:10-20` or `#heading` anchor).
 * Returns `null` for external links, pure `#anchors` and disallowed schemes.
 */
export function parseChatCodeLink(href: string): ChatCodeLink | null {
  const trimmed = href.trim();
  if (!trimmed || trimmed.startsWith("#") || trimmed.startsWith("//"))
    return null;
  if (/^file:/i.test(trimmed)) {
    if (!/^file:\/\/(?:localhost)?\//i.test(trimmed)) return null;
    let url: URL;
    try {
      url = new URL(trimmed);
    } catch {
      return null;
    }
    if (
      url.protocol !== "file:" ||
      (url.hostname !== "" && url.hostname.toLowerCase() !== "localhost")
    )
      return null;
    const decodedPath = decode(url.pathname);
    if (!decodedPath || decodedPath.includes("\0")) return null;
    const rawFragment = url.hash.startsWith("#") ? url.hash.slice(1) : "";
    const fragment = decode(rawFragment) ?? rawFragment;
    return buildCodeLink(decodedPath, fragment, true);
  }
  const schemeMatch = /^([a-z][a-z\d+.-]*):(.*)$/i.exec(trimmed);
  if (schemeMatch) {
    const scheme = schemeMatch[1]!.toLowerCase();
    const rest = schemeMatch[2]!;
    if (
      disallowedBareNames.has(scheme) ||
      !/^\d+(?::\d+)?(?:-\d+(?::\d+)?)?(?:[?#].*)?$/.test(rest)
    )
      return null;
  }
  const { rawPath, fragment } = splitReference(trimmed);
  if (!rawPath) return null;
  const decodedPath = decode(rawPath);
  if (decodedPath === undefined || decodedPath.includes("\0")) return null;
  return buildCodeLink(decodedPath, fragment);
}
