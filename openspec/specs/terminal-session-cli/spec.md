# terminal-session-cli Specification

## Purpose
让用户在本机或 SSH 终端中用 `webmux run|attach|ls|open` 创建、附着、列出和打开持久会话：离开时分离并把会话交给手机等其他客户端，回来后从原生终端接续同一个 PTY。

## Requirements

### Requirement: 从终端创建会话

`webmux run [--launch-config <名称>] [--name <名称>] [--cwd <目录>] [--env NAME=VALUE]... [--detach] -- <argv...>` SHALL 在本机 WebMux 中创建会话。未指定 `--launch-config` 时，`--` 之后的参数 SHALL 原样作为自定义命令 argv，SHALL NOT 经过 shell 展开。

指定 `--launch-config`（`--config-id` 为兼容别名）时，CLI SHALL 读取 `GET /api/launch-configs` 摘要（只含环境变量名，不含值），按区分大小写的精确名称匹配唯一的 Web 端启动配置，并以其 ID 作为 `configId` 发送；服务端 SHALL 按该 ID 解析完整 argv 与环境变量，CLI SHALL NOT 读取或复制环境变量值。

CLI SHALL NOT 把参数当作配置 ID 接受。名称不存在时 SHALL 不创建会话，在标准错误列出已保存的名称（没有任何配置时说明没有已保存配置）并以退出码 2 结束。两种模式 SHALL 互斥：启动配置名称不得与 `--` 后命令或 `--env` 同时提供，空名称 SHALL 以退出码 2 拒绝。

未指定 `--cwd` 时 SHALL 使用调用者的当前目录；`--cwd` 为相对路径时 SHALL 相对当前目录解析为绝对路径，以 `~` 开头时 SHALL 交由服务端展开。自定义命令模式缺少命令时 SHALL 输出用法并以退出码 2 结束。

`--detach` 时 SHALL 只向标准输出写入一行 `<会话 ID>\t<本机会话链接>` 并以退出码 0 结束。默认 SHALL 立即附着该会话；附着前 SHALL 在标准错误输出会话 ID、本机会话链接与分离键提示，SHALL NOT 向标准输出写入额外内容。

本机会话链接 SHALL 为服务报告的本机链接加 `/s/<会话 ID>`，SHALL NOT 包含 Token。

#### Scenario: 参数不被 shell 展开

- **WHEN** 用户执行 `webmux run --detach -- printf '%s;$(id)' 'a b'`
- **THEN** 会话 argv 为 `["printf", "%s;$(id)", "a b"]`，`$(id)` 不被执行，标准输出为会话 ID 与链接

#### Scenario: 附着前告知链接

- **WHEN** 用户在 `/home/u/proj` 执行 `webmux run -- claude`
- **THEN** 会话以 cwd `/home/u/proj` 创建，标准错误先出现会话 ID 与 `http://127.0.0.1:7681/s/<ID>`，随后终端附着到该会话

#### Scenario: 使用 Web 端保存的启动配置

- **WHEN** Web 端已保存名称为 `Claude Code`、ID 为 `k3m9q2x7` 的启动配置，用户执行 `webmux run --launch-config 'Claude Code' --cwd /home/u/proj --detach`
- **THEN** 客户端先读取启动配置摘要，再向 `POST /api/sessions` 发送 `configId: "k3m9q2x7"`、cwd 与 detach 所需的创建请求，不发送 `cmd` 或 `env`；服务端按该 ID 保存的完整 argv 与环境变量创建会话

#### Scenario: 启动配置名称不存在

- **WHEN** 已保存的配置名称为 `Shell` 与 `omp`，用户执行 `webmux run --launch-config nope --detach`，或以配置 ID `k3m9q2x7` 作为名称
- **THEN** 客户端不创建会话，标准错误列出 `"Shell", "omp"`，退出码为 2

#### Scenario: 启动配置互斥

- **WHEN** 用户执行 `webmux run --launch-config omp --env API_URL=https://example.test -- true`
- **THEN** 客户端输出用法并以退出码 2 结束，且在创建任何会话前拒绝参数

### Requirement: 附着会话

`webmux attach <会话 ID> [--observe]` SHALL 只在标准输入与标准输出都是终端时附着，否则 SHALL 以退出码 2 结束。附着期间终端 SHALL 处于原始模式：键盘输入（含 `Ctrl+C`）原样发送给会话，会话输出原样写入终端；附着开始时 SHALL 先复位终端再回放会话缓冲。

附着 SHALL 默认取得会话控制权，并在终端尺寸变化后 250 毫秒内同步；`--observe` 时 SHALL NOT 发送输入、尺寸或取得控制权。浏览器与原生终端 SHALL 遵循同一套控制权规则：任一端输入即取得控制权。

按下 `Ctrl+]` 时 SHALL 只断开连接、不把该按键发送给会话，以退出码 0 结束；会话进程退出时 SHALL 以会话退出码结束；连接中断或会话不存在时 SHALL 提示原因并以退出码 1 结束。任一结束方式（含 SIGINT/SIGTERM/SIGHUP）SHALL 先恢复终端原有模式。

#### Scenario: 手机接管后回到原生终端

