import { errorMessage } from "../../platform/error";
import { joinArgs, parseArgs } from "../../platform/api";
import type {
  AttentionMode,
  AttentionSettings,
  LaunchConfigSummary,
  PreviewSettings,
} from "../../contracts/schemas";

// Client-side mirror of the server limits; the server stays authoritative.
export const LAUNCH_CONFIGS_LIMIT = 100;
const LAUNCH_NAME_MAX = 64;
const LAUNCH_ARGS_MAX = 128;
export const LAUNCH_ENV_MAX = 64;
const LAUNCH_VALUE_BYTES = 8192;
export const LAUNCH_PREVIEW_LINES_MIN = 1;
export const LAUNCH_PREVIEW_LINES_MAX = 50;
export const LAUNCH_PREVIEW_SKIP_MIN = 0;
export const LAUNCH_PREVIEW_SKIP_MAX = 50;
export const LAUNCH_ATTENTION_MIN_TURN_MIN = 0;
export const LAUNCH_ATTENTION_MIN_TURN_MAX = 300;
export const LAUNCH_ATTENTION_DONE_DELAY_MIN = 0;
export const LAUNCH_ATTENTION_DONE_DELAY_MAX = 60;

export const defaultLaunchAttentionSettings: AttentionSettings = {
  mode: "all",
  programStatus: true,
  agentProtocol: true,
  notify: true,
  bell: true,
  minTurnSeconds: 0,
  doneDelaySeconds: 0,
};

export const LAUNCH_ATTENTION_MODE_OPTIONS: readonly {
  value: AttentionMode;
  label: string;
  description: string;
}[] = [
  {
    value: "all",
    label: "全部提醒",
    description: "包含任务完成、桌面通知、终端响铃以及需要介入的紧急提醒。",
  },
  {
    value: "urgent",
    label: "仅需介入",
    description:
      "仅保留提问、权限确认与失败提醒，抑制完成、普通通知与终端响铃。",
  },
  {
    value: "off",
    label: "关闭",
    description:
      "关闭此会话的一切自动提醒（显式调用 webmux notify 仍可触发）。",
  },
];
const envName = /^[A-Za-z_][A-Za-z0-9_]*$/;
const encoder = new TextEncoder();
const bytes = (value: string) => encoder.encode(value).length;

const STARTUP_SHELLS = ["bash", "zsh"] as const;
export type StartupShell = (typeof STARTUP_SHELLS)[number];
const STARTUP_PREFIXES: Record<StartupShell, readonly string[]> = {
  bash: ["bash", "-ic", 'exec -- "$@"', "webmux"],
  zsh: ["zsh", "-ic", 'exec -- "$@"', "webmux"],
};

export type EnvRow = { key: number; name: string; value: string };
export type LaunchConfigInput = {
  name: string;
  icon?: string;
  cmd: string[];
  env: Record<string, string>;
  preview: PreviewSettings | null;
  attention: AttentionSettings | null;
};
export type LaunchDraftErrors = {
  name?: string;
  command?: string;
  env?: string;
  preview?: string;
  attention?: string;
  rows: Record<number, { name?: string; value?: string }>;
};
/** Summaries as seen by the new-session sheet; values are never loaded. */
export type LaunchConfigsState =
  | { status: "loading" }
  | { status: "ready"; configs: LaunchConfigSummary[] }
  | { status: "unavailable"; error: string };

/** Restore only our exact wrapper; arbitrary user-written shell commands stay intact. */
export function launchCommandDraft(cmd: readonly string[]) {
  const startupShell = wrappedShell(cmd);
  return {
    command: joinArgs(
      startupShell ? cmd.slice(STARTUP_PREFIXES[startupShell].length) : cmd,
    ),
    startupShell,
  };
}

function wrappedShell(cmd: readonly string[]): StartupShell | null {
  for (const shell of STARTUP_SHELLS) {
    const prefix = STARTUP_PREFIXES[shell];
    if (
      cmd.length > prefix.length &&
      prefix.every((arg, index) => cmd[index] === arg)
    )
      return shell;
  }
  return null;
}

