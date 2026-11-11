import { spawnSync } from "node:child_process";
import { mkdir } from "node:fs/promises";
import { join } from "node:path";
import type { APIRequestContext, Page } from "@playwright/test";
import { test, expect, useDirectory, chooseLocation } from "./fixtures";

async function config(
  request: APIRequestContext,
  name: string,
  env: Record<string, string> = {},
) {
  const response = await request.post("/api/launch-configs", {
    data: {
      name,
      cmd: ["omp"],
      chat: true,
      env: { FAKEAGENT_VERSION: "18.13.1", ...env },
    },
  });
  expect(response.status(), await response.text()).toBe(201);
  return (await response.json()).id as string;
}
async function source(
  request: APIRequestContext,
  cwd: string,
  text = "original source constraint",
) {
  const configId = await config(request, "Agent A");
  const response = await request.post("/api/sessions", {
    data: { cwd, configId, view: "chat", name: "Handoff source" },
  });
  expect(response.status(), await response.text()).toBe(201);
  const { session } = await response.json();
  expect(session.conversationId).toMatch(/^[a-f0-9]{32}$/);
  expect(
    (
      await request.post(`/api/sessions/${session.id}/input`, {
        data: {
          clientRequestId: "source-original-turn",
          text,
          mode: "refuse",
        },
      })
    ).ok(),
  ).toBeTruthy();
  await expect
    .poll(async () => {
      const page = await (
        await request.get(
          `/api/conversations/${session.conversationId}/activities`,
        )
      ).json();
      return page.items.some(
        (item: { role: string; kind: string }) =>
          item.role === "agent" && item.kind === "message",
      );
    })
    .toBe(true);
  return {
    id: session.id as string,
    conversationId: session.conversationId as string,
    providerThreadId: session.providerThreadId as string,
    configId,
  };
}
async function open(page: Page) {
  await page.getByRole("button", { name: /^(会话菜单|终端菜单)$/ }).click();
  await page
    .getByRole("menuitem", { name: "更换 Agent…", exact: true })
    .click();
  const dialog = page.getByRole("dialog", { name: "更换 Agent", exact: true });
  await expect(
    dialog.getByRole("combobox", { name: "目标 Agent", exact: true }),
  ).toBeVisible();
  return dialog;
}
async function target(page: Page, name: string) {
  await page.getByRole("combobox", { name: "目标 Agent", exact: true }).click();
  await page.getByRole("option", { name, exact: true }).click();
}
async function latest(request: APIRequestContext, id: string, fork = false) {
  const response = await request.get(
    `/api/conversations/${id}/handoff-previews?fork=${fork ? 1 : 0}`,
  );
  expect(response.ok(), await response.text()).toBeTruthy();
  return response.json();
}
const history = (page: Page) =>
  page.getByRole("textbox", { name: "历史正文", exact: true });
const task = (page: Page) =>
  page.getByRole("textbox", { name: "交接后的用户任务", exact: true });

test("server draft survives close, refresh and restart without reading ChatModel", async ({
  page,
  request,
  server,
  workDir,
}) => {
  const a = await source(request, workDir);
  await config(request, "Agent B");
  await page.goto(`/s/${a.id}`);
  await open(page);
  await target(page, "Agent B");
  await task(page).fill("separate user task 中文🙂");
  await page.getByRole("button", { name: "生成预览", exact: true }).click();
  await expect(history(page)).toHaveValue(/original source constraint/);
  await history(page).fill("Preserved manual history 中文🙂");
  await expect
    .poll(
      async () =>
        (await latest(request, a.conversationId)).preview?.renderedText,
    )
    .toBe("Preserved manual history 中文🙂");
  await expect(
    page.getByText("已保存到服务端", { exact: false }),
  ).toBeVisible();
  await page
    .getByRole("dialog", { name: "更换 Agent", exact: true })
    .getByRole("button", { name: "保存并关闭", exact: true })
    .click();
  await expect(
    page.getByRole("dialog", { name: "更换 Agent", exact: true }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "会话菜单", exact: true }),
  ).toBeFocused();
  await open(page);
  await expect(history(page)).toHaveValue("Preserved manual history 中文🙂");
  await expect(task(page)).toHaveValue("separate user task 中文🙂");
  await page.reload();
  await open(page);
  await expect(history(page)).toHaveValue("Preserved manual history 中文🙂");
  await server.restart();
  await page.reload();
  await open(page);
  await expect(history(page)).toHaveValue("Preserved manual history 中文🙂");
  await expect(task(page)).toHaveValue("separate user task 中文🙂");
});

