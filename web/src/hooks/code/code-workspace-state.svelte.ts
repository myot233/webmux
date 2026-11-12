import { untrack } from "svelte";
import { SvelteMap } from "svelte/reactivity";
import type { z } from "zod";
import { api, ensureArtifactsFolder } from "../../platform/api";
import { nodeId } from "../../domains/code/code-reference";
import {
  aborted,
  applyWorkspaceStalenessResult,
  fetchNavigatorListingPage,
  fetchRefreshedTabContent,
  runWorkspaceStalenessCheck,
  upsertWorkspaceTab,
  type CodeContext,
  type CodeTabOrigin,
  type CodeWorkspaceTab,
} from "../../domains/code/code-workspace";
import { errorMessage } from "../../platform/error";
import {
  CodeChangesSchema,
  CodeContextSchema,
  CodeListingSchema,
  type CodeDiff,
  type CodeFile,
  type CodeListing,
  type CodeWorkspaceSnapshot,
} from "../../contracts/schemas";

export function useCodeWorkspaceState(options: {
  id: () => string;
  base: () => string;
  shown: () => boolean;
  seed?: CodeWorkspaceSnapshot;
  closeOverlayNav: () => void;
}) {
  const seed = options.seed;
  const s = $state({
    context: seed?.context as CodeContext | undefined,
    root: (seed?.root ?? "workspace") as "workspace" | "artifacts",
    rootError: "",
    listing: seed?.listing as CodeListing | undefined,
    rootListing: seed?.rootListing as CodeListing | undefined,
    treeRefresh: 0,
    changeQuery: seed?.changeQuery ?? "",
    changeSearch: seed?.changeSearch ?? "",
    changeGroup: seed?.changeGroup ?? "",
    changesError: "",
    changes: seed?.changes as z.infer<typeof CodeChangesSchema> | undefined,
    tabs: (seed?.tabs ?? []).map((tab) => ({
      key: tab.key,
      contextId: tab.contextId,
      rootKind: tab.rootKind,
      file: tab.file,
      diff: tab.diff,
      comparison: tab.comparison,
      status: tab.status,
      sequence: 0,
      restore: tab.view,
    })) as CodeWorkspaceTab[],
    active: seed?.active ?? "",
    nav: (seed?.nav === "changes" ? "changes" : "files") as "files" | "changes",
    navOpen: seed?.navOpen ?? true,
    navWidth: seed?.navWidth ?? 260,
    includeIgnored: seed?.includeIgnored ?? false,
    directory: seed?.directory ?? "",
    query: seed?.query ?? "",
    error: "",
    navStatus: seed?.navStatus ?? "",
    pending: false,
  });
  let contextController: AbortController | undefined;
  let rootIncludeIgnored = seed?.rootIncludeIgnored ?? false;
  let directoryVersions: Record<string, string> = {
    ...seed?.directoryVersions,
  };
  let controller: AbortController | undefined;
  const controllers = new SvelteMap<string, AbortController>();
  let navSequence = 0;
  let mounted = false;
  let checkController: AbortController | undefined;
  let checking = false;
  let navIdentity = seed?.navIdentity ?? "";
  let selectionSequence = 0;
  const beginOpen = () => ++selectionSequence;

  function params(extra: Record<string, string> = {}) {
    return new URLSearchParams({
      contextId: s.context?.contextId ?? "",
      includeIgnored: String(s.includeIgnored),
      ...extra,
    });
  }

  function activate(key: string) {
    beginOpen();
    s.active = key;
  }

  function supersedeOpenOnTabKey(event: KeyboardEvent) {
    const keys = ["Enter", " ", "ArrowLeft", "ArrowRight", "Home", "End"];
    if (keys.includes(event.key)) beginOpen();
  }

  function install(
    origin: CodeTabOrigin,
    file?: CodeFile,
    diff?: CodeDiff,
    shouldActivate = true,
  ) {
    const outcome = upsertWorkspaceTab(s.tabs, origin, file, diff);
    if (!outcome) return;
    if (outcome.error) {
      s.error = outcome.error;
      return;
    }
    if (shouldActivate) {
      s.active = outcome.key;
      if (outcome.added) options.closeOverlayNav();
    }
  }

  async function load(append = false) {
    if (!s.context || (append && s.pending)) return;
    controller?.abort();
    controller = new AbortController();
    const request = controller;
    const seq = ++navSequence;
    const requestQuery = s.query;
    const requestIgnored = s.includeIgnored;
    s.pending = true;
    s.error = "";
    try {
      if (s.nav === "changes") {
        if (!s.context.diffAvailable) {
          s.changes = undefined;
          return;
        }
        s.changesError = "";
        const cursor = append ? (s.changes?.nextCursor ?? "") : "";
        const value = await api(
          `${options.base()}/changes?${params({ cursor, q: s.changeQuery, group: s.changeGroup })}`,
          CodeChangesSchema,
          "GET",
          undefined,
          request.signal,
        );
        if (mounted && seq === navSequence) {
          s.changes =
            append && s.changes
              ? { ...value, entries: [...s.changes.entries, ...value.entries] }
              : value;
        }
      } else {
        await fetchNavigatorListingPage({
          base: options.base(),
          params,
          requestQuery,
          requestIgnored,
          rootIncludeIgnored,
          listing: s.listing,
          rootListing: s.rootListing,
          append,
          signal: request.signal,
          isCurrent: () => mounted && seq === navSequence,
          onSuccess: (next, identity) => {
            s.listing = next;
            navIdentity = identity;
            if (!requestQuery) {
              s.rootListing = s.listing;
              rootIncludeIgnored = requestIgnored;
              if (!append) {
                directoryVersions = {};
                s.treeRefresh++;
              }
            }
          },
          onResetRoot: (rootVal) => {
            s.rootListing = rootVal;
            rootIncludeIgnored = requestIgnored;
            directoryVersions = {};
            s.treeRefresh++;
          },
        });
      }
      if (seq === navSequence) s.navStatus = "";
    } catch (e) {
      if (!aborted(e) && mounted && seq === navSequence) {
        s.error = errorMessage(e);
        if (s.nav === "changes") s.changesError = s.error;
      }
    } finally {
      if (mounted && seq === navSequence) s.pending = false;
    }
  }

  async function fetchDirectory(
    path: string,
    cursor: string,
    signal: AbortSignal,
  ) {
    const opaqueId = nodeId(path);
    const value = await api(
      `${options.base()}/tree?${params({ nodeId: opaqueId, cursor })}`,
      CodeListingSchema,
      "GET",
      undefined,
      signal,
    );
    if (!signal.aborted) directoryVersions[opaqueId] = value.contentVersion;
    return value;
  }

  async function read<T>(
    key: string,
    request: () => Promise<T>,
    done: (value: T) => void,
  ) {
    controllers.get(key)?.abort();
    const c = new AbortController();
    controllers.set(key, c);
    s.error = "";
    try {
      const value = await request();
      if (mounted && !c.signal.aborted && controllers.get(key) === c)
        done(value);
      return "";
    } catch (e) {
      if (!mounted || aborted(e)) return "";
      s.error = errorMessage(e);
      return s.error;
    } finally {
      if (controllers.get(key) === c) controllers.delete(key);
    }
  }

  async function openTab<T extends z.ZodType>(
    key: string,
    path: string,
    schema: T,
    done: (value: z.infer<T>, activate: boolean) => void,
  ) {
    const intent = beginOpen();
    if (s.tabs.some((t) => t.key === key)) {
      s.active = key;
      options.closeOverlayNav();
      void check();
      return "";
    }
    return read(
      key,
      () => api(path, schema, "GET", undefined, controllers.get(key)?.signal),
      (value) => done(value, intent === selectionSequence),
    );
  }

  function close(key: string) {
    controllers.get(key)?.abort();
    controllers.delete(key);
    s.tabs = s.tabs.filter((t) => t.key !== key);
    if (s.active === key) activate(s.tabs.at(-1)?.key ?? "");
  }

  function filesChanged(removed: string[]) {
    const current = s.context?.contextId;
    for (const tab of s.tabs.filter((t) => {
      const path = t.file?.document.path ?? t.diff?.new.path ?? "";
      return (
        t.contextId === current &&
        removed.some((p) => path === p || path.startsWith(p + "/"))
      );
    }))
      close(tab.key);
    void load();
  }

  async function refresh(tab: CodeWorkspaceTab) {
    const seq = ++tab.sequence;
    await read(
      tab.key,
      () =>
        fetchRefreshedTabContent(
          options.base(),
          params,
          tab,
          controllers.get(tab.key)?.signal,
        ),
      (value) => {
        if (!s.tabs.includes(tab) || tab.sequence !== seq) return;
        tab.file = value.file;
        tab.diff = value.diff;
        tab.status = "unchanged";
        tab.comparison = undefined;
      },
    );
  }

  async function check() {
    const current = s.context;
    if (!mounted || !options.shown() || document.hidden || !current || checking)
      return;
    checking = true;
    checkController = new AbortController();
    const c = checkController;
    const listingVersion = s.rootListing?.contentVersion;
    const branchVersions = { ...directoryVersions };
    const changesVersion = s.changes?.contentVersion;
    const identity = navIdentity;
    const treeGeneration = s.treeRefresh;
    const checkIgnored = s.includeIgnored;
    try {
      const { documents, result } = await runWorkspaceStalenessCheck({
        base: options.base(),
        current,
        tabs: s.tabs,
        listingVersion,
        branchVersions,
        changesVersion,
        identity,
        nav: s.nav,
        query: s.query,
        includeIgnored: s.includeIgnored,
        signal: c.signal,
      });
      if (!mounted || c.signal.aborted) return;
      const nextStatus = applyWorkspaceStalenessResult({
        documents,
        result,
        tabs: s.tabs,
        current,
        activeContext: s.context,
        treeGeneration,
        treeRefresh: s.treeRefresh,
        checkIgnored,
        includeIgnored: s.includeIgnored,
        identity,
        navIdentity,
        listingVersion,
        rootListingVersion: s.rootListing?.contentVersion,
        branchVersions,
        directoryVersions,
        changesVersion,
        activeChangesVersion: s.changes?.contentVersion,
      });
      if (nextStatus) s.navStatus = nextStatus;
    } catch (e) {
      if (!aborted(e) && mounted) {
        for (const tab of s.tabs) tab.status = "unknown";
        if (current === s.context) s.navStatus = "检测失败，状态未知";
      }
    } finally {
      checking = false;
    }
  }

  async function loadContext() {
    contextController?.abort();
    const request = new AbortController();
    contextController = request;
    const kind = s.root;
    s.rootError = "";
    try {
      if (kind === "artifacts")
        await ensureArtifactsFolder(options.id(), request.signal);
      const value = await api(
        `${options.base()}/context${kind === "artifacts" ? "?root=artifacts" : ""}`,
        CodeContextSchema,
        "GET",
        undefined,
        request.signal,
      );
      if (!mounted || request.signal.aborted) return;
      s.context = value;
      void load();
    } catch (e) {
      if (!mounted || aborted(e) || request.signal.aborted) return;
      if (kind === "artifacts") s.rootError = errorMessage(e);
      else s.error = errorMessage(e);
    }
  }

  function switchRoot(kind: "workspace" | "artifacts") {
    if (kind === s.root) return;
    controller?.abort();
    checkController?.abort();
    navSequence++;
    directoryVersions = {};
    rootIncludeIgnored = false;
    navIdentity = "";
    Object.assign(s, {
      root: kind,
      context: undefined,
      listing: undefined,
      rootListing: undefined,
      changes: undefined,
      changesError: "",
      includeIgnored: false,
      navStatus: "",
      query: "",
      directory: "",
      pending: false,
      error: "",
      nav: "files",
      treeRefresh: s.treeRefresh + 1,
    });
    void loadContext();
  }

  function mountWorkspace() {
    mounted = true;
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
  }

  function syncConnectionCheck(connection: () => string) {
    $effect(() => {
      const show = options.shown();
      const state = connection();
      if (mounted) {
        if (show && state === "connected") void check();
        else checkController?.abort();
      }
    });
  }

  return {
    s,
    get mounted() {
      return mounted;
    },
    get selectionSequence() {
      return selectionSequence;
    },
    params,
    beginOpen,
    activate,
    supersedeOpenOnTabKey,
    install,
    load,
    fetchDirectory,
    openTab,
    close,
    filesChanged,
    refresh,
    loadContext,
    switchRoot,
    mountWorkspace,
    syncConnectionCheck,
    snapshotState: (
      captureTabView: (
        tab: CodeWorkspaceTab,
      ) => CodeWorkspaceSnapshot["tabs"][number]["view"],
      captureNavigator: () => CodeWorkspaceSnapshot["navigator"],
    ): CodeWorkspaceSnapshot =>
      untrack(() => ({
        version: 1,
        ...$state.snapshot(s),
        rootIncludeIgnored,
        directoryVersions: { ...directoryVersions },
        navIdentity,
        tabs: s.tabs.map((tab) => ({
          key: tab.key,
          contextId: tab.contextId,
          rootKind: tab.rootKind,
          file: $state.snapshot(tab.file),
          diff: $state.snapshot(tab.diff),
          comparison: $state.snapshot(tab.comparison),
          status: tab.status,
          view: captureTabView(tab),
        })),
        navigator: captureNavigator(),
      })),
  };
}
