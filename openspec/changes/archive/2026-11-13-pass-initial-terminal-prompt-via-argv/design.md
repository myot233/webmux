## Context

在通过 `POST /api/sessions`（包括 Web 端“新聊天”页勾选在终端中打开、新建会话，以及 `webmux run --prompt`）以终端视图启动带 `prompt` 的会话时：

- `workspace.Service.createSession` 按以下步骤处理首条提示：
  1. 先创建会话（`startSession`）；
  2. 通过 `composePrompt` 拼装仓库初始化指令（含 `{{dir}}` 与 `{{webmux}}` 占位符，后者依赖已分配的会话 ID）和用户输入；
  3. 调用 `sendFirstMessage` -> `session.Manager.SendInput` -> `SendWhenReady` 等待终端输出静默或就绪信号后写入 PTY。
- 对于混合会话，TUI 的启动命令由 `agentprofile.Command` 解析并生成：
  - `TUIArgv()` 返回首次启动的 `argv`（含档案注入的常驻参数，如 OpenCode 的 `--standalone`、Codex 的 `--no-daemon`、`agy` 的 `--settings`）；
  - `ResumeArgv(sessionID)` 返回从聊天切回终端或恢复会话时的 `argv`（丢弃原命令中的提示词参数，避免重复发送）。
- 实测核查本机安装的各 CLI agent 对命令行提示词参数及特殊字符（多行文本、前导 `-`、引号等）的处理行为：
  - **Claude Code (`claude`)**：支持 `[prompt]` 位置参数，且支持 `-- <prompt>` 终止选项解析，任意多行或以 `-` 开头的文本跟在 `--` 后均安全作为交互式首条提示词。
  - **Codex (`codex`)**：支持 `[PROMPT]` 位置参数，且支持 `-- <prompt>`，多行及以 `-` 开头的文本均安全。
  - **OpenCode (`opencode`)**：
    - 位置参数是 `[project]`（工作目录），不能传位置提示词，但提供 `--prompt` 标志；
    - yargs 在解析 `--prompt <val>`（分开两个 token）且 `<val>` 为 `-v` / `--help` 时会误触发全局标志，而单 token 形式 `--prompt=<prompt>` 可安全传递包括 `-v`、`-h`、多行文本在内的任意字符串。
  - **omp (`omp`)**：支持 `[messages...]` 位置参数与 `-- <prompt>`，但 `omp` 在解析 `--` 之前会先行检查全量 `argv` 是否包含 `-h`、`--help`、`-v`、`--version` 并直接退出打印帮助/版本；其余以 `-` 开头或多行的文本跟在 `--` 后均安全。
  - **Antigravity CLI (`agy`)**：支持 `-i, --prompt-interactive <prompt>`。基于 Commander 实现，当参数值以 `-` 开头时，无论写成 `-i <val>` 还是 `--prompt-interactive=<val>` 都会被当作选项而报错；非 `-` 开头的多行与特殊字符文本使用 `--prompt-interactive=<prompt>` 均安全。
  - **Qoder / QoderCN (`qoder` / `qodercn`)**：暂未迁移，继续使用原有的 `SendWhenReady` 注入方式。

## Goals / Non-Goals

**Goals:**

- 为已核查的内置 agent 档案提供通过 `argv` 传递首次终端启动提示词的能力，消除 TUI 启动阶段的 `SendWhenReady` 时序竞态与多行回车拆分问题。
- 保持渐进式迁移与无感回退：未配置 `InitialPrompt` 的档案、自定义命令/普通 shell 会话、启动配置本身已写有提示词参数，或提示词触发特定 CLI 解析器边界限制时，自动回退到原有的 `SendWhenReady` PTY 注入。
- 保证提示词中的 `{{dir}}` 与 `{{webmux}}` 占位符在首次 TUI 进程启动前用真实的会话 ID 展开。
- 保证提示词文本不写入 `SessionInfo.Cmd`、`hybrid-sessions.json` 持久化记录或后续恢复/切换命令。

**Non-Goals:**

