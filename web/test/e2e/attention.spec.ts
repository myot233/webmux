import { expect, test } from "./fixtures";

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
      preset: "custom",
      cmd: ["/bin/bash", "-c", agent],
      name: "E2E agent",
    },
  });
  expect(created.status()).toBe(201);
  const { id } = await created.json();
  const row = page.locator(`[data-session-id="${id}"]`);

  await page.goto("/");
  await expect(row).toContainText("需要回答");
  // Well past the 2-second output window that used to report "running".
  await page.waitForTimeout(2500);
  await expect(row).toContainText("需要回答");
  await expect(row).toHaveAttribute("data-status", "waiting");
  const info = await (await request.get(`/api/sessions/${id}`)).json();
  expect(info.attention).toMatchObject({ kind: "question", source: "agent" });

  await page.goto(`/s/${id}`);
  await expect(page.locator(".restore-overlay")).toBeHidden();
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
  expect(
    (await (await request.get(`/api/sessions/${id}`)).json()).attention,
  ).toMatchObject({ kind: "bell", source: "terminal" });
});
