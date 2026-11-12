// Pure binary stream framing and network locality predicates for P2P DataChannels.

const STREAM_TEXT_FINAL = 0x01;
const STREAM_TEXT_CONT = 0x02;
const STREAM_BIN_FINAL = 0x03;
const STREAM_BIN_CONT = 0x04;
const STREAM_CLOSE = 0x05;

export const P2P_CHUNK_BYTES = 32 * 1024;
const P2P_MAX_STREAM_MESSAGE_BYTES = 16 * 1024 * 1024;

const utf8Encoder = new TextEncoder();
const utf8Decoder = new TextDecoder();

export function encodeStreamFrames(
  data: string | Uint8Array<ArrayBuffer>,
  chunkBytes = P2P_CHUNK_BYTES,
): Uint8Array<ArrayBuffer>[] {
  const isText = typeof data === "string";
  const raw = isText ? utf8Encoder.encode(data) : data;
  const finalTag = isText ? STREAM_TEXT_FINAL : STREAM_BIN_FINAL;
  const contTag = isText ? STREAM_TEXT_CONT : STREAM_BIN_CONT;

  if (raw.byteLength === 0) {
    const out = new Uint8Array(1);
    out[0] = finalTag;
    return [out];
  }

  const frames: Uint8Array<ArrayBuffer>[] = [];
  for (let off = 0; off < raw.byteLength; off += chunkBytes) {
    const end = Math.min(off + chunkBytes, raw.byteLength);
    const slice = raw.subarray(off, end);
    const frame = new Uint8Array(1 + slice.byteLength);
    frame[0] = end === raw.byteLength ? finalTag : contTag;
    frame.set(slice, 1);
    frames.push(frame);
  }
  return frames;
}

export function encodeStreamCloseFrame(
  code: number,
  reason = "",
): Uint8Array<ArrayBuffer> {
  const reasonBytes = utf8Encoder.encode(reason).subarray(0, 123);
  const frame = new Uint8Array(3 + reasonBytes.byteLength);
  frame[0] = STREAM_CLOSE;
  frame[1] = (code >>> 8) & 0xff;
  frame[2] = code & 0xff;
  frame.set(reasonBytes, 3);
  return frame;
}

export type DecodedStreamEvent =
  | { kind: "message"; data: string | ArrayBuffer }
  | { kind: "close"; code: number; reason: string }
  | { kind: "error"; message: string };

export class StreamFrameReassembler {
  private pendingIsText: boolean | null = null;
  private chunks: Uint8Array[] = [];
  private totalBytes = 0;

  constructor(private readonly maxBytes = P2P_MAX_STREAM_MESSAGE_BYTES) {}

  feed(raw: ArrayBuffer | Uint8Array): DecodedStreamEvent | null {
    const bytes = raw instanceof Uint8Array ? raw : new Uint8Array(raw);
    if (bytes.byteLength === 0) return null;

    const tag = bytes[0];
    const body = bytes.subarray(1);

    if (tag === STREAM_CLOSE) {
      if (body.byteLength < 2) {
        return { kind: "close", code: 1005, reason: "" };
      }
      const code = (body[0]! << 8) | body[1]!;
      const reason = utf8Decoder.decode(body.subarray(2));
      return { kind: "close", code, reason };
    }

    if (
      tag !== STREAM_TEXT_FINAL &&
      tag !== STREAM_TEXT_CONT &&
      tag !== STREAM_BIN_FINAL &&
      tag !== STREAM_BIN_CONT
    ) {
      return { kind: "error", message: `Unknown P2P stream tag ${tag}` };
    }

    const isText = tag === STREAM_TEXT_FINAL || tag === STREAM_TEXT_CONT;
    const isFinal = tag === STREAM_TEXT_FINAL || tag === STREAM_BIN_FINAL;

    if (this.pendingIsText !== null && this.pendingIsText !== isText) {
      this.reset();
      return { kind: "error", message: "Interleaved P2P stream fragments" };
    }

    if (this.totalBytes + body.byteLength > this.maxBytes) {
      this.reset();
      return { kind: "error", message: "P2P stream message exceeds limit" };
    }

    if (!isFinal) {
      this.pendingIsText = isText;
      const copy = new Uint8Array(body.byteLength);
      copy.set(body);
      this.chunks.push(copy);
      this.totalBytes += copy.byteLength;
      return null;
    }

    let assembled: Uint8Array<ArrayBuffer>;
    if (this.chunks.length === 0) {
      assembled = new Uint8Array(body.byteLength);
      assembled.set(body);
    } else {
      assembled = new Uint8Array(this.totalBytes + body.byteLength);
      let offset = 0;
      for (const chunk of this.chunks) {
        assembled.set(chunk, offset);
        offset += chunk.byteLength;
      }
      assembled.set(body, offset);
    }
    this.reset();

    if (isText) {
      return { kind: "message", data: utf8Decoder.decode(assembled) };
    }
    return { kind: "message", data: assembled.buffer };
  }

  private reset() {
    this.pendingIsText = null;
    this.chunks = [];
    this.totalBytes = 0;
  }
}

/**
 * Returns true when hostname is loopback, RFC 1918 private LAN, link-local,
 * IPv6 ULA, or Tailscale MagicDNS / CGNAT (100.64.0.0/10) where WebMux is
 * already direct and P2P ICE adds zero benefit.
 */
export function isLocalOrOverlayHost(hostname: string): boolean {
  const clean = hostname
    .trim()
    .toLowerCase()
    .replace(/^\[|\]$/g, "");
  if (
    !clean ||
    clean === "localhost" ||
    clean.endsWith(".localhost") ||
    clean.endsWith(".local") ||
    clean.endsWith(".ts.net") ||
    clean === "::1" ||
    clean === "0:0:0:0:0:0:0:1"
  ) {
    return true;
  }

  // IPv6 ULA (fc00::/7) and link-local (fe80::/10)
  if (clean.includes(":")) {
    return (
      clean.startsWith("fc") ||
      clean.startsWith("fd") ||
      clean.startsWith("fe8") ||
      clean.startsWith("fe9") ||
      clean.startsWith("fea") ||
      clean.startsWith("feb")
    );
  }

  // IPv4 literal check
  const ipv4 = /^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})$/.exec(clean);
  if (!ipv4) return false;
  const a = Number(ipv4[1]);
  const b = Number(ipv4[2]);
  const c = Number(ipv4[3]);
  const d = Number(ipv4[4]);
  if ([a, b, c, d].some((octet) => octet < 0 || octet > 255)) return false;

  if (a === 127 || a === 10 || a === 0) return true;
  if (a === 192 && b === 168) return true;
  if (a === 172 && b >= 16 && b <= 31) return true;
  if (a === 169 && b === 254) return true;
  // Tailscale / CGNAT 100.64.0.0/10
  if (a === 100 && b >= 64 && b <= 127) return true;
  return false;
}

export function medianSample(samples: readonly number[]): number | null {
  if (samples.length === 0) return null;
  const sorted = [...samples].sort((x, y) => x - y);
  const mid = Math.floor(sorted.length / 2);
  return sorted.length % 2 === 1
    ? sorted[mid]!
    : Math.round((sorted[mid - 1]! + sorted[mid]!) / 2);
}

export function shouldUpgradeToP2P(
  baselineRttMs: number,
  p2pRttMs: number,
): boolean {
  return (
    baselineRttMs >= 15 &&
    p2pRttMs <= baselineRttMs - 10 &&
    p2pRttMs <= baselineRttMs * 0.85
  );
}
