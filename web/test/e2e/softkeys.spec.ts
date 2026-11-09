import { readFile } from "node:fs/promises";
import {
  test,
  expect,
  createShell,
  expectTerminalLine,
  recordInput,
} from "./fixtures";

test("mobile soft keys stay in one row with paste visible and all keys reachable", async ({
  page,
  workDir,
}, testInfo) => {
  test.skip(
    !testInfo.project.use.isMobile,
    "Soft keys are shown on touch devices",
  );
  await page.addInitScript(() => {
    Object.defineProperty(navigator.clipboard, "readText", {
      value: () =>
        Promise.reject(
          new DOMException("Permission denied", "NotAllowedError"),
        ),
    });
  });
  await createShell(page, "Single-row soft keys", workDir);
  const bar = page.getByRole("complementary", { name: "终端软键" });
  const paste = bar.getByRole("button", { name: "粘贴", exact: true });
  const keyboard = bar.getByRole("button", { name: "唤起或收起键盘" });
  const quick = bar.getByRole("button", { name: "快捷指令", exact: true });
  const gamepadToggle = bar.getByRole("button", {
    name: "展开摇杆",
    exact: true,
  });
  for (const width of [320, 390, 844]) {
    await page.setViewportSize({ width, height: 844 });
    await expect(gamepadToggle).toBeInViewport({ ratio: 1 });
    await expect(paste).toBeInViewport({ ratio: 1 });
    await expect(keyboard).toBeInViewport({ ratio: 1 });
    await expect(quick).toBeInViewport({ ratio: 1 });
    await expect(
      page
        .locator(".terminal-header")
        .getByRole("button", { name: "快捷指令" }),
    ).toBeHidden();
    const edges = {
      row: await bar.evaluate(
        (element) =>
          element.querySelector(".soft-row")!.getBoundingClientRect().right,
      ),
      quick: (await quick.boundingBox())!.x,
    };
    expect(
      edges.quick,
      "Quick commands sit right of the soft keys",
    ).toBeGreaterThanOrEqual(edges.row);
    const layout = await bar.evaluate((element) => {
      // The draft switch is hidden from 640px up and has no box.
      const buttons = [...element.querySelectorAll("button")]
        .map((button) => button.getBoundingClientRect())
        .filter((bounds) => bounds.width > 0);
      return {
        height: element.getBoundingClientRect().height,
        verticalSpread:
          Math.max(...buttons.map((bounds) => bounds.top)) -
          Math.min(...buttons.map((bounds) => bounds.top)),
        pageOverflow: document.documentElement.scrollWidth - innerWidth,
      };
    });
    expect(
      layout.verticalSpread,
      "All soft keys share one row",
    ).toBeLessThanOrEqual(1);
    expect(
      layout.height,
      "The toolbar must not consume two button rows",
    ).toBeLessThan(64);
    expect(layout.pageOverflow).toBe(0);
  }
  await page.setViewportSize({ width: 320, height: 844 });
  const before = await quick.boundingBox();
  await bar
    .getByRole("button", { name: "Ctrl+R", exact: true })
    .scrollIntoViewIfNeeded();
  expect(await quick.boundingBox()).toEqual(before);
  await expect(quick).toBeInViewport({ ratio: 1 });
  // Scroll offsets snap to device pixels (DPR 2.625 here), so the last key can
  // stay clipped by a fraction of a CSS pixel at maximum scroll.
  await expect(
    bar.getByRole("button", { name: "Ctrl+R", exact: true }),
  ).toBeInViewport({ ratio: 0.99 });
  await paste.scrollIntoViewIfNeeded();
  await expect(paste).toBeInViewport({ ratio: 1 });
  await keyboard.click();
  await expect(page.locator(".xterm-helper-textarea")).toBeFocused();
  await paste.click();
  await expect(page.getByRole("dialog", { name: "粘贴到终端" })).toBeVisible();
});

