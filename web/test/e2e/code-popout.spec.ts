import { execFileSync } from "node:child_process";
import { mkdir, readFile, writeFile } from "node:fs/promises";
import { join } from "node:path";
import type { Frame, FrameLocator, Locator, Page } from "@playwright/test";
import { test, expect, createShell } from "./fixtures";

function git(root: string, ...args: string[]) {
  execFileSync("git", ["-C", root, ...args], {
    env: {
      ...process.env,
      GIT_AUTHOR_NAME: "E2E",
      GIT_AUTHOR_EMAIL: "e2e@example.com",
      GIT_COMMITTER_NAME: "E2E",
      GIT_COMMITTER_EMAIL: "e2e@example.com",
    },
  });
}
const lines = (count: number, label = "line") =>
  Array.from({ length: count }, (_, n) => `${label} ${n + 1}`).join("\n") +
  "\n";

// The pop-out window only holds a frame running its own app instance.
function frameOf(win: Page) {
  return win.frameLocator('iframe[title="代码工作区"]');
}
type Scope = Page | FrameLocator;
function showCode(page: Page) {
  return page
    .locator(".terminal-header")
    .getByRole("button", { name: "代码", exact: true })
    .click();
}
function popOutButton(page: Page) {
  return page.getByRole("button", {
    name: "在独立窗口中显示代码",
    exact: true,
  });
}
function sendButton(scope: Scope, range?: string) {
  return scope
    .getByRole("button", {
      name: range ? `发送位置到终端（${range} 行）` : /^发送位置到/,
    })
    .filter({ visible: true });
}
// First line visible at the top of the active code reader.
function topLine(scope: Scope) {
  return scope
    .locator("[data-code-reader]:visible .cm-scroller")
    .evaluate((scroller) => {
      const top = scroller.getBoundingClientRect().top;
      const row = [
        ...scroller.querySelectorAll<HTMLElement>(
          ".cm-lineNumbers .cm-gutterElement",
        ),
      ].find(
        (element) =>
          element.textContent &&
          element.getBoundingClientRect().bottom > top + 2,
      );
      return Number(row?.textContent);
    });
}
async function gotoLine(scope: Scope, line: number) {
  // Font loading and closing the jump panel both change the virtualized
  // gutter's geometry. Finish those transitions before clicking a line number.
  await scope.locator("[data-code-reader]:visible").evaluate(async (reader) => {
    await reader.ownerDocument.fonts.ready;
  });
  await scope
    .getByRole("button", { name: "跳转行", exact: true })
    .filter({ visible: true })
    .click();
  const input = scope.locator("[data-code-goto-line] input");
  await input.fill(String(line));
  await input.press("Enter");
  await expect(input).toHaveCount(0);
  await expect(gutter(scope, line)).toBeInViewport();
}
function gutter(scope: Scope, line: number) {
  return scope
    .locator("[data-code-reader]:visible .cm-lineNumbers .cm-gutterElement")
    .filter({ hasText: new RegExp(`^${line}$`) });
}
function diffNumber(diff: Locator, line: number) {
  return diff
    .locator(
      `[data-column-number="${line}"]:not([data-line-type="change-deletion"]):not([data-deletions] *)`,
    )
    .first();
}
async function closeFrom(win: Page) {
  const closed = win.waitForEvent("close");
  // This control closes the window it lives in, so its click response can be
  // lost with the window; the close event is the assertion that matters.
  void frameOf(win)
    .getByRole("button", { name: "收回代码窗口", exact: true })
    .click()
    .catch(() => undefined);
  await closed;
}

