import { afterEach, expect, test, vi } from "vitest";
import { page, userEvent } from "vitest/browser";
import { cleanup, render } from "vitest-browser-svelte/pure";
import QuickCommandMenu from "../../src/components/settings/QuickCommandMenu.svelte";
import type { QuickCommand } from "../../src/contracts/schemas";

afterEach(async () => {
  await cleanup();
  await userEvent.cleanup();
});

const command: QuickCommand = {
  id: "a",
  name: "状态",
  command: "git status",
  enter: true,
};

function open(
  props: {
    commands?: QuickCommand[];
    target?: "terminal" | "draft";
    unavailable?: string;
  } = {},
) {
  const run = vi.fn();
  render(QuickCommandMenu, {
    props: {
      commands: props.commands ?? [command],
      target: props.target ?? "terminal",
      unavailable: props.unavailable ?? "",
      disabled: false,
      side: "bottom",
      run,
      manage: () => {},
      onCloseAutoFocus: () => {},
    },
  });
  return { run };
}

test("lists only saved commands and runs the chosen one", async () => {
  const { run } = open();
  await page.getByRole("button", { name: "快捷指令" }).click();
  const items = page.getByRole("menuitem");
  await expect.element(items.first()).toHaveAccessibleName(/状态/);
  await expect
    .element(page.getByRole("menuitem", { name: "插入临时文件夹提示词" }))
    .not.toBeInTheDocument();
  await items.first().click();
  expect(run).toHaveBeenCalledWith(command);
});

test("an empty list shows the empty state", async () => {
  open({ commands: [] });
  await page.getByRole("button", { name: "快捷指令" }).click();
  await expect.element(page.getByText("还没有快捷指令")).toBeVisible();
});

test("blocked direct mode disables commands; draft mode keeps them", async () => {
  open({ unavailable: "终端正在重连" });
  await page.getByRole("button", { name: "快捷指令" }).click();
  await expect
    .element(page.getByRole("status"))
    .toHaveTextContent("终端正在重连");
  const item = page.getByRole("menuitem", { name: /状态/ });
  await expect.element(item).toHaveAttribute("aria-disabled", "true");
  await cleanup();

  const { run } = open({ target: "draft", unavailable: "终端正在重连" });
  await page.getByRole("button", { name: "快捷指令" }).click();
  await expect.element(item).toHaveAttribute("aria-disabled", "false");
  await item.click();
  expect(run).toHaveBeenCalledOnce();
});
