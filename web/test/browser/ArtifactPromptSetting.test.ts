import { afterEach, expect, test, vi } from "vitest";
import { page, userEvent } from "vitest/browser";
import { cleanup, render } from "vitest-browser-svelte/pure";
import ArtifactPromptSetting from "../../src/components/ArtifactPromptSetting.svelte";
import { BUILTIN_ARTIFACT_PROMPT } from "../../src/artifactPrompt";
import { settings, stopSettings } from "../../src/settings.svelte";
import { seedSettings } from "./settings-fixture";

afterEach(async () => {
  await cleanup();
  await userEvent.cleanup();
  stopSettings();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

function patches() {
  return vi
    .mocked(fetch)
    .mock.calls.filter(([, init]) => init?.method === "PATCH")
    .map(([, init]) => JSON.parse(String(init!.body)).changes);
}

test("saves a normalized template and restores the built-in one", async () => {
  await seedSettings();
  render(ArtifactPromptSetting);
  const input = page.getByRole("textbox", { name: "临时文件夹提示词" });
  const save = page.getByRole("button", { name: "保存", exact: true });
  await expect.element(input).toHaveValue(BUILTIN_ARTIFACT_PROMPT);
  await expect.element(save).toBeDisabled();
  await expect
    .element(page.getByRole("button", { name: "恢复默认" }))
    .toBeDisabled();

  await input.fill("没有占位符");
  await expect.element(save).toBeDisabled();
  await expect
    .element(page.getByRole("alert"))
    .toHaveTextContent("提示词需要包含 {dir}");

  await input.fill("文件放到 {dir}\r\n\n");
  await expect.element(save).toBeEnabled();
  await save.click();
  await expect
    .poll(() => settings.values.artifactPrompt)
    .toBe("文件放到 {dir}");
  expect(patches()).toEqual([{ artifactPrompt: "文件放到 {dir}" }]);
  await expect.element(input).toHaveValue("文件放到 {dir}");
  await expect.element(save).toBeDisabled();

  await page.getByRole("button", { name: "恢复默认" }).click();
  await expect.poll(() => settings.values.artifactPrompt).toBeNull();
  expect(patches().at(-1)).toEqual({ artifactPrompt: null });
  await expect.element(input).toHaveValue(BUILTIN_ARTIFACT_PROMPT);
});

test("keeps typed input but disables saving while settings are unavailable", async () => {
  await seedSettings({ artifactPrompt: "旧 {dir}" });
  render(ArtifactPromptSetting);
  const input = page.getByRole("textbox", { name: "临时文件夹提示词" });
  await expect.element(input).toHaveValue("旧 {dir}");
  await input.fill("新 {dir}");
  stopSettings();
  await expect
    .element(page.getByRole("button", { name: "保存", exact: true }))
    .toBeDisabled();
  await expect
    .element(page.getByRole("button", { name: "恢复默认" }))
    .toBeDisabled();
  await expect.element(input).toHaveValue("新 {dir}");
});
