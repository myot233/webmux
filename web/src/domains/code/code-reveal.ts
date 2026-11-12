import type { z } from "zod";
import { api, apiBlob, APIError } from "../../platform/api";
import { codeRawPath, isImageType } from "./code-image";
import {
  aborted,
  type CodeContext,
  type CodeWorkspaceTab,
} from "./code-workspace";
import {
  SymlinkTargetError,
  targetFailure,
  type MarkdownWorkspace,
} from "./markdown-link";
import {
  CodeContextSchema,
  CodeFileSchema,
  CodeLocationSchema,
  type CodeReveal,
} from "../../contracts/schemas";

// Resolves a root-relative path from a Markdown preview. The server walks
// the absolute path through symlinks, so a canonical path that differs from
// the requested one went through a link.
async function resolveWorkspaceTarget(
  base: string,
  context: CodeContext | undefined,
  params: (extra?: Record<string, string>) => URLSearchParams,
  path: string,
  signal?: AbortSignal,
) {
  if (!context) throw new Error("代码工作区尚未就绪");
  const root = context.rootPath.replace(/\/+$/, "");
  const target = await api(
    `${base}/locate?${params({ path: path ? `${root}/${path}` : root || "/" })}`,
    CodeLocationSchema,
    "GET",
    undefined,
    signal,
  );
  if (target.path !== path) throw new SymlinkTargetError();
  return target;
}

export function createMarkdownWorkspace(options: {
  id: () => string;
  base: () => string;
  getContext: () => CodeContext | undefined;
  params: (extra?: Record<string, string>) => URLSearchParams;
  show: (target: z.infer<typeof CodeLocationSchema>) => Promise<string>;
  getTabs: () => CodeWorkspaceTab[];
  nextHeadingKey: () => number;
}): MarkdownWorkspace {
  return {
    async open(path, heading = "") {
      let target: z.infer<typeof CodeLocationSchema>;
      try {
        target = await resolveWorkspaceTarget(
          options.base(),
          options.getContext(),
          options.params,
          path,
        );
      } catch (e) {
        return targetFailure("链接目标", e);
      }
      const failure = await options.show(target);
      if (failure) return `链接目标不可读：${failure}`;
      const context = options.getContext();
      const tab = options
        .getTabs()
        .find(
          (t) => t.key === `${context?.contextId}:${target.nodeId}:worktree`,
        );
      if (heading && tab)
        tab.heading = { slug: heading, key: options.nextHeadingKey() };
      return "";
    },
    async image(path, signal) {
      try {
        const target = await resolveWorkspaceTarget(
          options.base(),
          options.getContext(),
          options.params,
          path,
          signal,
        );
        if (target.kind !== "file")
          return { ok: false, reason: "不是可预览的图片" };
        const { snapshotId, document: doc } = await api(
          `${options.base()}/file?${options.params({ nodeId: target.nodeId })}`,
          CodeFileSchema,
          "GET",
          undefined,
          signal,
        );
        if (doc.kind === "too-large" && isImageType(doc.mediaType))
          return { ok: false, reason: "超过 20 MiB 预览上限" };
        if (
          doc.kind !== "image" &&
          !(doc.kind === "text" && doc.mediaType === "image/svg+xml")
        )
          return { ok: false, reason: "不是可预览的图片" };
        const blob = await apiBlob(
          codeRawPath(options.id(), snapshotId),
          signal,
        );
        // An <img> renders SVG only with its media type; slicing is a view.
        return {
          ok: true,
          path: doc.path,
          blob:
            blob.type === doc.mediaType
              ? blob
              : blob.slice(0, blob.size, doc.mediaType),
        };
      } catch (e) {
        if (aborted(e)) throw e;
        return {
          ok: false,
          reason: targetFailure("图片", e),
          retry: e instanceof APIError && e.status === 429,
        };
      }
    },
  };
}

// Candidate order: a relative path resolves against the session's current
// directory first (where the agent runs, so where its paths are written
// from), then the repository root. A same-named file under the current
// directory therefore wins over the repository-root one.
function revealCandidates(origin: CodeContext, target: CodeReveal) {
  const rootPrefix = origin.rootPath.replace(/\/+$/, "");
  const list: { path: string; raw?: boolean }[] = [{ path: target.path }];
  const add = (path: string, raw = false) => {
    if (path && !list.some((item) => item.path === path))
      list.push({ path, raw });
  };
  if (!target.fromUri) {
    if (!target.path.startsWith("/")) add(`${rootPrefix}/${target.path}`);
    else if (
      target.path !== origin.rootPath &&
      !target.path.startsWith(`${rootPrefix}/`)
    )
      add(`${rootPrefix}${target.path}`);
  }
  if (target.rawPath && target.rawPath !== target.path) {
    add(target.rawPath, true);
    if (!target.fromUri && !target.rawPath.startsWith("/"))
      add(`${rootPrefix}/${target.rawPath}`, true);
  }
  return list;
}

export async function locateRevealTarget(
  base: string,
  params: (extra?: Record<string, string>) => URLSearchParams,
  origin: CodeContext,
  target: CodeReveal,
) {
  const candidates = revealCandidates(origin, target);
  let firstError: unknown;
  for (let i = 0; i < candidates.length; i++) {
    const candidate = candidates[i]!;
    try {
      const location = await api(
        `${base}/locate?${params({ contextId: origin.contextId, path: candidate.path })}`,
        CodeLocationSchema,
      );
      return { origin, location, usedRawPath: !!candidate.raw };
    } catch (e) {
      if (i === 0) firstError = e;
      if (
        !(
          e instanceof APIError &&
          (e.status === 404 ||
            e.status === 409 ||
            (!target.fromUri && e.status === 403))
        )
      )
        break;
    }
  }
  if (
    target.path.startsWith("/") &&
    firstError instanceof APIError &&
    firstError.status === 403
  ) {
    try {
      const artifactsOrigin = await api(
        `${base}/context?root=artifacts`,
        CodeContextSchema,
      );
      const artifactPaths =
        target.rawPath && target.rawPath !== target.path
          ? [
              { path: target.path, raw: false },
              { path: target.rawPath, raw: true },
            ]
          : [{ path: target.path, raw: false }];
      for (const item of artifactPaths) {
        try {
          const location = await api(
            `${base}/locate?${params({ contextId: artifactsOrigin.contextId, path: item.path })}`,
            CodeLocationSchema,
          );
          return {
            origin: artifactsOrigin,
            location,
            usedRawPath: item.raw,
          };
        } catch {
          // Try rawPath if present; otherwise report the workspace error.
        }
      }
    } catch {
      // No existing artifacts folder; report the workspace error.
    }
  }
  throw firstError;
}
