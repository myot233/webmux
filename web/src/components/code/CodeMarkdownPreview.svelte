<script module lang="ts">
  export type { MarkdownHeading } from "../../domains/code/code-markdown";
</script>

<script lang="ts">
  import { untrack } from "svelte";
  import type { CodeMarkdownState } from "../../contracts/schemas";
  import IconCheck from "~icons/tabler/check";
  import IconCopy from "~icons/tabler/copy";
  import { Button } from "$lib/components/ui/button";
  import InlineNotice from "../shared/InlineNotice.svelte";
  import { copyText } from "../../platform/clipboard";
  import {
    parseChatCodeLink,
    resolveMarkdownReference,
    type ChatCodeLink,
    type MarkdownWorkspace,
  } from "../../domains/code/markdown-link";
  import { headingIdPrefix } from "../../domains/code/markdown-slug";
  import {
    measureMarkdownPosition,
    observeMarkdownPictures,
    placeMarkdownPosition,
    renderMarkdownDocument,
    renderMessageBlocks,
    type MarkdownHeading,
    type MarkdownPicture,
  } from "../../domains/code/code-markdown";

  // `document` previews a Markdown file in the code panel; `message` renders
  // an agent's chat message, with each fenced code block copyable on its own.
  let {
    text,
    variant = "document",
    path = "",
    active = true,
    workspace,
    headings = $bindable([]),
    restore,
    onopenlink,
  }: {
    text: string;
    variant?: "document" | "message";
    /** Root-relative path of the previewed file; relative links resolve against it. */
    path?: string;
    /** Inactive previews cancel image reads and release their object URLs. */
    active?: boolean;
    workspace?: MarkdownWorkspace;
    headings?: MarkdownHeading[];
    /** A captured reading position, applied after the first render with layout. */
    restore?: CodeMarkdownState;
    /** Opens a code-location link from a chat message in the code browser. */
    onopenlink?: (target: ChatCodeLink) => void;
  } = $props();

  const parts = $derived(
    variant === "message" ? renderMessageBlocks(text, Boolean(onopenlink)) : [],
  );
  let copied = $state(-1);
  async function copy(index: number, code: string) {
    if (await copyText(code)) copied = index;
  }

  let messageRoot = $state<HTMLDivElement>();
  let region = $state<HTMLDivElement>();
  let content = $state<HTMLDivElement>();
  let pictures = $state.raw<MarkdownPicture[]>([]);
  let notice = $state("");
  let retryable = $state(false);
  let retry: (() => void) | undefined;
  let rendering = 0;

  // Renders, sanitizes and enhances the document: image placeholders,
  // heading anchors and (asynchronously) highlighted code blocks.
  $effect(() => {
    const element = content;
    if (variant !== "document" || !element) return;
    const source = text;
    const base = path;
    const generation = ++rendering;
    const { outline, pictures: found } = renderMarkdownDocument(
      element,
      source,
      base,
      () => generation === rendering,
    );
    untrack(() => {
      headings = outline;
      pictures = found;
      notice = "";
      applyRestore();
    });
  });

  // Until applied, `capture()` returns it unchanged.
  let pending = untrack(() => restore);
  // The position last measured while the preview had layout; a hidden tab
  // reports this one.
  let known: CodeMarkdownState = { heading: "", offset: 0 };
  // A restored position is held while late images change the layout above
  // it, until the reader moves on.
  let anchor: CodeMarkdownState | undefined;
  function measure(): CodeMarkdownState {
    known = measureMarkdownPosition(region, content, known);
    return known;
  }
  export function capture(): CodeMarkdownState {
    return pending ?? measure();
  }
  function place(state: CodeMarkdownState) {
    placeMarkdownPosition(region, content, state);
  }
  function applyRestore() {
    const state = pending;
    if (!state || !region || region.clientHeight === 0) return;
    pending = undefined;
    anchor = state;
    known = state;
    place(state);
  }
  // Defers a restore for a hidden tab, keeps a restored position anchored
  // while images load, and tracks the position while scrolling.
  $effect(() => {
    const scroller = region;
    const inner = content;
    if (variant !== "document" || !scroller || !inner) return;
    const observer = new ResizeObserver(() => {
      if (pending) applyRestore();
      else if (anchor && scroller.clientHeight > 0) place(anchor);
    });
    observer.observe(scroller);
    observer.observe(inner);
    let measuring = 0;
    const scrolled = () => {
      if (pending || measuring) return;
      measuring = requestAnimationFrame(() => {
        measuring = 0;
        if (!pending) measure();
      });
    };
    const release = () => {
      anchor = undefined;
    };
    const inputs = ["wheel", "pointerdown", "touchstart", "keydown"] as const;
    scroller.addEventListener("scroll", scrolled, { passive: true });
    for (const type of inputs)
      scroller.addEventListener(type, release, { passive: true });
    return () => {
      observer.disconnect();
      cancelAnimationFrame(measuring);
      scroller.removeEventListener("scroll", scrolled);
      for (const type of inputs) scroller.removeEventListener(type, release);
    };
  });

  // Loads images near the viewport through the workspace, at most two at a
  // time and fifty per document. Deactivating or re-rendering cancels the
  // reads and releases every object URL.
  $effect(() => {
    const list = pictures;
    const scroller = region;
    const source = workspace;
    // Without workspace access the placeholders stay as text.
    if (!active || !scroller || !source || !list.length) return;
    const handle = observeMarkdownPictures(list, scroller, source, (value) => {
      retryable = value;
    });
    retry = handle.retry;
    return () => {
      retry = undefined;
      handle.cleanup();
    };
  });

  /** Scrolls the preview to a heading slug; returns false and explains when it is missing. */
  export function scrollToHeading(slug: string) {
    const heading = content?.querySelector(
      `#${CSS.escape(headingIdPrefix + slug)}`,
    );
    if (!heading) {
      notice = `未找到标题“${slug}”`;
      return false;
    }
    notice = "";
    // An explicit navigation replaces any restored position.
    pending = anchor = undefined;
    heading.scrollIntoView({ block: "start" });
    return true;
  }

  async function follow(element: Element) {
    const opened = element.getAttribute("data-md-open");
    if (opened !== null) {
      notice = (await workspace?.open(opened)) ?? "";
      return;
    }
    const reference = resolveMarkdownReference(
      path,
      element.getAttribute("data-md-href") ?? "",
    );
    if (reference.kind === "outside") notice = "链接目标不在工作区内";
    else if (reference.kind === "invalid") notice = "链接地址无效";
    else if (reference.kind === "anchor") scrollToHeading(reference.fragment);
    else if (reference.path === path) {
      if (reference.fragment) scrollToHeading(reference.fragment);
      else region?.scrollTo({ top: 0 });
    } else if (!workspace) notice = "无法在此打开链接";
    else {
      notice = "";
      notice = await workspace.open(reference.path, reference.fragment);
    }
  }

  // Delegated so links and loaded images inside the rendered markup work the
  // same with or without modifier keys, and never navigate the app.
  $effect(() => {
    const element = variant === "message" ? messageRoot : content;
    if (!element) return;
    const hit = (event: Event) =>
      (event.target as Element | null)?.closest(
        "[data-md-href], [data-md-open]",
      );
    const activateLink = (link: Element) => {
      if (variant === "message") {
        const parsed = parseChatCodeLink(
          link.getAttribute("data-md-href") ?? "",
        );
        if (parsed) onopenlink?.(parsed);
        return;
      }
      void follow(link);
    };
    const click = (event: MouseEvent) => {
      const link = hit(event);
      if (!link) return;
      event.preventDefault();
      activateLink(link);
    };
    const keydown = (event: KeyboardEvent) => {
      const link = hit(event);
      if (!link || event.key !== "Enter") return;
      event.preventDefault();
      activateLink(link);
    };
    element.addEventListener("click", click);
    element.addEventListener("keydown", keydown);
    return () => {
      element.removeEventListener("click", click);
      element.removeEventListener("keydown", keydown);
    };
  });
