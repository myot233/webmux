import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { cleanup, render } from "vitest-browser-svelte/pure";
import P2PSettings from "../../src/components/settings/P2PSettings.svelte";
import {
  p2pState,
  startP2P,
  stopP2P,
  refreshP2PAfterSettingsChange,
  runP2PEchoABTest,
} from "../../src/platform/p2p.svelte";
import type { NegotiatedSession } from "../../src/platform/p2p-session";
import type { P2PStatus } from "../../src/contracts/schemas";
import type * as P2PFraming from "../../src/platform/p2p-framing";

const mocks = vi.hoisted(() => ({
  status: vi.fn(),
  baseline: vi.fn(),
  negotiate: vi.fn(),
}));
vi.mock("../../src/platform/api", () => ({ api: mocks.status }));
vi.mock("../../src/platform/p2p-session", () => ({
  measureBaselineHttpRtt: mocks.baseline,
  negotiateDirectPeer: mocks.negotiate,
}));
vi.mock("../../src/platform/p2p-framing", async (original) => ({
  ...(await original<typeof P2PFraming>()),
  isLocalOrOverlayHost: () => false,
}));
vi.mock("../../src/platform/ws", () => ({ handoffLiveSockets: vi.fn() }));

const status: P2PStatus = {
  supported: true,
  udpPort: 0,
  udpBindings: [{ network: "udp4", port: 45123 }],
  capabilities: { networks: ["udp4", "udp6"], iceMode: "full", turn: false },
  enabled: true,
  portMapping: false,
  stunServers: [],
  source: "config",
  storageAvailable: true,
  portMap: { state: "off" },
  activePeers: 0,
  peers: [],
};
let connection: EventTarget;

function session(rtt = 10): NegotiatedSession {
  const pc = {
    connectionState: "connected",
    onconnectionstatechange: null,
  } as unknown as RTCPeerConnection;
  const control = {
    readyState: "open",
    send: vi.fn(),
    onclose: null,
  } as unknown as RTCDataChannel;
  return {
    peerId: crypto.randomUUID(),
    pc,
    control,
    candidateType: "host/srflx",
    ipFamily: "ipv6",
    sendPing: vi.fn().mockResolvedValue(rtt),
    close: vi.fn(() => {
      pc.onconnectionstatechange = null;
      control.onclose = null;
    }),
  };
}

