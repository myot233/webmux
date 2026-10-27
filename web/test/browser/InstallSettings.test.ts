import { afterEach, expect, test, vi } from "vitest";
import { page } from "vitest/browser";
import { cleanup, render } from "vitest-browser-svelte/pure";
import InstallSettings from "../../src/components/InstallSettings.svelte";
import {
  installation,
  promptInstall,
  trackInstallation,
  type InstallPlatform,
} from "../../src/install.svelte";

afterEach(() => cleanup());

const secure: InstallPlatform = { secure: true, standalone: false, ios: false };
const insecureNotice = "当前地址不能提供完整 PWA 能力";
const installButton = () =>
  page.getByRole("button", { name: "安装 WebMux", exact: true });

const cases: {
  name: string;
  platform: Partial<InstallPlatform>;
  status?: { promptable: boolean; installed: boolean };
  shows: string[];
  hides: string[];
  button: boolean;
}[] = [
  {
    name: "Chromium with a kept prompt",
    platform: {},
    status: { promptable: true, installed: false },
    shows: ["未安装"],
    hides: [insecureNotice],
    button: true,
  },
  {
    name: "a secure browser without a prompt",
    platform: {},
    // Installed apps fire no prompt either, so this is not "未安装".
    shows: ["未以应用方式打开", "浏览器暂未提供安装提示", "添加到程序坞"],
    hides: [insecureNotice, "未安装"],
    button: false,
  },
  {
    name: "an installed app",
    platform: { standalone: true },
    status: { promptable: true, installed: false },
    shows: ["正在以应用方式运行"],
    hides: ["未安装"],
    button: false,
  },
  {
    name: "a tab after installation",
    platform: {},
    status: { promptable: false, installed: true },
    shows: ["已安装", "可从主屏幕或应用列表打开"],
    hides: ["未安装"],
    button: false,
  },
  {
    name: "iOS Safari",
    platform: { ios: true },
    shows: ["分享 → 添加到主屏幕", "重新输入 Token"],
    hides: [insecureNotice, "推送仍需要 HTTPS"],
    button: false,
  },
  {
    name: "iOS Safari over HTTP",
    platform: { ios: true, secure: false },
    shows: [insecureNotice, "分享 → 添加到主屏幕", "推送仍需要 HTTPS"],
    hides: [],
    button: false,
  },
  {
    name: "an insecure LAN address",
    platform: { secure: false },
    // Chromium cannot install from here even if it fired a prompt.
    status: { promptable: true, installed: false },
    shows: [insecureNotice, "查看公网访问配置"],
    hides: ["浏览器暂未提供安装提示"],
    button: false,
  },
];

for (const c of cases) {
  test(`install card for ${c.name}`, async () => {
    render(InstallSettings, {
      platform: { ...secure, ...c.platform },
      status: c.status ?? { promptable: false, installed: false },
      prompt: vi.fn(),
    });
    await expect.element(page.getByText("安装应用")).toBeVisible();
    for (const text of c.shows)
      expect(document.body.textContent).toContain(text);
    for (const text of c.hides)
      expect(document.body.textContent).not.toContain(text);
    expect(installButton().query() !== null).toBe(c.button);
  });
}

test("the install button shows the kept prompt and reports a dismissal", async () => {
  const prompt = vi.fn(async () => "dismissed" as const);
  render(InstallSettings, {
    platform: secure,
    status: { promptable: true, installed: false },
    prompt,
  });
  await installButton().click();
  expect(prompt).toHaveBeenCalledOnce();
  await expect.poll(() => document.body.textContent).toContain("已取消安装");
});

test("the startup listener keeps one install prompt until the app is installed", async () => {
  const target = new EventTarget();
  trackInstallation(target as Window);
  expect(await promptInstall()).toBe("unavailable");
  const event = Object.assign(new Event("beforeinstallprompt"), {
    prompt: vi.fn(async () => {}),
    userChoice: Promise.resolve({ outcome: "accepted" as const }),
  });
  target.dispatchEvent(event);
  expect(installation.promptable).toBe(true);
  expect(await promptInstall()).toBe("accepted");
  expect(event.prompt).toHaveBeenCalledOnce();
  // A prompt event is single-use.
  expect(installation.promptable).toBe(false);
  expect(await promptInstall()).toBe("unavailable");
  target.dispatchEvent(new Event("appinstalled"));
  expect(installation.installed).toBe(true);
});