test("the code panel pops out into a framed window and both ways keeps its reading state", async ({
  page,
  workDir,
  request,
}, testInfo) => {
  test.skip(
    testInfo.project.name !== "chromium-desktop",
    "Document Picture-in-Picture is a desktop browser feature",
  );
  git(workDir, "init", "-q");
  await writeFile(join(workDir, "long.txt"), lines(400));
  await writeFile(join(workDir, "diff.txt"), lines(100, "row"));
  await writeFile(join(workDir, "other.txt"), "other document\n");
  git(workDir, "add", ".");
  git(workDir, "commit", "-qm", "initial");
  await writeFile(
    join(workDir, "diff.txt"),
    lines(100, "row").replace("row 50\n", "changed 50\n"),
  );
  await createShell(page, "Popout state", workDir);
  await showCode(page);
  const panel = page.getByRole("region", { name: "代码工作区" });

  // A long file scrolled to line 200 with lines 195-198 selected.
  await page.getByRole("button", { name: "long.txt", exact: true }).click();
  await gotoLine(page, 200);
  await gutter(page, 195).click();
  await expect(sendButton(page, "195")).toBeEnabled();
  await gutter(page, 198).click({ modifiers: ["Shift"] });
  await expect(sendButton(page, "195-198")).toBeEnabled();
  const top = await topLine(page);
  expect(top).toBeGreaterThan(150);

  // A diff with unmodified context expanded and one line selected.
  await page.getByRole("tab", { name: "修改", exact: true }).click();
  await page
    .getByRole("button", { name: "修改 diff.txt", exact: true })
    .first()
    .click();
  const diff = page.locator("diffs-container:visible").first();
  await expect(page.locator(".code-diff-reader:visible")).toContainText(
    "changed 50",
  );
  await diffNumber(diff, 50).click();
  await expect(sendButton(page, "50")).toBeEnabled();
  await expect(diff).toContainText("46 行未修改");
  await diff.getByRole("button", { name: "向下展开 30 行" }).first().click();
  await expect(diff).toContainText("16 行未修改");
  await expect(sendButton(page, "50")).toBeEnabled();

  const opened = page.context().waitForEvent("page");
  await popOutButton(page).click();
  const pip = await opened;
  const frame = frameOf(pip);
  await expect(page.getByText("代码已在独立窗口中显示")).toBeVisible();
  await expect(panel).toHaveCount(0);
  const popped = frame.getByRole("region", { name: "代码工作区" });
  await expect(popped).toBeVisible();
  // The frame fills the window.
  const size = await pip.evaluate(() => [innerWidth, innerHeight]);
  const box = (await pip.locator("iframe").boundingBox())!;
  expect([box.width, box.height]).toEqual(size);

  // The diff tab is still active, expanded and selected.
  await expect(frame.getByRole("tab", { name: /^diff\.txt/ })).toHaveAttribute(
    "aria-selected",
    "true",
  );
  await expect(frame.locator("diffs-container:visible")).toContainText(
    "16 行未修改",
  );
  await expect(sendButton(frame, "50")).toBeEnabled();
  // The file tab keeps its position and selection.
  await frame.getByRole("tab", { name: /^long\.txt/ }).click();
  await expect(sendButton(frame, "195-198")).toBeEnabled();
  await expect.poll(() => topLine(frame)).toBe(top);

  // Server settings, such as the theme, reach the window.
  const settings = await (await request.get("/api/settings")).json();
  await request.patch("/api/settings", {
    data: { baseRevision: settings.revision, changes: { theme: "dark" } },
  });
  await expect(frame.locator("html")).toHaveClass(/\bdark\b/);
  const latest = await (await request.get("/api/settings")).json();
  await request.patch("/api/settings", {
    data: { baseRevision: latest.revision, changes: { theme: null } },
  });

  // Menus and dialogs open inside the window, at the pointer.
  await frame.getByRole("tab", { name: "文件", exact: true }).click();
  const row = frame.getByRole("button", { name: "other.txt", exact: true });
  const rowBox = (await row.boundingBox())!;
  await row.click({ button: "right", position: { x: 20, y: 5 } });
  const menu = frame.getByRole("menu", { name: "文件操作 other.txt" });
  await expect(menu).toBeVisible();
  const placed = (await menu.boundingBox())!;
  expect(Math.abs(placed.x - (rowBox.x + 20))).toBeLessThan(16);
  expect(Math.abs(placed.y - (rowBox.y + 5))).toBeLessThan(16);
  await expect(page.getByRole("menu")).toHaveCount(0);
  await menu.getByRole("menuitem", { name: "重命名" }).click();
  await expect(frame.getByRole("dialog")).toBeVisible();
  await expect(frame.getByRole("dialog").getByRole("textbox")).toBeFocused();
  await frame.getByRole("dialog").getByRole("textbox").press("Escape");
  await expect(frame.getByRole("dialog")).toHaveCount(0);

  // State changed in the window returns with it.
  await gotoLine(frame, 320);
  await gutter(frame, 318).click();
  await expect(sendButton(frame, "318")).toBeEnabled();
  const poppedTop = await topLine(frame);
  expect(poppedTop).toBeGreaterThan(top);
  await closeFrom(pip);
  await expect(page.getByText("代码已在独立窗口中显示")).toHaveCount(0);
  await expect(panel).toBeVisible();
  await expect(page.getByRole("tab", { name: /^long\.txt/ })).toHaveAttribute(
    "aria-selected",
    "true",
  );
  await expect(sendButton(page, "318")).toBeEnabled();
  await expect.poll(() => topLine(page)).toBe(poppedTop);
  await page.getByRole("tab", { name: /^diff\.txt/ }).click();
  await expect(page.locator("diffs-container:visible")).toContainText(
    "16 行未修改",
  );
  await expect(sendButton(page, "50")).toBeEnabled();
});

