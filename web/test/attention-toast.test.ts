import { expect, test } from "vitest";
import {
  attentionKey,
  buildAttentionToast,
  cleanAttentionTitle,
} from "../src/domains/notifications/attention-toast";

test("cleanAttentionTitle strips control characters, falls back to WebMux, and truncates long titles", () => {
  expect(cleanAttentionTitle("")).toBe("WebMux");
  expect(cleanAttentionTitle("  \u0007\u001b\u007f  ")).toBe("WebMux");
  expect(cleanAttentionTitle("  修复\u0007登录  ")).toBe("修复登录");
  expect(cleanAttentionTitle("汉".repeat(85))).toBe(`${"汉".repeat(80)}…`);
});

test("buildAttentionToast maps every attention kind to its label, actionable detail, and urgency", () => {
  const cases = [
    {
      kind: "question" as const,
      source: "agent" as const,
      label: "需要回答",
      detail: "Agent 正在等待你的输入",
      urgent: true,
    },
    {
      kind: "permission" as const,
      source: "agent" as const,
      label: "等待授权",
      detail: "Agent 请求执行工具操作",
      urgent: true,
    },
    {
      kind: "error" as const,
      source: "agent" as const,
      label: "出错停止",
      detail: "点击查看错误原因",
      urgent: true,
    },
    {
      kind: "done" as const,
      source: "agent" as const,
      label: "本轮完成",
      detail: "等待你的下一步",
      urgent: false,
    },
    {
      kind: "notify" as const,
      source: "terminal" as const,
      label: "有通知",
      detail: "程序发来了提醒",
      urgent: false,
    },
    {
      kind: "bell" as const,
      source: "terminal" as const,
      label: "响铃提醒",
      detail: "点击查看终端",
      urgent: false,
    },
  ];
  for (const row of cases) {
    const attention = {
      kind: row.kind,
      source: row.source,
      since: "2026-11-05T10:00:00Z",
    };
    expect(attentionKey(attention)).toBe(
      `${row.kind}\u00002026-11-05T10:00:00Z`,
    );
    expect(
      buildAttentionToast(
        { id: "sessaaaa", title: "测试会话", attention },
        attention,
      ),
    ).toEqual({
      sessionId: "sessaaaa",
      title: "测试会话",
      kind: row.kind,
      source: row.source,
      since: "2026-11-05T10:00:00Z",
      label: row.label,
      detail: row.detail,
      urgent: row.urgent,
    });
  }
});

test("buildAttentionToast shows a script's message instead of the generic detail", () => {
  const attention = {
    kind: "done" as const,
    source: "cli" as const,
    since: "2026-11-05T10:00:00Z",
    message: "  release\u0007 构建完成 ",
  };
  const toast = buildAttentionToast(
    { id: "sessaaaa", title: "构建", attention },
    attention,
  );
  expect(toast.label).toBe("本轮完成");
  expect(toast.detail).toBe("release 构建完成");
  expect(
    buildAttentionToast(
      {
        id: "sessaaaa",
        title: "构建",
        attention: { ...attention, message: "\u0007" },
      },
      { ...attention, message: "\u0007" },
    ).detail,
  ).toBe("等待你的下一步");
});
