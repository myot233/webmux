interface FileDropOptions {
  /** Capture before child handlers and take over file drops. */
  exclusive?: boolean;
  /** A non-file drag must not clear an existing drop status. */
  requireFileOnLeave?: boolean;
  /** Return false to keep a blocked drag out of the hover depth. */
  onEnter(transfer: DataTransfer, event: DragEvent): false | void;
  onOver(transfer: DataTransfer, event: DragEvent): void;
  onLeave(): void;
  /** Read the drag data and resolve the target during dispatch. */
  onDrop(transfer: DataTransfer, event: DragEvent): void;
}

function fileTransfer(event: DragEvent) {
  const transfer = event.dataTransfer;
  return transfer?.types.includes("Files") ? transfer : undefined;
}

export function attachFileDrop(host: HTMLElement, options: FileDropOptions) {
  const controller = new AbortController();
  const listeners = { capture: options.exclusive, signal: controller.signal };
  let depth = 0;
  host.addEventListener(
    "dragenter",
    (event) => {
      const transfer = fileTransfer(event);
      if (!transfer) return;
      event.preventDefault();
      if (options.onEnter(transfer, event) !== false) depth++;
    },
    listeners,
  );
  host.addEventListener(
    "dragover",
    (event) => {
      const transfer = fileTransfer(event);
      if (!transfer) return;
      event.preventDefault();
      options.onOver(transfer, event);
    },
    listeners,
  );
  host.addEventListener(
    "dragleave",
    (event) => {
      if (options.requireFileOnLeave && !fileTransfer(event)) return;
      depth = Math.max(0, depth - 1);
      if (depth === 0) options.onLeave();
    },
    listeners,
  );
  host.addEventListener(
    "drop",
    (event) => {
      const transfer = fileTransfer(event);
      if (!transfer) return;
      event.preventDefault();
      if (options.exclusive) event.stopImmediatePropagation();
      depth = 0;
      options.onDrop(transfer, event);
    },
    listeners,
  );
  return () => controller.abort();
}
