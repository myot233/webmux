import type {
  ChatContent,
  ChatEvent,
  ChatFrame,
  ChatOption,
  ChatRequest,
  StopReason,
  ToolStatus,
} from "./schemas";

// Pure reducer from the chat channel's frames to the timeline the chat view
// renders. Non-timeline state (plan, config, queue, requests...) arrives as
// whole `state` objects and is not tracked here.

type MessageEntry = {
  type: "message";
  id: string;
  role: "user" | "agent" | "thought";
  text: string;
  truncated: boolean;
};
export type ToolEntry = {
  type: "tool";
  id: string;
  kind: string;
  title: string;
  status: ToolStatus;
  content: ChatContent[];
  locations: { path: string; line?: number }[];
  /** Created by a patch whose opening event fell out of the cached log. */
  partial: boolean;
  /**
   * Outcome of the permission request attached to this call, if answered;
   * the chosen option's name and kind when the request was seen.
   */
  permission?: {
    outcome: string;
    optionId?: string;
    name?: string;
    kind?: ChatOption["kind"];
  };
};
export type TimelineEntry =
  | MessageEntry
  | ToolEntry
  | {
      type: "compaction";
      id: string;
      status: "running" | "completed" | "failed";
      summary: string;
    }
  | {
      type: "notice";
      id: string;
      level: "info" | "warning" | "error";
      text: string;
    }
  | { type: "turnEnd"; id: string; stopReason: StopReason; error?: string }
  | {
      type: "marker";
      id: string;
      kind: "loaded" | "switched" | "truncated" | "reconnected" | "resumed";
      side?: string;
    };

export type ChatModel = {
  generation: string;
  /** Sequence expected next; events below it were already applied. */
  next: number;
  /** Older events were dropped by the server's byte limit. */
  truncated: boolean;
  title?: string;
  entries: TimelineEntry[];
  /** Entry position by "type:id", for upserts. */
  index: ReadonlyMap<string, number>;
  /** Tool call and options of each permission request on a tool call. */
  requestTools: ReadonlyMap<
    string,
    { toolCallId: string; options: readonly ChatOption[] }
  >;
};

export function emptyChat(): ChatModel {
  return {
    generation: "",
    next: 0,
    truncated: false,
    entries: [],
    index: new Map(),
    requestTools: new Map(),
  };
}

/**
 * Where to resume the stream: the generation and the last applied sequence,
 * after which the server resends events.
 */
export function resumePoint(model: ChatModel) {
  return model.generation
    ? { generation: model.generation, seq: Math.max(0, model.next - 1) }
    : undefined;
}

export function applyFrame(model: ChatModel, frame: ChatFrame): ChatModel {
  switch (frame.type) {
    case "snapshot": {
      const generation = frame.generation ?? "";
      const keep = !frame.reset && generation === model.generation;
      const base = keep ? model : { ...emptyChat(), generation };
      return {
        ...base,
        next: keep ? model.next : (frame.from ?? 0),
        truncated: !!frame.truncated,
      };
    }
    case "events": {
      if (frame.generation && frame.generation !== model.generation)
        return model;
      let next = model;
      for (const event of frame.items ?? []) next = applyEvent(next, event);
      return next;
    }
    case "state":
      return model;
  }
}

/** Applies one event; an event at or below the applied sequence is ignored. */
function applyEvent(model: ChatModel, event: ChatEvent): ChatModel {
  if (event.seq < model.next) return model;
  const next = { ...model, next: event.seq + 1 };
  switch (event.kind) {
    case "message.append": {
      const { messageId, role, text, truncated } = event.message;
      return upsert(next, `message:${messageId}`, (prev) =>
        prev?.type === "message"
          ? {
              ...prev,
              text: prev.text + text,
              truncated: prev.truncated || !!truncated,
            }
          : {
              type: "message",
              id: messageId,
              role,
              text,
              truncated: !!truncated,
            },
      );
    }
    case "tool.upsert": {
      const patch = event.tool;
      return upsert(next, `tool:${patch.toolCallId}`, (prev) => {
        const base: ToolEntry =
          prev?.type === "tool"
            ? prev
            : {
                type: "tool",
                id: patch.toolCallId,
                kind: "other",
                title: "",
                status: "pending",
                content: [],
                locations: [],
                partial: patch.title === undefined,
              };
        return {
          ...base,
          kind: patch.kind ?? base.kind,
          title: patch.title ?? base.title,
          status: patch.status ?? base.status,
          content: patch.content ?? base.content,
          locations: patch.locations ?? base.locations,
        };
      });
    }
    case "info.set":
      return event.info.title === undefined
        ? next
        : { ...next, title: event.info.title };
    case "compaction.upsert": {
      const { id, status, summary } = event.compaction;
      return upsert(next, `compaction:${id}`, (prev) => ({
        type: "compaction",
        id,
        status,
        summary: summary ?? (prev?.type === "compaction" ? prev.summary : ""),
      }));
    }
    case "notice":
      return append(next, {
        type: "notice",
        id: String(event.seq),
        level: event.notice.level,
        text: event.notice.text,
      });
    case "turn.end":
      return append(next, {
        type: "turnEnd",
        id: event.turnEnd.turnId,
        stopReason: event.turnEnd.stopReason,
        ...(event.turnEnd.error ? { error: event.turnEnd.error } : {}),
      });
    case "marker":
      return append(next, {
        type: "marker",
        id: String(event.seq),
        kind: event.marker.kind,
        ...(event.marker.side ? { side: event.marker.side } : {}),
      });
    case "request.open":
      return trackRequest(next, event.request);
    case "request.close": {
      const { requestId, outcome, optionId } = event.resolved;
      const request = next.requestTools.get(requestId);
      const at = request
        ? next.index.get(`tool:${request.toolCallId}`)
        : undefined;
      const tool = at === undefined ? undefined : next.entries[at];
      if (tool?.type !== "tool") return next;
      const option = request?.options.find((o) => o.optionId === optionId);
      return replaceAt(next, at!, {
        ...tool,
        permission: {
          outcome,
          ...(optionId ? { optionId } : {}),
          ...(option ? { name: option.name, kind: option.kind } : {}),
        },
      });
    }
    case "turn.start":
    case "plan.set":
    case "commands.set":
    case "config.set":
    case "usage.set":
      // State frames carry these; the timeline does not show them.
      return next;
  }
}

