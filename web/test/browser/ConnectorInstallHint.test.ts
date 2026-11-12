import { afterEach, expect, test, vi } from "vitest";
import { page, userEvent } from "vitest/browser";
import { cleanup, render } from "vitest-browser-svelte/pure";
import ConnectorInstallHint from "../../src/components/sessions/ConnectorInstallHint.svelte";
import { claudeConnector as connector } from "./chat-fixture";

afterEach(async () => {
  await cleanup();
  await userEvent.cleanup();
  vi.restoreAllMocks();
});

test("names the missing connector, copies its install command and links its docs", async () => {
  const write = vi
    .spyOn(navigator.clipboard, "writeText")
    .mockResolvedValue(undefined);
  render(ConnectorInstallHint, { connector, detail: "安装后会自动重新检测" });
  await expect
    .element(
      page.getByText("未安装 Claude Code ACP 适配器（claude-agent-acp）"),
    )
    .toBeVisible();
  await expect.element(page.getByText("安装后会自动重新检测")).toBeVisible();
  await expect
    .element(page.getByRole("link", { name: "安装说明" }))
    .toHaveAttribute("href", connector.docsUrl);
  const copy = page.getByRole("button", { name: "复制安装命令" });
  await expect.element(copy).toHaveAttribute("title", "复制安装命令");
  await copy.click();
  expect(write).toHaveBeenCalledWith(connector.installCommand);
  await expect
    .element(page.getByRole("button", { name: "已复制安装命令" }))
    .toBeVisible();
  expect(page.getByRole("button", { name: "关闭安装提示" }).elements()).toEqual(
    [],
  );
});

test("a copy that finishes after unmount does not schedule feedback", async () => {
  const write = Promise.withResolvers<void>();
  vi.spyOn(navigator.clipboard, "writeText").mockReturnValue(write.promise);
  render(ConnectorInstallHint, { connector });
  await page.getByRole("button", { name: "复制安装命令" }).click();
  await cleanup();

  const schedule = vi.spyOn(window, "setTimeout");
  write.resolve();
  // Let both the clipboard adapter and its caller finish their awaits.
  await Promise.resolve();
  await Promise.resolve();
  expect(schedule).not.toHaveBeenCalledWith(expect.any(Function), 2000);
});

test("a custom message replaces the missing text and the close control dismisses", async () => {
  const ondismiss = vi.fn();
  render(ConnectorInstallHint, {
    connector,
    message: "聊天端需要另外安装 claude-agent-acp",
    tone: "info",
    ondismiss,
  });
  await expect
    .element(page.getByText("聊天端需要另外安装 claude-agent-acp"))
    .toBeVisible();
  await expect.element(page.getByText(connector.installCommand)).toBeVisible();
  await page.getByRole("button", { name: "关闭安装提示" }).click();
  expect(ondismiss).toHaveBeenCalledOnce();
});
