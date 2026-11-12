import { afterEach, expect, test, vi } from "vitest";
import { cleanup, render } from "vitest-browser-svelte/pure";
import CodePopoutFrameHarness from "./CodePopoutFrameHarness.svelte";
import {
  codePopoutHandshake,
  type CodePopoutOutbound,
  type CodeWorkspaceSnapshot,
} from "../../src/contracts/schemas";
import type { useCodePopoutFrame } from "../../src/hooks/code/code-popout-frame.svelte";

afterEach(async () => {
  try {
    await cleanup();
  } finally {
    vi.restoreAllMocks();
  }
});

const sampleSnapshot: CodeWorkspaceSnapshot = {
  version: 1,
  root: "workspace",
  rootIncludeIgnored: false,
  directoryVersions: {},
  navIdentity: "",
  navStatus: "",
  changeQuery: "",
  changeSearch: "",
  changeGroup: "",
  includeIgnored: false,
  directory: "",
  query: "",
  nav: "files",
  navOpen: true,
  navWidth: 260,
  active: "",
  tabs: [],
};

test("starts standalone when window.parent is window", () => {
  vi.spyOn(window, "parent", "get").mockReturnValue(window);
  let frame!: ReturnType<typeof useCodePopoutFrame>;
  render(CodePopoutFrameHarness, {
    oninit: (instance) => {
      frame = instance;
    },
  });
  expect(frame.phase).toBe("standalone");
});

test("handshakes with opener, syncs host/reveal/collect, and forwards paste and close", async () => {
  let frame!: ReturnType<typeof useCodePopoutFrame>;
  render(CodePopoutFrameHarness, {
    oninit: (instance) => {
      frame = instance;
    },
  });
  expect(frame.phase).toBe("waiting");
  await expect(frame.host.paste("before-port")).resolves.toBe("failed");

  const channel = new MessageChannel();
  const outbound: CodePopoutOutbound[] = [];
  channel.port1.onmessage = (event: MessageEvent<CodePopoutOutbound>) => {
    outbound.push(event.data);
  };
  channel.port1.start();

  window.dispatchEvent(
    new MessageEvent("message", {
      origin: location.origin,
      data: codePopoutHandshake,
      ports: [channel.port2],
    }),
  );

  const reportedCwd = {
    path: "/work/sub",
    host: "localhost",
    remote: false,
    since: "2026-10-16T00:00:00Z",
  };
  channel.port1.postMessage({
    type: "init",
    host: {
      draftEnabled: true,
      connection: "connected",
      sessionCwd: "/work",
      reportedCwd,
    },
    snapshot: sampleSnapshot,
  });

  await expect.poll(() => frame.phase).toBe("ready");
  expect(frame.initial).toEqual(sampleSnapshot);
  expect(frame.host.draftEnabled).toBe(true);
  expect(frame.host.connection).toBe("connected");
  expect(frame.host.sessionCwd).toBe("/work");
  expect(frame.host.reportedCwd).toEqual(reportedCwd);

  frame.ready(() => ({ ...sampleSnapshot, query: "main.go" }));
  await expect.poll(() => outbound.some((m) => m.type === "ready")).toBe(true);

  channel.port1.postMessage({
    type: "host",
    host: {
      draftEnabled: false,
      connection: "reconnecting",
      sessionCwd: "/work",
      reportedCwd: null,
    },
  });
  await expect.poll(() => frame.host.connection).toBe("reconnecting");
  expect(frame.host.draftEnabled).toBe(false);

  const reveal = {
    key: 1,
    path: "/work/main.go",
    line: 10,
  };
  channel.port1.postMessage({ type: "reveal", reveal });
  await expect.poll(() => frame.host.reveal).toEqual(reveal);

  channel.port1.postMessage({ type: "collect", requestId: 42 });
  await expect
    .poll(() =>
      outbound.find((m) => m.type === "snapshot" && m.requestId === 42),
    )
    .toMatchObject({
      type: "snapshot",
      requestId: 42,
      snapshot: { query: "main.go" },
    });

  const pastePromise = frame.host.paste("main.go:10");
  await expect
    .poll(() => outbound.find((m) => m.type === "paste"))
    .toMatchObject({
      type: "paste",
      requestId: 1,
      text: "main.go:10",
    });
  channel.port1.postMessage({
    type: "pasteResult",
    requestId: 1,
    result: "draft",
  });
  await expect(pastePromise).resolves.toBe("draft");

  frame.close();
  await expect.poll(() => outbound.some((m) => m.type === "close")).toBe(true);
  channel.port1.close();
});
