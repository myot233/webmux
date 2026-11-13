# hybrid-agent-profiles Specification

## Purpose

内置 agent 档案描述 webmux 如何识别、启动、恢复和中断某个支持 ACP 的 agent，让混合会话在 TUI 与 ACP 两端使用一致的命令、参数与环境。新增 agent 时只需新增档案，不改变会话编排。各 agent 专有的环境、配置推导、命令与进程边界见 `hybrid-agent-specifics`。

## Requirements

### Requirement: 内置档案与可识别的命令写法

系统 SHALL 内置 OpenCode、omp、Codex、Claude Code 与 Antigravity CLI（`agy`）五个档案，
SHALL NOT 开放用户自定义档案。启动配置的 argv 只有在以下写法之一中，且可执行文件的基本名与某个档案匹配时，
才 SHALL 视为可用于混合模式：

1. 直接执行：`[<可执行文件>, 参数…]`；
2. webmux 的启动文件包装：`["bash", "-ic", "exec -- \"$@\"", "webmux", <可执行文件>, 参数…]`
   或 `["zsh", "-ic", "exec -- \"$@\"", "webmux", <可执行文件>, 参数…]`。

其他任何写法（例如其他 shell 包装、`env` 前缀）SHALL 视为不可用，并给出原因“命令写法不受支持”。
未匹配任何档案时，服务端 SHALL 在 `hybridSupport.supportedAgents` 中返回当前注册表支持的
agent 显示名称列表。

#### Scenario: 预置的 bashrc 包装

- **WHEN** 配置的 argv 为 `["bash","-ic","exec -- \"$@\"","webmux","omp"]`
- **THEN** 判定为可用，档案为 omp

#### Scenario: Codex 预置配置

- **WHEN** 配置的 argv 为 `["bash","-ic","exec -- \"$@\"","webmux","codex","--dangerously-bypass-approvals-and-sandbox"]`
- **THEN** 判定为可用，档案为 codex

#### Scenario: agy 的 bashrc 包装

- **WHEN** 配置的 argv 为 `["bash","-ic","exec -- \"$@\"","webmux","agy","--dangerously-skip-permissions"]`
- **THEN** 判定为可用，档案为 `agy`

#### Scenario: 自定义包装

- **WHEN** 配置的 argv 为 `["sh","-c","omp"]`
- **THEN** 判定为不可用，原因为命令写法不受支持

### Requirement: 使两端看到不同会话库或目录的参数

以下参数 SHALL 使配置不可用于混合模式：

- 让 TUI 与 ACP 看到不同会话库的参数，原因为“该参数使聊天视图无法找到会话”：
  - omp 的 `--session-dir` 只对 ACP 新建的会话生效，ACP 的列出、加载与恢复始终使用默认目录；
    `--no-session` 不保存会话；
  - OpenCode 的 `--server` 使 TUI 连接另一个服务及其会话库；
  - Codex 的 `--remote`、`--remote-auth-token-env` 使 TUI 连接远端 app-server；
  - Claude Code 的 `--no-session-persistence`、`--environment`；
  - `agy` 的 `--project` 与 `--new-project` 使用云端项目会话，忽略本地 `--conversation` 会话恢复。
- 在命令中指定工作目录的参数，原因为“请在新建会话时选择目录，不要在命令中写目录”：
  OpenCode 的位置参数、omp 的 `--cwd`、Codex 的 `-C`/`--cd` 与 `--worktree`，
  以及 Claude Code 的 `-w`/`--worktree` 与 `--tmux`。
- 打开已有会话的参数，原因为“该参数打开已有会话，混合配置请不要指定会话”（原因码 `session_arg`）：
  Codex 的 `resume`、`fork`，以及 Claude Code 的 `-c`/`--continue`、`-r`/`--resume`、
  `--session-id`、`--fork-session`、`--from-pr`。

omp 的 `--profile` SHALL NOT 视为不可用：它同时透传给 ACP 进程，两端使用同一个配置档的会话库。

#### Scenario: OpenCode 命令中写了目录

