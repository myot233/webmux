import "../../src/styles.css";
import { expect, test, vi } from "vitest";
import { page, userEvent } from "vitest/browser";
import { render } from "vitest-browser-svelte/pure";
import type { ChatRequest, SessionInfo } from "../../src/contracts/schemas";
import Harness from "./ChatViewHarness.svelte";
import {
  chatState,
  commands,
  events,
  installChatFixture,
  respond,
  snapshot,
  socketAt,
} from "./chat-fixture";

installChatFixture();

const id = "hybrid22";
const base = `/api/sessions/${id}/chat`;

async function open(
  location?: Pick<SessionInfo, "workspace" | "cwd">,
  allowDocumentScroll = false,
  clippedHost = false,
) {
  render(Harness, { id, location, allowDocumentScroll, clippedHost });
  return socketAt(`/ws/s/${id}/chat`);
}

const read = (toolCallId: string, kind: string, title: string) =>
  ({
    kind: "tool.upsert",
    tool: { toolCallId, kind, title, status: "completed" },
  }) as const;

test("renders the snapshot's timeline and updates a tool row in place", async () => {
  const socket = await open();
  socket.frame(
    snapshot(chatState({ turn: { running: true } }), { truncated: true }),
  );
  socket.frame({
    type: "events",
    generation: "g1",
    items: events(
      {
        kind: "message.append",
        message: { messageId: "u1", role: "user", text: "修复首页排序" },
      },
      {
        kind: "message.append",
        message: { messageId: "t1", role: "thought", text: "先找排序函数" },
      },
      read("r1", "read", "web/src/pages/Home.svelte"),
      read("r2", "search", "sortWorkspaces"),
      read("r3", "read", "web/src/homeWorktrees.ts"),
      {
        kind: "message.append",
        message: { messageId: "a1", role: "agent", text: "排序只比较了" },
      },
      {
        kind: "message.append",
        message: { messageId: "a1", role: "agent", text: "最近活动时间。" },
      },
      {
        kind: "tool.upsert",
        tool: {
          toolCallId: "edit-1",
          kind: "edit",
          title: "编辑 web/src/homeWorktrees.ts",
          status: "in_progress",
          content: [
            {
              type: "diff",
              path: "web/src/homeWorktrees.ts",
              oldText: "a\nb",
              newText: "a\nc\nd",
            },
          ],
        },
      },
    ),
  });

  await expect
    .element(page.getByText("更早的历史未缓存，可切到终端查看"))
    .toBeVisible();
  const transcript = page.getByRole("list", { name: "对话" });
  await expect.element(transcript.getByText(/修复首页排序/)).toBeVisible();
  // Agent chunks of one message join into one paragraph.
  await expect
    .element(transcript.getByText("排序只比较了最近活动时间。"))
    .toBeVisible();
  // Thinking and the tool calls after it fold into one row that shows the
  // latest call; opening it lists the thought and every call in order.
  const folded = transcript.getByRole("button", { name: /共 3 步/ });
  await expect.element(folded).toBeVisible();
  await expect
    .element(folded)
    .toMatchTextContent(/web\/src\/homeWorktrees\.ts/);
  await expect
    .element(transcript.getByText("先找排序函数"))
    .not.toBeInTheDocument();
  await expect
    .element(transcript.getByText("sortWorkspaces"))
    .not.toBeInTheDocument();
  await folded.click();
  await expect.element(transcript.getByText("sortWorkspaces")).toBeVisible();
  const thought = transcript.getByRole("button", { name: "已思考" });
  await expect.element(thought).toHaveAttribute("aria-expanded", "false");
  await expect
    .element(transcript.getByText("先找排序函数"))
    .not.toBeInTheDocument();
  await thought.click();
  await expect.element(transcript.getByText("先找排序函数")).toBeVisible();

  const edit = transcript.getByRole("button", {
    name: /编辑 web\/src\/homeWorktrees\.ts/,
  });
  await expect.element(edit).toMatchTextContent(/新增 2 行，删除 1 行/);
  await expect.element(edit).toMatchTextContent(/进行中/);
  socket.frame({
    type: "events",
    generation: "g1",
    items: events({
      kind: "tool.upsert",
      tool: { toolCallId: "edit-1", status: "completed" },
    }),
  });
  await expect.element(edit).toMatchTextContent(/完成/);
  await expect.element(edit).toMatchTextContent(/新增 2 行，删除 1 行/);
  expect(document.querySelectorAll('[data-tool-call="edit-1"]')).toHaveLength(
    1,
  );
  await edit.click();
  await transcript.getByRole("button", { name: "在代码面板中查看" }).click();
  await expect
    .element(page.getByRole("status", { name: "Reviewed path" }))
    .toHaveTextContent("web/src/homeWorktrees.ts");
});

test("opens code links from agent messages, tool locations, resource links and read rows", async () => {
  const socket = await open();
  socket.frame(snapshot(chatState()));
  socket.frame({
    type: "events",
    generation: "g1",
    items: events(
      {
        kind: "tool.upsert",
        tool: {
          toolCallId: "read-single",
          kind: "read",
          title: "读取 Home.svelte",
          status: "completed",
          locations: [{ path: "web/src/pages/Home.svelte", line: 48 }],
        },
      },
      {
        kind: "tool.upsert",
        tool: {
          toolCallId: "read-multi",
          kind: "search",
          title: "搜索 sortWorkspaces",
          status: "completed",
          locations: [
            { path: "web/src/homeWorktrees.ts", line: 15 },
            { path: "web/src/pages/Home.svelte", line: 60 },
          ],
        },
      },
      {
        kind: "message.append",
        message: {
          messageId: "a-links",
          role: "agent",
          text: [
            "请查看 [sortWorkspaces](web/src/homeWorktrees.ts#L15-L30)",
            "以及 [Home.svelte:48](file:///repo/web/src/pages/Home.svelte#L48)",
            "或 [文档说明](docs/hybrid-sessions.md#聊天视图)。",
          ].join(" "),
        },
      },
      {
        kind: "tool.upsert",
        tool: {
          toolCallId: "search-tool",
          kind: "other",
          title: "定位引用",
          status: "completed",
          locations: [{ path: "/repo/internal/session/manager.go", line: 120 }],
          content: [
            {
              type: "resource_link",
              uri: "file:///repo/internal/api/routes.go#L42",
              name: "routes.go",
            },
          ],
        },
      },
    ),
  });

  const transcript = page.getByRole("list", { name: "对话" });
  const opened = page.getByRole("status", { name: "Opened location" });
  const readOpened = () => JSON.parse(opened.element().textContent || "null");

  await transcript.getByRole("button", { name: /共 2 步/ }).click();
  await transcript.getByRole("button", { name: /读取 Home.svelte/ }).click();
  await transcript
    .getByRole("button", { name: "web/src/pages/Home.svelte:48" })
    .click();
  await expect.poll(readOpened).toEqual({
    path: "web/src/pages/Home.svelte",
    line: 48,
  });
  await transcript
    .getByRole("button", { name: /搜索 sortWorkspaces/ })
    .last()
    .click();
  await transcript
    .getByRole("button", { name: "web/src/pages/Home.svelte:60" })
    .click();
  await expect.poll(readOpened).toEqual({
    path: "web/src/pages/Home.svelte",
    line: 60,
  });

  await transcript.getByRole("link", { name: "sortWorkspaces" }).click();
  await expect.poll(readOpened).toEqual({
    path: "web/src/homeWorktrees.ts",
    line: 15,
    endLine: 30,
  });

  await transcript.getByRole("link", { name: "Home.svelte:48" }).click();
  await expect.poll(readOpened).toEqual({
    path: "/repo/web/src/pages/Home.svelte",
    fromUri: true,
    line: 48,
  });

  await transcript.getByRole("link", { name: "文档说明" }).click();
  await expect.poll(readOpened).toEqual({
    path: "docs/hybrid-sessions.md",
    heading: "聊天视图",
  });

  await transcript.getByRole("button", { name: /定位引用/ }).click();
  await transcript
    .getByRole("button", { name: "/repo/internal/session/manager.go:120" })
    .click();
  await expect.poll(readOpened).toEqual({
    path: "/repo/internal/session/manager.go",
    line: 120,
  });

  await transcript.getByRole("button", { name: "routes.go" }).click();
  await expect.poll(readOpened).toEqual({
    path: "/repo/internal/api/routes.go",
    fromUri: true,
    line: 42,
  });
});

