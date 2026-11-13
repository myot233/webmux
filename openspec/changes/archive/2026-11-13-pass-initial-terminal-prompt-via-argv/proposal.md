## Why

在“新聊天”（快速开始）或 `POST /api/sessions` / `webmux run` 中选择以终端视图启动带 `prompt` 的混合会话时，现有实现先启动 TUI 进程，再用 `SendWhenReady` 监测 PTY 输出静默窗口（或固定超时），随后向 PTY 注入文本与回车（`\r`）。这种基于时序猜测的输入模拟存在两类固有缺陷：

1. **启动竞态与吞字**：部分 TUI 在首次渲染或认证/插件加载阶段存在短暂静默，过早注入的字节可能被终端初始化逻辑丢弃，或落在错误的输入框焦点上。
2. **多行提示被拆分为多次提交**：当提示包含换行符（尤其是开启仓库初始化指令时拼接出的 `初始化指令\n\n用户提示`）且 TUI 尚未开启 bracketed paste（`\x1b[?2004h`）时，注入的 `\n` 会被 TUI 当作多次回车逐步提交，导致首条提示被截断或拆散。

主流 CLI agent（Claude Code、Codex、OpenCode、omp、Antigravity CLI）均原生支持在命令行参数（`argv`）中直接接收交互式 TUI 的首条提示，由进程在 TUI 初始化就绪后自行开始第一轮对话。

将已核查的 agent 迁移为通过 `argv` 传递首条提示，并在尚未迁移或参数受限的场景下自动回退到 `SendWhenReady`，可以消除终端启动时的输入竞态与多行截断问题。

## What Changes

- `internal/agentprofile`：
  - `Profile` 新增可选钩子 `InitialPrompt func(prompt string) ([]string, bool)`，声明各 agent 档案如何将首条提示编码为首次 TUI 启动的附加参数。
  - `Command` 在解析启动配置 `Cmd` 时记录命令本身是否已显式包含提示词参数（位置提示词、`--`、`--prompt`、`-i`、`--prompt-interactive`），并提供 `InitialTUIArgv(prompt string) ([]string, bool)`：
    - 仅当提示非空、原命令未自带提示词且档案的 `InitialPrompt` 返回 `ok == true` 时，生成附加了首条提示参数的首次启动 `argv`；
    - 其余情况返回 `false`，交由回退路径处理。
  - 迁移已完成实测核查的五个内置档案：
    - `claude`（Claude Code）：追加 `-- <prompt>`；
    - `codex`（Codex）：追加 `-- <prompt>`；
    - `opencode`（OpenCode）：追加 `--prompt=<prompt>`；
    - `omp`：追加 `-- <prompt>`；当 `prompt` 为 `-h`、`--help`、`--version` 或 `-v` 时返回 `false` 回退到 `SendWhenReady`（因 `omp` 在 `--` 之前先行拦截这些全局标志）；
    - `agy`（Antigravity CLI）：追加 `--prompt-interactive=<prompt>`；当 `prompt` 以 `-` 开头时返回 `false` 回退到 `SendWhenReady`（因 Commander 将以 `-` 开头的值误判为未知选项）。
  - 尚未迁移的档案（`qoder`、`qodercn`）保持 `InitialPrompt == nil`，自动沿用 `SendWhenReady` 向 PTY 注入首条提示。
- `internal/hybrid`：
  - `CreateInput` 新增 `InitialPrompt func(id string) (string, error)` 与 `InitialPromptSent *bool`。
  - 从终端视图创建混合会话时：
    - 先由 `CreateShell` 分配会话 ID，再调用 `InitialPrompt(info.ID)` 生成完整提示（确保初始化指令中的 `{{dir}}` 与 `{{webmux}}` 占位符使用真实会话 ID 展开）；
    - 若 `cmd.InitialTUIArgv(prompt)` 命中，则仅将该 `argv` 传给首次 `shell.StartRun(argv)`；
    - 会话元数据 `SessionInfo.Cmd` 与持久化记录 `Record.Cmd` 保持为不含提示文本的基础命令。
- `internal/workspace`：
  - `createSession` 在终端视图启动且带 `prompt` 时，向 `startSession` 传入 `composePrompt` 回调；当 `promptSent` 为 `true` 时跳过 `sendFirstMessage`（`SendWhenReady`），否则回退到 `sendFirstMessage`。

## Capabilities

### Modified Capabilities

- `hybrid-agent-profiles`：定义档案可选的首条终端提示参数生成规则、已迁移档案的参数格式、边界输入回退条件，以及会话元数据与持久化记录不保存提示词文本的约束。
- `hybrid-sessions`：从终端视图创建带首条提示的混合会话时，优先通过首次 TUI 启动参数传递展开后的完整提示，不满足条件时自动回退到 `SendWhenReady` 终端输入提交。

## Impact

- 后端：`internal/agentprofile`（及 `agy`、`claudecode`、`codex`、`omp`、`opencode` 子包）、`internal/hybrid`、`internal/workspace`。
- 测试：`internal/acp/testdata/fakeagent`、`internal/agentprofile/agents/agents_test.go`、`internal/workspace/create_session_test.go`。
- 文档与用户故事：`docs/usage/sessions.md`、US-B01（`docs/user-stories/access-and-sessions.md`）、US-Q18（`docs/user-stories/hybrid-sessions.md`）。
- 对外 HTTP / WebSocket 契约与持久化 schema 无破坏性变更。