- **WHEN** 用户对 argv 为 `["opencode","/home/u/proj"]` 的配置勾选混合模式
- **THEN** 保存返回 400，原因提示在新建会话时选择目录

#### Scenario: Codex 命令中写了恢复子命令

- **WHEN** 用户对 argv 为 `["codex","resume","--last"]` 的配置勾选混合模式
- **THEN** 保存返回 400，原因码为 `session_arg`

#### Scenario: agy 使用云端项目参数

- **WHEN** 用户对 argv 为 `["agy","--project","my-cloud-proj"]` 的配置勾选混合模式
- **THEN** 保存返回 400，原因为该参数使聊天视图无法找到会话

### Requirement: 不启动交互界面的参数

以下参数 SHALL 使配置不可用于混合模式，原因为“该参数不启动交互界面”：

- omp 的 `-p`/`--print`、`--mode`、`--export`、`--alias`、`-h`/`--help`、`-v`/`--version`，
  以及首个位置参数为 omp 子命令（例如 `commit`、`acp`）的写法；
- OpenCode 的 `-h`/`--help`、`-v`/`--version`、`--completions`、`--wizard`；
- Codex 的 `-h`/`--help`、`-V`/`--version`，以及首个位置参数为以下子命令的写法：
  `exec`/`e`、`review`、`login`、`logout`、`mcp`、`plugin`、`app-server`、`remote-control`、
  `app`、`completion`、`update`、`doctor`、`sandbox`、`debug`、`apply`/`a`、`cloud`、
  `exec-server`、`features`、`agents`、`queue`、`archive`、`delete`、`unarchive`、`help`，
  以及以 `migrate-` 开头的子命令；
- Claude Code 的 `-p`/`--print`、`--output-format`、`--input-format`、`--json-schema`、
  `--include-partial-messages`、`--replay-user-messages`、`--bare`、`--init-only`、
  `-h`/`--help`、`-v`/`--version`，以及非交互子命令；
- `agy` 的 `-p`/`--print`/`--prompt`、`--output-format`、`--input-format`、`--json-schema`、
  `--print-timeout`、`--disable-slash-commands`、`-h`/`--help`、`-v`/`--version`，以及首个位置参数为
  `agy` 子命令（`acp`、`agent`、`agents`、`changelog`、`help`、`install`、`mcp`、
  `mic-serve`、`models`、`plugin`、`plugins`、`remote-control`、`update`）的写法。

#### Scenario: 非交互用法

- **WHEN** 用户对 argv 为 `["omp","-p","总结改动"]` 的配置勾选混合模式
- **THEN** 保存返回 400，原因为该参数不启动交互界面

#### Scenario: Codex 非交互子命令

- **WHEN** 用户对 argv 为 `["codex","exec","修 bug"]` 的配置勾选混合模式
- **THEN** 保存返回 400，原因为该参数不启动交互界面

#### Scenario: agy 非交互或子命令用法

- **WHEN** 用户对 argv 为 `["agy","-p","总结改动"]` 或 `["agy","mcp"]` 的配置勾选混合模式
- **THEN** 保存返回 400，原因为该参数不启动交互界面

### Requirement: 恢复命令的生成

切换或恢复到 TUI 时，系统 SHALL 按以下方式生成恢复命令：

- 由可执行文件、档案要求注入的 TUI 参数、原 argv 中位于档案白名单内的参数、档案的恢复参数与
  agent 会话 ID 组成，并套用与原命令相同的写法（直接执行或 Bash/Zsh 启动文件包装）。
  恢复参数：OpenCode 为 `-s <id>`，omp 为 `--resume <id>`，Codex 为子命令 `resume <id>`
  （全局参数位于子命令之前），Claude Code 为 `--resume <id>`，`agy` 为 `--conversation <id>`。
