import { HighlightStyle, type LanguageSupport } from "@codemirror/language";
import { highlightCode, tags } from "@lezer/highlight";

/**
 * Mirrors CodeMirror's default highlight tags, but reads colors from
 * `--code-*` variables so `.dark` swaps the palette without reconfiguring.
 */
export const codeHighlightStyle = HighlightStyle.define([
  { tag: tags.meta, color: "var(--code-meta)" },
  { tag: tags.link, textDecoration: "underline" },
  { tag: tags.heading, textDecoration: "underline", fontWeight: "bold" },
  { tag: tags.emphasis, fontStyle: "italic" },
  { tag: tags.strong, fontWeight: "bold" },
  { tag: tags.strikethrough, textDecoration: "line-through" },
  { tag: tags.keyword, color: "var(--code-keyword)" },
  {
    tag: [
      tags.atom,
      tags.bool,
      tags.url,
      tags.contentSeparator,
      tags.labelName,
    ],
    color: "var(--code-atom)",
  },
  { tag: [tags.literal, tags.inserted], color: "var(--code-literal)" },
  { tag: [tags.string, tags.deleted], color: "var(--code-string)" },
  {
    tag: [tags.regexp, tags.escape, tags.special(tags.string)],
    color: "var(--code-regexp)",
  },
  {
    tag: tags.definition(tags.variableName),
    color: "var(--code-definition)",
  },
  { tag: tags.local(tags.variableName), color: "var(--code-local)" },
  { tag: [tags.typeName, tags.namespace], color: "var(--code-type)" },
  { tag: tags.className, color: "var(--code-class)" },
  {
    tag: [tags.special(tags.variableName), tags.macroName],
    color: "var(--code-special)",
  },
  {
    tag: tags.definition(tags.propertyName),
    color: "var(--code-property)",
  },
  { tag: tags.comment, color: "var(--code-comment)" },
  { tag: tags.invalid, color: "var(--code-invalid)" },
]);

// Editors mount the style's rules themselves; static highlighting has to
// adopt them into each document it renders into.
const styled = new WeakSet<Document>();

/**
 * Replaces `target`'s content with `text` highlighted by `support`, built only
 * from text nodes and class-carrying spans, so code is never parsed as HTML.
 */
export function highlightInto(
  target: Element,
  text: string,
  support: LanguageSupport,
) {
  const document = target.ownerDocument;
  const rules = codeHighlightStyle.module?.getRules();
  if (rules && !styled.has(document) && document.defaultView) {
    const sheet = new document.defaultView.CSSStyleSheet();
    sheet.replaceSync(rules);
    document.adoptedStyleSheets = [...document.adoptedStyleSheets, sheet];
    styled.add(document);
  }
  const fragment = document.createDocumentFragment();
  highlightCode(
    text,
    support.language.parser.parse(text),
    codeHighlightStyle,
    (chunk, classes) => {
      if (!classes) return fragment.append(chunk);
      const span = document.createElement("span");
      span.className = classes;
      span.textContent = chunk;
      fragment.append(span);
    },
    () => fragment.append("\n"),
  );
  target.replaceChildren(fragment);
}