test("soft Shift arms Shift+Tab once and a long press keeps it locked", async ({
  page,
  workDir,
}, testInfo) => {
  test.skip(
    !testInfo.project.use.isMobile,
    "Soft keys are shown on touch devices",
  );
  await createShell(page, "Soft Shift", workDir);
  const received = await recordInput(page, workDir);
  const bar = page.getByRole("complementary", { name: "终端软键" });
  const shift = bar.getByRole("button", { name: /^Shift/ });
  const tab = bar.getByRole("button", { name: "Tab", exact: true });

  // A tap arms Shift for one key; Tab then sends Shift+Tab.
  await shift.tap();
  await tab.tap();
  await expect.poll(() => readFile(received, "utf8")).toBe("\x1b[Z");

  // A long press locks Shift, so two Tabs both carry the modifier.
  await shift.click({ delay: 1000 });
  await expect(shift.locator("svg")).toBeVisible();
  await tab.tap();
  await tab.tap();
  await expect
    .poll(() => readFile(received, "utf8"))
    .toBe("\x1b[Z\x1b[Z\x1b[Z");

  // Tapping the locked key releases it and Tab returns to a plain tab.
  await shift.tap();
  await expect(shift.locator("svg")).toHaveCount(0);
  await tab.tap();
  await expect
    .poll(() => readFile(received, "utf8"))
    .toBe("\x1b[Z\x1b[Z\x1b[Z\t");
});

test("mobile terminal taps leave input unfocused and only the keyboard button toggles focus", async ({
  page,
  workDir,
}, testInfo) => {
  test.skip(!testInfo.project.use.isMobile, "Requires native touch input");
  await createShell(page, "Explicit mobile keyboard", workDir);
  const input = page.locator(".xterm-helper-textarea");
  const screen = page.locator(".xterm-screen");
  const bar = page.getByRole("complementary", { name: "终端软键" });
  const keyboard = bar.getByRole("button", { name: "唤起或收起键盘" });

  await expect(input).not.toBeFocused();
  // Locator.tap uses the browser's native touch path, including compatibility
  // mouse events; dispatching just pointerup would miss accidental tap focus.
  await screen.tap();
  await expect(input).not.toBeFocused();
  await keyboard.tap();
  await expect(input).toBeFocused();
  await page.keyboard.type("printf 'mobile-keyboard-%s\\n' ready");
  await page.keyboard.press("Enter");
  await expectTerminalLine(page, "mobile-keyboard-ready");
  // Soft keys must not dismiss an open keyboard: the direction key sends its
  // sequence without moving focus off the terminal input.
  await bar.getByRole("button", { name: "←", exact: true }).tap();
  await expect(input).toBeFocused();
  await keyboard.tap();
  await expect(input).not.toBeFocused();
  await screen.tap();
  await expect(input).not.toBeFocused();
});

test("touch tablets with a fine primary pointer only raise the keyboard from the keyboard button", async ({
  page,
  workDir,
}, testInfo) => {
  test.skip(!testInfo.project.use.isMobile, "Requires a touch screen");
  // Many tablets (stylus, keyboard covers, Android desktop modes) report a
  // fine primary pointer while still raising a virtual keyboard on focus.
  await page.addInitScript(() => {
    const native = window.matchMedia.bind(window);
    window.matchMedia = (query: string) => {
      const result = native(query);
      if (
        !query.includes("any-pointer") &&
        /pointer: (fine|coarse)/.test(query)
      )
        Object.defineProperty(result, "matches", {
          value: query.includes("pointer: fine"),
        });
      return result;
    };
  });
  const id = await createShell(page, "Tablet keyboard", workDir);
  const input = page.locator(".xterm-helper-textarea");
  const keyboard = page
    .getByRole("complementary", { name: "终端软键" })
    .getByRole("button", { name: "唤起或收起键盘" });

  await expect(keyboard).toBeVisible();
  await expect(input).not.toBeFocused();
  await page.goto(`/s/${id}`);
  await expect(page.locator(".restore-overlay")).toBeHidden();
  await expect(input).not.toBeFocused();

  // A stylus tap arrives as a pen pointer, not a touch pointer.
  const bounds = (await page.locator(".xterm-screen").boundingBox())!;
  const cdp = await page.context().newCDPSession(page);
  const pen = {
    x: bounds.x + bounds.width / 2,
    y: bounds.y + bounds.height / 2,
    button: "left" as const,
    clickCount: 1,
    pointerType: "pen" as const,
  };
  await cdp.send("Input.dispatchMouseEvent", { type: "mousePressed", ...pen });
  await cdp.send("Input.dispatchMouseEvent", { type: "mouseReleased", ...pen });
  await cdp.detach();
  await expect(input).not.toBeFocused();

  await page.getByRole("button", { name: "终端菜单", exact: true }).tap();
  await page.getByRole("menuitem", { name: "字体大小", exact: true }).tap();
  const dialog = page.getByRole("dialog", { name: "字体大小" });
  await expect(dialog).toBeVisible();
  await expect(dialog.getByRole("slider")).toHaveAttribute(
    "aria-valuemin",
    "8",
  );
  await page.keyboard.press("Escape");
  await expect(dialog).toBeHidden();
  await expect(input).not.toBeFocused();

  await keyboard.tap();
  await expect(input).toBeFocused();
});

