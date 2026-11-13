## ADDED Requirements

### Requirement: Devin CLI 档案

系统 SHALL 内置 Devin CLI 档案，稳定标识为 `devin`，显示名称为 `Devin CLI`，图标为 `devin`，实测版本为 `3000.12.57`。直接执行 `devin` 或使用既有 Bash/Zsh 启动文件包装的 `devin` 配置 SHALL 判定为支持混合模式，不需要第三方 ACP 连接器。

Devin 的 ACP 启动命令 SHALL 为 `devin acp`。TUI 恢复命令 SHALL 保留白名单参数并追加 `--resume <sessionId>`；中断键 SHALL 为一次 Esc（`\x1b`）。版本低于 `3000.12.57` 时 SHALL 按通用规则提示但不阻止切换。

Devin 的 `agentInfo.version` 实测为 `0.0.0-dev`，版本核对 SHALL 使用 `devin --version` 的输出，SHALL NOT 以 `agentInfo.version` 判定。

Devin 的 `session/resume` SHALL NOT 使用：其实测返回 `Method not found`，崩溃重连与切到聊天一律使用 `session/load` 回放。

#### Scenario: 预置的 bashrc 包装

- **WHEN** 配置的 argv 为 `["bash","-ic","exec -- \"$@\"","webmux","devin","--permission-mode","dangerous"]`
- **THEN** 判定为可用，`hybridSupport.agent` 为 `devin`，且没有连接器信息

#### Scenario: 恢复到 TUI

- **WHEN** Devin 混合会话切回 TUI，agent 会话 ID 为 `luminous-acorn`
- **THEN** 恢复命令保留白名单参数并追加 `--resume luminous-acorn`

#### Scenario: 崩溃重连走加载回放

- **WHEN** Devin 混合会话的聊天端进程异常退出后重连
- **THEN** 系统使用 `session/load` 回放会话，不调用 `session/resume`

### Requirement: Devin 恢复命令保留的参数

Devin 的恢复命令 SHALL 保留带值参数 `--model`、`--permission-mode`、`--verbosity`、可选值参数 `--respect-workspace-trust`、`--export`，以及开关 `--sandbox`。这些参数 SHALL 支持 `--flag value` 与 `--flag=value` 形式。

`--prompt-file`、`--` 之后的位置参数与未识别参数 SHALL 按通用规则丢弃。

#### Scenario: 白名单参数进入恢复命令

- **WHEN** 配置的 argv 为 `["devin","--permission-mode","dangerous","--model","swe-2-high","--sandbox"]`，会话切回 TUI 恢复会话 `calm-fjord`
- **THEN** 恢复命令依次包含这些白名单参数与 `--resume calm-fjord`

### Requirement: Devin 会话选择、目录与会话库参数

`-c`/`--continue` 与 `-r`/`--resume`（值可选）SHALL 使配置不可用于混合模式，原因码为 `session_arg`。

`--cloud` 与 `--config <path>` SHALL 使配置不可用于混合模式，原因为会话库或配置不一致。

首个位置参数为 PATH 时（`devin [PATH]...` 打开 Devin Desktop）SHALL 使配置不可用于混合模式，原因提示在新建会话时选择目录。

#### Scenario: 恢复参数不可用

- **WHEN** 用户对 argv 为 `["devin","--resume","old-session"]` 的配置勾选混合模式
- **THEN** 保存返回 400，原因码为 `session_arg`

#### Scenario: 云端模式不可用

- **WHEN** 用户对 argv 为 `["devin","--cloud"]` 的配置勾选混合模式
- **THEN** 保存返回 400，原因为该参数使聊天视图无法找到会话

#### Scenario: Desktop 路径参数不可用

- **WHEN** 用户对 argv 为 `["devin","/home/u/proj"]` 的配置勾选混合模式
- **THEN** 保存返回 400，原因提示在新建会话时选择目录

### Requirement: Devin 不启动交互界面的参数

`-p`/`--print`（可选值）、`-h`/`--help`、`-V`/`--version` SHALL 使配置不可用于混合模式，原因为“该参数不启动交互界面”。

首个位置参数为 `acp`、`auth`、`cloud`、`desktop`、`doctor`、`forward`、`help`、`list`、`ls`、`mcp`、`migrate`、`models`、`plugins`、`rm`、`rules`、`sandbox`、`setup`、`skills`、`ssh`、`uninstall`、`update` 或 `version` 时，SHALL 以相同原因拒绝混合模式。

