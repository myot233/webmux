import {
  KittyCommandError,
  type KittyDecodeInput,
  type KittyImage,
  maxEncodedBytes,
  maxImageBytes,
  maxImageDimension,
  maxPixels,
  optionalInteger,
} from "./kitty-stream";

const base64ChunkChars = 4 * 1024 * 1024;
type FromBase64 = (input: string) => Uint8Array;
const fromBase64 = (Uint8Array as unknown as { fromBase64?: FromBase64 })
  .fromBase64;

function yieldToMainThread() {
  return new Promise<void>((resolve) => setTimeout(resolve, 0));
}

function pngSize(bytes: Uint8Array) {
  if (bytes.length < 24) return undefined;
  const signature = [137, 80, 78, 71, 13, 10, 26, 10];
  if (signature.some((value, index) => bytes[index] !== value))
    return undefined;
  if (
    bytes[12] !== 73 ||
    bytes[13] !== 72 ||
    bytes[14] !== 68 ||
    bytes[15] !== 82
  )
    return undefined;
  const view = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength);
  const width = view.getUint32(16);
  const height = view.getUint32(20);
  return width > 0 && height > 0 ? { width, height } : undefined;
}

export async function decodeKittyImage({
  bytes,
  format,
  width,
  height,
}: KittyDecodeInput): Promise<KittyImage> {
  if (format === 100) {
    const bitmap = await createImageBitmap(
      new Blob([asArrayBuffer(bytes)], { type: "image/png" }),
    );
    return {
      width: bitmap.width,
      height: bitmap.height,
      source: bitmap,
      dispose: () => bitmap.close(),
    };
  }
  if ((format !== 24 && format !== 32) || !width || !height)
    throw new KittyCommandError("EINVAL", "invalid image dimensions");
  const channels = format === 24 ? 3 : 4;
  if (bytes.length !== width * height * channels)
    throw new KittyCommandError("EINVAL", "invalid pixel data length");
  const rgba = new Uint8ClampedArray(width * height * 4);
  for (let source = 0, target = 0; target < rgba.length;) {
    rgba[target++] = bytes[source++]!;
    rgba[target++] = bytes[source++]!;
    rgba[target++] = bytes[source++]!;
    rgba[target++] = channels === 3 ? 255 : bytes[source++]!;
  }
  const bitmap = await createImageBitmap(new ImageData(rgba, width, height));
  return {
    width,
    height,
    source: bitmap,
    dispose: () => bitmap.close(),
  };
}

function asArrayBuffer(bytes: Uint8Array) {
  if (bytes.byteOffset === 0 && bytes.byteLength === bytes.buffer.byteLength)
    return bytes.buffer as ArrayBuffer;
  return bytes.buffer.slice(
    bytes.byteOffset,
    bytes.byteOffset + bytes.byteLength,
  ) as ArrayBuffer;
}

function checkSourceDimensions(width: number, height: number) {
  if (
    !Number.isInteger(width) ||
    !Number.isInteger(height) ||
    width < 1 ||
    height < 1 ||
    width > maxImageDimension ||
    height > maxImageDimension ||
    width * height > maxPixels
  )
    throw new KittyCommandError(
      "EINVAL",
      "image dimensions exceed source limits",
    );
}

async function inflateKittyPayload(bytes: Uint8Array, current: () => boolean) {
  const reader = new Blob([asArrayBuffer(bytes)])
    .stream()
    .pipeThrough(new DecompressionStream("deflate"))
    .getReader();
  const chunks: Uint8Array[] = [];
  let total = 0;
  for (;;) {
    const { done, value } = await reader.read();
    if (!current()) {
      await reader.cancel();
      return;
    }
    if (done) break;
    total += value.length;
    if (total > maxImageBytes) {
      await reader.cancel();
      throw new KittyCommandError("EINVAL", "image is too large");
    }
    chunks.push(value);
  }
  const merged = new Uint8Array(total);
  let offset = 0;
  for (const chunk of chunks) {
    merged.set(chunk, offset);
    offset += chunk.length;
  }
  return merged;
}

