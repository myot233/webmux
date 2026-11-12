import { afterEach, expect, test, vi } from "vitest";
import { page, userEvent } from "vitest/browser";
import { cleanup, render } from "vitest-browser-svelte/pure";
import QuickCommandDialog from "../../src/components/settings/QuickCommandDialog.svelte";
import type { QuickCommand } from "../../src/contracts/schemas";

afterEach(async () => {
  await cleanup();
  await userEvent.cleanup();
});

const existing: QuickCommand[] = [
  {
    id: "cmd-existing",
    name: "查看状态",
    command: "git status",
    enter: true,
  },
];

test("validates duplicate names and saves a new quick command", async () => {
  const save = vi.fn(async () => "");
  render(QuickCommandDialog, {
    open: true,
    initial: undefined,
    existing,
    save,
  });

  await expect
    .element(page.getByRole("heading", { name: "添加快捷指令" }))
    .toBeVisible();
  const saveBtn = page.getByRole("button", { name: "保存" });
  await expect.element(saveBtn).toBeDisabled();

  const nameInput = page.getByRole("textbox", { name: "名称" });
  const bodyInput = page.getByRole("textbox", { name: "指令" });

  await nameInput.fill("查看状态");
  await bodyInput.fill("git diff\r\n");
  await expect.element(page.getByText(/已有同名快捷指令/)).toBeVisible();
  await expect.element(saveBtn).toBeDisabled();

  await nameInput.fill("  查看差异  ");
  await page.getByRole("switch", { name: "发送后自动回车" }).click();
  await expect.element(saveBtn).toBeEnabled();
  await saveBtn.click();

  await vi.waitFor(() => expect(save).toHaveBeenCalledOnce());
  expect(save.mock.calls[0]![0]).toMatchObject({
    name: "查看差异",
    command: "git diff",
    enter: true,
  });
});

test("edits an existing command and surfaces save errors without closing", async () => {
  const save = vi.fn(async () => "保存快捷指令失败");
  render(QuickCommandDialog, {
    open: true,
    initial: existing[0],
    existing,
    save,
  });

  await expect
    .element(page.getByRole("heading", { name: "编辑快捷指令" }))
    .toBeVisible();
  const nameInput = page.getByRole("textbox", { name: "名称" });
  await expect.element(nameInput).toHaveValue("查看状态");

  await page.getByRole("button", { name: "保存" }).click();
  await expect
    .element(page.getByRole("alert"))
    .toHaveTextContent("保存快捷指令失败");
  expect(save).toHaveBeenCalledWith({
    id: "cmd-existing",
    name: "查看状态",
    command: "git status",
    enter: true,
  });

  await page.getByRole("button", { name: "取消" }).click();
  await expect
    .element(page.getByRole("heading", { name: "编辑快捷指令" }))
    .not.toBeInTheDocument();
});
