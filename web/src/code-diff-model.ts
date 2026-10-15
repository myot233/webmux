import { z } from "zod";

export const CodeLineSchema = z.object({
  kind: z.enum(["context", "add", "delete", "marker"]),
  oldLine: z.number().int().min(0).max(20000),
  newLine: z.number().int().min(0).max(20000),
  text: z.string(),
});
export type CodeLine = z.infer<typeof CodeLineSchema>;
export type CodeBlock = {
  kind: "gap" | "lines";
  start: number;
  end: number;
  count: number;
  gapId: string;
  lines: CodeLine[];
};
export const DiffResultSchema = z.discriminatedUnion("kind", [
  z.object({
    kind: z.literal("ready"),
    lines: z.array(CodeLineSchema).max(40002),
  }),
  z.object({ kind: z.literal("limited"), reason: z.string() }),
]);
export type DiffResult = z.infer<typeof DiffResultSchema>;
export const DiffReplySchema = z.object({
  snapshotId: z.string(),
  result: DiffResultSchema,
});

function block(
  all: CodeLine[],
  start: number,
  end: number,
  gap = false,
): CodeBlock {
  return {
    kind: gap ? "gap" : "lines",
    start,
    end,
    count: end - start,
    gapId: gap ? `${start}:${end}` : "",
    lines: gap ? [] : all.slice(start, end),
  };
}

export function collapseDiff(all: CodeLine[]): CodeBlock[] {
  const result: CodeBlock[] = [];
  for (let i = 0; i < all.length;) {
    const context = all[i].kind === "context";
    let end = i + 1;
    while (end < all.length && (all[end].kind === "context") === context) end++;
    const from = i === 0 ? i : i + 3;
    const to = end === all.length ? end : end - 3;
    if (context && to > from) {
      if (from > i) result.push(block(all, i, from));
      result.push(block(all, from, to, true));
      if (to < end) result.push(block(all, to, end));
    } else result.push(block(all, i, end));
    i = end;
  }
  return result;
}

export function expandDiff(
  all: CodeLine[],
  blocks: CodeBlock[],
  gapId: string,
  direction: "up" | "down" | "all",
): CodeBlock[] {
  if (gapId === "all") return all.length ? [block(all, 0, all.length)] : [];
  return blocks.flatMap((gap) => {
    if (gap.kind !== "gap" || gap.gapId !== gapId) return [gap];
    const from =
      direction === "up" ? Math.max(gap.start, gap.end - 30) : gap.start;
    const to =
      direction === "down" ? Math.min(gap.end, gap.start + 30) : gap.end;
    const result: CodeBlock[] = [];
    if (from > gap.start) result.push(block(all, gap.start, from, true));
    result.push(block(all, from, to));
    if (to < gap.end) result.push(block(all, to, gap.end, true));
    return result;
  });
}
