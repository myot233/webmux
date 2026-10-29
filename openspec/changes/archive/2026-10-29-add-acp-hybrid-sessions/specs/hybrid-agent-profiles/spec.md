## Purpose

内置 agent 档案描述 webmux 如何识别、启动、恢复和中断某个支持 ACP 的 agent，让混合会话在 TUI 与 ACP 两端使用一致的命令、参数与环境。新增 agent 时只需新增档案，不改变会话编排。

## ADDED Requirements

### Requirement: 内置档案与可识别的命令写法

系统 SHALL 内置 OpenCode 与 omp 两个档案，第一期 SHALL NOT 开放用户自定义档案。启动配置的 argv 只有在以下两种写法之一中，且可执行文件的基本名与某个档案匹配时，才 SHALL 视为可用于混合模式：

1. 直接执行：`[<可执行文件>, 参数…]`；
2. webmux 的 bashrc 包装：`["bash", "-ic", "exec -- \"$@\"", "webmux", <可执行文件>, 参数…]`。

其他任何写法（例如其他 shell 包装、`env` 前缀）SHALL 视为不可用，并给出原因“命令写法不受支持”。

#### Scenario: 预置的 bashrc 包装

- **WHEN** 配置的 argv 为 `["bash","-ic","exec -- \"$@\"","webmux","omp"]`
- **THEN** 判定为可用，档案为 omp

#### Scenario: 自定义包装

- **WHEN** 配置的 argv 为 `["sh","-c","omp"]`
- **THEN** 判定为不可用，原因为命令写法不受支持

### Requirement: 使两端看到不同会话库或目录的参数

以下参数 SHALL 使配置不可用于混合模式：

- 让 TUI 与 ACP 看到不同会话库的参数，原因为“该参数使聊天视图无法找到会话”：
  - omp 的 `--session-dir` 只对 ACP 新建的会话生效，ACP 的列出、加载与恢复始终使用默认目录；`--no-session` 不保存会话；
  - OpenCode 的 `--server` 使 TUI 连接另一个服务及其会话库。
- 在命令中指定工作目录的参数，原因为“请在新建会话时选择目录，不要在命令中写目录”：OpenCode 的位置参数，以及 omp 的 `--cwd`。

omp 的 `--profile` SHALL NOT 视为不可用：它同时透传给 ACP 进程，两端使用同一个配置档的会话库。

#### Scenario: OpenCode 命令中写了目录

- **WHEN** 用户对 argv 为 `["opencode","/home/u/proj"]` 的配置勾选混合模式
- **THEN** 保存返回 400，原因提示在新建会话时选择目录

### Requirement: 不启动交互界面的参数

以下参数 SHALL 使配置不可用于混合模式，原因为“该参数不启动交互界面”：

- omp 的 `-p`/`--print`、`--mode`、`--export`、`--alias`、`-h`/`--help`、`-v`/`--version`，以及首个位置参数为 omp 子命令（例如 `commit`、`acp`）的写法；
- OpenCode 的 `-h`/`--help`、`-v`/`--version`、`--completions`、`--wizard`。

#### Scenario: 非交互用法

- **WHEN** 用户对 argv 为 `["omp","-p","总结改动"]` 的配置勾选混合模式
- **THEN** 保存返回 400，原因为该参数不启动交互界面

### Requirement: 恢复命令的生成

切换或恢复到 TUI 时，系统 SHALL 按以下方式生成恢复命令：

- 由可执行文件、原 argv 中位于档案白名单内的参数、档案的恢复参数（OpenCode 为 `-s <id>`，omp 为 `--resume <id>`）组成，并套用与原命令相同的写法（直接执行或 bashrc 包装）。
- 白名单参数 SHALL 同时识别 `--flag value` 与 `--flag=value` 两种形式，并按档案声明的参数个数解析。
- 系统 SHALL 丢弃以下参数：
  - omp 的位置参数（提示词）；
  - 提示词参数（例如 OpenCode 的 `--prompt`）；
  - 会话类参数（`-c`、`--continue`、`-r`、`--resume`、`--restore`、`-s`、`--session`、`--fork` 等）；
  - 白名单之外的参数。
