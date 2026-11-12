# Spec Delta

## ADDED Requirements

### Requirement: 会话级提醒范围与信号源开关覆盖

当会话由包含自定义 `attention` 设置的启动配置创建时，服务端 SHALL 按该会话绑定的 `attention` 策略覆盖全局默认配置：

- `mode = "off"` 时 MUST 抑制所有终端与聊天回合自动产生的提醒，仅保留 `webmux notify` 命令行显式提醒；
- `mode = "urgent"` 时 MUST 仅允许 `question`、`permission`、`error` 三类阻塞介入提醒，并忽略 `done`、`notify`、`bell`；
- 信号来源开关 `programStatus`、`agentProtocol`、`notify`、`bell` MUST 分别控制该会话是否启用 `OSC 7501`、Warp `cli-agent`（含 `WARP_CLI_AGENT_PROTOCOL_VERSION=1` 注入）、`OSC 9`/`OSC 777` 桌面通知和 `BEL` 响铃；
- 已启用的信号源之间 MUST 保持 `OSC 7501` > Warp Agent 事件 > 桌面通知 > `BEL` 响铃的优先级覆盖规则。

#### Scenario: 仅需介入模式抑制已完成与通知响铃

- **WHEN** 会话的启动配置设置 `attention.mode = "urgent"`，且会话收到 `OSC 7501` 的 `state=done`、Warp `stop`/`idle_prompt`、`OSC 9` 桌面通知或 `BEL` 响铃
- **THEN** 会话更新运行状态（如从 `working` 切回 `idle`），但不记录 `done`、`notify` 或 `bell` 提醒，也不发送弹窗、系统通知或 Web Push
- **WHEN** 同一会话收到 `question`、`permission` 或 `error` 信号
- **THEN** 会话立即记录对应提醒并广播通知

#### Scenario: 关闭自动提醒模式仍保留命令行显式提醒

- **WHEN** 会话的启动配置设置 `attention.mode = "off"`
- **THEN** 终端输出中的 `OSC 7501`、Warp `cli-agent`、`OSC 9`/`OSC 777` 和 `BEL` 均不产生会话提醒
- **WHEN** 同一会话内执行 `webmux notify "构建已完成"`
- **THEN** 会话仍记录 `notify` 提醒并触发通知

#### Scenario: 按预设关闭 OSC 7501 后回退到下一优先级信号源

- **WHEN** 会话的启动配置设置 `programStatus = false` 且 `agentProtocol = true`
- **THEN** 会话忽略终端输出的 `OSC 7501` 状态序列，并由下一优先级的 Warp `cli-agent` 事件驱动会话提醒与状态变更

### Requirement: 完成类提醒的最短回合耗时与确认延迟防误报

对于 `done`、`notify`、`bell` 三类非紧急终端提醒信号（不包括 `webmux notify` 显式命令行提醒），服务端 SHALL 检查会话的 `minTurnSeconds` 与 `doneDelaySeconds` 配置：

- 若当前回合（自最近一次进入工作状态、用户提交输入或会话启动起算）持续时间短于 `minTurnSeconds`，服务端 MUST 忽略该完成提醒；
- 若设置了 `doneDelaySeconds > 0`，服务端 MUST 在等待 `doneDelaySeconds` 秒且期间会话未重新进入 `working` 状态、未收到新的工作事件或用户输入后才正式发布该提醒；
- `question`、`permission`、`error` 紧急提醒 MUST 始终立即发布。

#### Scenario: 短于最短回合耗时的完成信号被过滤

- **WHEN** 会话配置 `minTurnSeconds = 10`，用户在提交输入后 3 秒内 Agent 即输出 `done` 或 `stop`
- **THEN** 会话状态恢复为空闲，但不产生 `done` 提醒

#### Scenario: 完成确认延迟窗口内恢复工作时自动取消提醒

- **WHEN** 会话配置 `doneDelaySeconds = 5`，Agent 在某一步骤结束时发出 `done`，但在 3 秒后继续工作（例如上报 `OSC 7501` `state=working`、发出 Warp `tool_call` 或用户在终端输入）
- **THEN** 挂起的 `done` 提醒被取消，不会向用户发出误报通知
- **WHEN** Agent 发出最终 `done` 后在 5 秒窗口内保持空闲且无新输入
- **THEN** 延迟窗口到期后会话正式发布 `done` 提醒并触发通知

### Requirement: Warp Agent 事件主会话标识过滤子代理完成误报

当会话解析 Warp `cli-agent` 结构化事件时，服务端 SHALL 记录主会话的 `session_id`（在 `prompt_submit` 事件，或主会话尚未记录标识时的首个 `session_start` 事件上更新），并按以下规则过滤子代理事件：

- 当收到携带非空 `session_id` 且与当前已记录主会话 `session_id` 不一致的 `stop`、`idle_prompt`、`stop_failure` 或 `subagent_start` 事件时，服务端 MUST 忽略该事件，不清除工作状态也不触发 `done`/`error` 提醒；
- 子代理发出的 `question_asked` 与 `permission_request` 事件 MUST 正常触发提醒。

#### Scenario: 忽略子代理的 stop 事件避免提前报完成

- **WHEN** 主会话在 `prompt_submit` 中确立 `session_id = "root-1"` 并进入工作状态，随后子代理输出 `{"v":1,"agent":"omp","event":"stop","session_id":"sub-2"}`
- **THEN** 会话忽略该子代理的 `stop` 事件，保持 `working` 状态且不触发 `done` 提醒
- **WHEN** 主会话随后输出 `{"v":1,"agent":"omp","event":"stop","session_id":"root-1"}`
- **THEN** 会话正常结束工作状态并按配置触发 `done` 提醒

#### Scenario: 子代理请求权限或提问时仍正常提醒

- **WHEN** 主会话 `session_id = "root-1"` 正在运行，子代理输出 `{"v":1,"agent":"omp","event":"permission_request","session_id":"sub-2"}`
- **THEN** 会话立即触发 `permission` 提醒通知用户介入
