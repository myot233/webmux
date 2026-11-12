import {
  apiBuffer,
  agentGuidePrompt,
  ensureArtifactsFolder,
} from "../../platform/api";
import {
  clipboardFiles,
  copyText as writeClipboard,
} from "../../platform/clipboard";
import { browserDraftStorage } from "./draft-storage";
import { errorMessage } from "../../platform/error";
import {
  renderArtifactDir,
  renderWebmuxBrief,
  usesArtifactDir,
  usesWebmuxBrief,
} from "../settings/quick-commands";
import type { QuickCommand, SubmissionProgress } from "../../contracts/schemas";
import type {
  DraftOperation,
  TerminalDraft,
  TerminalDrafts,
  TextInsertion,
} from "./terminal-draft.svelte";
import type { HybridView } from "../sessions/hybrid";
import type { SessionInfo } from "../../contracts/schemas";
import type { TerminalController } from "./terminal-search";
import { newSubmissionId } from "./terminal-protocol";
import { hasTouchInput } from "../../platform/touch-input";
import type { Connection } from "../../platform/ws";

export interface SessionHybridView {
  readonly view: HybridView;
  readonly missing: boolean;
  readonly info: SessionInfo | undefined;
  readonly chat: { readonly connection: Connection };
}

export function createHeaderTerminalView(
  hybrid: SessionHybridView,
  t: TerminalController,
  terminalShown: () => boolean,
) {
  return {
    get info() {
      return hybrid.info;
    },
    get connection() {
      return terminalShown() ? t.connection : hybrid.chat.connection;
    },
    get exitCode() {
      return terminalShown() ? t.exitCode : null;
    },
    get restoring() {
      return t.restoring;
    },
    get historyTruncated() {
      return t.historyTruncated;
    },
    get canFind() {
      return t.canFind;
    },
    scrollToBottom: () => t.scrollToBottom(),
    clear: () => t.clear(),
    resync: () => t.resync(),
  };
}

export function deriveSendUnavailable(t: TerminalController): string {
  return t.connection === "missing"
    ? "会话不存在，无法发送"
    : t.exitCode !== null || t.connection === "exited"
      ? "会话已退出，可编辑或复制草稿"
      : !t.ready
        ? "终端未就绪，草稿不会自动发送"
        : "";
}

export function deriveQuickUnavailable(t: TerminalController): string {
  return t.exitCode !== null || t.connection === "exited"
    ? "会话已退出，无法发送"
    : !t.ready
      ? "终端未就绪，暂不能发送"
      : "";
}

export function deriveChatUploadUnavailable(hybrid: SessionHybridView): string {
  return hybrid.missing
    ? "会话不存在，无法上传文件"
    : hybrid.info?.status === "exited"
      ? "会话已退出，无法上传文件"
      : "";
}

export function createDraftReporter(
  draft: TerminalDraft,
  drafts: TerminalDrafts,
  isMounted: () => boolean,
  setError: (msg: string) => void,
  setNotice: (msg: string) => void,
) {
  return (id: string, announce: boolean) => (state: SubmissionProgress) => {
    drafts.updateSubmission(draft, id, state);
    if (!isMounted()) return;
    if (state === "written") {
      setError("");
      if (announce) setNotice("上次提交的草稿已写入终端。");
    } else if (draftMessages[state]) setError(draftMessages[state]);
  };
}

export function submitSessionDraft(
  draft: TerminalDraft,
  drafts: TerminalDrafts,
  t: TerminalController,
  resend: boolean,
  report: (
    id: string,
    announce: boolean,
  ) => (state: SubmissionProgress) => void,
  setError: (msg: string) => void,
  setNotice: (msg: string) => void,
) {
  if (!draft.enabled || !draft.text || !drafts.valid(draft)) return;
  const pending = draft.submission;
  if (pending && (!resend || pending.state !== "unknown")) return;
  const id = pending?.id ?? newSubmissionId();
  if (!pending) drafts.beginSubmission(draft, id);
  setNotice("");
  if (t.submit(id, draft.text, true, report(id, false))) setError("");
  else {
    if (!pending) drafts.releaseSubmission(draft);
    setError("终端未就绪，草稿未发送；请恢复连接后手动重试。");
  }
}

const draftStorage = browserDraftStorage();

export function savedStorageLabel(
  persisted: boolean,
  oversized: string,
): string {
  return persisted && draftStorage.enabled
    ? "已保存在本机"
    : draftStorage.enabled
      ? oversized
      : "仅当前页面，刷新后丢失";
}

const draftMessages: Partial<Record<SubmissionProgress, string>> = {
  failed: "草稿未写入终端，可修改后重新发送。",
  missing:
    "服务端没有这次提交的记录：可能未送达，或记录已过期。请检查终端后决定是否重新发送。",
  partial: "草稿只有部分写入终端，提交键未写入；请检查终端后再处理草稿。",
};

