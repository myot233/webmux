# Tasks

本清单是设计评审后的实现计划，当前全部未执行。PR observe 与 PR protocol 各自包含完整迁移，不提交只定义类型但没有真实消费者的骨架。

## 1. PR observe：共享观察与一致性修复

- [ ] 1.1 引入 `sessionobserve` 的聊天/会话订阅与明确 selector，支持提交前 Prime 和提交后附加；用真实 WebSocket fixture 验证 snapshot、replay 与输入之间无观察空窗。
- [ ] 1.2 将 CLI 与 MCP 的目标回合、queue present/started/removed、待回答请求和结束判断迁到共享状态机；表驱动用例证明初始 absent、删除、同帧 start 优先和其他回合隔离。
- [ ] 1.3 在状态机中保留缺失回放、unfinished 与单调完整性标记，回复追加时应用 MCP 字节上限，CLI 按需收集事件；覆盖 start 被裁掉、只有 end、state fallback、已解决请求及跨 UTF-8 边界。
- [ ] 1.4 同时迁移 CLI 状态策略和 MCP waitExit 使用共享会话事实；验证多会话 changed/stable/any 未退化，idle 不满足退出，退出码 7 与 removed-before-exit 正确区分。
- [ ] 1.5 删除已被替代的 turnWait、queuedGone 和 MCP 内联等待判断，不保留双实现；运行 `go test -race ./internal/sessionobserve ./internal/clicmd ./internal/mcpcontrol`，确认 deadline、零超时、断线和取消映射正确。
- [ ] 1.6 将新包登记为客户端层，更新 `docs/usage/mcp.md` 的队列删除行为与 US-S23；核对 US-S09/S14 的差异不变，运行 `go test ./internal/archtest` 与 `make docs-check`。
- [ ] 1.7 构建并启动临时配置、临时状态目录的隔离实例，实际通过 CLI 和 stdio MCP 等待 fake agent，另一客户端删除已排队输入；观察两端及时报移除、超时后会话仍存在，并记录实际 smoke 命令与结果。

## 2. PR protocol：协议提取与全部 stdio 消费者迁移

- [ ] 2.1 将 MCP 请求/响应/初始化/工具结果外壳 DTO 放入 `internal/contract`，实现共享 Registry 与独立 Session 分派；协议用例验证原有 JSON 字段、空值、握手顺序及未知工具错误保持不变。
- [ ] 2.2 将 stdio 缩为有界逐行读写，同时迁移控制工具与 threadread 到新协议类型；运行现有 `mcpstdio`、`mcpcontrol`、`threadread` race 测试，证明 stdout 只有协议和原有协商版本仍可用。
- [ ] 2.3 删除旧 `mcpstdio.Tool/Server` 所有权及失效代码，不增加 type alias/re-export；登记 mcpprotocol 为共享层，运行 archtest 验证没有服务/控制器反向依赖。
- [ ] 2.4 保持控制调用 185 秒、历史调用 10 秒，工具执行不持握手状态锁；用受控阻塞 handler 验证 Session 状态锁不阻塞独立 dispatch，stdio 自身仍按原顺序执行。
- [ ] 2.5 更新 `docs/protocol/handoff-mcp.md` 的协议归属说明，不改冻结来源权限；核对 US-S20 与冻结历史故事不变，运行 docguard 及现有冻结点/跨 conversation 拒绝回归。
- [ ] 2.6 实际启动控制与冻结历史 stdio 进程完成 initialize、initialized、tools/list 和一次工具调用；验证控制无 thread_read、冻结历史无 session_launch，进程错误不污染后续协议调用。

## 3. Stack integration

- [ ] 3.1 在 observe、protocol 两个已独立通过的分支上创建父子 PR，逐层运行相关 race 与架构/文档门禁；上层只依赖真实迁移后的 API，不依赖未提交代码。
- [ ] 3.2 记录各 PR 的可见行为、验证命令、已知限制和故事影响；现有 wording/source-text 测试不重 pin，CI 和 bot 审查全部通过后保持 PR 开放，不自动合并。

## Workflow follow-up

- 核心两层之后实施指南变更，再实施网络 MCP；具体功能验收由各自 change 定义。
- 设计文件不证明任务完成；仅在实现与验证真实完成后勾选任务，按仓库流程归档与同步主规格。
