## 1. 度量与基线

- [x] 1.1 在 `internal/hybrid` 内加一个确定性的合成会话生成器（仅测试使用）：用固定种子生成对话、流式块（3–24 字符）、读文件 / 搜索 / 编辑 / 执行类 tool 调用与用量、回合结束事件；文本取自测试内联样本，不读取仓库文件（实现为 `eventlog_test.go` 中的内联合成会话，文本为内联样本）
- [x] 1.2 用它写基线测试：记录当前实现下 2 MiB 估算预算内能保留的轮数、文本载荷占估算大小的比例，并把结果写进测试注释，作为改动前后对比（基线为“只丢弃最早事件”的保留轮数，在 `TestChatLogOutputDoesNotCrowdOutConversation` 中现算，不记录固定数字）
- [ ] 1.3 用基线结果核对 proposal 中的数字（轮数 57–66、文本载荷 34%–43%）；若差异显著，先更新 proposal 与 design 的度量和阈值，再继续

## 2. 旧 tool 输出降级

- [x] 2.1 `internal/hybrid/eventlog.go`：实现保护窗口（日志尾部估算大小不超过 `capacity / 4`）与 2 KiB 内容块缩减（UTF-8 边界截断、置 `Truncated`），常量集中定义
- [x] 2.2 `append` 的驱逐流程改为：先从旧到新降级直到不再超出，仍超出时再丢弃最早事件；降级时构造新的 `Event` / `Tool` / `Content`，不改已存储对象，并同步更新 `sizes` 与 `bytes`
- [x] 2.3 单元测试：
  - 未超出预算时不改动任何内容；
  - 超出时只缩减窗口之外的内容，标题、状态、位置与 `Seq` 不变；
  - 已不超过阈值的内容块不被改动，缩减幂等；
  - 缩减后恰好不再超出时不丢弃事件，也不置 `truncated`；
  - 多字节字符不被截断到字符中间；
  - 订阅者持有的对象在降级后保持不变（`-race`）。
- [x] 2.4 续接测试：客户端已收到某条旧 tool 事件，降级后带着其后的序号重连，不再收到该事件

## 3. 相邻消息块合并

- [x] 3.1 在存储事件上保存首序号 `first` 与块的累计字节偏移；`Seq` 取组内最后序号；`eventSize` 对合并事件按合并后的文本计量
- [x] 3.2 `append` 在追加不延续当前组的事件时，把尾部已封存的 `agent` / `thought` 无附件同 ID 组合并为一条存储事件；用户消息与含附件的消息不合并；构造新事件，不修改共享对象
- [x] 3.3 `fromLocked` 取最早存储事件的 `first`；`subscribe` 的起点改为按序号二分查找，落在组内时生成只含剩余文字的同 ID `message.append`（序号为组的最后序号），并保持 `Reset` 条件不变
- [x] 3.4 单元测试：
  - 合并后文本与逐块连接完全一致，`truncated` 取“或”；
  - 仍在流式中的最后一组不合并，实时帧仍逐块；
  - 续接点分别落在组首、组中、组尾、组外一格时，客户端最终文本都与原文一致，且不触发重置；
  - 序号空洞下的 `from` 与 `Reset` 判定；
  - 日志丢弃最早事件时按存储事件整体丢弃；
  - 合并后相同预算保留的对话轮数不少于合并前（用 1.2 的基线对比）。
- [x] 3.5 竞态与慢客户端：`go test -race ./internal/hybrid ./internal/hub`，覆盖合并 / 降级与订阅并发、慢客户端 4408 与重置行为不变

## 4. 契约、前端与其他消费者

- [x] 4.1 `grep` 日志事件的消费者（`internal/hub`、`internal/wire`、`internal/sessioncmd`、`internal/client`、`web/src` 与测试辅助函数），确认没有假定 `seq` 连续的位置；有则一并修改
- [x] 4.2 `web/test` 中补一个 `chat-model` 用例：应用“组内补发剩余文字”的 `events` 帧后，消息文本完整且 `next` 正确推进
- [x] 4.3 确认无需修改 `internal/contract`、`internal/wire` 与 `web/src/contracts/schemas.ts`；`go test ./internal/api -run TestWireSnapshots` 无差异，不重录快照（本地 `TestWireSnapshots/workspace` 因 git 提交时间格式在基线上同样失败，与本变更无关；hybrid 相关快照无差异）

## 5. 文档与用户故事

- [x] 5.1 `docs/user-stories/hybrid-sessions.md`：更新 US-Q05 的验收要点——断线重连只补发缺失事件（包括落在合并消息中间的情形）、超出 `scrollback_bytes` 时先缩短旧工具输出再丢弃最早事件；无新增故事，ID 不变
- [x] 5.2 `docs/protocol/http-hybrid.md`：如有聊天事件续接说明，补充序号可能不连续与组内补发的语义
- [ ] 5.3 `docs/user-stories/verification-log-2026-11.md`：只记录实际运行过的验证；`docs/user-stories.md` 与 `coverage.md` 的计数不变则不改
- [ ] 5.4 PR 描述写明受影响故事 ID（US-Q05），并说明无可见 UI 变更；若决定改 `ToolRow` 文案，则按 `webmux-ui` 技能补截图

## 6. 验证

- [ ] 6.1 `go test -race ./internal/hybrid ./internal/hub ./internal/wire ./internal/api ./internal/sessioncmd`
- [ ] 6.2 `pnpm --dir web exec vitest run --config vitest.config.ts` 中聊天相关用例，以及 `pnpm --dir web check:arch`
- [ ] 6.3 `make check`（需要 golangci-lint v2.15.0）与 `make docs-check`
- [ ] 6.4 `openspec validate compact-chat-event-log --strict --no-interactive` 通过
- [ ] 6.5 人工验证：用 fake agent 跑一个长会话，手机宽度下断线重连 / 刷新，对比改动前后重放字节数与保留轮数，把真实结果写入 PR 描述
