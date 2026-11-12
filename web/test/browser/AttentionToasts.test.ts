import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { page } from "vitest/browser";
import { cleanup, render } from "vitest-browser-svelte/pure";
import {
  attentionToasts,
  dismissAttentionToast,
  observeAttentionRemoved,
  observeAttentionSession,
  observeAttentionSnapshot,
  pauseAttentionToast,
  resetAttentionToasts,
  resumeAttentionToast,
  setToastNotificationsEnabled,
} from "../../src/domains/notifications/attention-toast.svelte";
import AttentionToasts from "../../src/components/notifications/AttentionToasts.svelte";
import type { SessionInfo } from "../../src/contracts/schemas";

function makeSession(overrides: Partial<SessionInfo> = {}): SessionInfo {
  return {
    id: "sessaaaa",
    title: "修复登录逻辑",
    cwd: "/workspace/webmux",
    reportedCwd: null,
    configId: "",
    createdAt: "2026-11-05T10:00:00Z",
    lastActiveAt: "2026-11-05T10:01:00Z",
    status: "running",
    exitCode: null,
    attention: null,
    clients: 1,
    preview: "",
    ...overrides,
  };
}

beforeEach(() => {
  localStorage.removeItem("webmux.toastNotifications");
  resetAttentionToasts();
  history.replaceState(null, "", "/");
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  localStorage.removeItem("webmux.toastNotifications");
  resetAttentionToasts();
  history.replaceState(null, "", "/");
});

test("first snapshot establishes baseline without emitting toasts, and subsequent attention emits a toast", async () => {
  render(AttentionToasts);
  observeAttentionSnapshot([
    makeSession({
      id: "sessaaaa",
      status: "waiting",
      attention: {
        kind: "question",
        source: "agent",
        since: "2026-11-05T10:01:00Z",
      },
    }),
  ]);
  expect(attentionToasts.list).toEqual([]);
  expect(page.getByRole("region", { name: "会话通知" }).elements()).toEqual([]);

  const waiting = makeSession({
    id: "sessbbbb",
    title: "构建发布脚本",
    status: "waiting",
    attention: {
      kind: "permission",
      source: "agent",
      since: "2026-11-05T10:02:00Z",
    },
  });
  observeAttentionSession(waiting);
  const region = page.getByRole("region", { name: "会话通知" });
  await expect.element(region).toBeVisible();
  await expect
    .element(region.getByText("等待授权", { exact: true }))
    .toBeVisible();
  await expect
    .element(region.getByText("构建发布脚本", { exact: true }))
    .toBeVisible();
  await expect
    .element(region.getByText("Agent 请求执行工具操作", { exact: true }))
    .toBeVisible();

  // Dismissing via the close button does not navigate and prevents duplicate spinner re-emission.
  await region
    .getByRole("button", { name: "关闭 构建发布脚本 的通知" })
    .click();
  expect(location.pathname).toBe("/");
  expect(attentionToasts.list).toEqual([]);
  observeAttentionSession({ ...waiting, preview: "| spinner" });
  expect(attentionToasts.list).toEqual([]);
});

test("upgrading attention replaces the session's toast and clears on resolution or removal", () => {
  observeAttentionSnapshot([
    makeSession({ id: "sessaaaa" }),
    makeSession({ id: "sessbbbb" }),
  ]);
  observeAttentionSession(
    makeSession({
      id: "sessaaaa",
      status: "waiting",
      attention: {
        kind: "done",
        source: "agent",
        since: "2026-11-05T10:02:00Z",
      },
    }),
  );
  expect(attentionToasts.list[0]).toMatchObject({
    sessionId: "sessaaaa",
    kind: "done",
    label: "本轮完成",
    detail: "等待你的下一步",
    urgent: false,
  });

  observeAttentionSession(
    makeSession({
      id: "sessaaaa",
      status: "waiting",
      attention: {
        kind: "error",
        source: "agent",
        since: "2026-11-05T10:03:00Z",
      },
    }),
  );
  expect(attentionToasts.list).toHaveLength(1);
  expect(attentionToasts.list[0]).toMatchObject({
    sessionId: "sessaaaa",
    kind: "error",
    label: "出错停止",
    detail: "点击查看错误原因",
    urgent: true,
  });

  // Clearing attention on the session automatically removes its toast.
  observeAttentionSession(
    makeSession({
      id: "sessaaaa",
      status: "running",
      attention: null,
    }),
  );
  expect(attentionToasts.list).toEqual([]);

  // Reconnect snapshot emits alerts raised while disconnected and removes vanished sessions.
  observeAttentionSnapshot([
    makeSession({
      id: "sessbbbb",
      title: "数据库迁移",
      status: "waiting",
      attention: {
        kind: "question",
        source: "agent",
        since: "2026-11-05T10:04:00Z",
      },
    }),
  ]);
  expect(attentionToasts.list.map((t) => t.sessionId)).toEqual(["sessbbbb"]);
  observeAttentionRemoved("sessbbbb");
  expect(attentionToasts.list).toEqual([]);
});

test("toasts auto-dismiss after timeout, pause on hover/focus, and respect the device toggle", () => {
  vi.useFakeTimers();
  resetAttentionToasts(3_000);
  observeAttentionSnapshot([]);
  observeAttentionSession(
    makeSession({
      id: "sessaaaa",
      status: "waiting",
      attention: {
        kind: "bell",
        source: "terminal",
        since: "2026-11-05T10:05:00Z",
      },
    }),
  );
  expect(attentionToasts.list).toHaveLength(1);

  vi.advanceTimersByTime(2_000);
  pauseAttentionToast("sessaaaa");
  vi.advanceTimersByTime(5_000);
  expect(attentionToasts.list).toHaveLength(1);

  resumeAttentionToast("sessaaaa");
  vi.advanceTimersByTime(3_000);
  expect(attentionToasts.list).toEqual([]);

  observeAttentionSession(
    makeSession({
      id: "sessbbbb",
      status: "waiting",
      attention: {
        kind: "question",
        source: "agent",
        since: "2026-11-05T10:06:00Z",
      },
    }),
  );
  expect(attentionToasts.list).toHaveLength(1);
  setToastNotificationsEnabled(false);
  expect(attentionToasts.enabled).toBe(false);
  expect(attentionToasts.list).toEqual([]);

  observeAttentionSession(
    makeSession({
      id: "sesscccc",
      status: "waiting",
      attention: {
        kind: "permission",
        source: "agent",
        since: "2026-11-05T10:07:00Z",
      },
    }),
  );
  expect(attentionToasts.list).toEqual([]);
  dismissAttentionToast("sesscccc");
});
