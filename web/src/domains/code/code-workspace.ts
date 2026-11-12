import type { Snippet } from "svelte";
import type { z } from "zod";
import { api } from "../../platform/api";
import { nodeId } from "./code-reference";
import {
  CodeChangesSchema,
  CodeCheckSchema,
  CodeContextSchema,
  CodeDiffSchema,
  CodeFileSchema,
  CodeListingSchema,
  type CodeDiff,
  type CodeDocumentViewState,
  type CodeFile,
  type CodeListing,
} from "../../contracts/schemas";

export type CodeContext = z.infer<typeof CodeContextSchema>;

export type CodeWorkspaceTab = {
  key: string;
  // The root the tab was opened from; refreshes and checks use it even
  // after the navigator switches roots.
  contextId: string;
  rootKind: CodeContext["rootKind"];
  file?: CodeFile;
  diff?: CodeDiff;
  status: string;
  sequence: number;
  comparison?: CodeDiff;
  // Narrow-screen menu content supplied by the tab's document view.
  menu?: Snippet;
  // A heading a Markdown link asked to show once the file is previewed.
  heading?: { slug: string; key: number };
  // A line or file view a code link asked to show in the file tab.
  jump?: { line?: number; endLine?: number; column?: number; key: number };
  // Reading state captured in another window, handed to the first view.
  restore?: CodeDocumentViewState;
};

export type CodeTabOrigin = Pick<CodeWorkspaceTab, "contextId" | "rootKind">;

export function aborted(e: unknown) {
  return e instanceof DOMException && e.name === "AbortError";
}

export function upsertWorkspaceTab(
  tabs: CodeWorkspaceTab[],
  origin: CodeTabOrigin,
  file?: CodeFile,
  diff?: CodeDiff,
): { key: string; added: boolean; error?: string } | undefined {
  const d = file?.document ?? diff?.new;
  if (!d) return undefined;
  const key = `${origin.contextId}:${d.nodeId}:${diff ? "diff:" + diff.change.group : d.source}`;
  const existing = tabs.find((t) => t.key === key);
  if (existing) {
    if (
      file
        ? file.document.contentVersion !==
          existing.file?.document.contentVersion
        : diff?.new.contentVersion !== existing.diff?.new.contentVersion ||
          diff?.old.contentVersion !== existing.diff?.old.contentVersion
    )
      existing.status = "updated";
    return { key, added: false };
  }
  if (tabs.length >= 20)
    return { key, added: false, error: "最多打开 20 个标签，请先关闭一个" };
  tabs.push({
    key,
    contextId: origin.contextId,
    rootKind: origin.rootKind,
    file,
    diff,
    status: "unchanged",
    sequence: 0,
  });
  return { key, added: true };
}

// Keep the narrow menu clear of the status bar and home indicator; env()
// is readable only through computed custom properties.
export function safeAreaPadding(section?: HTMLElement) {
  const style = getComputedStyle(section ?? document.documentElement);
  const inset = (name: string) => parseFloat(style.getPropertyValue(name)) || 0;
  return {
    top: 8 + inset("--safe-top"),
    bottom: 8 + inset("--safe-bottom"),
    left: 8,
    right: 8,
  };
}

// Below 1024px the navigator overlays the document, so selecting a file must
// reveal it; side-by-side layouts keep the navigator until the user collapses it.
export function shouldCloseOverlayNav(section?: HTMLElement) {
  return Boolean(
    section?.closest('[data-terminal-compact="true"]') ||
    matchMedia("(max-width:1023px)").matches,
  );
}

export async function fetchRefreshedTabContent(
  base: string,
  params: (extra?: Record<string, string>) => URLSearchParams,
  tab: CodeWorkspaceTab,
  signal?: AbortSignal,
): Promise<{ file?: CodeFile; diff?: CodeDiff }> {
  const contextId = tab.contextId;
  if (tab.diff) {
    const next = await api(
      `${base}/changes?${params({ contextId, nodeId: tab.diff.change.nodeId })}`,
      CodeChangesSchema,
      "GET",
      undefined,
      signal,
    );
    const c = next.entries.find(
      (c) =>
        c.group === tab.diff!.change.group &&
        c.newPath === tab.diff!.change.newPath,
    );
    if (!c) throw new Error("此差异已不存在，请从文件导航打开完整文件");
    return {
      diff: await api(
        `${base}/diff?${params({ contextId, changeId: c.changeId })}`,
        CodeDiffSchema,
        "GET",
        undefined,
        signal,
      ),
    };
  }
  if (!tab.file) throw new Error("文件不可用");
  return {
    file: await api(
      `${base}/file?${params({ contextId, nodeId: tab.file.document.nodeId, source: tab.file.document.source })}`,
      CodeFileSchema,
      "GET",
      undefined,
      signal,
    ),
  };
}

