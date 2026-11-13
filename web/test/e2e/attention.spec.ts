import { readFile, writeFile } from "node:fs/promises";
import { join } from "node:path";
import { expect, expectConnected, test } from "./fixtures";

// A stand-in agent: it reports a question over the Warp cli-agent protocol,
// then keeps redrawing a spinner exactly like omp does while it waits.
const agent = String.raw`
printf '\033]777;notify;warp://cli-agent;{"v":1,"agent":"e2e","event":"question_asked"}\007'
( while :; do printf '\r| waiting'; sleep 0.1; printf '\r/ waiting'; sleep 0.1; done ) &
stty -echo -icanon min 1
dd bs=1 count=1 >/dev/null 2>&1
printf '\007'
sleep 1.5
printf '\007'
wait
`;

test("agent attention survives spinner output and clears on user input", async ({
  page,
  request,
  workDir,
}) => {
  const created = await request.post("/api/sessions", {
    data: {
      cwd: workDir,
      cmd: ["/bin/bash", "-c", agent],
      name: "E2E agent",
    },
  });
  expect(created.status()).toBe(201);
  const { id } = (await created.json()).session;
  const row = page.locator(`[data-session-id="${id}"]`);

  await page.goto("/");
  await expect(row).toContainText("需要回答");
  await expect(row.locator("[data-tone]")).toHaveAttribute(
    "data-tone",
    "destructive",
  );
  // Well past the 2-second output window that used to report "running".
  await page.waitForTimeout(2500);
  await expect(row).toContainText("需要回答");
  await expect(row).toHaveAttribute("data-status", "waiting");
  // A waiting session is not "executing": no title marker or animated icon.
  await expect(page).toHaveTitle("WebMux · 工作区");
  await expect(page.locator('link[rel="icon"]')).toHaveAttribute(
    "href",
    "/icon.svg",
  );
  const info = await (await request.get(`/api/sessions/${id}`)).json();
  expect(info.attention).toMatchObject({ kind: "question", source: "agent" });

  await page.goto(`/s/${id}`);
  await expect(page.locator(".restore-overlay")).toBeHidden();
  await expect(page).toHaveTitle("E2E agent · WebMux");
  await expect(page.locator(".terminal-header .status-dot")).toHaveClass(
    /status-failed/,
  );
  await page.locator(".xterm-helper-textarea").focus();
  await page.keyboard.type("y");
  // The bell rung right after the keystroke is ignored ...
  await expect
    .poll(
      async () =>
        (await (await request.get(`/api/sessions/${id}`)).json()).attention,
      { intervals: [50] },
    )
    .toBeNull();
  // ... while the bell after 1.5 s of quiet raises attention again.
  await page.goto("/");
  await expect(row).toContainText("响铃提醒");
  await expect(row.locator("[data-tone]")).toHaveAttribute(
    "data-tone",
    "warning",
  );
  expect(
    (await (await request.get(`/api/sessions/${id}`)).json()).attention,
  ).toMatchObject({ kind: "bell", source: "terminal" });
});

// omp queries terminal modes and sizes at startup. Opening the page replays
// those queries into xterm.js, whose automatic replies reach the PTY; they
// must not count as the user answering.
const queryingAgent = String.raw`
printf '\033[?2026$p\033[18t\033[14t\033[16t\033[c\033[6n'
printf '\033]777;notify;warp://cli-agent;{"v":1,"agent":"e2e","event":"question_asked"}\007'
stty -echo -icanon min 1
( while :; do printf '\r| waiting'; sleep 0.1; printf '\r/ waiting'; sleep 0.1; done ) &
cat > replies.bin
`;

test("opening a session does not clear attention through terminal replies", async ({
  page,
  request,
  workDir,
}) => {
  const created = await request.post("/api/sessions", {
    data: {
      cwd: workDir,
      cmd: ["/bin/bash", "-c", queryingAgent],
      name: "E2E querying agent",
    },
  });
  expect(created.status()).toBe(201);
  const { id } = (await created.json()).session;
  const attention = async () =>
    (await (await request.get(`/api/sessions/${id}`)).json()).attention;
  await expect.poll(attention).toMatchObject({ kind: "question" });

  for (const visit of [1, 2]) {
    await page.goto(`/s/${id}`);
    await expectConnected(page);
    await expect(page.locator(".restore-overlay")).toBeHidden();
    // Give the replayed queries' replies time to reach the PTY.
    await expect
      .poll(() => readFile(join(workDir, "replies.bin"), "latin1"), {
        message: `visit ${visit} replies`,
      })
      .toMatch(/\$y/);
    await page.waitForTimeout(800);
    expect(await attention(), `visit ${visit}`).toMatchObject({
      kind: "question",
      source: "agent",
    });
  }
});

