## ADDED Requirements

### Requirement: 首次终端启动的首条提示参数

内置 agent 档案 SHALL 支持声明可选的首条终端提示编码规则（`InitialPrompt`），用于在从终端视图创建混合会话且携带非空首条提示时，将提示词作为命令行参数追加到首次 TUI 启动的 `argv` 中。

系统 SHALL 仅在同时满足以下条件时生成带首条提示的首次 TUI 启动 `argv`：

1. 提示词文本非空；
2. 启动配置的原 `argv` 未包含提示词参数（位置提示词、`--`、`--prompt`、`-i` 或 `--prompt-interactive`）；
3. 档案声明了首条终端提示编码规则，且当前提示词文本满足该档案的参数解析边界约束。

已支持首条终端提示参数的内置档案 SHALL 按以下方式编码：

- Claude Code（`claude`）：追加 `--` 与 `<prompt>`；
- Codex（`codex`）：追加 `--` 与 `<prompt>`；
- OpenCode（`opencode`）：追加单参数 `--prompt=<prompt>`；
- omp（`omp`）：当 `<prompt>` 为 `-h`、`--help`、`-v` 或 `--version` 时 SHALL NOT 通过 `argv` 传递（回退到终端输入提交）；其余情况追加 `--` 与 `<prompt>`；
- Antigravity CLI（`agy`）：当 `<prompt>` 以 `-` 开头时 SHALL NOT 通过 `argv` 传递（回退到终端输入提交）；其余情况追加单参数 `--prompt-interactive=<prompt>`。

未声明该规则的档案（如 `qoder`、`qodercn`）或不满足上述任一条件时，系统 SHALL 使用不含提示词的 `TUIArgv` 启动 TUI，并回退到终端输入提交流程发送首条提示。

首条终端提示参数 SHALL 仅用于首次 TUI 进程的启动，SHALL NOT 写入会话信息的 `cmd` 字段、混合会话持久化记录的 `cmd` 字段，或后续从聊天切回终端与恢复会话时生成的恢复命令。

#### Scenario: 已迁移档案生成带首条提示的首次启动命令

- **WHEN** 启动配置为 `["bash","-ic","exec -- \"$@\"","webmux","claude","--dangerously-skip-permissions"]`，从终端视图创建会话并传入首条提示 `"先读 AGENTS.md\n\n修复 bug"`
- **THEN** 首次 TUI 进程执行的 `argv` 末尾追加 `"--"` 与 `"先读 AGENTS.md\n\n修复 bug"`，而会话信息的 `cmd` 与混合会话记录的 `cmd` 仍为不含提示词的原命令

#### Scenario: 原启动配置已包含提示词参数时不重复追加

- **WHEN** 启动配置为 `["opencode","--prompt","已有提示"]`，从终端视图创建会话并传入首条提示 `"新提示"`
- **THEN** 首次 TUI 启动命令不追加第二个 `--prompt`，新提示回退到终端输入提交流程

#### Scenario: agy 遇到前导连字符提示词时回退

- **WHEN** 启动配置为 `["agy","--dangerously-skip-permissions"]`，从终端视图创建会话并传入首条提示 `"- check status"`
- **THEN** 首次 TUI 启动命令不追加 `--prompt-interactive`，首条提示回退到终端输入提交流程
