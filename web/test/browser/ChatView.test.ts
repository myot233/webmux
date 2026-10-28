import "../../src/styles.css";
import { expect, test } from "vitest";
import { page, userEvent } from "vitest/browser";
import { render } from "vitest-browser-svelte/pure";
import type { ChatRequest } from "../../src/schemas";
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

async function open() {
  render(Harness, { id });
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
  // Thinking is folded to one row until opened.
  const thought = transcript.getByRole("button", { name: "已思考" });
  await expect.element(thought).toHaveAttribute("aria-expanded", "false");
  await expect.element(transcript.getByText("先找排序函数")).not.toBeVisible();
  await thought.click();
  await expect.element(transcript.getByText("先找排序函数")).toBeVisible();
  // Consecutive reads and searches are one "browsed" row.
  const browsed = transcript.getByRole("button", { name: "已浏览 3 项" });
  await expect.element(browsed).toBeVisible();
  await browsed.click();
  await expect.element(transcript.getByText("sortWorkspaces")).toBeVisible();

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

test("the composer posts the prompt with a request ID and mode auto", async () => {
  respond(/^POST .*\/chat\/prompts$/, () => Response.json({ queued: false }));
  const socket = await open();
  socket.frame(snapshot(chatState()));
  const input = page.getByRole("textbox", { name: "消息" });
  await input.fill("补一个回归测试");
  await page.getByRole("button", { name: "发送", exact: true }).click();
  await expect.poll(commands).toEqual([
    {
      method: "POST",
      path: `${base}/prompts`,
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
  await expect
    .element(
      page.getByRole("button", { name: /pnpm --dir web exec vitest run/ }),
    )
    .toMatchTextContent(/等待授权/);
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

test("steering is offered only while a turn runs on an agent that takes it", async () => {
  respond(/^POST .*\/chat\/prompts$/, () => Response.json({ queued: false }));
  const socket = await open();
  const steer = page.getByRole("button", { name: "插话" });
  socket.frame(snapshot(chatState({ steering: true })));
  await page.getByRole("textbox", { name: "消息" }).fill("顺便检查空列表");
  await expect
    .element(page.getByRole("button", { name: "发送" }))
    .toBeVisible();
  expect(steer.elements()).toHaveLength(0);

  socket.frame({
    type: "state",
    state: chatState({ steering: false, turn: { running: true } }),
  });
  await expect
    .element(page.getByRole("button", { name: "停止" }))
    .toBeVisible();
  expect(steer.elements()).toHaveLength(0);

  socket.frame({
    type: "state",
    state: chatState({ steering: true, turn: { running: true } }),
  });
  await steer.click();
  await expect.poll(commands).toEqual([
    {
      method: "POST",
      path: `${base}/prompts`,
      body: {
        clientRequestId: expect.any(String),
        text: "顺便检查空列表",
        mode: "steer",
      },
    },
  ]);
});

test("typing / lists the agent's commands and filters them", async () => {
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
  await userEvent.keyboard("{Enter}");
  await expect.element(input).toHaveValue("/reset ");
  await expect.element(menu).not.toBeInTheDocument();
  expect(commands()).toEqual([]);

  await input.fill("/zz");
  await expect.element(menu).not.toBeInTheDocument();
});
