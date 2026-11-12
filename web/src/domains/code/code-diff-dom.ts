import type { CodeView, FileDiffMetadata } from "@pierre/diffs";
import type { CodeDiffReaderState } from "../../contracts/schemas";

export const diffExpansionLines = 30;

// The rendered diff instance, its expandHunk wrapped to record expansions
// exactly as the library's renderer counts them.
export function getDiffInstance(
  viewer: CodeView | undefined,
  recorded: WeakSet<object>,
  expanded: Map<number, { fromStart: number; fromEnd: number }>,
) {
  const item = viewer?.getRenderedItems()[0];
  if (item?.type !== "diff") return;
  const { instance } = item;
  if (!recorded.has(instance)) {
    recorded.add(instance);
    const expand = instance.expandHunk;
    instance.expandHunk = (index, direction, count) => {
      // The schema needs finite counts; this exceeds any region.
      const lines = Math.min(count ?? diffExpansionLines, 1e9);
      const region = expanded.get(index) ?? { fromStart: 0, fromEnd: 0 };
      if (direction !== "down")
        region.fromStart = Math.min(region.fromStart + lines, 1e9);
      if (direction !== "up")
        region.fromEnd = Math.min(region.fromEnd + lines, 1e9);
      expanded.set(index, region);
      expand(index, direction, count);
    };
  }
  return instance;
}

// The first source row at least partly below the top edge.
export function measureDiffTopLine(
  viewer: CodeView | undefined,
  host: HTMLElement,
  knownTop: CodeDiffReaderState["top"],
): CodeDiffReaderState["top"] {
  if (host.clientHeight === 0) return knownTop;
  const item = viewer?.getRenderedItems()[0];
  const edge = host.getBoundingClientRect().top;
  for (const row of item?.element.shadowRoot?.querySelectorAll<HTMLElement>(
    "[data-line]",
  ) ?? []) {
    const line = Number(row.dataset.line);
    if (!Number.isInteger(line) || line < 1) continue;
    if (row.getBoundingClientRect().bottom <= edge + 0.5) continue;
    // Split columns name their side; unified rows only by their type.
    const deletions = row.closest("[data-deletions]")
      ? true
      : row.closest("[data-additions]")
        ? false
        : (row.dataset.lineType ?? "").includes("deletion");
    return { line, side: deletions ? "deletions" : "additions" };
  }
  return knownTop;
}

// The library's expand controls are role=button divs. Supply names and
// keyboard handling here, and enforce the frozen-snapshot policy.
export function enhanceDiffControls(
  viewer: CodeView | undefined,
  expired: boolean,
) {
  for (const item of viewer?.getRenderedItems() ?? []) {
    for (const button of item.element.shadowRoot?.querySelectorAll<HTMLElement>(
      "[data-expand-button]",
    ) ?? []) {
      const all =
        button.hasAttribute("data-expand-all-button") ||
        button.hasAttribute("data-expand-both");
      button.setAttribute(
        "aria-label",
        all
          ? "展开全部上下文"
          : button.hasAttribute("data-expand-up")
            ? `向上展开 ${diffExpansionLines} 行`
            : `向下展开 ${diffExpansionLines} 行`,
      );
      button.setAttribute("aria-disabled", String(expired));
      button.tabIndex = expired ? -1 : 0;
      if (button.hasAttribute("data-expand-all-button"))
        button.textContent = "展开全部";
    }
    for (const label of item.element.shadowRoot?.querySelectorAll<HTMLElement>(
      "[data-unmodified-lines]",
    ) ?? []) {
      const count = label.textContent?.match(/\d+/)?.[0];
      if (count) label.textContent = `${count} 行未修改`;
    }
  }
}

export function findDiffExpandTarget(event: Event) {
  return event
    .composedPath()
    .find(
      (node): node is HTMLElement =>
        node instanceof HTMLElement &&
        (node.hasAttribute("data-expand-button") ||
          node.hasAttribute("data-unmodified-lines")),
    );
}

export function parseDiffGotoLine(
  value: string,
  current: number,
  totalLines: number,
): number | undefined {
  const match = /^([+-])?(\d+)?(:\d+)?(%)?$/.exec(value.trim());
  if (!match) return undefined;
  const [, sign, digits, , percent] = match;
  let target = digits ? Number(digits) : current;
  if (digits && percent) {
    target =
      Math.round((totalLines * target) / 100) * (sign === "-" ? -1 : 1) +
      (sign ? current : 0);
  } else if (digits && sign)
    target = current + target * (sign === "-" ? -1 : 1);
  return target;
}

export function findUnexpandedDiffRegion(
  fileDiff: FileDiffMetadata,
  side: "old" | "new",
  number: number,
): number | undefined {
  const startOf = (hunk: FileDiffMetadata["hunks"][number]) =>
    side === "old" ? hunk.deletionStart : hunk.additionStart;
  let region = fileDiff.hunks.findIndex((hunk) => number < startOf(hunk));
  if (region < 0) region = fileDiff.hunks.length;
  const previous = fileDiff.hunks[region - 1];
  const previousEnd = previous
    ? startOf(previous) +
      (side === "old" ? previous.deletionCount : previous.additionCount) -
      1
    : 0;
  return number > previousEnd ? region : undefined;
}
