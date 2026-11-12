import { errorMessage } from "../../platform/error";
import {
  decodeKittyImage,
  decodeKittyPayload,
  inspectKittyImage,
} from "./kitty-decode";
import {
  addKittyPlacement,
  eraseKittyDisplay,
  removeKittyPlacements,
  respondKitty,
  type StoredImage,
  type StoredPlacement,
  writeKittyDirectText,
} from "./kitty-placements";
import {
  createKittyStream,
  KittyCommandError,
  type KittyEvent,
  type KittyGraphicsOptions,
  type KittyImage,
  type KittyPlacement,
  type KittyPlacementSpec,
  maxEncodedBytes,
  maxImages,
  maxTotalPixels,
  optionalId,
  parseKittyControl,
} from "./kitty-stream";

export {
  createKittyStream,
  type KittyGraphicsOptions,
  type KittyImage,
  type KittyPlacement,
  type KittyPlacementSpec,
  parseKittyControl,
};

export function createKittyGraphics(options: KittyGraphicsOptions) {
  const stream = createKittyStream();
  const images = new Map<number, StoredImage>();
  const numbers = new Map<number, number>();
  const placements = new Set<StoredPlacement>();
  const decoder = options.decode ?? decodeKittyImage;
  // Includes anonymous images retained only by a live placement.
  const resources = new Map<KittyImage, number>();
  let resourcePixels = 0;
  // Reservations outlive reset until their non-cancellable decoder settles.
  let pendingImages = 0;
  let pendingPixels = 0;
  let upload:
    | { control: Record<string, string>; parts: string[]; size: number }
    | undefined;
  let nextId = 1;
  let chain: Promise<void> = Promise.resolve();
  let chainId = 0;
  let chainBusy = false;
  let failureControl: Record<string, string> = {};
  let pendingWrites = 0;
  let writeWaiters: (() => void)[] = [];
  let disposed = false;
  let generation = 0;

  function current(expected: number) {
    return !disposed && expected === generation;
  }

  function respond(
    params: Record<string, string>,
    message: string,
    assigned?: { id?: number; number?: number },
  ) {
    respondKitty(options.reply, params, message, assigned);
  }

  function respondError(params: Record<string, string>, error: unknown) {
    const command =
      error instanceof KittyCommandError
        ? error
        : new KittyCommandError("EINVAL", errorMessage(error));
    respond(params, `${command.code}:${command.message}`);
  }

  function releaseImage(image: KittyImage) {
    const pixels = resources.get(image);
    if (pixels === undefined) return;
    resources.delete(image);
    resourcePixels -= pixels;
    image.dispose?.();
  }

  function storeImage(image: KittyImage, params: Record<string, string>) {
    const id = optionalId(params, "i");
    const number = optionalId(params, "I");
    const assigned = id ?? (number === undefined ? 0 : allocateId());
    if (id !== undefined) removeImage(id);
    const stored = { id: assigned, number, image };
    if (assigned !== 0) images.set(assigned, stored);
    if (number !== undefined) numbers.set(number, assigned);
    return stored;
  }

  function allocateId() {
    for (let attempts = 0; attempts < 1_000_000; attempts++) {
      const id = nextId;
      nextId = nextId >= 4_294_967_295 ? 1 : nextId + 1;
      if (!images.has(id)) return id;
    }
    throw new KittyCommandError("ENOSPC", "no image ids available");
  }

  function hasPlacements(imageId: number) {
    for (const placement of placements)
      if (placement.imageId === imageId) return true;
    return false;
  }

  function releaseAnonymousImage(image: KittyImage) {
    for (const placement of placements) if (placement.image === image) return;
    releaseImage(image);
  }

  function forgetPlacement(placement: StoredPlacement) {
    if (!placements.delete(placement)) return;
    if (placement.imageId === 0) releaseAnonymousImage(placement.image);
  }

  function removeImage(imageId: number) {
    for (const placement of Array.from(placements)) {
      if (placement.imageId !== imageId) continue;
      placement.handle.dispose();
      forgetPlacement(placement);
    }
    const stored = images.get(imageId);
    if (stored) releaseImage(stored.image);
    images.delete(imageId);
    if (stored?.number !== undefined && numbers.get(stored.number) === imageId)
      numbers.delete(stored.number);
  }

  function resolveImage(params: Record<string, string>) {
    const id = optionalId(params, "i");
    const number = optionalId(params, "I");
    if (id !== undefined && number !== undefined)
      throw new KittyCommandError("EINVAL", "specify either i or I");
    const resolved =
      id ?? (number === undefined ? undefined : numbers.get(number));
    if (resolved === undefined || resolved === 0) return undefined;
    return images.get(resolved);
  }

  async function decode(
    params: Record<string, string>,
    payload: string,
    expected: number,
  ) {
    const prepared = await decodeKittyPayload(params, payload, () =>
      current(expected),
    );
    if (!prepared) return;
    const { bytes, format, size } = prepared;
    if (
      resources.size + pendingImages + 1 > maxImages ||
      resourcePixels + pendingPixels + size.width * size.height > maxTotalPixels
    )
      throw new KittyCommandError(
        "ENOSPC",
        "decoded image resource limit exceeded",
      );
    pendingImages++;
    pendingPixels += size.width * size.height;
    try {
      const image = await decoder({
        bytes,
        format,
        width: size.width,
        height: size.height,
      });
      if (!current(expected)) {
        image.dispose?.();
        return;
      }
      if (image.width !== size.width || image.height !== size.height) {
        image.dispose?.();
        throw new KittyCommandError(
          "EINVAL",
          "decoded image dimensions do not match",
        );
      }
      resources.set(image, image.width * image.height);
      resourcePixels += image.width * image.height;
      return image;
    } finally {
      pendingImages--;
      pendingPixels -= size.width * size.height;
    }
  }

  async function transmit(
    params: Record<string, string>,
    payload: string,
    display: boolean,
    expected: number,
  ) {
    const image = await decode(params, payload, expected);
    if (!image) return;
    if (!current(expected)) {
      releaseImage(image);
      return;
    }
    let stored: StoredImage;
    try {
      stored = storeImage(image, params);
    } catch (error) {
      releaseImage(image);
      throw error;
    }
    if (display) {
      try {
        await addKittyPlacement(
          stored,
          params,
          () => current(expected),
          options,
          placements,
          forgetPlacement,
        );
      } catch (error) {
        if (!current(expected)) return;
        if (stored.id === 0) releaseImage(image);
        else removeImage(stored.id);
        throw error;
      }
    } else if (stored.id === 0) {
      releaseImage(image);
    }
    if (!current(expected)) return;
    respond(params, "OK", {
      id: stored.id === 0 ? undefined : stored.id,
      number: stored.number,
    });
  }

  async function execute(
    params: Record<string, string>,
    payload: string,
    expected: number,
  ) {
    const action = params.a ?? "t";
    if (action === "q") {
      await inspectKittyImage(params, payload);
      if (!current(expected)) return;
      respond(params, "OK");
      return;
    }
    if (
      params.P !== undefined ||
      params.Q !== undefined ||
      params.H !== undefined ||
      params.V !== undefined
    )
      throw new KittyCommandError(
        "ENOTSUP",
        "relative placements are not supported",
      );
    if (params.U === "1")
      throw new KittyCommandError(
        "ENOTSUP",
        "unicode placeholder placements are not supported",
      );
    switch (action) {
      case "t":
        await transmit(params, payload, false, expected);
        return;
      case "T":
        await transmit(params, payload, true, expected);
        return;
      case "p": {
        const stored = resolveImage(params);
        if (!stored) throw new KittyCommandError("ENOENT", "image not found");
        await addKittyPlacement(
          stored,
          params,
          () => current(expected),
          options,
          placements,
          forgetPlacement,
        );
        if (!current(expected)) return;
        respond(params, "OK", { id: stored.id, number: stored.number });
        return;
      }
      case "d":
        removeKittyPlacements(
          params,
          placements,
          numbers,
          options.viewport(),
          options.cursor(),
          optionalId,
          forgetPlacement,
          hasPlacements,
          removeImage,
        );
        return;
      case "f":
      case "a":
      case "c":
        throw new KittyCommandError("ENOTSUP", "animation is not supported");
      default:
        throw new KittyCommandError("EINVAL", "unknown action");
    }
  }

  async function handle(control: string, payload: string, expected: number) {
    let params: Record<string, string>;
    try {
      params = parseKittyControl(control);
    } catch {
      return;
    }
    failureControl = params;
    const action = params.a;
    if (upload) {
      if (action === undefined) {
        upload.parts.push(payload);
        upload.size += payload.length;
        if (upload.size > maxEncodedBytes) {
          const failed = upload.control;
          upload = undefined;
          respondError(
            failed,
            new KittyCommandError("EINVAL", "image is too large"),
          );
          return;
        }
        if (params.m === "1") return;
        const finished = upload;
        upload = undefined;
        try {
          await execute(finished.control, finished.parts.join(""), expected);
        } catch (error) {
          if (current(expected)) respondError(finished.control, error);
        }
        return;
      }
      upload = undefined;
    }
    if (action === "f" || action === "a" || action === "c") {
      respondError(
        params,
        new KittyCommandError("ENOTSUP", "animation is not supported"),
      );
      return;
    }
    if (action === undefined && params.m !== undefined) {
      respondError(
        params,
        new KittyCommandError("EINVAL", "unexpected image chunk"),
      );
      return;
    }
    if (params.m === "1") {
      if (payload.length > maxEncodedBytes) {
        respondError(
          params,
          new KittyCommandError("EINVAL", "image is too large"),
        );
        return;
      }
      upload = { control: params, parts: [payload], size: payload.length };
      return;
    }
    try {
      await execute(params, payload, expected);
    } catch (error) {
      if (current(expected)) respondError(params, error);
    }
  }

  function writeText(data: Uint8Array, parsed?: () => void) {
    pendingWrites++;
    options.write(data, () => {
      pendingWrites--;
      if (pendingWrites === 0 && writeWaiters.length > 0) {
        const waiters = writeWaiters;
        writeWaiters = [];
        for (const resolve of waiters) resolve();
      }
      parsed?.();
    });
  }

  function drainWrites() {
    if (pendingWrites === 0) return Promise.resolve();
    return new Promise<void>((resolve) => writeWaiters.push(resolve));
  }

  function process(data: Uint8Array, parsed: () => void) {
    if (disposed) {
      parsed();
      return;
    }
    const expected = generation;
    const events: KittyEvent[] = [];
    stream.write(data, (event) => events.push(event));
    const hasGraphics = events.some((event) => event.kind === "graphics");
    if (!hasGraphics && !chainBusy) {
      writeKittyDirectText(events, writeText, parsed);
      return;
    }
    const id = ++chainId;
    chainBusy = true;
    chain = chain
      .then(async () => {
        if (!current(expected)) return;
        // Text keeps xterm's write order without a round trip per fragment;
        // only a graphics command waits, so it sees the cursor the preceding
        // text left. Awaiting every fragment cost one timer task each and
        // stretched a mixed redraw across many painted frames.
        for (const event of events) {
          if (!current(expected)) return;
          if (event.kind === "text") {
            if (event.data.length > 0) writeText(event.data);
          } else {
            await drainWrites();
            await handle(event.control, event.payload, expected);
          }
        }
        await drainWrites();
      })
      .catch((error: unknown) => {
        if (
          current(expected) &&
          (failureControl.i !== undefined || failureControl.I !== undefined)
        )
          respondError(failureControl, error);
      })
      .finally(() => {
        if (id === chainId) chainBusy = false;
        parsed();
      });
  }

  function clear() {
    generation++;
    chainId++;
    chainBusy = false;
    failureControl = {};
    chain = Promise.resolve();
    upload = undefined;
    stream.reset();
    for (const placement of Array.from(placements)) {
      placement.handle.dispose();
      forgetPlacement(placement);
    }
    placements.clear();
    for (const image of resources.keys()) releaseImage(image);
    images.clear();
    numbers.clear();
  }

  return {
    write: process,
    eraseDisplay(mode: number) {
      eraseKittyDisplay(
        mode,
        disposed,
        placements,
        options.viewport,
        forgetPlacement,
      );
    },
    reset() {
      clear();
      nextId = 1;
    },
    dispose() {
      if (disposed) return;
      disposed = true;
      clear();
    },
  };
}
