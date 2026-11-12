import { describe, expect, test } from "vitest";
import {
  connectorMissingText,
  connectorNoticeText,
  eligibilityText,
  chatSupportReason,
  sessionPageLabel,
  sessionViewLabel,
  outdatedText,
  shortId,
  switchSteps,
  switchTarget,
} from "../src/domains/sessions/hybrid";
import type { SessionInfo } from "../src/contracts/schemas";

const connector = {
  executable: "codex-acp",
  title: "Codex ACP 适配器",
  installCommand: "npm install -g @agentclientprotocol/codex-acp",
  docsUrl: "https://github.com/agentclientprotocol/codex-acp",
};

describe("hybrid labels", () => {
  test("opening a chat session does not claim to load a terminal", () => {
    const chat: Pick<SessionInfo, "views" | "view" | "transition"> = {
      views: ["terminal", "chat"],
      view: "chat",
      transition: null,
    };
    expect(sessionPageLabel(chat)).toBe("聊天");
    expect(
      sessionPageLabel({
        views: ["terminal"],
        view: "terminal",
        transition: null,
      }),
    ).toBe("终端");
    expect(sessionPageLabel(undefined)).toBe("会话");
    expect(
      sessionPageLabel({
        views: ["terminal", "chat"],
        view: "terminal",
        transition: { to: "chat", phase: "stopping" },
      }),
    ).toBe("会话");
  });

  test("a transition means switching even before the view flips", () => {
    expect(sessionViewLabel({ view: "terminal", transition: null })).toBe(
      "终端",
    );
    expect(sessionViewLabel({ view: "chat", transition: null })).toBe("聊天");
    expect(
      sessionViewLabel({
        view: "terminal",
        transition: { to: "chat", phase: "stopping" },
      }),
    ).toBe("切换中");
  });

  test("support and eligibility reasons fall back for unknown codes", () => {
    expect(chatSupportReason(null)).toBe("");
    expect(
      chatSupportReason({
        supported: true,
        supportedAgents: [],
        agent: "omp",
      }),
    ).toBe("");
    expect(chatSupportReason({ supported: false, supportedAgents: [] })).toBe(
      "未识别到支持的 Agent",
    );
    expect(
      chatSupportReason({
        supported: false,
        supportedAgents: ["A", "B"],
        reason: "unknown_agent",
      }),
    ).toBe("未识别到支持的 Agent，当前版本支持：A、B");
    expect(eligibilityText({ available: true, reason: "occupied" })).toBe("");
    expect(eligibilityText({ available: false, reason: "occupied" })).toBe(
      "该会话已在另一个会话中打开",
    );
    expect(eligibilityText({ available: false, reason: "future" })).toBe(
      "future",
    );
    expect(eligibilityText({ available: false })).toBe("暂时不能切到聊天");
  });

  test("Codex reasons name the connector and the other client", () => {
    expect(
      chatSupportReason({
        supported: false,
        supportedAgents: [],
        reason: "session_arg",
      }),
    ).toBe("该参数打开已有会话，混合配置请不要指定会话");
    expect(connectorMissingText(connector)).toBe(
      "未安装 Codex ACP 适配器（codex-acp）",
    );
    expect(connectorNoticeText(connector)).toBe("聊天端需要另外安装 codex-acp");
    expect(
      eligibilityText({
        available: false,
        reason: "connector_missing",
        connector,
      }),
    ).toBe("未安装 Codex ACP 适配器（codex-acp）");
    expect(
      eligibilityText({ available: false, reason: "occupied_external" }),
    ).toBe("该会话正被另一个 Agent 客户端使用");
    expect(
      outdatedText({ name: "codex-acp", version: "2.4.0", verified: "2.5.0" }),
    ).toBe("codex-acp 2.4.0 低于已验证版本 2.5.0，部分功能可能不可用");
  });

  test("short IDs keep short ones whole", () => {
    expect(shortId("ses_e7fb6e69effeUOvdil8kU5qkcf")).toBe("ses_e7fb…");
    expect(shortId("01a")).toBe("01a");
  });
});

describe("switch progress", () => {
  test("the direction comes from one-way phases, else the view left", () => {
    expect(switchTarget("awaiting_choice", undefined)).toBe("chat");
    expect(switchTarget("cancelling", undefined)).toBe("terminal");
    expect(switchTarget("stopping", "chat")).toBe("terminal");
    expect(switchTarget("starting", "terminal")).toBe("chat");
    expect(switchTarget("stopping", undefined)).toBeNull();
  });

  test("steps mark done, current and pending; awaiting a choice is interrupting", () => {
    expect(switchSteps("chat", "stopping")).toEqual([
      { label: "中断中", state: "done" },
      { label: "退出中", state: "current" },
      { label: "启动中", state: "pending" },
      { label: "加载历史中", state: "pending" },
    ]);
    expect(switchSteps("chat", "awaiting_choice")[0]).toEqual({
      label: "中断中",
      state: "current",
    });
    expect(switchSteps("terminal", "starting").map((s) => s.state)).toEqual([
      "done",
      "done",
      "current",
      "pending",
    ]);
  });

  test("a switch to the terminal ends by connecting it; a request starts at the first step", () => {
    expect(switchSteps("terminal", "arriving")).toEqual([
      { label: "中断中", state: "done" },
      { label: "退出中", state: "done" },
      { label: "启动中", state: "done" },
      { label: "连接终端", state: "current" },
    ]);
    expect(switchSteps("chat", "requested").map((s) => s.state)).toEqual([
      "current",
      "pending",
      "pending",
      "pending",
    ]);
    expect(switchSteps(null, "requested")).toEqual([]);
  });

  test("a rollback or unknown direction is shown on its own", () => {
    expect(switchSteps("chat", "rolling_back")).toEqual([
      { label: "切换失败，正在回到原端", state: "current" },
    ]);
    expect(switchSteps(null, "stopping")).toEqual([
      { label: "退出中", state: "current" },
    ]);
  });
});
