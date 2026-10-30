import { afterEach, expect, test, vi } from "vitest";
import { page, userEvent } from "vitest/browser";
import { cleanup, render } from "vitest-browser-svelte/pure";
import QuickCommandMenu from "../../src/components/QuickCommandMenu.svelte";
import type { QuickCommand } from "../../src/schemas";

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
  const artifact = vi.fn();
  const run = vi.fn();
  render(QuickCommandMenu, {
    props: {
      commands: props.commands ?? [command],
      target: props.target ?? "terminal",
      unavailable: props.unavailable ?? "",
      disabled: false,
      side: "bottom",
      run,
      artifact,
      manage: () => {},
      onCloseAutoFocus: () => {},
    },
  });
  return { artifact, run };
}
const fixed = () =>
  page.getByRole("menuitem", { name: "插入临时文件夹提示词" });

test("the fixed item precedes user commands and invokes its callback", async () => {
  const { artifact, run } = open();
  await page.getByRole("button", { name: "快捷指令" }).click();
  const items = page.getByRole("menuitem");
  await expect
    .element(items.first())
    .toHaveAccessibleName("插入临时文件夹提示词");
  await expect.element(items.nth(1)).toHaveAccessibleName(/状态/);
  await fixed().click();
  expect(artifact).toHaveBeenCalledOnce();
  expect(run).not.toHaveBeenCalled();
});

test("the fixed item stays above the empty state", async () => {
  open({ commands: [] });
  await page.getByRole("button", { name: "快捷指令" }).click();
  await expect.element(fixed()).toBeEnabled();
  const empty = page.getByText("还没有快捷指令");
  await expect.element(empty).toBeVisible();
  expect(
    fixed().element().compareDocumentPosition(empty.element()) &
      Node.DOCUMENT_POSITION_FOLLOWING,
  ).toBeTruthy();
});

test("blocked direct mode disables the fixed item; draft mode keeps it", async () => {
  open({ unavailable: "终端正在重连" });
  await page.getByRole("button", { name: "快捷指令" }).click();
  await expect
    .element(page.getByRole("status"))
    .toHaveTextContent("终端正在重连");
  await expect.element(fixed()).toHaveAttribute("aria-disabled", "true");
  await cleanup();

  const { artifact } = open({ target: "draft", unavailable: "终端正在重连" });
  await page.getByRole("button", { name: "快捷指令" }).click();
  await expect.element(fixed()).toHaveAttribute("aria-disabled", "false");
  await fixed().click();
  expect(artifact).toHaveBeenCalledOnce();
});
