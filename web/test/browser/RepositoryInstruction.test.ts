import { expect, test, vi } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte/pure";
import AddRepositoryDialog from "../../src/components/workspace/AddRepositoryDialog.svelte";
import RepositorySettingsDialog from "../../src/components/workspace/RepositorySettingsDialog.svelte";
import { UpdateRepositoryRequestSchema } from "../../src/contracts/schemas";
import { commands, installChatFixture, respond } from "./chat-fixture";
import { repository } from "./shell-fixture";

installChatFixture();

test("adding a repository sends only its path and has no instruction field", async () => {
  respond(/^POST \/api\/repositories$/, () =>
    Response.json(repository(), { status: 201 }),
  );
  const onclose = vi.fn();
  render(AddRepositoryDialog, { defaultCwd: "~", onclose, onchange: vi.fn() });
  expect(page.getByLabelText("初始化指令（可选）").elements()).toEqual([]);
  await page.getByLabelText("仓库目录").fill("/repos/webmux");
  await page.getByRole("button", { name: "添加仓库" }).click();
  await vi.waitFor(() => expect(onclose).toHaveBeenCalled());
  expect(commands().at(-1)?.body).toEqual({ path: "/repos/webmux" });
});

test("the limit blocks an over-long instruction before sending", async () => {
  render(RepositorySettingsDialog, {
    repository: repository(),
    onclose: vi.fn(),
    onchange: vi.fn(),
  });
  await page
    .getByRole("textbox", { name: "初始化指令" })
    .fill("长".repeat(4001));
  await expect
    .element(page.getByRole("button", { name: "保存" }))
    .toBeDisabled();
});

test("the settings dialog edits or clears the instruction", async () => {
  respond(/^PATCH \/api\/repositories\/repo1$/, ({ body }) =>
    Response.json(
      repository({
        initInstruction:
          UpdateRepositoryRequestSchema.parse(body).initInstruction,
      }),
    ),
  );
  const onclose = vi.fn();
  render(RepositorySettingsDialog, {
    repository: repository({ initInstruction: "先读 AGENTS.md" }),
    onclose,
    onchange: vi.fn(),
  });
  await expect.element(page.getByText("仓库设置")).toBeVisible();
  const field = page.getByRole("textbox", { name: "初始化指令" });
  await expect.element(field).toHaveValue("先读 AGENTS.md");
  await field.fill("");
  await page.getByRole("button", { name: "保存" }).click();
  await vi.waitFor(() => expect(onclose).toHaveBeenCalled());
  expect(commands().at(-1)).toMatchObject({
    method: "PATCH",
    path: "/api/repositories/repo1",
    body: { initInstruction: "" },
  });
});

test("quick command chips insert references and unknown ones warn without blocking", async () => {
  render(RepositorySettingsDialog, {
    repository: repository(),
    onclose: vi.fn(),
    onchange: vi.fn(),
  });
  const field = page.getByRole("textbox", { name: "初始化指令" });
  await field.fill("先看");
  await page.getByRole("button", { name: "插入 恢复会话" }).click();
  await expect.element(field).toHaveValue("先看{{恢复会话}}");
  expect(page.getByRole("status").elements()).toEqual([]);

  await field.fill("{{恢复会话}} {{没有}} {{没有}}");
  await expect
    .element(page.getByRole("status"))
    .toMatchTextContent(/找不到这些快捷指令：\{\{没有\}\}。/);
  await expect
    .element(page.getByRole("button", { name: "保存" }))
    .toBeEnabled();
});

test("a rejected save keeps the dialog open with the reason", async () => {
  respond(/^PATCH \/api\/repositories\/repo1$/, () =>
    Response.json({ error: "初始化指令最多 4000 个字符" }, { status: 400 }),
  );
  const onclose = vi.fn();
  render(RepositorySettingsDialog, {
    repository: repository(),
    onclose,
    onchange: vi.fn(),
  });
  await page.getByRole("textbox", { name: "初始化指令" }).fill("新指令");
  await page.getByRole("button", { name: "保存" }).click();
  await expect
    .element(page.getByRole("alert"))
    .toHaveTextContent("初始化指令最多 4000 个字符");
  expect(onclose).not.toHaveBeenCalled();
});
