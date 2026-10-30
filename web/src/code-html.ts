// Static HTML preview: the source is parsed inertly (DOMParser neither runs
// scripts nor loads resources), stripped of navigation and active content,
// and given a CSP before it is placed in an opaque sandboxed frame.

export const htmlPreviewPolicy =
  "default-src 'none'; style-src 'unsafe-inline'; img-src data:; font-src data:";

// `link` covers stylesheets, preloads and prefetches: the CSP would block
// them, but the browser would still attempt the request. `template` content
// is invisible to this walk, yet `shadowrootmode` turns it live in the frame.
const removedElements =
  'script, base, link, template, meta[http-equiv="refresh" i], object, embed, iframe, frame, frameset, portal';
// SVG animation can write an `href` back at render time without script.
// SVG names are case-sensitive, so they are compared in lower case.
const animationElements: Record<string, true> = {
  animate: true,
  animatemotion: true,
  animatetransform: true,
  set: true,
};
// Attributes that make the browser fetch a resource; only inline `data:`
// values survive, so nothing reaches the network or the app origin.
// `srcset` is a candidate list a single URL check cannot classify, so it is
// always dropped and the element falls back to its `src`.
const resourceAttributes: Record<string, true> = {
  src: true,
  poster: true,
  background: true,
  href: true,
  "xlink:href": true,
};

// Uses the URL parser itself, which ignores leading controls and embedded
// tabs or newlines that a pattern would have to imitate, and keeps other
// leading characters, which leave a relative URL.
function urlProtocol(value: string) {
  try {
    return new URL(value, "https://preview.invalid/").protocol;
  } catch {
    return "";
  }
}

export function staticHtml(text: string) {
  const doc = new DOMParser().parseFromString(text, "text/html");
  for (const element of doc.querySelectorAll(removedElements)) element.remove();
  for (const element of doc.querySelectorAll("*")) {
    if (animationElements[element.localName.toLowerCase()]) {
      element.remove();
      continue;
    }
    const link = element.localName === "a" || element.localName === "area";
    // A snapshot of names: removal mutates the live attribute map.
    for (const name of element.getAttributeNames()) {
      const value = element.getAttribute(name) ?? "";
      const key = name.toLowerCase();
      const protocol = urlProtocol(value);
      // Link targets and form submissions are dropped whatever their value.
      if (
        key.startsWith("on") ||
        key === "action" ||
        key === "formaction" ||
        key === "srcset" ||
        (link && (key === "href" || key === "xlink:href")) ||
        (resourceAttributes[key] && protocol !== "data:") ||
        protocol === "javascript:"
      )
        element.removeAttribute(name);
    }
  }
  const meta = doc.createElement("meta");
  meta.httpEquiv = "Content-Security-Policy";
  meta.content = htmlPreviewPolicy;
  doc.head.prepend(meta);
  return `<!doctype html>\n${doc.documentElement.outerHTML}`;
}
