<script lang="ts">
  import { errorMessage } from "../../platform/error";
  import { untrack, onMount, tick, type Snippet } from "svelte";
  import { SvelteSet } from "svelte/reactivity";
  import type {
    CodeFile,
    CodeDiff,
    CodeDocumentViewState,
  } from "../../contracts/schemas";
  import {
    fetchFileComparisonDiff,
    fetchSnapshotSideFile,
    resolveCalculatedDiff,
  } from "../../domains/code/code-diff-client";
  import type { FileDiffMetadata, SelectedLineRange } from "@pierre/diffs";
  import CodeDocumentHeader from "./CodeDocumentHeader.svelte";
  import CodeDocumentBody from "./CodeDocumentBody.svelte";
  import CodeDocumentFooter from "./CodeDocumentFooter.svelte";
  import { codeReference } from "../../domains/code/code-reference";
  import type { MarkdownHeading } from "./CodeMarkdownPreview.svelte";
  import type { MarkdownWorkspace } from "../../domains/code/markdown-link";
  import { isImageType } from "../../domains/code/code-image";
  import { pdfMediaType, previewFormat } from "../../domains/code/code-preview";
  import type { TextInsertion } from "../../domains/terminal/terminal-draft.svelte";

  let {
    id,
    active = true,
    file,
    diff,
    status,
    paste,
    draftEnabled,
    refresh,
    openSide,
    beginOpen,
    contextId,
    comparison,
    onComparison,
    onState,
    menu = $bindable(),
    workspace,
    heading,
    onHeading,
    jump,
    onJump,
    diffAvailable = true,
    restore,
  }: {
    id: string;
    active?: boolean;
    file?: CodeFile;
    diff?: CodeDiff;
    status: string;
    paste: (text: string) => Promise<TextInsertion>;
    draftEnabled: boolean;
    refresh: () => void;
    openSide: (file: CodeFile, intent: number) => void;
    beginOpen: () => number;
    contextId: string;
    comparison?: CodeDiff;
    onComparison: (diff: CodeDiff) => void;
    onState: (state: string) => void;
    menu?: Snippet;
    /** Lets Markdown previews open workspace links and load relative images. */
    workspace?: MarkdownWorkspace;
    /** A one-time request from a link to preview and show a heading. */
    heading?: { slug: string; key: number };
    onHeading?: () => void;
    /** A one-time request from a link to show the file and optional line range. */
    jump?: { line?: number; endLine?: number; column?: number; key: number };
    onJump?: () => void;
    /** False for roots without Git (the artifacts folder): no Diff toggle. */
    diffAvailable?: boolean;
    /** Reading state from another window, applied once. */
    restore?: CodeDocumentViewState;
  } = $props();

  const initial = untrack(() => restore);
  let mode = $state<"file" | "diff">(initial?.mode ?? "file");
  let shownDiff = $derived(diff ?? (mode === "diff" ? comparison : undefined));
  let mounted = false;
  const requests = new SvelteSet<AbortController>();
  onMount(() => {
    mounted = true;
    return () => {
      mounted = false;
      for (const c of requests) c.abort();
    };
  });
  // Renderer metadata crosses the highlighting Worker boundary; keep it plain.
  let fileDiff = $state.raw<FileDiffMetadata>();
  let modelSnapshot = $state("");
  let calculation = $state<"pending" | "ready" | "limited" | "error">(
    "pending",
  );
  let calculationReason = $state("");
  const diffReady = $derived(
    modelSnapshot === shownDiff?.snapshotId && calculation === "ready",
  );
  let side = $state<"old" | "new">(initial?.side ?? "new");
  let start = $state<number | undefined>(initial?.start);
  let end = $state<number | undefined>(initial?.end);
  let invalidSelection = $state(false);
  let error = $state("");
  let busy = $state(false);
  let generation = $state(0);
  let body = $state<CodeDocumentBody>();
  let headings = $state<MarkdownHeading[]>([]);
  let anchor = $state(1);
  // A restored view already shows its version; the first run resets nothing.
  let lastVersion = $state(
    initial ? untrack(() => file?.snapshotId ?? diff?.snapshotId ?? "") : "",
  );
  let keepRestoredSide = !!initial;

  export function capture(): CodeDocumentViewState {
    return {
      mode,
      side,
      start,
      end,
      svgView,
      formatView,
      ...body?.captureLeaves(),
    };
  }
  // Selection guidance from the active reader, shown beside the actions.
  let hint = $state("");
  let doc = $derived(
    shownDiff
      ? side === "old"
        ? shownDiff.old
        : shownDiff.new
      : file?.document,
  );
  const defaultSide = $derived<"old" | "new">(
    shownDiff?.new.source === "empty" || file?.document.source === "head"
      ? "old"
      : "new",
  );
  const snapshotId = $derived(shownDiff?.snapshotId ?? file?.snapshotId ?? "");
  // Diff snapshots need the side; file snapshots ignore it.
  const rawSide = $derived(shownDiff ? side : undefined);
  // SVG is a text document: preview by default, source on request or failure.
  let svgView = $state<"preview" | "source">(initial?.svgView ?? "preview");
  let svgNotice = $state("");
  const svgFile = $derived(
    !shownDiff && doc?.kind === "text" && doc.mediaType === "image/svg+xml",
  );
  // Markdown, HTML, CSV/TSV and JSON stay in source mode until the reader
  // asks for a preview; the choice resets with the document.
  let formatView = $state<"source" | "preview">(
    initial?.formatView ?? "source",
  );
  const formatFile = $derived(
    !shownDiff && doc?.kind === "text" ? previewFormat(doc.path) : undefined,
  );
  const formatPreview = $derived(formatView === "preview" && !!formatFile);
  const pdfFile = $derived(
    !shownDiff && !!doc && doc.mediaType === pdfMediaType,
  );
  const imagePreview = $derived(
    !!doc &&
      doc.source !== "empty" &&
      (shownDiff
        ? shownDiff.kind === "image" &&
          (doc.kind === "image" ||
            (doc.kind === "text" && doc.mediaType === "image/svg+xml"))
        : doc.kind === "image" || (svgFile && svgView === "preview")),
  );
  // Image documents, over-limit images and either side of an image Diff.
  const imageDocument = $derived(
    !!doc &&
      (shownDiff ? shownDiff.kind === "image" : isImageType(doc.mediaType)),
  );
  let preview = $derived.by(() => {
    if (
      invalidSelection ||
      !doc ||
      !doc.referenceable ||
      doc.source === "empty"
    )
      return "";
    try {
      return codeReference(doc, start, end);
    } catch {
      return "";
    }
  });
  $effect(() => {
    const version = file?.snapshotId ?? diff?.snapshotId ?? "";
    untrack(() => {
      if (version !== lastVersion) {
        anchor = body?.line() ?? 1;
        lastVersion = version;
        mode = "file";
        start = undefined;
        end = undefined;
        invalidSelection = false;
        error = "";
        svgNotice = "";
        formatView = "source";
        generation++;
      }
    });
  });
  $effect(() => {
    const current = shownDiff;
    untrack(() => {
      if (keepRestoredSide) {
        keepRestoredSide = false;
        return;
      }
      anchor = body?.line() ?? 1;
      generation++;
      start = undefined;
      end = undefined;
      invalidSelection = false;
      formatView = "source";
      if (current?.kind !== "image") side = defaultSide;
      // A deleted or added image has nothing to preview on one side.
      if (
        current?.kind === "image" &&
        (side === "new" ? current.new : current.old).source === "empty"
      )
        side = side === "new" ? "old" : "new";
    });
  });
  $effect(() => {
    const request = heading;
    if (!request) return;
    untrack(() => {
      onHeading?.();
      const leaving = !!shownDiff && !diff;
      mode = "file";
      const current = file?.document;
      if (!current || current.kind !== "text") return;
      if (previewFormat(current.path) !== "markdown") return;
      const reveal = () => {
        showFormat("preview");
        void tick().then(() => body?.scrollToHeading(request.slug));
      };
      if (leaving) void tick().then(reveal);
      else reveal();
    });
  });
  $effect(() => {
    const request = jump;
    if (!request) return;
    untrack(() => {
      onJump?.();
      mode = "file";
      if (request.line === undefined) return;
      showFormat("source");
      const current = file?.document;
      if (current?.kind === "text" && current.mediaType === "image/svg+xml")
        showSvg("source");
      void tick().then(() =>
        body?.revealLine(request.line!, request.endLine, request.column),
      );
    });
  });
  $effect(() => {
    const current = shownDiff;
    if (!active || current?.kind !== "text") return;
    if (modelSnapshot === current.snapshotId) return;
    const controller = new AbortController();
    untrack(() => {
      calculation = "pending";
      calculationReason = "";
      fileDiff = undefined;
    });
    void resolveCalculatedDiff(current, controller.signal).then((state) => {
      if (
        controller.signal.aborted ||
        shownDiff?.snapshotId !== current.snapshotId
      )
        return;
      modelSnapshot = state.modelSnapshot;
      calculation = state.calculation;
      calculationReason = state.calculationReason;
      if (state.fileDiff) {
        fileDiff = state.fileDiff;
        generation++;
      }
    });
    return () => controller.abort();
  });
  async function compare() {
    if (!file) return;
    if (comparison) {
      mode = "diff";
      return;
    }
    const controller = new AbortController();
    requests.add(controller);
    busy = true;
    try {
      const value = await fetchFileComparisonDiff(
        id,
        contextId,
        file,
        controller.signal,
        onState,
      );
      if (!mounted || controller.signal.aborted) return;
      onComparison(value);
      side = file.document.source === "head" ? "old" : "new";
      mode = "diff";
    } catch (e) {
      if (mounted && !controller.signal.aborted) error = errorMessage(e);
    } finally {
      requests.delete(controller);
      busy = false;
    }
  }
  function select(a: number, z: number) {
    error = "";
    invalidSelection = false;
    start = a;
    end = z;
  }
  function selectDiff(range: SelectedLineRange | null) {
    error = "";
    start = undefined;
    end = undefined;
    invalidSelection = false;
    if (!range) {
      side = defaultSide;
      return;
    }
    if (range.endSide && range.endSide !== range.side) {
      invalidSelection = true;
      error = "请选择同一来源的连续行";
      return;
    }
    side = range.side === "deletions" ? "old" : "new";
    start = Math.min(range.start, range.end);
    end = Math.max(range.start, range.end);
  }
  async function send(reference = preview) {
    if (!reference || status !== "unchanged") return;
    error = "";
    const result = await paste(reference);
    if (mounted && result === "failed")
      error = "输入目标未就绪，位置未插入；请重试";
  }
  function clearSelection() {
    start = undefined;
    end = undefined;
    invalidSelection = false;
    error = "";
    body?.clearLineAction();
  }
  function showSvg(view: "preview" | "source") {
    if (view === svgView) return;
    clearSelection();
    svgNotice = "";
    svgView = view;
  }
  function showFormat(view: "preview" | "source") {
    if (view === formatView) return;
    clearSelection();
    formatView = view;
  }
  function previewFailed(message: string) {
    if (!svgFile || svgView !== "preview") return;
    clearSelection();
    svgView = "source";
    svgNotice = `SVG 预览不可用，已切换到源码：${message}`;
  }
  function downloadFailed(message: string, code: number) {
    error = message;
    if (code === 410) onState("expired");
    else if (code === 409) onState("updated");
  }
  async function full(which: "old" | "new") {
    if (!shownDiff) return;
    const sid = shownDiff.snapshotId;
    const intent = beginOpen();
    const controller = new AbortController();
    requests.add(controller);
    try {
      const value = await fetchSnapshotSideFile(
        id,
        sid,
        which,
        controller.signal,
        onState,
      );
      if (
        mounted &&
        !controller.signal.aborted &&
        shownDiff?.snapshotId === sid
      )
        openSide(value, intent);
    } catch (e) {
      if (mounted && !controller.signal.aborted) error = errorMessage(e);
    } finally {
      requests.delete(controller);
    }
  }