test("locking the terminal size keeps the PTY grid when the mobile viewport shrinks", async ({
  page,
  workDir,
}, testInfo) => {
  test.skip(!testInfo.project.use.isMobile, "Requires the mobile viewport");
  const frames: Buffer[] = [];
  page.on("websocket", (socket) =>
    socket.on("framesent", ({ payload }) => {
      frames.push(Buffer.isBuffer(payload) ? payload : Buffer.from(payload));
    }),
  );
  await createShell(page, "Locked terminal size", workDir);
  const screen = page.locator(".xterm-screen");
  const grid = (await screen.boundingBox())!;
  const sizeFrames = () => frames.filter((frame) => frame[0] === 1).length;

  await page.getByRole("button", { name: "终端菜单", exact: true }).tap();
  const lock = page.getByRole("menuitemcheckbox", {
    name: "锁定尺寸",
    exact: true,
  });
  await lock.tap();
  await expect(lock).toBeChecked();
  await page.keyboard.press("Escape");
  await expect(lock).toBeHidden();

  const baseline = sizeFrames();
  const viewport = page.viewportSize()!;
  const keyboardHeight = viewport.height - 200;
  await page.setViewportSize({
    width: viewport.width,
    height: keyboardHeight,
  });
  await page.waitForTimeout(300);
  expect(sizeFrames()).toBe(baseline);
  const locked = (await screen.boundingBox())!;
  expect(locked.width).toBe(grid.width);
  expect(locked.height).toBe(grid.height);
  // The grid stays bottom-anchored, so the newest prompt row remains visible.
  const prompt = page
    .locator(".xterm-rows > div", { hasText: "webmux-e2e$" })
    .last();
  const promptBox = (await prompt.boundingBox())!;
  expect(promptBox.y + promptBox.height).toBeLessThanOrEqual(keyboardHeight);
});

test("opening the keyboard from the keyboard button keeps the PTY grid until it closes", async ({
  page,
  workDir,
}, testInfo) => {
  test.skip(!testInfo.project.use.isMobile, "Requires the mobile viewport");
  const frames: Buffer[] = [];
  page.on("websocket", (socket) =>
    socket.on("framesent", ({ payload }) => {
      frames.push(Buffer.isBuffer(payload) ? payload : Buffer.from(payload));
    }),
  );
  await createShell(page, "Keyboard size hold", workDir);
  const screen = page.locator(".xterm-screen");
  const input = page.locator(".xterm-helper-textarea");
  const keyboard = page
    .getByRole("complementary", { name: "终端软键" })
    .getByRole("button", { name: "唤起或收起键盘" });
  const grid = (await screen.boundingBox())!;
  const sizeFrames = () => frames.filter((frame) => frame[0] === 1).length;
  const viewport = page.viewportSize()!;
  const keyboardHeight = viewport.height - 300;
  // Chromium emulation has no soft keyboard; shrinking the viewport stands in
  // for its open and close animation.
  const openKeyboard = () =>
    page.setViewportSize({ width: viewport.width, height: keyboardHeight });
  const closeKeyboard = () => page.setViewportSize(viewport);

  const baseline = sizeFrames();
  await keyboard.tap();
  await expect(input).toBeFocused();
  await openKeyboard();
  await page.waitForTimeout(400);
  expect(sizeFrames()).toBe(baseline);
  expect((await screen.boundingBox())!.height).toBe(grid.height);
  const prompt = page
    .locator(".xterm-rows > div", { hasText: "webmux-e2e$" })
    .last();
  const promptBox = (await prompt.boundingBox())!;
  expect(promptBox.y + promptBox.height).toBeLessThanOrEqual(keyboardHeight);

  // The button blurs before the keyboard animates away; the hold outlasts it.
  await keyboard.tap();
  await expect(input).not.toBeFocused();
  await page.waitForTimeout(300);
  await closeKeyboard();
  await page.waitForTimeout(600);
  expect(sizeFrames()).toBe(baseline);

  const menu = page.getByRole("button", { name: "终端菜单", exact: true });
  const option = page.getByRole("menuitemcheckbox", {
    name: "键盘弹出时保持尺寸",
    exact: true,
  });
  await menu.tap();
  await expect(option).toBeChecked();
  await option.tap();
  await expect(option).not.toBeChecked();
  await page.keyboard.press("Escape");
  await page.reload();
  await expect(screen).toBeVisible();
  await menu.tap();
  await expect(option).not.toBeChecked();
  await page.keyboard.press("Escape");
  await expect(option).toBeHidden();

  const unheld = sizeFrames();
  await keyboard.tap();
  await expect(input).toBeFocused();
  await openKeyboard();
  await expect.poll(sizeFrames).toBeGreaterThan(unheld);
});

