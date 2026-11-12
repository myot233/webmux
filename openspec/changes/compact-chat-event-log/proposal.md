## Why

聊天事件日志与终端缓冲共用 `scrollback_bytes`（默认 2 MiB），但两者装进去的“有效内容”并不对等：

- 终端缓冲几乎全是用户会读到的字节；聊天日志按编码后的 JSON 估算计量，每个流式块都带 `seq`、`kind`、`at`、`messageId` 等包装字段，而且 tool 的原始输出（整文件读取、长命令输出、diff）会和对话争抢同一份预算。
- 用 `replaysize` 脚本（合成会话，文本取自本仓库真实的 Go 源码与中文文档，流式块 3–24 字符）量得 100 轮会话约 2.5–2.9 MiB 的 JSON：
  - 真正的文本载荷只占 34%–43%，其余是事件包装与转义。
  - tool 输出文本与 diff 占 26%–36%；回复、思考、用户提示占 53%–62%（其中大半是流式块的包装）。
  - 在 2 MiB 估算预算内，400 轮会话只能保留最新 57–66 轮；若把 tool 输出限制到每块 2 KiB 可保留 76 轮，完全丢弃输出则为 91 轮。
- 用户翻看旧历史时很少需要旧 tool 的原始输出；标题、状态与位置已足以回顾过程。更早的原始输出还可以在终端或 agent 自己的历史中查看。
- 同一份日志也是重连时的重放内容：满载一次重放压缩后约 0.4–0.45 MiB（permessage-deflate，已默认对手机启用）。预算利用率提高后，同样的内存与同样的流量能装下更多对话。

本变更提高聊天日志的内容密度，不改变 `scrollback_bytes` 的含义与默认值。

## What Changes

- **旧 tool 输出降级**：日志超出预算时，先把较旧事件中 tool 的文本、diff 内容缩减到每块至多 2 KiB 并标记 `truncated`，保留标题、状态、类型与位置；只有仍超出预算时才按现状丢弃最早的事件。最新一段日志（预算的四分之一）不降级。未超预算的短会话行为不变。
- **相邻同消息块合并存储**：同一条消息（相同 `messageId` 与角色，无附件）的连续 `message.append` 块在消息结束后合并为一条存储事件，序号取该组最后一个。新订阅者重放时只收到合并后的事件；断线时恰好落在组内的客户端只补发其未收到的文字，不重复、不闪烁。实时推送仍逐块发送。
- 线上 JSON 字段不变：`seq`、`kind`、`message`、`tool.content[].truncated` 都已存在，`web/src/contracts/schemas.ts` 与 `internal/contract` 无需修改，前端 `chat-model` 不变。
- 不做：首屏只发最近一段并按需向前加载（需要新增协议，另行提出）；终端缓冲；压缩级别调整；修改 `scrollback_bytes` 默认值；丢弃 tool 的标题、状态与位置。

## Capabilities

### New Capabilities

无。

### Modified Capabilities

- `chat-view`：修改“聊天事件续接与历史上限”，新增“旧工具输出降级”“相邻消息块合并存储”两条要求。

## Impact

- 后端：`internal/hybrid/eventlog.go`（降级、合并、`from` 与续接计算）及 `eventlog_test.go`、`chat_test.go`；`eventSize` 对合并事件按合并后的文本计量。无新增依赖、路由、配置项或持久化。
- 契约与前端：无字段变化，`internal/api/testdata/wire` 快照不需要重录；仅需确认 `web/src/domains/chat/chat-model.ts` 对“组内补发剩余文字”的处理已由现有 `seq` 去重规则覆盖。
- 文档：`docs/user-stories/hybrid-sessions.md` 的 US-Q05（断线重连只补发缺失事件、旧事件丢弃的提示）；`docs/protocol/http-hybrid.md` 中聊天续接说明（如有）。
- 不涉及：会话生命周期、持久化迁移（日志只在内存中）、终端回放、认证与 Origin 校验。
- 风险：续接计算是协议正确性的核心，必须有边界测试；降级使旧 tool 输出在新订阅者看到的历史中变短，实时订阅者仍保留已收到的完整内容。