- 被丢弃的参数 SHALL 在切换结果中列出，界面 SHALL 提示“以下参数在恢复时未保留”。

#### Scenario: 丢弃提示词与会话参数

- **WHEN** 原 argv 为 `["omp","--model","haiku","--continue","修 bug"]`，agent 会话 ID 为 `01abc`
- **THEN** 恢复命令为 `["omp","--model","haiku","--resume","01abc"]`，界面列出未保留的 `--continue` 与 `修 bug`

### Requirement: OpenCode 的 TUI 以独立服务运行

OpenCode 的 TUI 默认连接后台服务，回合在后台服务中运行，结束 TUI 并不会停止回合。因此，混合会话中 OpenCode 的每一次 TUI 启动（包括首次启动与恢复）SHALL 带上 `--standalone`，使 TUI 持有自己的服务进程，结束 TUI 即停止其回合；原 argv 已包含该参数时 SHALL NOT 重复添加。首次启动时追加的参数 SHALL 在会话信息的命令中可见。

#### Scenario: OpenCode 以独立服务运行

- **WHEN** 混合配置的 argv 为 `["opencode","--prompt","hi"]`，agent 会话 ID 为 `ses_1`，用户从聊天切到终端
- **THEN** 恢复命令为 `["opencode","--standalone","-s","ses_1"]`，界面列出未保留的 `--prompt hi`；之后切回聊天时，结束 TUI 即停止其回合

### Requirement: ACP 启动命令

系统 SHALL 按档案启动 ACP 进程：OpenCode 为 `opencode acp`，omp 为 `omp acp`，并套用与原命令相同的写法。

omp 的 `omp acp` 解析完整 argv，并静默忽略未知参数。因此原 argv 中位于 omp 白名单内的参数 SHALL 全部透传给 ACP 进程（例如 `--model`、`--thinking`、`--profile`、`--config`、`--approval-mode`、`--auto-approve`），使两端使用相同的模型、配置档与批准策略。`opencode acp` 只接受 `--login`，OpenCode 的白名单参数 SHALL NOT 透传；`--auto` 也 SHALL NOT 在聊天视图提示不支持，聊天端的工具批准由聊天视图的权限模式决定（默认自动批准）。

使用 bashrc 包装时，`.bashrc` 执行期间写到 stdout 的内容 SHALL NOT 进入 ACP 协议通道。

#### Scenario: bashrc 有输出

- **WHEN** 用户的 `.bashrc` 会 `echo` 一行欢迎语，混合会话从聊天开始
- **THEN** ACP 握手成功，欢迎语不导致协议错误

#### Scenario: omp 参数透传

- **WHEN** 混合配置的 argv 为 `["omp","--profile","work","--model","haiku","修 bug"]`，会话从聊天开始
- **THEN** ACP 进程的 argv 为 `["omp","acp","--profile","work","--model","haiku"]`

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

保存启动配置时，若 `hybrid: true` 而 argv 不满足可识别的写法，服务端 SHALL 返回 400，并给出不可用原因。启动配置摘要与完整配置 SHALL 提供只读字段 `hybridSupport`，内容为 `{supported, agent?, reason?}`，供设置页决定勾选框是否可用及显示原因。

#### Scenario: 对 shell 配置勾选

- **WHEN** 客户端保存 `{name: "Shell", cmd: ["/bin/bash","-l"], hybrid: true}`
- **THEN** 服务端返回 400，原因为未识别到支持的 agent

### Requirement: 版本提示

agent 的版本来自终端信号或 ACP `initialize` 返回的 `agentInfo.version`。版本低于档案记录的实测版本（OpenCode 2.1.4、omp 18.13.1）时，系统 SHALL 在会话页显示提示，SHALL NOT 因此阻止切换；能否切换只由资格判定决定。版本无法解析时 SHALL 不提示。

#### Scenario: 旧版本

- **WHEN** ACP `initialize` 返回 omp 版本 18.0.0
- **THEN** 聊天视图显示“低于已验证版本 18.13.1”的提示，其余功能照常