test("shows server-timed progress and keeps the final duration", async () => {
  const socket = await open();
  vi.useFakeTimers({
    now: new Date("2026-01-01T12:00:00Z"),
    toFake: ["Date", "setInterval", "clearInterval"],
  });
  try {
    const startedAt = "2026-01-01T11:58:55Z";
    socket.frame(
      snapshot(chatState({ turn: { running: true, turnId: "t1", startedAt } })),
    );
    const status = page
      .getByRole("status")
      .filter({ hasText: /正在工作|已完成|已结束/ });
    await expect.element(status).toHaveTextContent("正在工作… · 1 分 05 秒");
    await expect
      .element(status.locator("span.tabular-nums"))
      .toHaveAttribute("aria-hidden", "true");
    await vi.advanceTimersByTimeAsync(1000);
    await expect.element(status).toHaveTextContent("正在工作… · 1 分 06 秒");

    socket.frame({
      type: "state",
      state: chatState({
        turn: {
          running: false,
          turnId: "t1",
          stopReason: "end_turn",
          startedAt,
          endedAt: "2026-01-01T12:01:00Z",
        },
      }),
    });
    await expect.element(status).toHaveTextContent("已完成 · 用时 2 分 05 秒");

    socket.frame({
      type: "state",
      state: chatState({
        turn: {
          running: false,
          turnId: "t1",
          stopReason: "error",
          startedAt,
          endedAt: "2026-01-01T12:01:00Z",
        },
      }),
    });
    await expect.element(status).toHaveTextContent("已结束 · 用时 2 分 05 秒");
    expect(
      document
        .querySelector('[data-turn-result="error"] svg')
        ?.classList.contains("text-destructive"),
    ).toBe(true);

    socket.frame({
      type: "state",
      state: chatState({
        turn: {
          running: true,
          turnId: "t2",
          startedAt: new Date().toISOString(),
        },
      }),
    });
    await expect.element(status).toHaveTextContent("正在工作… · 0 秒");
  } finally {
    vi.useRealTimers();
  }
});

test("an end_turn with a tool still running is not reported as completed", async () => {
  const socket = await open();
  socket.frame(snapshot(chatState({ turn: { running: true, turnId: "t1" } })));
  socket.frame({
    type: "events",
    generation: "g1",
    items: events({
      kind: "tool.upsert",
      tool: {
        toolCallId: "bg-1",
        kind: "execute",
        title: "sleep 25",
        status: "in_progress",
      },
    }),
  });
  socket.frame({
    type: "state",
    state: chatState({
      turn: {
        running: false,
        unfinished: true,
        turnId: "t1",
        stopReason: "end_turn",
        startedAt: "2026-01-01T11:58:55Z",
        endedAt: "2026-01-01T12:01:00Z",
      },
    }),
  });
  await expect
    .element(page.getByRole("status").filter({ hasText: /用时/ }))
    .toHaveTextContent("本轮已结束，仍有任务未完成 · 用时 2 分 05 秒");

  // Reconnecting after the tool event fell out of the cached log still shows
  // the unfinished label from `turn.unfinished`.
  socket.frame(
    snapshot(
      chatState({
        turn: {
          running: false,
          unfinished: true,
          turnId: "t1",
          stopReason: "end_turn",
          startedAt: "2026-01-01T11:58:55Z",
          endedAt: "2026-01-01T12:01:00Z",
        },
      }),
      { generation: "g2", reset: true, truncated: true },
    ),
  );
  await expect
    .element(page.getByRole("status").filter({ hasText: /用时/ }))
    .toHaveTextContent("本轮已结束，仍有任务未完成 · 用时 2 分 05 秒");
});

test("a missing connector offers its install hint and the terminal; outdated parts each warn", async () => {
  const socket = await open();
  socket.frame(
    snapshot(
      chatState({
        outdated: [
          { name: "codex", version: "0.150.0", verified: "0.166.0" },
          { name: "codex-acp", version: "2.4.0", verified: "2.5.0" },
        ],
        connectorMissing: {
          executable: "codex-acp",
          title: "Codex ACP 适配器",
          installCommand: "npm install -g @agentclientprotocol/codex-acp",
          docsUrl: "https://github.com/agentclientprotocol/codex-acp",
        },
      }),
    ),
  );
  await expect
    .element(page.getByText("未安装 Codex ACP 适配器（codex-acp）"))
    .toBeVisible();
  await expect
    .element(
      page.getByText(
        "codex 0.150.0 低于已验证版本 0.166.0，部分功能可能不可用",
      ),
    )
    .toBeVisible();
  await expect
    .element(
      page.getByText(
        "codex-acp 2.4.0 低于已验证版本 2.5.0，部分功能可能不可用",
      ),
    )
    .toBeVisible();
  await page.getByRole("button", { name: "切到终端", exact: true }).click();
  await expect
    .element(page.getByRole("status", { name: "Terminal requests" }))
    .toHaveTextContent("1");
});

test("renders the attachments of a user prompt", async () => {
  const png = new Uint8Array([
    0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d,
    0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
    0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4, 0x89, 0x00, 0x00, 0x00,
    0x0a, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00,
    0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00, 0x00, 0x00, 0x00, 0x49,
    0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82,
  ]);
  respond(
    /^GET \/api\/sessions\/hybrid22\/upload$/,
    () => new Response(png, { headers: { "Content-Type": "image/png" } }),
  );
  const socket = await open();
  socket.frame(snapshot(chatState()));
  socket.frame({
    type: "events",
    generation: "g1",
    items: events({
      kind: "message.append",
      message: {
        messageId: "u1",
        role: "user",
        text: "",
        attachments: [
          {
            path: "/tmp/webmux-upload-a/shot.png",
            name: "shot.png",
            mimeType: "image/png",
            size: 12,
            kind: "image",
          },
          {
            path: "/tmp/webmux-upload-a/notes.txt",
            name: "notes.txt",
            mimeType: "text/plain",
            size: 5,
            kind: "file",
          },
        ],
      },
    }),
  });
  const attachments = page.getByRole("list", { name: "附件" });
  await expect.element(attachments.getByText("shot.png")).toBeVisible();
  await expect.element(attachments.getByText("notes.txt")).toBeVisible();
  const thumb = attachments.getByRole("img", { name: "shot.png" });
  await expect.element(thumb).toBeVisible();
  expect(thumb.element().getAttribute("src")).toMatch(/^blob:/);
});

test("the composer posts the prompt with a request ID and mode auto", async () => {
  respond(/^POST .*\/input$/, () =>
    Response.json({ view: "chat", queued: false }),
  );
  const socket = await open();
  socket.frame(snapshot(chatState()));
  const input = page.getByRole("textbox", { name: "消息" });
  await input.fill("补一个回归测试");
  await page.getByRole("button", { name: "发送", exact: true }).click();
  await expect.poll(commands).toEqual([
    {
      method: "POST",
      path: `/api/sessions/${id}/input`,
      body: {
        clientRequestId: expect.stringMatching(/\S/),
        text: "补一个回归测试",
        mode: "auto",
      },
    },
  ]);
  await expect.element(input).toHaveValue("");
});