</script>

<div class="flex h-full min-h-0 flex-col overflow-hidden">
  <CodeDocumentHeader
    {doc}
    {file}
    {diffAvailable}
    {busy}
    {shownDiff}
    bind:mode
    bind:side
    {formatFile}
    {formatPreview}
    {headings}
    {svgFile}
    {svgView}
    {svgNotice}
    {status}
    bind:menu
    oncompare={() => void compare()}
    onshowformat={showFormat}
    onshowsvg={showSvg}
    onscrolltoheading={(slug) => body?.scrollToHeading(slug)}
    onclearselection={clearSelection}
    onfull={(which) => void full(which)}
    onrefresh={refresh}
  />
  <CodeDocumentBody
    bind:this={body}
    {id}
    {active}
    {snapshotId}
    {rawSide}
    {doc}
    {shownDiff}
    {diffReady}
    {calculation}
    {calculationReason}
    {fileDiff}
    bind:modelSnapshot
    {side}
    {status}
    {draftEnabled}
    bind:hint
    {imagePreview}
    {imageDocument}
    {pdfFile}
    {formatPreview}
    {formatFile}
    bind:headings
    {workspace}
    {lastVersion}
    {generation}
    {anchor}
    restore={initial}
    onselect={select}
    onselectdiff={selectDiff}
    onpreviewfailed={previewFailed}
    ondownloaderror={downloadFailed}
    onshowformat={showFormat}
  />
  <CodeDocumentFooter
    {id}
    {snapshotId}
    {rawSide}
    {doc}
    {shownDiff}
    {side}
    {hint}
    {start}
    {end}
    {invalidSelection}
    {preview}
    {status}
    {draftEnabled}
    {imageDocument}
    {pdfFile}
    {error}
    onresetselection={() => {
      side = defaultSide;
      clearSelection();
    }}
    onsend={() => void send()}
    ondownloaderror={downloadFailed}
  />
</div>