export const LAUNCH_ICON_OPTIONS = [
  { value: "terminal", label: "终端" },
  { value: "claudecode", label: "Claude Code" },
  { value: "codex", label: "Codex" },
  { value: "antigravity", label: "Antigravity CLI" },
  { value: "geminicli", label: "Gemini CLI（旧配置）" },
  { value: "cursor", label: "Cursor CLI" },
  { value: "githubcopilot", label: "GitHub Copilot CLI" },
  { value: "opencode", label: "OpenCode" },
  { value: "qwen", label: "Qwen Code" },
  { value: "kimi", label: "Kimi Code CLI" },
  { value: "codebuddy", label: "CodeBuddy Code" },
  { value: "pi", label: "Pi / omp" },
  { value: "qoder", label: "Qoder" },
] as const;

/** The one-line description of a saved launch config under its name. */
export function launchConfigSummary(config: {
  chat?: boolean;
  envNames: readonly string[];
  preview?: { lines: number; skip: number } | null;
  attention?: AttentionSettings | null;
}) {
  const attentionLabel = !config.attention
    ? ""
    : config.attention.mode === "urgent"
      ? "仅需介入"
      : config.attention.mode === "off"
        ? "提醒已关闭"
        : "自定义提醒";
  return [
    config.chat ? "混合会话" : "终端",
    config.envNames.length ? `${config.envNames.length} 个环境变量` : "",
    config.preview ? `预览 ${config.preview.lines} 行` : "",
    attentionLabel,
  ]
    .filter(Boolean)
    .join(" · ");
}

/** Old configs get a display default without rewriting their saved commands. */
export function launchConfigIcon(config: {
  icon?: string;
  cmd: readonly string[];
}) {
  if (config.icon) return config.icon;
  const shell = wrappedShell(config.cmd);
  const executable = config.cmd[shell ? STARTUP_PREFIXES[shell].length : 0]
    ?.split(/[\\/]/)
    .pop()
    ?.toLowerCase()
    .replace(/\.exe$/, "");
  switch (executable) {
    case "claude":
      return "claudecode";
    case "codex":
      return "codex";
    case "agy":
      return "antigravity";
    case "gemini":
      return "geminicli";
    case "agent":
    case "cursor-agent":
      return "cursor";
    case "copilot":
      return "githubcopilot";
    case "opencode":
      return "opencode";
    case "qwen":
      return "qwen";
    case "kimi":
      return "kimi";
    case "codebuddy":
      return "codebuddy";
    case "omp":
    case "pi":
      return "pi";
    case "qoder":
    case "qodercn":
    case "qoderclicn":
    case "qoder-cn":
      return "qoder";
    default:
      return "terminal";
  }
}

/** Error for one env variable name, or "" when valid. */
function envNameError(name: string): string {
  if (!name) return "请输入变量名";
  if (!envName.test(name))
    return "变量名只能包含字母、数字和下划线，且不能以数字开头";
  if (name === "WEBMUX_SESSION_ID") return "WEBMUX_SESSION_ID 由 WebMux 保留";
  return "";
}

/**
 * Validates the editor fields with the server's limits. Rows whose name and
 * value are both empty are ignored so an added-but-unused row never blocks.
 */