beforeEach(() => {
  vi.useFakeTimers({
    toFake: ["setTimeout", "clearTimeout", "setInterval", "clearInterval"],
  });
  connection = new EventTarget();
  const originalNavigator = navigator;
  vi.stubGlobal(
    "navigator",
    new Proxy(originalNavigator, {
      get(target, property) {
        return property === "connection"
          ? connection
          : Reflect.get(target, property, target);
      },
    }),
  );
  mocks.status.mockReset().mockResolvedValue(status);
  mocks.baseline.mockReset().mockResolvedValue([100, 100, 100]);
  mocks.negotiate.mockReset().mockImplementation(async () => session());
});
afterEach(async () => {
  stopP2P();
  await cleanup();
  vi.useRealTimers();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

test("cellular change replaces a connected peer with newly gathered candidates", async () => {
  const first = session();
  const second = session();
  mocks.negotiate.mockResolvedValueOnce(first).mockResolvedValueOnce(second);
  startP2P();
  await vi.advanceTimersByTimeAsync(0);
  expect(p2pState.mode).toBe("p2p");

  connection.dispatchEvent(new Event("change"));
  window.dispatchEvent(new Event("online"));
  expect(p2pState.mode).toBe("ws");
  expect(first.close).toHaveBeenCalled();
  await vi.advanceTimersByTimeAsync(1_000);
  expect(p2pState.mode).toBe("p2p");
  expect(p2pState.phase).toBe("connected");
  expect(mocks.negotiate).toHaveBeenCalledTimes(2);
  expect(second.close).not.toHaveBeenCalled();
});

test("an obsolete negotiation cannot overwrite a replacement or restart after stop", async () => {
  const { promise, resolve: resolveOld } =
    Promise.withResolvers<NegotiatedSession>();
  mocks.negotiate.mockReturnValueOnce(promise);
  startP2P();
  await vi.advanceTimersByTimeAsync(0);
  const signal = mocks.negotiate.mock.calls[0]![1] as AbortSignal;
  connection.dispatchEvent(new Event("change"));
  expect(signal.aborted).toBe(true);
  await vi.advanceTimersByTimeAsync(1_000);
  expect(p2pState.mode).toBe("p2p");

  const obsolete = session();
  resolveOld(obsolete);
  await vi.advanceTimersByTimeAsync(0);
  expect(obsolete.close).toHaveBeenCalled();
  expect(p2pState.mode).toBe("p2p");
  stopP2P();
  connection.dispatchEvent(new Event("change"));
  window.dispatchEvent(new Event("online"));
  await vi.advanceTimersByTimeAsync(60_000);
  expect(p2pState.phase).toBe("idle");
  expect(mocks.negotiate).toHaveBeenCalledTimes(2);
});

test("distinguishes failed connectivity from successful direct path with insufficient gain", async () => {
  mocks.negotiate.mockRejectedValueOnce(new Error("ICE failed"));
  startP2P();
  await vi.advanceTimersByTimeAsync(0);
  expect(p2pState.phase).toBe("direct_failed");
  expect(p2pState.mode).toBe("ws");
  const direct = session(95);
  mocks.negotiate.mockResolvedValueOnce(direct);
  await vi.advanceTimersByTimeAsync(5_000);
  expect(p2pState.phase).toBe("standby_hysteresis");
  expect(p2pState.mode).toBe("ws");
  expect(p2pState.p2pRttMs).toBe(95);
  expect(p2pState.ipFamily).toBe("ipv6");
  expect(direct.close).toHaveBeenCalled();
});

test("disabling P2P cancels a pending peer and prevents stale errors scheduling retries", async () => {
  const { promise, reject: rejectOld } =
    Promise.withResolvers<NegotiatedSession>();
  mocks.negotiate.mockReturnValueOnce(promise);
  startP2P();
  await vi.advanceTimersByTimeAsync(0);
  refreshP2PAfterSettingsChange({ ...status, enabled: false });
  rejectOld(new Error("cancelled old peer"));
  await vi.advanceTimersByTimeAsync(60_000);
  expect(p2pState.phase).toBe("disabled");
  expect(p2pState.mode).toBe("ws");
  expect(mocks.negotiate).toHaveBeenCalledTimes(1);
});

test("returning to the foreground reevaluates a previously unprofitable direct path", async () => {
  mocks.negotiate.mockResolvedValueOnce(session(95));
  startP2P();
  await vi.advanceTimersByTimeAsync(0);
  expect(p2pState.phase).toBe("standby_hysteresis");
  document.dispatchEvent(new Event("visibilitychange"));
  await vi.advanceTimersByTimeAsync(0);
  expect(p2pState.phase).toBe("connected");
  expect(p2pState.mode).toBe("p2p");
});

test("stopping during an A/B negotiation closes a late peer without reporting success", async () => {
  const { promise, resolve } = Promise.withResolvers<NegotiatedSession>();
  mocks.negotiate.mockReturnValueOnce(promise);
  const result = runP2PEchoABTest([]);
  await vi.advanceTimersByTimeAsync(0);
  stopP2P();
  const obsolete = session();
  resolve(obsolete);
  await result;
  expect(obsolete.close).toHaveBeenCalled();
  expect(p2pState.abTest.directSuccess).toBe(false);
  expect(p2pState.abTest.running).toBe(false);
});

test("opening settings updates diagnostics without replacing a connected peer", async () => {
  const connected = session();
  mocks.negotiate.mockResolvedValueOnce(connected);
  startP2P();
  await vi.advanceTimersByTimeAsync(0);
  expect(p2pState.mode).toBe("p2p");
  const switchCount = p2pState.switchCount;
  mocks.status.mockResolvedValue({
    ...status,
    portMap: { state: "mapped", protocol: "pcp", externalPort: 43210 },
  });

  render(P2PSettings);
  await vi.advanceTimersByTimeAsync(0);

  expect(connected.close).not.toHaveBeenCalled();
  expect(p2pState.mode).toBe("p2p");
  expect(p2pState.switchCount).toBe(switchCount);
  expect(p2pState.portMapState).toBe("mapped");
  expect(p2pState.portMapExternalPort).toBe(43210);
});

test("port mapping polls preserve a slow negotiation until it can upgrade", async () => {
  mocks.status.mockResolvedValue({
    ...status,
    portMapping: true,
    portMap: { state: "probing" },
  });
  render(P2PSettings);
  await vi.advanceTimersByTimeAsync(0);
  const pending = Promise.withResolvers<NegotiatedSession>();
  mocks.negotiate.mockReturnValueOnce(pending.promise);
  startP2P();
  await vi.advanceTimersByTimeAsync(0);
  const signal = mocks.negotiate.mock.calls[0]![1] as AbortSignal;

  await vi.advanceTimersByTimeAsync(6_000);
  expect(signal.aborted).toBe(false);
  const direct = session();
  pending.resolve(direct);
  await vi.advanceTimersByTimeAsync(0);

  expect(p2pState.mode).toBe("p2p");
  expect(p2pState.phase).toBe("connected");
  expect(direct.close).not.toHaveBeenCalled();
});

test("port mapping polls preserve an in-flight A/B measurement", async () => {
  mocks.status.mockResolvedValue({
    ...status,
    portMapping: true,
    portMap: { state: "probing" },
  });
  render(P2PSettings);
  await vi.advanceTimersByTimeAsync(0);
  const pending = Promise.withResolvers<NegotiatedSession>();
  mocks.negotiate.mockReturnValueOnce(pending.promise);
  const result = runP2PEchoABTest([]);
  await vi.advanceTimersByTimeAsync(0);
  const signal = mocks.negotiate.mock.calls[0]![1] as AbortSignal;

  await vi.advanceTimersByTimeAsync(2_000);
  expect(signal.aborted).toBe(false);
  const direct = session();
  pending.resolve(direct);
  await result;

  expect(p2pState.abTest.directSuccess).toBe(true);
  expect(p2pState.abTest.p2pMedianMs).toBe(10);
  expect(p2pState.abTest.running).toBe(false);
  expect(direct.close).toHaveBeenCalled();
});