function trackRequest(model: ChatModel, request: ChatRequest): ChatModel {
  if (!request.toolCallId) return model;
  const requestTools = new Map(model.requestTools);
  requestTools.set(request.requestId, {
    toolCallId: request.toolCallId,
    options: request.options ?? [],
  });
  return { ...model, requestTools };
}

function upsert(
  model: ChatModel,
  key: string,
  build: (prev: TimelineEntry | undefined) => TimelineEntry,
): ChatModel {
  const at = model.index.get(key);
  if (at !== undefined) return replaceAt(model, at, build(model.entries[at]));
  const index = new Map(model.index);
  index.set(key, model.entries.length);
  return { ...model, entries: [...model.entries, build(undefined)], index };
}

function append(model: ChatModel, entry: TimelineEntry): ChatModel {
  return { ...model, entries: [...model.entries, entry] };
}

function replaceAt(model: ChatModel, at: number, entry: TimelineEntry) {
  const entries = model.entries.slice();
  entries[at] = entry;
  return { ...model, entries };
}

export type DisplayEntry =
  | TimelineEntry
  | { type: "reads"; id: string; calls: ToolEntry[] };

const browsingKinds: Record<string, true> = {
  read: true,
  search: true,
  fetch: true,
};

/**
 * Collapses consecutive read-only tool calls (read, search, fetch) into one
 * "browsed N items" row. A single read stays a normal tool row.
 */
export function groupTimeline(entries: readonly TimelineEntry[]) {
  const out: DisplayEntry[] = [];
  let run: ToolEntry[] = [];
  const flush = () => {
    if (run.length > 1) out.push({ type: "reads", id: run[0]!.id, calls: run });
    else out.push(...run);
    run = [];
  };
  for (const entry of entries) {
    if (
      entry.type === "tool" &&
      browsingKinds[entry.kind] &&
      !entry.permission
    ) {
      run.push(entry);
      continue;
    }
    flush();
    out.push(entry);
  }
  flush();
  return out;
}

const DIFF_CELL_LIMIT = 4_000_000;

/** Added and removed line counts of a diff content block. */
export function diffStats(content: ChatContent) {
  const before = content.oldText ? content.oldText.split("\n") : [];
  const after = content.newText ? content.newText.split("\n") : [];
  let start = 0;
  while (
    start < before.length &&
    start < after.length &&
    before[start] === after[start]
  )
    start++;
  let endBefore = before.length;
  let endAfter = after.length;
  while (
    endBefore > start &&
    endAfter > start &&
    before[endBefore - 1] === after[endAfter - 1]
  ) {
    endBefore--;
    endAfter--;
  }
  const a = before.slice(start, endBefore);
  const b = after.slice(start, endAfter);
  if (a.length * b.length > DIFF_CELL_LIMIT)
    return { added: b.length, removed: a.length };
  const common = lcsLength(a, b);
  return { added: b.length - common, removed: a.length - common };
}

function lcsLength(a: readonly string[], b: readonly string[]) {
  let prev = new Uint32Array(b.length + 1);
  for (const line of a) {
    const row = new Uint32Array(b.length + 1);
    for (let j = 1; j <= b.length; j++)
      row[j] =
        line === b[j - 1] ? prev[j - 1]! + 1 : Math.max(prev[j]!, row[j - 1]!);
    prev = row;
  }
  return prev[b.length]!;
}
