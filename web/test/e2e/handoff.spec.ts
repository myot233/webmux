import { spawnSync } from "node:child_process";
import { mkdir, readFile, readdir } from "node:fs/promises";
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

test("autosave persists later task and history edits made during an earlier PATCH", async ({
  page,
  request,
  workDir,
}) => {
  const a = await source(request, workDir);
  await config(request, "Autosave target");
  await page.goto(`/s/${a.id}`);
  await open(page);
  await target(page, "Autosave target");
  await task(page).fill("initial task");
  await page.getByRole("button", { name: "生成预览", exact: true }).click();
  await expect(history(page)).toHaveValue(/original source constraint/);
  const { preview } = await latest(request, a.conversationId);
  const path = `/api/handoffs/${preview.id}`;
  for (const field of ["userText", "renderedText"] as const) {
    const started = Promise.withResolvers<void>();
    const complete = Promise.withResolvers<void>();
    let hold = true;
    await page.route(`**${path}`, async (route) => {
      if (route.request().method() !== "PATCH" || !hold) {
        await route.continue();
        return;
      }
      hold = false;
      const response = await route.fetch();
      started.resolve();
      await complete.promise;
      await route.fulfill({ response });
    });
    try {
      const input = field === "userText" ? task(page) : history(page);
      await input.fill(`first ${field} 中文🙂`);
      await started.promise;
      await input.fill(`later ${field} 中文🙂`);
      complete.resolve();
      await expect
        .poll(
          async () => (await latest(request, a.conversationId)).preview[field],
        )
        .toBe(`later ${field} 中文🙂`);
      await expect(input).toHaveValue(`later ${field} 中文🙂`);
    } finally {
      complete.resolve();
      await page.unroute(`**${path}`);
    }
  }
  const { preview: saved, delivery } = await latest(request, a.conversationId);
  expect(saved.userText).toBe("later userText 中文🙂");
  expect(saved.renderedText).toBe("later renderedText 中文🙂");
  expect(delivery).toBeNull();
});

test("a conflicting local draft follows pending delivery to completion without discarding edits", async ({
  page,
  request,
  workDir,
}) => {
  const a = await source(request, workDir);
  await config(request, "Delayed conflict target", {
    FAKEAGENT_DELAY: "session/new=4s",
  });
  await forkPage(page, a.id, a.conversationId);
  await target(page, "Delayed conflict target");
  await task(page).fill("initial reviewed task");
  await page.getByRole("button", { name: "生成预览", exact: true }).click();
  await expect(history(page)).toHaveValue(/original source constraint/);
  const { preview } = await latest(request, a.conversationId, true);
  const path = `/api/handoffs/${preview.id}`;
  await page.route(`**${path}`, async (route) => {
    if (route.request().method() === "PATCH") await route.abort();
    else await route.continue();
  });
  const unsaved = page.waitForRequest(
    (r) => r.url().endsWith(path) && r.method() === "PATCH",
  );
  await history(page).fill("local unsent history 中文🙂");
  await task(page).fill("local unsent task 中文🙂");
  await unsaved;
  const updated = await request.patch(path, {
    data: {
      revision: preview.revision,
      text: "other device reviewed history",
      userText: "other device sent task",
    },
  });
  expect(updated.status()).toBe(200);
  const remote = await updated.json();
  const sent = await request.post(
    `/api/conversations/${a.conversationId}/messages`,
    {
      data: {
        clientRequestId: "conflict-other-device-send",
        handoffId: remote.id,
        revision: remote.revision,
        text: remote.userText,
        configId: remote.configId,
        agentConfig: remote.agentConfig,
        location: remote.location,
        attachments: remote.attachments,
        retry: false,
      },
    },
  );
  expect(sent.status()).toBe(202);
  await forkPage(page, a.id, a.conversationId);
  const keep = page.getByRole("button", {
    name: "保留编辑重新预览",
    exact: true,
  });
  await expect(keep).toBeVisible();
  await expect(keep).toBeDisabled();
  let localWrites = 0;
  page.on("request", (r) => {
    if (r.method() === "PATCH" || r.method() === "POST") localWrites++;
  });
  await expect
    .poll(
      async () => (await (await request.get(`${path}/delivery`)).json()).status,
    )
    .toBe("inline");
  await expect(keep).toBeEnabled();
  await expect(task(page)).toHaveValue("local unsent task 中文🙂");
  await expect(history(page)).toHaveValue("local unsent history 中文🙂");
  expect(localWrites).toBe(0);
  const saved = await latest(request, a.conversationId, true);
  expect(saved.preview.userText).toBe("other device sent task");
  expect(saved.preview.renderedText).toBe("other device reviewed history");
});

