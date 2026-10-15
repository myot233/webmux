<script lang="ts">
  import { untrack, onMount } from "svelte";
  import { api, APIError } from "../api";
  import {
    CodeFileSchema,
    CodeChangesSchema,
    CodeDiffSchema,
    type CodeFile,
    type CodeDiff,
    type CodeDocument,
  } from "../schemas";
  import { calculateDiff } from "../code-diff-client";
  import {
    collapseDiff,
    expandDiff,
    type CodeLine,
    type CodeBlock,
  } from "../code-diff-model";
  import { codeReference, nodeId } from "../code-reference";
  import CodeReader from "./CodeReader.svelte";
  import { Button } from "$lib/components/ui/button";
  let {
    id,
    active = true,
    file,
    diff,
    status,
    paste,
    refresh,
    openSide,
    beginOpen,
    contextId,
    comparison,
    onComparison,
    onState,
  }: {
    id: string;
    active?: boolean;
    file?: CodeFile;
    diff?: CodeDiff;
    status: string;
    paste: (text: string) => boolean;
    refresh: () => void;
    openSide: (file: CodeFile, intent: number) => void;
    beginOpen: () => number;
    contextId: string;
    comparison?: CodeDiff;
    onComparison: (diff: CodeDiff) => void;
    onState: (state: string) => void;
  } = $props();
  let mode = $state<"file" | "diff">("file");
  let shownDiff = $derived(diff ?? (mode === "diff" ? comparison : undefined));
  let mounted = false;
  const requests = new Set<AbortController>();
  onMount(() => {
    mounted = true;
    return () => {
      mounted = false;
      for (const c of requests) c.abort();
    };
  });
  let blocks = $state<CodeBlock[]>([]);
  let allLines = $state<CodeLine[]>([]);
  let modelSnapshot = $state("");
  let calculation = $state<"pending" | "ready" | "limited" | "error">(
    "pending",
  );
  let calculationReason = $state("");
  const diffReady = $derived(
    modelSnapshot === shownDiff?.snapshotId && calculation === "ready",
  );
  let side = $state<"old" | "new">("new");
  let start = $state<number>();
  let end = $state<number>();
  let error = $state("");
  let message = $state("");
  let busy = $state(false);
  let generation = $state(0);
  let reader = $state<CodeReader>();
  let anchor = $state(1);
  let lastVersion = $state("");
  let doc = $derived(
    shownDiff
      ? side === "old"
        ? shownDiff.old
        : shownDiff.new
      : file?.document,
  );
  let mapping = $derived(
    shownDiff
      ? blocks.flatMap((b) => (b.kind === "gap" ? [null] : b.lines))
      : undefined,
  );
  let text = $derived(
    shownDiff
      ? (mapping ?? [])
          .map((l) =>
            l
              ? `${l.kind === "add" ? "+" : l.kind === "delete" ? "-" : " "}${l.text}`
              : "⋯ 未展开上下文 ⋯",
          )
          .join("\n")
      : (doc?.text ?? ""),
  );
  let preview = $derived.by(() => {
    if (!doc || !doc.referenceable || doc.source === "empty") return "";
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
        anchor = reader?.line() ?? 1;
        lastVersion = version;
        mode = "file";
        start = undefined;
        end = undefined;
        error = "";
        message = "";
        generation++;
      }
    });
  });
  $effect(() => {
    const current = shownDiff;
    untrack(() => {
      anchor = reader?.line() ?? 1;
      generation++;
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
      allLines = [];
      blocks = [];
    });
    void calculateDiff(
      current.snapshotId,
      current.old.text,
      current.new.text,
      controller.signal,
    )
      .then((result) => {
        if (
          controller.signal.aborted ||
          shownDiff?.snapshotId !== current.snapshotId
        )
          return;
        modelSnapshot = current.snapshotId;
        calculation = result.kind;
        if (result.kind === "ready") {
          allLines = result.lines;
          blocks = collapseDiff(result.lines);
          generation++;
        } else calculationReason = result.reason;
      })
      .catch((error: unknown) => {
        if (
          controller.signal.aborted ||
          shownDiff?.snapshotId !== current.snapshotId
        )
          return;
        modelSnapshot = current.snapshotId;
        calculation = "error";
        calculationReason =
          error instanceof Error
            ? error.message
            : "差异计算失败，请查看两侧完整文件";
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
      const params = new URLSearchParams({
        contextId,
        nodeId: file.document.nodeId ?? nodeId(file.document.path),
      });
      const list = await api(
        `/sessions/${id}/code/changes?${params}`,
        CodeChangesSchema,
        "GET",
        undefined,
        controller.signal,
      );
      const group = file.document.source !== "worktree" ? "staged" : "unstaged";
      const change = list.entries.find(
        (c) => c.group === group || c.group === "untracked",
      );
      if (!change) throw new Error("此文件没有可用的未提交差异，保留完整阅读");
      const value = await api(
        `/sessions/${id}/code/diff?${new URLSearchParams({ contextId, changeId: change.changeId })}`,
        CodeDiffSchema,
        "GET",
        undefined,
        controller.signal,
      );
      if (!mounted || controller.signal.aborted) return;
      const candidate = file.document.source === "head" ? value.old : value.new;
      if (candidate.contentVersion !== file.document.contentVersion) {
        onState("updated");
        throw new Error("比较来源已更新，请刷新完整文件后再切换 Diff");
      }
      onComparison(value);
      side = file.document.source === "head" ? "old" : "new";
      mode = "diff";
    } catch (e) {
      if (mounted && !controller.signal.aborted) error = (e as Error).message;
    } finally {
      requests.delete(controller);
      busy = false;
    }
  }
  function select(a: number, z: number) {
    error = "";
    message = "";
    if (mapping) {
      const chosen = mapping.slice(a - 1, z);
      if (chosen.some((l) => !l)) {
        error = "请先展开范围内的上下文";
        return;
      }
      const numbers = chosen
        .map((l) => (side === "old" ? l!.oldLine : l!.newLine))
        .filter((n) => n > 0);
      if (!numbers.length) {
        error = "所选行在此来源没有行号";
        return;
      }
      if (numbers.some((n, i) => i > 0 && n !== numbers[i - 1] + 1)) {
        error = "所选行不是连续范围";
        return;
      }
      start = numbers[0];
      end = numbers.at(-1);
    } else {
      start = a;
      end = z;
    }
  }
  function expand(gapId: string, direction: "up" | "down" | "all") {
    if (!diffReady || status === "expired") return;
    anchor = reader?.line() ?? 1;
    blocks = expandDiff(allLines, blocks, gapId, direction);
    generation++;
  }
  async function full(which: "old" | "new") {
    if (!shownDiff) return;
    const sid = shownDiff.snapshotId;
    const intent = beginOpen();
    const controller = new AbortController();
    requests.add(controller);
    try {
      const value = await api(
        `/sessions/${id}/code/snapshots/${sid}/file?side=${which}`,
        CodeFileSchema,
        "GET",
        undefined,
        controller.signal,
      );
      if (
        mounted &&
        !controller.signal.aborted &&
        shownDiff?.snapshotId === sid
      )
        openSide(value, intent);
    } catch (e) {
      if (mounted && !controller.signal.aborted) {
        if (e instanceof APIError && e.status === 410) onState("expired");
        error = (e as Error).message;
      }
    } finally {
      requests.delete(controller);
    }
  }
</script>

<div class="flex h-full min-h-0 flex-col overflow-hidden">
  <div class="shrink-0 space-y-1 border-b p-2 text-xs">
    <p class="break-all">{doc?.absolutePath}</p>
    <p class="text-muted-foreground">
      来源：{doc?.source} · 读取于 {new Date(
        file?.checkedAt ?? diff?.checkedAt ?? "",
      ).toLocaleTimeString()} · 只读快照
    </p>
    {#if file}<div class="flex gap-2">
        <Button variant="outline" size="sm" onclick={() => (mode = "file")}
          >完整文件</Button
        ><Button
          variant="outline"
          size="sm"
          disabled={busy}
          onclick={() => void compare()}>查看 Diff</Button
        >
      </div>{/if}
    {#if shownDiff}
      <p class="break-all">
        {shownDiff.change.oldPath} → {shownDiff.change.newPath}
      </p>
      <div class="flex flex-wrap items-center gap-2">
        <label
          >引用来源 <select
            aria-label="引用来源"
            bind:value={side}
            onchange={() => {
              start = undefined;
              end = undefined;
            }}
            class="rounded border bg-background p-1"
            ><option value="new">新侧 ({shownDiff.new.source})</option><option
              value="old">旧侧 ({shownDiff.old.source})</option
            ></select
          ></label
        >
        <Button variant="outline" size="sm" onclick={() => void full("old")}
          >旧侧完整文件</Button
        >
        <Button variant="outline" size="sm" onclick={() => void full("new")}
          >新侧完整文件</Button
        >
      </div>
    {/if}
    {#if status !== "unchanged"}<p role="status">
        {status === "updated"
          ? "内容已更新"
          : status === "missing"
            ? "文件已不存在"
            : status === "expired"
              ? "快照已过期"
              : "检测失败，状态未知"}，保留旧内容。<Button
          variant="outline"
          size="sm"
          onclick={refresh}>刷新文档</Button
        >
      </p>{/if}
    {#if shownDiff?.kind === "metadata" || doc?.kind !== "text"}<p>
        {shownDiff?.reason || doc?.reason}
      </p>{/if}
  </div>
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
  {#if shownDiff?.kind === "text" && diffReady}
    <div
      class="flex max-h-28 shrink-0 flex-wrap gap-1 overflow-auto border-b p-1"
      aria-label="差异上下文"
    >
      {#if blocks.some((b) => b.kind === "gap")}<Button
          size="sm"
          variant="outline"
          disabled={status === "expired"}
          onclick={() => expand("all", "all")}>展开全部</Button
        >{/if}
      {#each blocks.filter((b) => b.kind === "gap") as b (b.gapId)}
        <span class="rounded border p-1 text-xs"
          >省略 {b.count} 行
          <Button
            size="sm"
            variant="ghost"
            disabled={status === "expired"}
            onclick={() => expand(b.gapId, "up")}>向上 30 行</Button
          >
          <Button
            size="sm"
            variant="ghost"
            disabled={status === "expired"}
            onclick={() => expand(b.gapId, "down")}>向下 30 行</Button
          >
          <Button
            size="sm"
            variant="ghost"
            disabled={status === "expired"}
            onclick={() => expand(b.gapId, "all")}>展开此段</Button
          >
        </span>
      {/each}
    </div>
  {/if}
  {#if doc?.kind === "text" && (!shownDiff || (shownDiff.kind === "text" && diffReady))}
    {#key `${lastVersion}:${generation}`}<CodeReader
        bind:this={reader}
        {text}
        path={doc.path}
        {mapping}
        restoreLine={anchor}
        onselect={select}
      />{/key}
  {/if}
  <div class="mt-auto shrink-0 space-y-1 border-t p-2">
    {#if error}<p class="text-sm text-destructive" role="alert">{error}</p>{/if}
    {#if message}<p class="text-xs" role="status">{message}</p>{/if}
    <p class="break-all font-mono text-xs" aria-label="位置预览">
      {preview || "此来源无法引用"}
    </p>
    <div class="flex gap-2">
      <Button
        variant="outline"
        size="sm"
        onclick={() => {
          start = undefined;
          end = undefined;
        }}>引用整个文件</Button
      >
      <Button
        size="sm"
        disabled={!preview || status !== "unchanged"}
        onclick={() => {
          if (paste(preview)) {
            message = "已插入终端，未自动回车";
            error = "";
          } else {
            error = "终端未就绪，位置未插入；请连接后重试";
          }
        }}>发送位置到终端</Button
      >
    </div>
  </div>
</div>
