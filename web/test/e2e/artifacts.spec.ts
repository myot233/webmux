import { readFile, stat } from "node:fs/promises";
import type { Page } from "@playwright/test";
import {
  test,
  expect,
  createShell,
  headerAction,
  recordInput,
} from "./fixtures";

const quick = (page: Page) =>
  page.getByRole("button", { name: "快捷指令", exact: true });

test("uploads land in the session folder the code panel browses", async ({
  page,
  workDir,
}, info) => {
  test.skip(
    info.project.use.isMobile === true,
    "The side-by-side code panel is a desktop layout",
  );
  const id = await createShell(page, "Artifacts upload", workDir);
  const response = page.waitForResponse((r) =>
    r.url().endsWith(`/api/sessions/${id}/upload`),
  );
  const chooser = page.waitForEvent("filechooser");
  await (await headerAction(page, "上传文件")).click();
  await (
    await chooser
  ).setFiles({
    name: "shot.txt",
    mimeType: "text/plain",
    buffer: Buffer.from("ARTIFACT_CONTENT\n"),
  });
  const { path } = await (await response).json();
  expect(path).toMatch(
    new RegExp(`/webmux-artifacts-\\d+-[0-9a-f]{8}/${id}/uploads/shot\\.txt$`),
  );
  expect(path.startsWith(workDir)).toBe(false);

  await page
    .locator(".terminal-header")
    .getByRole("button", { name: "代码", exact: true })
    .click();
  const code = page.getByRole("region", { name: "代码工作区" });
  await expect(code).toBeVisible();
  await code.getByRole("tab", { name: "临时文件夹", exact: true }).click();
  await expect(
    code.getByRole("tab", { name: "修改", exact: true }),
  ).toHaveCount(0);
  await code.getByRole("button", { name: "uploads", exact: true }).click();
  await code.getByRole("button", { name: "shot.txt", exact: true }).click();
  await expect(code.getByText("ARTIFACT_CONTENT")).toBeVisible();
  await expect(
    code.getByRole("button", { name: "查看 Diff", exact: true }),
  ).toHaveCount(0);

  await code.getByRole("tab", { name: "工作区", exact: true }).click();
  await expect(
    code.getByRole("tab", { name: "修改", exact: true }),
  ).toBeVisible();
});

test("the quick menu pastes the folder prompt without Enter, and removal deletes the folder", async ({
  page,
  request,
  workDir,
}) => {
  const id = await createShell(page, "Artifacts prompt", workDir);
  const received = await recordInput(page, workDir);
  await quick(page).click();
  await page
    .getByRole("menuitem", { name: "插入临时文件夹提示词", exact: true })
    .click();
  const folder = await (
    await request.post(`/api/sessions/${id}/artifacts`)
  ).json();
  expect((await stat(folder.path)).isDirectory()).toBe(true);
  await expect
    .poll(async () => (await readFile(received, "utf8")).endsWith("\x1b[201~"))
    .toBe(true);
  const bytes = await readFile(received, "utf8");
  expect(bytes.startsWith(`\x1b[200~本会话的临时文件夹：${folder.path}`)).toBe(
    true,
  );
  expect(bytes).toContain(`${folder.path}/uploads/`);
  // Exactly one bracketed paste: nothing was submitted after it.
  expect(bytes.split("\x1b[200~")).toHaveLength(2);

  expect((await request.delete(`/api/sessions/${id}`)).status()).toBe(204);
  await expect
    .poll(async () =>
      (await request.delete(`/api/sessions/${id}?remove=1`)).status(),
    )
    .toBe(404);
  await expect
    .poll(() =>
      stat(folder.path).then(
        () => "present",
        () => "gone",
      ),
    )
    .toBe("gone");
  expect((await request.post(`/api/sessions/${id}/artifacts`)).status()).toBe(
    404,
  );
});