- 白名单参数 SHALL 同时识别 `--flag value` 与 `--flag=value` 两种形式，并按档案声明的参数个数解析。
- Codex 的白名单为：`-m`/`--model`、`-p`/`--profile`、`-s`/`--sandbox`、`-a`/`--ask-for-approval`、
  `--approve-for-me`、`--dangerously-bypass-approvals-and-sandbox`、`--dangerously-bypass-hook-trust`、
  `-c`/`--config`、`--enable`、`--disable`、`--search`、`--add-dir`、`--oss`、`--local-provider`、
  `--no-alt-screen`、`--strict-config`、`--no-daemon`。
- `agy` 的白名单为开关 `--dangerously-skip-permissions`、`--sandbox`，以及带值参数 `--agent`、
  `--add-dir`、`--log-file`、`--model`、`--effort`、`--mode`。
- 系统 SHALL 丢弃以下参数：
  - omp、Codex、Claude Code 与 `agy` 的位置参数（提示词）；
  - 提示词与附图参数（例如 OpenCode 的 `--prompt`、Codex 的 `-i`/`--image`、`agy` 的 `-i`/`--prompt-interactive`）；
  - 会话类与会话级覆盖参数（`-c`、`--continue`、`-r`、`--resume`、`--restore`、`-s`、`--session`、
    `--conversation`、`--fork`、`agy` 的 `--settings` 与 `--remote-control` 等，按档案区分）；
  - 白名单之外的参数。
- 被丢弃的参数 SHALL 在切换结果中列出，界面 SHALL 提示“以下参数在恢复时未保留”。

#### Scenario: 丢弃提示词与会话参数

- **WHEN** 原 argv 为 `["omp","--model","haiku","--continue","修 bug"]`，agent 会话 ID 为 `01abc`
- **THEN** 恢复命令为 `["omp","--model","haiku","--resume","01abc"]`，界面列出未保留的 `--continue` 与 `修 bug`

#### Scenario: Codex 子命令式恢复

- **WHEN** 原 argv 为 `["codex","-m","gpt-5.6-luna","-s","read-only","修 bug"]`，agent 会话 ID 为 `01a18715`
- **THEN** 恢复命令为 `["codex","--no-daemon","-m","gpt-5.6-luna","-s","read-only","resume","01a18715"]`，
  界面列出未保留的 `修 bug`

#### Scenario: agy 生成恢复命令

- **WHEN** 原 argv 为 `["agy","--dangerously-skip-permissions","--model","gemini-3.8-flash","-c","修 bug"]`，
  agent 会话 ID 为 `89c9a933-60f6-46ad-977b-a8daf22f8e4d`
- **THEN** 恢复命令保留 `--dangerously-skip-permissions` 与 `--model gemini-3.8-flash` 并追加
  `--conversation 89c9a933-60f6-46ad-977b-a8daf22f8e4d`，界面列出未保留的 `-c` 与 `修 bug`

### Requirement: OpenCode 的 TUI 以独立服务运行

OpenCode 的 TUI 默认连接后台服务，回合在后台服务中运行，结束 TUI 并不会停止回合。因此，混合会话中 OpenCode 的每一次 TUI 启动（包括首次启动与恢复）SHALL 带上 `--standalone`，使 TUI 持有自己的服务进程，结束 TUI 即停止其回合。

- 原 argv 已包含该参数时 SHALL NOT 重复添加。
- 首次启动时追加的参数 SHALL 在会话信息的命令中可见。

#### Scenario: OpenCode 以独立服务运行

- **WHEN** 混合配置的 argv 为 `["opencode","--prompt","hi"]`，agent 会话 ID 为 `ses_1`，用户从聊天切到终端
- **THEN** 恢复命令为 `["opencode","--standalone","-s","ses_1"]`，界面列出未保留的 `--prompt hi`；之后切回聊天时，结束 TUI 即停止其回合

### Requirement: ACP 启动命令

系统 SHALL 按档案声明的 ACP 启动方式启动 ACP 进程，并套用与原命令相同的写法：

- 内置子命令：OpenCode 为 `opencode acp`，omp 为 `omp acp`，`agy` 为 `agy acp`；
- 第三方连接器：Codex 为 `codex-acp`，Claude Code 为 `claude-agent-acp`，启动前 SHALL 按
  `acp-connector-availability` 检查连接器已安装。

