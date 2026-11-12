<script lang="ts">
  import { untrack } from "svelte";
  import type { FileDiffMetadata, SelectedLineRange } from "@pierre/diffs";
  import type {
    CodeDiff,
    CodeDocumentViewState,
  } from "../../contracts/schemas";
  import type { MarkdownWorkspace } from "../../domains/code/markdown-link";
  import type { PreviewFormat } from "../../domains/code/code-preview";
  import { formatBytes, imageFormat } from "../../domains/code/code-image";
  import { Button } from "$lib/components/ui/button";
  import CodeDiffReader from "./CodeDiffReader.svelte";
  import CodeReader from "./CodeReader.svelte";
  import CodeImageView from "./CodeImageView.svelte";
  import CodeMarkdownPreview, {
    type MarkdownHeading,
  } from "./CodeMarkdownPreview.svelte";
  import CodeHtmlPreview from "./CodeHtmlPreview.svelte";
  import CodeTablePreview from "./CodeTablePreview.svelte";
  import CodeJsonPreview from "./CodeJsonPreview.svelte";
  import CodePdfView from "./CodePdfView.svelte";

  let {
    id,
    active = true,
    snapshotId,
    rawSide,
    doc,
    shownDiff,
    diffReady,
    calculation,
    calculationReason,
    fileDiff,
    modelSnapshot = $bindable(""),
    side,
    status,
    draftEnabled,
    hint = $bindable(""),
    imagePreview,
    imageDocument,
    pdfFile,
    formatPreview,
    formatFile,
    headings = $bindable<MarkdownHeading[]>([]),
    workspace,
    lastVersion,
    generation,
    anchor,
    restore,
    onselect,
    onselectdiff,
    onpreviewfailed,
    ondownloaderror,
    onshowformat,
  }: {
    id: string;
    active?: boolean;
    snapshotId: string;
    rawSide?: "old" | "new";
    doc: CodeDiff["old"] | undefined;
    shownDiff?: CodeDiff;
    diffReady: boolean;
    calculation: "pending" | "ready" | "limited" | "error";
    calculationReason: string;
    fileDiff?: FileDiffMetadata;
    modelSnapshot: string;
    side: "old" | "new";
    status: string;
    draftEnabled: boolean;
    hint: string;
    imagePreview: boolean;
    imageDocument: boolean;
    pdfFile: boolean;
    formatPreview: boolean;
    formatFile?: PreviewFormat;
    headings: MarkdownHeading[];
    workspace?: MarkdownWorkspace;
    lastVersion: string;
    generation: number;
    anchor: number;
    restore?: CodeDocumentViewState;
    onselect: (a: number, z: number) => void;
    onselectdiff: (range: SelectedLineRange | null) => void;
    onpreviewfailed: (message: string) => void;
    ondownloaderror: (message: string, code: number) => void;
    onshowformat: (view: "preview" | "source") => void;
  } = $props();

  const initial = untrack(() => restore);
  type Leaf = "reader" | "diff" | "markdown" | "image" | "pdf" | "json";
  const taken: Partial<Record<Leaf, true>> = {};
  function restoreFor<K extends Leaf>(
    key: K,
  ): CodeDocumentViewState[K] | undefined {
    return taken[key] ? undefined : initial?.[key];
  }

  let reader = $state<CodeReader>();
  let diffReader = $state<CodeDiffReader>();
  let imageView = $state<CodeImageView>();
  let pdfView = $state<CodePdfView>();
  let jsonView = $state<CodeJsonPreview>();
  let markdownPreview = $state<CodeMarkdownPreview>();

  $effect(() => {
    if (reader) taken.reader = true;
    if (diffReader) taken.diff = true;
    if (markdownPreview) taken.markdown = true;
    if (imageView) taken.image = true;
    if (pdfView) taken.pdf = true;
    if (jsonView) taken.json = true;
  });

  export function captureLeaves() {
    return {
      reader: reader?.capture() ?? restoreFor("reader"),
      diff: diffReader?.capture() ?? restoreFor("diff"),
      markdown: markdownPreview?.capture() ?? restoreFor("markdown"),
      image: imageView?.capture() ?? restoreFor("image"),
      pdf: pdfView?.capture() ?? restoreFor("pdf"),
      json: jsonView?.capture() ?? restoreFor("json"),
    };
  }

  export function line() {
    return reader?.line() ?? 1;
  }

  export function clearLineAction() {
    reader?.clearLineAction();
    diffReader?.clearLineAction();
  }

  export function scrollToHeading(slug: string) {
    return markdownPreview?.scrollToHeading(slug);
  }

  export function revealLine(line: number, endLine?: number, column?: number) {
    reader?.revealLine(line, endLine, column);
  }
