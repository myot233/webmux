# Tasks

## 1. Shared guide and semantic corrections

- [ ] 1.1 扩展 agentguide 公开上下文/纯渲染接口，迁移初始化、CLI、MCP instructions 并删除重复常量；完成判据：全部调用者使用共享规则，核心不读 env/PID、不导入 service，语义单测覆盖 transport/locality/self 隔离。
- [ ] 1.2 修正逻辑操作稳定 ID 与 create 10 分钟、混合 256/30 分钟、终端 receipt/generation 的独立边界；完成判据：语义测试和隔离 CLI 丢响应 smoke 断言同操作同 ID、调用次数、淘汰/重启后先 inspect，不盲目重发。
- [ ] 1.3 增加输入 accepted/written、状态变化、指定回合结束、进程退出与 exitCode 四类证据，并单列业务验收；完成判据：idle、end_turn、非零进程退出、超时继续等待测试均不越级推断任务成功。
- [ ] 1.4 修正 optionId≠kind、仅当前请求/用户授权、version/status 比较、真实配置及模型发现；完成判据：真实 CLI/fake-agent 使用自定义权限 ID、非 Codex 配置和动态模型值，断言真实请求内容与调用次数，指南不硬编码 provider/model。
- [ ] 1.5 修正普通 rm 与混合 rm/archive/resume/forget/淘汰/重启目录说明；完成判据：隔离实例标记文件 smoke 验证目录存留，远程说明不将路径视作写能力，交付相对路径与 uploads 规则保留。
- [ ] 1.6 保留命令表生成的 commands 并提供 MCP 精选主题，更新受影响帮助和操作主题；完成判据：配方所引用命令/工具均存在，不恢复旧 chat send/switch，不新增 answer/view/upload，不用整篇快照替代语义断言。
- [ ] 1.7 实现完成后更新 US-S05/S06/S08/S09/S11/S14 的相应故事与验收；完成判据：故事只描述已实现行为，新增故事时同步 ID、索引与计数，本规划阶段不改故事文件。

## 2. MCP guide adapter

- [ ] 2.1 从实际 registry 为 instructions/capabilities/guide_read 提供一致工具列表，wire DTO 放入 internal/contract；完成判据：工具列表一致性测试通过，远程 self 为 null，不混入 threadread 冻结 registry。
- [ ] 2.2 注册 guide_read 严格 schema、index/topic 与 65536-byte 完整编码上限，沿用 isError:true 错误文本与成功默认 JSON text；完成判据：null/空值/未知字段/未知主题/大小边界测试覆盖，无新 handler 协议错误或专属 envelope。
- [ ] 2.3 保持 TCP llms 静态文档无实例事实，本机仅接收既有允许事实；完成判据：TCP 各 llms 路径测试不含版本、路径、会话、Token，本机与远程上下文语义测试通过。
- [ ] 2.4 更新 US-S20/S21/S22/S23 中相应指南/工具发现验收；完成判据：仅在实现后更新，实际工具与能力限制一致，新增故事同步索引、ID 和计数，不提前声明 HTTP 能力。

## 3. CLI unfinished adapter

- [ ] 3.1 在现有 turnResult 对象增加顶层 unfinished，映射真实观察标记，文本 true 时 stderr 提示；完成判据：没有伪事件、额外 envelope/completionLevel/selector/truncated 字段，原 stopReason/error/reply/events/request 保持不变。
- [ ] 3.2 用真实 CLI 与 fake-agent 覆盖 unfinished true/false、end_turn、待请求、队列删除、超时及非零进程退出；完成判据：JSON 对象字段、events、stdout/stderr 和全部既有退出映射断言通过，end_turn+unfinished 仍为 0，不使用开发者会话。
- [ ] 3.3 更新 US-S08/S09/S11 对应输出验收与受影响 CLI 帮助、发布说明；完成判据：记录新增 boolean 和真实后台工作语义，不宣称改变 JSON envelope 或退出码，不把未经验收混同 unfinished。

## 4. Integration gates

- [ ] 4.1 所有实现落地后统一运行相关 Go/CLI/fake-agent、架构、docguard 与 make test-fast；完成判据：记录每项实际结果及失败处置，规划阶段不运行检查。
- [ ] 4.2 按仓库合并要求运行 make test；完成判据：完整门禁结果可追溯，无未实现工具、重复旧指南路径或未迁移调用者，不借用已有开发者会话。