async function decodeBase64Payload(payload: string) {
  if (payload.length > maxEncodedBytes)
    throw new KittyCommandError("EINVAL", "image is too large");
  if (/[\t\n\f\r ]/.test(payload))
    payload = payload.replace(/[\t\n\f\r ]/g, "");
  if (payload.length === 0)
    throw new KittyCommandError("EINVAL", "empty image payload");
  if (payload.length % 4 !== 0)
    throw new KittyCommandError("EINVAL", "invalid base64 payload");
  let padding = 0;
  if (payload.endsWith("==")) padding = 2;
  else if (payload.endsWith("=")) padding = 1;
  const paddingStart = payload.indexOf("=");
  if (paddingStart !== -1 && paddingStart < payload.length - padding)
    throw new KittyCommandError("EINVAL", "invalid base64 payload");
  const size = (payload.length / 4) * 3 - padding;
  if (size > maxImageBytes)
    throw new KittyCommandError("EINVAL", "image is too large");
  const bytes = new Uint8Array(size);
  let target = 0;
  for (let offset = 0; offset < payload.length; offset += base64ChunkChars) {
    const end = Math.min(payload.length, offset + base64ChunkChars);
    let part: Uint8Array;
    if (fromBase64) {
      try {
        part = fromBase64(payload.slice(offset, end));
      } catch {
        throw new KittyCommandError("EINVAL", "invalid base64 payload");
      }
      bytes.set(part, target);
      target += part.length;
    } else {
      let binary: string;
      try {
        binary = atob(payload.slice(offset, end));
      } catch {
        throw new KittyCommandError("EINVAL", "invalid base64 payload");
      }
      for (let index = 0; index < binary.length; index++)
        bytes[target++] = binary.charCodeAt(index);
    }
    if (end < payload.length) await yieldToMainThread();
  }
  return bytes;
}

function transferFormat(params: Record<string, string>) {
  if ((params.t ?? "d") !== "d")
    throw new KittyCommandError(
      "ENOTSUP",
      "only direct image transmission is supported",
    );
  const format = optionalInteger(params, "f", 0, 1_000_000, 32) ?? 32;
  if (format !== 24 && format !== 32 && format !== 100)
    throw new KittyCommandError("EINVAL", "unsupported image format");
  return format;
}

export async function inspectKittyImage(
  params: Record<string, string>,
  payload: string,
) {
  const format = transferFormat(params);
  const bytes = await decodeBase64Payload(payload);
  if (params.o !== undefined) {
    if (params.o !== "z")
      throw new KittyCommandError("ENOTSUP", "unsupported compression");
    return;
  }
  if (format === 100) {
    const size = pngSize(bytes);
    if (!size) throw new KittyCommandError("EINVAL", "invalid PNG data");
    checkSourceDimensions(size.width, size.height);
    return;
  }
  const width = optionalInteger(params, "s", 1, 1_000_000);
  const height = optionalInteger(params, "v", 1, 1_000_000);
  if (!width || !height)
    throw new KittyCommandError("EINVAL", "image dimensions are required");
  checkSourceDimensions(width, height);
  if (bytes.length !== width * height * (format === 24 ? 3 : 4))
    throw new KittyCommandError("EINVAL", "invalid pixel data length");
}

export async function decodeKittyPayload(
  params: Record<string, string>,
  payload: string,
  current: () => boolean,
) {
  const format = transferFormat(params);
  let bytes = await decodeBase64Payload(payload);
  if (params.o !== undefined) {
    if (params.o !== "z")
      throw new KittyCommandError("ENOTSUP", "unsupported compression");
    const inflated = await inflateKittyPayload(bytes, current);
    if (!inflated) return;
    bytes = inflated;
  }
  const width = optionalInteger(params, "s", 1, 1_000_000);
  const height = optionalInteger(params, "v", 1, 1_000_000);
  const size =
    format === 100
      ? pngSize(bytes)
      : width && height
        ? { width, height }
        : undefined;
  if (!size)
    throw new KittyCommandError(
      "EINVAL",
      format === 100 ? "invalid PNG data" : "image dimensions are required",
    );
  checkSourceDimensions(size.width, size.height);
  const id = optionalInteger(params, "i", 0, 4_294_967_295);
  const number = optionalInteger(params, "I", 0, 4_294_967_295);
  if (id && number)
    throw new KittyCommandError("EINVAL", "specify either i or I");
  if (format !== 100) {
    const expected = size.width * size.height * (format === 24 ? 3 : 4);
    if (bytes.length !== expected)
      throw new KittyCommandError("EINVAL", "invalid pixel data length");
  }
  return { bytes, format, size };
}
