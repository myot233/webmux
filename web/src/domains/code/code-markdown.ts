import DOMPurify, { type Config } from "dompurify";
import { marked, type Token, type TokensList } from "marked";
import { highlightInto } from "./code-highlight";
import { loadFenceLanguage } from "./code-language";
import type { CodeMarkdownState } from "../../contracts/schemas";
import {
  isExternalLink,
  isWorkspaceReference,
  parseChatCodeLink,
  resolveMarkdownReference,
  targetFailure,
  type MarkdownImage,
  type MarkdownWorkspace,
} from "./markdown-link";
import { createSlugger, headingIdPrefix } from "./markdown-slug";

export type MarkdownHeading = { level: number; text: string; slug: string };

const forbiddenTags = [
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
  // Image maps would keep a raw relative `href` on `<area>`.
  "map",
  "area",
];
// Authored `id` / `name` could clobber application elements.
const forbiddenAttributes = ["style", "srcset", "background", "id", "name"];

// Each variant owns its DOMPurify instance: hooks are instance-wide state,
// so the file preview's link and image handling never reaches chat messages.
let allowMessageCodeLinks = false;
const pendingCodeHrefs = new WeakMap<Element, string>();
const messagePurifier = DOMPurify(window);
messagePurifier.addHook("uponSanitizeAttribute", (node, data) => {
  if (data.attrName.startsWith("data-md-")) {
    data.keepAttr = false;
    return;
  }
  // Capture valid code-location targets before DOMPurify's built-in URI
  // filter strips non-HTTP schemes (`file:///...`) or colon line suffixes
  // (`Makefile:15`), and never leave a non-external `href` on `<a>`.
  if (
    node.tagName === "A" &&
    data.attrName === "href" &&
    !isExternalLink(data.attrValue)
  ) {
    if (allowMessageCodeLinks && parseChatCodeLink(data.attrValue))
      pendingCodeHrefs.set(node, data.attrValue);
    data.keepAttr = false;
  }
});
messagePurifier.addHook("afterSanitizeAttributes", (node) => {
  if (node.nodeType !== Node.ELEMENT_NODE) return;
  const element = node as Element;
  if (element.tagName !== "A") return;
  const href = element.getAttribute("href");
  if (href !== null && isExternalLink(href)) {
    element.setAttribute("target", "_blank");
    element.setAttribute("rel", "noopener noreferrer");
    return;
  }
  element.removeAttribute("href");
  const codeHref = pendingCodeHrefs.get(element);
  if (!codeHref) return;
  pendingCodeHrefs.delete(element);
  element.setAttribute("data-md-href", codeHref);
  element.setAttribute("role", "link");
  element.setAttribute("tabindex", "0");
});
const messageConfig: Config = {
  FORBID_TAGS: ["img", ...forbiddenTags],
  FORBID_ATTR: forbiddenAttributes,
  ADD_ATTR: ["target", "rel"],
};
function sanitizeMessageHtml(html: string, codeLinks: boolean) {
  allowMessageCodeLinks = codeLinks;
  try {
    return messagePurifier.sanitize(html, messageConfig);
  } finally {
    allowMessageCodeLinks = false;
  }
}

const documentPurifier = DOMPurify(window);
// `data-md-*` attributes carry the preview's own resolved references; an
// author must not be able to forge them.
documentPurifier.addHook("uponSanitizeAttribute", (_node, data) => {
  if (data.attrName.startsWith("data-md-")) data.keepAttr = false;
});
// Workspace addresses leave `href` / `src` while the markup is still inert,
// so the browser never requests them as application routes.
documentPurifier.addHook("afterSanitizeAttributes", (node) => {
  if (node.nodeType !== Node.ELEMENT_NODE) return;
  const element = node as Element;
  if (element.tagName === "A") {
    const href = element.getAttribute("href");
    if (href === null) return;
    if (isExternalLink(href)) {
      element.setAttribute("target", "_blank");
      element.setAttribute("rel", "noopener noreferrer");
      return;
    }
    element.removeAttribute("href");
    if (!isWorkspaceReference(href)) return;
    element.setAttribute("data-md-href", href);
    element.setAttribute("role", "link");
    element.setAttribute("tabindex", "0");
  } else if (element.tagName === "IMG") {
    const src = element.getAttribute("src") ?? "";
    element.removeAttribute("src");
    element.setAttribute(
      isWorkspaceReference(src) ? "data-md-src" : "data-md-external",
      src,
    );
  }
});
const documentConfig = {
  FORBID_TAGS: forbiddenTags,
  FORBID_ATTR: forbiddenAttributes,
  ADD_ATTR: ["target", "rel"],
  RETURN_DOM_FRAGMENT: true,
} as const satisfies Config;

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
const imageText = (label: string, address: string) =>
  `图片：${label || address}${label && address ? `（${address}）` : ""}`;