test("a paused queue shows its prompts and continues or clears", async () => {
  const socket = await open();
  socket.frame(snapshot(chatState()));
  socket.frame({
    type: "state",
    state: chatState({
      turn: { running: false, stopReason: "cancelled" },
      queue: {
        paused: true,
        items: [
          {
            id: "q1",
            text: "然后更新 README",
            createdAt: "2026-10-28T00:00:00Z",
          },
          { id: "q2", text: "最后运行测试", createdAt: "2026-10-28T00:00:01Z" },
        ],
      },
    }),
  });
  const queue = page.getByRole("region", { name: "排队的消息" });
  await expect.element(queue).toMatchTextContent(/排队 2/);
  await expect
    .element(queue.getByRole("status"))
    .toHaveTextContent("回合已中断，队列暂停");
  await expect
    .element(queue.getByRole("textbox", { name: "编辑排队消息 1" }))
    .toHaveValue("然后更新 README");
  await expect
    .element(queue.getByRole("textbox", { name: "编辑排队消息 2" }))
    .toHaveValue("最后运行测试");
  // While the queue is paused a new prompt joins it.
  await expect
    .element(page.getByRole("button", { name: "加入排队" }))
    .toBeInTheDocument();

  await queue.getByRole("button", { name: "继续" }).click();
  await queue.getByRole("button", { name: "清空" }).click();
  await expect
    .poll(() => commands().map((request) => request.path))
    .toEqual([`${base}/queue/resume`, `${base}/queue/clear`]);
  for (const request of commands())
    expect(request.body).toEqual({ clientRequestId: expect.any(String) });

  socket.frame({ type: "state", state: chatState() });
  await expect.element(queue).not.toBeInTheDocument();
});

const permission: ChatRequest = {
  requestId: "perm-1",
  kind: "permission",
  toolCallId: "exec-1",
  options: [
    { optionId: "once", name: "允许一次", kind: "allow_once" },
    { optionId: "always", name: "始终允许", kind: "allow_always" },
    { optionId: "reject", name: "拒绝", kind: "reject_once" },
  ],
};

async function awaitingPermission() {
  const socket = await open();
  socket.frame(snapshot(chatState({ turn: { running: true } })));
  socket.frame({
    type: "events",
    generation: "g1",
    items: events(
      {
        kind: "tool.upsert",
        tool: {
          toolCallId: "exec-1",
          kind: "execute",
          title: "pnpm --dir web exec vitest run",
          status: "pending",
        },
      },
      { kind: "request.open", request: permission },
    ),
  });
  socket.frame({
    type: "state",
    state: chatState({ turn: { running: true }, requests: [permission] }),
  });
  const prompt = page.getByRole("region", { name: "等待授权" });
  await expect
    .element(prompt)
    .toMatchTextContent(/pnpm --dir web exec vitest run/);
  // The request takes the input's place.
  await expect
    .element(page.getByRole("textbox", { name: "消息" }))
    .not.toBeInTheDocument();
  return { socket, prompt };
}

test("answering a permission request posts the chosen option", async () => {
  const { prompt } = await awaitingPermission();
  expect(prompt.element().classList.contains("border-destructive/40")).toBe(
    true,
  );
  const toolButton = page.getByRole("button", {
    name: /pnpm --dir web exec vitest run/,
  });
  await expect.element(toolButton).toMatchTextContent(/等待授权/);
  expect(
    toolButton.element().querySelector(".text-destructive"),
  ).not.toBeNull();
  await prompt.getByRole("button", { name: /允许一次/ }).click();
  await expect.poll(commands).toEqual([
    {
      method: "POST",
      path: `${base}/requests/perm-1`,
      body: {
        clientRequestId: expect.any(String),
        outcome: "selected",
        optionId: "once",
      },
    },
  ]);
});

test("a request answered on another device closes with the next state", async () => {
  const { socket, prompt } = await awaitingPermission();
  socket.frame({
    type: "events",
    generation: "g1",
    items: events({
      kind: "request.close",
      resolved: {
        requestId: "perm-1",
        outcome: "selected",
        optionId: "reject",
      },
    }),
  });
  socket.frame({
    type: "state",
    state: chatState({ turn: { running: true } }),
  });
  await expect.element(prompt).not.toBeInTheDocument();
  await expect
    .element(page.getByRole("textbox", { name: "消息" }))
    .toBeVisible();
  await expect
    .element(
      page.getByRole("button", { name: /pnpm --dir web exec vitest run/ }),
    )
    .toMatchTextContent(/已拒绝/);
  expect(commands()).toEqual([]);
});

test("Ctrl+Enter steers only while a turn runs on an agent that takes it", async () => {
  respond(/^POST .*\/input$/, () =>
    Response.json({ view: "chat", queued: false }),
  );
  const socket = await open();
  expect(page.getByRole("button", { name: "插话" }).elements()).toHaveLength(0);
  socket.frame(snapshot(chatState({ steering: true })));
  await page.getByRole("textbox", { name: "消息" }).fill("顺便检查空列表");
  await expect
    .element(page.getByRole("button", { name: "发送" }))
    .toBeVisible();

  socket.frame({
    type: "state",
    state: chatState({ steering: false, turn: { running: true } }),
  });
  // With text typed the send button queues instead of stopping.
  await expect
    .element(page.getByRole("button", { name: "加入排队" }))
    .toBeVisible();

  socket.frame({
    type: "state",
    state: chatState({ steering: true, turn: { running: true } }),
  });
  await page.getByRole("textbox", { name: "消息" }).click();
  await userEvent.keyboard("{Control>}{Enter}{/Control}");
  await expect.poll(commands).toEqual([
    {
      method: "POST",
      path: `/api/sessions/${id}/input`,
      body: {
        clientRequestId: expect.any(String),
        text: "顺便检查空列表",
        mode: "steer",
      },
    },
  ]);
});

test("the send button stops a running turn while there is nothing to send", async () => {
  respond(/^POST .*\/interrupt$/, () => new Response(null, { status: 204 }));
  const socket = await open();
  socket.frame(snapshot(chatState({ turn: { running: true } })));
  const input = page.getByRole("textbox", { name: "消息" });
  await expect
    .element(page.getByRole("button", { name: "停止" }))
    .toBeVisible();
  expect(
    page.getByRole("button", { name: "加入排队" }).elements(),
  ).toHaveLength(0);

  await input.fill("下一步");
  await expect
    .element(page.getByRole("button", { name: "加入排队" }))
    .toBeVisible();
  expect(page.getByRole("button", { name: "停止" }).elements()).toHaveLength(0);

  await input.fill("");
  await page.getByRole("button", { name: "停止" }).click();
  await expect.poll(commands).toEqual([
    {
      method: "POST",
      path: `/api/sessions/${id}/interrupt`,
      body: { clientRequestId: expect.any(String) },
    },
  ]);
});

