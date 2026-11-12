import { afterEach, expect, test, vi } from "vitest";
import { page, userEvent } from "vitest/browser";
import { cleanup, render } from "vitest-browser-svelte/pure";
import P2PSettings from "../../src/components/settings/P2PSettings.svelte";
import SidebarFooter from "../../src/components/shell/SidebarFooter.svelte";
import { p2pState, stopP2P } from "../../src/platform/p2p.svelte";
import type { P2PStatus } from "../../src/contracts/schemas";

const baseStatus: P2PStatus = {
  supported: true,
  enabled: true,
  portMapping: true,
  stunServers: [
    "stun:stun.cloudflare.com:3478",
    "stun:stun.l.google.com:19302",
  ],
  source: "config",
  storageAvailable: true,
  portMap: {
    state: "mapped",
    protocol: "upnp",
    externalPort: 45123,
  },
  activePeers: 1,
  peers: [],
};

type Call = { method: string; url: string; body?: unknown };

function serveP2P(initial: P2PStatus = baseStatus) {
  let current = structuredClone(initial);
  const calls: Call[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>(async (input, init) => {
      const method = init?.method ?? "GET";
      const url = new URL(String(input), location.href).pathname;
      const body = init?.body ? JSON.parse(String(init.body)) : undefined;
      calls.push({ method, url, body });
      if (url === "/api/p2p/ping") {
        return Response.json({ seq: 1, serverTs: Date.now() });
      }
      if (url === "/api/health") {
        return Response.json({ status: "ok" });
      }
      if (url === "/api/p2p/settings" && method === "PUT") {
        current = {
          ...current,
          enabled: body.enabled,
          portMapping: body.portMapping,
          stunServers: body.stunServers.length
            ? body.stunServers
            : initial.stunServers,
          source: "web",
        };
        return Response.json(current);
      }
      if (url === "/api/p2p/settings" && method === "DELETE") {
        current = structuredClone(initial);
        return Response.json(current);
      }
      if (url === "/api/p2p") {
        return Response.json(current);
      }
      return Response.json({ error: "not found" }, { status: 404 });
    }),
  );
  return calls;
}

afterEach(async () => {
  stopP2P();
  p2pState.candidateType = "";
  p2pState.ipFamily = "";
  p2pState.baselineRttMs = null;
  p2pState.p2pRttMs = null;
  p2pState.upgradeDurationMs = null;
  p2pState.switchCount = 0;
  p2pState.abTest.error = "";
  p2pState.abTest.baselineMedianMs = null;
  await cleanup();
  await userEvent.cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

test("renders P2P status, port mapping badge, STUN validation, save, and reset", async () => {
  const calls = serveP2P();
  p2pState.mode = "p2p";
  p2pState.phase = "connected";
  p2pState.candidateType = "srflx/srflx";
  p2pState.ipFamily = "IPv6";
  p2pState.baselineRttMs = 142;
  p2pState.p2pRttMs = 18;
  p2pState.upgradeDurationMs = 310;
  p2pState.switchCount = 1;

  render(P2PSettings);

  await expect
    .element(page.getByText("已映射 UPNP :45123", { exact: true }))
    .toBeVisible();
  await expect
    .element(page.getByText("P2P 直连", { exact: true }))
    .toBeVisible();
  await expect
    .element(page.getByText("srflx/srflx (IPv6)", { exact: true }))
    .toBeVisible();
  await expect
    .element(page.getByText("142 ms / 18 ms", { exact: true }))
    .toBeVisible();

  const resetBtn = page.getByRole("button", { name: "恢复配置文件默认" });
  await expect.element(resetBtn).toBeDisabled();

  // Reject turn: URI in STUN textarea
  const stunInput = page.getByLabelText(
    "STUN 服务器（每行一个，留空恢复预置）",
  );
  await stunInput.fill("turn:relay.example.com:3478");
  await page.getByRole("button", { name: "保存 STUN 列表" }).click();
  await expect
    .element(
      page
        .getByRole("alert")
        .filter({ hasText: "必须以 stun: 或 stuns: 开头" }),
    )
    .toBeVisible();

  // Save valid STUN server
  await stunInput.fill("stun:stun.cloudflare.com:3478");
  await page.getByRole("button", { name: "保存 STUN 列表" }).click();

  await vi.waitFor(() =>
    expect(
      calls.some((c) => c.method === "PUT" && c.url === "/api/p2p/settings"),
    ).toBe(true),
  );
  await expect
    .element(page.getByText("网页设置", { exact: true }))
    .toBeVisible();
  await expect.element(resetBtn).toBeEnabled();

  // Toggle P2P off
  const enableSwitch = page.getByRole("switch", {
    name: "自动尝试 P2P 直连升级",
  });
  await enableSwitch.click();
  await expect.element(enableSwitch).toHaveAttribute("aria-checked", "false");

  // Reset back to config defaults
  await resetBtn.click();
  await vi.waitFor(() =>
    expect(
      calls.some((c) => c.method === "DELETE" && c.url === "/api/p2p/settings"),
    ).toBe(true),
  );
  await expect
    .element(page.getByText("配置文件", { exact: true }))
    .toBeVisible();
});

test("SidebarFooter host status popover reflects P2P direct mode and RTT", async () => {
  serveP2P();
  p2pState.mode = "p2p";
  p2pState.phase = "connected";
  p2pState.candidateType = "host/host";
  p2pState.ipFamily = "IPv4";
  p2pState.p2pRttMs = 9;

  render(SidebarFooter, {
    connection: "connected",
    hostname: "devbox",
    setupSettled: true,
    onforward: () => {},
    onadd: () => {},
  });

  await page.getByRole("button", { name: /^查看主机状态：/ }).click();
  const dialog = page.getByRole("dialog", { name: "主机状态详情" });
  await expect.element(dialog).toBeVisible();
  await expect
    .element(dialog.getByText("P2P 直连（host/host/IPv4）", { exact: true }))
    .toBeVisible();
  await expect.element(dialog.getByText("9 ms", { exact: true })).toBeVisible();
});
