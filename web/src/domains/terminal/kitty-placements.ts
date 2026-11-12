import {
  KittyCommandError,
  type KittyCursor,
  type KittyEvent,
  type KittyGraphicsOptions,
  type KittyImage,
  kittyLayout,
  type KittyPlacement,
  type KittyViewport,
  maxPlacements,
  optionalId,
  optionalInteger,
} from "./kitty-stream";

export interface StoredImage {
  id: number;
  number?: number;
  image: KittyImage;
}

export function writeKittyDirectText(
  events: KittyEvent[],
  writeText: (data: Uint8Array, parsed?: () => void) => void,
  parsed: () => void,
) {
  let last = -1;
  for (let index = 0; index < events.length; index++) {
    const event = events[index];
    if (event?.kind === "text" && event.data.length > 0) last = index;
  }
  if (last < 0) {
    parsed();
    return;
  }
  for (let index = 0; index <= last; index++) {
    const event = events[index];
    if (event?.kind !== "text" || event.data.length === 0) continue;
    writeText(event.data, index === last ? parsed : undefined);
  }
}

export interface StoredPlacement {
  image: KittyImage;
  imageId: number;
  placementId: number;
  z: number;
  handle: KittyPlacement;
}

function intersects(placement: StoredPlacement, column: number, line: number) {
  const bounds = placement.handle.bounds();
  return (
    column >= bounds.column &&
    column < bounds.column + bounds.columns &&
    line >= bounds.line &&
    line < bounds.line + bounds.rows
  );
}

export function removeKittyPlacements(
  params: Record<string, string>,
  placements: Set<StoredPlacement>,
  numbers: Map<number, number>,
  viewport: KittyViewport,
  cursor: KittyCursor,
  optionalId: (
    params: Record<string, string>,
    key: string,
  ) => number | undefined,
  forgetPlacement: (placement: StoredPlacement) => void,
  hasPlacements: (imageId: number) => boolean,
  removeImage: (imageId: number) => void,
) {
  const d = params.d ?? "a";
  if (d === "f" || d === "F") return;
  const kind = d.toLowerCase();
  const hard = d === d.toUpperCase();
  const z = optionalInteger(params, "z", -2_147_483_648, 2_147_483_647);
  const placement = optionalId(params, "p");
  const imageId = optionalId(params, "i");
  const number = optionalId(params, "I");
  const usesCell = kind === "p" || kind === "q" || kind === "x" || kind === "y";
  const targetColumn = usesCell
    ? optionalInteger(params, "x", 1, 4_294_967_295)
    : undefined;
  const targetRow = usesCell
    ? optionalInteger(params, "y", 1, 4_294_967_295)
    : undefined;
  const targetCellColumn =
    targetColumn === undefined ? undefined : targetColumn - 1;
  const targetCellLine =
    targetRow === undefined ? undefined : viewport.top + targetRow - 1;
  const rangeLow =
    kind === "r" ? optionalInteger(params, "x", 0, 4_294_967_295) : undefined;
  const rangeHigh =
    kind === "r" ? optionalInteger(params, "y", 0, 4_294_967_295) : undefined;
  const numberId = number === undefined ? undefined : numbers.get(number);
  const removedImages = new Set<number>();
  for (const item of Array.from(placements)) {
    if (
      kind !== "i" &&
      kind !== "n" &&
      kind !== "r" &&
      kind !== "z" &&
      item.handle.active?.() === false
    )
      continue;
    const bounds = item.handle.bounds();
    let match = false;
    switch (kind) {
      case "a":
        match =
          bounds.line + bounds.rows > viewport.top &&
          bounds.line < viewport.bottom;
        break;
      case "i":
        match =
          imageId !== undefined &&
          item.imageId === imageId &&
          (placement === undefined || item.placementId === placement);
        break;
      case "n":
        match =
          numberId !== undefined &&
          item.imageId === numberId &&
          (placement === undefined || item.placementId === placement);
        break;
      case "c":
        match = intersects(item, cursor.column, cursor.line);
        break;
      case "p":
        match =
          targetCellColumn !== undefined &&
          targetCellLine !== undefined &&
          intersects(item, targetCellColumn, targetCellLine);
        break;
      case "q":
        match =
          targetCellColumn !== undefined &&
          targetCellLine !== undefined &&
          intersects(item, targetCellColumn, targetCellLine) &&
          (z === undefined || item.z === z);
        break;
      case "r":
        match =
          rangeLow !== undefined &&
          rangeHigh !== undefined &&
          item.imageId >= rangeLow &&
          item.imageId <= rangeHigh;
        break;
      case "x":
        match =
          targetCellColumn !== undefined &&
          targetCellColumn >= bounds.column &&
          targetCellColumn < bounds.column + bounds.columns;
        break;
      case "y":
        match =
          targetCellLine !== undefined &&
          targetCellLine >= bounds.line &&
          targetCellLine < bounds.line + bounds.rows;
        break;
      case "z":
        match = z !== undefined && item.z === z;
        break;
      default:
        throw new KittyCommandError("EINVAL", "unknown delete operation");
    }
    if (!match) continue;
    item.handle.dispose();
    forgetPlacement(item);
    if (item.imageId !== 0) removedImages.add(item.imageId);
  }
  if (hard) {
    if (numberId !== undefined) removedImages.add(numberId);
    if (imageId !== undefined) removedImages.add(imageId);
    for (const id of removedImages) if (!hasPlacements(id)) removeImage(id);
  }
}