const quickMessages: Partial<Record<SubmissionProgress, string>> = {
  failed: "快捷指令未写入终端；请检查终端后重试。",
  missing: "快捷指令未送达服务端；请检查终端后重试。",
  partial: "快捷指令正文已写入，但回车未写入；请检查终端后手动提交。",
  unknown: "连接中断，快捷指令结果未知；重连后自动查询。",
};

export interface SessionQuickCommandContext {
  id: string;
  isMounted: () => boolean;
  isChatView: () => boolean;
  draft: TerminalDraft;
  drafts: TerminalDrafts;
  beginInput: () => DraftOperation;
  insertText: (text: string, operation?: DraftOperation) => TextInsertion;
  resetModifiers: () => void;
  submitTerminal: (
    id: string,
    text: string,
    enter: boolean,
    onProgress: (state: SubmissionProgress) => void,
  ) => boolean;
  getError: () => string;
  setError: (msg: string) => void;
}

// The operation binds this session's draft and target at selection. A body
// with `{{dir}}` or `{{webmux}}` first waits for the artifacts folder or
// the WebMux brief, then runs as written; the brief goes in last, so
// nothing in it is replaced.
export async function executeSessionQuickCommand(
  cmd: QuickCommand,
  ctx: SessionQuickCommandContext,
): Promise<void> {
  ctx.resetModifiers();
  const chat = ctx.isChatView();
  let text = cmd.command;
  let operation: DraftOperation | undefined;
  if (usesArtifactDir(text)) {
    operation = ctx.beginInput();
    let dir: string;
    try {
      dir = await ensureArtifactsFolder(ctx.id);
    } catch (err) {
      if (ctx.isMounted())
        ctx.setError(`临时文件夹不可用：${errorMessage(err)}`);
      return;
    }
    if (!ctx.isMounted()) return;
    text = renderArtifactDir(text, dir);
  }
  if (usesWebmuxBrief(text)) {
    operation ??= ctx.beginInput();
    let brief: string;
    try {
      brief = await agentGuidePrompt(ctx.id);
    } catch (err) {
      if (ctx.isMounted())
        ctx.setError(`WebMux 工具说明不可用：${errorMessage(err)}`);
      return;
    }
    if (!ctx.isMounted()) return;
    text = renderWebmuxBrief(text, brief);
  }
  if (!chat && cmd.enter && ctx.drafts.destination(ctx.draft) === "terminal") {
    // Text and Enter form one acknowledged submission.
    const sent = ctx.submitTerminal(newSubmissionId(), text, true, (state) => {
      if (!ctx.isMounted()) return;
      if (quickMessages[state]) ctx.setError(quickMessages[state]);
      else if (state === "written" && ctx.getError() === quickMessages.unknown)
        ctx.setError("");
    });
    ctx.setError(sent ? "" : "终端未就绪，快捷指令未发送；请恢复连接后重试。");
    return;
  }
  // The chat side only inserts: sending stays with the message box.
  const result = ctx.insertText(text, operation ?? ctx.beginInput());
  if (result === "failed")
    ctx.setError(
      chat || ctx.drafts.destination(ctx.draft, operation) === "draft"
        ? "草稿正在提交到终端，快捷指令未插入；请确认结果后重试。"
        : "终端未就绪，快捷指令未发送；请恢复连接后重试。",
    );
  else ctx.setError("");
}

export interface SessionClipboardContext {
  id: string;
  isMounted: () => boolean;
  isActiveAndVisible: () => boolean;
  isTerminalVisible: () => boolean;
  draft: TerminalDraft;
  drafts: TerminalDrafts;
  insertText: (text: string, operation?: DraftOperation) => TextInsertion;
  uploadFiles: (
    files: FileList | File[],
    operation?: DraftOperation,
  ) => Promise<void>;
  focusComposer: () => void;
  setPendingCopy: (text: string | null) => void;
  openCopyDialog: (text: string) => void;
  openPasteDialog: (operation: DraftOperation) => void;
  setError: (msg: string) => void;
}

export async function copySessionText(
  text: string,
  userGesture: boolean,
  ctx: SessionClipboardContext,
): Promise<void> {
  if (!ctx.isMounted()) return;
  // Keep only the latest background request. It must neither replace the
  // clipboard nor open a focus-trapped dialog in another pane.
  if (!ctx.isActiveAndVisible() || (!userGesture && !ctx.isTerminalVisible())) {
    ctx.setPendingCopy(text);
    return;
  }
  if (window.isSecureContext && navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(text);
      return;
    } catch {
      // Permission denied or a missing transient activation.
    }
  }
  // A toolbar click or Ctrl/Cmd+C is a gesture. The legacy execCommand path
  // only fills the gap on plain-HTTP LAN/VPN origins; a secure origin that
  // denied the Clipboard API keeps the manual text box fallback.
  if (userGesture && !window.isSecureContext && (await writeClipboard(text)))
    return;
  if (!ctx.isMounted()) return;
  if (!ctx.isActiveAndVisible() || (!userGesture && !ctx.isTerminalVisible())) {
    ctx.setPendingCopy(text);
    return;
  }
  ctx.openCopyDialog(text);
}