test("the transcript stays at its end when the composer grows on a phone", async () => {
  await page.viewport(390, 700);
  try {
    const socket = await open();
    socket.frame(snapshot(chatState()));
    socket.frame({
      type: "events",
      generation: "g1",
      items: events(
        ...Array.from({ length: 30 }, (_, index) => ({
          kind: "message.append" as const,
          message: {
            messageId: `m${index}`,
            role: "agent" as const,
            text: `第 ${index} 条回复`,
          },
        })),
      ),
    });
    const transcript = page.getByRole("list", { name: "对话" });
    await expect.element(transcript.getByText("第 29 条回复")).toBeVisible();
    const scroller = transcript
      .element()
      .closest("[data-slot=scroll-area-viewport]")!;
    const atEnd = () =>
      Math.abs(
        scroller.scrollHeight - scroller.clientHeight - scroller.scrollTop,
      ) < 2;
    expect(atEnd()).toBe(true);

    // Typing expands the compact bar; the newest reply must stay in view.
    await page.getByRole("textbox", { name: "消息" }).fill("a\nb\nc\nd");
    await expect.poll(atEnd).toBe(true);
    await expect.element(transcript.getByText("第 29 条回复")).toBeVisible();
  } finally {
    await page.viewport(1024, 900);
  }
});

test("conversation ticks distinguish roles, jump to history and preserve reading position", async () => {
  await page.viewport(1280, 900);
  try {
    const socket = await open();
    socket.frame(snapshot(chatState()));
    socket.frame({
      type: "events",
      generation: "g1",
      items: events(
        ...Array.from({ length: 10 }, (_, index) => [
          {
            kind: "message.append" as const,
            message: {
              messageId: `nav-u${index}`,
              role: "user" as const,
              text: `第 ${index + 1} 轮请求`,
            },
          },
          {
            kind: "message.append" as const,
            message: {
              messageId: `nav-a${index}`,
              role: "agent" as const,
              text: Array.from(
                { length: 8 },
                (_, line) => `第 ${index + 1} 轮回复，段落 ${line + 1}`,
              ).join("\n\n"),
            },
          },
        ]).flat(),
      ),
    });

    const navigation = page.getByRole("navigation", { name: "对话轮次" });
    await expect.element(navigation).toBeVisible();
    const first = navigation.getByRole("button", {
      name: "第 1 轮 · 你：第 1 轮请求",
    });
    const firstAgent = navigation.getByRole("button", {
      name: /^第 1 轮 · AI：第 1 轮回复，段落 1/,
    });
    await expect.element(first).toBeVisible();
    await expect.element(firstAgent).toBeVisible();
    expect(navigation.getByRole("button").elements()).toHaveLength(20);
    expect(
      getComputedStyle(first.element().querySelector("span")!).backgroundColor,
    ).not.toBe(
      getComputedStyle(firstAgent.element().querySelector("span")!)
        .backgroundColor,
    );

    const transcript = page.getByRole("list", { name: "对话" });
    const scroller = transcript
      .element()
      .closest("[data-slot=scroll-area-viewport]") as HTMLElement;
    await expect.poll(() => scroller.scrollTop).toBeGreaterThan(0);
    await first.click();
    await expect.element(first).toHaveAttribute("aria-current", "location");
    await expect.poll(() => scroller.scrollTop).toBeLessThan(100);
    await userEvent.keyboard("{ArrowDown}");
    await expect.element(firstAgent).toHaveFocus();
    await expect
      .element(firstAgent)
      .toHaveAttribute("aria-current", "location");
    await userEvent.keyboard("{ArrowUp}");
    await expect.element(first).toHaveFocus();

    const readingOffset = scroller.scrollTop;
    socket.frame({
      type: "events",
      generation: "g1",
      items: events({
        kind: "message.append",
        message: {
          messageId: "nav-new-1",
          role: "agent",
          text: "历史阅读期间的新回复",
        },
      }),
    });
    await expect
      .element(transcript.getByText("历史阅读期间的新回复"))
      .toBeVisible();
    expect(scroller.scrollTop).toBe(readingOffset);

    await page.getByRole("button", { name: "回到最新" }).click();
    await expect
      .poll(
        () =>
          scroller.scrollTop + scroller.clientHeight >=
          scroller.scrollHeight - 2,
      )
      .toBe(true);
    socket.frame({
      type: "events",
      generation: "g1",
      items: events({
        kind: "message.append",
        message: {
          messageId: "nav-new-2",
          role: "agent",
          text: "回到最新后的新回复",
        },
      }),
    });
    await expect
      .element(transcript.getByText("回到最新后的新回复"))
      .toBeVisible();
    await expect
      .poll(
        () =>
          scroller.scrollTop + scroller.clientHeight >=
          scroller.scrollHeight - 2,
      )
      .toBe(true);

    const nearEnd = navigation.getByRole("button", {
      name: /^第 10 轮 · AI：历史阅读期间的新回复/,
    });
    await nearEnd.click();
    const nearEndOffset = scroller.scrollTop;
    socket.frame({
      type: "events",
      generation: "g1",
      items: events({
        kind: "message.append",
        message: {
          messageId: "nav-new-3",
          role: "agent",
          text: Array.from(
            { length: 8 },
            (_, line) => `末尾历史跳转后的新段落 ${line + 1}`,
          ).join("\n\n"),
        },
      }),
    });
    await expect
      .element(transcript.getByText("末尾历史跳转后的新段落 8"))
      .toBeVisible();
    expect(scroller.scrollTop).toBe(nearEndOffset);

    await page.viewport(390, 844);
    await expect
      .poll(() =>
        getComputedStyle(document.querySelector('nav[aria-label="对话轮次"]')!),
      )
      .toHaveProperty("display", "none");
    expect(document.documentElement.scrollWidth).toBeLessThanOrEqual(390);
  } finally {
    await page.viewport(1024, 900);
  }
});

test("turn navigation ignores invisible replies and never scrolls a clipped chat host", async () => {
  await page.viewport(1280, 900);
  try {
    const socket = await open(undefined, false, true);
    socket.frame(snapshot(chatState()));
    socket.frame({
      type: "events",
      generation: "g1",
      items: events(
        ...Array.from({ length: 80 }, (_, index) => ({
          kind: "message.append" as const,
          message: {
            messageId: `bounded-${index}`,
            role: "agent" as const,
            text: `回复 ${index + 1}\n\n${"可见内容。".repeat(100)}`,
          },
        })),
        {
          kind: "message.append",
          message: { messageId: "empty", role: "agent", text: "" },
        },
        {
          kind: "message.append",
          message: { messageId: "whitespace", role: "agent", text: " \n " },
        },
      ),
    });
    const navigation = page.getByRole("navigation", { name: "对话轮次" });
    await expect.element(navigation).toBeVisible();
    expect(navigation.getByRole("button").elements()).toHaveLength(80);
    expect(
      document.querySelector('[data-conversation-message="empty"]'),
    ).toBeNull();
    expect(
      document.querySelector('[data-conversation-message="whitespace"]'),
    ).toBeNull();
    const host = document.querySelector<HTMLElement>("[data-chat-test-host]")!;
    const chat = document.querySelector<HTMLElement>("[data-chat-view]")!;
    const dock = document.querySelector<HTMLElement>("[data-chat-dock]")!;
    const scroller = chat.querySelector<HTMLElement>(
      "[data-slot=scroll-area-viewport]",
    )!;
    const ticks = navigation.element().firstElementChild as HTMLElement;
    host.scrollTop = 48;
    ticks.scrollTop = 0;
    const origin = {
      host: host.scrollTop,
      window: window.scrollY,
      chat: chat.getBoundingClientRect().top,
      dock: dock.getBoundingClientRect().top,
    };
    const first = navigation.getByRole("button", {
      name: /^此前 · AI：回复 1 /,
    });
    await first.click();
    await expect.poll(() => scroller.scrollTop).toBeLessThan(100);
    await userEvent.keyboard("{End}");
    await expect
      .element(navigation.getByRole("button", { name: /^此前 · AI：回复 80 / }))
      .toHaveFocus();
    await expect.poll(() => scroller.scrollTop).toBeGreaterThan(1000);
    const focused = document.activeElement!.getBoundingClientRect();
    const tickBounds = ticks.getBoundingClientRect();
    expect(focused.top).toBeGreaterThanOrEqual(tickBounds.top);
    expect(focused.bottom).toBeLessThanOrEqual(tickBounds.bottom);
    expect({
      host: host.scrollTop,
      window: window.scrollY,
      chat: chat.getBoundingClientRect().top,
      dock: dock.getBoundingClientRect().top,
    }).toEqual(origin);
  } finally {
    await page.viewport(1024, 900);
  }
});