test("A to B to A reuses the Conversation and original provider runtime", async ({
  page,
  request,
  workDir,
}) => {
  const a = await source(request, workDir);
  await config(request, "Agent B");
  await page.goto(`/s/${a.id}`);
  await open(page);
  await target(page, "Agent B");
  await task(page).fill("task for B");
  await page.getByRole("button", { name: "生成预览", exact: true }).click();
  await expect(page.getByText("可发送", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "发送交接", exact: true }).click();
  await expect(page.getByText("已投递", { exact: true })).toBeVisible();
  const first = await latest(request, a.conversationId);
  expect(first.delivery.status).toBe("inline");
  expect(first.delivery.targetConversationId).toBe(a.conversationId);
  const b = first.delivery.sessionId;
  expect(b).not.toBe(a.id);
  await page.getByRole("button", { name: "打开目标聊天", exact: true }).click();
  await expect(page).toHaveURL(`/s/${b}`);
  await expect
    .poll(() => page.evaluate(() => innerWidth))
    .toBe(page.viewportSize()!.width);
  await open(page);
  await target(page, "Agent A");
  await task(page).fill("new task after B");
  await page
    .getByRole("button", { name: "重新预览", exact: true })
    .first()
    .click();
  await expect(page.getByText("增量历史", { exact: true })).toBeVisible();
  const back = await latest(request, a.conversationId);
  expect(back.preview.targetProviderThreadId).toBe(a.providerThreadId);
  await page.getByRole("button", { name: "发送交接", exact: true }).click();
  await expect(page.getByText("已投递", { exact: true })).toBeVisible();
  const returned = await latest(request, a.conversationId);
  expect(returned.delivery.sessionId).toBe(a.id);
  await page.getByRole("button", { name: "打开目标聊天", exact: true }).click();
  await expect(page).toHaveURL(`/s/${a.id}`);
  const info = await (await request.get(`/api/sessions/${a.id}`)).json();
  expect(info.cwd).toBe(workDir);
  expect(info.conversationId).toBe(a.conversationId);
});

async function forkPage(page: Page, id: string, conversation: string) {
  await page.goto(`/s/${id}`);
  await page.getByRole("button", { name: /^(会话菜单|终端菜单)$/ }).click();
  await page
    .getByRole("menuitem", { name: "交接到新聊天…", exact: true })
    .click();
  await expect(page).toHaveURL(`/handoff/${conversation}`);
  await expect(
    page.getByRole("combobox", { name: "目标 Agent", exact: true }),
  ).toBeVisible();
}
async function registerRepository(request: APIRequestContext, root: string) {
  await mkdir(root);
  for (const args of [
    ["init", "-q", "-b", "main"],
    [
      "-c",
      "user.name=QA",
      "-c",
      "user.email=qa@example.invalid",
      "commit",
      "--allow-empty",
      "-m",
      "initial",
    ],
  ])
    expect(spawnSync("git", ["-C", root, ...args]).status).toBe(0);
  const response = await request.post("/api/repositories", {
    data: { path: root },
  });
  expect(response.status(), await response.text()).toBe(201);
  return response.json();
}
for (const mode of ["directory", "existing", "new"] as const) {
  test(`fork keeps history separate and sends in ${mode} location`, async ({
    page,
    request,
    workDir,
  }) => {
    const root = join(workDir, "handoff-app");
    const repository = await registerRepository(request, root);
    let expected = join(workDir, "target-directory");
    if (mode === "directory") await mkdir(expected);
    if (mode === "existing") {
      expected = join(workDir, "existing-tree");
      const created = await request.post(
        `/api/repositories/${repository.id}/worktrees`,
        {
          data: {
            mode: "new",
            branch: "existing-handoff",
            base: "main",
            path: expected,
          },
        },
      );
      expect(created.status(), await created.text()).toBe(201);
    }
    const a = await source(request, root);
    await config(request, "Agent B");
    await forkPage(page, a.id, a.conversationId);
    await target(page, "Agent B");
    await task(page).fill(`fork task ${mode}`);
    if (mode === "directory") await useDirectory(page, expected);
    if (mode === "existing") {
      await page.getByRole("button", { name: "位置", exact: true }).click();
      await page
        .getByRole("option")
        .filter({ hasText: "existing-handoff" })
        .click();
    }
    if (mode === "new") {
      await chooseLocation(page, "新建工作树…");
      await page
        .getByRole("textbox", { name: "新分支名称", exact: true })
        .fill("handoff-created");
    }
    await expect(
      page.getByRole("button", { name: "生成预览", exact: true }),
    ).toBeEnabled();
    await page.getByRole("button", { name: "生成预览", exact: true }).click();
    await expect(page.getByText("可发送", { exact: true })).toBeVisible();
    const draft = await latest(request, a.conversationId, true);
    expect(draft.preview.userText).toBe(`fork task ${mode}`);
    expect(draft.preview.sourceLocation.cwd).toBe(root);
    if (mode === "new")
      expect(
        spawnSync("git", ["-C", root, "worktree", "list", "--porcelain"], {
          encoding: "utf8",
        }).stdout,
      ).not.toContain("refs/heads/handoff-created");
    await page.getByRole("button", { name: "发送交接", exact: true }).click();
    await expect(page.getByText("已投递", { exact: true })).toBeVisible();
    const delivered = await latest(request, a.conversationId, true);
    expect(delivered.delivery.targetConversationId).not.toBe(a.conversationId);
    const info = await (
      await request.get(`/api/sessions/${delivered.delivery.sessionId}`)
    ).json();
    if (mode === "new") {
      expected = delivered.delivery.worktreePath;
      expect(expected).not.toBe("");
      expect(
        spawnSync("git", ["-C", expected, "branch", "--show-current"], {
          encoding: "utf8",
        }).stdout.trim(),
      ).toBe("handoff-created");
    }
    expect(info.cwd).toBe(expected);
    const activities = await (
      await request.get(
        `/api/conversations/${delivered.delivery.targetConversationId}/activities`,
      )
    ).json();
    expect(
      activities.items
        .filter((item: { role: string }) => item.role === "user")
        .map((item: { text: string }) => item.text),
    ).toEqual([`fork task ${mode}`]);
  });
}

