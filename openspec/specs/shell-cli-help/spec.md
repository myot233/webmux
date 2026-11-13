# shell-cli-help Specification

## Purpose

让每个 `webmux` 命令都有完整、一致、可机器读取的帮助，并提供 shell 补全，使人和 agent 都能从命令本身学会用法。

## Requirements

### Requirement: 命令定义的单一来源

`webmux` 的每个命令与子命令 SHALL 在一张命令表中声明，表项包括：命令路径、分组、用法、一句话摘要、详细说明、位置参数、选项、
用到的环境变量、输出、退出码、示例，以及相关的 llms 主题与命令。覆盖范围包括现有命令（`serve`、`start`、`version`、`status`、`stop`、`config`、
`service`、`update`、`bridge`、`run`、`attach`、`ls`、`open`）和本变更新增的全部命令。

- 每个选项 SHALL 声明：长名、可选短名、值类型、默认值、是否可重复、说明，以及互斥或依赖关系。
- 会话类命令的选项只能经命令表声明（`Bind`），解析与帮助共用同一份声明。
- 运维命令（`serve`、`config`、`service`、`update` 等）保留各自的解析器；它们的 `--help` 由分发层统一按命令表输出，测试 SHALL 比对每个运维命令实现接受的选项与命令表一致。
- 除 `run` 等以其他程序 argv 结尾的命令外，选项 SHALL 可以写在位置参数之后；`--` 之后的参数 SHALL 原样作为位置参数。
- `--help` 输出、`help --json`、shell 补全，以及 llms 中的命令参考，SHALL 全部由这张表生成或经测试比对，不维护第二份手写副本。
- 共享命令放在 `internal/clicmd`，Linux/macOS CLI 与 Windows 控制台客户端 SHALL 得到相同的帮助。
  只属于某个平台的命令（例如 `service`）SHALL 只出现在该平台的帮助中。

#### Scenario: 新增选项忘记写说明

- **WHEN** 开发者给 `chat send` 新增一个选项，但没有写说明或默认值
- **THEN** 命令表一致性测试失败，并指出缺少的字段

### Requirement: 完整的 --help

每个命令与子命令 SHALL 提供 `--help`，内容依次包括以下各节，某节没有内容时省略：

1. **Usage**：完整用法，标出可选与可重复的部分；
2. **摘要与说明**：做什么，以及与相近命令的区别；
3. **Arguments**：每个位置参数的含义，包括 `self` 与仓库参数的写法；
4. **Options**：每个选项的类型、默认值与说明，互斥关系，以及全局选项 `--config` 和 `--socket`；
5. **Environment**：读取的环境变量（`WEBMUX_SOCKET`、`WEBMUX_SESSION_ID`、`WEBMUX_BIN` 等）；
6. **Output**：文本与 `--json` 的输出形式，以及对应的契约类型名；流式命令说明 NDJSON；
7. **Exit status**：该命令可能返回的退出码及含义；
8. **Examples**：至少两个示例，一个基本用法，一个脚本或 agent 用法；
9. **See also**：相关命令，以及 `webmux guide <主题>`。

帮助正文 SHALL 使用英文，与现有 CLI 一致；示例中的提示文字可以是中文。帮助 SHALL 按 80 列换行。

#### Scenario: 查看子命令帮助

- **WHEN** 用户执行 `webmux chat send --help`
- **THEN** 输出包含用法、全部选项及默认值、`--wait` 的退出码含义、至少两个示例，以及 `webmux guide chat`

### Requirement: 帮助的调用与输出约定

- `-h`、`--help`、`webmux help <命令> [子命令]` SHALL 等价：把帮助写到标准输出并以 0 退出，
  SHALL NOT 连接服务、读取配置或要求其他参数合法。
- 用法错误 SHALL 只在标准错误输出一行原因、用法行，以及 `Run 'webmux <命令> --help' for details.`，并以 2 退出。
  这修正了现有 `run`、`attach`、`ls`、`open` 把 `--help` 当作用法错误、写到标准错误并返回 2 的行为。
- 只给出分组命令而没有子命令时（例如 `webmux chat`），SHALL 把分组帮助（子命令列表与摘要）写到标准错误并以 2 退出。
- `webmux help` 与不带参数的 `webmux --help` SHALL 按分组列出全部命令：服务与运维、会话、启动、聊天、工作区、工具说明、帮助与补全。

#### Scenario: --help 不再是错误

- **WHEN** 用户执行 `webmux run --help; echo $?`
- **THEN** 帮助写到标准输出，`$?` 为 0，服务未运行也一样

#### Scenario: 用法错误给出指引

- **WHEN** 用户执行 `webmux send` 而不给任何参数
- **THEN** 标准错误给出原因、用法行和 `Run 'webmux send --help' for details.`，退出码 2

### Requirement: 机器可读的帮助

`webmux help --json [命令 [子命令]]` SHALL 把命令表输出为 JSON，供 agent 自动发现命令：

- 顶层为 `{version, commands}`；每个命令包含 `path`（数组）、`group`、`usage`、`summary`、`description`、`args`、
  `options`（`name`、`short`、`type`、`default`、`repeatable`、`description`、`conflicts`、`requires`）、`env`、`output`、`exitCodes`、`examples`、`seeAlso`。
- 给出命令路径时 SHALL 只输出该命令及其子命令。路径不存在时 SHALL 以 2 结束。
- 输出 SHALL 不连接服务，内容与同一可执行文件的 `--help` 一致。

#### Scenario: agent 发现 send 的选项

- **WHEN** agent 执行 `webmux help --json send | jq -r '.commands[0].options[].name'`
- **THEN** 输出 `enter`、`key`、`wait`、`timeout` 等选项名

### Requirement: shell 补全

`webmux completion bash|zsh|fish` SHALL 把对应 shell 的补全脚本写到标准输出，帮助中 SHALL 给出安装方法。

- 静态补全：命令、子命令、选项名，以及枚举值（状态、起始端、`--kind`、llms 主题、shell 名）。
- 动态补全由脚本调用隐藏命令 `webmux __complete <words...>` 完成：
  - 补全会话 ID（zsh 与 fish 附带标题作为说明）、启动配置名称、仓库、worktree、混合会话记录 ID 与转发端口；
  - 经本机 API 查询，超时 1 秒；服务未运行或超时时，只返回静态候选，SHALL NOT 输出错误；
  - SHALL NOT 输出 Token 或环境变量的值。
- `__complete` SHALL NOT 出现在帮助与 `help --json` 中。
- Windows 控制台客户端 SHALL 提供 `--help` 与 `help --json`，本变更不为 PowerShell 提供补全。

#### Scenario: 补全会话 ID

- **WHEN** 用户在 zsh 中输入 `webmux send ` 后按 Tab
- **THEN** 列出当前会话 ID，每项附带会话标题

#### Scenario: 服务未运行

- **WHEN** 服务未运行时，用户在 bash 中输入 `webmux chat se` 后按 Tab
- **THEN** 补全出 `send`、`set`，没有报错输出

### Requirement: llms 命令参考与帮助一致

llms 的 `sessions`、`launch`、`chat`、`workspace` 等主题中的命令参考 SHALL 由命令表生成，内容包括用法、摘要与选项。
各主题 SHALL 提示：用 `webmux <命令> --help` 查看完整说明，用 `webmux help --json` 获取机器可读版本。

#### Scenario: 帮助与手册同步

- **WHEN** 开发者修改 `wait` 的一个选项说明
- **THEN** `webmux wait --help`、`webmux help --json wait` 与 `/llms/sessions.md` 中的说明同时更新，无需另外编辑手册