// A stand-in for an OSC 7501 agent such as Claude Code: it asks whether the
// terminal speaks the protocol before any browser is attached, then reports
// its status while it keeps redrawing a spinner. The test advances it by
// creating step files in its working directory.
const programStatusReply =
  "\x1b]7501;?:states=idle,working,done,blocked,error:kinds=permission,question,auth\x1b\\";
const statusAgent = String.raw`
stty raw -echo
printf '\033]7501;?\007'
reply=$(dd bs=1 count=${programStatusReply.length} 2>/dev/null)
case "$reply" in *'7501;?:states='*) printf 'PROBE-OK\r\n' ;; *) printf 'PROBE-MISSING\r\n' ;; esac
printf '\033]7501;state=blocked:kind=permission:app=e2e\007'
( while :; do printf '\r| busy'; sleep 0.1; printf '\r/ busy'; sleep 0.1; done ) &
until [ -e step1 ]; do sleep 0.05; done
printf '\033]7501;state=working:app=e2e\007'
until [ -e step2 ]; do sleep 0.05; done
# Assembled at run time so the command line itself does not carry it.
msg="c2Vj$(printf cmV0)"
printf '\033]7501;state=done:app=e2e:msg=%s\007' "$msg"

cat > /dev/null
`;

test("program status reports drive attention and status", async ({
  page,
  request,
  workDir,
}) => {
  const created = await request.post("/api/sessions", {
    data: {
      cwd: workDir,
      cmd: ["/bin/bash", "-c", statusAgent],
      name: "E2E status agent",
    },
  });
  expect(created.status()).toBe(201);
  const { id } = (await created.json()).session;
  const info = async () =>
    (await request.get(`/api/sessions/${id}`)).json() as Promise<{
      status: string;
      attention: { kind: string; source: string } | null;
    }>;
  const row = page.locator(`[data-session-id="${id}"]`);

  await expect.poll(info).toMatchObject({
    status: "waiting",
    attention: { kind: "permission", source: "agent" },
  });
  await page.goto("/");
  await expect(row).toContainText("等待授权");
  await expect(row).toHaveAttribute("data-status", "waiting");
  await expect(row.locator("[data-tone]")).toHaveAttribute(
    "data-tone",
    "destructive",
  );

  await writeFile(join(workDir, "step1"), "");
  await expect.poll(info).toMatchObject({ status: "running", attention: null });
  // A working record keeps the session running however the output looks.
  await page.waitForTimeout(2500);
  expect(await info()).toMatchObject({ status: "running", attention: null });

  await writeFile(join(workDir, "step2"), "");
  await expect(row).toContainText("本轮完成");
  await expect(row).toHaveAttribute("data-status", "waiting");
  await expect(row.locator("[data-tone]")).toHaveAttribute(
    "data-tone",
    "warning",
  );
  const reported = await (await request.get(`/api/sessions/${id}`)).text();
  expect(reported).not.toContain("c2VjcmV0");

  await page.goto(`/s/${id}`);
  await expect(page.locator(".restore-overlay")).toBeHidden();
  await expect(page.locator(".xterm-rows")).toContainText("PROBE-OK");
  await page.locator(".xterm-helper-textarea").focus();
  await page.keyboard.type("y");
  // Seen, the finished turn is idle even while the spinner keeps drawing.
  await expect.poll(info).toMatchObject({ status: "idle", attention: null });
});

// The last line looks like a prompt and the program is blocked reading the
// terminal: the heuristics call that waiting, unless the program already said
// it reports its own waiting through structured events.
const promptLike = (structured: boolean) => String.raw`
${structured ? `printf '\\033]777;notify;warp://cli-agent;{"v":1,"event":"prompt_submit"}\\007'` : ""}
printf 'Continue? '
stty -echo -icanon min 1
dd bs=1 count=1 > /dev/null 2>&1
`;