test("preview regeneration waits for uploaded attachments and retains them", async ({
  page,
  request,
  workDir,
}) => {
  const a = await source(request, workDir);
  await config(request, "Agent B");
  await page.goto(`/s/${a.id}`);
  const dialog = await open(page);
  await target(page, "Agent B");
  await task(page).fill("retain the uploaded handoff proof");
  await dialog.getByRole("button", { name: "生成预览", exact: true }).click();
  await expect(history(page)).toHaveValue(/original source constraint/);
  const uploading = Promise.withResolvers<void>();
  const complete = Promise.withResolvers<void>();
  await page.route("**/api/uploads", async (route) => {
    const response = await route.fetch();
    uploading.resolve();
    await complete.promise;
    await route.fulfill({ response });
  });
  try {
    const chooser = page.waitForEvent("filechooser");
    await dialog.getByRole("button", { name: "上传文件", exact: true }).click();
    await (
      await chooser
    ).setFiles({
      name: "handoff-proof.txt",
      mimeType: "text/plain",
      buffer: Buffer.from("keep this uploaded handoff attachment"),
    });
    await uploading.promise;
    const regenerate = dialog
      .getByRole("button", { name: "重新预览", exact: true })
      .last();
    await expect(regenerate).toBeDisabled();
    complete.resolve();
    await expect(
      dialog.getByText("handoff-proof.txt", { exact: true }),
    ).toBeVisible();
    await expect(regenerate).toBeEnabled();
    await regenerate.click();
    await expect
      .poll(async () =>
        (await latest(request, a.conversationId)).preview?.attachments.map(
          (attachment: { name: string }) => attachment.name,
        ),
      )
      .toEqual(["handoff-proof.txt"]);
  } finally {
    complete.resolve();
    await page.unroute("**/api/uploads");
  }
});

for (const image of [false, true]) {
  test(`empty-task ${image ? "image" : "file"} handoff waits for explicit send and delivers attachment bytes`, async ({
    page,
    request,
    server,
    workDir,
  }) => {
    const a = await source(request, workDir);
    await config(request, "Attachment target", {
      FAKEAGENT_PROMPT_IMAGE: "1",
      FAKEAGENT_EMBEDDED_CONTEXT: "1",
    });
    const providerUsers = async () => {
      const directory = join(server.env.FAKEAGENT_STORE!, "sessions");
      const records = await Promise.all(
        (await readdir(directory))
          .filter((name) => name.endsWith(".json"))
          .map(async (name) =>
            JSON.parse(await readFile(join(directory, name), "utf8")),
          ),
      );
      return records
        .filter((record) => record.cwd === workDir)
        .flatMap((record) =>
          record.messages.filter(
            (item: { role: string }) => item.role === "user",
          ),
        );
    };
    await page.goto(`/s/${a.id}`);
    const dialog = await open(page);
    await target(page, "Attachment target");
    await expect(task(page)).toHaveValue("");
    await dialog.getByRole("button", { name: "生成预览", exact: true }).click();
    await expect(
      dialog.getByRole("button", { name: "发送交接", exact: true }),
    ).toBeDisabled();
    const bytes = image
      ? Buffer.from(
          "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jRZkAAAAASUVORK5CYII=",
          "base64",
        )
      : Buffer.from("actual attachment-only handoff bytes 中文🙂");
    const name = `attachment-only-${a.id}.${image ? "png" : "txt"}`;
    const chooser = page.waitForEvent("filechooser");
    await dialog.getByRole("button", { name: "上传文件", exact: true }).click();
    await (
      await chooser
    ).setFiles({
      name,
      mimeType: image ? "image/png" : "text/plain",
      buffer: bytes,
    });
    await expect(dialog.getByText(name, { exact: true })).toBeVisible();
    await dialog
      .getByRole("button", { name: "重新预览", exact: true })
      .last()
      .click();
    const send = dialog.getByRole("button", { name: "发送交接", exact: true });
    await expect(send).toBeEnabled();
    await expect(task(page)).toHaveValue("");
    expect((await latest(request, a.conversationId)).preview.userText).toBe("");
    expect(await providerUsers()).toHaveLength(1);
    expect((await latest(request, a.conversationId)).delivery).toBeNull();
    await send.click();
    await expect(page.getByText("已投递", { exact: true })).toBeVisible();
    await expect.poll(async () => (await providerUsers()).length).toBe(2);
    const sent = await latest(request, a.conversationId);
    const activities = await (
      await request.get(
        `/api/conversations/${sent.delivery.targetConversationId}/activities`,
      )
    ).json();
    const delivered = activities.items
      .filter(
        (item: { role: string; kind: string }) =>
          item.role === "user" && item.kind === "message",
      )
      .at(-1);
    expect(delivered.text).toBe("");
    expect(delivered.attachments).toHaveLength(1);
    expect(await readFile(delivered.attachments[0].reference)).toEqual(bytes);
    const users = await providerUsers();
    const received = users
      .flatMap((user) => user.blocks ?? [])
      .find(
        (block: { type: string }) =>
          block.type === (image ? "image" : "resource"),
      );
    expect(received).toBeTruthy();
    expect(
      image
        ? Buffer.from(received.data, "base64")
        : Buffer.from(received.resource.text),
    ).toEqual(bytes);
  });
}

