# hybrid-session-recovery Specification

## Purpose
混合会话的价值在于延续同一段 agent 对话。webmux 只持久化 webmux 会话与 agent 会话之间的对应关系；服务重启、进程中断或用户主动关闭之后，用户可以把它恢复为混合会话，恢复到任一端。完整历史由 agent 自己保存。

## Requirements

### Requirement: 混合会话记录的内容

系统 SHALL 为每个混合会话保存一条记录，内容包括：

- webmux 会话 ID、名称、启动配置 ID、档案名、cwd、argv 快照；
- 所在端、当前 agent 会话 ID、最近至多 8 个被替换下来的 agent 会话 ID；
- 最近一次的配置项取值；
- 状态（`active`、`closed`、`interrupted`）、中断前的状态，以及当时队列中的条数（只记条数，不记内容）；
- 创建、更新、中断时间；
- 根进程的 PID 与进程启动标识。

记录 SHALL NOT 包含环境变量值、消息内容、队列、token 或终端输出。

#### Scenario: 记录不含密钥

- **WHEN** 混合配置的环境变量含 `API_KEY=secret-value`，用户创建并使用混合会话
- **THEN** `hybrid-sessions.json` 中任何位置都不出现 `secret-value`

### Requirement: 混合会话记录的存储

记录 SHALL 保存在 `<state_dir>/hybrid-sessions.json`，使用独立的文件锁，权限为 0600，并按现有状态文件规则原子写入，登记到持久化清单。文件不可用（锁冲突、损坏或不可读写）时，系统 SHALL 只禁用混合会话的创建与恢复，相关接口返回 503 并附状态文件路径；普通会话与其他功能 SHALL 照常可用，原文件 SHALL 保留不变。

记录最多 200 条。达到上限时，创建混合会话 SHALL 返回 409，并提示先移除旧记录。

#### Scenario: 记录文件损坏

- **WHEN** `hybrid-sessions.json` 内容损坏后服务启动
- **THEN** 混合会话的创建与恢复返回 503，普通会话照常创建，损坏的文件保持原样

### Requirement: 重启与中断后的状态

服务启动时，系统 SHALL 把所有 `active` 记录改为 `interrupted`。改之前 SHALL 按记录中的 PID 与启动标识检查进程是否残留，两者都匹配时 SHALL 结束其进程组。

在会话运行期间，以下情况 SHALL 把记录改为 `interrupted`：TUI 自行退出、ACP 重连失败、切换回滚失败。用户关闭 SHALL 把记录改为 `closed`。

普通 PTY 会话 SHALL 仍不跨服务重启。

#### Scenario: 服务重启

- **WHEN** 一个混合会话在聊天端时服务重启
- **THEN** 重启后它不在运行中列表，而出现在“可恢复”分组中，标注“已中断”，并记录中断前在聊天端

### Requirement: 恢复到任一端

`POST /api/sessions/{id}/resume {side}` SHALL 只对 `closed` 或 `interrupted` 的记录可用，`side` 取 `tui` 或 `acp`，缺省为记录中的所在端。恢复时，系统 SHALL 按以下步骤执行：

1. 用记录的 cwd 调用 `session/list`，确认 agent 会话 ID 仍然存在。不存在时，SHALL 返回 409，并附上历史 ID 与 `session/list` 中同一 cwd 下的候选，由用户选择后重试或放弃。
2. 以记录中的 webmux 会话 ID 重新创建会话外壳。
3. 恢复到聊天端时，启动 ACP 并 `session/load`；恢复到终端端时，在 PTY 中执行档案生成的恢复命令。
4. 重新应用记录中的配置项取值。

恢复后的会话 SHALL 仍是同一个混合会话，可以继续切换。

#### Scenario: 恢复到终端

- **WHEN** 用户在“可恢复”分组中对一条已中断的记录选择“恢复到终端”
- **THEN** 会话以原 ID 出现在运行中列表，处于 `tui` 端，TUI 显示此前的对话

#### Scenario: agent 会话已被删除

- **WHEN** 记录中的 agent 会话 ID 已在 agent 中被删除，用户选择恢复
- **THEN** 服务端返回 409，界面列出可选的候选会话，用户也可以放弃恢复

### Requirement: 启动配置被修改或删除

argv SHALL 取自记录中的快照；环境变量 SHALL 在恢复时按记录的启动配置 ID 重新读取。

配置已被删除时，恢复请求 SHALL 返回 409 `config_missing`。客户端以 `{confirmWithoutEnv: true}` 再次提交后，系统 SHALL 不带配置中的环境变量完成恢复。界面 SHALL 提示“将不带配置中的环境变量”。

#### Scenario: 配置已删除

- **WHEN** 原启动配置已被删除，用户选择恢复到聊天
- **THEN** 界面先提示将不带环境变量并请求确认，确认后完成恢复

### Requirement: 移除记录

`DELETE /api/hybrid-sessions/{id}` SHALL 只对 `closed` 或 `interrupted` 的记录可用，对活动会话 SHALL 返回 409。移除 SHALL 删除记录，SHALL NOT 删除 agent 的会话库。移除后，该记录 SHALL NOT 再出现在任何列表中。

#### Scenario: 移除

- **WHEN** 用户对一条已关闭的记录选择“移除”并确认
- **THEN** 记录从“可恢复”分组消失；在 agent 的 TUI 中仍可以用其自身的恢复参数打开该会话

### Requirement: 可恢复记录的列表

`GET /api/hybrid-sessions?state=recoverable` SHALL 返回所有 `closed` 与 `interrupted` 的记录摘要，按更新时间倒序排列。每条摘要包含：

- ID、名称、档案、cwd、所在端、状态、更新时间与中断时间；
- 中断前的状态，用于提示“上一轮可能未完成”；
- 启动配置是否仍存在。

摘要 SHALL NOT 包含环境变量名或值。

首页 SHALL 在运行中的会话下方显示“可恢复”分组，默认折叠并显示数量。每张卡片 SHALL 提供“恢复到聊天 / 恢复到终端 / 移除”三个操作。会话列表事件涉及混合会话时，首页 SHALL 重新获取该列表。

#### Scenario: 首页分组

- **WHEN** 有一条已关闭和一条已中断的记录
- **THEN** 首页“可恢复”分组折叠显示数量 2，展开后两张卡片分别标注“已关闭”和“已中断”，并各有三个操作
