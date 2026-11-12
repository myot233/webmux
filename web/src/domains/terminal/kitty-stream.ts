const ESC = 0x1b;
const APC = 0x5f;
const GRAPHICS = 0x47;
const ST = 0x5c;
const SEMICOLON = 0x3b;
const latin1Decoder = new TextDecoder("latin1");

// Source budgets include anonymous placements and outstanding decode reservations.
// Saturation is explicit ENOSPC; live resources are never silently evicted.
export const maxEncodedBytes = 48 * 1024 * 1024;
export const maxImageBytes = 32 * 1024 * 1024;
export const maxPixels = maxImageBytes / 4;
export const maxImageDimension = 8192;
export const maxTotalPixels = 64 * 1024 * 1024;
export const maxImages = 256;
export const maxPlacements = 1024;
const maxPendingEscapeBytes = 64 * 1024 * 1024;

export class KittyCommandError extends Error {
  constructor(
    readonly code: string,
    message: string,
  ) {
    super(message);
  }
}

export interface KittyCellSize {
  width: number;
  height: number;
}

export interface KittyCursor {
  column: number;
  line: number;
}

export interface KittyViewport {
  top: number;
  bottom: number;
}

export interface KittyImage {
  width: number;
  height: number;
  source: CanvasImageSource;
  dispose?(): void;
}

export interface KittyPlacementSpec {
  sourceX: number;
  sourceY: number;
  sourceWidth: number;
  sourceHeight: number;
  offsetX: number;
  offsetY: number;
  drawWidth: number;
  drawHeight: number;
  columns: number;
  rows: number;
  column: number;
  line: number;
  z: number;
  relayout(cell: KittyCellSize): KittyLayout;
}

export interface KittyPlacement {
  dispose(): void;
  active?(): boolean;
  bounds(): { column: number; line: number; columns: number; rows: number };
}

interface KittyRenderer {
  place(
    image: KittyImage,
    spec: KittyPlacementSpec,
    dismissed: () => void,
  ): KittyPlacement;
}

export interface KittyDecodeInput {
  bytes: Uint8Array;
  format: number;
  width?: number;
  height?: number;
}

type KittyDecode = (input: KittyDecodeInput) => Promise<KittyImage>;

export interface KittyGraphicsOptions {
  cellSize: () => KittyCellSize | undefined;
  cursor: () => KittyCursor;
  viewport: () => KittyViewport;
  reply: (text: string) => void;
  write: (data: Uint8Array, parsed: () => void) => void;
  moveCursor: (columns: number, rows: number) => Promise<void> | void;
  renderer: KittyRenderer;
  decode?: KittyDecode;
}

export type KittyEvent =
  | { kind: "text"; data: Uint8Array }
  | { kind: "graphics"; control: string; payload: string };

export function createKittyStream() {
  let prefix = 0;
  let graphics = false;
  let discarded = false;
  let escaped = false;
  let pending = new Uint8Array(0);
  let length = 0;

  function append(bytes: Uint8Array) {
    if (discarded || bytes.length === 0) return;
    if (length + bytes.length > maxPendingEscapeBytes - 4) {
      discarded = true;
      pending = new Uint8Array(0);
      length = 0;
      return;
    }
    if (length + bytes.length > pending.length) {
      const capacity = Math.min(
        maxPendingEscapeBytes - 4,
        Math.max(256, length + bytes.length, pending.length * 2),
      );
      const grown = new Uint8Array(capacity);
      grown.set(pending.subarray(0, length));
      pending = grown;
    }
    pending.set(bytes, length);
    length += bytes.length;
  }

  return {
    write(data: Uint8Array, emit: (event: KittyEvent) => void) {
      let cursor = 0;
      let textStart = 0;
      const flushText = (end: number) => {
        if (end > textStart)
          emit({ kind: "text", data: data.subarray(textStart, end) });
      };
      const flushPrefix = () => {
        if (prefix === 1) emit({ kind: "text", data: new Uint8Array([ESC]) });
        else if (prefix === 2)
          emit({ kind: "text", data: new Uint8Array([ESC, APC]) });
        prefix = 0;
      };
      while (cursor < data.length) {
        if (graphics) {
          if (escaped) {
            escaped = false;
            if (data[cursor] === ST) {
              if (!discarded) emitGraphics(pending.subarray(0, length), emit);
              graphics = false;
              discarded = false;
              pending = new Uint8Array(0);
              length = 0;
              textStart = ++cursor;
              continue;
            }
            append(new Uint8Array([ESC]));
          }
          const stop = data.indexOf(ESC, cursor);
          const end = stop < 0 ? data.length : stop;
          append(data.subarray(cursor, end));
          cursor = end;
          if (stop >= 0) {
            escaped = true;
            cursor++;
          }
          textStart = cursor;
          continue;
        }
        if (prefix === 1) {
          if (data[cursor] === APC) {
            prefix = 2;
            cursor++;
            textStart = cursor;
            continue;
          }
          flushPrefix();
          textStart = cursor;
          continue;
        }
        if (prefix === 2) {
          if (data[cursor] === GRAPHICS) {
            prefix = 0;
            graphics = true;
            cursor++;
            textStart = cursor;
            continue;
          }
          flushPrefix();
          textStart = cursor;
          continue;
        }
        const escape = data.indexOf(ESC, cursor);
        if (escape < 0) {
          cursor = data.length;
          break;
        }
        if (escape + 1 >= data.length) {
          flushText(escape);
          prefix = 1;
          cursor = data.length;
          textStart = cursor;
          break;
        }
        if (data[escape + 1] === APC) {
          if (escape + 2 >= data.length) {
            flushText(escape);
            prefix = 2;
            cursor = data.length;
            textStart = cursor;
            break;
          }
          if (data[escape + 2] === GRAPHICS) {
            flushText(escape);
            graphics = true;
            cursor = escape + 3;
            textStart = cursor;
            continue;
          }
        }
        cursor = escape + 1;
      }
      if (!graphics) flushText(cursor);
    },
    reset() {
      prefix = 0;
      graphics = false;
      discarded = false;
      escaped = false;
      pending = new Uint8Array(0);
      length = 0;
    },
    get pendingBytes() {
      return graphics ? length + (escaped ? 1 : 0) + 3 : prefix;
    },
  };
}