test("conflicting device edit is retained across reload and can create a separate revision", async ({
  page,
  request,
  workDir,
}) => {
  const a = await source(request, workDir);
  await config(request, "Agent B");
  await page.goto(`/s/${a.id}`);
  await open(page);
  await target(page, "Agent B");
  await task(page).fill("independent task");
  await page.getByRole("button", { name: "生成预览", exact: true }).click();
  await expect(history(page)).toHaveValue(/original source constraint/);
  const before = (await latest(request, a.conversationId)).preview;
  let release!: () => void;
  const hold = new Promise<void>((resolve) => (release = resolve));
  let intercepted!: () => void;
  const started = new Promise<void>((resolve) => (intercepted = resolve));
  await page.route(`**/api/handoffs/${before.id}`, async (route) => {
    if (route.request().method() === "PATCH") {
      intercepted();
      await hold;
    }
    await route.continue();
  });
  await history(page).fill("local edited constraint");
  await started;
  const external = await request.patch(`/api/handoffs/${before.id}`, {
    data: { revision: before.revision, text: "other device constraint" },
  });
  expect(external.status()).toBe(200);
  release();
  await expect(
    page.getByRole("alert").filter({ hasText: "另一个设备" }),
  ).toBeVisible();
  await page.reload();
  await open(page);
  await expect(history(page)).toHaveValue("local edited constraint");
  await expect(
    page.getByRole("alert").filter({ hasText: "服务端已有其他版本" }),
  ).toBeVisible();
  await page
    .getByRole("button", { name: "保留编辑重新预览", exact: true })
    .click();
  await expect
    .poll(
      async () =>
        (await latest(request, a.conversationId)).preview?.renderedText,
    )
    .toBe("local edited constraint");
  const saved = (await latest(request, a.conversationId)).preview;
  expect(saved.id).not.toBe(before.id);
  expect(saved.manual).toBe(true);
  expect(saved.userText).toBe("independent task");
  expect(
    (await (await request.get(`/api/handoffs/${before.id}`)).json())
      .renderedText,
  ).toBe("other device constraint");
});

test("over-budget task and manual context survive refresh without truncation", async ({
  page,
  request,
  workDir,
}) => {
  const a = await source(request, workDir);
  await config(request, "Agent B");
  await page.goto(`/s/${a.id}`);
  await open(page);
  await target(page, "Agent B");
  await task(page).fill("short task");
  await page.getByRole("button", { name: "生成预览", exact: true }).click();
  await history(page).fill("protected manual constraint");
  await expect
    .poll(
      async () =>
        (await latest(request, a.conversationId)).preview?.renderedText,
    )
    .toBe("protected manual constraint");
  const huge = "x".repeat(65537);
  await task(page).fill(huge);
  await expect
    .poll(
      async () =>
        (await latest(request, a.conversationId)).preview?.budget.overBudget,
    )
    .toBe(true);
  await expect(page.getByText("超出预算", { exact: true })).toBeVisible();
  await page.reload();
  await open(page);
  await expect(task(page)).toHaveValue(huge);
  await expect(history(page)).toHaveValue("protected manual constraint");
  await expect(
    page.getByRole("button", { name: "发送交接", exact: true }),
  ).toHaveCount(0);
});