const messageRenderer = new marked.Renderer();
// Chat messages never load images: keep the reference as text.
messageRenderer.image = ({ href, text }) =>
  `<span class="markdown-image">${escapeHtml(imageText(text, href))}</span>`;

export type MarkdownBlock =
  | { kind: "html"; html: string }
  | { kind: "code"; code: string };

// Prose between fenced code blocks is rendered (and sanitized) as before;
// the code blocks themselves stay plain text next to their copy buttons.
export function renderMessageBlocks(
  source: string,
  codeLinks: boolean,
): MarkdownBlock[] {
  const tokens = marked.lexer(source, { gfm: true });
  const out: MarkdownBlock[] = [];
  let run: Token[] = [];
  const flush = () => {
    if (!run.length) return;
    const list = Object.assign(run, { links: tokens.links }) as TokensList;
    out.push({
      kind: "html",
      html: sanitizeMessageHtml(
        marked.parser(list, { gfm: true, renderer: messageRenderer }),
        codeLinks,
      ),
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

// A relative image waiting in its placeholder until it is near the viewport.
export type MarkdownPicture = {
  element: HTMLElement;
  address: string;
  alt: string;
  path: string;
  width: string;
  height: string;
};

function placeholder(image: Element) {
  const span = image.ownerDocument.createElement("span");
  span.className = "markdown-image";
  const alt = image.getAttribute("alt") ?? "";
  const address =
    image.getAttribute("data-md-src") ??
    image.getAttribute("data-md-external") ??
    "";
  span.textContent = imageText(alt, address);
  image.replaceWith(span);
  return { span, alt, address };
}

export function renderMarkdownDocument(
  element: HTMLElement,
  source: string,
  base: string,
  isCurrent: () => boolean,
) {
  const fragment = documentPurifier.sanitize(
    marked.parse(source, { async: false, gfm: true }),
    documentConfig,
  );
  const found: MarkdownPicture[] = [];
  for (const image of fragment.querySelectorAll("img")) {
    const relative = image.getAttribute("data-md-src");
    const { span, alt, address } = placeholder(image);
    if (relative === null) continue;
    const reference = resolveMarkdownReference(base, relative);
    if (reference.kind !== "path") {
      span.textContent = `${reference.kind === "outside" ? "图片不在工作区内" : "图片地址无效"}（${address}）`;
      continue;
    }
    found.push({
      element: span,
      address,
      alt,
      path: reference.path,
      width: image.getAttribute("width") ?? "",
      height: image.getAttribute("height") ?? "",
    });
  }
  const slug = createSlugger();
  const outline: MarkdownHeading[] = [];
  for (const heading of fragment.querySelectorAll("h1, h2, h3, h4, h5, h6")) {
    const label = heading.textContent ?? "";
    const id = slug(label);
    heading.id = headingIdPrefix + id;
    outline.push({
      level: Number(heading.tagName[1]),
      text: label,
      slug: id,
    });
  }
  element.replaceChildren(fragment);
  for (const code of element.querySelectorAll("pre > code")) {
    const language = /(?:^|\s)language-(\S+)/.exec(code.className)?.[1];
    if (!language) continue;
    const plain = code.textContent ?? "";
    void loadFenceLanguage(language)
      .then((support) => {
        if (support && isCurrent()) highlightInto(code, plain, support);
      })
      // A grammar chunk that fails to load leaves the block as plain text.
      .catch(() => undefined);
  }
  return { outline, pictures: found };
}

const maxImages = 50;
const parallelImages = 2;

export function observeMarkdownPictures(
  list: MarkdownPicture[],
  scroller: HTMLElement,
  source: MarkdownWorkspace,
  onRetryableChange: (retryable: boolean) => void,
) {
  const controller = new AbortController();
  const urls: string[] = [];
  const queue: MarkdownPicture[] = [];
  const busy: MarkdownPicture[] = [];
  let running = 0;
  const pump = () => {
    while (running < parallelImages && queue.length) {
      const picture = queue.shift()!;
      running++;
      void load(picture).finally(() => {
        running--;
        if (!controller.signal.aborted) pump();
      });
    }
  };
  const fail = (picture: MarkdownPicture, reason: string) => {
    picture.element.textContent = `${reason}（${picture.address}）`;
  };
  const load = async (picture: MarkdownPicture) => {
    picture.element.setAttribute("aria-busy", "true");
    let result: MarkdownImage;
    try {
      result = await source.image(picture.path, controller.signal);
    } catch (error) {
      result = { ok: false, reason: targetFailure("图片", error) };
    }
    if (controller.signal.aborted) return;
    picture.element.removeAttribute("aria-busy");
    if (!result.ok) {
      fail(picture, result.reason);
      if (result.retry) {
        busy.push(picture);
        onRetryableChange(true);
      }
      return;
    }
    const url = URL.createObjectURL(result.blob);
    urls.push(url);
    const image = picture.element.ownerDocument.createElement("img");
    image.alt = picture.alt;
    for (const name of ["width", "height"] as const)
      if (/^\d+$/.test(picture[name])) image.setAttribute(name, picture[name]);
    image.src = url;
    picture.element.replaceChildren(image);
    picture.element.dataset.mdOpen = result.path;
    picture.element.setAttribute("role", "link");
    picture.element.setAttribute("tabindex", "0");
    picture.element.title = `在代码面板中打开 ${result.path}`;
  };
  const observer = new IntersectionObserver(
    (entries) => {
      for (const entry of entries) {
        if (!entry.isIntersecting) continue;
        observer.unobserve(entry.target);
        const picture = list.find((item) => item.element === entry.target);
        if (picture) queue.push(picture);
      }
      pump();
    },
    { root: scroller, rootMargin: "100% 0px" },
  );
  list.forEach((picture, index) => {
    if (index < maxImages) observer.observe(picture.element);
    else fail(picture, "图片过多，未加载");
  });
  return {
    retry: () => {
      onRetryableChange(false);
      queue.push(...busy.splice(0));
      pump();
    },
    cleanup: () => {
      controller.abort();
      observer.disconnect();
      for (const url of urls) URL.revokeObjectURL(url);
      onRetryableChange(false);
      for (const picture of list) {
        picture.element.removeAttribute("aria-busy");
        picture.element.removeAttribute("data-md-open");
        picture.element.removeAttribute("role");
        picture.element.removeAttribute("tabindex");
        picture.element.removeAttribute("title");
        picture.element.textContent = imageText(picture.alt, picture.address);
      }
    },
  };
}

export function measureMarkdownPosition(
  region: HTMLElement | undefined,
  content: HTMLElement | undefined,
  fallback: CodeMarkdownState,
): CodeMarkdownState {
  if (!region || !content || region.clientHeight === 0) return fallback;
  const top = region.getBoundingClientRect().top;
  let last: Element | undefined;
  for (const heading of content.querySelectorAll("h1, h2, h3, h4, h5, h6")) {
    if (heading.getBoundingClientRect().top > top + 1) break;
    last = heading;
  }
  const range = region.scrollHeight - region.clientHeight;
  return last
    ? {
        heading: last.id.slice(headingIdPrefix.length),
        offset: top - last.getBoundingClientRect().top,
      }
    : {
        heading: "",
        offset: range > 0 ? Math.min(1, region.scrollTop / range) : 0,
      };
}

export function placeMarkdownPosition(
  region: HTMLElement | undefined,
  content: HTMLElement | undefined,
  state: CodeMarkdownState,
) {
  if (!region || !content) return;
  if (!state.heading) {
    region.scrollTop =
      state.offset * (region.scrollHeight - region.clientHeight);
    return;
  }
  const heading = content.querySelector(
    `#${CSS.escape(headingIdPrefix + state.heading)}`,
  );
  if (heading)
    region.scrollTop +=
      heading.getBoundingClientRect().top -
      region.getBoundingClientRect().top +
      state.offset;
}