test("a completed POST clears the next task without losing delivered task or attachment bytes", async ({
  page,
  request,
  server,
  workDir,
}) => {
  const a = await source(request, workDir);
  await config(request, "Completed target", {
    FAKEAGENT_EMBEDDED_CONTEXT: "1",
  });
  await page.goto(`/s/${a.id}`);
  const dialog = await open(page);
  await target(page, "Completed target");
  const deliveredTask = "deliver this reviewed task exactly once 中文🙂";
  await task(page).fill(deliveredTask);
  const name = `completed-${a.id}.txt`;
  const bytes = Buffer.from("actual completed POST handoff attachment 中文🙂");
  const chooser = page.waitForEvent("filechooser");
  await dialog.getByRole("button", { name: "上传文件", exact: true }).click();
  await (
    await chooser
  ).setFiles({ name, mimeType: "text/plain", buffer: bytes });
  await expect(dialog.getByText(name, { exact: true })).toBeVisible();
  await dialog.getByRole("button", { name: "生成预览", exact: true }).click();
  await expect(history(page)).toHaveValue(/original source constraint/);
  const preview = (await latest(request, a.conversationId)).preview;
  let submissions = 0;
  // Polling must not hide a missing transition in the POST completion path.
  await page.route(`**/api/handoffs/${preview.id}/delivery`, (route) =>
    route.abort(),
  );
  await page.route(
    `**/api/conversations/${a.conversationId}/messages`,
    async (route) => {
      submissions++;
      const frozen = route.request().postDataJSON();
      expect(frozen.text).toBe(deliveredTask);
      expect(frozen.attachments).toEqual(preview.attachments);
      const accepted = await route.fetch();
      expect(accepted.status()).toBe(202);
      // Replay the identical idempotent request only after the real backend
      // completes it, so the browser receives a genuine completed POST body.
      await expect
        .poll(async () => {
          const response = await request.get(
            `/api/handoffs/${preview.id}/delivery`,
          );
          expect(response.ok(), await response.text()).toBeTruthy();
          return (await response.json()).status;
        })
        .toBe("inline");
      const completed = await route.fetch();
      expect(completed.status()).toBe(202);
      expect((await completed.json()).status).toBe("inline");
      await route.fulfill({ response: completed });
    },
  );
  try {
    await dialog.getByRole("button", { name: "发送交接", exact: true }).click();
    await expect(dialog.getByText("已投递", { exact: true })).toBeVisible();
    await expect(task(page)).toHaveValue("");
    await expect(task(page)).toBeEnabled();
    await expect(dialog.getByText(name, { exact: true })).toHaveCount(0);
    await expect(
      dialog.getByRole("combobox", { name: "目标 Agent", exact: true }),
    ).toHaveText("选择目标 Agent");
    const sent = await latest(request, a.conversationId);
    expect(sent.preview.id).toBe(preview.id);
    expect(sent.preview.revision).toBe(preview.revision);
    expect(sent.preview.renderedText).toBe(preview.renderedText);
    expect(sent.preview.userText).toBe(deliveredTask);
    expect(sent.preview.attachments).toEqual(preview.attachments);
    const activities = await (
      await request.get(
        `/api/conversations/${sent.delivery.targetConversationId}/activities`,
      )
    ).json();
    const delivered = activities.items.filter(
      (item: { role: string; kind: string; text: string }) =>
        item.role === "user" &&
        item.kind === "message" &&
        item.text === deliveredTask,
    );
    expect(delivered).toHaveLength(1);
    expect(delivered[0].attachments).toHaveLength(1);
    expect(await readFile(delivered[0].attachments[0].reference)).toEqual(
      bytes,
    );
    const records = await Promise.all(
      (await readdir(join(server.env.FAKEAGENT_STORE!, "sessions")))
        .filter((filename) => filename.endsWith(".json"))
        .map(async (filename) =>
          JSON.parse(
            await readFile(
              join(server.env.FAKEAGENT_STORE!, "sessions", filename),
              "utf8",
            ),
          ),
        ),
    );
    const users = records
      .filter((record) => record.cwd === workDir)
      .flatMap((record) =>
        record.messages.filter(
          (item: { role: string }) => item.role === "user",
        ),
      );
    expect(users).toHaveLength(2);
    const attachment = users
      .flatMap((user) => user.blocks ?? [])
      .find((block: { type: string }) => block.type === "resource");
    expect(attachment).toBeTruthy();
    expect(Buffer.from(attachment.resource.text)).toEqual(bytes);
    expect(submissions).toBe(1);
    await dialog
      .getByRole("button", { name: "保存并关闭", exact: true })
      .click();
    await open(page);
    await expect(task(page)).toHaveValue("");
    await expect(dialog.getByText(name, { exact: true })).toHaveCount(0);
    expect(submissions).toBe(1);
  } finally {
    await page.unroute(`**/api/handoffs/${preview.id}/delivery`);
    await page.unroute(`**/api/conversations/${a.conversationId}/messages`);
  }
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
      let release!: () => void;
      const hold = new Promise<void>((resolve) => (release = resolve));
      let entered!: () => void;
      const started = new Promise<void>((resolve) => (entered = resolve));
      await page.route(
        `**/api/repositories/${repository.id}/branches`,
        async (route) => {
          const response = await route.fetch();
          entered();
          await hold;
          await route.fulfill({ response });
        },
      );
      await chooseLocation(page, "新建工作树…");
      await started;
      await expect(
        page.getByRole("textbox", { name: "新分支名称", exact: true }),
      ).toBeDisabled();
      release();
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

    if (mode === "new") {
      // The repository moves while the saved preview stays frozen.
      expect(
        spawnSync("git", [
          "-C",
          root,
          "-c",
          "user.name=QA",
          "-c",
          "user.email=qa@example.invalid",
          "commit",
          "--allow-empty",
          "-m",
          "advanced",
        ]).status,
      ).toBe(0);
      await page.reload();
      await expect(
        page.getByRole("textbox", { name: "新分支名称", exact: true }),
      ).toHaveValue("handoff-created");
      await expect(
        page.getByRole("button", { name: "发送交接", exact: true }),
      ).toBeEnabled();
    }
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
      expect(
        spawnSync("git", ["-C", expected, "rev-parse", "HEAD"], {
          encoding: "utf8",
        }).stdout.trim(),
      ).toBe(draft.preview.location.newWorktreeBase);
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

for (const resolution of ["server", "keep"] as const) {
  test(`unpreviewed device work survives another device preview until ${resolution} choice`, async ({
    page,
    browser,
    request,
    server,
    workDir,
  }) => {
    const a = await source(request, workDir);
    const localFilename = `device-a-${a.id}-proof.txt`;
    const localConfig = await config(request, "Local target");
    const remoteConfig = await config(request, "Remote target");
    const localDirectory = join(workDir, "local-target");
    await mkdir(localDirectory);
    await forkPage(page, a.id, a.conversationId);
    await target(page, "Local target");
    await page.getByRole("textbox", { name: "模型（可选）" }).fill("smart");
    await useDirectory(page, localDirectory);
    await task(page).fill("device A unpreviewed task 中文🙂");
    const chooser = page.waitForEvent("filechooser");
    await page.getByRole("button", { name: "上传文件", exact: true }).click();
    await (
      await chooser
    ).setFiles({
      name: localFilename,
      mimeType: "text/plain",
      buffer: Buffer.from("actual uploaded device A attachment"),
    });
    await expect(page.getByText(localFilename, { exact: true })).toBeVisible();
    const backup = () =>
      page.evaluate((id) => {
        const key = `webmux.handoff.${id}.fork`;
        return {
          choice: JSON.parse(localStorage.getItem(`${key}.choice`) ?? "null"),
          recovery: JSON.parse(
            localStorage.getItem(`${key}.recovery`) ?? "null",
          ),
        };
      }, a.conversationId);
    await expect
      .poll(async () => (await backup()).choice?.attachments.length)
      .toBe(1);
    const saved = await backup();
    expect(saved.choice.configId).toBe(localConfig);
    expect(saved.choice.location.cwd).toBe(localDirectory);
    expect(saved.recovery).toBeNull();
    expect((await latest(request, a.conversationId, true)).preview).toBeNull();
    await page.goto("/");

    const device = await browser.newContext({ baseURL: server.baseURL });
    try {
      const remote = await device.newPage();
      await forkPage(remote, a.id, a.conversationId);
      await target(remote, "Remote target");
      await task(remote).fill("device B server task");
      await remote
        .getByRole("button", { name: "生成预览", exact: true })
        .click();
      await expect(history(remote)).toHaveValue(/original source constraint/);
      await history(remote).fill("device B manually reviewed history");
      await expect
        .poll(
          async () =>
            (await latest(request, a.conversationId, true)).preview
              ?.renderedText,
        )
        .toBe("device B manually reviewed history");
      const remotePreview = (await latest(request, a.conversationId, true))
        .preview;
      await remote.goto("/");

      const conflict = page.getByRole("button", {
        name: "保留编辑重新预览",
        exact: true,
      });
      for (let visit = 0; visit < 2; visit++) {
        await page.goto(`/handoff/${a.conversationId}`);
        await expect(conflict).toBeVisible();
        await expect(history(page)).toBeDisabled();
        await expect(task(page)).toHaveValue(saved.choice.task);
        await expect(
          page.getByRole("combobox", { name: "目标 Agent", exact: true }),
        ).toHaveText("Local target");
        await expect(
          page.getByRole("textbox", { name: "模型（可选）" }),
        ).toHaveValue("smart");
        await expect(
          page.getByText(localFilename, { exact: true }),
        ).toBeVisible();
        await expect.poll(backup).toEqual(saved);
        expect((await latest(request, a.conversationId, true)).preview).toEqual(
          remotePreview,
        );
        expect((await (await request.get("/api/sessions")).json()).length).toBe(
          1,
        );
        await page.goto("/");
      }
      await page.goto(`/handoff/${a.conversationId}`);
      await expect(conflict).toBeVisible();
      if (resolution === "server") {
        await page
          .getByRole("button", { name: "读取服务端版本", exact: true })
          .click();
        await expect(task(page)).toHaveValue("device B server task");
        await expect(history(page)).toHaveValue(
          "device B manually reviewed history",
        );
        await expect(
          page.getByRole("combobox", { name: "目标 Agent", exact: true }),
        ).toHaveText("Remote target");
        await expect(
          page.getByText(localFilename, { exact: true }),
        ).toHaveCount(0);
        await expect(conflict).toHaveCount(0);
        await page.reload();
        await expect(task(page)).toHaveValue("device B server task");
        await expect(conflict).toHaveCount(0);
        expect((await backup()).choice.configId).toBe(remoteConfig);
      } else {
        await conflict.click();
        await expect(conflict).toHaveCount(0);
        await expect(history(page)).toHaveValue(/original source constraint/);
        const regenerated = (await latest(request, a.conversationId, true))
          .preview;
        expect(regenerated.id).not.toBe(remotePreview.id);
        expect(regenerated.userText).toBe(saved.choice.task);
        expect(regenerated.configId).toBe(localConfig);
        expect(regenerated.agentConfig.model).toBe("smart");
        expect(regenerated.location.cwd).toBe(localDirectory);
        expect(regenerated.manual).toBe(false);
        expect(
          (
            await (
              await request.get(`/api/handoffs/${remotePreview.id}`)
            ).json()
          ).renderedText,
        ).toBe("device B manually reviewed history");
        expect(
          (await latest(request, a.conversationId, true)).delivery,
        ).toBeNull();
        expect((await (await request.get("/api/sessions")).json()).length).toBe(
          1,
        );
        await page
          .getByRole("button", { name: "发送交接", exact: true })
          .click();
        await expect(page.getByText("已投递", { exact: true })).toBeVisible();
        const sent = await latest(request, a.conversationId, true);
        const destination = await (
          await request.get(
            `/api/conversations/${sent.delivery.targetConversationId}/activities`,
          )
        ).json();
        const delivered = destination.items.find(
          (item: { role: string; kind: string }) =>
            item.role === "user" && item.kind === "message",
        ).attachments[0];
        expect(delivered.name).toBe(localFilename);
        expect(await readFile(delivered.reference, "utf8")).toBe(
          "actual uploaded device A attachment",
        );
      }
    } finally {
      await device.close();
    }
  });
}

test("untouched source-location defaults adopt another device preview", async ({
  page,
  request,
  workDir,
}) => {
  const a = await source(request, workDir);
  const remoteConfig = await config(request, "Remote target");
  await forkPage(page, a.id, a.conversationId);
  await expect(task(page)).toHaveValue("");
  await page.goto("/");
  const response = await request.post(
    `/api/conversations/${a.conversationId}/handoff-previews`,
    {
      data: {
        clientRequestId: "remote-first-preview",
        configId: remoteConfig,
        agentConfig: {},
        userText: "remote task after untouched local defaults",
        fork: true,
        location: { cwd: workDir },
        attachments: [],
      },
    },
  );
  expect(response.status(), await response.text()).toBe(201);
  await page.goto(`/handoff/${a.conversationId}`);
  await expect(task(page)).toHaveValue(
    "remote task after untouched local defaults",
  );
  await expect(
    page.getByRole("button", { name: "读取服务端版本", exact: true }),
  ).toHaveCount(0);
});

test("same-preview recovery keeps its base through repeated conflicts and resets after server adoption", async ({
  page,
  browser,
  request,
  server,
  workDir,
}) => {
  const a = await source(request, workDir);
  await config(request, "Agent B");
  await page.goto(`/s/${a.id}`);
  await open(page);
  await target(page, "Agent B");
  await task(page).fill("initial reviewed task");
  await page.getByRole("button", { name: "生成预览", exact: true }).click();
  await expect(history(page)).toHaveValue(/original source constraint/);
  const before = (await latest(request, a.conversationId)).preview;
  expect(before.revision).toBe(1);
  const previewPath = `/api/handoffs/${before.id}`;
  let localSaves = 0;
  await page.route(`**${previewPath}`, async (route) => {
    if (route.request().method() === "PATCH") {
      localSaves++;
      await route.abort();
    } else await route.continue();
  });
  const editOffline = async (text: string, userText: string) => {
    const failed = page.waitForEvent(
      "requestfailed",
      (request) =>
        request.url().endsWith(previewPath) && request.method() === "PATCH",
    );
    await history(page).fill(text);
    await task(page).fill(userText);
    await failed;
    await page.goto("/");
  };
  const conflict = page.getByRole("button", {
    name: "保留编辑重新预览",
    exact: true,
  });
  const device = await browser.newContext({ baseURL: server.baseURL });
  try {
    const remote = await device.newPage();
    await remote.goto(`/s/${a.id}`);
    await open(remote);
    await expect(history(remote)).toHaveValue(before.renderedText);
    await editOffline("device A unsaved context", "device A unsaved task");
    await history(remote).fill("device B reviewed context");
    await task(remote).fill("device B reviewed task");
    await expect
      .poll(
        async () => (await latest(request, a.conversationId)).preview.userText,
      )
      .toBe("device B reviewed task");
    const serverVersion = (await latest(request, a.conversationId)).preview;
    expect(serverVersion.id).toBe(before.id);
    expect(serverVersion.revision).toBe(2);
    expect(serverVersion.renderedText).toBe("device B reviewed context");
    await remote.goto("/");
    const savedAttempts = localSaves;
    for (const visit of ["reopen", "reload", "close-reopen"] as const) {
      if (visit === "reload") await page.reload();
      else {
        if (visit === "close-reopen") {
          await page
            .getByRole("button", { name: "保存并关闭", exact: true })
            .click();
          await expect(
            page.getByRole("dialog", { name: "更换 Agent", exact: true }),
          ).toHaveCount(0);
        } else await page.goto(`/s/${a.id}`);
      }
      await open(page);
      await expect(conflict).toBeVisible();
      await expect(history(page)).toHaveValue("device A unsaved context");
      await expect(task(page)).toHaveValue("device A unsaved task");
      // Observe beyond the 350 ms autosave debounce: an unresolved conflict
      // must not attempt a write, even after another remount.
      await page.waitForTimeout(500);
      expect(localSaves).toBe(savedAttempts);
      expect((await latest(request, a.conversationId)).preview).toEqual(
        serverVersion,
      );
    }
    await page
      .getByRole("button", { name: "读取服务端版本", exact: true })
      .click();
    await expect(conflict).toHaveCount(0);
    await expect(history(page)).toHaveValue(serverVersion.renderedText);
    await expect(task(page)).toHaveValue(serverVersion.userText);

    await editOffline("device A second context", "device A second task");
    const resumedSave = page.waitForEvent(
      "requestfailed",
      (request) =>
        request.url().endsWith(previewPath) && request.method() === "PATCH",
    );
    await page.goto(`/s/${a.id}`);
    await open(page);
    await expect(conflict).toHaveCount(0);
    await expect(task(page)).toHaveValue("device A second task");
    await resumedSave;
    // Revision 2 is now the recovery base, not the discarded revision 1.
    await page.goto("/");
    await remote.goto(`/s/${a.id}`);
    await open(remote);
    await expect(task(remote)).toHaveValue(serverVersion.userText);
    await history(remote).fill("device B second reviewed context");
    await task(remote).fill("device B second reviewed task");
    await expect
      .poll(
        async () => (await latest(request, a.conversationId)).preview.userText,
      )
      .toBe("device B second reviewed task");
    const secondServerVersion = (await latest(request, a.conversationId))
      .preview;
    expect(secondServerVersion.revision).toBe(3);
    await remote.goto("/");
    await page.goto(`/s/${a.id}`);
    await open(page);
    await expect(conflict).toBeVisible();
    await expect(history(page)).toHaveValue("device A second context");
    await expect(task(page)).toHaveValue("device A second task");
    await conflict.click();
    await expect(conflict).toHaveCount(0);
    await expect
      .poll(
        async () =>
          (await latest(request, a.conversationId)).preview?.renderedText,
      )
      .toBe("device A second context");
    const regenerated = (await latest(request, a.conversationId)).preview;
    expect(regenerated.id).not.toBe(before.id);
    expect(regenerated.manual).toBe(true);
    expect(regenerated.userText).toBe("device A second task");
    expect(await (await request.get(previewPath)).json()).toEqual(
      secondServerVersion,
    );
  } finally {
    await device.close();
    await page.unroute(`**${previewPath}`);
  }
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

test("send rejects a device revision that was never reviewed", async ({
  page,
  request,
  workDir,
}) => {
  const a = await source(request, workDir);
  await config(request, "Agent B");
  await page.goto(`/s/${a.id}`);
  await open(page);
  await target(page, "Agent B");
  await task(page).fill("my reviewed task");
  await page.getByRole("button", { name: "生成预览", exact: true }).click();
  await expect(history(page)).toHaveValue(/original source constraint/);
  const p = (await latest(request, a.conversationId)).preview;
  let changed = false,
    messages = 0;
  page.on("request", (r) => {
    if (
      r.method() === "POST" &&
      r.url().endsWith(`/api/conversations/${a.conversationId}/messages`)
    )
      messages++;
  });
  await page.route(`**/api/handoffs/${p.id}`, async (route) => {
    if (route.request().method() === "GET" && !changed) {
      changed = true;
      const response = await request.patch(`/api/handoffs/${p.id}`, {
        data: {
          revision: p.revision,
          text: "unseen other history",
          userText: "unseen other task",
        },
      });
      expect(response.ok()).toBe(true);
    }
    await route.continue();
  });
  await page.getByRole("button", { name: "发送交接", exact: true }).click();
  await expect(
    page.getByRole("alert").filter({ hasText: "尚未发送" }),
  ).toBeVisible();
  await expect(task(page)).toHaveValue("my reviewed task");
  expect(messages).toBe(0);
  await page
    .getByRole("button", { name: "读取服务端版本", exact: true })
    .click();
  await expect(task(page)).toHaveValue("unseen other task");
  await expect(history(page)).toHaveValue("unseen other history");
  expect(messages).toBe(0);
});

test("a delayed polling response cannot roll back a successful autosave", async ({
  page,
  request,
  workDir,
}) => {
  const a = await source(request, workDir);
  await config(request, "Agent B");
  await page.goto(`/s/${a.id}`);
  await open(page);
  await target(page, "Agent B");
  await task(page).fill("first reviewed task");
  await page.getByRole("button", { name: "生成预览", exact: true }).click();
  await expect(history(page)).toHaveValue(/original source constraint/);
  const p = (await latest(request, a.conversationId)).preview;
  let release!: () => void;
  const hold = new Promise<void>((resolve) => (release = resolve));
  let seen!: () => void;
  const started = new Promise<void>((resolve) => (seen = resolve));
  let held = false;
  await page.route(`**/api/handoffs/${p.id}`, async (route) => {
    if (route.request().method() === "GET" && !held) {
      held = true;
      const response = await route.fetch();
      seen();
      await hold;
      await route.fulfill({ response });
      return;
    }
    await route.continue();
  });
  await started;
  await task(page).fill("saved after the old GET began");
  await expect
    .poll(
      async () => (await latest(request, a.conversationId)).preview?.userText,
    )
    .toBe("saved after the old GET began");
  release();
  // Wait for the held response itself, rather than a fixed scheduling delay.
  await page.unrouteAll({ behavior: "wait" });
  await expect(task(page)).toHaveValue("saved after the old GET began");
  await expect(
    page.getByText("用户任务已独立保存。", { exact: true }),
  ).toBeVisible();
  await task(page).fill("later edit on the saved revision");
  await expect
    .poll(
      async () => (await latest(request, a.conversationId)).preview?.userText,
    )
    .toBe("later edit on the saved revision");
});

for (const fork of [false, true]) {
  test(`another device delivery cannot clear a newer local task (fork=${fork})`, async ({
    page,
    request,
    workDir,
  }) => {
    const a = await source(request, workDir);
    await config(request, "Agent B");
    if (fork) await forkPage(page, a.id, a.conversationId);
    else {
      await page.goto(`/s/${a.id}`);
      await open(page);
    }
    await target(page, "Agent B");
    await task(page).fill("the task sent by another device");
    await page.getByRole("button", { name: "生成预览", exact: true }).click();
    await expect(history(page)).toHaveValue(/original source constraint/);
    const p = (await latest(request, a.conversationId, fork)).preview;
    let release!: () => void;
    const hold = new Promise<void>((resolve) => (release = resolve));
    let seen!: () => void;
    const started = new Promise<void>((resolve) => (seen = resolve));
    let held = false;
    await page.route(`**/api/handoffs/${p.id}/delivery`, async (route) => {
      if (!held) {
        const response = await route.fetch();
        if (
          response.status() === 200 &&
          (await response.json()).status === "inline"
        ) {
          held = true;
          seen();
          await hold;
          await route.fulfill({ response });
          return;
        }
        await route.fulfill({ response });
        return;
      }
      await route.continue();
    });
    const sent = await request.post(
      `/api/conversations/${a.conversationId}/messages`,
      {
        data: {
          clientRequestId: "other-device-delivery",
          handoffId: p.id,
          revision: p.revision,
          text: p.userText,
          configId: p.configId,
          agentConfig: p.agentConfig,
          location: p.location,
          attachments: p.attachments,
          retry: false,
        },
      },
    );
    expect(sent.status()).toBe(202);
    await started;
    await task(page).fill("my next unsent task");
    release();
    await expect(page.getByText("已投递", { exact: true })).toBeVisible();
    await expect(task(page)).toHaveValue("my next unsent task");
    await page.reload();
    if (!fork) await open(page);
    await expect(task(page)).toHaveValue("my next unsent task");
    if (fork) {
      const sentDraft = await latest(request, a.conversationId, true);
      expect(
        (
          await request.delete(
            `/api/sessions/${sentDraft.delivery.sessionId}?forget=1`,
          )
        ).status(),
      ).toBe(204);
      expect(
        (await latest(request, a.conversationId, true)).preview,
      ).toBeNull();
      await page.reload();
      await expect(task(page)).toHaveValue("my next unsent task");
      await page.getByRole("button", { name: "生成预览", exact: true }).click();
      await expect(history(page)).toHaveValue(/original source constraint/);
      expect(
        (await latest(request, a.conversationId, true)).preview.userText,
      ).toBe("my next unsent task");
    }
  });
}

for (const action of ["send", "close"] as const) {
  test(`${action} freezes reviewed controls before awaiting a draft save`, async ({
    page,
    request,
    workDir,
  }) => {
    const a = await source(request, workDir);
    await config(request, "Agent B");
    await page.goto(`/s/${a.id}`);
    await open(page);
    await target(page, "Agent B");
    await task(page).fill("initial task");
    await page.getByRole("button", { name: "生成预览", exact: true }).click();
    await expect(history(page)).toHaveValue(/original source constraint/);
    const p = (await latest(request, a.conversationId)).preview;
    let release!: () => void;
    const hold = new Promise<void>((resolve) => (release = resolve));
    let seen!: () => void;
    const started = new Promise<void>((resolve) => (seen = resolve));
    await page.route(`**/api/handoffs/${p.id}`, async (route) => {
      if (route.request().method() === "PATCH") {
        seen();
        await hold;
      }
      await route.continue();
    });
    await task(page).fill("the reviewed saved task");
    if (action === "send")
      await page.getByRole("button", { name: "发送交接", exact: true }).click();
    else
      await page
        .getByRole("button", { name: "保存并关闭", exact: true })
        .click();
    await started;
    await expect(task(page)).toBeDisabled();
    await expect(history(page)).toBeDisabled();
    await expect(
      page.getByRole("combobox", { name: "目标 Agent", exact: true }),
    ).toBeDisabled();
    await expect(
      page.getByRole("textbox", { name: "模型（可选）", exact: true }),
    ).toBeDisabled();
    release();
    if (action === "send") {
      await expect(page.getByText("已投递", { exact: true })).toBeVisible();
      const saved = await latest(request, a.conversationId);
      expect(saved.preview.userText).toBe("the reviewed saved task");
      expect(saved.delivery.status).toBe("inline");
    } else {
      await expect(
        page.getByRole("dialog", { name: "更换 Agent", exact: true }),
      ).toHaveCount(0);
      await open(page);
      await expect(task(page)).toHaveValue("the reviewed saved task");
    }
  });
}

test("temporary source lookup failure retains unsaved manual context", async ({
  page,
  request,
  workDir,
}) => {
  const a = await source(request, workDir);
  await config(request, "Agent B");
  await page.goto(`/s/${a.id}`);
  await open(page);
  await target(page, "Agent B");
  await task(page).fill("independent protected task");
  await page.getByRole("button", { name: "生成预览", exact: true }).click();
  await expect(history(page)).toHaveValue(/original source constraint/);
  const p = (await latest(request, a.conversationId)).preview;
  await page.route(`**/api/handoffs/${p.id}`, async (route) => {
    if (route.request().method() === "PATCH") await route.abort();
    else await route.continue();
  });
  await history(page).fill("unsaved protected manual context");
  const recovery = () =>
    page.evaluate(
      (id) =>
        Object.entries(localStorage).some(
          ([key, value]) =>
            key.includes(`handoff.${id}.switch.recovery`) &&
            value.includes("unsaved protected manual context"),
        ),
      a.conversationId,
    );
  await expect.poll(recovery).toBe(true);
  const sourcePath = `**/api/conversations/${a.conversationId}/handoff-previews?fork=0`;
  await page.route(sourcePath, (route) =>
    route.fulfill({
      status: 503,
      contentType: "application/json",
      body: JSON.stringify({ error: "unavailable" }),
    }),
  );
  await page.reload();
  await open(page);
  await expect(
    page.getByRole("alert").filter({ hasText: "未发送内容已保留" }),
  ).toBeVisible();
  await expect.poll(recovery).toBe(true);
  await page.unroute(sourcePath);
  await page.unroute(`**/api/handoffs/${p.id}`);
  await page.reload();
  await open(page);
  await expect(history(page)).toHaveValue("unsaved protected manual context");
  await expect(task(page)).toHaveValue("independent protected task");
});
