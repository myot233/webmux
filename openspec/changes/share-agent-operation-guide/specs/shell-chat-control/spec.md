# Spec Delta

## ADDED Requirements

### Requirement: CLI 等待显式输出未完成工作

CLI 等待 SHALL 在现有 turnResult JSON 对象增加顶层 unfinished boolean，取自共享观察的真实标记，不从回复推断。
既有 stopReason、error、reply、events、request SHALL 保持原结构和语义，不增加伪事件或替换 envelope。
文本 SHALL 保留回复，unfinished 为 true 时在 stderr 提示后台工作未完成。
end_turn SHALL 仍退出 0，即使 unfinished 为 true；其他退出映射全部不变。unfinished 与独立业务验收 SHALL NOT 混为一谈。
命令入口 SHALL 依据当前命令表，不恢复旧 chat send/switch。

#### Scenario: 正常回合仍有后台工作

- **WHEN** 目标回合 end_turn 且 unfinished 为 true
- **THEN** JSON 顶层 unfinished 为 true，events 不变，文本 stderr 提示后台工作未完成，退出码仍为 0

#### Scenario: 正常结束没有后台工作

- **WHEN** 观察的 unfinished 为 false
- **THEN** JSON 仍显式输出 false，不根据任务是否经过验收改写，也不输出后台未完成警告

#### Scenario: 超时或待请求

- **WHEN** 等待超时或被当前 pending request 阻塞
- **THEN** 分别保持既有 124 或 1 退出映射，不取消执行、批准请求或重复发送操作