test("typing / lists and filters published commands without authentication flows", async () => {
  const socket = await open();
  socket.frame(
    snapshot(
      chatState({
        commands: [
          { name: "compact", description: "压缩上下文" },
          { name: "review", description: "审阅当前改动", hint: "文件" },
          { name: "reset", description: "开始新会话" },
        ],
      }),
    ),
  );
  const input = page.getByRole("textbox", { name: "消息" });
  const menu = page.getByRole("listbox", { name: "命令" });
  await input.fill("/");
  await expect.element(menu).toBeVisible();
  expect(menu.getByRole("option").elements()).toHaveLength(3);
  // The profile removes these before publication; there is no other auth UI.
  expect(
    menu.getByRole("option", { name: /^\/(login|logout)/ }).elements(),
  ).toEqual([]);

  await input.fill("/re");
  await expect
    .poll(() =>
      menu
        .getByRole("option")
        .elements()
        .map((o) => o.textContent),
    )
    .toEqual([
      expect.stringContaining("/review"),
      expect.stringContaining("/reset"),
    ]);
  await expect
    .element(menu.getByRole("option", { selected: true }))
    .toMatchTextContent(/^\/review/);
  await userEvent.keyboard("{ArrowDown}");
  await expect
    .element(menu.getByRole("option", { selected: true }))
    .toMatchTextContent(/^\/reset/);
  await userEvent.keyboard("{Tab}");
  await expect.element(input).toHaveValue("/reset ");
  await expect.element(menu).not.toBeInTheDocument();
  expect(commands()).toEqual([]);

  await input.fill("/zz");
  await expect.element(menu).not.toBeInTheDocument();
});

test("the permission select sets the session's mode and shows the server's", async () => {
  let release!: () => void;
  respond(
    /^PUT .*\/chat\/permission-mode$/,
    () =>
      new Promise<Response>((resolve) => {
        release = () => resolve(new Response(null, { status: 204 }));
      }),
  );
  const socket = await open();
  socket.frame(snapshot(chatState({ permissionMode: "ask" })));
  const select = page.getByRole("button", { name: "权限：每次询问" });
  await expect
    .element(select.getByText("每次询问", { exact: true }))
    .toBeVisible();
  await expect
    .element(select)
    .toHaveAttribute("title", "权限：每次工具调用都询问");
  await select.click();
  await page.getByRole("menuitemradio", { name: "全部接受" }).click();
  await expect.poll(commands).toEqual([
    {
      method: "PUT",
      path: `${base}/permission-mode`,
      body: { clientRequestId: expect.stringMatching(/\S/), mode: "auto" },
    },
  ]);
  // The select waits for the server while the change is on its way.
  await expect.element(select).toBeDisabled();
  release();
  socket.frame({
    type: "state",
    state: chatState({ permissionMode: "auto" }),
  });
  const auto = page.getByRole("button", { name: "权限：全部接受" });
  await expect.element(auto).toBeEnabled();
  await expect
    .element(auto.getByText("全部接受", { exact: true }))
    .toBeVisible();
  await expect
    .element(auto)
    .toHaveAttribute(
      "title",
      "权限：全部接受 Agent 的工具调用请求（表单仍需回答）",
    );

  respond(/^PUT .*\/chat\/permission-mode$/, () =>
    Response.json({ error: "混合会话正在切换" }, { status: 409 }),
  );
  // The menu stays open on a pick, showing the server's value.
  await expect
    .element(page.getByRole("menuitemradio", { name: "全部接受" }))
    .toHaveAttribute("aria-checked", "true");
  await page.getByRole("menuitemradio", { name: "每次询问" }).click();
  await expect.element(page.getByText("混合会话正在切换")).toBeVisible();
  await expect.element(auto).toBeEnabled();
  expect(commands()).toHaveLength(2);
});

test("config options expose the actual fallback mode and adapter notice", async () => {
  const socket = await open();
  socket.frame(
    snapshot(
      chatState({
        config: [
          {
            id: "model",
            name: "模型",
            category: "model",
            type: "select",
            value: "smart",
            options: [
              { value: "fast", name: "Fast", description: "Low latency" },
              { value: "smart", name: "Smart", description: "Deep reasoning" },
              {
                value: "vision",
                name: "Vision Pro",
                description: "Multimodal",
              },
            ],
          },
          {
            id: "mode",
            name: "模式",
            category: "mode",
            type: "select",
            value: "acceptEdits",
            options: [
              { value: "auto", name: "auto" },
              { value: "acceptEdits", name: "acceptEdits" },
            ],
          },
          {
            id: "memory",
            name: "记忆",
            category: "model_config",
            type: "boolean",
            enabled: true,
            options: [],
          },
        ],
      }),
    ),
  );
  socket.frame({
    type: "events",
    generation: "g1",
    items: events({
      kind: "notice",
      notice: {
        level: "warning",
        text: "当前模型不支持 auto，实际权限模式已回退到 acceptEdits",
      },
    }),
  });
  const model = page.getByRole("button", { name: "模型：Smart" });
  await expect.element(model).toHaveAttribute("title", "模型：Smart");
  await expect
    .element(
      page
        .getByRole("button", { name: "模式：acceptEdits" })
        .getByText("acceptEdits", { exact: true }),
    )
    .toBeVisible();
  expect(page.getByRole("button", { name: "模式：auto" }).elements()).toEqual(
    [],
  );
  await expect
    .element(
      page.getByText("当前模型不支持 auto，实际权限模式已回退到 acceptEdits"),
    )
    .toBeVisible();
  await expect.element(model.getByText("Smart", { exact: true })).toBeVisible();
  await expect
    .element(page.getByRole("switch", { name: "记忆：已开启" }))
    .toBeChecked();

  await model.click();
  const modelSearch = page.getByRole("combobox", { name: "模型" });
  const modelList = page.getByRole("listbox", { name: "模型列表" });
  await expect
    .poll(() => modelList.getByRole("option").elements())
    .toHaveLength(3);
  await modelSearch.fill("nonexistent");
  await expect.element(page.getByText("没有匹配的模型")).toBeVisible();
  // Closing the popover resets the search filter for the next opening.
  await userEvent.keyboard("{Escape}");
  await model.click();
  await expect.element(modelSearch).toHaveValue("");
  await expect
    .poll(() => modelList.getByRole("option").elements())
    .toHaveLength(3);
  await modelSearch.fill("low latency");
  await expect
    .element(modelList.getByRole("option", { name: /Fast/ }))
    .toBeVisible();
  await expect
    .element(modelList.getByRole("option", { name: /Smart/ }))
    .not.toBeInTheDocument();
  await modelList.getByRole("option", { name: /Fast/ }).click();
  await expect.poll(commands).toEqual([
    {
      method: "PUT",
      path: `${base}/config/model`,
      body: { clientRequestId: expect.stringMatching(/\S/), value: "fast" },
    },
  ]);
});

