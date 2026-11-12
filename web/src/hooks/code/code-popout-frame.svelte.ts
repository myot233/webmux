import { onMount } from "svelte";
import {
  CodePopoutHandshakeSchema,
  CodePopoutInboundSchema,
  codePopoutHandshake,
  type CodeHostState,
  type CodePopoutOutbound,
  type CodeReveal,
  type CodeWorkspaceSnapshot,
} from "../../contracts/schemas";
import type { CodeHost } from "../../domains/code/code-host";
import { SentBodies } from "../../domains/code/code-snapshot";
import type { TextInsertion } from "../../domains/terminal/terminal-draft.svelte";

// A paste the opener does not answer in time did not reach any input.
const pasteTimeout = 5_000;
// Reading state is pushed this long after the last interaction, so the
// opener holds recent state even if the window closes without notice.
const pushDelay = 1_000;
const interactions = [
  "pointerup",
  "keyup",
  "wheel",
  "scroll",
  "input",
  "change",
] as const;

/**
 * The pop-out frame's side of the channel to the terminal page that opened
 * it: receives the reading state and host fields, answers state requests and
 * forwards reference insertions. A frame that is not embedded is
 * "standalone" and never connects.
 */
export function useCodePopoutFrame() {
  const state = $state<{
    phase: "waiting" | "standalone" | "ready";
    host: CodeHostState;
    reveal?: CodeReveal;
    initial: CodeWorkspaceSnapshot | undefined;
  }>({
    phase: window.parent === window ? "standalone" : "waiting",
    host: {
      draftEnabled: false,
      connection: "connecting",
      sessionCwd: "",
      reportedCwd: null,
    },
    initial: undefined,
  });
  let port: MessagePort | undefined;
  let capture: (() => CodeWorkspaceSnapshot) | undefined;
  let pasteSequence = 0;
  const pastes: Record<number, (result: TextInsertion) => void> = {};
  const send = (message: CodePopoutOutbound) => port?.postMessage(message);
  // Document bodies the opener already holds.
  const sent = new SentBodies();
  const pushSnapshot = (requestId?: number) => {
    if (!capture || !port) return;
    send({
      type: "snapshot",
      requestId,
      snapshot: sent.reference(capture()),
    });
  };

  const host: CodeHost = {
    get draftEnabled() {
      return state.host.draftEnabled;
    },
    get connection() {
      return state.host.connection;
    },
    get sessionCwd() {
      return state.host.sessionCwd;
    },
    get reportedCwd() {
      return state.host.reportedCwd;
    },
    get reveal() {
      return state.reveal;
    },
    paste(text) {
      if (!port) return Promise.resolve("failed");
      const requestId = ++pasteSequence;
      const { promise, resolve } = Promise.withResolvers<TextInsertion>();
      const timer = window.setTimeout(() => {
        delete pastes[requestId];
        resolve("failed");
      }, pasteTimeout);
      pastes[requestId] = (result) => {
        clearTimeout(timer);
        resolve(result);
      };
      send({ type: "paste", requestId, text });
      return promise;
    },
  };

  const receive = (event: MessageEvent) => {
    const parsed = CodePopoutInboundSchema.safeParse(event.data);
    if (!parsed.success) {
      if (import.meta.env.DEV)
        console.warn("code pop-out frame: dropped message", parsed.error);
      return;
    }
    const message = parsed.data;
    switch (message.type) {
      case "init":
        state.host = message.host;
        state.initial = message.snapshot ?? undefined;
        // The opener handed these bodies over; it keeps them.
        if (message.snapshot) sent.reference(message.snapshot);
        state.phase = "ready";
        break;
      case "host":
        state.host = message.host;
        break;
      case "reveal":
        state.reveal = message.reveal;
        break;
      case "pasteResult":
        pastes[message.requestId]?.(message.result);
        delete pastes[message.requestId];
        break;
      case "collect":
        pushSnapshot(message.requestId);
        break;
    }
  };

  onMount(() => {
    if (state.phase === "standalone") return;
    // The first same-origin reply carries the channel. It comes from the
    // terminal page that opened the window, not from the window itself.
    const handshake = (event: MessageEvent) => {
      if (
        port ||
        event.origin !== location.origin ||
        event.ports.length !== 1 ||
        !CodePopoutHandshakeSchema.safeParse(event.data).success
      )
        return;
      port = event.ports[0]!;
      port.onmessage = receive;
      window.removeEventListener("message", handshake);
    };
    window.addEventListener("message", handshake);
    window.parent.postMessage(codePopoutHandshake, location.origin);

    let timer: number | undefined;
    const schedule = () => {
      clearTimeout(timer);
      timer = window.setTimeout(() => pushSnapshot(), pushDelay);
    };
    for (const type of interactions)
      document.addEventListener(type, schedule, {
        capture: true,
        passive: true,
      });
    // The window is closing: report the final state now.
    const hide = () => pushSnapshot();
    window.addEventListener("pagehide", hide);
    return () => {
      clearTimeout(timer);
      for (const type of interactions)
        document.removeEventListener(type, schedule, { capture: true });
      window.removeEventListener("message", handshake);
      window.removeEventListener("pagehide", hide);
      for (const resolve of Object.values(pastes)) resolve("failed");
      port?.close();
    };
  });

  return {
    get phase() {
      return state.phase;
    },
    get initial() {
      return state.initial;
    },
    host,
    /** The workspace is shown; from now on its state is reported. */
    ready(read: () => CodeWorkspaceSnapshot) {
      capture = read;
      send({ type: "ready" });
    },
    /** Asks the opener to take the workspace back. */
    close() {
      send({ type: "close" });
    },
  };
}