ACP 进程的参数透传按 agent 区分：

- omp：`omp acp` 解析完整 argv，并静默忽略未知参数。因此原 argv 中位于 omp 白名单内的参数 SHALL 全部透传给
  ACP 进程（例如 `--model`、`--thinking`、`--profile`、`--config`、`--approval-mode`、`--auto-approve`），
  使两端使用相同的模型、配置档与批准策略。
- OpenCode：`opencode acp` 只接受 `--login`，因此 OpenCode 的白名单参数 SHALL NOT 透传。`--auto` 也
  SHALL NOT 在聊天视图提示不支持，因为聊天端的工具批准由聊天视图的权限模式决定（默认自动批准）。
- Codex：`codex-acp` 不接受这些参数，白名单参数 SHALL NOT 透传。模型、推理强度与批准/沙箱参数按
  “Codex 的聊天端配置推导”延续；`-p`/`--profile`、`-c`/`--config`、`--enable`、`--disable`、
  `--search`、`--add-dir`、`--oss`、`--local-provider` 在聊天端不生效，SHALL 在聊天视图列为
  “以下参数在聊天端不生效”。
- Claude Code：`claude-agent-acp` 不接受 CLI 参数，模型、推理强度与权限模式按“Claude Code 的聊天端配置推导”延续，
  其余保留参数在聊天视图列为“以下参数在聊天端不生效”。
- `agy`：`agy acp` 校验其子命令标志，原 argv 中位于 `agy` 白名单且 `agy acp` 接受的参数
  （`--dangerously-skip-permissions`、`--sandbox`、`--agent`、`--add-dir`、`--log-file`）SHALL 透传给
  `agy acp`；仅 TUI 接受的 `--model`、`--effort`、`--mode` SHALL NOT 透传给 `agy acp`。
  `--model` 与 `--effort` SHALL 分别作为 `model` 与 `thought_level` 配置种子延续到聊天端，
  `--mode` SHALL 在聊天视图的 `unsupported` 列表中提示。

使用启动文件包装时，`.bashrc`/`.zshrc` 执行期间写到 stdout 的内容 SHALL NOT 进入 ACP 协议通道。

#### Scenario: bashrc 有输出

- **WHEN** 用户的 `.bashrc` 会 `echo` 一行欢迎语，混合会话从聊天开始
- **THEN** ACP 握手成功，欢迎语不导致协议错误

#### Scenario: omp 参数透传

- **WHEN** 混合配置的 argv 为 `["omp","--profile","work","--model","haiku","修 bug"]`，会话从聊天开始
- **THEN** ACP 进程的 argv 为 `["omp","acp","--profile","work","--model","haiku"]`

#### Scenario: Codex 使用连接器

- **WHEN** 混合配置的 argv 为 `["bash","-ic","exec -- \"$@\"","webmux","codex","--search","-m","gpt-5.6-luna"]`，
  会话从聊天开始
- **THEN** ACP 进程以 bashrc 包装执行 `codex-acp`，不带任何参数；聊天视图列出 `--search` 在聊天端不生效

#### Scenario: agy 参数按子集透传

- **WHEN** 混合配置的 argv 为 `["agy","--dangerously-skip-permissions","--sandbox","--model","gemini-3.8-flash","--mode","plan"]`，
  会话从聊天开始
- **THEN** ACP 进程的 argv 为 `["agy","acp","--dangerously-skip-permissions","--sandbox"]`，
  聊天配置种子包含 `model=gemini-3.8-flash`，`unsupported` 包含 `["--mode"]`

### Requirement: 两端环境一致

ACP 进程、恢复后的 TUI 与最初的 TUI SHALL 使用同一套环境：

- 启动配置保存的环境变量；
- 服务进程环境，按现有规则移除宿主终端变量；
- webmux 注入的变量，包括同一个 `WEBMUX_SESSION_ID`。

`webmux run --launch-config` 创建的混合会话同样 SHALL NOT 继承调用方 shell 的环境。