test("narrow screens move session options into a sheet beside the upload button", async () => {
  await page.viewport(320, 800);
  try {
    const config = [
      {
        id: "model",
        name: "Model",
        type: "select" as const,
        value: "a",
        options: [
          { value: "a", name: "A" },
          { value: "b", name: "B" },
        ],
      },
    ];
    const socket = await open({
      workspace: {
        repositoryId: "repo1",
        repositoryName: "webmux",
        worktreeId: "wt1",
        worktreePath: "/repos/webmux",
        branch: "main",
        detached: false,
      },
      cwd: "/repos/webmux",
    });
    socket.frame(snapshot(chatState({ config })));
    await expect
      .element(page.getByRole("toolbar", { name: "会话选项" }))
      .not.toBeInTheDocument();
    // Idle on a narrow screen, the composer collapses into a single-line bar
    // with only the message input and send button.
    const where = page.getByRole("group", {
      name: "会话位置",
      includeHidden: true,
    });
    const optionsButton = page.getByRole("button", {
      name: "会话选项",
      includeHidden: true,
    });
    await expect.element(where).not.toBeVisible();
    await expect.element(optionsButton).not.toBeVisible();
    await expect
      .element(page.getByRole("button", { name: "发送", exact: true }))
      .toBeVisible();

    // Settings appear inside the card only after entering compose mode.
    const input = page.getByRole("textbox", { name: "消息" });
    await expect.element(input).not.toHaveFocus();
    await input.click();
    await expect.element(where).toBeVisible();
    await page.getByRole("button", { name: "会话选项" }).click();
    await expect.element(page.getByLabelText("权限")).toBeVisible();
    const model = page.getByRole("button", { name: "Model", exact: true });
    await expect.element(model).toHaveTextContent("A");
    await model.click();
    await page.getByRole("combobox", { name: "Model" }).fill("b");
    await page.getByRole("option", { name: "B", exact: true }).click();
    await expect.poll(commands).toEqual([
      {
        method: "PUT",
        path: `${base}/config/model`,
        body: { clientRequestId: expect.stringMatching(/\S/), value: "b" },
      },
    ]);
    await expect
      .element(page.getByRole("dialog", { name: "会话选项" }))
      .toBeVisible();
    await userEvent.keyboard("{Escape}");

    // With draft text present, blurring keeps the composer expanded.
    await input.fill("保留展开");
    (input.element() as HTMLTextAreaElement).blur();
    await expect.element(where).toBeVisible();
    await expect.element(optionsButton).toBeVisible();

    // Clearing the text and blurring returns to the compact bar.
    await input.fill("");
    (input.element() as HTMLTextAreaElement).blur();
    await expect.element(where).not.toBeVisible();
    await expect.element(optionsButton).not.toBeVisible();
    // The resting bar stays one line of transcript-friendly height with
    // symmetric vertical inset and no residual bottom margin from hidden rows.
    const card = document.querySelector("[data-composer-card]")!;
    // Sized in rem, so it grows with the mobile root font size.
    await expect
      .poll(() => card.getBoundingClientRect().height)
      .toBeLessThanOrEqual(46);
    expect(getComputedStyle(card).marginBottom).toBe("0px");
    const action = card.querySelector("[data-composer-action]")!;
    await expect
      .poll(() => {
        const cardBounds = card.getBoundingClientRect();
        const actionBounds = action.getBoundingClientRect();
        return Math.abs(
          actionBounds.top -
            cardBounds.top -
            (cardBounds.bottom - actionBounds.bottom),
        );
      })
      .toBeLessThanOrEqual(1);
  } finally {
    await page.viewport(1024, 900);
  }
});

test("mobile options keep confirmed values on rejection and show errors inside the sheet", async () => {
  await page.viewport(375, 667);
  try {
    const config = [
      {
        id: "model",
        name: "模型",
        type: "select" as const,
        value: "a",
        options: [
          { value: "a", name: "A" },
          { value: "b", name: "B" },
        ],
      },
      {
        id: "memory",
        name: "记忆",
        type: "boolean" as const,
        enabled: false,
        options: [],
      },
    ];
    const socket = await open();
    socket.frame(snapshot(chatState({ permissionMode: "auto", config })));
    respond(/^PUT .*\/chat\/(permission-mode|config\/.*)$/, () =>
      Response.json({ error: "保存失败，请重试" }, { status: 409 }),
    );
    await page.getByRole("textbox", { name: "消息", exact: true }).click();
    await page.getByRole("button", { name: "会话选项", exact: true }).click();
    const sheet = page.getByRole("dialog", { name: "会话选项", exact: true });
    const permission = sheet.getByRole("combobox", {
      name: "权限",
      exact: true,
    });
    const model = sheet.getByRole("button", { name: "模型", exact: true });
    const memory = sheet.getByRole("switch", { name: "记忆", exact: true });
    await userEvent.selectOptions(permission, "ask");
    await expect
      .element(sheet.getByRole("alert"))
      .toHaveTextContent("保存失败，请重试");
    await expect.element(permission).toHaveValue("auto");
    await model.click();
    await sheet.getByRole("option", { name: "B", exact: true }).click();
    await expect
      .element(sheet.getByRole("alert"))
      .toHaveTextContent("保存失败，请重试");
    await expect.element(model).toHaveTextContent("A");
    // Refreshing the option catalog must not resurrect the rejected selection.
    socket.frame({
      type: "state",
      state: chatState({
        permissionMode: "auto",
        config: config.map((option) =>
          option.id === "model"
            ? {
                ...option,
                options: [...option.options, { value: "c", name: "C" }],
              }
            : option,
        ),
      }),
    });
    await model.click();
    await expect
      .element(sheet.getByRole("option", { name: "C", exact: true }))
      .toBeInTheDocument();
    await userEvent.keyboard("{Escape}");
    await expect.element(model).toHaveTextContent("A");
    await memory.click();
    await expect
      .element(sheet.getByRole("alert"))
      .toHaveTextContent("保存失败，请重试");
    await expect.element(memory).not.toBeChecked();
    expect(commands().map(({ body }) => body)).toEqual([
      expect.objectContaining({ mode: "ask" }),
      expect.objectContaining({ value: "b" }),
      expect.objectContaining({ enabled: true }),
    ]);
    // A successful request still waits for confirmed state from the server.
    respond(
      /^PUT .*\/chat\/(permission-mode|config\/.*)$/,
      () => new Response(null, { status: 204 }),
    );
    await userEvent.selectOptions(permission, "ask");
    await expect.element(permission).toBeEnabled();
    await expect.element(permission).toHaveValue("auto");
    await expect.element(sheet.getByRole("alert")).not.toBeInTheDocument();
    socket.frame({
      type: "state",
      state: chatState({
        permissionMode: "ask",
        config: config.map((option) => ({
          ...option,
          value: "b",
          enabled: true,
        })),
      }),
    });
    await expect.element(permission).toHaveValue("ask");
    await expect.element(model).toHaveTextContent("B");
    await expect.element(memory).toBeChecked();
  } finally {
    await page.viewport(1024, 900);
  }
});

