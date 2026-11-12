# Codex 原生历史交接

未编辑的结构化交接可把历史写入 Codex 原生线程，再把用户新请求作为独立 turn 发送。
预览只在已经观察到该启动配置的受支持 CLI 版本时提供 `nativeAvailable`，不会为探测能力启动 agent。
未观察到能力、其他 agent 或手工编辑的草稿继续使用 inline。通用流程见[交接预览](http-handoff.md)。

## 已核实的协议

| 实现与版本             | 核实结果                                                                         | 处理方式                                                 |
| ---------------------- | -------------------------------------------------------------------------------- | -------------------------------------------------------- |
| codex-acp 2.5.0、2.6.0 | 没有历史注入路由或 initialize capability；steering 属于当前用户输入              | 不猜测 ACP 私有方法                                      |
| Codex 0.166.0、0.167.0 | app-server 提供 thread/inject_items，接受 raw Responses items，响应不返回项目 ID | 独立 native 客户端，使用持久的逻辑项目 ID 和有序摘要对账 |
| 其他或未知 CLI 版本    | 未核实原生协议                                                                   | 在 mutation 前使用 inline                                |

矩阵基于以下固定版本的上游源码、schema 和集成测试，不代表真实账号模型请求的实测：

- [codex-acp 2.6.0 路由](https://github.com/agentclientprotocol/codex-acp/blob/v2.6.0/src/AcpAgentRouter.ts)及[初始化 metadata](https://github.com/agentclientprotocol/codex-acp/blob/v2.6.0/src/CodexAcpServer.ts)。
- [Codex 0.167.0 请求 schema](https://github.com/openai/codex/blob/rust-v0.167.0/codex-rs/app-server-protocol/schema/json/v2/ThreadInjectItemsParams.json)。
- [Codex 0.167.0 handler](https://github.com/openai/codex/blob/rust-v0.167.0/codex-rs/app-server/src/request_processors/turn_processor.rs)及[注入集成测试](https://github.com/openai/codex/blob/rust-v0.167.0/codex-rs/app-server/tests/suite/v2/thread_inject_items.rs)。
- [原生帧格式](https://github.com/openai/codex/blob/rust-v0.167.0/codex-rs/app-server-protocol/src/rpc.rs)不要求或发送 jsonrpc 版本字段；不能沿用 ACP 的帧校验。

## 角色、位置与生命周期

coverage 独立成项；历史 user 使用 user/input_text，历史 agent 和规范化工具结果使用 assistant/output_text。
每项包含 Conversation、ProviderThread、Run、Activity、类型和状态来源；不携带 reasoning、原生工具状态或附件正文。
用户新请求不在注入列表中。原生历史预算独立于 prompt 的字节上限，用户新请求仍受现有上限约束。

持久 owner 先确认空闲并结束 ACP writer，再启动同一 shell 管理的短时 app-server helper。
helper 使用精确原生 ID 执行 thread/resume 与注入，不启动 turn，不修改全局配置。
确认 helper 及子进程退出后，ACP 真正 resume，再发送用户新请求；helper 的 PID/启动身份写入运行恢复记录。

## 接受与恢复

- mutation 前保存 pending、原生 ID、有序项目 ID/角色/摘要，并以单向 gate 防止重复调用。
- 确认接受后单独保存 injected 与覆盖进度；后续用户 prompt 失败或取消不会抹掉此事实。响应以 historyStatus 和 promptStatus 区分两者。
- 明确不支持且未 mutation 时回退 inline，并重新核对组合后的上限；不够则拒绝，保留用户请求与草稿。
- 请求可能已写入后超时、断连或其他错误保持 unknown，不自动回退或重注入。
- load 只有精确原生身份、角色和完整有序摘要匹配才确认 unknown；部分匹配只标缺口。历史导入项不再作为新的 Activity 或 delta。
- 目标持有最小回放摘要，忘记来源后仍能识别已导入项，不保存额外来源正文，也不重建已删除的来源操作。