export function validateLaunchDraft(
  name: string,
  command: string,
  rows: readonly EnvRow[],
  startupShell: StartupShell | null = null,
  preview: PreviewSettings | null = null,
  attention: AttentionSettings | null = null,
):
  | { ok: true; value: LaunchConfigInput; errors: LaunchDraftErrors }
  | { ok: false; errors: LaunchDraftErrors } {
  const errors: LaunchDraftErrors = { rows: {} };
  if (
    preview &&
    (!Number.isInteger(preview.lines) ||
      preview.lines < LAUNCH_PREVIEW_LINES_MIN ||
      preview.lines > LAUNCH_PREVIEW_LINES_MAX)
  )
    errors.preview = `显示行数须为 ${LAUNCH_PREVIEW_LINES_MIN}–${LAUNCH_PREVIEW_LINES_MAX} 的整数`;
  else if (
    preview &&
    (!Number.isInteger(preview.skip) ||
      preview.skip < LAUNCH_PREVIEW_SKIP_MIN ||
      preview.skip > LAUNCH_PREVIEW_SKIP_MAX)
  )
    errors.preview = `跳过末尾行数须为 ${LAUNCH_PREVIEW_SKIP_MIN}–${LAUNCH_PREVIEW_SKIP_MAX} 的整数`;

  if (
    attention &&
    attention.mode !== "all" &&
    attention.mode !== "urgent" &&
    attention.mode !== "off"
  )
    errors.attention = "提醒范围无效";
  else if (
    attention &&
    (!Number.isInteger(attention.minTurnSeconds) ||
      attention.minTurnSeconds < LAUNCH_ATTENTION_MIN_TURN_MIN ||
      attention.minTurnSeconds > LAUNCH_ATTENTION_MIN_TURN_MAX)
  )
    errors.attention = `最短回合耗时须为 ${LAUNCH_ATTENTION_MIN_TURN_MIN}–${LAUNCH_ATTENTION_MIN_TURN_MAX} 秒的整数`;
  else if (
    attention &&
    (!Number.isInteger(attention.doneDelaySeconds) ||
      attention.doneDelaySeconds < LAUNCH_ATTENTION_DONE_DELAY_MIN ||
      attention.doneDelaySeconds > LAUNCH_ATTENTION_DONE_DELAY_MAX)
  )
    errors.attention = `完成确认延迟须为 ${LAUNCH_ATTENTION_DONE_DELAY_MIN}–${LAUNCH_ATTENTION_DONE_DELAY_MAX} 秒的整数`;
  const trimmed = name.trim();
  if (!trimmed) errors.name = "请输入名称";
  else if (Array.from(trimmed).length > LAUNCH_NAME_MAX)
    errors.name = `名称最多 ${LAUNCH_NAME_MAX} 个字符`;

  let cmd: string[] = [];
  try {
    cmd = parseArgs(command);
    if (!cmd.length || !cmd[0]) errors.command = "请输入可执行程序及参数";
    else {
      if (startupShell) cmd = [...STARTUP_PREFIXES[startupShell], ...cmd];
      if (cmd.length > LAUNCH_ARGS_MAX)
        errors.command = startupShell
          ? `加载 shell 启动文件后命令最多 ${LAUNCH_ARGS_MAX} 个参数（包含 shell 启动参数）`
          : `命令最多 ${LAUNCH_ARGS_MAX} 个参数`;
      else if (cmd.some((arg) => bytes(arg) > LAUNCH_VALUE_BYTES))
        errors.command = `单个参数不能超过 ${LAUNCH_VALUE_BYTES} 字节`;
    }
  } catch (err) {
    errors.command = errorMessage(err);
  }

  const env: Record<string, string> = {};
  const used = rows.filter((row) => row.name || row.value);
  if (used.length > LAUNCH_ENV_MAX)
    errors.env = `最多 ${LAUNCH_ENV_MAX} 个环境变量`;
  for (const row of used) {
    const rowErrors: { name?: string; value?: string } = {};
    const nameError = envNameError(row.name);
    if (nameError) rowErrors.name = nameError;
    else if (Object.hasOwn(env, row.name)) rowErrors.name = "变量名重复";
    if (row.value.includes("\0")) rowErrors.value = "变量值不能包含 NUL 字符";
    else if (bytes(row.value) > LAUNCH_VALUE_BYTES)
      rowErrors.value = `变量值不能超过 ${LAUNCH_VALUE_BYTES} 字节`;
    if (rowErrors.name || rowErrors.value) errors.rows[row.key] = rowErrors;
    else env[row.name] = row.value;
  }

  if (
    errors.name ||
    errors.command ||
    errors.env ||
    errors.preview ||
    errors.attention ||
    Object.keys(errors.rows).length
  )
    return { ok: false, errors };
  return {
    ok: true,
    value: { name: trimmed, cmd, env, preview, attention },
    errors,
  };
}