test("a short options sheet scrolls long configuration lists and keeps close reachable", async () => {
  await page.viewport(320, 320);
  try {
    const socket = await open();
    socket.frame(
      snapshot(
        chatState({
          config: [
            ...Array.from({ length: 10 }, (_, i) => ({
              id: `option-${i}`,
              name: `配置项 ${i} with a long unbroken label`,
              type: "select" as const,
              value: "long-model",
              options: [
                {
                  value: "long-model",
                  name: "provider/a-very-long-model-name-with-version",
                },
              ],
            })),
            {
              id: "memory",
              name: "记忆",
              type: "boolean",
              enabled: false,
              options: [],
            },
          ],
        }),
      ),
    );
    await page.getByRole("textbox", { name: "消息", exact: true }).click();
    await page.getByRole("button", { name: "会话选项", exact: true }).click();
    const sheet = page.getByRole("dialog", { name: "会话选项", exact: true });
    const close = sheet.getByRole("button", { name: "关闭选项", exact: true });
    const memory = sheet.getByRole("switch", { name: "记忆", exact: true });
    await memory.click();
    await expect.poll(commands).toEqual([
      {
        method: "PUT",
        path: `${base}/config/memory`,
        body: { clientRequestId: expect.stringMatching(/\S/), enabled: true },
      },
    ]);
    const body = memory.element().closest(".overflow-y-auto")!;
    expect(body.scrollTop).toBeGreaterThan(0);
    expect(body.scrollHeight).toBeGreaterThan(body.clientHeight);
    const bounds = close.element().getBoundingClientRect();
    expect(bounds.top).toBeGreaterThanOrEqual(0);
    expect(bounds.bottom).toBeLessThanOrEqual(320);
    expect(document.documentElement.scrollWidth).toBeLessThanOrEqual(320);
    await close.click();
    await expect.element(sheet).not.toBeInTheDocument();
  } finally {
    await page.viewport(1024, 900);
  }
});

async function composerButtonSizes() {
  const socket = await open();
  socket.frame(
    snapshot(chatState({ steering: true, turn: { running: true } })),
  );
  await page.getByRole("textbox", { name: "消息" }).fill("下一步");
  const names = ["上传文件", "快捷指令", "加入排队"];
  const sizes: Record<string, string> = {};
  for (const name of names) {
    const button = page.getByRole("button", { name });
    await expect.element(button).toBeVisible();
    const box = button.element().getBoundingClientRect();
    sizes[name] = `${box.width}x${box.height}`;
  }
  return sizes;
}

test("composer toolbar buttons share one box size on desktop", async () => {
  const sizes = await composerButtonSizes();
  expect(new Set(Object.values(sizes)).size, JSON.stringify(sizes)).toBe(1);
});

test("composer toolbar buttons share one box size on a phone", async () => {
  await page.viewport(390, 844);
  try {
    const sizes = await composerButtonSizes();
    expect(new Set(Object.values(sizes)).size, JSON.stringify(sizes)).toBe(1);
  } finally {
    await page.viewport(1024, 900);
  }
});

test("the usage ring opens the exact token counts", async () => {
  const socket = await open();
  socket.frame(
    snapshot(
      chatState({
        usage: { used: 12_499, size: 1_000_000, cost: 0.5, currency: "USD" },
      }),
    ),
  );
  await page
    .getByRole("button", { name: "上下文已用 1%（12.5k / 1000.0k） · $0.50" })
    .click();
  const details = page.getByRole("dialog", { name: "Token 使用情况" });
  await expect.element(details).toBeVisible();
  const row = (name: string) =>
    details.getByText(name, { exact: true }).element().nextElementSibling
      ?.textContent;
  expect(row("已用")).toBe(`${(12_499).toLocaleString("zh-CN")} tokens`);
  expect(row("剩余")).toBe(`${(987_501).toLocaleString("zh-CN")} tokens`);
  expect(row("本会话费用")).toBe("$0.50");
  await expect
    .element(details.getByRole("progressbar", { name: "上下文已用" }))
    .toHaveAttribute("aria-valuenow", "1");
});

test("the jump-to-latest button stays above a tall composer area", async () => {
  const socket = await open();
  socket.frame(snapshot(chatState()));
  socket.frame({
    type: "events",
    generation: "g1",
    items: events(
      ...Array.from({ length: 40 }, (_, i) => ({
        kind: "message.append" as const,
        message: {
          messageId: `a${i}`,
          role: "agent" as const,
          text: `第 ${i} 段回复`,
        },
      })),
    ),
  });
  // A queue makes the area under the conversation taller than the composer.
  socket.frame({
    type: "state",
    state: chatState({
      turn: { running: true },
      queue: {
        paused: false,
        items: ["一", "二", "三"].map((text, i) => ({
          id: `q${i}`,
          text,
          createdAt: "2026-10-28T00:00:00Z",
        })),
      },
    }),
  });
  const conversation = page.getByRole("list", { name: "对话" });
  await expect.element(conversation).toMatchTextContent(/第 39 段回复/);
  const scroller = conversation
    .element()
    .closest("[data-slot=scroll-area-viewport]")!;
  scroller.scrollTop = 0;
  scroller.dispatchEvent(new Event("scroll"));
  const jump = page.getByRole("button", { name: "回到最新" });
  await expect.element(jump).toBeVisible();
  const button = jump.element().getBoundingClientRect();
  const area = scroller.getBoundingClientRect();
  const queue = page
    .getByRole("region", { name: "排队的消息" })
    .element()
    .getBoundingClientRect();
  expect(button.bottom).toBeLessThanOrEqual(area.bottom);
  expect(button.bottom).toBeLessThanOrEqual(queue.top);
});

test("a form request highlights the prompt in destructive red", async () => {
  const socket = await open();
  const formRequest = {
    requestId: "form-1",
    kind: "form" as const,
    title: "确认发布参数",
    form: {
      message: "请填写目标环境",
      fields: [
        { name: "env", title: "环境", type: "string" as const, required: true },
      ],
      supported: true,
    },
  };
  socket.frame(
    snapshot(chatState({ turn: { running: true }, requests: [formRequest] })),
  );
  const form = page.getByRole("form", { name: "Agent 的表单" });
  await expect.element(form).toMatchTextContent(/确认发布参数/);
  expect(form.element().classList.contains("border-destructive/40")).toBe(true);
  expect(form.element().querySelector("svg.text-destructive")).not.toBeNull();
});

test("submits multi-select array fields in a form elicitation", async () => {
  const socket = await open();
  const formRequest = {
    requestId: "form-multi",
    kind: "form" as const,
    form: {
      message: "The agent has 2 questions.",
      fields: [
        {
          name: "q1",
          title: "Pick features",
          type: "array" as const,
          required: true,
          enum: ["auth", "cache", "logs"],
          enumNames: ["Authentication", "Caching", "Logging"],
          default: ["cache"],
        },
        {
          name: "q1_other",
          title: "Other",
          type: "string" as const,
          required: false,
        },
      ],
      supported: true,
    },
  };
  socket.frame(
    snapshot(chatState({ turn: { running: true }, requests: [formRequest] })),
  );
  const form = page.getByRole("form", { name: "Agent 的表单" });
  await expect.element(form).toMatchTextContent(/The agent has 2 questions\./);
  const submit = form.getByRole("button", { name: "提交" });
  await expect.element(submit).toBeEnabled();

  await form.getByRole("checkbox", { name: "Caching" }).click();
  await expect.element(submit).toBeDisabled();

  await form.getByRole("checkbox", { name: "Logging" }).click();
  await form.getByRole("checkbox", { name: "Authentication" }).click();
  await userEvent.fill(form.getByRole("textbox", { name: "Other" }), "metrics");

  await submit.click();
  await expect.poll(commands).toEqual([
    {
      method: "POST",
      path: `${base}/requests/form-multi`,
      body: {
        clientRequestId: expect.stringMatching(/\S/),
        outcome: "accept",
        content: { q1: ["auth", "logs"], q1_other: "metrics" },
      },
    },
  ]);
});

