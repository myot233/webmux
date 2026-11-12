<script lang="ts">
  import { onMount, type Snippet } from "svelte";
  import { Button } from "$lib/components/ui/button";
  import { Label } from "$lib/components/ui/label";
  import * as NativeSelect from "$lib/components/ui/native-select";
  import * as DropdownMenu from "$lib/components/ui/dropdown-menu";
  import * as SegmentedControl from "$lib/components/ui/segmented-control";
  import IconChevronDown from "~icons/tabler/chevron-down";
  import type { CodeDiff, CodeFile } from "../../contracts/schemas";
  import type { MarkdownHeading } from "./CodeMarkdownPreview.svelte";
  import { formatBytes } from "../../domains/code/code-image";
  import {
    previewFormatLabel,
    type PreviewFormat,
  } from "../../domains/code/code-preview";
  import { sourceLabel } from "../../domains/code/code-diff-client";

  let {
    doc,
    file,
    diffAvailable = true,
    busy,
    shownDiff,
    mode = $bindable<"file" | "diff">("file"),
    side = $bindable<"old" | "new">("new"),
    formatFile,
    formatPreview,
    headings,
    svgFile,
    svgView,
    svgNotice,
    status,
    menu = $bindable(),
    oncompare,
    onshowformat,
    onshowsvg,
    onscrolltoheading,
    onclearselection,
    onfull,
    onrefresh,
  }: {
    doc: CodeFile["document"] | CodeDiff["old"] | undefined;
    file?: CodeFile;
    diffAvailable?: boolean;
    busy: boolean;
    shownDiff?: CodeDiff;
    mode: "file" | "diff";
    side: "old" | "new";
    formatFile?: PreviewFormat;
    formatPreview: boolean;
    headings: MarkdownHeading[];
    svgFile: boolean;
    svgView: "preview" | "source";
    svgNotice: string;
    status: string;
    menu?: Snippet;
    oncompare: () => void;
    onshowformat: (view: "preview" | "source") => void;
    onshowsvg: (view: "preview" | "source") => void;
    onscrolltoheading: (slug: string) => void;
    onclearselection: () => void;
    onfull: (which: "old" | "new") => void;
    onrefresh: () => void;
  } = $props();

  const diffSides = ["new", "old"] as const;
  const outline = $derived(
    formatFile === "markdown" && formatPreview && headings.length >= 2,
  );
  const outlineTop = $derived(Math.min(...headings.map((h) => h.level)));
  // Outline entries indent one step per heading level below the top one.
  const indents = ["", "ps-3", "ps-6", "ps-9", "ps-12", "ps-15"];

  const imageChange = $derived(
    shownDiff?.kind !== "image"
      ? ""
      : shownDiff.old.source === "empty"
        ? "图片已新增"
        : shownDiff.new.source === "empty"
          ? "图片已删除"
          : "图片已修改",
  );
  const renamed = $derived(
    !!shownDiff && shownDiff.change.oldPath !== shownDiff.change.newPath,
  );
  const unreadable = $derived(
    shownDiff?.kind === "metadata" ||
      (doc?.kind !== "text" && doc?.kind !== "image" && doc?.kind !== "pdf"),
  );
  // Notices stay visible on narrow screens, where the rest of the document
  // bar moves into the code menu.
  const notices = $derived(
    !!imageChange || status !== "unchanged" || unreadable || !!svgNotice,
  );
  const sideSize = (value: CodeDiff["old"]) =>
    value.source === "empty" ? "不存在" : formatBytes(value.size);

  onMount(() => {
    menu = documentMenu;
  });
</script>