- **WHEN** 用户在原生终端附着运行 agent 的会话，按 `Ctrl+]` 分离，在手机浏览器中打开该会话并输入，然后再次执行 `webmux attach <ID>`
- **THEN** 三次操作作用于同一进程；重新附着后原生终端显示最新内容，输入即取得控制权，终端尺寸按原生终端更新

#### Scenario: 退出码透传

- **WHEN** 附着的会话进程以退出码 7 结束
- **THEN** 终端模式恢复，`webmux` 以退出码 7 结束

### Requirement: 列出与打开会话

`webmux ls` SHALL 以表格列出会话（ID、名称、状态与退出码、命令、启动目录）；`--json` 时 SHALL 向标准输出写入服务端会话列表的原始 JSON 数组。`webmux open <会话 ID>` SHALL 输出该会话的本机会话链接，`--browser` 时 SHALL 同时用默认浏览器打开；会话不存在时 SHALL 以退出码 1 结束。

#### Scenario: 机器可读列表

- **WHEN** 用户执行 `webmux ls --json`
- **THEN** 标准输出恰好是服务端会话列表 JSON，可直接交给 `jq`

### Requirement: 连接与错误约定

这些命令 SHALL 经本机套接字访问本机服务，SHALL NOT 需要或输出 Token，并 SHALL 接受 `--config <path>` 以与服务相同的规则定位服务。服务未运行时 SHALL 以退出码 3 结束并提示启动方式，SHALL NOT 隐式启动服务。诊断信息 SHALL 只写标准错误。

#### Scenario: 服务未运行

- **WHEN** 没有 WebMux 运行，用户执行 `webmux run -- claude`
- **THEN** 不创建任何进程，标准错误提示服务未运行与启动方式，退出码 3

### Requirement: 用混合配置从终端创建会话

`webmux run --launch-config <名称>` 引用勾选了混合模式的配置时，SHALL 创建混合会话，起始端固定为终端。行为 SHALL 与 Web 端创建的完全相同：写入混合会话记录，按资格判定决定能否切到聊天。

未指定 `--detach` 时，本地终端 SHALL 像普通会话一样附着，可以输入。本地终端 SHALL NOT 提供切换命令；切换只能在 Web 端发起。

`webmux run -- <argv…>` SHALL 始终创建普通会话，即使命令是 omp 或 opencode。

#### Scenario: 从终端启动混合配置

- **WHEN** 用户执行 `webmux run --launch-config omp-hybrid`
- **THEN** 本地终端附着并运行 omp；Web 端列表中该会话带有终端端标记，可以在资格满足后切到聊天

#### Scenario: 直接命令不是混合会话

- **WHEN** 用户执行 `webmux run -- omp`
- **THEN** 创建普通会话，Web 端不显示聊天切换

### Requirement: 混合会话的附着规则

`webmux attach <ID>`（包括 `--observe`）作用于混合会话时：

- 会话在终端端：SHALL 与普通会话相同，可以附着、输入，控制权规则不变。
- 会话在聊天端、休眠中或切换中：SHALL 拒绝附着，不进入原始模式，在标准错误提示“会话在聊天视图，请在 Web 端操作，或切回终端后重新附着”，并以退出码 1 结束，与连接中断等无法附着的情况一致。

已附着的本地终端在会话切到聊天端时，SHALL 视为被动分离：

- 恢复终端模式；
- 在标准错误提示“已切换到聊天视图，可在 Web 端继续；切回终端后用 `webmux attach <ID>` 重新接入”；
- 以退出码 0 结束，与按分离键相同。会话仍然存活，SHALL NOT 透传任何进程退出码。

会话从聊天端切回终端时，SHALL NOT 自动重新附着任何本地终端。

#### Scenario: 切走时断开

- **WHEN** 用户在本地终端附着一个终端端的混合会话，随后在手机上切到聊天
- **THEN** 本地终端恢复原模式，显示提示后以退出码 0 结束；`webmux run --launch-config` 启动的附着同样如此

#### Scenario: 聊天端拒绝附着

- **WHEN** 会话在聊天端，用户执行 `webmux attach <ID> --observe`
- **THEN** 不进入原始模式，标准错误显示提示，退出码为 1

### Requirement: 会话移到另一端的关闭码

服务端 SHALL 用 WebSocket 关闭码 4409 表示“会话已移到另一端”，关闭原因中 SHALL 包含当前所在端。4409 SHALL 写入协议文档。浏览器与本机客户端 SHALL 把 4409 视为终止状态，SHALL NOT 自动重连。

#### Scenario: 浏览器终端页收到 4409

- **WHEN** 浏览器终端页连接着一个混合会话，该会话在另一设备上切到聊天
- **THEN** 服务端以 4409 关闭终端 WebSocket，关闭原因包含所在端，浏览器不自动重连

### Requirement: 列表显示所在端

`webmux ls` 的表格 SHALL 为混合会话在状态列中附加所在端（`终端`、`聊天`、`聊天·休眠`、`切换中`）。`webmux ls --json` SHALL 原样包含服务端会话信息中的 `hybrid` 对象。普通会话的输出 SHALL 不变。

#### Scenario: 机器可读的所在端

- **WHEN** 用户执行 `webmux ls --json`，其中一个混合会话在聊天端
- **THEN** 该会话的 JSON 含 `hybrid.side = "acp"`，普通会话不含 `hybrid`