test("closing the opener's code view keeps the popped-out panel live", async ({
  page,
  workDir,
}, testInfo) => {
  test.skip(
    testInfo.project.name !== "chromium-desktop",
    "Document Picture-in-Picture is a desktop browser feature",
  );
  await writeFile(join(workDir, "live.txt"), "before\n");
  await createShell(page, "Popout live", workDir);
  const toggle = page
    .locator(".terminal-header")
    .getByRole("button", { name: /^(代码|折叠代码)$/ });
  await toggle.click();
  const opened = page.context().waitForEvent("page");
  await popOutButton(page).click();
  const frame = frameOf(await opened);
  await frame.getByRole("button", { name: "live.txt", exact: true }).click();
  await expect(frame.locator(".cm-content")).toContainText("before");
  // Hiding the code pane in the opener must not pause the popped-out panel's
  // update checks.
  await toggle.click();
  await writeFile(join(workDir, "live.txt"), "after\n");
  await expect(
    frame.getByRole("tab", { name: /^live\.txt · worktree ●$/ }),
  ).toBeVisible({ timeout: 15_000 });
});

test("the popped-out window selects line ranges and inserts references into the opener's terminal", async ({
  page,
  workDir,
  request,
}, testInfo) => {
  test.skip(
    testInfo.project.name !== "chromium-desktop",
    "Document Picture-in-Picture is a desktop browser feature",
  );
  git(workDir, "init", "-q");
  await writeFile(join(workDir, "pick.txt"), "one\ntwo\nthree\n");
  git(workDir, "add", ".");
  git(workDir, "commit", "-qm", "initial");
  await writeFile(join(workDir, "pick.txt"), "one\nTWO\nTHREE\n");
  const id = await createShell(page, "Popout pick", workDir);
  await page.locator(".xterm-helper-textarea").focus();
  await page.keyboard.type("printf 'reference: %s\\n' ");
  await showCode(page);
  const opened = page.context().waitForEvent("page");
  await popOutButton(page).click();
  const frame = frameOf(await opened);

  // File view: CodeMirror gutter.
  await frame.getByRole("button", { name: "pick.txt", exact: true }).click();
  await gutter(frame, 1).click();
  await gutter(frame, 3).click({ modifiers: ["Shift"] });
  await expect(sendButton(frame, "1-3")).toBeEnabled();

  // Diff view: the diff library tracks the pointer on its own document.
  await frame.getByRole("tab", { name: "修改", exact: true }).click();
  await frame
    .getByRole("button", { name: "修改 pick.txt", exact: true })
    .first()
    .click();
  const diff = frame.locator("diffs-container").first();
  await expect(frame.locator(".code-diff-reader").first()).toContainText("TWO");
  await diffNumber(diff, 2).click();
  await expect(sendButton(frame, "2")).toBeEnabled();
  await diffNumber(diff, 3).click({ modifiers: ["Shift"] });
  await expect(sendButton(frame, "2-3")).toBeEnabled();
  // A new click after a range starts a new selection.
  await diffNumber(diff, 2).click();
  await sendButton(frame, "2").click();
  await expect(frame.getByRole("alert")).toHaveCount(0);

  // The reference reached the opener's terminal input.
  const buffer = async () =>
    await (await request.get(`/api/sessions/${id}/buffer`)).text();
  // Insertion is asynchronous; submit only once the input shows it.
  await expect
    .poll(async () =>
      ((await page.locator(".xterm-rows").textContent()) ?? "").replaceAll(
        "\u00a0",
        " ",
      ),
    )
    .toContain(`${join(workDir, "pick.txt")}:2`);
  await page.locator(".xterm-helper-textarea").focus();
  await page.keyboard.press("Enter");
  await expect
    .poll(buffer)
    .toContain(`reference: ${join(workDir, "pick.txt")}:2\r\n`);
});

test("a narrow pop-out window lays out for its own width", async ({
  page,
  workDir,
}, testInfo) => {
  test.skip(
    testInfo.project.name !== "chromium-desktop",
    "Document Picture-in-Picture is a desktop browser feature",
  );
  await writeFile(join(workDir, "narrow.txt"), "narrow document\n");
  await createShell(page, "Popout narrow", workDir);
  await showCode(page);
  const opened = page.context().waitForEvent("page");
  await popOutButton(page).click();
  const pip = await opened;
  await pip.setViewportSize({ width: 520, height: 600 });
  const frame = frameOf(pip);
  await frame
    .getByRole("button", { name: "文件操作 narrow.txt", exact: true })
    .click();
  // Below 640px file actions use a bottom sheet, in the window itself.
  await expect(frame.getByRole("dialog")).toBeVisible();
  await expect(page.getByRole("dialog")).toHaveCount(0);
});

// Drops `name` onto `target` as the system's file manager would.
async function dropFile(
  context: Page | Frame,
  target: Locator,
  name: string,
  text: string,
) {
  const transfer = await context.evaluateHandle(
    ([name, text]) => {
      const transfer = new DataTransfer();
      transfer.items.add(new File([text], name, { type: "text/plain" }));
      return transfer;
    },
    [name, text] as const,
  );
  for (const type of ["dragenter", "dragover", "drop"])
    await target.dispatchEvent(type, { dataTransfer: transfer });
}