传给 ACP `session/new`、`session/list`、`session/load` 与 `session/resume` 的 cwd SHALL 是解析符号链接后的绝对路径。TUI 记录的是解析后的路径，而 OpenCode 的 `session/list` 按字符串精确匹配 cwd，未解析的符号链接路径查不到 TUI 创建的会话。

#### Scenario: 符号链接目录

- **WHEN** 用户在符号链接目录 `/home/u/proj-link`（指向 `/home/u/proj`）中新建 OpenCode 混合会话，在 TUI 中发出首条消息后切到聊天
- **THEN** 资格核对与 `session/load` 都使用 `/home/u/proj`，切换成功

#### Scenario: 配置文件目录变量

- **WHEN** 混合配置设置了 `PI_CODING_AGENT_DIR=/tmp/agentdir`
- **THEN** ACP 进程与恢复后的 TUI 都能看到同样的值，并能列出同一目录中的会话

### Requirement: 档案与混合开关的校验

保存启动配置时，若 `hybrid: true` 而 argv 不满足可识别的写法，服务端 SHALL 返回 400，并给出不可用原因。启动配置摘要与完整配置 SHALL 提供只读字段 `hybridSupport`，内容为 `{supported, agent?, reason?, connector?, supportedAgents}`，供设置页决定勾选框是否可用及显示原因。

#### Scenario: 对 shell 配置勾选

- **WHEN** 客户端保存 `{name: "Shell", cmd: ["/bin/bash","-l"], hybrid: true}`
- **THEN** 服务端返回 400，原因为未识别到支持的 agent

### Requirement: 版本提示

agent 的版本来自终端信号、ACP `initialize` 返回的 `agentInfo.version`，或档案声明的版本命令。
版本低于档案记录的实测版本时，系统 SHALL 在会话页显示提示，SHALL NOT 因此阻止切换；能否切换只由资格判定决定。
版本无法解析时 SHALL 不提示。

- OpenCode、omp 与 `agy` 的实测版本分别为 2.1.4、18.13.1 与 1.4.12，来自终端信号或 `agentInfo.version`。
- 使用第三方连接器的档案 SHALL 分别记录 agent 与连接器的实测版本：Codex 0.165.0 与 codex-acp 2.3.0，
  Claude Code 2.1.316 与 claude-agent-acp 0.99.1。两者任一低于实测版本时 SHALL 分别提示。

#### Scenario: 旧版本

- **WHEN** ACP `initialize` 返回 omp 版本 18.0.0
- **THEN** 聊天视图显示“低于已验证版本 18.13.1”的提示，其余功能照常

#### Scenario: 旧版连接器

- **WHEN** Codex 混合会话的 `initialize` 返回 `agentInfo.version` 为 2.0.0，`codex --version` 为 0.165.0
- **THEN** 聊天视图只提示 Codex ACP 适配器低于已验证版本 2.3.0

#### Scenario: agy 旧版本

- **WHEN** ACP `initialize` 返回 `agy` 版本 1.3.0
- **THEN** 聊天视图显示“低于已验证版本 1.4.12”的提示，其余功能照常

### Requirement: 连接器检查跟随启动文件 shell

混合会话在启动 ACP 进程前检查档案声明的第三方连接器时，若配置使用启动文件包装，SHALL 使用同一 shell（Bash 或 Zsh）执行 `command -v`。启动文件的输出 SHALL 先于结果标记；超时、输出上限、结果解析与“启动前以 127 退出表示连接器缺失”的判定 SHALL 与既有规则一致。

#### Scenario: Zsh 下连接器已安装

- **WHEN** 配置使用 `zsh -ic` 包装且 `~/.zshrc` 设置了包含 `codex-acp` 的 PATH
- **THEN** 连接器检查经同一 Zsh 包装找到 `codex-acp`，聊天端启动

#### Scenario: Zsh 启动文件有输出

- **WHEN** `~/.zshrc` 输出多行提示后以 127 退出且未找到连接器
- **THEN** 检查结果仍不含启动文件输出，并判定为连接器缺失

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