export async function fetchNavigatorListingPage(options: {
  base: string;
  params: (extra?: Record<string, string>) => URLSearchParams;
  requestQuery: string;
  requestIgnored: boolean;
  rootIncludeIgnored: boolean;
  listing: CodeListing | undefined;
  rootListing: CodeListing | undefined;
  append: boolean;
  signal: AbortSignal;
  isCurrent: () => boolean;
  onResetRoot: (root: CodeListing) => void;
  onSuccess: (next: CodeListing, identity: string) => void;
}) {
  if (
    options.requestQuery &&
    options.rootIncludeIgnored !== options.requestIgnored
  ) {
    const rootVal = await api(
      `${options.base}/tree?${options.params({ nodeId: nodeId("") })}`,
      CodeListingSchema,
      "GET",
      undefined,
      options.signal,
    );
    if (!options.isCurrent()) return;
    options.onResetRoot(rootVal);
  }
  const previous = options.requestQuery ? options.listing : options.rootListing;
  const cursor = options.append ? (previous?.nextCursor ?? "") : "";
  const value = await api(
    `${options.base}/${options.requestQuery ? "files" : "tree"}?${options.params({ nodeId: nodeId(""), q: options.requestQuery, cursor })}`,
    CodeListingSchema,
    "GET",
    undefined,
    options.signal,
  );
  if (!options.isCurrent()) return;
  const merged =
    options.append && previous
      ? {
          ...value,
          children: [...previous.children, ...value.children],
        }
      : value;
  options.onSuccess(merged, options.requestQuery ? "" : nodeId(""));
}

export async function runWorkspaceStalenessCheck(options: {
  base: string;
  current: CodeContext;
  tabs: CodeWorkspaceTab[];
  listingVersion: string | undefined;
  branchVersions: Record<string, string>;
  changesVersion: string | undefined;
  identity: string;
  nav: string;
  query: string;
  includeIgnored: boolean;
  signal: AbortSignal;
}) {
  const documents = options.tabs.map((t) => ({
    tab: t,
    sid: t.file?.snapshotId ?? t.diff!.snapshotId,
    sequence: t.sequence,
  }));
  const directories =
    options.listingVersion !== undefined &&
    options.nav === "files" &&
    !options.query
      ? Object.entries({
          ...options.branchVersions,
          [options.identity]: options.listingVersion,
        })
      : [];
  const request = (
    contextId: string,
    docs: typeof documents,
    dirs: typeof directories,
    version: string,
  ) =>
    api(
      `${options.base}/check`,
      CodeCheckSchema,
      "POST",
      {
        contextId,
        snapshotIds: [
          ...new Set(
            docs.flatMap((d) =>
              d.tab.comparison ? [d.sid, d.tab.comparison.snapshotId] : [d.sid],
            ),
          ),
        ],
        directories: Object.fromEntries(dirs),
        changesVersion: version,
        includeIgnored: options.includeIgnored,
      },
      options.signal,
    );
  const inContext = (contextId: string) =>
    documents.filter((d) => d.tab.contextId === contextId);
  const others = [...new Set(documents.map((d) => d.tab.contextId))].filter(
    (contextId) => contextId !== options.current.contextId,
  );
  const r = await request(
    options.current.contextId,
    inContext(options.current.contextId),
    directories.slice(0, 30),
    options.changesVersion ?? "",
  );
  // The check endpoint accepts at most 30 directories per request.
  for (let start = 30; start < directories.length; start += 30) {
    const next = await request(
      options.current.contextId,
      [],
      directories.slice(start, start + 30),
      "",
    );
    Object.assign(r.directories, next.directories);
  }
  for (const contextId of others) {
    try {
      const next = await request(contextId, inContext(contextId), [], "");
      Object.assign(r.snapshots, next.snapshots);
    } catch (e) {
      // An expired root leaves its tabs unknown, not the active root.
      if (aborted(e)) throw e;
    }
  }
  return { documents, result: r };
}

export function applyWorkspaceStalenessResult(options: {
  documents: { tab: CodeWorkspaceTab; sid: string; sequence: number }[];
  result: z.infer<typeof CodeCheckSchema>;
  tabs: CodeWorkspaceTab[];
  current: CodeContext;
  activeContext: CodeContext | undefined;
  treeGeneration: number;
  treeRefresh: number;
  checkIgnored: boolean;
  includeIgnored: boolean;
  identity: string;
  navIdentity: string;
  listingVersion: string | undefined;
  rootListingVersion: string | undefined;
  branchVersions: Record<string, string>;
  directoryVersions: Record<string, string>;
  changesVersion: string | undefined;
  activeChangesVersion: string | undefined;
}): string | undefined {
  const { result: r } = options;
  for (const d of options.documents) {
    if (
      options.tabs.includes(d.tab) &&
      d.tab.sequence === d.sequence &&
      (d.tab.file?.snapshotId ?? d.tab.diff?.snapshotId) === d.sid
    )
      d.tab.status =
        d.tab.comparison &&
        r.snapshots[d.tab.comparison.snapshotId] !== "unchanged"
          ? (r.snapshots[d.tab.comparison.snapshotId] ?? "unknown")
          : (r.snapshots[d.sid] ?? "unknown");
  }
  if (options.current !== options.activeContext) return undefined;
  let status: string | undefined;
  if (
    options.treeGeneration === options.treeRefresh &&
    options.checkIgnored === options.includeIgnored &&
    options.identity === options.navIdentity &&
    options.listingVersion === options.rootListingVersion &&
    Object.entries(r.directories).some(
      ([key, s]) =>
        s !== "unchanged" &&
        (key === options.identity ||
          options.branchVersions[key] === options.directoryVersions[key]),
    )
  )
    status = "目录已更新或检测失败";
  if (
    options.changesVersion !== undefined &&
    options.changesVersion === options.activeChangesVersion &&
    r.changes !== "unchanged"
  )
    status = "修改列表已更新或检测失败";
  return status;
}