test("structured agents do not trigger the waiting heuristics", async ({
  request,
  workDir,
}) => {
  const ids: Record<string, string> = {};
  for (const structured of [true, false]) {
    const created = await request.post("/api/sessions", {
      data: {
        cwd: workDir,
        cmd: ["/bin/bash", "-c", promptLike(structured)],
        name: structured ? "E2E structured" : "E2E plain",
      },
    });
    expect(created.status()).toBe(201);
    ids[String(structured)] = (await created.json()).session.id;
  }
  const status = async (structured: boolean) =>
    (
      await (
        await request.get(`/api/sessions/${ids[String(structured)]}`)
      ).json()
    ).status;
  // Past the 2-second output window: only the heuristics decide now.
  await expect.poll(() => status(false)).toBe("waiting");
  expect(await status(true)).toBe("running");
});

const delayedQuestionAgent = String.raw`
until [ -e ask-now ]; do sleep 0.05; done
printf '\033]777;notify;warp://cli-agent;{"v":1,"agent":"e2e","event":"question_asked"}\007'
cat > /dev/null
`;

test("raises a top-right toast by default while browsing and navigates to the session on click", async ({
  page,
  request,
  workDir,
}) => {
  // The first session-list snapshot is the toast baseline; raise the
  // question only once the page has seen the session without attention.
  const frames: string[] = [];
  page.on("websocket", (socket) => {
    if (new URL(socket.url()).pathname !== "/ws/sessions") return;
    socket.on("framereceived", ({ payload }) => frames.push(String(payload)));
  });
  await page.goto("/new");
  const created = await request.post("/api/sessions", {
    data: {
      cwd: workDir,
      cmd: ["/bin/bash", "-c", delayedQuestionAgent],
      name: "E2E toast agent",
    },
  });
  expect(created.status()).toBe(201);
  const { id } = (await created.json()).session;
  await expect
    .poll(() => frames.some((frame) => frame.includes(`"${id}"`)))
    .toBe(true);

  await writeFile(join(workDir, "ask-now"), "");
  const region = page.getByRole("region", { name: "会话通知" });
  await expect(region).toBeVisible();
  const toast = region.locator(`[data-toast-session-id="${id}"]`);
  await expect(toast).toHaveAttribute("data-tone", "destructive");
  await expect(toast).toContainText("需要回答");
  await expect(toast).toContainText("E2E toast agent");
  await expect(toast).toContainText("Agent 正在等待你的输入");

  await toast
    .getByRole("button", {
      name: "打开会话 E2E toast agent：需要回答 · Agent 正在等待你的输入",
    })
    .click();
  await expect(page).toHaveURL(new RegExp(`/s/${id}$`));
  await expect(region).toBeHidden();
});

test("shows a script's notify message in the toast and clears it on request", async ({
  page,
  request,
  workDir,
}) => {
  const created = await request.post("/api/sessions", {
    data: {
      cwd: workDir,
      cmd: ["/bin/sh", "-c", "sleep 60"],
      name: "E2E notify",
    },
  });
  expect(created.status()).toBe(201);
  const { id } = (await created.json()).session;
  // The first snapshot is the toast baseline: raise only after the page
  // has seen the session.
  const frames: string[] = [];
  page.on("websocket", (socket) => {
    if (new URL(socket.url()).pathname !== "/ws/sessions") return;
    socket.on("framereceived", ({ payload }) => frames.push(String(payload)));
  });
  await page.goto("/new");
  await expect
    .poll(() => frames.some((frame) => frame.includes(`"${id}"`)))
    .toBe(true);

  const raised = await request.post(`/api/sessions/${id}/attention`, {
    data: { kind: "done", message: "release 构建完成" },
  });
  expect(raised.status()).toBe(200);
  expect((await raised.json()).attention).toMatchObject({
    kind: "done",
    source: "cli",
    message: "release 构建完成",
  });
  const toast = page
    .getByRole("region", { name: "会话通知" })
    .locator(`[data-toast-session-id="${id}"]`);
  await expect(toast).toContainText("本轮完成");
  await expect(toast).toContainText("E2E notify");
  await expect(toast).toContainText("release 构建完成");

  const cleared = await request.delete(`/api/sessions/${id}/attention`);
  expect(cleared.status()).toBe(204);
  await expect(toast).toBeHidden();
});

