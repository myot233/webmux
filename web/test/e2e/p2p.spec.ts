import {
  test,
  expect,
  createShell,
  runCommand,
  expectTerminalLine,
  expectConnected,
  headerAction,
  revealSidebar,
} from "./fixtures";

test("skips P2P automatically on loopback without ?p2p=force", async ({
  page,
  request,
}) => {
  await page.goto("/settings#network");
  const network = page.locator("#network");
  await expect(
    network.getByText("WebSocket 基线", { exact: true }),
  ).toBeVisible();
  await expect(
    network.getByText("本机/内网或加密虚拟网直连，已跳过", { exact: true }),
  ).toBeVisible();
  const status = await (await request.get("/api/p2p")).json();
  expect(status.activePeers).toBe(0);
});

test("upgrades terminal stream and file upload to WebRTC DataChannel, then falls back cleanly", async ({
  page,
  request,
  workDir,
}) => {
  const id = await createShell(page, "P2P shell", workDir);
  await runCommand(page, "echo BASELINE_READY");
  await expectTerminalLine(page, "BASELINE_READY");

  // Reopen with ?p2p=force so Chromium negotiates a real WebRTC DataChannel
  // against the Go Pion backend while keeping baseline-first session startup.
  await page.goto(`/s/${id}?p2p=force`);
  await expectConnected(page);
  await expectTerminalLine(page, "BASELINE_READY");

  await expect
    .poll(async () => {
      const status = await (await request.get("/api/p2p")).json();
      const peer = status.peers?.[0];
      if (!peer || peer.state !== "connected") return "";
      return String(peer.candidateType || "direct");
    })
    .not.toBe("");

  const upgraded = await (await request.get("/api/p2p")).json();
  expect(upgraded.activePeers).toBe(1);
  expect(String(upgraded.peers[0].candidateType)).not.toContain("relay");

  // Verify host status popover reports P2P direct path.
  const openedDrawer = await revealSidebar(page);
  await page.getByRole("button", { name: /^查看主机状态：/ }).click();
  const hostDialog = page.getByRole("dialog", { name: "主机状态详情" });
  await expect(hostDialog).toBeVisible();
  await expect(hostDialog.getByText(/P2P 直连/)).toBeVisible();
  await page.keyboard.press("Escape");
  if (openedDrawer) await page.keyboard.press("Escape");

  let httpUploadRequests = 0;
  page.on("request", (req) => {
    if (req.url().endsWith(`/api/sessions/${id}/upload`)) {
      httpUploadRequests++;
    }
  });

  // Terminal PTY stream I/O over WebRTC DataChannel.
  await runCommand(page, "echo P2P_DATACHANNEL_OK");
  await expectTerminalLine(page, "P2P_DATACHANNEL_OK");

  // Chunked multipart file upload over WebRTC DataChannel RPC (0 HTTP XHRs).
  await page.locator(".xterm-helper-textarea").focus();
  await page.keyboard.type("cat ");
  const chooser = page.waitForEvent("filechooser");
  await (await headerAction(page, "上传文件")).click();
  await (
    await chooser
  ).setFiles({
    name: "p2p-upload.txt",
    mimeType: "text/plain",
    buffer: Buffer.from("P2P_UPLOAD_PAYLOAD\n"),
  });
  await expect(page.locator(".xterm-rows")).toContainText("p2p-upload.txt");
  expect(httpUploadRequests).toBe(0);
  await page.locator(".xterm-helper-textarea").press("Enter");
  await expectTerminalLine(page, "P2P_UPLOAD_PAYLOAD");

  // Server-side P2P disable tears down active PeerConnections; the browser
  // must seamlessly fall back to baseline WebSocket without losing the PTY.
  const disableRes = await request.put("/api/p2p/settings", {
    data: { enabled: false, portMapping: false, stunServers: [] },
  });
  expect(disableRes.ok()).toBeTruthy();

  await expect
    .poll(
      async () => (await (await request.get("/api/p2p")).json()).activePeers,
    )
    .toBe(0);

  await expectConnected(page);
  await runCommand(page, "echo WS_FALLBACK_ALIVE");
  await expectTerminalLine(page, "WS_FALLBACK_ALIVE");
});

test("Settings #network runs A/B echo speed test and toggles P2P settings without horizontal overflow", async ({
  page,
}) => {
  await page.goto("/settings?p2p=force#network");
  const network = page.locator("#network");

  await expect(network.getByText("P2P 直连", { exact: true })).toBeVisible();
  await network.getByRole("button", { name: "双路径回显测速" }).click();
  await expect(
    network
      .getByRole("status")
      .filter({ hasText: /基线 HTTP 回显：.*P2P DataChannel 回显：.*中位数/ }),
  ).toBeVisible();

  const enableSwitch = network.getByRole("switch", {
    name: "自动尝试 P2P 直连升级",
  });
  await enableSwitch.click();
  await expect(enableSwitch).toHaveAttribute("aria-checked", "false");
  await expect(
    network.getByText("WebSocket 基线", { exact: true }),
  ).toBeVisible();
  await expect(network.getByText("网页设置", { exact: true })).toBeVisible();

  const overflow = await page.evaluate(
    () => document.documentElement.scrollWidth > window.innerWidth,
  );
  expect(overflow).toBe(false);
});
