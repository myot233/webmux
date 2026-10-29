# acp-agent-connection Specification

## Purpose
规定 webmux 作为 ACP 客户端与 agent 子进程交互的协议边界：只使用 v1 稳定能力，声明哪些客户端能力，如何处理错误、取消和异常输出，以及不记录哪些内容。这样协议细节可以被测试，agent 行为异常时也不会影响服务或泄露内容。

## Requirements

### Requirement: 版本与能力协商

系统 SHALL 以 ACP 协议版本 1 调用 `initialize`。agent 返回其他版本时，系统 SHALL 结束该进程，并以“协议版本不受支持”结束当前操作。

客户端能力 SHALL 声明 `session.notices`、`session.compaction` 与 `elicitation.form`，SHALL NOT 声明 `fs` 与 `terminal` 能力。agent 调用 `fs/*`、`terminal/*` 或其他未声明的客户端方法时，系统 SHALL 返回 JSON-RPC 方法不存在错误。

系统 SHALL 根据 agent 返回的能力决定使用 `session/load`、`session/resume`、`session/list`、`session/close`。agent 没有声明 `loadSession` 或 `session/list` 时，该 agent SHALL 视为不支持混合模式，并给出原因。

#### Scenario: agent 请求读文件

- **WHEN** agent 在回合中调用 `fs/read_text_file`
- **THEN** 系统返回方法不存在错误，回合继续，服务端不读取任何文件

### Requirement: 使用的方法子集

系统 SHALL 只调用以下 agent 方法：`initialize`、`session/new`、`session/load`、`session/resume`、`session/list`、`session/close`、`session/prompt`、`session/cancel`、`session/set_config_option`，以及档案声明且 agent 在 `_meta` 中声明支持的私有扩展方法。

系统 SHALL 处理以下来自 agent 的方法：`session/update`、`session/request_permission`、`elicitation/create`。未知的 `session/update` 种类与未知通知 SHALL 被忽略，不中断连接。

#### Scenario: 未知更新种类

- **WHEN** agent 发送 `sessionUpdate` 为未知值的通知
- **THEN** 系统忽略该通知，后续消息照常处理

### Requirement: 传输的健壮性

agent 的 stdout SHALL 只被解析为按行分隔的 JSON-RPC 消息。

- 单行长度上限为 32 MiB。超出上限，或出现无法解析为 JSON-RPC 的行时，系统 SHALL 视为协议错误：结束该进程，并按 `hybrid-sessions` 的异常退出规则处理。
- agent 的 stderr SHALL 被持续读取，保留最近 64 KiB，只在启动或切换失败时作为错误详情返回给发起请求的用户。
- 进程退出时，所有挂起的请求 SHALL 立即失败。

#### Scenario: stdout 输出了非协议内容

- **WHEN** agent 进程向 stdout 写入一行纯文本
- **THEN** 连接以协议错误结束，聊天视图显示连接中断，服务端日志不包含这行文本

### Requirement: 取消与挂起请求

取消回合时，系统 SHALL 发送 `session/cancel`，并把所有挂起的权限请求答复为 `cancelled`，把挂起的表单请求答复为 `cancel`。取消之后到达的更新 SHALL 照常处理。

除 `session/prompt` 外的请求超过各自的期限时，系统 SHALL 发送 `$/cancel_request`，并让该请求以超时失败。期限为：`initialize` 30 秒，`session/load` 120 秒，其他 30 秒。

#### Scenario: 取消时有待批准的请求

- **WHEN** 回合中有一个待批准的权限请求，用户点击停止
- **THEN** agent 收到该请求的 `cancelled` 答复与 `session/cancel`，聊天视图中该请求显示为已取消

### Requirement: 日志与内容保护

服务端日志 SHALL NOT 包含 agent 的 stdout/stderr 内容、消息正文、工具输入输出、权限选项文本、表单内容或用户提示词。日志 SHALL 只记录方法名、错误码、计数与耗时。

#### Scenario: 协议错误的日志

- **WHEN** agent 返回包含用户代码片段的错误消息
- **THEN** 服务端日志只记录方法名与错误码，不包含该代码片段
