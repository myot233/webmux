## ADDED Requirements

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
