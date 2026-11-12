import type { z } from "zod";
import { SvelteMap, SvelteSet } from "svelte/reactivity";
import {
  createForward,
  deleteForward,
  updateForward,
} from "../../platform/api";
import {
  SessionEventSchema,
  type Forward,
  type ForwardOptions,
  type ForwardRequest,
  type ForwardUpdate,
  type ListeningPort,
} from "../../contracts/schemas";
import { settings } from "../settings/settings.svelte";
import { hostOrigin } from "../../platform/transport";
import {
  connect,
  type Connection,
  type SocketConnection,
} from "../../platform/ws";

export type SessionEvent = z.infer<typeof SessionEventSchema>;

// The server's port forwards for this tab, replaced whole by the
// `/ws/sessions` snapshot and every `forwards` event; successful writes
// update it at once rather than waiting for the event. `ready` is false
// until a snapshot arrives on an open session-list connection, so the
// terminal page never prompts for a port that is already forwarded.
let list = $state.raw<Forward[]>([]);
let ready = $state(false);

export const forwards = {
  get list(): readonly Forward[] {
    return list;
  },
  get ready(): boolean {
    return ready;
  },
};

// Every compatibility rewrite on and the prefix stripped, so a server that
// only uses relative paths works unchanged; servers with absolute paths
// switch to keeping it and set their base to `/forward/{port}/`.
export const defaultForwardOptions: ForwardOptions = {
  rewriteHost: true,
  rewriteOrigin: true,
  forwardedHeaders: true,
  rewriteResponse: true,
  prefix: "strip",
};

function replace(next: readonly Forward[]) {
  list = [...next].sort((a, b) => a.port - b.port);
}
function upsert(forward: Forward) {
  replace([...list.filter((entry) => entry.port !== forward.port), forward]);
}

/**
 * Validates one `/ws/sessions` frame. Snapshot and `forwards` events replace
 * the forward list; an invalid frame throws before any state changes.
 */
export function receiveSessionFrame(data: string | ArrayBuffer): SessionEvent {
  if (typeof data !== "string") throw new Error("会话列表帧必须是 JSON 文本");
  const event = SessionEventSchema.parse(JSON.parse(data));
  if (event.type === "snapshot" || event.type === "forwards") {
    replace(event.forwards);
    ready = true;
  }
  return event;
}

/**
 * The session-list connection. Session events go to `onEvent`; forwards stay
 * in this module. Closing it marks the list stale until the next snapshot.
 */
export function connectSessions(
  onEvent: (event: SessionEvent) => void,
  onState: (state: Connection) => void,
  onError: (message: string) => void,
): SocketConnection {
  const socket = connect(
    "/ws/sessions",
    (data) => onEvent(receiveSessionFrame(data)),
    onState,
    onError,
  );
  return {
    ...socket,
    close() {
      socket.close();
      ready = false;
    },
  };
}

export async function addForward(request: ForwardRequest): Promise<Forward> {
  const forward = await createForward(request);
  upsert(forward);
  return forward;
}
export async function changeForward(
  port: number,
  update: ForwardUpdate,
): Promise<Forward> {
  const forward = await updateForward(port, update);
  upsert(forward);
  return forward;
}
export async function removeForward(port: number): Promise<void> {
  await deleteForward(port);
  list = list.filter((entry) => entry.port !== port);
}

/** The absolute address of a forward path, for sharing. */
export function forwardLink(path: string): string {
  return new URL(path, hostOrigin()).href;
}

/** Ports from 30000 up are usually ephemeral RPC or helper sockets. */
export const HIGH_PORT_MIN = 30000;

/** Whether automatic port prompts and detected lists offer `port`. */
export function isPromptedPort(port: number): boolean {
  return port < HIGH_PORT_MIN || settings.values.ignoreHighPorts === false;
}

// Ports dismissed with "忽略" on the terminal page, per session ID. They live
// only in this tab: nothing is stored or synced. A port leaves the set once
// its session stops listening on it, so a restarted server prompts again.
const ignored = new SvelteMap<string, SvelteSet<number>>();

export function ignorePort(session: string, port: number): void {
  const ports = ignored.get(session);
  if (ports) ports.add(port);
  else ignored.set(session, new SvelteSet([port]));
}
export function isIgnored(session: string, port: number): boolean {
  return ignored.get(session)?.has(port) ?? false;
}
/** Forgets ignored ports the session no longer listens on. */
export function pruneIgnored(
  session: string,
  listening: readonly ListeningPort[],
): void {
  const ports = ignored.get(session);
  if (!ports) return;
  for (const port of ports)
    if (!listening.some((entry) => entry.port === port)) ports.delete(port);
  if (!ports.size) ignored.delete(session);
}