test("opening the paste dialog while the keyboard is held does not resize the PTY", async ({
  page,
  workDir,
}, testInfo) => {
  test.skip(!testInfo.project.use.isMobile, "Requires the mobile viewport");
  await page.addInitScript(() => {
    Object.defineProperty(navigator.clipboard, "readText", {
      value: () =>
        Promise.reject(
          new DOMException("Permission denied", "NotAllowedError"),
        ),
    });
  });
  const frames: Buffer[] = [];
  page.on("websocket", (socket) =>
    socket.on("framesent", ({ payload }) => {
      frames.push(Buffer.isBuffer(payload) ? payload : Buffer.from(payload));
    }),
  );
  await createShell(page, "Paste while held", workDir);
  const screen = page.locator(".xterm-screen");
  const input = page.locator(".xterm-helper-textarea");
  const bar = page.getByRole("complementary", { name: "终端软键" });
  const keyboard = bar.getByRole("button", { name: "唤起或收起键盘" });
  const paste = bar.getByRole("button", { name: "粘贴", exact: true });
  const sizeFrames = () => frames.filter((frame) => frame[0] === 1).length;
  const viewport = page.viewportSize()!;
  const keyboardHeight = viewport.height - 300;
  const grid = (await screen.boundingBox())!;

  const baseline = sizeFrames();
  await keyboard.tap();
  await expect(input).toBeFocused();
  await page.setViewportSize({ width: viewport.width, height: keyboardHeight });
  await page.waitForTimeout(400);
  expect(sizeFrames()).toBe(baseline);

  // The dialog takes focus while the simulated keyboard stays up; the hold
  // must outlast the close limit instead of fitting the compressed area.
  await paste.tap();
  const dialog = page.getByRole("dialog", { name: "粘贴到终端" });
  await expect(dialog).toBeVisible();
  await page.waitForTimeout(1500);
  expect(sizeFrames()).toBe(baseline);
  expect((await screen.boundingBox())!.height).toBe(grid.height);

  await page.keyboard.press("Escape");
  await expect(dialog).toBeHidden();
  await page.setViewportSize(viewport);
  await page.waitForTimeout(600);
  expect(sizeFrames()).toBe(baseline);
});

test("desktop devices can force the soft-key toolbar on and off through the terminal menu", async ({
  page,
  workDir,
}, testInfo) => {
  test.skip(testInfo.project.use.isMobile, "Exercises a non-coarse pointer");
  await createShell(page, "Manual soft-key toolbar", workDir);
  const bar = page.getByRole("complementary", { name: "终端软键" });
  const menu = page.getByRole("button", { name: "终端菜单", exact: true });
  const override = page.getByRole("menuitemcheckbox", {
    name: "强制显示底部工具栏",
    exact: true,
  });
  const input = page.locator(".xterm-helper-textarea");
  const keyboard = bar.getByRole("button", { name: "唤起或收起键盘" });

  await expect(bar).toBeHidden();
  await menu.click();
  await expect(override).not.toBeChecked();
  await override.click();
  await page.keyboard.press("Escape");
  await expect(bar).toBeVisible();
  await expect(keyboard).toBeInViewport({ ratio: 1 });
  await keyboard.click();
  await expect(input).toBeFocused();
  await page.keyboard.type("printf 'manual-toolbar-%s\\n' ready");
  await page.keyboard.press("Enter");
  await expectTerminalLine(page, "manual-toolbar-ready");
  await keyboard.click();
  await expect(input).not.toBeFocused();

  await bar.getByRole("button", { name: "展开摇杆", exact: true }).click();
  const gamepad = bar.getByRole("toolbar", { name: "终端摇杆" });
  await expect(gamepad).toBeVisible();
  await expect(
    bar.getByRole("button", { name: "收起摇杆", exact: true }),
  ).toHaveAttribute("aria-expanded", "true");

  await menu.click();
  await expect(override).toBeChecked();
  await override.click();
  await page.keyboard.press("Escape");
  await expect(bar).toBeHidden();
  await expect(gamepad).toBeHidden();
  await menu.click();
  await expect(override).not.toBeChecked();
  await page.keyboard.press("Escape");
});

