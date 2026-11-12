# Spec Delta

## ADDED Requirements

### Requirement: 会话级 OSC 7501 状态协议开关覆盖

服务端 SHALL 允许每个会话根据其启动配置的 `attention.programStatus` 字段独立启用或禁用 `OSC 7501` 状态协议（当 `attention` 为空时跟随服务端全局 `program_status` 配置）：

- 当某个会话禁用 `programStatus` 时，服务端 MUST 忽略该会话 PTY 输出中的所有 `OSC 7501` 状态上报、清除与查询序列（不对 `OSC 7501;?` 写回应答）；
- 禁用当前会话的 `programStatus` MUST 不影响其他已启用 `programStatus` 的会话。

#### Scenario: 启动配置禁用 programStatus 时忽略 OSC 7501 且不应答查询

- **WHEN** 全局 `program_status = true`，但当前会话使用的启动配置显式设置 `attention.programStatus = false`
- **AND** 会话内程序输出 `\x1b]7501;?\x07` 或 `\x1b]7501;state=done;msg=Finished\x07`
- **THEN** 服务端不向 PTY 写回 `OSC 7501;1` 应答，不记录 `OSC 7501` 状态槽位，也不阻塞同一会话的 Warp `cli-agent`、`OSC 9`/`OSC 777` 或 `BEL` 信号源

#### Scenario: 启动配置启用 programStatus 时可覆盖全局关闭设置

- **WHEN** 全局 `program_status = false`，但当前会话使用的启动配置显式设置 `attention.programStatus = true`
- **AND** 会话内程序输出 `\x1b]7501;?\x07`
- **THEN** 服务端向该会话 PTY 写回 `\x1b]7501;1\x07` 并正常解析该会话的 `OSC 7501` 状态序列
