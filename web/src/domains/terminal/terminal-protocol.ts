// Pure helpers for the terminal WebSocket contract in docs/protocol/websocket.md.
import { randomHex } from "../../platform/ids";

const encoder = new TextEncoder();

export function newSubmissionId() {
  return "s-" + randomHex(16);
}

// Mirrors xterm's paste(): newlines become CR, and the text is wrapped in
// bracketed-paste markers while the application has enabled that mode.
export function preparePaste(text: string, bracketed: boolean) {
  const data = text.replace(/\r?\n/g, "\r");
  return bracketed ? `\x1b[200~${data}\x1b[201~` : data;
}

// A 0x03 payload: one JSON header line, then the body and submit key bytes.
// Terminal bytes stay raw, never JSON strings.
export function encodeSubmission(
  id: string,
  generation: string,
  body: string,
  submit: string,
) {
  const bodyBytes = encoder.encode(body);
  const header = encoder.encode(
    JSON.stringify({ id, generation, bodyBytes: bodyBytes.length }) + "\n",
  );
  const submitBytes = encoder.encode(submit);
  const payload = new Uint8Array(
    header.length + bodyBytes.length + submitBytes.length,
  );
  payload.set(header);
  payload.set(bodyBytes, header.length);
  payload.set(submitBytes, header.length + bodyBytes.length);
  return payload;
}

export function deviceLabel(userAgent: string, touchPoints = 0) {
  const device = /iPhone/.test(userAgent)
    ? "iPhone"
    : /iPad/.test(userAgent) || (/Macintosh/.test(userAgent) && touchPoints > 1)
      ? "iPad"
      : /Android/.test(userAgent)
        ? "Android"
        : /Macintosh|Mac OS X/.test(userAgent)
          ? "Mac"
          : /Windows/.test(userAgent)
            ? "Windows"
            : /Linux|X11/.test(userAgent)
              ? "Linux"
              : "浏览器";
  const browser = /EdgA?\//.test(userAgent)
    ? "Edge"
    : /Firefox\/|FxiOS\//.test(userAgent)
      ? "Firefox"
      : /Chrome\/|CriOS\//.test(userAgent)
        ? "Chrome"
        : /Safari\//.test(userAgent)
          ? "Safari"
          : "";
  return browser ? `${device} · ${browser}` : device;
}

const clientKey = "webmux.clientId";
const liveKey = "webmux.clientLive";

// Reads this tab's client ID from session storage. Duplicating a tab copies
// session storage, so the live marker, cleared only when a page goes away,
// tells a copy from a reload: a copy still sees its original's marker and
// takes a new ID. A tab restored after the system discarded it never cleared
// its marker either and also takes a new ID, which only costs it control.
export function resolveClientId(storage: Storage, generate: () => string) {
  let id = storage.getItem(clientKey);
  if (!id || !/^[A-Za-z0-9_-]{8,64}$/.test(id) || storage.getItem(liveKey)) {
    id = generate();
    storage.setItem(clientKey, id);
  }
  storage.setItem(liveKey, "1");
  return id;
}

let identity: string | undefined;
// One identity per tab: it survives reloads and reconnects of this tab, so
// control and submission acknowledgements follow it, while another tab on
// the same device, including a duplicated one, is a separate client.
export function clientIdentity() {
  if (!identity) {
    const generate = () => "c-" + randomHex(12);
    try {
      identity = resolveClientId(sessionStorage, generate);
      addEventListener("pagehide", () => {
        try {
          sessionStorage.removeItem(liveKey);
        } catch {
          /* storage became unavailable */
        }
      });
      // A page restored from the back-forward cache is live again.
      addEventListener("pageshow", (event) => {
        try {
          if (event.persisted) sessionStorage.setItem(liveKey, "1");
        } catch {
          /* storage became unavailable */
        }
      });
    } catch {
      identity = generate();
    }
  }
  return {
    id: identity,
    label: deviceLabel(navigator.userAgent, navigator.maxTouchPoints),
  };
}

export function encodeIdsQuery(ids: Iterable<string>) {
  return encoder.encode(JSON.stringify({ ids: [...ids] }));
}

export function encodeTerminalSize(cols: number, rows: number) {
  return encoder.encode(JSON.stringify({ cols, rows }));
}

export function formatTerminalSocketPath(
  id: string,
  client: { id: string; label: string },
  synced: boolean,
  generation: string | undefined,
  offset: number,
  compressed: boolean,
) {
  const params = new URLSearchParams({
    client: client.id,
    label: client.label,
  });
  if (synced && generation) {
    params.set("generation", generation);
    params.set("offset", String(offset));
  }
  if (!compressed) params.set("zreplay", "1");
  return `/ws/s/${id}?${params}`;
}

export type AppendOverlapDecision =
  | { accept: true; skipBytes: number; hasTail: boolean }
  | { accept: false };

// Evaluates a 0x04 append position frame against the client's already-rendered
// stream offset, trimming any overlap delivered during a make-before-break
// transport handoff.
export function resolveAppendOverlap(
  synced: boolean,
  currentGeneration: string | undefined,
  currentOffset: number,
  position: { generation: string; start: number; end: number },
): AppendOverlapDecision {
  if (
    !synced ||
    position.generation !== currentGeneration ||
    position.start > currentOffset ||
    currentOffset > position.end
  ) {
    return { accept: false };
  }
  return {
    accept: true,
    skipBytes: currentOffset - position.start,
    hasTail: position.end > currentOffset,
  };
}

export function trimOverlapChunk<T extends ArrayBufferLike>(
  payload: Uint8Array<T>,
  skipBytes: number,
): { remainingSkip: number; slice: Uint8Array<T> | null } {
  if (skipBytes <= 0) return { remainingSkip: 0, slice: payload };
  if (payload.byteLength <= skipBytes) {
    return { remainingSkip: skipBytes - payload.byteLength, slice: null };
  }
  return { remainingSkip: 0, slice: payload.subarray(skipBytes) };
}
