import "../../src/styles.css";
import { afterEach, expect, test } from "vitest";
import { page, userEvent } from "vitest/browser";
import { render } from "vitest-browser-svelte/pure";
import { nextPromptPreference } from "../../src/domains/chat/next-prompt.svelte";
import Harness from "./ChatViewHarness.svelte";
import {
  chatState,
  commands,
  installChatFixture,
  respond,
  snapshot,
  socketAt,
} from "./chat-fixture";

installChatFixture();
afterEach(() => nextPromptPreference.set(false));

const id = "nextbbbb";
const path = `/ws/s/${id}/chat`;
const text = "请修复本轮错误，并重新运行测试。";
const suggestion = () => page.getByRole("button", { name: text, exact: true });
const input = () => page.getByRole("textbox", { name: "消息", exact: true });
const running = (turnId: string) =>
  chatState({
    turn: { running: true, turnId, startedAt: "2026-11-12T12:00:00Z" },
  });
const finished = (turnId: string, stopReason: "error" | "end_turn" = "error") =>
  chatState({
    turn: {
      ...running(turnId).turn,
      running: false,
      endedAt: "2026-11-12T12:00:01Z",
      stopReason,
    },
  });

async function open() {
  nextPromptPreference.set(true);
  respond(/POST .*\/chat\/next-prompt$/, () => Response.json({ text }));
  render(Harness, { id });
  return socketAt(path);
}

test("only a live eligible completion offers an editable suggestion without taking Tab", async () => {
  const socket = await open();
  // A recently completed error in a snapshot is history, not a new turn.
  socket.frame(snapshot(finished("historical")));
  await expect.element(input()).toBeEnabled();
  await expect.element(suggestion()).not.toBeInTheDocument();
  expect(commands()).toEqual([]);

  socket.frame({ type: "state", state: running("normal") });
  await expect
    .element(page.getByRole("button", { name: "停止", exact: true }))
    .toBeVisible();
  socket.frame({ type: "state", state: finished("normal", "end_turn") });
  await expect
    .element(page.getByRole("button", { name: "发送", exact: true }))
    .toBeVisible();
  expect(commands()).toEqual([]);

  socket.frame({ type: "state", state: running("live") });
  await expect
    .element(page.getByRole("button", { name: "停止", exact: true }))
    .toBeVisible();
  socket.frame({ type: "state", state: finished("live") });
  await expect.element(suggestion()).toBeVisible();
  expect(commands()).toEqual([
    {
      method: "POST",
      path: `/api/sessions/${id}/chat/next-prompt`,
      body: { turnId: "live", endedAt: "2026-11-12T12:00:01Z" },
    },
  ]);
  await input().click();
  await userEvent.keyboard("{Tab}");
  await expect.element(input()).not.toHaveFocus();
  await expect.element(input()).toHaveValue("");
  await suggestion().click();
  await expect.element(input()).toHaveValue(text);
  expect(commands()).toHaveLength(1);
});

test("a dismissed turn stays dismissed even if its running state is replayed", async () => {
  const socket = await open();
  socket.frame(snapshot(running("dismissed")));
  await expect
    .element(page.getByRole("button", { name: "停止", exact: true }))
    .toBeVisible();
  socket.frame({ type: "state", state: finished("dismissed") });
  await expect.element(suggestion()).toBeVisible();
  await page.getByRole("button", { name: "忽略", exact: true }).click();
  await expect.element(suggestion()).not.toBeInTheDocument();
  expect(
    JSON.parse(localStorage.getItem("webmux.nextPromptDismissals") ?? "[]"),
  ).toContainEqual([id, "dismissed:2026-11-12T12:00:01Z"]);
  socket.frame({ type: "state", state: running("dismissed") });
  await expect
    .element(page.getByRole("button", { name: "停止", exact: true }))
    .toBeVisible();
  socket.frame({ type: "state", state: finished("dismissed") });
  await expect
    .element(page.getByRole("button", { name: "发送", exact: true }))
    .toBeVisible();
  await expect.element(suggestion()).not.toBeInTheDocument();
  expect(commands()).toHaveLength(1);

  socket.frame({ type: "state", state: running("after-dismissal") });
  await expect
    .element(page.getByRole("button", { name: "停止", exact: true }))
    .toBeVisible();
  socket.frame({ type: "state", state: finished("after-dismissal") });
  await expect.element(suggestion()).toBeVisible();
  expect(commands()).toHaveLength(2);
});

test("reconnecting after a missed completion does not resurface that historical turn", async () => {
  const socket = await open();
  socket.frame(snapshot(running("missed")));
  await expect
    .element(page.getByRole("button", { name: "停止", exact: true }))
    .toBeVisible();
  socket.drop(1006);
  await expect.element(input()).toBeDisabled();
  const reconnected = await socketAt(path, 1);
  // Opening the transport precedes the snapshot in a real reconnect.
  await expect.element(input()).toBeEnabled();
  reconnected.frame(snapshot(finished("missed")));
  await expect.element(input()).toBeEnabled();
  await expect.element(suggestion()).not.toBeInTheDocument();
  expect(commands()).toEqual([]);

  reconnected.frame({ type: "state", state: running("after-reconnect") });
  await expect
    .element(page.getByRole("button", { name: "停止", exact: true }))
    .toBeVisible();
  reconnected.frame({ type: "state", state: finished("after-reconnect") });
  await expect.element(suggestion()).toBeVisible();
  expect(commands()).toHaveLength(1);
});
