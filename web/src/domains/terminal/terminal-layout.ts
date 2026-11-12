import { z } from "zod";
import { api } from "../../platform/api";
import {
  CreateSessionRequestSchema,
  CreateSessionResultSchema,
  SessionInfoSchema,
  type SessionInfo,
} from "../../contracts/schemas";

export async function createSplitShellSession(source: SessionInfo) {
  const { session } = await api(
    "/sessions",
    CreateSessionResultSchema,
    "POST",
    {
      cmd: ["/bin/sh", "-i"],
      cwd: source.cwd,
      ...(source.workspace
        ? {
            workspace: {
              repositoryId: source.workspace.repositoryId,
              worktreeId: source.workspace.worktreeId,
            },
          }
        : {}),
    },
    undefined,
    CreateSessionRequestSchema,
  );
  return session;
}

export type PrefixCommand =
  | { kind: "ignore" }
  | { kind: "enter-prefix" }
  | { kind: "send-prefix" }
  | { kind: "split"; direction: SplitDirection }
  | { kind: "cycle" }
  | { kind: "zoom" }
  | { kind: "close" }
  | { kind: "arrow"; key: string }
  | { kind: "cancel" };

export function parsePrefixKeydown(
  event: KeyboardEvent,
  prefix: boolean,
): PrefixCommand {
  if (
    event.isComposing ||
    event.keyCode === 229 ||
    !(event.target instanceof Element) ||
    !event.target.closest(".terminal-host")
  )
    return { kind: "ignore" };
  // Shift is pressed before % and "; it must not consume the prefix.
  if (["Shift", "Control", "Alt", "Meta"].includes(event.key))
    return { kind: "ignore" };
  const isPrefix =
    event.ctrlKey &&
    !event.altKey &&
    !event.metaKey &&
    event.key.toLowerCase() === "b";
  if (!prefix && !isPrefix) return { kind: "ignore" };
  if (!prefix) return { kind: "enter-prefix" };
  if (isPrefix) return { kind: "send-prefix" };
  if (event.key === "%" || (event.shiftKey && event.code === "Digit5"))
    return { kind: "split", direction: "horizontal" };
  if (event.key === '"' || (event.shiftKey && event.code === "Quote"))
    return { kind: "split", direction: "vertical" };
  if (event.key === "o") return { kind: "cycle" };
  if (event.key === "z") return { kind: "zoom" };
  if (event.key === "x") return { kind: "close" };
  if (event.key.startsWith("Arrow")) return { kind: "arrow", key: event.key };
  return { kind: "cancel" };
}

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
/** The visible line; the pane divider widens its hit area around it. */
const DIVIDER_SIZE = 1;
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
    const gap = Math.min(DIVIDER_SIZE, horizontal ? rect.width : rect.height);
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

export function directionalPane(
  panes: (PaneRect & { sessionId: string })[],
  active: string,
  key: string,
): string | undefined {
  const current = panes.find((pane) => pane.sessionId === active);
  if (!current) return undefined;
  const cx = current.x + current.width / 2;
  const cy = current.y + current.height / 2;
  const dx = key === "ArrowLeft" ? -1 : key === "ArrowRight" ? 1 : 0;
  const dy = key === "ArrowUp" ? -1 : key === "ArrowDown" ? 1 : 0;
  const candidates = panes
    .filter((pane) => pane.sessionId !== active)
    .map((pane) => {
      const x = pane.x + pane.width / 2 - cx;
      const y = pane.y + pane.height / 2 - cy;
      return { pane, forward: x * dx + y * dy, distance: Math.hypot(x, y) };
    })
    .filter((candidate) => candidate.forward > 0)
    .sort((a, b) => a.distance - b.distance);
  return candidates[0]?.pane.sessionId;
}

export function pointerSplitRatio(
  event: PointerEvent,
  workspaceBounds: DOMRect,
  divider: Divider,
): number | undefined {
  const horizontal = divider.direction === "horizontal";
  const position = horizontal
    ? event.clientX - workspaceBounds.x - divider.parent.x
    : event.clientY - workspaceBounds.y - divider.parent.y;
  const extent =
    (horizontal ? divider.parent.width : divider.parent.height) - DIVIDER_SIZE;
  return extent > 0 ? (position - DIVIDER_SIZE / 2) / extent : undefined;
}

export function keyboardSplitRatio(
  key: string,
  divider: Divider,
): number | undefined {
  const horizontal = divider.direction === "horizontal";
  const decrease = horizontal ? "ArrowLeft" : "ArrowUp";
  const increase = horizontal ? "ArrowRight" : "ArrowDown";
  if (![decrease, increase, "Home", "End", "Enter"].includes(key))
    return undefined;
  return key === "Home"
    ? 0.1
    : key === "End"
      ? 0.9
      : key === "Enter"
        ? 0.5
        : divider.ratio + (key === decrease ? -0.05 : 0.05);
}
