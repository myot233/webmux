import type { SessionInfo } from "../../contracts/schemas";
import { hasUrgentAttention } from "../sessions/session-status";

type Attention = NonNullable<SessionInfo["attention"]>;
type AttentionKind = Attention["kind"];

export interface AttentionToast {
  readonly sessionId: string;
  readonly title: string;
  readonly kind: AttentionKind;
  readonly source: Attention["source"];
  readonly since: string;
  readonly label: string;
  readonly detail: string;
  readonly urgent: boolean;
}

const labels: Record<AttentionKind, string> = {
  question: "需要回答",
  permission: "等待授权",
  error: "出错停止",
  done: "本轮完成",
  notify: "有通知",
  bell: "响铃提醒",
};

const details: Record<AttentionKind, string> = {
  question: "Agent 正在等待你的输入",
  permission: "Agent 请求执行工具操作",
  error: "点击查看错误原因",
  done: "等待你的下一步",
  notify: "程序发来了提醒",
  bell: "点击查看终端",
};

export const maxVisibleToasts = 3;
export const defaultToastDurationMs = 8_000;

function stripControls(raw: string): string {
  return Array.from(raw)
    .filter((ch) => {
      const code = ch.codePointAt(0) ?? 0;
      return code >= 0x20 && code !== 0x7f && (code < 0x80 || code > 0x9f);
    })
    .join("")
    .trim();
}

export function cleanAttentionTitle(raw: string): string {
  const cleaned = stripControls(raw);
  if (!cleaned) return "WebMux";
  const chars = Array.from(cleaned);
  return chars.length > 80 ? `${chars.slice(0, 80).join("")}…` : cleaned;
}

export function attentionKey(attention: Attention): string {
  return `${attention.kind}\u0000${attention.since}`;
}

export function buildAttentionToast(
  session: Pick<SessionInfo, "id" | "title" | "attention">,
  attention: Attention,
): AttentionToast {
  return {
    sessionId: session.id,
    title: cleanAttentionTitle(session.title),
    kind: attention.kind,
    source: attention.source,
    since: attention.since,
    label: labels[attention.kind],
    detail: stripControls(attention.message ?? "") || details[attention.kind],
    urgent: hasUrgentAttention({ attention }),
  };
}
