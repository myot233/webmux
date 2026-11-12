<script lang="ts">
  import { Button } from "$lib/components/ui/button";
  import CodeDownloadButton from "./CodeDownloadButton.svelte";
  import { basename, codeRawPath } from "../../domains/code/code-image";
  import { sourceLabel } from "../../domains/code/code-diff-client";
  import type { CodeDiff, CodeFile } from "../../contracts/schemas";

  let {
    id,
    snapshotId,
    rawSide,
    doc,
    shownDiff,
    side,
    hint,
    start,
    end,
    invalidSelection,
    preview,
    status,
    draftEnabled,
    imageDocument,
    pdfFile,
    error,
    onresetselection,
    onsend,
    ondownloaderror,
  }: {
    id: string;
    snapshotId: string;
    rawSide?: "old" | "new";
    doc: CodeFile["document"] | CodeDiff["old"] | undefined;
    shownDiff?: CodeDiff;
    side: "old" | "new";
    hint: string;
    start?: number;
    end?: number;
    invalidSelection: boolean;
    preview: string;
    status: string;
    draftEnabled: boolean;
    imageDocument: boolean;
    pdfFile: boolean;
    error: string;
    onresetselection: () => void;
    onsend: () => void;
    ondownloaderror: (message: string, code: number) => void;
  } = $props();
</script>

<div class="mt-auto shrink-0 space-y-1 border-t p-2">
  {#if error}<p class="text-sm text-destructive" role="alert">{error}</p>{/if}
  <div class="flex flex-wrap items-center gap-2">
    <!-- Zero basis: the text truncates instead of wrapping the buttons. -->
    <p class="min-w-0 flex-1 basis-0 truncate text-xs text-muted-foreground">
      {#if shownDiff}{side === "old" ? "修改前" : "修改后"} · {sourceLabel(
          doc?.source ?? "",
        )}{/if}{#if hint}<span class="max-sm:hidden"
          >{shownDiff ? " · " : ""}<span>{hint}</span></span
        >{/if}
    </p>
    {#if start !== undefined || invalidSelection}<Button
        variant="outline"
        size="sm"
        onclick={onresetselection}>引用整个文件</Button
      >{/if}
    <Button
      size="sm"
      disabled={!preview || status !== "unchanged"}
      onclick={onsend}
      >发送位置到{draftEnabled
        ? "草稿"
        : "终端"}{#if start !== undefined}（{start}{end !== undefined &&
        end !== start
          ? `-${end}`
          : ""} 行）{/if}</Button
    >
    {#if doc && ((imageDocument && doc.source !== "empty") || (pdfFile && doc.kind !== "pdf"))}<CodeDownloadButton
        path={codeRawPath(id, snapshotId, rawSide)}
        filename={basename(doc.path)}
        disabled={!doc.downloadable}
        onerror={ondownloaderror}
      />{#if !doc.downloadable}<p
          class="self-center text-xs text-muted-foreground"
        >
          超过 100 MiB 下载上限，不可下载
        </p>{/if}{/if}
  </div>
</div>
