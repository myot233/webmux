import { z } from "zod";
import { SessionInfoSchema } from "./schemas";

export const MAX_PANES = 8;
export type SplitDirection = "horizontal" | "vertical";
export type PaneLayout =
  | { kind: "pane"; sessionId: string }
  | {
      kind: "split";
      id: string;
      direction: SplitDirection;
      ratio: number;
      first: PaneLayout;
      second: PaneLayout;
    };

const LayoutSchema: z.ZodType<PaneLayout> = z.lazy(() =>
  z.discriminatedUnion("kind", [
    z.object({
      kind: z.literal("pane"),
      sessionId: SessionInfoSchema.shape.id,
    }),
    z.object({
      kind: z.literal("split"),
      id: z.string().min(1).max(64),
      direction: z.enum(["horizontal", "vertical"]),
      ratio: z.number().min(0.1).max(0.9),
      first: LayoutSchema,
      second: LayoutSchema,
    }),
  ]),
);

export function paneIds(layout: PaneLayout): string[] {
  return layout.kind === "pane"
    ? [layout.sessionId]
    : [...paneIds(layout.first), ...paneIds(layout.second)];
}

export function restoreLayout(value: string, fallback: string) {
  try {
    // Bound recursion before parsing an untrusted local setting.
    if (value.length > 4096) throw new Error("Layout too large");
    const raw = JSON.parse(value);
    const bounded = (node: unknown, depth = 0): boolean => {
      if (!node || typeof node !== "object" || depth >= MAX_PANES) return false;
      const entry = node as Record<string, unknown>;
      return (
        entry.kind === "pane" ||
        (entry.kind === "split" &&
          bounded(entry.first, depth + 1) &&
          bounded(entry.second, depth + 1))
      );
    };
    if (!bounded(raw)) throw new Error("Invalid layout");
    const layout = LayoutSchema.parse(raw);
    const ids = paneIds(layout);
    const splits = layoutGeometry(layout, 1, 1).dividers.map((d) => d.id);
    if (
      ids.length > MAX_PANES ||
      new Set(ids).size !== ids.length ||
      new Set(splits).size !== splits.length
    )
      throw new Error("Duplicate or excessive panes");
    return layout;
  } catch {
    return { kind: "pane", sessionId: fallback } satisfies PaneLayout;
  }
}

export function splitPane(
  layout: PaneLayout,
  target: string,
  sessionId: string,
  direction: SplitDirection,
): PaneLayout {
  if (
    paneIds(layout).length >= MAX_PANES ||
    paneIds(layout).includes(sessionId)
  )
    return layout;
  if (layout.kind === "pane") {
    return layout.sessionId === target
      ? {
          kind: "split",
          // WebMux also runs over trusted HTTP, where randomUUID is absent.
          id: Array.from(crypto.getRandomValues(new Uint32Array(4)), (word) =>
            word.toString(16),
          ).join("-"),
          direction,
          ratio: 0.5,
          first: layout,
          second: { kind: "pane", sessionId },
        }
      : layout;
  }
  return {
    ...layout,
    first: splitPane(layout.first, target, sessionId, direction),
    second: splitPane(layout.second, target, sessionId, direction),
  };
}

export function removePane(
  layout: PaneLayout,
  sessionId: string,
): PaneLayout | undefined {
  if (layout.kind === "pane")
    return layout.sessionId === sessionId ? undefined : layout;
  const first = removePane(layout.first, sessionId);
  const second = removePane(layout.second, sessionId);
  return first && second ? { ...layout, first, second } : (first ?? second);
}

export function resizeSplit(
  layout: PaneLayout,
  id: string,
  ratio: number,
): PaneLayout {
  if (layout.kind === "pane") return layout;
  if (layout.id === id)
    return { ...layout, ratio: Math.max(0.1, Math.min(0.9, ratio)) };
  return {
    ...layout,
    first: resizeSplit(layout.first, id, ratio),
    second: resizeSplit(layout.second, id, ratio),
  };
}

export type PaneRect = { x: number; y: number; width: number; height: number };
export type Divider = PaneRect & {
  id: string;
  direction: SplitDirection;
  ratio: number;
  parent: PaneRect;
};

export function layoutGeometry(
  layout: PaneLayout,
  width: number,
  height: number,
) {
  const panes: (PaneRect & { sessionId: string })[] = [];
  const dividers: Divider[] = [];
  const visit = (node: PaneLayout, rect: PaneRect) => {
    if (node.kind === "pane") {
      panes.push({ ...rect, sessionId: node.sessionId });
      return;
    }
    const horizontal = node.direction === "horizontal";
    const gap = Math.min(6, horizontal ? rect.width : rect.height);
    const extent = Math.max(0, (horizontal ? rect.width : rect.height) - gap);
    const first = extent * node.ratio;
    dividers.push({
      ...rect,
      id: node.id,
      direction: node.direction,
      ratio: node.ratio,
      parent: rect,
      ...(horizontal
        ? { x: rect.x + first, width: gap }
        : { y: rect.y + first, height: gap }),
    });
    visit(node.first, {
      ...rect,
      ...(horizontal ? { width: first } : { height: first }),
    });
    visit(node.second, {
      ...rect,
      ...(horizontal
        ? { x: rect.x + first + gap, width: extent - first }
        : { y: rect.y + first + gap, height: extent - first }),
    });
  };
  visit(layout, {
    x: 0,
    y: 0,
    width: Math.max(0, width),
    height: Math.max(0, height),
  });
  return { panes, dividers };
}
