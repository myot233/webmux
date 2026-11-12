# hybrid-agent-specifics Specification

## Purpose

各内置 agent（Claude Code、Codex、Antigravity CLI）在混合会话中的专有约定：ACP 环境与认证、聊天端配置推导、命令与回合能力、取消与进程边界。通用的识别、启动、恢复与校验规则见 `hybrid-agent-profiles`。

## Requirements

### Requirement: Claude Code 的 ACP 环境与认证

Claude Code 的 ACP 进程（含资格探测）SHALL 复用会话环境，并在配置未设置 `CLAUDE_CODE_EXECUTABLE` 时注入 TUI 实际使用的 `claude` 绝对路径，使两端运行同一个 Claude Code。配置已设置同名变量时 SHALL 使用配置值。

ACP 进程 SHALL 使用与 TUI 相同的服务用户、`HOME` 与 `CLAUDE_CONFIG_DIR`，复用其中已有的订阅登录、API key、OAuth token 或第三方 provider。系统 SHALL NOT 为聊天端要求一套独立凭据，SHALL NOT 在记录、负载或日志中保存凭据值，也 SHALL NOT 自动执行登录或注销。

#### Scenario: 两端使用同一个 Claude Code

- **WHEN** 会话 PATH 中 `claude` 解析为 `/home/u/.local/bin/claude`，配置未设置 `CLAUDE_CODE_EXECUTABLE`
- **THEN** `claude-agent-acp` 的环境中 `CLAUDE_CODE_EXECUTABLE=/home/u/.local/bin/claude`

#### Scenario: 复用现有订阅登录

- **WHEN** 服务用户已在同一配置目录登录 Claude 订阅，未设置额外 ACP 凭据
- **THEN** 聊天端以该身份创建和恢复会话，任何凭据内容都不进入 WebMux 记录或日志

### Requirement: Claude Code 的聊天端配置推导

Claude Code 档案 SHALL 从保留的 argv 推导首次聊天端配置：

| argv | 配置项取值 |
|---|---|
| `--model <m>` | `model = <m>` |
| `--effort <e>` | `effort = <e>` |
| `--permission-mode default`/`manual` | `mode = default` |
| `--permission-mode acceptEdits`/`plan`/`auto`/`bypassPermissions` | `mode` 为同名值 |
| `--dangerously-skip-permissions` | `mode = bypassPermissions`，优先于其他权限参数 |

Claude Code 2.1.316 的 `--permission-mode` 接受 `acceptEdits`、`auto`、`bypassPermissions`、`default`（`manual` 为别名）、`dontAsk` 与 `plan`。

`--permission-mode dontAsk` 与未知取值 SHALL NOT 静默映射为其他模式，SHALL 在聊天端列为不生效参数。

每次冷 `session/load` 或 `session/resume` 后，系统 SHALL 以适配器发布的实际配置为准。本次实测基线下，`model` 与 `effort` 均回到环境或设置默认值，档案 SHALL 在两种恢复方法后重新应用记录值；`mode` 会从 transcript 恢复，SHALL NOT 用过期 argv 覆盖适配器实际模式。

适配器因运行条件不支持而把 `auto` 或 `bypassPermissions` 回退时，界面 SHALL 显示实际生效模式与适配器通知，SHALL NOT 继续显示请求值已生效。

#### Scenario: 预置配置的权限模式

- **WHEN** 预置 Claude Code 混合配置从终端开始并切到聊天
- **THEN** 首次聊天端请求 `mode=bypassPermissions`；若适配器允许该模式，界面显示其已生效

#### Scenario: Auto 被适配器回退

- **WHEN** argv 请求 `--permission-mode auto`，当前模型不支持 Auto
- **THEN** 界面显示适配器实际回退的 `acceptEdits` 与相应通知，而不是仍显示 `auto`

#### Scenario: 当前 provider 接受 Auto 与 Bypass

- **WHEN** 本次实测的第三方 provider 会话依次请求 `mode=auto` 与 `mode=bypassPermissions`
- **THEN** 适配器分别发布实际值 `auto` 与 `bypassPermissions`，不产生 Auto 回退通知

#### Scenario: 冷恢复后重应用模型与 Effort

- **WHEN** 原会话请求 `model=sonnet`、`effort=high`、`mode=plan`，连接器冷启动后执行 `session/load` 或 `session/resume`
- **THEN** 适配器先发布其恢复值，WebMux 重应用 `model=sonnet` 与 `effort=high`，并保留适配器恢复的 `mode=plan`

### Requirement: Claude Code 的聊天端命令与回合能力

Claude Code 档案 SHALL 声明 `session/resume` 可用于崩溃重连，SHALL 使用 `_session/steering` 和 `_meta.steering.idleBehavior=promptRequired` 处理插话竞态。本次实测中，空闲 steering 返回 `{"outcome":"promptRequired","reason":"noRunningTurn"}`，不得启动新回合。

