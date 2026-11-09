import type { Page } from "@playwright/test";

// Check geometry and text, not just scrollWidth: a clipped ancestor can hide
// overflow while leaving the document's dimensions unchanged.
export async function overflowViolations(page: Page): Promise<string[]> {
  return page.evaluate(() => {
    const width = document.documentElement.clientWidth;
    const problems = new Set<string>();
    if (document.documentElement.scrollWidth > width + 1)
      problems.add(
        `document width ${document.documentElement.scrollWidth} > ${width}`,
      );
    const intentionalText =
      'input, textarea, [data-slot="terminal-preview"], .truncate, .sr-only, [aria-hidden="true"]';
    function visible(element: Element) {
      if (element.closest('.sr-only, [aria-hidden="true"]')) return false;
      const style = getComputedStyle(element);
      return (
        style.visibility !== "hidden" &&
        style.display !== "none" &&
        element.getClientRects().length > 0
      );
    }
    function check(rect: DOMRect, description: string) {
      if (rect.left < -1 || rect.right > width + 1)
        problems.add(
          `${description}: x=${rect.left.toFixed(1)}..${rect.right.toFixed(1)} viewport=${width}`,
        );
    }
    for (const element of document.body.querySelectorAll("*")) {
      if (element instanceof HTMLElement && visible(element))
        check(
          element.getBoundingClientRect(),
          `${element.tagName.toLowerCase()}${element.getAttribute("data-slot") ? `[data-slot=${element.getAttribute("data-slot")}]` : ""}`,
        );
    }
    const walker = document.createTreeWalker(
      document.body,
      NodeFilter.SHOW_TEXT,
    );
    for (let node = walker.nextNode(); node; node = walker.nextNode()) {
      const parent = node.parentElement;
      if (
        !node.textContent?.trim() ||
        !parent ||
        !visible(parent) ||
        parent.closest(intentionalText)
      )
        continue;
      const range = document.createRange();
      range.selectNodeContents(node);
      const rect = range.getBoundingClientRect();
      if (!rect.width || !rect.height) continue;
      const description = `text ${JSON.stringify(node.textContent.slice(0, 50))}`;
      check(rect, description);
      for (
        let ancestor: HTMLElement | null = parent;
        ancestor;
        ancestor = ancestor.parentElement
      ) {
        if (
          !["hidden", "clip", "auto", "scroll"].includes(
            getComputedStyle(ancestor).overflowX,
          )
        )
          continue;
        const clip = ancestor.getBoundingClientRect();
        if (rect.left < clip.left - 1 || rect.right > clip.right + 1)
          problems.add(
            `${description} clipped by ${ancestor.tagName.toLowerCase()}`,
          );
      }
    }
    return [...problems];
  });
}
