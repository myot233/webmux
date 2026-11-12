// Shows the code workspace in a separate window. The window holds only a
// same-origin frame at /code/<id>, which runs its own app instance; the opener
// hands it the reading state and answers its requests over a MessageChannel.
// Chromium desktop browsers in secure contexts get an always-on-top Document
// Picture-in-Picture window; other desktop browsers (Safari, Firefox,
// plain-HTTP origins) get an ordinary same-origin popup.

import { isStandalone, popoutOffered } from "../../platform/capabilities";
import {
  CodePopoutHandshakeSchema,
  codePopoutHandshake,
  CodePopoutOutboundSchema,
  type CodeHostState,
  type CodePopoutInbound,
  type CodeReveal,
  type CodeWorkspaceSnapshot,
} from "../../contracts/schemas";
import type { TextInsertion } from "../../domains/terminal/terminal-draft.svelte";
import { SnapshotBodies } from "../../domains/code/code-snapshot";

type DocumentPictureInPicture = {
  requestWindow(options?: { width?: number; height?: number }): Promise<Window>;
  readonly window: Window | null;
};

function documentPictureInPicture(): DocumentPictureInPicture | undefined {
  if (!isSecureContext || !("documentPictureInPicture" in window)) return;
  // Missing from TypeScript's DOM library; the shape is the spec's interface.
  const api = window.documentPictureInPicture as DocumentPictureInPicture;
  return api;
}
export function popoutAvailable() {
  return popoutOffered({
    documentPictureInPicture: !!documentPictureInPicture(),
    finePointer: matchMedia("(pointer: fine)").matches,
    touch: matchMedia("(any-pointer: coarse)").matches,
    standalone: isStandalone(),
  });
}
async function openWindow(width: number, height: number) {
  const api = documentPictureInPicture();
  if (api) return api.requestWindow({ width, height });
  const popup = window.open(
    "",
    "_blank",
    `popup,width=${width},height=${height}`,
  );
  if (!popup) throw new Error("弹出窗口被浏览器拦截，请允许本站弹出窗口");
  return popup;
}

// The frame has this long to load and restore before the pop-out fails.
const readyTimeout = 10_000;
// After a close request, how long the opener waits for the final state.
const collectTimeout = 2_000;
// A window closed by the user may still deliver its final state shortly
// after its pagehide.
const finalTimeout = 500;

export type CodePopout = {
  /** Collects the frame's latest state, then closes the window. */
  close(): Promise<void>;
  host(state: CodeHostState): void;
  reveal(target: CodeReveal): void;
  /** Closes the window without handing state back, for a leaving opener. */
  dispose(): void;
};

// Must be called from a user gesture: both window kinds need activation.
export async function popOut(options: {
  id: string;
  title: string;
  width: number;
  height: number;
  snapshot: CodeWorkspaceSnapshot;
  host: CodeHostState;
  paste(text: string): TextInsertion;
  /** The window is gone; `snapshot` is the latest state it reported. */
  closed(snapshot: CodeWorkspaceSnapshot): void;
}): Promise<CodePopout> {
  const win = await openWindow(
    Math.max(480, Math.round(options.width)),
    Math.max(360, Math.round(options.height)),
  );
  const shell = win.document;
  shell.title = options.title;
  const style = shell.createElement("style");
  // The frame paints the app; the shell only avoids a white flash.
  const background = getComputedStyle(document.body).backgroundColor;
  style.textContent = `html,body{height:100%;margin:0;background:${background};color-scheme:${getComputedStyle(document.documentElement).colorScheme || "normal"}}iframe{display:block;width:100%;height:100%;border:0}`;
  shell.head.append(style);
  const frame = shell.createElement("iframe");
  frame.title = "代码工作区";
  frame.src = `/code/${options.id}`;
  shell.body.append(frame);

  let latest = options.snapshot;
  // The frame sends each document body once; later snapshots reference it.
  const bodies = new SnapshotBodies();
  bodies.resolve(options.snapshot);
  let port: MessagePort | undefined;
  let finished = false;
  let disposed = false;
  let collecting: { id: number; done: () => void } | undefined;
  let collectSequence = 0;
  let afterHide: (() => void) | undefined;
  const ready = Promise.withResolvers<void>();
  // A window closed before it was ready only rejects the awaited promise.
  ready.promise.catch(() => undefined);

  const send = (message: CodePopoutInbound) => port?.postMessage(message);
  const receive = (event: MessageEvent) => {
    const parsed = CodePopoutOutboundSchema.safeParse(event.data);
    if (!parsed.success) {
      if (import.meta.env.DEV)
        console.warn("code pop-out: dropped message", parsed.error);
      return;
    }
    const message = parsed.data;
    switch (message.type) {
      case "ready":
        ready.resolve();
        break;
      case "paste":
        send({
          type: "pasteResult",
          requestId: message.requestId,
          result: options.paste(message.text),
        });
        break;
      case "snapshot":
        latest = bodies.resolve(message.snapshot);
        if (collecting && message.requestId === collecting.id)
          collecting.done();
        afterHide?.();
        break;
      case "close":
        void popout.close();
        break;
    }
  };
  // Only the frame this call created may open the channel.
  const handshake = (event: MessageEvent) => {
    if (
      port ||
      event.origin !== location.origin ||
      event.source !== frame.contentWindow ||
      !CodePopoutHandshakeSchema.safeParse(event.data).success
    )
      return;
    const channel = new MessageChannel();
    port = channel.port1;
    port.onmessage = receive;
    frame.contentWindow!.postMessage(codePopoutHandshake, location.origin, [
      channel.port2,
    ]);
    send({ type: "init", snapshot: options.snapshot, host: options.host });
  };
  win.addEventListener("message", handshake);

  // Ends the pop-out. The window always closes: one whose page went away
  // (reloaded, navigated) would otherwise stay open and empty.
  const finish = () => {
    if (finished) return;
    finished = true;
    window.removeEventListener("pagehide", closeWithOpener);
    win.removeEventListener("message", handshake);
    port?.close();
    win.close();
    if (!disposed) options.closed(latest);
  };
  // An ordinary popup outlives its opener; close it with the opener so the
  // workspace is never stranded in an orphaned window.
  const closeWithOpener = () => popout.dispose();
  window.addEventListener("pagehide", closeWithOpener);
  win.addEventListener(
    "pagehide",
    () => {
      ready.reject(new Error("独立窗口已关闭"));
      if (finished) return;
      // The frame reports its final state from its own pagehide; that
      // message may arrive just after this one.
      const timer = window.setTimeout(finish, finalTimeout);
      afterHide = () => {
        clearTimeout(timer);
        finish();
      };
    },
    { once: true },
  );

  const popout: CodePopout = {
    async close() {
      if (finished || collecting) return;
      const id = ++collectSequence;
      const collected = Promise.withResolvers<void>();
      const timer = window.setTimeout(collected.resolve, collectTimeout);
      collecting = {
        id,
        done: () => {
          clearTimeout(timer);
          collected.resolve();
        },
      };
      send({ type: "collect", requestId: id });
      await collected.promise;
      finish();
    },
    host: (state) => send({ type: "host", host: state }),
    reveal: (target) => send({ type: "reveal", reveal: target }),
    dispose() {
      disposed = true;
      finish();
    },
  };

  const timer = window.setTimeout(
    () => ready.reject(new Error("独立窗口加载超时")),
    readyTimeout,
  );
  try {
    await ready.promise;
  } catch (error) {
    popout.dispose();
    throw error;
  } finally {
    clearTimeout(timer);
  }
  return popout;
}