<!-- Markdown headings, indented by level, scroll the preview when chosen. -->
{#snippet outlineItems()}
  {#each headings as item, index (index)}
    <DropdownMenu.Item onSelect={() => onscrolltoheading(item.slug)}
      ><span class={["truncate", indents[item.level - outlineTop]]}
        >{item.text}</span
      ></DropdownMenu.Item
    >
  {/each}
{/snippet}

<!-- Narrow screens move the document bar into the code panel's menu. -->
{#snippet documentMenu()}
  <DropdownMenu.Label variant="title" class="break-all">
    {doc?.absolutePath}
    {#if renamed}
      <span class="mt-1 block text-muted-foreground"
        >{shownDiff?.change.oldPath} → {shownDiff?.change.newPath}</span
      >
    {/if}
  </DropdownMenu.Label>
  {#if file && diffAvailable}
    <DropdownMenu.Item
      disabled={busy}
      onSelect={() => (shownDiff ? (mode = "file") : oncompare())}
      >{shownDiff ? "返回文件" : "查看 Diff"}</DropdownMenu.Item
    >
  {/if}
  {#if formatFile}
    <DropdownMenu.Item
      onSelect={() => onshowformat(formatPreview ? "source" : "preview")}
      >{formatPreview
        ? "返回源码"
        : previewFormatLabel(formatFile)}</DropdownMenu.Item
    >
  {/if}
  {#if outline}
    <DropdownMenu.Sub>
      <DropdownMenu.SubTrigger>目录</DropdownMenu.SubTrigger>
      <DropdownMenu.SubContent class="max-h-[60svh] overflow-y-auto">
        {@render outlineItems()}
      </DropdownMenu.SubContent>
    </DropdownMenu.Sub>
  {/if}
  {#if svgFile}
    <DropdownMenu.Separator />
    <DropdownMenu.RadioGroup
      value={svgView}
      onValueChange={(value) => onshowsvg(value as "preview" | "source")}
    >
      <DropdownMenu.GroupHeading>SVG 显示方式</DropdownMenu.GroupHeading>
      <!-- The view changes underneath; close like the other entries. -->
      <DropdownMenu.RadioItem value="preview" closeOnSelect
        >预览</DropdownMenu.RadioItem
      >
      <DropdownMenu.RadioItem value="source" closeOnSelect
        >源码</DropdownMenu.RadioItem
      >
    </DropdownMenu.RadioGroup>
  {/if}
  {#if shownDiff}
    {#if imageChange}
      <DropdownMenu.Separator />
      <DropdownMenu.RadioGroup
        value={side}
        onValueChange={(value) => {
          side = value as "old" | "new";
          onclearselection();
        }}
      >
        <DropdownMenu.GroupHeading>预览来源</DropdownMenu.GroupHeading>
        {#each diffSides as which (which)}
          <DropdownMenu.RadioItem
            value={which}
            closeOnSelect
            disabled={shownDiff[which].source === "empty"}
            >{which === "old" ? "修改前" : "修改后"}<span
              class="ml-auto text-xs text-muted-foreground"
              >{sourceLabel(shownDiff[which].source)}</span
            ></DropdownMenu.RadioItem
          >
        {/each}
      </DropdownMenu.RadioGroup>
    {/if}
    <DropdownMenu.Separator />
    <DropdownMenu.Group>
      <DropdownMenu.GroupHeading>完整文件</DropdownMenu.GroupHeading>
      {@render fullItems()}
    </DropdownMenu.Group>
  {/if}
  <DropdownMenu.Separator />
  <DropdownMenu.Item onSelect={onrefresh}>刷新文档</DropdownMenu.Item>
{/snippet}

{#snippet fullItems()}
  {#if shownDiff}
    {#each diffSides as which (which)}
      {#if shownDiff[which].source !== "empty"}
        <DropdownMenu.Item onSelect={() => onfull(which)}>
          {which === "old" ? "修改前的文件" : "修改后的文件"}
          <span class="ml-auto text-xs text-muted-foreground"
            >{sourceLabel(shownDiff[which].source)}</span
          >
        </DropdownMenu.Item>
      {/if}
    {/each}
  {/if}
{/snippet}

<div
  class={[
    "shrink-0 space-y-1 border-b px-2 py-1 text-xs",
    !notices && "max-sm:hidden",
  ]}
>
  <div class="flex items-center gap-2 max-sm:hidden">
    <p
      class="min-w-0 flex-1 truncate text-left text-muted-foreground [direction:rtl]"
      title={doc?.absolutePath}
    >
      <bdi>{doc?.absolutePath}</bdi>
    </p>
    {#if file && diffAvailable}<Button
        variant="ghost"
        size="sm"
        disabled={busy}
        onclick={() => (shownDiff ? (mode = "file") : oncompare())}
        >{shownDiff ? "返回文件" : "查看 Diff"}</Button
      >{/if}
    {#if formatFile}<Button
        variant="ghost"
        size="sm"
        aria-pressed={formatPreview}
        onclick={() => onshowformat(formatPreview ? "source" : "preview")}
        >{formatPreview ? "返回源码" : "预览"}</Button
      >{/if}
    {#if outline}<DropdownMenu.Root>
        <DropdownMenu.Trigger>
          {#snippet child({ props })}
            <Button {...props} variant="ghost" size="sm">
              目录<IconChevronDown aria-hidden="true" />
            </Button>
          {/snippet}
        </DropdownMenu.Trigger>
        <DropdownMenu.Content align="end">
          {@render outlineItems()}
        </DropdownMenu.Content>
      </DropdownMenu.Root>{/if}
    {#if svgFile}<SegmentedControl.Root
        value={svgView}
        onValueChange={(view) =>
          onshowsvg(view === "source" ? "source" : "preview")}
        aria-label="SVG 显示方式"
        class="shrink-0"
      >
        <SegmentedControl.Item value="preview">预览</SegmentedControl.Item>
        <SegmentedControl.Item value="source">源码</SegmentedControl.Item>
      </SegmentedControl.Root>{/if}
    {#if shownDiff}
      {#if imageChange}<Label size="sm" class="shrink-0"
          >预览来源
          <NativeSelect.Root
            size="sm"
            aria-label="预览来源"
            bind:value={side}
            onchange={onclearselection}
            ><NativeSelect.Option value="new"
              >修改后（{sourceLabel(
                shownDiff.new.source,
              )}）</NativeSelect.Option
            ><NativeSelect.Option value="old"
              >修改前（{sourceLabel(
                shownDiff.old.source,
              )}）</NativeSelect.Option
            ></NativeSelect.Root
          ></Label
        >{/if}
      <DropdownMenu.Root>
        <DropdownMenu.Trigger>
          {#snippet child({ props })}
            <Button {...props} variant="ghost" size="sm">
              完整文件<IconChevronDown aria-hidden="true" />
            </Button>
          {/snippet}
        </DropdownMenu.Trigger>
        <DropdownMenu.Content align="end">
          {@render fullItems()}
        </DropdownMenu.Content>
      </DropdownMenu.Root>
    {/if}
  </div>
  {#if renamed}<p class="break-all max-sm:hidden">
      {shownDiff?.change.oldPath} → {shownDiff?.change.newPath}
    </p>{/if}
  {#if imageChange && shownDiff}<p>
      <span class="font-medium">{imageChange}</span> ·
      <span aria-label="大小变化"
        >{sideSize(shownDiff.old)} → {sideSize(shownDiff.new)}</span
      >
    </p>{/if}
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
        onclick={onrefresh}>刷新文档</Button
      >
    </p>{/if}
  {#if unreadable}<p>
      {shownDiff?.reason || doc?.reason}
    </p>{/if}
  {#if svgNotice}<p role="status">{svgNotice}</p>{/if}
</div>