export function eraseKittyDisplay(
  mode: number,
  disposed: boolean,
  placements: Set<StoredPlacement>,
  viewport: () => KittyViewport,
  forgetPlacement: (placement: StoredPlacement) => void,
) {
  // Kitty specifies that partial text erasure must not affect graphics.
  if (disposed || mode !== 2) return;
  const vp = viewport();
  for (const item of Array.from(placements)) {
    if (item.handle.active?.() === false) continue;
    const bounds = item.handle.bounds();
    const firstLine = Math.max(bounds.line, vp.top);
    const lastLine = Math.min(bounds.line + bounds.rows, vp.bottom) - 1;
    if (firstLine > lastLine) continue;
    item.handle.dispose();
    forgetPlacement(item);
  }
}

export async function addKittyPlacement(
  stored: StoredImage,
  params: Record<string, string>,
  current: () => boolean,
  options: KittyGraphicsOptions,
  placements: Set<StoredPlacement>,
  forgetPlacement: (placement: StoredPlacement) => void,
) {
  if (!current()) return;
  if (params.U === "1")
    throw new KittyCommandError(
      "ENOTSUP",
      "unicode placeholder placements are not supported",
    );
  const size = options.cellSize();
  if (
    !size ||
    !Number.isFinite(size.width) ||
    !Number.isFinite(size.height) ||
    size.width <= 0 ||
    size.height <= 0
  )
    throw new KittyCommandError("EINVAL", "terminal is not ready");
  const layout = kittyLayout(params, stored.image, size);
  const cursor = options.cursor();
  const placementId = optionalId(params, "p") ?? 0;
  const z = optionalInteger(params, "z", -2_147_483_648, 2_147_483_647, 0) ?? 0;
  if (stored.id !== 0 && placementId !== 0) {
    for (const placement of Array.from(placements)) {
      if (
        placement.imageId === stored.id &&
        placement.placementId === placementId
      ) {
        placement.handle.dispose();
        forgetPlacement(placement);
      }
    }
  }
  if (placements.size >= maxPlacements)
    throw new KittyCommandError("ENOSPC", "placement limit exceeded");
  // A dismissal reported before place() returns has nothing stored to forget.
  let placement: StoredPlacement | undefined;
  const handle = options.renderer.place(
    stored.image,
    {
      ...layout,
      column: cursor.column,
      line: cursor.line,
      z,
      relayout: (next) => kittyLayout(params, stored.image, next),
    },
    () => {
      if (placement) forgetPlacement(placement);
    },
  );
  placement = {
    image: stored.image,
    imageId: stored.id,
    placementId: stored.id === 0 ? 0 : placementId,
    z,
    handle,
  };
  placements.add(placement);
  if (params.C !== "1")
    await options.moveCursor(layout.columns, Math.max(0, layout.rows - 1));
  return placement;
}

export function respondKitty(
  reply: (text: string) => void,
  params: Record<string, string>,
  message: string,
  assigned?: { id?: number; number?: number },
) {
  const quiet = optionalInteger(params, "q", 0, 1_000_000, 0) ?? 0;
  if (quiet >= 2) return;
  if (message === "OK" && quiet >= 1) return;
  const parts: string[] = [];
  const id = assigned?.id ?? optionalId(params, "i");
  const number = assigned?.number ?? optionalId(params, "I");
  if (id === undefined && number === undefined) return;
  if (id !== undefined) parts.push(`i=${id}`);
  if (number !== undefined) parts.push(`I=${number}`);
  const placement = optionalId(params, "p");
  if (placement !== undefined) parts.push(`p=${placement}`);
  reply(`\x1b_G${parts.join(",")};${message}\x1b\\`);
}