</script>

{#if shownDiff?.kind === "text" && !diffReady}
  <p class="p-2 text-sm" role="status">
    {calculation === "pending" ? "正在计算差异…" : calculationReason}
  </p>
  {#if calculation === "error" || calculation === "limited"}
    <Button variant="outline" size="sm" onclick={() => (modelSnapshot = "")}
      >重试差异计算</Button
    >
  {/if}
{/if}
{#if shownDiff?.kind === "text" && diffReady && fileDiff}
  {#key modelSnapshot}
    <CodeDiffReader
      bind:this={diffReader}
      {fileDiff}
      {side}
      expired={status === "expired"}
      {draftEnabled}
      bind:hint
      onselect={onselectdiff}
      restore={restoreFor("diff")}
    />
  {/key}
{:else if imagePreview && doc}
  {#key `${snapshotId}:${rawSide ?? ""}`}<CodeImageView
      bind:this={imageView}
      {id}
      {snapshotId}
      side={rawSide}
      document={doc}
      onerror={onpreviewfailed}
      restore={restoreFor("image")}
    />{/key}
{:else if doc?.kind === "pdf" && pdfFile}
  {#key snapshotId}<CodePdfView
      bind:this={pdfView}
      {id}
      {snapshotId}
      document={doc}
      {active}
      {ondownloaderror}
      restore={restoreFor("pdf")}
    />{/key}
{:else if formatPreview && doc}
  {#if formatFile === "markdown"}
    <CodeMarkdownPreview
      bind:this={markdownPreview}
      bind:headings
      text={doc.text}
      path={doc.path}
      {active}
      {workspace}
      restore={restoreFor("markdown")}
    />
  {:else if formatFile === "html"}
    <CodeHtmlPreview text={doc.text} />
  {:else if formatFile === "csv" || formatFile === "tsv"}
    <CodeTablePreview
      text={doc.text}
      delimiter={formatFile === "csv" ? "," : "\t"}
      onsource={() => onshowformat("source")}
    />
  {:else if formatFile === "json"}
    <CodeJsonPreview
      bind:this={jsonView}
      text={doc.text}
      onsource={() => onshowformat("source")}
      restore={restoreFor("json")}
    />
  {/if}
{:else if doc?.kind === "text" && !shownDiff}
  {#key `${lastVersion}:${generation}`}<CodeReader
      bind:this={reader}
      text={doc.text}
      path={doc.path}
      restoreLine={anchor}
      {onselect}
      restore={restoreFor("reader")}
      {active}
      {draftEnabled}
      bind:hint
    />{/key}
{:else if imageDocument && doc}
  <div class="space-y-1 p-2 text-sm">
    {#if doc.source === "empty"}<p role="status">
        {side === "old" ? "旧侧" : "新侧"}不存在
      </p>
    {:else}<p aria-label="图片信息">
        {imageFormat(doc.mediaType)} · {formatBytes(doc.size)}
      </p>
      {#if doc.downloadable}<p class="text-muted-foreground">
          不加载预览，可下载原始文件查看。
        </p>{/if}
    {/if}
  </div>
{:else if pdfFile && doc}
  <div class="space-y-1 p-2 text-sm">
    <p aria-label="PDF 信息">PDF · {formatBytes(doc.size)}</p>
    {#if doc.downloadable}<p class="text-muted-foreground">
        不加载预览，可下载原始文件查看。
      </p>{/if}
  </div>
{/if}
