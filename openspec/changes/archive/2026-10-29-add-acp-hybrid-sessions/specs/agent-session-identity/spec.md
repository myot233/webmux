## Purpose

混合会话在 TUI 端时，webmux 需要知道 TUI 当前打开的是哪个 agent 会话，才能在切换时加载正确的对话。本能力从终端信号中识别 agent 身份、当前 agent 会话 ID 与 cwd，判定能否切到聊天视图，并防止同一个 agent 会话被两处同时打开。

## ADDED Requirements

### Requirement: 从终端信号中提取身份

系统 SHALL 在所有会话的实时 PTY 输出上，额外提取以下标识字段：

- Warp cli-agent 事件中的 `agent`、`plugin_version`、`session_id`、`cwd`；
- OSC 7501 根记录中的 `app`；
- OSC 7501 子记录中 `id` 的第一段。

提取 SHALL 满足以下限制：

- 字段长度 SHALL 有上限：ID 与名称不超过 128 字节，cwd 不超过 4096 字节。
- 字段 SHALL 只接受可打印字符，不合法的字段 SHALL 被忽略。
- 提取 SHALL NOT 改变现有的关注、状态与 OSC 7501 根记录语义，也 SHALL NOT 修改转发给客户端的字节。

#### Scenario: omp 启动即带出 ID

- **WHEN** 混合会话中的 omp 发出 `session_start` 事件，带有 `session_id` 与 `cwd`
- **THEN** 会话信息中的 `hybrid.agentSessionId` 更新为该 ID，现有关注与状态不变

### Requirement: 身份字段的可见范围与根进程信息

这些字段 SHALL 只保存在服务端内存中。它们 SHALL NOT 出现在普通会话的会话信息、日志或推送中；对混合会话，只有经档案解释后的 agent 名与 agent 会话 ID 出现在 `hybrid` 对象中。会话退出时 SHALL 清空这些字段。

系统 SHALL 另外记录会话 PTY 根进程的可执行文件名与当前工作目录（Linux 读取 `/proc`，macOS 使用系统进程信息），供资格判定使用。

#### Scenario: 普通会话不泄露

- **WHEN** 普通 shell 会话中运行 omp 并发出带 `session_id` 的事件
- **THEN** 该会话的 REST 与 WebSocket 负载中不出现该 ID

### Requirement: 跟踪 agent 会话 ID 的变化

agent 会话 ID 发生变化（例如在 TUI 中执行 `/new`、`/resume`、`/fork`）时，系统 SHALL：

- 更新记录中的当前 ID，并把旧 ID 加入历史（至多 8 个）；
- 在会话页显示提示“TUI 已切换到新会话 `<新 ID 前缀>`（上一个：`<旧 ID 前缀>`）”，用户可以关闭该提示。

#### Scenario: 执行 /new 后切换

- **WHEN** 用户在 TUI 中执行 `/new` 并对话一轮，然后切到聊天
- **THEN** 切换前页面已显示 ID 变化的提示，聊天视图加载的是新会话

### Requirement: 切到聊天的资格

会话在 `tui` 端时，只有同时满足以下五个条件，切到聊天的操作才 SHALL 可用：

1. 终端信号识别出的 agent 有对应的档案；
2. 该 agent 进程是会话 PTY 的根进程；
3. 已得到当前 agent 会话 ID；
4. 用正确的 cwd 调用 `session/list` 能查到这个 ID。cwd 的来源由档案决定（omp 取事件中的 `cwd`，OpenCode 取根进程的 cwd）；
5. 这个 ID 没有被其他会话占用。

不满足时，界面 SHALL 禁用该操作并显示对应原因，依次为：

- “未识别到支持的 Agent”；
- “Agent 不是由本会话直接启动的”；
- “请先在 TUI 中发送一条消息”；
- “会话尚未保存”或“找不到会话”；
- “该会话已在另一个会话中打开”。

条件 4 SHALL 在 ID 或 cwd 变化后核对一次，并在执行切换前再核对一次；它 SHALL NOT 在每个信号到来时都执行。核对 SHALL 只使用 `session/list`，SHALL NOT 加载或恢复该会话。

#### Scenario: 首条消息之前

- **WHEN** 从终端开始的 OpenCode 混合会话还没有发送过消息
- **THEN** “聊天”按钮不可用，提示“请先在 TUI 中发送一条消息”

#### Scenario: 在 shell 中手动启动

- **WHEN** 混合配置的 TUI 退出后，用户在同一 PTY 中又通过其他程序启动了 omp（omp 不是根进程）
- **THEN** “聊天”按钮不可用，提示 Agent 不是由本会话直接启动的

### Requirement: 跨会话占用检测

判定条件 5 时，系统 SHALL 检查所有 webmux 会话的身份字段（包括普通会话）以及所有活动混合会话的记录。只要有其他会话报出相同的 agent 会话 ID，就 SHALL 视为占用。

在 webmux 之外的终端中打开同一 agent 会话无法检测，用户文档 SHALL 说明这一风险。

#### Scenario: 普通会话中恢复了同一会话

- **WHEN** 用户在一个普通 PTY 会话中执行 `omp --resume <id>`，该 ID 正被一个混合会话使用
- **THEN** 该混合会话的“聊天”按钮不可用，提示“该会话已在另一个会话中打开”
