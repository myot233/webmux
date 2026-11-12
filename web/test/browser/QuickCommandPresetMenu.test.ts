import { afterEach, expect, test, vi } from "vitest";
import { page, userEvent } from "vitest/browser";
import { cleanup, render } from "vitest-browser-svelte/pure";
import QuickCommandPresetMenu from "../../src/components/settings/QuickCommandPresetMenu.svelte";
import {
  QUICK_COMMANDS_LIMIT,
  QUICK_COMMAND_PRESETS,
} from "../../src/domains/settings/quick-commands";
import type { QuickCommand } from "../../src/contracts/schemas";

afterEach(async () => {
  await cleanup();
  await userEvent.cleanup();
});

const own = (id: string): QuickCommand => ({
  id,
  name: `自定义 ${id}`,
  command: "echo",
  enter: false,
});
const trigger = () => page.getByRole("button", { name: "添加系统预设" });

test("lists only missing presets and adds the chosen one", async () => {
  const onadd = vi.fn();
  render(QuickCommandPresetMenu, {
    props: {
      commands: [QUICK_COMMAND_PRESETS[0]!, own("x")],
      busy: false,
      onadd,
    },
  });
  await trigger().click();
  await expect
    .element(page.getByRole("menuitem", { name: "添加预设 恢复会话" }))
    .not.toBeInTheDocument();
  await page.getByRole("menuitem", { name: "添加预设 WebMux 工具" }).click();
  expect(onadd).toHaveBeenCalledWith([QUICK_COMMAND_PRESETS[2]]);
});

test("adds every missing preset at once", async () => {
  const onadd = vi.fn();
  render(QuickCommandPresetMenu, {
    props: { commands: [], busy: false, onadd },
  });
  await trigger().click();
  await page.getByRole("menuitem", { name: /补回全部缺失预设/ }).click();
  expect(onadd).toHaveBeenCalledWith([...QUICK_COMMAND_PRESETS]);
});

test("is disabled when nothing is missing or the list is full", async () => {
  render(QuickCommandPresetMenu, {
    props: {
      commands: [...QUICK_COMMAND_PRESETS],
      busy: false,
      onadd: vi.fn(),
    },
  });
  await expect.element(trigger()).toBeDisabled();
  await cleanup();

  render(QuickCommandPresetMenu, {
    props: {
      commands: Array.from({ length: QUICK_COMMANDS_LIMIT }, (_, i) =>
        own(`c${i}`),
      ),
      busy: false,
      onadd: vi.fn(),
    },
  });
  await expect.element(trigger()).toBeDisabled();
});