#### Scenario: 非交互打印

- **WHEN** 用户对 argv 为 `["devin","-p","总结改动"]` 的配置勾选混合模式
- **THEN** 保存返回 400，原因为该参数不启动交互界面

#### Scenario: 会话管理子命令

- **WHEN** 用户对 argv 为 `["devin","list"]` 或 `["devin","rm","old-session"]` 的配置勾选混合模式
- **THEN** 保存返回 400，原因为该参数不启动交互界面

## MODIFIED Requirements

### Requirement: 内置档案与可识别的命令写法

系统 SHALL 内置 OpenCode、omp、Codex、Claude Code、Antigravity CLI（`agy`）、Qoder、QoderCN、Kimi Code CLI 与 Devin CLI 九个档案，
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

#### Scenario: Devin 预置配置

- **WHEN** 配置的 argv 为 `["bash","-ic","exec -- \"$@\"","webmux","devin","--permission-mode","dangerous"]`
- **THEN** 判定为可用，档案为 `devin`

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
  - `agy` 的 `--project` 与 `--new-project` 使用云端项目会话，忽略本地 `--conversation` 会话恢复；
  - `devin` 的 `--cloud` 切换到云端会话库，`--config` 可改变会话存储等配置且 `devin acp` 无对应参数。
- 在命令中指定工作目录的参数，原因为“请在新建会话时选择目录，不要在命令中写目录”：
  OpenCode 的位置参数、omp 的 `--cwd`、Codex 的 `-C`/`--cd` 与 `--worktree`，
  Claude Code 的 `-w`/`--worktree` 与 `--tmux`，以及 `devin` 的位置 PATH 参数
  （`devin [PATH]...` 打开 Devin Desktop 而非 TUI）。
- 打开已有会话的参数，原因为“该参数打开已有会话，混合配置请不要指定会话”（原因码 `session_arg`）：
  Codex 的 `resume`、`fork`，Claude Code 的 `-c`/`--continue`、`-r`/`--resume`、
  `--session-id`、`--fork-session`、`--from-pr`，以及 `devin` 的 `-c`/`--continue`、
  `-r`/`--resume`。

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

#### Scenario: Devin 命令中写了恢复参数

- **WHEN** 用户对 argv 为 `["devin","--resume","old-session"]` 或 `["devin","-c"]` 的配置勾选混合模式
- **THEN** 保存返回 400，原因码为 `session_arg`

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
  `mic-serve`、`models`、`plugin`、`plugins`、`remote-control`、`update`）的写法；
- `devin` 的 `-p`/`--print`、`-h`/`--help`、`-V`/`--version`，以及首个位置参数为
  `devin` 子命令（`acp`、`auth`、`cloud`、`desktop`、`doctor`、`forward`、`help`、
  `list`/`ls`、`mcp`、`migrate`、`models`、`plugins`、`rm`、`rules`、`sandbox`、
  `setup`、`skills`、`ssh`、`uninstall`、`update`、`version`）的写法。

#### Scenario: 非交互用法

- **WHEN** 用户对 argv 为 `["omp","-p","总结改动"]` 的配置勾选混合模式
- **THEN** 保存返回 400，原因为该参数不启动交互界面

#### Scenario: Codex 非交互子命令

- **WHEN** 用户对 argv 为 `["codex","exec","修 bug"]` 的配置勾选混合模式
- **THEN** 保存返回 400，原因为该参数不启动交互界面

#### Scenario: agy 非交互或子命令用法

- **WHEN** 用户对 argv 为 `["agy","-p","总结改动"]` 或 `["agy","mcp"]` 的配置勾选混合模式
- **THEN** 保存返回 400，原因为该参数不启动交互界面

#### Scenario: Devin 非交互或子命令用法

- **WHEN** 用户对 argv 为 `["devin","-p","总结改动"]` 或 `["devin","list"]` 的配置勾选混合模式
- **THEN** 保存返回 400，原因为该参数不启动交互界面

### Requirement: 恢复命令的生成

切换或恢复到 TUI 时，系统 SHALL 按以下方式生成恢复命令：

- 由可执行文件、档案要求注入的 TUI 参数、原 argv 中位于档案白名单内的参数、档案的恢复参数与
  agent 会话 ID 组成，并套用与原命令相同的写法（直接执行或 Bash/Zsh 启动文件包装）。
  恢复参数：OpenCode 为 `-s <id>`，omp 为 `--resume <id>`，Codex 为子命令 `resume <id>`
  （全局参数位于子命令之前），Claude Code 为 `--resume <id>`，`agy` 为 `--conversation <id>`，
  `devin` 为 `--resume <id>`。