export async function copySessionBuffer(
  ctx: SessionClipboardContext,
): Promise<void> {
  try {
    await copySessionText(
      new TextDecoder().decode(await apiBuffer(ctx.id)),
      true,
      ctx,
    );
  } catch (err) {
    ctx.setError(errorMessage(err));
  }
}

export async function pasteFromSessionClipboard(
  ctx: SessionClipboardContext,
): Promise<void> {
  const operation = ctx.drafts.begin(ctx.draft);
  try {
    if (
      matchMedia("(pointer: fine)").matches &&
      typeof navigator.clipboard?.read === "function"
    ) {
      const items = await navigator.clipboard.read();
      if (!ctx.isMounted()) return;
      const files = await clipboardFiles(items);
      if (files.length) {
        if (ctx.isMounted()) await ctx.uploadFiles(files, operation);
        return;
      }
      const textItem = items.find((item) => item.types.includes("text/plain"));
      if (textItem) {
        const text = await (await textItem.getType("text/plain")).text();
        if (ctx.isMounted()) ctx.insertText(text, operation);
        return;
      }
    }
    const text = await navigator.clipboard.readText();
    if (ctx.isMounted()) ctx.insertText(text, operation);
  } catch {
    if (ctx.isMounted()) {
      if (ctx.drafts.destination(ctx.draft, operation) === "draft") {
        ctx.setError(
          ctx.draft.enabled
            ? "无法读取剪贴板，请在现有草稿框内手动粘贴。"
            : "无法读取剪贴板，请重新开启草稿模式后在草稿框内手动粘贴。",
        );
        if (ctx.draft.enabled && ctx.isActiveAndVisible() && !hasTouchInput())
          ctx.focusComposer();
      } else if (ctx.drafts.destination(ctx.draft, operation) !== "failed") {
        ctx.openPasteDialog(operation);
      }
    }
  }
}

export function applySessionVoiceResult(
  text: string,
  ctx: {
    isMounted: () => boolean;
    isChatView: () => boolean;
    draft: TerminalDraft;
    drafts: TerminalDrafts;
    beginInput: () => DraftOperation;
    insertText: (text: string, operation?: DraftOperation) => TextInsertion;
    setError: (msg: string) => void;
    onInserted: () => void;
  },
) {
  if (!text || !ctx.isMounted()) return;
  const chat = ctx.isChatView();
  const operation = ctx.beginInput();
  const result = ctx.insertText(text, operation);
  if (result === "failed") {
    ctx.setError(
      chat || ctx.drafts.destination(ctx.draft, operation) === "draft"
        ? "草稿正在提交到终端，语音识别内容未插入；请确认结果后重试。"
        : "终端未就绪，语音识别内容未发送；请恢复连接后重试。",
    );
  } else {
    ctx.setError("");
    ctx.onInserted();
  }
}

export function captureFindReturnFocus(): HTMLElement | null {
  const active = document.activeElement;
  return active instanceof HTMLElement &&
    (active.matches(".xterm-helper-textarea") || active.closest(".draft-pane"))
    ? active
    : null;
}

export function isEditableActiveElement(): boolean {
  const active = document.activeElement;
  return (
    active instanceof HTMLInputElement ||
    active instanceof HTMLTextAreaElement ||
    (active instanceof HTMLElement && active.isContentEditable)
  );
}

export function isVoiceDisabled(
  hybrid: SessionHybridView,
  t: TerminalController,
  draft: TerminalDraft,
): boolean {
  return hybrid.view === "chat"
    ? hybrid.missing || !!draft.submission
    : t.connection === "missing" ||
        (draft.enabled
          ? !!draft.submission
          : t.exitCode !== null || t.connection === "exited" || !t.ready);
}

export function clearFindFocus(
  clearFind: () => void,
  hasTouch: boolean,
  canFocus: () => boolean,
  returnFocus: HTMLElement | null,
  fallbackFocus: () => void,
  afterTick: (fn: () => void) => void,
) {
  clearFind();
  if (document.activeElement instanceof HTMLElement)
    document.activeElement.blur();
  if (!hasTouch)
    afterTick(() => {
      if (!canFocus()) return;
      if (returnFocus?.isConnected) returnFocus.focus();
      else fallbackFocus();
    });
}