test("omitted history is read from bounded source pages", async ({
  page,
  request,
  workDir,
}) => {
  const a = await source(
    request,
    workDir,
    "omitted old constraint " + "old history ".repeat(3000),
  );
  await config(request, "Agent B");
  await page.goto(`/s/${a.id}`);
  await open(page);
  await target(page, "Agent B");
  await task(page).fill("short new task");
  await page.getByRole("button", { name: "生成预览", exact: true }).click();
  await page.getByRole("button", { name: "查看省略历史", exact: true }).click();
  await expect(
    page.getByRole("textbox", { name: "省略历史片段", exact: true }),
  ).toHaveValue(/omitted old constraint/);
  await page.getByRole("button", { name: "下一片段", exact: true }).click();
  await expect(page.getByText(/字节 offset 4000/)).toBeVisible();
  await expect(task(page)).toHaveValue("short new task");
});

for (const status of ["unknown", "failed"] as const) {
  test(`${status} delivery preserves its task and offers only valid recovery`, async ({
    page,
    request,
    workDir,
  }) => {
    const a = await source(request, workDir);
    await config(
      request,
      "Agent B",
      status === "unknown"
        ? {
            FAKEAGENT_CRASH: "prompt-saved",
            FAKEAGENT_FAIL: "session/load=-32603",
          }
        : { FAKEAGENT_FAIL: "session/prompt=-32000" },
    );
    await forkPage(page, a.id, a.conversationId);
    await target(page, "Agent B");
    await task(page).fill("retained failure task");
    await page.getByRole("button", { name: "生成预览", exact: true }).click();
    await page.getByRole("button", { name: "发送交接", exact: true }).click();
    await expect(
      page.getByText(status === "unknown" ? "结果待对账" : "投递失败", {
        exact: true,
      }),
    ).toBeVisible();
    const first = (await latest(request, a.conversationId, true)).delivery;
    await page.reload();
    await expect(task(page)).toHaveValue("retained failure task");
    if (status === "unknown") {
      await expect(
        page.getByRole("button", { name: "明确重试交接", exact: true }),
      ).toHaveCount(0);
      await expect(
        page.getByRole("button", { name: "等待对账", exact: true }),
      ).toBeDisabled();
    } else {
      await page
        .getByRole("button", { name: "明确重试交接", exact: true })
        .click();
      await expect
        .poll(
          async () =>
            (await latest(request, a.conversationId, true)).delivery
              ?.executionId,
        )
        .not.toBe(first.executionId);
      const next = (await latest(request, a.conversationId, true)).delivery;
      expect(next.sessionId).toBe(first.sessionId);
      expect(next.retry).toBe(true);
    }
  });
}

test("new source work marks the draft stale without overwriting edits", async ({
  page,
  request,
  workDir,
}) => {
  const a = await source(request, workDir);
  await config(request, "Agent B");
  await page.goto(`/s/${a.id}`);
  await open(page);
  await target(page, "Agent B");
  await task(page).fill("retained new task");
  await page.getByRole("button", { name: "生成预览", exact: true }).click();
  await history(page).fill("retained edited constraint");
  await expect
    .poll(
      async () =>
        (await latest(request, a.conversationId)).preview?.renderedText,
    )
    .toBe("retained edited constraint");
  expect(
    (
      await request.post(`/api/sessions/${a.id}/input`, {
        data: {
          clientRequestId: "another-device-turn",
          text: "another device started work",
          mode: "refuse",
        },
      })
    ).ok(),
  ).toBe(true);
  await expect(page.getByText("来源已变化", { exact: true })).toBeVisible();
  await expect(history(page)).toHaveValue("retained edited constraint");
  await expect(task(page)).toHaveValue("retained new task");
  await page.reload();
  await open(page);
  await expect(history(page)).toHaveValue("retained edited constraint");
});

test("forgetting a source clears its recovered local draft on the next visit", async ({
  page,
  request,
  workDir,
}) => {
  const a = await source(request, workDir);
  await config(request, "Agent B");
  await forkPage(page, a.id, a.conversationId);
  await target(page, "Agent B");
  await task(page).fill("private unsent task");
  const keys = () =>
    page.evaluate(
      (id) =>
        Object.keys(localStorage).filter((key) =>
          key.includes(`handoff.${id}.fork`),
        ),
      a.conversationId,
    );
  await expect.poll(async () => (await keys()).length).toBeGreaterThan(0);
  expect(
    (await request.delete(`/api/sessions/${a.id}?forget=1`)).status(),
  ).toBe(204);
  await page.reload();
  await expect(task(page)).toHaveValue("");
  await expect.poll(keys).toEqual([]);
  await expect(
    page.getByRole("button", { name: "生成预览", exact: true }),
  ).toBeDisabled();
});