图片与嵌入资源 SHALL 按握手中的 prompt 能力沿用现有附件管线。0.99.1 对二者均声明支持，并已在同一真实 prompt 中完成 `end_turn`。不支持的能力 SHALL 按现有规则降级，SHALL NOT 假定所有 MIME 类型均可发送。

连接器 0.99.1 发布的命令菜单已经不包含 `login` 与 `logout`。档案 SHALL 继续声明隐藏这两项作为兼容保护，WebMux SHALL NOT 通过其他入口在聊天端发起认证流程或注销服务用户的全局 Claude 登录。

Claude TUI 启动后查询 OSC 7501 能力，收到 WebMux 应答后发布 `state=idle:app=claude-code`。启动阶段不产生假 `working`，档案的 `StartupQuiet` SHALL 为 0；prompt 使用 `working → done`，退出使用 `clear`。

每一次 TUI 启动 SHALL 使用一次 Esc 作为中断键。本次实测一次 Esc 使进行中的回合 `working → idle`，进程继续运行。

#### Scenario: 空闲时插话

- **WHEN** 用户发送插话时 Claude 回合恰好结束
- **THEN** 适配器返回 `promptRequired`，WebMux 按现有队列规则处理，不产生未跟踪的独立回合

#### Scenario: 认证命令不出现在菜单

- **WHEN** 用户在 Claude Code 混合会话的聊天视图中输入 `/`
- **THEN** 命令菜单中没有 `/login` 或 `/logout`

### Requirement: Claude Code 的取消与后台进程边界

`session/cancel` SHALL 只视为当前 ACP 回合的中断，SHALL NOT 描述为整个进程树的清理。档案的强制切换说明 SHALL 明确：已经启动的后台 Bash 或子任务可能在 cancel 后继续运行；强制切换必须终止 ACP 连接器并等待其受管后代进程退出，不能仅等待 prompt 返回 `cancelled`。

本次实测中，后台 `sleep` 在 cancel 返回后一秒仍存活；终止连接器后，受测 Claude、shell 与后台进程全部退出。实现 SHALL 保留进程组清理与测试，不得把这一结果推广为对任意自行 daemonize 进程的保证。

#### Scenario: cancel 留下后台进程

- **WHEN** Claude 在真实 ACP 回合中启动 `sleep 60`，客户端随后发送 `session/cancel`
- **THEN** prompt 返回 `stopReason=cancelled`，后台进程可能仍存活，界面按档案说明提示

#### Scenario: 强制切换清理受管进程

- **WHEN** 用户确认强制切换，且后台任务仍属于 ACP 连接器的受管进程树
- **THEN** 系统终止连接器并等待受管后代退出后才启动另一端，不把 cancel 当作清理完成

### Requirement: Codex 的 TUI 不使用共享 daemon

混合会话中 Codex 的每一次 TUI 启动（包括首次启动与恢复）SHALL 带上 `--no-daemon`。
Codex 默认把回合交给共享的 app-server daemon 执行，daemon 位于 webmux 的进程组之外，结束 TUI 不会停止回合。

- 原 argv 已包含该参数时 SHALL NOT 重复添加。
- 首次启动时追加的参数 SHALL 在会话信息的命令中可见。

#### Scenario: 结束 TUI 不残留进程

- **WHEN** Codex 混合会话从终端开始并发出一条消息，用户切到聊天
- **THEN** TUI 以 `codex --no-daemon …` 运行；切换结束后，会话的进程组中没有残留的 codex 或 app-server 进程

### Requirement: Codex 的 ACP 环境

Codex 的 ACP 进程（含资格探测）SHALL 在“两端环境一致”的环境之上额外注入：

- `CODEX_PATH`：TUI 实际使用的 `codex` 可执行文件，按与连接器检查相同的写法与环境解析；找不到时 SHALL NOT 注入，
  聊天视图 SHALL 提示“未找到 codex，聊天端使用适配器自带的 Codex”；
- `NO_BROWSER=1`：聊天端不执行浏览器登录。

启动配置已设置同名变量时，SHALL 使用配置中的值，SHALL NOT 覆盖。

#### Scenario: 两端使用同一个 codex

- **WHEN** 服务 PATH 中 `codex` 解析为 `/usr/local/bin/codex`，配置未设置 `CODEX_PATH`
- **THEN** `codex-acp` 进程的环境中 `CODEX_PATH=/usr/local/bin/codex`、`NO_BROWSER=1`

### Requirement: Codex 的聊天端配置推导

Codex 在每次 `session/load` 后都会把模型、推理强度与批准/沙箱模式恢复为默认值。因此 Codex 档案 SHALL 声明
`model`、`reasoning_effort`、`mode` 为加载后重新应用的配置项，并 SHALL 提供从 argv 推导这些值的映射：

| argv | 配置项取值 |
|---|---|
| `-m <m>`/`--model <m>` | `model = <m>` |
| `-c model_reasoning_effort=<e>`（值可带引号） | `reasoning_effort = <e>` |
| `--dangerously-bypass-approvals-and-sandbox` | `mode = agent-full-access` |
| `--approve-for-me` | `mode = agent` |
| `-s read-only` | `mode = read-only` |
| `-s workspace-write` | `mode = workspace-write` |

