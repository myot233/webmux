import { vi } from "vitest";
import {
  defaultForwardOptions,
  receiveSessionFrame,
} from "../../src/domains/workspace/forwards.svelte";
import type {
  Forward,
  ListeningPort,
  SessionInfo,
} from "../../src/contracts/schemas";

export function forward(port: number, fields: Partial<Forward> = {}): Forward {
  return {
    port,
    path: `/forward/${port}/`,
    label: "",
    session: "",
    process: "",
    options: { ...defaultForwardOptions },
    status: "listening",
    createdAt: "2026-11-05T10:00:00Z",
    ...fields,
  };
}

export function session(
  id: string,
  title: string,
  listeningPorts: ListeningPort[],
): SessionInfo {
  return {
    id,
    generation: "0123456789abcdef",
    title,
    cwd: "/work",
    cmd: ["sh"],
    status: "running",
    exitCode: null,
    listeningPorts,
    state: "active",
    views: ["terminal"],
    view: "terminal",
    transition: null,
    agent: null,
    preview: "",
    createdAt: "2026-11-05T09:00:00Z",
    lastActiveAt: "2026-11-05T09:00:01Z",
    cols: 80,
    rows: 24,
    clients: 1,
    workspace: null,
    attention: null,
    reportedCwd: null,
  };
}

/** Replaces the live forward list as a `forwards` event would. */
export function seedForwards(forwards: Forward[]) {
  receiveSessionFrame(JSON.stringify({ type: "forwards", forwards }));
}

/** Answers every fetch with `respond` and records the calls. */
export function stubFetch(
  respond: (url: string, init?: RequestInit) => Response | Promise<Response>,
) {
  const fetch = vi.fn<typeof globalThis.fetch>(async (input, init) =>
    respond(String(input), init),
  );
  vi.stubGlobal("fetch", fetch);
  return fetch;
}

export function body(init?: RequestInit): unknown {
  return JSON.parse(String(init?.body));
}