</script>

{#if variant === "message"}
  <div
    bind:this={messageRoot}
    class="markdown-preview markdown-message text-sm"
  >
    {#each parts as part, i (i)}
      {#if part.kind === "html"}
        <!-- Sanitized by DOMPurify; raw HTML never reaches the page unsanitized. -->
        {@html part.html}
      {:else}
        <div class="relative">
          <pre><code>{part.code}</code></pre>
          <Button
            variant="ghost"
            size="icon-xs"
            class="absolute top-1 right-1"
            aria-label={copied === i ? "已复制代码" : "复制代码"}
            title={copied === i ? "已复制" : "复制代码"}
            onclick={() => copy(i, part.code)}
            >{#if copied === i}<IconCheck aria-hidden="true" />{:else}<IconCopy
                aria-hidden="true"
              />{/if}</Button
          >
        </div>
      {/if}
    {/each}
  </div>
{:else}
  <div class="flex h-full min-h-0 flex-col">
    {#if notice || retryable}
      <div class="flex shrink-0 flex-wrap items-center gap-2 p-2">
        {#if notice}<InlineNotice tone="warning" class="min-w-0 flex-1 py-1.5"
            >{notice}</InlineNotice
          >{/if}
        {#if retryable}<Button
            variant="outline"
            size="sm"
            onclick={() => retry?.()}>重试加载图片</Button
          >{/if}
      </div>
    {/if}
    <!-- Content is a DOMPurify fragment; raw HTML never reaches the page unsanitized. -->
    <!-- svelte-ignore a11y_no_noninteractive_tabindex (scrollable preview needs keyboard focus) -->
    <div
      bind:this={region}
      class="markdown-preview min-h-0 flex-1 overflow-auto p-4 text-sm"
      role="region"
      tabindex="0"
      aria-label="Markdown 预览"
    >
      <div bind:this={content}></div>
    </div>
  </div>
{/if}

<style>
  .markdown-preview {
    line-height: 1.6;
  }
  .markdown-message {
    min-width: 0;
    max-width: 100%;
    overflow-wrap: anywhere;
  }
  .markdown-preview :global(h1),
  .markdown-preview :global(h2),
  .markdown-preview :global(h3),
  .markdown-preview :global(h4),
  .markdown-preview :global(h5),
  .markdown-preview :global(h6) {
    margin: 1.2em 0 0.6em;
    font-weight: 600;
    line-height: 1.3;
  }
  .markdown-preview :global(h1) {
    font-size: 1.5em;
  }
  .markdown-preview :global(h2) {
    font-size: 1.3em;
  }
  .markdown-preview :global(h3) {
    font-size: 1.15em;
  }
  .markdown-preview > :global(:first-child),
  .markdown-preview > div > :global(:first-child) {
    margin-top: 0;
  }
  .markdown-message > :global(:last-child),
  .markdown-message > div:last-child > pre {
    margin-bottom: 0;
  }
  .markdown-preview :global(p),
  .markdown-preview :global(ul),
  .markdown-preview :global(ol),
  .markdown-preview :global(blockquote),
  .markdown-preview :global(pre),
  .markdown-preview :global(table) {
    margin: 0.8em 0;
  }
  .markdown-preview :global(ul),
  .markdown-preview :global(ol) {
    padding-left: 1.5em;
  }
  .markdown-preview :global(ul) {
    list-style: disc;
  }
  .markdown-preview :global(ol) {
    list-style: decimal;
  }
  .markdown-preview :global(li) {
    margin: 0.25em 0;
  }
  .markdown-preview :global(blockquote) {
    border-left: 3px solid var(--border);
    padding-left: 1em;
    color: var(--muted-foreground);
  }
  .markdown-preview :global(code) {
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    background: var(--muted);
    padding: 0.1em 0.35em;
    font-family: var(--font-mono, monospace);
    font-size: 0.9em;
  }
  .markdown-preview :global(pre) {
    overflow-x: auto;
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    background: var(--muted);
    padding: 0.75em 1em;
  }
  .markdown-preview :global(pre code) {
    border: 0;
    background: transparent;
    padding: 0;
    font-size: 0.9em;
  }
  .markdown-preview :global(a[href]),
  .markdown-preview :global(a[data-md-href]),
  .markdown-preview :global([data-md-open]) {
    border-radius: var(--radius-sm);
    color: var(--primary);
    text-decoration: underline;
    text-underline-offset: 2px;
    cursor: pointer;
  }
  .markdown-preview :global(a[href]:hover),
  .markdown-preview :global(a[data-md-href]:hover),
  .markdown-preview :global([data-md-open]:hover) {
    opacity: 0.8;
  }
  .markdown-preview :global(a[href]:focus-visible),
  .markdown-preview :global(a[data-md-href]:focus-visible),
  .markdown-preview :global([data-md-open]:focus-visible) {
    outline: 2px solid var(--ring);
    outline-offset: 2px;
  }
  .markdown-preview :global(hr) {
    margin: 1.5em 0;
    border: 0;
    border-top: 1px solid var(--border);
  }
  .markdown-preview :global(table) {
    width: 100%;
    border-collapse: collapse;
  }
  .markdown-preview :global(th),
  .markdown-preview :global(td) {
    border: 1px solid var(--border);
    padding: 0.4em 0.6em;
    text-align: left;
  }
  .markdown-preview :global(th) {
    background: var(--muted);
    font-weight: 600;
  }
  .markdown-preview :global(.markdown-image) {
    color: var(--muted-foreground);
    font-style: italic;
  }
  .markdown-preview :global(.markdown-image img) {
    display: inline-block;
    max-width: 100%;
    height: auto;
  }
</style>