多个批准/沙箱参数同时出现时，按表中自上而下的顺序取第一个匹配。其他批准/沙箱组合（例如 `-s danger-full-access` 或单独的 `-a`）
SHALL NOT 推导 `mode`，并 SHALL 在聊天视图列为“以下参数在聊天端不生效”。

适配器在时间线中插入的“recorded with model … resuming with …”文本 SHALL 原样显示，系统 SHALL NOT 改写 agent 的消息内容。

#### Scenario: 预置配置的批准模式

- **WHEN** 预置的 Codex CLI 混合配置从终端开始，发出首条消息后切到聊天
- **THEN** 聊天端的 `mode` 为 `agent-full-access`，而不是适配器默认的 `agent`

### Requirement: Codex 的聊天端命令

Codex 档案 SHALL 声明在聊天视图命令菜单中隐藏的命令 `logout`：在聊天中执行它会注销服务用户的全局 Codex 登录，而聊天端不执行登录。
隐藏的命令 SHALL NOT 出现在命令菜单中；用户手动输入 `/logout` 时 SHALL 按普通文本发送给 agent。

#### Scenario: 命令菜单

- **WHEN** 用户在 Codex 混合会话的聊天视图中输入 `/`
- **THEN** 命令菜单中没有 `/logout`

### Requirement: Antigravity CLI 的 TUI 会话标识注入

Antigravity CLI（`agy`）的 TUI 原生通过 `OSC 7501` 根记录上报 `app=antigravity` 与运行状态，
但不在根记录中携带会话 ID。混合会话中 `agy` 的每一次 TUI 启动（包括首次启动与恢复）SHALL 注入
`--settings` 会话级设置，配置 `title` 命令在状态变化时将 `conversation_id` 作为 `OSC 7501`
子记录写入控制终端，且不向标准输出打印内容以免覆盖终端标题；原 argv 中的 `--settings` SHALL
作为丢弃参数省略并在切换结果中提示。首次启动时注入的参数 SHALL 在会话信息的命令中可见。

#### Scenario: agy 首次启动与恢复均注入会话级设置

- **WHEN** 混合配置的 argv 为 `["agy","--dangerously-skip-permissions","--settings","{}"]`，
  agent 会话 ID 为 `89c9a933-60f6-46ad-977b-a8daf22f8e4d`，用户从聊天切到终端
- **THEN** 恢复命令包含注入的 `--settings <title 配置>`、保留的 `--dangerously-skip-permissions`
  与 `--conversation 89c9a933-60f6-46ad-977b-a8daf22f8e4d`，界面列出未保留的 `--settings {}`

### Requirement: Antigravity CLI 的跨进程加载配置补齐与中断键

`agy acp` 在新进程执行 `session/load` 或 `session/resume` 时会将 `model` 与
`thought_level` 重置为默认值。`agy` 档案 SHALL 声明加载后需补齐的配置项列表为
`["model", "thought_level"]`，由系统在跨进程加载或恢复后按记录中的取值自动调用
`session/set_config_option` 恢复。`agy` 档案的中断键 SHALL 为单次 `Esc`（`\x1b`），
并启用 `session/resume` 用于崩溃重连。

#### Scenario: agy 跨进程加载后补齐模型与推理等级

- **WHEN** 用户在 `agy` 混合会话聊天视图中将 `model` 改为 `gemini-3.8-flash`、将
  `thought_level` 改为 `low`，随后切到终端再切回聊天（新 `agy acp` 进程的 `session/load`
  返回默认的 `gemini-4-argon` 与 `medium`）
- **THEN** 系统自动对 `model` 与 `thought_level` 调用 `session/set_config_option`，
  切回聊天后两项配置仍为 `gemini-3.8-flash` 与 `low`

### Requirement: Antigravity CLI 的后台状态检查 prompt

`agy` 档案 SHALL 声明后台状态检查 prompt。后台进程退出且连续两次静默轮询后，系统 SHALL 向 agy 发送以下英文消息，使其补发工具完成状态与结果；其他 agent 档案 SHALL NOT 因该机制额外发起模型回合。

> The background processes from the previous turn have exited. Please check the latest background task status and report the outcome to the user. Do not start any new tasks.

自动检查 SHALL 使用正常 `session/prompt` 管线，SHALL NOT 伪造工具退出码或读取 agy 的私有 transcript。检查回合 SHALL 标记为后台刷新，旧工具完成事件 SHALL NOT 再次触发检查。

#### Scenario: agy 后台退出后自动补发状态

- **WHEN** agy 的后台进程退出且连续两次轮询没有发现新进程
- **THEN** 系统自动发送上述英文消息，agy 补发旧工具的 `completed` 更新与结果回复，随后清除未完成状态并产生 `done`，且不再自动发起第二次检查

#### Scenario: 其他 agent 不使用 agy 专用检查

- **WHEN** Kimi 的后台进程退出且连续两次轮询没有发现新进程
- **THEN** 系统直接按提醒策略产生 `done`，不自动发送英文状态检查消息