test("deferring an inbox alert lets it return without stopping the session", async ({
  page,
  request,
  workDir,
}, info) => {
  const created = await request.post("/api/sessions", {
    data: {
      cwd: workDir,
      name: "Review alert",
      cmd: ["/bin/sh", "-c", "sleep 600"],
    },
  });
  expect(created.status()).toBe(201);
  const { id } = (await created.json()).session;
  const raised = await request.post(`/api/sessions/${id}/attention`, {
    data: { kind: "error", message: "Review the failed check" },
  });
  expect(raised.status()).toBe(200);
  await page.goto(`/s/${id}`);
  if (info.project.use.isMobile)
    await page.getByRole("button", { name: "打开侧栏", exact: true }).click();
  await page.getByRole("button", { name: /^待我处理/ }).click();
  const inbox = page.getByRole("dialog", { name: "待我处理", exact: true });
  await page.clock.install();
  const defer = inbox.getByRole("button", {
    name: "稍后（15 分钟后提醒）：Review alert",
  });
  await defer.click();
  await expect(inbox.getByText("已暂缓", { exact: true })).toBeVisible();
  const renamed = await request.patch(`/api/sessions/${id}`, {
    data: { name: "Renamed review alert" },
  });
  expect(renamed.ok()).toBe(true);
  await expect(
    inbox.getByRole("button", { name: "Renamed review alert", exact: true }),
  ).toBeVisible();
  await page.clock.fastForward(15 * 60_000 + 50);
  await expect(
    inbox.getByRole("button", {
      name: "稍后（15 分钟后提醒）：Renamed review alert",
    }),
  ).toBeVisible();
  await expect(page.getByRole("region", { name: "会话通知" })).toContainText(
    "Renamed review alert",
  );
  expect(
    (await (await request.get(`/api/sessions/${id}`)).json()).status,
  ).not.toBe("exited");
});

test("inbox has one conditional entry and keeps every landscape action reachable", async ({
  page,
  request,
  workDir,
}) => {
  await page.setViewportSize({ width: 667, height: 375 });
  const quiet = await request.post("/api/sessions", {
    data: {
      cwd: workDir,
      name: "Quiet session",
      cmd: ["/bin/sh", "-c", "sleep 600"],
    },
  });
  expect(quiet.status()).toBe(201);
  const quietId = (await quiet.json()).session.id;
  await page.goto(`/s/${quietId}`);
  await expectConnected(page);
  await page.getByRole("button", { name: "展开侧栏", exact: true }).click();
  const trigger = page.getByRole("button", { name: /^待我处理/ });
  await expect(trigger).toHaveCount(0);
  const ids: string[] = [];
  for (let index = 0; index < 8; index++) {
    const created = await request.post("/api/sessions", {
      data: {
        cwd: workDir,
        name: `Landscape alert ${index}`,
        cmd: ["/bin/sh", "-c", "sleep 600"],
      },
    });
    expect(created.status()).toBe(201);
    const { id } = (await created.json()).session;
    ids.push(id);
    const raised = await request.post(`/api/sessions/${id}/attention`, {
      data: { kind: "error" },
    });
    expect(raised.status()).toBe(200);
  }
  await expect(trigger).toHaveCount(1);
  await trigger.click();
  const inbox = page.getByRole("dialog", { name: "待我处理", exact: true });
  const lastAction = inbox.getByRole("button", {
    name: "稍后（15 分钟后提醒）：Landscape alert 0",
    exact: true,
  });
  await lastAction.scrollIntoViewIfNeeded();
  await expect(lastAction).toBeInViewport({ ratio: 1 });
  const bounds = await inbox.boundingBox();
  expect(bounds).not.toBeNull();
  expect(bounds!.y).toBeGreaterThanOrEqual(0);
  expect(bounds!.y + bounds!.height).toBeLessThanOrEqual(375);
  await lastAction.click();
  await expect(inbox.getByText("已暂缓", { exact: true })).toBeVisible();
  await page.reload();
  await page.getByRole("button", { name: "展开侧栏", exact: true }).click();
  await trigger.click();
  await expect(inbox.getByText("已暂缓", { exact: true })).toHaveCount(0);
  for (const id of ids) {
    const cleared = await request.delete(`/api/sessions/${id}/attention`);
    expect(cleared.status()).toBe(204);
  }
  // Keep the open empty state usable, then remove the entry on dismissal.
  await expect(inbox.getByText("当前没有等待处理的会话。")).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(trigger).toHaveCount(0);
});