- 不改变聊天视图（`view: "chat"`）发送首条提示的流程（仍走 ACP `session/prompt`）。
- 不改变会话运行过程中通过 `POST /api/sessions/{id}/input` 向终端写入输入的机制。

## Decisions

### D1 `agentprofile` 的 `InitialPrompt` 钩子与冲突检测

在 `agentprofile.Profile` 上增加可选函数字段：

```go
InitialPrompt func(prompt string) ([]string, bool)
```

- 当档案未迁移（`InitialPrompt == nil`）时，`Command.InitialTUIArgv(prompt)` 直接返回 `(nil, false)`。
- 在 `classify()` 扫描启动配置 `Cmd` 时，若遇到位置提示词、`--`（及其后的位置参数）、`--prompt`、`-i` 或 `--prompt-interactive`，标记 `cmd.hasPrompt = true`。
  - 当 `cmd.hasPrompt` 为 `true` 时，说明用户在启动配置中已经硬编码了初始提示词；
  - 此时 `InitialTUIArgv` 返回 `(nil, false)`，交由回退路径处理，避免产生两个冲突的提示词参数。
- 各档案的编码策略：
  - `claude`、`codex`：`[]string{"--", prompt}, true`；
  - `opencode`：`[]string{"--prompt=" + prompt}, true`；
  - `omp`：若 `prompt` 为 `-h`、`--help`、`--version` 或 `-v` 则返回 `nil, false`，否则返回 `[]string{"--", prompt}, true`；
  - `agy`：若 `strings.HasPrefix(prompt, "-")` 则返回 `nil, false`，否则返回 `[]string{"--prompt-interactive=" + prompt}, true`；
  - `qoder`、`qodercn`：保持 `InitialPrompt: nil`。

### D2 会话 ID 分配后再展开提示词并仅用于首次 `StartRun`

仓库初始化指令中的 `{{webmux}}` 模板需要展开为包含新会话 ID 的环境说明（由 `agentguide.ExpandInitInstruction(raw, cwd, sessionID)` 生成）。如果在分配会话 ID 之前拼装 `argv`，`{{webmux}}` 将缺少会话 ID。

因此，在 `hybrid.Service.Create` 的终端启动分支中：

1. 先以不含提示词的 `cmd.TUIArgv()` 调用 `s.sessions.CreateShell(...)`，分配会话对象并取得 `info.ID`：
   - 此时 `SessionInfo.Cmd` 和持久化 `Record.Cmd` 记录的都是干净的 `cmd.TUIArgv()`；
   - 既符合内容安全规则（不把提示词持久化到 `hybrid-sessions.json` 或暴露在会话命令摘要中），也保证后续切回 TUI 时使用 `ResumeArgv`。
2. 调用传入的 `in.InitialPrompt(info.ID)` 获取展开后的完整提示词文本。若返回错误，销毁刚创建的空壳会话并在 `step: "prompt"` 报告错误。
3. 调用 `cmd.InitialTUIArgv(prompt)`：
   - 若返回 `(argv, true)`，将该一次性 `argv` 传给 `newActor` 用作首次 `shell.StartRun(runArgv)`，并将 `*in.InitialPromptSent = true`；
   - 若返回 `false`，首次 `shell.StartRun` 仍使用 `cmd.TUIArgv()`，`*in.InitialPromptSent` 保持 `false`。
4. `workspace.Service.createSession` 检查 `promptSent`：已通过 `argv` 发送时不再调用 `sendFirstMessage`；未发送时照常调用 `sendFirstMessage`（`SendWhenReady`）写入终端并回车。

## Risks / Trade-offs

- [部分 CLI 对极端前导字符的解析限制] → 通过实测识别出 `omp` 的 `-h`/`--help`/`-v`/`--version` 与 `agy` 的前导 `-`，在 `InitialPrompt` 中返回 `false` 精确回退到 `SendWhenReady`，兼顾常规多行提示的可靠性与极端输入的可用性。
- [未迁移的 agent 档案] → `InitialPrompt` 为可选字段，未设置时 `InitialTUIArgv` 返回 `false`，完全保持现有 `SendWhenReady` 行为。