- 白名单参数 SHALL 同时识别 `--flag value` 与 `--flag=value` 两种形式，并按档案声明的参数个数解析。
- Codex 的白名单为：`-m`/`--model`、`-p`/`--profile`、`-s`/`--sandbox`、`-a`/`--ask-for-approval`、
  `--approve-for-me`、`--dangerously-bypass-approvals-and-sandbox`、`--dangerously-bypass-hook-trust`、
  `-c`/`--config`、`--enable`、`--disable`、`--search`、`--add-dir`、`--oss`、`--local-provider`、
  `--no-alt-screen`、`--strict-config`、`--no-daemon`。
- `agy` 的白名单为开关 `--dangerously-skip-permissions`、`--sandbox`，以及带值参数 `--agent`、
  `--add-dir`、`--log-file`、`--model`、`--effort`、`--mode`。
- `devin` 的白名单为开关 `--sandbox`，带值参数 `--model`、`--permission-mode`、`--verbosity`，
  以及可选值参数 `--respect-workspace-trust`、`--export`。
- 系统 SHALL 丢弃以下参数：
  - omp、Codex、Claude Code、`agy` 与 `devin` 的位置参数（提示词或目录）；
  - 提示词与附图参数（例如 OpenCode 的 `--prompt`、Codex 的 `-i`/`--image`、`agy` 的 `-i`/`--prompt-interactive`、
    `devin` 的 `--prompt-file` 与 `--` 之后的内容）；
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

#### Scenario: Devin 生成恢复命令

- **WHEN** 原 argv 为 `["devin","--permission-mode","dangerous","--model","swe-2-high","--","修 bug"]`，
  agent 会话 ID 为 `calm-fjord`
- **THEN** 恢复命令保留 `--permission-mode dangerous` 与 `--model swe-2-high` 并追加
  `--resume calm-fjord`，界面列出未保留的 `-- 修 bug`

### Requirement: ACP 启动命令

系统 SHALL 按档案声明的 ACP 启动方式启动 ACP 进程，并套用与原命令相同的写法：

- 内置子命令：OpenCode 为 `opencode acp`，omp 为 `omp acp`，`agy` 为 `agy acp`，`devin` 为 `devin acp`；
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
- `devin`：`devin acp` 只接受 `--model`、`--agent-type`、`--refusal-fallback` 与 `--cloud`，
  因此白名单中只有 `--model` SHALL 透传给 `devin acp`；`--permission-mode` 按
  “Devin 的聊天端配置推导”映射为 `mode` 配置种子，`--sandbox`、`--verbosity`、
  `--respect-workspace-trust` 与 `--export` SHALL 在聊天视图列为“以下参数在聊天端不生效”。

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

#### Scenario: Devin 只透传模型

- **WHEN** 混合配置的 argv 为 `["devin","--permission-mode","dangerous","--model","swe-2-high","--sandbox"]`，
  会话从聊天开始
- **THEN** ACP 进程的 argv 为 `["devin","acp","--model","swe-2-high"]`，聊天配置种子包含
  `model=swe-2-high` 与 `mode=bypass`，`unsupported` 包含 `["--sandbox"]`

### Requirement: 版本提示

agent 的版本 SHALL 来自终端信号、ACP `initialize` 返回的 `agentInfo.version`，或档案声明的版本命令。
版本低于档案记录的实测版本时，系统 SHALL 在会话页显示提示，SHALL NOT 因此阻止切换；能否切换只由资格判定决定。
版本无法解析时 SHALL 不提示。

- OpenCode、omp 与 `agy` 的实测版本分别为 2.1.4、18.13.1 与 1.4.12，来自终端信号或 `agentInfo.version`。
- `devin` 的实测版本为 3000.12.57，来自 `devin --version`；其 `agentInfo.version` 恒为
  `0.0.0-dev`，SHALL NOT 用于版本比较。
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

#### Scenario: Devin 版本不来自 agentInfo

- **WHEN** Devin 混合会话的 `initialize` 返回 `agentInfo.version` 为 `0.0.0-dev`，`devin --version` 为 3000.12.57
- **THEN** 系统以 3000.12.57 判定版本，不显示“低于已验证版本”的提示