test("touch gamepad mode lays out inside the soft-key bar without covering the terminal, follows control, and deduplicates bottom keys", async ({
  page,
  browser,
  workDir,
}, testInfo) => {
  test.skip(
    !testInfo.project.use.isMobile,
    "Exercises mobile touch gamepad and touch scrolling",
  );
  const id = await createShell(page, "Touch gamepad", workDir);
  const bar = page.getByRole("complementary", { name: "终端软键" });
  const gamepad = bar.getByRole("toolbar", { name: "终端摇杆" });
  const gamepadToggle = bar.getByRole("button", {
    name: "展开摇杆",
    exact: true,
  });

  // Initially off: bottom row has Esc, Tab, Enter, Ctrl, arrows, Ctrl+C.
  await expect(gamepad).toBeHidden();
  await expect(
    bar.getByRole("button", { name: "Esc", exact: true }),
  ).toBeVisible();
  await expect(
    bar.getByRole("button", { name: "↵", exact: true }),
  ).toBeVisible();

  // Expand the gamepad from the fixed soft-key action.
  await expect(gamepadToggle).toHaveAttribute("aria-expanded", "false");
  await gamepadToggle.tap();
  await expect(
    bar.getByRole("button", { name: "收起摇杆", exact: true }),
  ).toHaveAttribute("aria-expanded", "true");

  // Controlling client sees the gamepad in the bar and a deduplicated key row.
  await expect(gamepad).toBeVisible();
  for (const button of await bar.locator(".soft-trailing button").all()) {
    await expect(button).toBeInViewport({ ratio: 1 });
  }
  for (const button of await gamepad.getByRole("button").all()) {
    await expect(button).toBeInViewport({ ratio: 1 });
  }
  const terminalBox = await page.locator(".terminal-area").boundingBox();
  const gamepadBox = await gamepad.boundingBox();
  const barBox = await bar.boundingBox();
  expect(terminalBox).not.toBeNull();
  expect(gamepadBox).not.toBeNull();
  expect(barBox).not.toBeNull();
  expect(gamepadBox!.y).toBeGreaterThanOrEqual(
    terminalBox!.y + terminalBox!.height,
  );
  expect(gamepadBox!.y).toBeGreaterThanOrEqual(barBox!.y);
  expect(gamepadBox!.y + gamepadBox!.height).toBeLessThanOrEqual(
    barBox!.y + barBox!.height,
  );
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true);
  for (const label of ["Esc", "Tab", "↵", "↑", "↓", "←", "→", "Ctrl+C"]) {
    await expect(
      bar
        .locator(".soft-row")
        .getByRole("button", { name: label, exact: true }),
    ).toHaveCount(0);
  }
  await expect(
    bar.locator(".soft-row").getByRole("button", { name: "Ctrl", exact: true }),
  ).toBeVisible();
  await expect(
    bar.locator(".soft-row").getByRole("button", { name: "Alt", exact: true }),
  ).toBeVisible();

  // Open a second client that takes control, turning this mobile page into an observer.
  const otherContext = await browser.newContext();
  const otherPage = await otherContext.newPage();
  try {
    await otherPage.goto(`/s/${id}`);
    await otherPage
      .getByRole("button", { name: "接管控制", exact: true })
      .click();
    // Observer must not show the gamepad, and bottom row restores full keys.
    await expect(
      page.getByRole("button", { name: "接管控制", exact: true }),
    ).toBeVisible();
    await expect(gamepad).toBeHidden();
    await expect(
      bar
        .locator(".soft-row")
        .getByRole("button", { name: "Esc", exact: true }),
    ).toBeVisible();

    // Claiming control back restores the gamepad.
    await page.getByRole("button", { name: "接管控制", exact: true }).tap();
    await expect(gamepad).toBeVisible();
  } finally {
    await otherContext.close();
  }

  // Verify gamepad keys send input (including Enter and Space).
  const received = await recordInput(page, workDir);
  await gamepad.getByRole("button", { name: "Esc", exact: true }).tap();
  await gamepad.getByRole("button", { name: "空格", exact: true }).tap();
  await gamepad.getByRole("button", { name: "回车", exact: true }).tap();
  await expect.poll(() => readFile(received, "utf8")).toBe("\x1b \r");

  await bar.getByRole("button", { name: "收起摇杆", exact: true }).tap();
  await expect(gamepad).toBeHidden();
  await expect(
    bar.locator(".soft-row").getByRole("button", { name: "Esc", exact: true }),
  ).toBeVisible();
});