function emitGraphics(payload: Uint8Array, emit: (event: KittyEvent) => void) {
  const separator = payload.indexOf(SEMICOLON);
  emit({
    kind: "graphics",
    control:
      separator < 0
        ? latin1Decoder.decode(payload)
        : latin1Decoder.decode(payload.subarray(0, separator)),
    payload:
      separator < 0
        ? ""
        : latin1Decoder.decode(payload.subarray(separator + 1)),
  });
}

export function parseKittyControl(text: string): Record<string, string> {
  const params: Record<string, string> = {};
  for (const part of text.split(",")) {
    if (!part) continue;
    const equals = part.indexOf("=");
    if (equals <= 0) throw new KittyCommandError("EINVAL", "invalid control");
    const key = part.slice(0, equals);
    if (key.length !== 1 || key in params)
      throw new KittyCommandError("EINVAL", "invalid control");
    params[key] = part.slice(equals + 1);
  }
  return params;
}

export interface KittyLayout {
  sourceX: number;
  sourceY: number;
  sourceWidth: number;
  sourceHeight: number;
  offsetX: number;
  offsetY: number;
  drawWidth: number;
  drawHeight: number;
  columns: number;
  rows: number;
}

export function kittyLayout(
  params: Record<string, string>,
  image: KittyImage,
  cell: KittyCellSize,
): KittyLayout {
  const sourceX = optionalInteger(
    params,
    "x",
    0,
    Math.max(0, image.width - 1),
    0,
  );
  const sourceY = optionalInteger(
    params,
    "y",
    0,
    Math.max(0, image.height - 1),
    0,
  );
  const availableWidth = image.width - sourceX;
  const availableHeight = image.height - sourceY;
  const sourceWidth = Math.min(
    optionalInteger(params, "w", 1, 4_294_967_295, availableWidth),
    availableWidth,
  );
  const sourceHeight = Math.min(
    optionalInteger(params, "h", 1, 4_294_967_295, availableHeight),
    availableHeight,
  );
  if (sourceWidth <= 0 || sourceHeight <= 0)
    throw new KittyCommandError("EINVAL", "invalid source rectangle");
  const columns = optionalInteger(params, "c", 1, 1_000_000);
  const rows = optionalInteger(params, "r", 1, 1_000_000);
  const offsetX = optionalInteger(params, "X", 0, cell.width, 0);
  const offsetY = optionalInteger(params, "Y", 0, cell.height, 0);
  const aspect = sourceWidth / sourceHeight;
  let drawWidth: number;
  let drawHeight: number;
  let usedColumns: number;
  let usedRows: number;
  let letterboxX = 0;
  let letterboxY = 0;
  if (columns !== undefined && rows !== undefined) {
    const boxWidth = columns * cell.width;
    const boxHeight = rows * cell.height;
    const scale = Math.min(boxWidth / sourceWidth, boxHeight / sourceHeight);
    drawWidth = sourceWidth * scale;
    drawHeight = sourceHeight * scale;
    letterboxX = (boxWidth - drawWidth) / 2;
    letterboxY = (boxHeight - drawHeight) / 2;
    usedColumns = columns;
    usedRows = rows;
  } else if (columns !== undefined) {
    drawWidth = columns * cell.width;
    drawHeight = drawWidth / aspect;
    usedColumns = columns;
    usedRows = Math.ceil(drawHeight / cell.height);
  } else if (rows !== undefined) {
    drawHeight = rows * cell.height;
    drawWidth = drawHeight * aspect;
    usedColumns = Math.ceil(drawWidth / cell.width);
    usedRows = rows;
  } else {
    drawWidth = sourceWidth;
    drawHeight = sourceHeight;
    usedColumns = Math.ceil(drawWidth / cell.width);
    usedRows = Math.ceil(drawHeight / cell.height);
  }
  return {
    sourceX,
    sourceY,
    sourceWidth,
    sourceHeight,
    offsetX: offsetX + letterboxX,
    offsetY: offsetY + letterboxY,
    drawWidth,
    drawHeight,
    columns: usedColumns,
    rows: usedRows,
  };
}

export function optionalId(params: Record<string, string>, key: string) {
  const value = optionalInteger(params, key, 0, 4_294_967_295);
  return value === 0 ? undefined : value;
}

export function optionalInteger(
  params: Record<string, string>,
  key: string,
  min: number,
  max: number,
): number | undefined;
export function optionalInteger(
  params: Record<string, string>,
  key: string,
  min: number,
  max: number,
  fallback: number,
): number;
export function optionalInteger(
  params: Record<string, string>,
  key: string,
  min: number,
  max: number,
  fallback?: number,
) {
  const raw = params[key];
  if (raw === undefined || raw === "") return fallback;
  if (!/^-?\d+$/.test(raw))
    throw new KittyCommandError("EINVAL", `invalid ${key}`);
  const value = Number(raw);
  if (!Number.isSafeInteger(value) || value < min || value > max)
    throw new KittyCommandError("EINVAL", `invalid ${key}`);
  return value;
}
