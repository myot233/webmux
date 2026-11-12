# Spec Delta

## ADDED Requirements

### Requirement: 聊天回合完成提醒遵守启动配置提醒策略

当混合会话处于聊天端（`activeSurface == "chat"`）且一个 ACP 回合以 `stopReason == "end_turn"` 结束（且无运行中的后台终端任务）时，服务端 SHALL 按照该会话启动配置绑定的 `attention` 策略决定是否发出以及何时发出 `done` 提醒：

- 若 `mode` 为 `"urgent"` 或 `"off"`，服务端 MUST 不产生回合完成的 `done` 提醒；
- 若 `minTurnSeconds > 0` 且当前聊天回合耗时短于 `minTurnSeconds`，服务端 MUST 忽略本次 `done` 提醒；
- 若 `doneDelaySeconds > 0`，服务端 MUST 等待 `doneDelaySeconds` 秒且期间未发起新回合、未切换到终端或产生新的后台任务后才发布 `done` 提醒；
- 聊天端的权限审批（`permission`）、提问（`question`）与回合错误（`error`）在 `mode != "off"` 时 MUST 立即发布提醒。

#### Scenario: 仅需介入模式下聊天回合完成不发提醒但权限审批照常提醒

- **WHEN** 混合会话的启动配置设置 `attention.mode = "urgent"`，且当前活跃端为聊天端
- **AND** Agent 在回合中发起 `session/request_permission`
- **THEN** 会话立即发布 `permission` 提醒
- **WHEN** 用户批准后该回合以 `end_turn` 正常结束
- **THEN** 聊天流正常写入 `turn_end` 事件，但会话不发布 `done` 提醒

#### Scenario: 聊天回合耗时低于最短耗时阈值时不触发完成提醒

- **WHEN** 混合会话的启动配置设置 `attention.minTurnSeconds = 15`，用户发送一条简短消息后 Agent 在 4 秒内以 `end_turn` 完成回复
- **THEN** 聊天界面正常显示完整回复与回合结束状态，但服务端不发布 `done` 提醒

#### Scenario: 聊天回合完成确认延迟期间发起新回合时取消提醒

- **WHEN** 混合会话的启动配置设置 `attention.doneDelaySeconds = 5`，回合以 `end_turn` 结束
- **AND** 在 5 秒延迟窗口内用户发送了下一条提示词或会话启动了新的后台任务
- **THEN** 服务端取消挂起的 `done` 提醒，不在旧回合上发出通知