test("pins the plan beside the transcript on wide panes and above the input otherwise", async () => {
  const socket = await open();
  const entries = Array.from({ length: 40 }, (_, i) => ({
    content:
      i === 0 ? "定位排序函数" : i === 1 ? "修改实现" : `验证步骤 ${i + 1}`,
    priority: "medium" as const,
    status:
      i === 0
        ? ("completed" as const)
        : i === 1
          ? ("in_progress" as const)
          : ("pending" as const),
  }));
  socket.frame(snapshot(chatState({ plan: { entries } })));
  const dock = page.getByRole("button", { name: /计划 1\/40/ });
  await expect.element(dock).toBeVisible();
  const panel = page.getByRole("region", { name: "计划", includeHidden: true });
  await expect.element(panel).not.toBeVisible();

  await page.viewport(1400, 500);
  try {
    await expect.element(panel).toBeVisible();
    await expect.element(panel.getByText("修改实现")).toBeVisible();
    await expect
      .element(
        page.getByRole("button", { name: /计划 1\/40/, includeHidden: true }),
      )
      .not.toBeVisible();
    const list = panel.element().querySelector("ol")?.parentElement;
    expect(list).not.toBeNull();
    expect(list!.scrollHeight).toBeGreaterThan(list!.clientHeight);
  } finally {
    await page.viewport(1024, 768);
  }
});

test("document scroll keeps wide conversation ticks in the viewport and preserves history reading", async () => {
  await page.viewport(900, 900);
  try {
    const socket = await open(undefined, true);
    socket.frame(snapshot(chatState()));
    socket.frame({
      type: "events",
      generation: "g1",
      items: events(
        ...Array.from({ length: 20 }, (_, index) => ({
          kind: "message.append" as const,
          message: {
            messageId: `doc-nav-${index}`,
            role: "agent" as const,
            text: Array.from(
              { length: 8 },
              (_, line) => `第 ${index + 1} 条回复，段落 ${line + 1}`,
            ).join("\n\n"),
          },
        })),
      ),
    });
    const navigation = page.getByRole("navigation", { name: "对话轮次" });
    await expect.element(navigation).toBeVisible();
    expect(getComputedStyle(navigation.element()).position).toBe("fixed");
    const bounds = navigation.element().getBoundingClientRect();
    expect(bounds.top).toBeGreaterThanOrEqual(0);
    expect(bounds.bottom).toBeLessThanOrEqual(window.innerHeight);
    await expect.poll(() => window.scrollY).toBeGreaterThan(0);

    const first = navigation.getByRole("button", {
      name: /^此前 · AI：第 1 条回复/,
    });
    await first.click();
    await expect.element(first).toHaveAttribute("aria-current", "location");
    await expect.poll(() => window.scrollY).toBeLessThan(100);
    const readingOffset = window.scrollY;
    socket.frame({
      type: "events",
      generation: "g1",
      items: events({
        kind: "message.append",
        message: {
          messageId: "doc-nav-new",
          role: "agent",
          text: "文档滚动阅读期间的新回复",
        },
      }),
    });
    await expect
      .element(page.getByText("文档滚动阅读期间的新回复"))
      .toBeVisible();
    expect(window.scrollY).toBe(readingOffset);
  } finally {
    window.scrollTo(0, 0);
    await page.viewport(1024, 900);
  }
});

test("document scroll mode scrolls the document, pins the dock, and preserves reading offset across input focus", async () => {
  await page.viewport(390, 844);
  try {
    const socket = await open(undefined, true);
    socket.frame(snapshot(chatState()));
    socket.frame({
      type: "events",
      generation: "g1",
      items: events(
        ...Array.from({ length: 30 }, (_, index) => ({
          kind: "message.append" as const,
          message: {
            messageId: `doc-${index}`,
            role: "agent" as const,
            text: `文档滚动第 ${index} 条回复`,
          },
        })),
      ),
    });

    const transcript = page.getByRole("list", { name: "对话" });
    await expect
      .element(transcript.getByText("文档滚动第 29 条回复"))
      .toBeVisible();
    const chatView = document.querySelector("[data-chat-view]")!;
    expect(chatView.getAttribute("data-document-scroll")).toBe("true");
    expect(document.scrollingElement!.scrollHeight).toBeGreaterThan(
      window.innerHeight,
    );
    await expect.poll(() => window.scrollY).toBeGreaterThan(0);

    // Scroll up to a reading offset; the jump-to-latest button appears above the sticky dock.
    window.scrollTo(0, 120);
    window.dispatchEvent(new Event("scroll"));
    const jump = page.getByRole("button", { name: "回到最新" });
    await expect.element(jump).toBeVisible();
    const dock = document.querySelector("[data-chat-dock]")!;
    expect(getComputedStyle(dock).position).toBe("sticky");
    expect(jump.element().getBoundingClientRect().bottom).toBeLessThanOrEqual(
      dock.getBoundingClientRect().top,
    );

    // Focusing the message box falls back to container scroll and preserves the reading offset.
    const input = page.getByRole("textbox", { name: "消息" });
    const scroller = transcript
      .element()
      .closest("[data-slot=scroll-area-viewport]") as HTMLElement;
    await input.click();
    await expect
      .poll(() => chatView.getAttribute("data-document-scroll"))
      .toBeNull();
    await expect.poll(() => Math.round(scroller.scrollTop)).toBe(120);
    expect(window.scrollY).toBe(0);

    // Blurring returns to document scroll and restores the window scroll offset.
    (input.element() as HTMLTextAreaElement).blur();
    await expect
      .poll(() => chatView.getAttribute("data-document-scroll"))
      .toBe("true");
    await expect.poll(() => Math.round(window.scrollY)).toBe(120);

    // Let the composer finish its height transition before clicking the dock.
    await new Promise((resolve) => requestAnimationFrame(resolve));
    await expect
      .poll(() =>
        document
          .querySelector("[data-composer-card]")!
          .getAnimations()
          .some((animation) => animation.playState === "running"),
      )
      .toBe(false);

    // Jump to latest scrolls the document back to the bottom.
    await jump.click();
    await expect
      .poll(
        () =>
          window.scrollY + window.innerHeight >=
          document.scrollingElement!.scrollHeight - 32,
      )
      .toBe(true);
  } finally {
    window.scrollTo(0, 0);
    await page.viewport(1024, 900);
  }
});

test("recalls sent text and slash commands without taking over the caret", async () => {
  const socket = await open();
  socket.frame(
    snapshot(
      chatState({
        commands: [{ name: "compact", description: "压缩上下文" }],
      }),
    ),
  );
  const wrapped = "检查测试 ".repeat(240).trim();
  socket.frame({
    type: "events",
    generation: "g1",
    items: events(
      {
        kind: "message.append",
        message: { messageId: "u1", role: "user", text: wrapped },
      },
      {
        kind: "message.append",
        message: { messageId: "a1", role: "agent", text: "测试通过" },
      },
      {
        kind: "message.append",
        message: { messageId: "u2", role: "user", text: "/compact" },
      },
    ),
  });
  const input = page.getByRole("textbox", { name: "消息" });
  await expect
    .element(page.getByText("/compact", { exact: false }))
    .toBeVisible();
  await input.click();
  await userEvent.keyboard("{ArrowUp}");
  await expect.element(input).toHaveValue("/compact");
  await expect
    .element(page.getByRole("listbox", { name: "命令" }))
    .not.toBeInTheDocument();
  await userEvent.keyboard("{ArrowUp}");
  await expect.element(input).toHaveValue(wrapped);
  (input.element() as HTMLTextAreaElement).setSelectionRange(200, 200);
  await userEvent.keyboard("{ArrowUp}");
  await expect.element(input).toHaveValue(wrapped);
  (input.element() as HTMLTextAreaElement).setSelectionRange(
    wrapped.length,
    wrapped.length,
  );
  await userEvent.keyboard("{ArrowDown}{ArrowDown}");
  await expect.element(input).toHaveValue("");
  await userEvent.keyboard("{ArrowUp}");
  await input.fill("未发送的草稿");
  await userEvent.keyboard("{ArrowUp}{ArrowDown}");
  await expect.element(input).toHaveValue("未发送的草稿");
});
