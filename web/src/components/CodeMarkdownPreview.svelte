<script module lang="ts">
  import DOMPurify from "dompurify";

  // A dedicated instance keeps this preview's link policy out of any other
  // sanitizer in the app.
  const purifier = DOMPurify(window);
  // Raw HTML links bypass the Markdown renderer: send absolute links to a new
  // tab and drop relative hrefs that would reload the app at an unknown route.
  purifier.addHook("afterSanitizeAttributes", (node) => {
    if (node.nodeType !== Node.ELEMENT_NODE) return;
    const element = node as Element;
    if (element.tagName !== "A") return;
    const href = element.getAttribute("href") ?? "";
    if (/^(?:https?:|mailto:)/i.test(href)) {
      element.setAttribute("target", "_blank");
      element.setAttribute("rel", "noopener noreferrer");
    } else element.removeAttribute("href");
  });
</script>

<script lang="ts">
  import { marked, type Token, type TokensList } from "marked";
  import IconCheck from "~icons/tabler/check";
  import IconCopy from "~icons/tabler/copy";
  import { Button } from "$lib/components/ui/button";
  import { copyText } from "../clipboard";

  // `document` previews a Markdown file in the code panel; `message` renders
  // an agent's chat message, with each fenced code block copyable on its own.
  let {
    text,
    variant = "document",
  }: { text: string; variant?: "document" | "message" } = $props();

  const escapeHtml = (value: string) =>
    value.replace(
      /[&<>"']/g,
      (char) =>
        ({
          "&": "&amp;",
          "<": "&lt;",
          ">": "&gt;",
          '"': "&quot;",
          "'": "&#39;",
        })[char]!,
    );

  const renderer = new marked.Renderer();
  // Relative and repository images are not served by the snapshot endpoint;
  // keep the reference as text instead of fetching remote resources.
  renderer.image = ({ href, text }) => {
    const label = text || href;
    const suffix = text && href ? `（${escapeHtml(href)}）` : "";
    return `<span class="markdown-image">图片：${escapeHtml(label)}${suffix}</span>`;
  };

  const sanitize = (markup: string) =>
    purifier.sanitize(markup, {
      FORBID_TAGS: [
        "img",
        "image",
        "svg",
        "math",
        "use",
        "foreignobject",
        "picture",
        "video",
        "audio",
        "source",
        "track",
        "iframe",
        "object",
        "embed",
        "form",
        "input",
        "button",
        "textarea",
        "select",
        "option",
        "link",
        "meta",
        "base",
        "style",
      ],
      FORBID_ATTR: ["style", "srcset", "background"],
      ADD_ATTR: ["target", "rel"],
    });

  type Block = { kind: "html"; html: string } | { kind: "code"; code: string };
  // Prose between fenced code blocks is rendered (and sanitized) as before;
  // the code blocks themselves stay plain text next to their copy buttons.
  function blocks(source: string): Block[] {
    const tokens = marked.lexer(source, { gfm: true });
    const out: Block[] = [];
    let run: Token[] = [];
    const flush = () => {
      if (!run.length) return;
      const list = Object.assign(run, { links: tokens.links }) as TokensList;
      out.push({
        kind: "html",
        html: sanitize(marked.parser(list, { gfm: true, renderer })),
      });
      run = [];
    };
    for (const token of tokens) {
      if (token.type !== "code") {
        run.push(token);
        continue;
      }
      flush();
      out.push({ kind: "code", code: token.text });
    }
    flush();
    return out;
  }

  const html = $derived(
    variant === "document"
      ? sanitize(marked.parse(text, { async: false, gfm: true, renderer }))
      : "",
  );
  const parts = $derived(variant === "message" ? blocks(text) : []);
  let copied = $state(-1);
  async function copy(index: number, code: string) {
    if (await copyText(code)) copied = index;
  }
</script>

{#if variant === "message"}
  <div class="markdown-preview markdown-message text-sm">
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
  <!-- Sanitized by DOMPurify; raw HTML never reaches the page unsanitized. -->
  <!-- svelte-ignore a11y_no_noninteractive_tabindex (scrollable preview needs keyboard focus) -->
  <div
    class="markdown-preview h-full overflow-auto p-4 text-sm"
    role="region"
    tabindex="0"
    aria-label="Markdown 预览"
  >
    {@html html}
  </div>
{/if}

<style>
  .markdown-preview {
    line-height: 1.6;
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
  .markdown-preview > :global(:first-child) {
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
    border-radius: 4px;
    background: var(--muted);
    padding: 0.1em 0.35em;
    font-family: var(--font-mono, monospace);
    font-size: 0.9em;
  }
  .markdown-preview :global(pre) {
    overflow-x: auto;
    border: 1px solid var(--border);
    border-radius: 6px;
    background: var(--muted);
    padding: 0.75em 1em;
  }
  .markdown-preview :global(pre code) {
    border: 0;
    background: transparent;
    padding: 0;
    font-size: 0.9em;
  }
  .markdown-preview :global(a) {
    color: var(--primary);
    text-decoration: underline;
    text-underline-offset: 2px;
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
</style>