test("files dropped on the file tree upload into the folder, in the page and in the window", async ({
  page,
  workDir,
}, testInfo) => {
  test.skip(
    testInfo.project.name !== "chromium-desktop",
    "Document Picture-in-Picture is a desktop browser feature",
  );
  await mkdir(join(workDir, "docs"));
  await writeFile(join(workDir, "docs", "a.txt"), "a\n");
  await createShell(page, "Drop upload", workDir);
  await showCode(page);
  await dropFile(
    page,
    page.getByRole("button", { name: "docs", exact: true }),
    "dropped.txt",
    "from the page\n",
  );
  await expect
    .poll(() =>
      readFile(join(workDir, "docs", "dropped.txt"), "utf8").catch(() => ""),
    )
    .toBe("from the page\n");

  const opened = page.context().waitForEvent("page");
  await popOutButton(page).click();
  const pip = await opened;
  const frame = frameOf(pip);
  await frame.getByRole("button", { name: "docs", exact: true }).click();
  const row = frame.getByRole("button", { name: "a.txt", exact: true });
  await expect(row).toBeVisible();
  const context = (await pip.locator("iframe").elementHandle())!;
  // A file row receives into its folder; a taken name gets a suffix.
  await dropFile(
    (await context.contentFrame())!,
    row,
    "dropped.txt",
    "from the window\n",
  );
  await expect
    .poll(() =>
      readFile(join(workDir, "docs", "dropped (2).txt"), "utf8").catch(
        () => "",
      ),
    )
    .toBe("from the window\n");
  await expect(
    frame.getByRole("button", { name: "dropped (2).txt", exact: true }),
  ).toBeVisible();
});

// Safari, Firefox and plain-HTTP origins have no Document Picture-in-Picture.
function withoutPictureInPicture(page: Page) {
  return page.addInitScript(() => {
    Reflect.deleteProperty(window, "documentPictureInPicture");
  });
}

test("without Picture-in-Picture a desktop browser pops out into an ordinary popup", async ({
  page,
  workDir,
}, testInfo) => {
  test.skip(
    testInfo.project.name !== "chromium-desktop",
    "mobile browsers get no pop-out control without Picture-in-Picture",
  );
  await withoutPictureInPicture(page);
  await writeFile(join(workDir, "popup.txt"), "popup document\nsecond\n");
  await createShell(page, "Popup code", workDir);
  await showCode(page);
  expect(await page.evaluate(() => "documentPictureInPicture" in window)).toBe(
    false,
  );
  const opened = page.waitForEvent("popup");
  await popOutButton(page).click();
  const popup = await opened;
  const frame = frameOf(popup);
  await expect(page.getByText("代码已在独立窗口中显示")).toBeVisible();
  await frame.getByRole("button", { name: "popup.txt", exact: true }).click();
  await expect(frame.locator(".cm-content")).toContainText("popup document");
  await expect(frame.locator(".cm-editor")).toHaveCSS("display", "flex");
  await gutter(frame, 2).click();
  await expect(sendButton(frame, "2")).toBeEnabled();

  // Closing the popup with the window control returns the latest state.
  await popup.close();
  await expect(page.getByText("代码已在独立窗口中显示")).toHaveCount(0);
  await expect(
    page.locator("[data-code-reader]:visible .cm-content"),
  ).toContainText("popup document");
  await expect(sendButton(page, "2")).toBeEnabled();

  // Reloading the opener closes a popup instead of stranding it.
  const again = page.waitForEvent("popup");
  await popOutButton(page).click();
  const second = await again;
  const closed = second.waitForEvent("close");
  await page.reload();
  await closed;
});

test("the frame route opened directly points back to the session", async ({
  page,
  workDir,
}) => {
  const id = await createShell(page, "Direct code", workDir);
  await page.goto(`/code/${id}`);
  await expect(page.getByText("独立代码窗口需从会话页面打开。")).toBeVisible();
  await page.getByRole("link", { name: "返回会话" }).click();
  await expect(page).toHaveURL(new RegExp(`/s/${id}$`));
});

test("mobile browsers without Picture-in-Picture offer no pop-out", async ({
  page,
  workDir,
}, testInfo) => {
  test.skip(testInfo.project.name === "chromium-desktop", "touch-only device");
  await withoutPictureInPicture(page);
  await writeFile(join(workDir, "mobile.txt"), "mobile document\n");
  await createShell(page, "Mobile code", workDir);
  await page.getByRole("button", { name: "代码", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "mobile.txt", exact: true }),
  ).toBeVisible();
  await expect(popOutButton(page)).toHaveCount(0);
});
