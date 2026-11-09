<script lang="ts">
  import { errorMessage } from "../error";
  import { dragWidth, keyWidth } from "../splitter";
  import { onMount, untrack, type Snippet } from "svelte";
  import { SvelteMap } from "svelte/reactivity";
  import CodeNavigator from "./CodeNavigator.svelte";
  import { api, apiBlob, APIError, ensureArtifactsFolder } from "../api";
  import {
    CodeContextSchema,
    CodeListingSchema,
    CodeFileSchema,
    CodeChangesSchema,
    CodeDiffSchema,
    CodeLocationSchema,
    CodeCheckSchema,
    type CodeFile,
    type CodeDiff,
    type CodeChange,
    type CodeListing,
    type CodeReveal,
    type CodeWorkspaceSnapshot,
    type CodeDocumentViewState,
  } from "../schemas";
  import type { z } from "zod";
  import type { CodeHost } from "../code-host";
  import { nodeId } from "../code-reference";
  import { codeRawPath, isImageType } from "../code-image";
  import {
    SymlinkTargetError,
    targetFailure,
    type MarkdownWorkspace,
  } from "../markdown-link";
  import { Button } from "$lib/components/ui/button";
  import { ListButton } from "$lib/components/ui/list-button";
  import * as Tabs from "$lib/components/ui/tabs";
  import IconLayoutSidebarLeftCollapse from "~icons/tabler/layout-sidebar-left-collapse";
  import IconLayoutSidebarLeftExpand from "~icons/tabler/layout-sidebar-left-expand";
  import IconPhoto from "~icons/tabler/photo";
  import IconPictureInPictureOff from "~icons/tabler/picture-in-picture-off";
  import IconPictureInPictureOn from "~icons/tabler/picture-in-picture-on";
  import IconRefresh from "~icons/tabler/refresh";
  import IconDots from "~icons/tabler/dots";
  import IconX from "~icons/tabler/x";
  import * as DropdownMenu from "$lib/components/ui/dropdown-menu";
  import CodeDocumentView from "./CodeDocumentView.svelte";
  import "../fonts-terminal.css";
  let {
    id,
    visible,
    host,
    initial,
    popout,
  }: {
    id: string;
    visible: boolean;
    host: CodeHost;
    /** Reading state captured by another instance, such as another window. */
    initial?: CodeWorkspaceSnapshot;
    /** The pop-out toggle; absent where the panel cannot change windows. */
    popout?: { popped: boolean; toggle: () => void };
  } = $props();
  const seed = untrack(() => initial);
  type CodeContext = z.infer<typeof CodeContextSchema>;
  type Tab = {
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
  let context = $state<CodeContext | undefined>(seed?.context);
  // The navigator's root; artifacts is the session's temporary folder.
  let root = $state<"workspace" | "artifacts">(seed?.root ?? "workspace");
  let rootError = $state("");
  let contextController: AbortController | undefined;
  let listing = $state<CodeListing | undefined>(seed?.listing);
  let rootListing = $state<CodeListing | undefined>(seed?.rootListing);
  let treeRefresh = $state(0);
  let rootIncludeIgnored = seed?.rootIncludeIgnored ?? false;
  let directoryVersions: Record<string, string> = {
    ...seed?.directoryVersions,
  };
  let changeQuery = $state(seed?.changeQuery ?? "");
  let changeSearch = $state(seed?.changeSearch ?? "");
  let changeGroup = $state(seed?.changeGroup ?? "");
  let changesError = $state("");
  let changes = $state<z.infer<typeof CodeChangesSchema> | undefined>(
    seed?.changes,
  );
  let tabs = $state<Tab[]>(
    (seed?.tabs ?? []).map((tab) => ({
      key: tab.key,
      contextId: tab.contextId,
      rootKind: tab.rootKind,
      file: tab.file,
      diff: tab.diff,
      comparison: tab.comparison,
      status: tab.status,
      sequence: 0,
      restore: tab.view,
    })),
  );
  let active = $state(seed?.active ?? "");
  const activePath = $derived(
    tabs.find((tab) => tab.key === active)?.file?.document.path ?? "",
  );
  let nav = $state(seed?.nav ?? "files");
  let navOpen = $state(seed?.navOpen ?? true);
  const navMin = 120,
    navMax = 640;
  let navWidth = $state(seed?.navWidth ?? 260);
  let section = $state<HTMLElement>();
  let navigator = $state<CodeNavigator>();
  let dropStatus = $state("");
  // Document views by tab, for capturing their reading state.
  const views = new WeakMap<Tab, CodeDocumentView>();
  const shown = $derived(visible || !!popout?.popped);
  const activeMenu = $derived(tabs.find((tab) => tab.key === active)?.menu);
  /** The reading state of this workspace, for another instance to restore. */
  export function capture(): CodeWorkspaceSnapshot {
    return {
      version: 1,
      root,
      context: $state.snapshot(context),
      listing: $state.snapshot(listing),
      rootListing: $state.snapshot(rootListing),
      rootIncludeIgnored,
      directoryVersions: { ...directoryVersions },
      navIdentity,
      navStatus,
      changes: $state.snapshot(changes),
      changeQuery,
      changeSearch,
      changeGroup,
      includeIgnored,
      directory,
      query,
      nav,
      navOpen,
      navWidth,
      active,
      tabs: tabs.map((tab) => ({
        key: tab.key,
        contextId: tab.contextId,
        rootKind: tab.rootKind,
        file: $state.snapshot(tab.file),
        diff: $state.snapshot(tab.diff),
        comparison: $state.snapshot(tab.comparison),
        status: tab.status,
        view: views.get(tab)?.capture() ?? $state.snapshot(tab.restore),
      })),
      navigator: navigator?.capture() ?? seed?.navigator,
    };
  }
  // Keep the narrow menu clear of the status bar and home indicator; env()
  // is readable only through computed custom properties.
  function safeAreaPadding() {
    const style = getComputedStyle(section ?? document.documentElement);
    const inset = (name: string) =>
      parseFloat(style.getPropertyValue(name)) || 0;
    return {
      top: 8 + inset("--safe-top"),
      bottom: 8 + inset("--safe-bottom"),
      left: 8,
      right: 8,
    };
  }
  let inner = $state<HTMLDivElement>();
  function setNavWidth(width: number) {
    const limit = Math.min(navMax, (inner?.clientWidth ?? navMax) - 160);
    navWidth = Math.round(Math.max(navMin, Math.min(limit, width)));
  }
  function resizeStart(event: PointerEvent) {
    dragWidth(event, navWidth, setNavWidth);
  }
  function resizeKey(event: KeyboardEvent) {
    keyWidth(event, navWidth, { min: navMin, max: navMax }, setNavWidth);
  }
  let includeIgnored = $state(seed?.includeIgnored ?? false);
  let directory = $state(seed?.directory ?? "");
  let query = $state(seed?.query ?? "");
  let error = $state("");
  let navStatus = $state(seed?.navStatus ?? "");
  let pending = $state(false);
  let controller: AbortController | undefined;
  const controllers = new SvelteMap<string, AbortController>();
  let navSequence = 0;
  let mounted = false;
  let checkController: AbortController | undefined;
  let checking = false;
  let navIdentity = seed?.navIdentity ?? "";
  let base = $derived(`/sessions/${id}/code`);
  function params(extra: Record<string, string> = {}) {
    return new URLSearchParams({
      contextId: context?.contextId ?? "",
      includeIgnored: String(includeIgnored),
      ...extra,
    });
  }
  function aborted(e: unknown) {
    return e instanceof DOMException && e.name === "AbortError";
  }
  async function load(append = false) {
    if (!context || (append && pending)) return;
    controller?.abort();
    controller = new AbortController();
    const request = controller;
    const seq = ++navSequence;
    const requestQuery = query;
    const requestIgnored = includeIgnored;
    pending = true;
    error = "";
    try {
      if (nav === "changes") {
        if (!context.diffAvailable) {
          changes = undefined;
          return;
        }
        changesError = "";
        const cursor = append ? (changes?.nextCursor ?? "") : "";
        const value = await api(
          `${base}/changes?${params({ cursor, q: changeQuery, group: changeGroup })}`,
          CodeChangesSchema,
          "GET",
          undefined,
          request.signal,
        );
        if (mounted && seq === navSequence) {
          changes =
            append && changes
              ? { ...value, entries: [...changes.entries, ...value.entries] }
              : value;
        }
      } else {
        if (requestQuery && rootIncludeIgnored !== requestIgnored) {
          const root = await api(
            `${base}/tree?${params({ nodeId: nodeId("") })}`,
            CodeListingSchema,
            "GET",
            undefined,
            request.signal,
          );
          if (!mounted || seq !== navSequence) return;
          rootListing = root;
          rootIncludeIgnored = requestIgnored;
          directoryVersions = {};
          treeRefresh++;
        }
        const previous = requestQuery ? listing : rootListing;
        const cursor = append ? (previous?.nextCursor ?? "") : "";
        const value = await api(
          `${base}/${requestQuery ? "files" : "tree"}?${params({ nodeId: nodeId(""), q: requestQuery, cursor })}`,
          CodeListingSchema,
          "GET",
          undefined,
          request.signal,
        );
        if (mounted && seq === navSequence) {
          listing =
            append && previous
              ? {
                  ...value,
                  children: [...previous.children, ...value.children],
                }
              : value;
          navIdentity = requestQuery ? "" : nodeId("");
          if (!requestQuery) {
            rootListing = listing;
            rootIncludeIgnored = requestIgnored;
            if (!append) {
              directoryVersions = {};
              treeRefresh++;
            }
          }
        }
      }
      if (seq === navSequence) navStatus = "";
    } catch (e) {
      if (!aborted(e) && mounted && seq === navSequence) {
        error = errorMessage(e);
        if (nav === "changes") changesError = error;
      }
    } finally {
      if (mounted && seq === navSequence) pending = false;
    }
  }
  async function fetchDirectory(
    path: string,
    cursor: string,
    signal: AbortSignal,
  ) {
    const opaqueId = nodeId(path);
    const value = await api(
      `${base}/tree?${params({ nodeId: opaqueId, cursor })}`,
      CodeListingSchema,
      "GET",
      undefined,
      signal,
    );
    if (!signal.aborted) directoryVersions[opaqueId] = value.contentVersion;
    return value;
  }
  let selectionSequence = 0;
  const beginOpen = () => ++selectionSequence;
  function activate(key: string) {
    beginOpen();
    active = key;
  }
  function supersedeOpenOnTabKey(event: KeyboardEvent) {
    // Selecting the same tab is still newer than an in-flight open.
    const keys = ["Enter", " ", "ArrowLeft", "ArrowRight", "Home", "End"];
    if (keys.includes(event.key)) beginOpen();
  }
  type TabOrigin = Pick<Tab, "contextId" | "rootKind">;
  function install(
    origin: TabOrigin,
    file?: CodeFile,
    diff?: CodeDiff,
    shouldActivate = true,
  ) {
    const d = file?.document ?? diff?.new;
    if (!d) return;
    const key = `${origin.contextId}:${d.nodeId}:${diff ? "diff:" + diff.change.group : d.source}`;
    const existing = tabs.find((t) => t.key === key);
    if (existing) {
      if (shouldActivate) active = key;
      if (
        file
          ? file.document.contentVersion !==
            existing.file?.document.contentVersion
          : diff?.new.contentVersion !== existing.diff?.new.contentVersion ||
            diff?.old.contentVersion !== existing.diff?.old.contentVersion
      )
        existing.status = "updated";
      return;
    }
    if (tabs.length >= 20) {
      error = "最多打开 20 个标签，请先关闭一个";
      return;
    }
    tabs.push({
      key,
      contextId: origin.contextId,
      rootKind: origin.rootKind,
      file,
      diff,
      status: "unchanged",
      sequence: 0,
    });
    if (shouldActivate) {
      active = key;
      closeOverlayNav();
    }
  }
  // Below 1024px the navigator overlays the document, so selecting a file must
  // reveal it; side-by-side layouts keep the navigator until the user collapses it.
  function closeOverlayNav() {
    if (
      section?.closest('[data-terminal-compact="true"]') ||
      matchMedia("(max-width:1023px)").matches
    )
      navOpen = false;
  }
  // The server resolves typed paths against the canonical code root; shell
  // reports and launch directories may be logical paths through symlinks.
  async function locate(input: string) {
    if (!context) return;
    await show(
      await api(
        `${base}/locate?${params({ path: input })}`,
        CodeLocationSchema,
      ),
    );
  }
  // Reveals a directory in the file navigator or opens a file; resolves to
  // the reason a file could not be opened, "" otherwise.
  async function show(target: z.infer<typeof CodeLocationSchema>) {
    if (target.kind === "file") return openFile(target.path, target.nodeId);
    nav = "files";
    navOpen = true;
    directory = "";
    query = "";
    await load();
    directory = target.path;
    return "";
  }
  async function openFileInOrigin(
    origin: TabOrigin,
    path: string,
    opaqueId = nodeId(path),
  ) {
    return openTab(
      `${origin.contextId}:${opaqueId}:worktree`,
      `${base}/file?${params({ contextId: origin.contextId, nodeId: opaqueId })}`,
      CodeFileSchema,
      (value, activate) => install(origin, value, undefined, activate),
    );
  }
  async function openFile(path: string, opaqueId = nodeId(path)) {
    const origin = context;
    if (!origin) return "";
    return openFileInOrigin(origin, path, opaqueId);
  }
  // Resolves a root-relative path from a Markdown preview. The server walks
  // the absolute path through symlinks, so a canonical path that differs from
  // the requested one went through a link.
  async function workspaceTarget(path: string, signal?: AbortSignal) {
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
  let headingSequence = 0;
  let jumpSequence = 0;
  const markdownWorkspace: MarkdownWorkspace = {
    async open(path, heading = "") {
      let target: z.infer<typeof CodeLocationSchema>;
      try {
        target = await workspaceTarget(path);
      } catch (e) {
        return targetFailure("链接目标", e);
      }
      const failure = await show(target);
      if (failure) return `链接目标不可读：${failure}`;
      const tab = tabs.find(
        (t) => t.key === `${context?.contextId}:${target.nodeId}:worktree`,
      );
      if (heading && tab)
        tab.heading = { slug: heading, key: ++headingSequence };
      return "";
    },
    async image(path, signal) {
      try {
        const target = await workspaceTarget(path, signal);
        if (target.kind !== "file")
          return { ok: false, reason: "不是可预览的图片" };
        const { snapshotId, document: doc } = await api(
          `${base}/file?${params({ nodeId: target.nodeId })}`,
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
        const blob = await apiBlob(codeRawPath(id, snapshotId), signal);
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
  async function openDiff(change: CodeChange) {
    const origin = context;
    if (!origin) return;
    await openTab(
      `${origin.contextId}:${change.nodeId}:diff:${change.group}`,
      `${base}/diff?${params({ changeId: change.changeId })}`,
      CodeDiffSchema,
      (value, activate) => install(origin, undefined, value, activate),
    );
  }
  // Focuses an already open tab, or fetches and installs it. A response only
  // activates its tab when no later open request superseded it. Resolves to
  // the reason the tab could not be read, "" otherwise.
  async function openTab<T extends z.ZodType>(
    key: string,
    path: string,
    schema: T,
    done: (value: z.infer<T>, activate: boolean) => void,
  ) {
    const intent = beginOpen();
    if (tabs.some((t) => t.key === key)) {
      active = key;
      closeOverlayNav();
      void check();
      return "";
    }
    return read(
      key,
      () => api(path, schema, "GET", undefined, controllers.get(key)?.signal),
      (value) => done(value, intent === selectionSequence),
    );
  }
  async function read<T>(
    key: string,
    request: () => Promise<T>,
    done: (value: T) => void,
  ) {
    controllers.get(key)?.abort();
    const c = new AbortController();
    controllers.set(key, c);
    error = "";
    try {
      const value = await request();
      if (mounted && !c.signal.aborted && controllers.get(key) === c)
        done(value);
      return "";
    } catch (e) {
      if (!mounted || aborted(e)) return "";
      error = errorMessage(e);
      return error;
    } finally {
      if (controllers.get(key) === c) controllers.delete(key);
    }
  }
  function close(key: string) {
    controllers.get(key)?.abort();
    controllers.delete(key);
    tabs = tabs.filter((t) => t.key !== key);
    if (active === key) activate(tabs.at(-1)?.key ?? "");
  }
  // After a file operation: tabs of the current root showing a path that no
  // longer exists are closed, and the navigator re-reads its directories.
  function filesChanged(removed: string[]) {
    const current = context?.contextId;
    for (const tab of tabs.filter((t) => {
      const path = t.file?.document.path ?? t.diff?.new.path ?? "";
      return (
        t.contextId === current &&
        removed.some((p) => path === p || path.startsWith(p + "/"))
      );
    }))
      close(tab.key);
    void load();
  }
  async function refresh(tab: Tab) {
    const seq = ++tab.sequence;
    await read(
      tab.key,
      async () => {
        const signal = controllers.get(tab.key)?.signal;
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
      },
      (value) => {
        if (!tabs.includes(tab) || tab.sequence !== seq) return;
        tab.file = value.file;
        tab.diff = value.diff;
        tab.status = "unchanged";
        tab.comparison = undefined;
      },
    );
  }
  async function check() {
    const current = context;
    if (!mounted || !shown || document.hidden || !current || checking) return;
    checking = true;
    checkController = new AbortController();
    const c = checkController;
    const documents = tabs.map((t) => ({
      tab: t,
      sid: t.file?.snapshotId ?? t.diff!.snapshotId,
      sequence: t.sequence,
    }));
    const listingVersion = rootListing?.contentVersion;
    const branchVersions = { ...directoryVersions };
    const changesVersion = changes?.contentVersion;
    const identity = navIdentity;
    const treeGeneration = treeRefresh;
    const checkIgnored = includeIgnored;
    const directories =
      rootListing && nav === "files" && !query
        ? Object.entries({ ...branchVersions, [identity]: listingVersion })
        : [];
    // Snapshots belong to the root they were read from; tabs of the other
    // root are checked under their own context.
    const request = (
      contextId: string,
      docs: typeof documents,
      dirs: typeof directories,
      version: string,
    ) =>
      api(
        `${base}/check`,
        CodeCheckSchema,
        "POST",
        {
          contextId,
          snapshotIds: [
            ...new Set(
              docs.flatMap((d) =>
                d.tab.comparison
                  ? [d.sid, d.tab.comparison.snapshotId]
                  : [d.sid],
              ),
            ),
          ],
          directories: Object.fromEntries(dirs),
          changesVersion: version,
          includeIgnored,
        },
        c.signal,
      );
    const inContext = (contextId: string) =>
      documents.filter((d) => d.tab.contextId === contextId);
    const others = [...new Set(documents.map((d) => d.tab.contextId))].filter(
      (contextId) => contextId !== current.contextId,
    );
    try {
      const r = await request(
        current.contextId,
        inContext(current.contextId),
        directories.slice(0, 30),
        changesVersion ?? "",
      );
      // The check endpoint accepts at most 30 directories per request.
      for (let start = 30; start < directories.length; start += 30) {
        const next = await request(
          current.contextId,
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
      if (!mounted || c.signal.aborted) return;
      for (const d of documents) {
        if (
          tabs.includes(d.tab) &&
          d.tab.sequence === d.sequence &&
          (d.tab.file?.snapshotId ?? d.tab.diff?.snapshotId) === d.sid
        )
          d.tab.status =
            d.tab.comparison &&
            r.snapshots[d.tab.comparison.snapshotId] !== "unchanged"
              ? (r.snapshots[d.tab.comparison.snapshotId] ?? "unknown")
              : (r.snapshots[d.sid] ?? "unknown");
      }
      if (current !== context) return;
      if (
        treeGeneration === treeRefresh &&
        checkIgnored === includeIgnored &&
        identity === navIdentity &&
        listingVersion === rootListing?.contentVersion &&
        Object.entries(r.directories).some(
          ([key, status]) =>
            status !== "unchanged" &&
            (key === identity ||
              branchVersions[key] === directoryVersions[key]),
        )
      )
        navStatus = "目录已更新或检测失败";
      if (
        changesVersion !== undefined &&
        changesVersion === changes?.contentVersion &&
        r.changes !== "unchanged"
      )
        navStatus = "修改列表已更新或检测失败";
    } catch (e) {
      if (!aborted(e) && mounted) {
        for (const d of documents) {
          if (tabs.includes(d.tab) && d.tab.sequence === d.sequence)
            d.tab.status = "unknown";
        }
        if (current === context) navStatus = "检测失败，状态未知";
      }
    } finally {
      checking = false;
    }
  }
  // Reads the navigator's root context. The artifacts folder is ensured
  // first; a failure leaves the navigator empty rather than showing another
  // root's content.
  async function loadContext() {
    contextController?.abort();
    const request = new AbortController();
    contextController = request;
    const kind = root;
    rootError = "";
    try {
      if (kind === "artifacts") await ensureArtifactsFolder(id, request.signal);
      const value = await api(
        `${base}/context${kind === "artifacts" ? "?root=artifacts" : ""}`,
        CodeContextSchema,
        "GET",
        undefined,
        request.signal,
      );
      if (!mounted || request.signal.aborted) return;
      context = value;
      void load();
    } catch (e) {
      if (!mounted || aborted(e) || request.signal.aborted) return;
      if (kind === "artifacts") rootError = errorMessage(e);
      else error = errorMessage(e);
    }
  }
  function switchRoot(kind: "workspace" | "artifacts") {
    if (kind === root) return;
    root = kind;
    controller?.abort();
    checkController?.abort();
    navSequence++;
    context = undefined;
    listing = undefined;
    rootListing = undefined;
    changes = undefined;
    changesError = "";
    directoryVersions = {};
    rootIncludeIgnored = false;
    includeIgnored = false;
    navIdentity = "";
    navStatus = "";
    query = "";
    directory = "";
    pending = false;
    error = "";
    nav = "files";
    treeRefresh++;
    void loadContext();
  }
  onMount(() => {
    mounted = true;
    // A restored workspace keeps the listings it was handed; checks report
    // what changed since. A listing still loading when the state was
    // captured is read now.
    if (!seed?.context) void loadContext();
    else if (!(seed.nav === "changes" ? seed.changes : seed.listing))
      void load();
    const timer = window.setInterval(() => void check(), 5000);
    const visibility = () => {
      if (document.hidden) checkController?.abort();
      else void check();
    };
    document.addEventListener("visibilitychange", visibility);
    return () => {
      mounted = false;
      contextController?.abort();
      controller?.abort();
      checkController?.abort();
      for (const c of controllers.values()) c.abort();
      clearInterval(timer);
      document.removeEventListener("visibilitychange", visibility);
    };
  });
  $effect(() => {
    const show = shown;
    const state = host.connection;
    if (mounted) {
      if (show && state === "connected") void check();
      else checkController?.abort();
    }
  });
  // Opens a requested path once the code context is known: the file's
  // working-tree diff when it has changes, the file itself otherwise.
  // A restored workspace was handed over after the current request was shown.
  let revealed = untrack(() => (seed ? (host.reveal?.key ?? 0) : 0));
  $effect(() => {
    const target = host.reveal;
    const current = context;
    if (!target || target.key === revealed) return;
    // Revealed paths resolve in the workspace root before the artifacts one.
    if (untrack(() => root) !== "workspace") {
      untrack(() => switchRoot("workspace"));
      return;
    }
    if (!current) return;
    revealed = target.key;
    untrack(() => void showReveal(target));
  });
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
  async function locateReveal(origin: CodeContext, target: CodeReveal) {
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
  async function showReveal(target: CodeReveal) {
    const current = context;
    if (!current) return;
    try {
      const { origin, location, usedRawPath } = await locateReveal(
        current,
        target,
      );
      if (location.kind !== "file") {
        if (origin.contextId === context?.contextId) await show(location);
        return;
      }
      const mode =
        target.mode ??
        (target.line !== undefined || target.heading ? "file" : "diff");
      if (mode === "diff" && origin.diffAvailable) {
        const found = await api(
          `${base}/changes?${params({ contextId: origin.contextId, nodeId: location.nodeId })}`,
          CodeChangesSchema,
        );
        const change =
          found.entries.find((entry) => entry.group !== "staged") ??
          found.entries[0];
        if (change) {
          await openDiff(change);
          return;
        }
      }
      const failure = await openFileInOrigin(
        origin,
        location.path,
        location.nodeId,
      );
      if (failure || mode !== "file") return;
      const tab = tabs.find(
        (t) => t.key === `${origin.contextId}:${location.nodeId}:worktree`,
      );
      if (!tab) return;
      if (!usedRawPath && target.line !== undefined) {
        tab.jump = {
          line: target.line,
          endLine: target.endLine,
          column: target.column,
          key: ++jumpSequence,
        };
      } else if (!usedRawPath && target.heading) {
        tab.heading = { slug: target.heading, key: ++headingSequence };
      } else {
        tab.jump = { key: ++jumpSequence };
      }
    } catch (e) {
      if (mounted) error = errorMessage(e);
    }
  }
</script>

<Tabs.Root value={active} onValueChange={activate}>
  {#snippet child({ props })}
    <section
      {...props}
      bind:this={section}
      class="code-panel relative flex h-full min-h-0 min-w-0 flex-col overflow-hidden bg-background"
      aria-label="代码工作区"
    >
      <div class="flex shrink-0 items-stretch border-b">
        <div class="flex shrink-0 items-center px-2 py-1">
          <Button
            variant="ghost"
            size="icon-sm"
            class="relative"
            aria-label={navOpen ? "收起导航" : "文件导航"}
            aria-expanded={navOpen}
            title={navOpen
              ? "收起导航"
              : navStatus
                ? `文件导航（${navStatus}）`
                : "文件导航"}
            onclick={() => (navOpen = !navOpen)}
            >{#if navOpen}<IconLayoutSidebarLeftCollapse
                aria-hidden="true"
              />{:else}<IconLayoutSidebarLeftExpand
                aria-hidden="true"
              />{/if}{#if navStatus && !navOpen}<span
                class="absolute top-0.5 right-0.5 size-1.5 rounded-full bg-warning"
                aria-hidden="true"
              ></span>{/if}</Button
          >
        </div>
        <Tabs.List variant="line" aria-label="文件标签">
          {#snippet child({ props })}
            <div
              {...props}
              class="flex min-w-0 flex-1 overflow-x-auto scrollbar-thin"
            >
              {#each tabs as tab (tab.key)}
                {@const path =
                  tab.file?.document.path ?? tab.diff?.new.path ?? ""}
                {@const meta = `${tab.rootKind === "artifacts" ? "临时文件夹" : tab.diff ? tab.diff.change.group : tab.file?.document.source}${tab.status !== "unchanged" ? " ●" : ""}`}
                {@const image = [
                  tab.file?.document.mediaType,
                  tab.diff?.new.mediaType,
                  tab.diff?.old.mediaType,
                ].some((type) => type?.startsWith("image/"))}
                <div class="flex shrink-0 items-center border-r first:border-l">
                  <Tabs.Trigger
                    value={tab.key}
                    aria-label={`${path} · ${meta}`}
                    title={tab.rootKind === "artifacts"
                      ? `临时文件夹 · ${path}`
                      : path}
                    onclick={beginOpen}
                    onkeydown={supersedeOpenOnTabKey}
                  >
                    {#snippet child({ props })}
                      <ListButton {...props} class={undefined} variant="tab"
                        >{#if image}<IconPhoto
                            class="mr-1 inline size-3.5 align-[-0.125em]"
                            aria-hidden="true"
                          />{/if}{path.slice(path.lastIndexOf("/") + 1)} · {meta}</ListButton
                      >
                    {/snippet}
                  </Tabs.Trigger>
                  <ListButton
                    variant="tab-close"
                    aria-label={`关闭 ${path}`}
                    title={`关闭 ${path}`}
                    onclick={() => close(tab.key)}
                    ><IconX aria-hidden="true" /></ListButton
                  >
                </div>{/each}
            </div>
          {/snippet}
        </Tabs.List>
        <div class="flex shrink-0 items-center gap-1 px-2 py-1">
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label="刷新全部"
            title={context?.rootPath
              ? `刷新全部（${context.rootPath}）`
              : "刷新全部"}
            disabled={pending}
            onclick={() => {
              void load();
              for (const tab of tabs) void refresh(tab);
            }}><IconRefresh aria-hidden="true" /></Button
          >
          {#if popout}<Button
              variant="ghost"
              size="icon-sm"
              class="max-sm:hidden"
              aria-label={popout.popped
                ? "收回代码窗口"
                : "在独立窗口中显示代码"}
              aria-pressed={popout.popped}
              title={popout.popped ? "收回代码窗口" : "在独立窗口中显示代码"}
              onclick={() => popout?.toggle()}
              >{#if popout.popped}<IconPictureInPictureOff
                  aria-hidden="true"
                />{:else}<IconPictureInPictureOn
                  aria-hidden="true"
                />{/if}</Button
            >{/if}
          {#if activeMenu}<DropdownMenu.Root>
              <DropdownMenu.Trigger>
                {#snippet child({ props })}<Button
                    {...props}
                    variant="ghost"
                    size="icon-sm"
                    class="sm:hidden"
                    aria-label="代码菜单"
                    title="代码菜单"><IconDots aria-hidden="true" /></Button
                  >{/snippet}
              </DropdownMenu.Trigger>
              <DropdownMenu.Content
                align="end"
                collisionPadding={safeAreaPadding()}
                class="w-72 max-w-[calc(100vw-1rem)]"
              >
                {@render activeMenu()}
              </DropdownMenu.Content>
            </DropdownMenu.Root>{/if}
        </div>
      </div>
      {#if error}<p
          role="alert"
          class="shrink-0 px-2 py-1 text-sm break-all text-destructive"
        >
          {error}
        </p>{/if}
      {#if context?.reason}<p
          class="shrink-0 px-2 py-1 text-xs text-muted-foreground"
        >
          {context.reason}
        </p>{/if}
      {#if dropStatus && !navOpen}<p
          role="status"
          class="shrink-0 px-2 py-1 text-xs text-muted-foreground"
        >
          {dropStatus}
        </p>{/if}
      <div
        class="code-inner flex min-h-0 flex-1 overflow-hidden"
        style:--code-nav-width={`${navWidth}px`}
        bind:this={inner}
      >
        <div class="contents" class:hidden={!navOpen}>
          <CodeNavigator
            bind:this={navigator}
            restore={seed?.navigator}
            dropHost={section}
            bind:dropStatus
            {id}
            {context}
            {listing}
            {rootListing}
            {fetchDirectory}
            {treeRefresh}
            {activePath}
            {changes}
            {changesError}
            {pending}
            sessionCwd={host.sessionCwd}
            reportedCwd={host.reportedCwd}
            {navStatus}
            {root}
            {rootError}
            {switchRoot}
            retryRoot={() => void loadContext()}
            reload={() => void load()}
            changed={filesChanged}
            changesEnabled={shown && !changesError && !navStatus}
            bind:nav
            bind:query
            bind:directory
            bind:includeIgnored
            bind:changeGroup
            bind:changeQuery
            bind:changeSearch
            load={(append) => load(append)}
            openFile={(path, opaqueId) => void openFile(path, opaqueId)}
            {locate}
            openDiff={(change) => void openDiff(change)}
          />
          <!-- A focusable separator with aria-valuenow is an interactive widget (WAI-ARIA window splitter). -->
          <!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions -->
          <div
            class="code-nav-divider"
            role="separator"
            aria-orientation="vertical"
            aria-label="调整导航宽度"
            aria-valuemin={navMin}
            aria-valuemax={navMax}
            aria-valuenow={navWidth}
            tabindex="0"
            onpointerdown={resizeStart}
            onkeydown={resizeKey}
          ></div>
        </div>
        <div class="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden">
          {#each tabs as tab (tab.key)}
            <Tabs.Content value={tab.key} tabindex={-1}>
              {#snippet child({ props })}
                <div {...props} class="min-h-0 flex-1 overflow-hidden">
                  <CodeDocumentView
                    bind:this={
                      () => views.get(tab),
                      (view) =>
                        view ? views.set(tab, view) : views.delete(tab)
                    }
                    restore={tab.restore}
                    {id}
                    active={shown && active === tab.key}
                    file={tab.file}
                    diff={tab.diff}
                    status={tab.status}
                    paste={(text) => host.paste(text)}
                    draftEnabled={host.draftEnabled}
                    refresh={() => void refresh(tab)}
                    contextId={tab.contextId}
                    comparison={tab.comparison}
                    onComparison={(value) => (tab.comparison = value)}
                    onState={(state) => (tab.status = state)}
                    {beginOpen}
                    openSide={(file, intent) =>
                      install(
                        tab,
                        file,
                        undefined,
                        intent === selectionSequence,
                      )}
                    bind:menu={tab.menu}
                    workspace={tab.contextId === context?.contextId
                      ? markdownWorkspace
                      : undefined}
                    heading={tab.heading}
                    onHeading={() => (tab.heading = undefined)}
                    jump={tab.jump}
                    onJump={() => (tab.jump = undefined)}
                    diffAvailable={tab.rootKind !== "artifacts"}
                  />
                </div>
              {/snippet}
            </Tabs.Content>
          {/each}
          {#if !tabs.length}<p class="p-4 text-sm text-muted-foreground">
              选择文件阅读完整内容，或从修改列表打开 Diff。
            </p>{/if}
        </div>
      </div>
    </section>
  {/snippet}
</Tabs.Root>
