## MODIFIED Requirements

### Requirement: 结构化事件解析的安全边界

系统 SHALL 只在负载为合法 JSON 对象、`v` 为不小于 1 的整数、`event` 为字符串时处理结构化事件；否则 SHALL 忽略该序列，且 SHALL NOT 将其降级为普通通知。

系统 SHALL 只使用 `event` 字段决定关注。此外，系统 SHALL 可以按 `agent-session-identity` 的规则，在服务端内存中提取有界、校验过的标识字段 `agent`、`plugin_version`、`session_id` 与 `cwd`，用于混合会话的身份跟踪与占用检测；这些字段 SHALL NOT 影响关注与状态，SHALL NOT 写入日志，也 SHALL NOT 出现在普通会话的任何 API 负载中。

`query`、`response`、`summary`、`tool_input` 等内容，以及普通通知的标题与正文，SHALL NOT 被存储、写入日志或通过任何 API 下发。

#### Scenario: 非法 JSON

- **WHEN** 程序输出 `OSC 777;notify;warp://cli-agent;{not json` 并以 BEL 结束
- **THEN** 不产生任何关注，也不把结束符 BEL 计为裸 BEL

#### Scenario: 不泄露正文

- **WHEN** omp 发出包含用户提问原文的 `stop` 事件
- **THEN** 会话信息中只出现 `attention.kind = done` 与 `source = agent`，任何 REST、WebSocket 负载和服务端日志中都不出现该原文

#### Scenario: 标识字段只供混合会话使用

- **WHEN** 普通会话中的 omp 发出带 `session_id` 与 `cwd` 的 `session_start` 事件
- **THEN** 该会话的关注按既有规则被清除，会话信息中不出现 `session_id` 或 `cwd`

## ADDED Requirements

### Requirement: 聊天端的关注来源

混合会话在聊天端时，关注与状态 SHALL 由聊天端的回合状态决定（映射见 `chat-view` 的“聊天端的关注映射”），来源为 `agent`，并 SHALL 沿用既有的锁存与清除语义。

切换到终端端后，关注 SHALL 被清除，并重新由终端信号决定；切换到聊天端时，终端信号留下的关注与 OSC 7501 根记录 SHALL 被清除。

#### Scenario: 切走后不残留

- **WHEN** 聊天端有一个“等待授权”的关注，用户取消回合并切到终端
- **THEN** 会话的关注被清除，之后只由 TUI 的终端信号决定
