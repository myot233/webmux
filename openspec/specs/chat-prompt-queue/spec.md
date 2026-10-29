# chat-prompt-queue Specification

## Purpose
agent 在回合进行中通常不能接受新的提示：OpenCode 会直接报错，omp 会打断当前回合。webmux 在服务端为每个混合会话维护一个多设备共享的发送队列，提供可预期的“排队”行为；agent 支持时，再额外提供“插话”。

## Requirements

### Requirement: 发送与排队

`POST /api/sessions/{id}/chat/prompts` SHALL 接受 `{clientRequestId, text, mode}`，`mode` 取 `auto` 或 `steer`，缺省为 `auto`。

- `auto` 模式：没有运行中的回合且队列未暂停时，SHALL 立即发送；否则 SHALL 追加到队尾。
- 队列中的条目 SHALL 在上一回合正常结束后自动发送，一次一条。

`text` 去除首尾空白后 SHALL 不为空，且不超过 64 KiB。会话不在聊天端时，请求 SHALL 返回 409；会话处于休眠时，SHALL 先唤醒再处理。

#### Scenario: 运行中继续发送

- **WHEN** 回合进行中，用户先后发送 A、B 两条消息
- **THEN** A、B 依次出现在队列中；当前回合结束后 A 自动发送，A 的回合结束后 B 自动发送

### Requirement: 请求去重

服务端 SHALL 按 `clientRequestId` 对发送、队列操作、取消、答复与配置变更去重，保留最近 256 条、30 分钟。重复的请求 SHALL 返回首次处理的结果，SHALL NOT 再次执行。

#### Scenario: 重试不重复发送

- **WHEN** 手机网络中断导致客户端以同一 `clientRequestId` 重新提交发送
- **THEN** 消息只发送一次，两次请求得到相同结果

### Requirement: 编辑队列

用户 SHALL 可以对队列中尚未发送的条目执行以下操作：

- 编辑文本；
- 删除；
- 上移一位；
- 立即发送：取消当前回合后立即发送该条，需要二次确认。

操作 SHALL 作用于服务端队列，所有设备 SHALL 看到相同的队列。条目已发送时，对它的操作 SHALL 返回 409。

手机上的队列 SHALL 默认折叠为一行，显示条数与第一条的摘要。

#### Scenario: 另一台设备删除

- **WHEN** 桌面删除了队列中的第一条
- **THEN** 手机上的队列随即少一条，手机对该条的编辑请求返回 409

### Requirement: 暂停与继续

回合以 `cancelled`、错误或 `refusal` 结束时，队列 SHALL 暂停，界面 SHALL 提示“继续 / 清空”。

- 选择“继续”：从队首开始发送。
- 选择“清空”：删除所有条目。

暂停期间新发送的 `auto` 消息 SHALL 追加到队尾，SHALL NOT 自动发送。

#### Scenario: 停止后暂停

- **WHEN** 队列中有两条消息时，用户点击停止
- **THEN** 当前回合以中断结束，队列暂停并提示继续或清空，两条消息都未发送

### Requirement: 插话

只有当档案声明了插话方法、agent 在初始化时声明支持、且有运行中的回合时，界面才 SHALL 显示“插话”。`steer` 模式的发送 SHALL 把文本注入当前回合，不进入队列。

不满足条件时，`steer` 请求 SHALL 返回 409，界面 SHALL NOT 显示插话入口。

#### Scenario: OpenCode 没有插话

- **WHEN** 用户在 OpenCode 的混合会话中有回合运行
- **THEN** 输入框旁只有发送与停止，没有插话按钮

### Requirement: 切到终端时退回草稿

会话从聊天端切到终端时，队列中尚未发送的条目 SHALL 按顺序以空行连接，合并成一段文本，退回到发起切换的设备的输入草稿中，然后清空队列。

- 其他设备 SHALL 收到“排队的消息已退回发起切换的设备的草稿”提示。
- 退回的文本 SHALL NOT 被自动发送到 TUI。

队列只保存在内存中。服务重启后队列丢失，恢复后的界面 SHALL 提示“排队的消息已丢失”；浏览器中的草稿不受影响。

#### Scenario: 切换时有排队

- **WHEN** 队列中有两条消息时用户切到终端
- **THEN** 发起切换的页面输入草稿中出现这两条消息的合并文本，TUI 没有收到任何输入
