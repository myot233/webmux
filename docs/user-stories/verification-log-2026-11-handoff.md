# Portable handoff 核对记录

本页记录 2026-11-11 起交接增强层的实际验证。故事 ID 保持不变。
身份、Activity、历史接口、预览和 inline 的先前依据见[基础层记录](verification-log-2026-11-06.md)；同一对话切换的验证保留在本页。

## 2026-11-11：Handoff 06 同一对话切换与原生继续（US-Q12、US-Q20）

- 范围：同一 Conversation 的 A→B→A 与 A→B→C、真实 session/resume、已确认 Run/position 覆盖、共享运行绑定与原生身份轮换；位置固定，不经过快速开始。
- 协议回归：实际 fake-agent 验证 A 原生 ID/运行 ID 不变、只选择 B 的未见项、旧 Run 的迟到结果仍计入、保持一个 writer；缺少 resume 时生成 full fresh 草稿并等单独投递。
- 并发与生命周期：前台回合、后台工作及 Codex 外部 writer 锁拒绝切换；持久 owner 阻止其他设备同时投递，重启后继续原 attempt，unknown 不绕过；忘记共享绑定保留其余线程。
- 验证：`make check`、全仓 `go test -race -p 4 ./...`、268 项历史/Zod/wire 测试通过。继承的 CODEX_PATH、XDG 配置和运行时变量在测试中清除，使用隔离临时目录。
- 限制：本层无可见 UI，尚未验证真实 provider 账户；nativeContinue 只标识原生 resume，不表示原生历史注入，后者属于下一层。
- 评审补充：恢复与投递共用 Conversation 的 admission token，阻止旧 provider 在成功切换后通过普通归档恢复重开 writer；unknown 恢复拒绝终端端和原生 ID 替换。实际 fake-agent 回归覆盖拒绝后无新 shell、无新 prompt，原 writer 保持运行。
- 中断补充：原生 TUI 实际退出后，确认 PID 0，再退场 actor/已退出 shell；切换在短时测试期限内完成，原 interrupted 记录保留，不再等执行总超时。
- 来源恢复补充：旧 agent 因活跃回合或暂停队列拒绝退场时，失败目标的 owner 不再占用旧运行；目标明确失败的普通聊天保护与终端恢复语义不变，pending/unknown 仍覆盖来源与目标。
- 实例核验：隔离 HOME/XDG 和 fake-agent 存储，实际私有 WebMux 可执行文件经认证 HTTP 完成忙碌来源的归档恢复、服务重启自动恢复；原生身份不变，普通输入均为 200。实际失败目标恢复后普通输入仍为 409、终端恢复为 200，pending/unknown 来源恢复到两端均为 409。
- 定向验证：conversation、handoff、hybrid、API 未缓存 Go race、固定版本 linter 和文档治理通过；无真实 provider 或账户验收，未重新核验其他故事。

## Handoff 07：Codex 原生历史（US-Q12、US-Q20）

- 范围：可选 HistoryInjector、独立 stdio app-server 客户端、目标持有的注入 receipt、历史与用户 prompt 分别记账；预览不启动 agent 探测能力。
- 协议：核对 codex-acp 2.5.0/2.6.0 与 Codex 0.166/0.167 的固定源码；原生帧省略版本字段，角色保持 user/input_text 与 assistant/output_text。
- 实际 stdio：fake Codex 共享 ACP/native 原生存储，核对 coverage、每项角色、顺序和完整正文；用户新请求独立发送，load/restart 不重复 Activity 或 prompt。
- 故障：注入保存后退出保持 unknown，无 inline 回退；显式 load 的完整有序证明只确认历史，原用户命令单独继续，不重复注入。
- 边界：旧版本与明确 MethodNotFound 回退 inline；64 KiB 用户请求可独立于原生历史，inline 回退超限时拒绝且不截短。
- 存储：claim 后崩溃、错误原生 ID、部分证明、新命令绕过、后续用户失败和来源删除的 Go race 测试通过。
- 传输：无版本字段、紧随 ACK 的 EOF、阻塞写取消、超长帧拒绝、错误正文不泄漏的 Go race 测试通过。
- 集成：`make check`、全仓 `go test -race -p 4 ./...`（隔离 XDG/CODEX 环境）、268 项 wire 前端测试通过。
- same-thread 对账：load 使来源过期时保留历史事实和原用户草稿，旧命令拒绝发送；重新预览由用户决定。

## Handoff 08：只读历史 MCP（US-Q20）

- 范围：固定 argv scope 的 `mcp-serve`，经本机 HTTP API 读取冻结来源；数据库仍由服务端独占生命周期。
- 协议：采用 MCP initialize 时代协议，支持 2024-11-05 至 2025-11-25 的四个固定版本；现代 discovery 明确返回 MethodNotFound 以便客户端选择 legacy。
- 实际链路：活 SQLite → Unix HTTP API → CLI stdio MCP → 独立 fake provider，取回 45 项历史，分页无重复；长中文/emoji 按字节 offset 完整重建。
- 活跃复用：普通目标首次接入 MCP，以及连续两次 handoff 刷新冻结 source point；保留原 runtime/native ID，经真实 stdio 读到当前 scope。
- scope 与生命周期：非法 cursor/limit、跨 Conversation、未来项拒绝；new/load/resume 配置与关闭后的 helper 清理通过。
- 门禁：`make check`、隔离环境全仓 Go race、237 项 wire-contract 前端测试通过。补齐集成用例 Manager 关闭后，MCP+既有 CLI 联合重复 2 次通过；既有 CLI 单独重复 5 次通过。

### 2026-11-12：准入取消与写者所有权（US-Q12）

- 准入结果不随调用者取消而丢弃；取消发生在接管后时仍等待旧 ACP 写者退出，清除打开中状态，未开始的原生调用保持 pending。
- 定向 Go race 回归通过：准入阻塞时取消不会提前返回；实际 fake Codex 在 `session/close` 期间取消后原 PID 已退出，会话可正常关闭并保留中断记录。
- `internal/handoff` 的定向原生注入 Go race 套件通过，覆盖独立用户请求、来源删除后回放、未知结果、重启与版本回退。
- 首次生命周期测试错误地预期关闭会将 interrupted 改成 closed；按既有中断记录语义修正断言后，`internal/hybrid -run TestHistory` 通过。

### 2026-11-12：历史 reader 刷新取消（US-Q20）

- 准入使用保留 execution context 的不可取消结果等待；排队时取消不接管 writer，等待旧进程退出期间取消仍完成清理和 actor 转换，不恢复 provider。
- Go race 定向验证 `internal/hybrid` 的 reader 排队取消、关闭期取消、连接丢失，以及 `cmd/webmux` 的真实 MCP 集成通过；受影响 hybrid 包 lint 为 0 issues。
- 独立编译程序经公开 service API 正常刷新 MCP，再在旧 writer 的延迟 `session/close` 中取消并请求正常关闭；真实 `webmux mcp-serve` 通过 Unix HTTP/SQLite 完成读取。
- 独立 smoke 确认旧 writer 与 MCP 子进程均已退出，取消至 join 为 2007 ms，记录为 interrupted/PID 0；只有首次正常刷新发生一次 resume，无用户 prompt，服务关闭在本次测量中为 0 ms。
- 精确排队竞争由确定性回归覆盖；独立 smoke 不使用 `go test`，但 provider 仍为隔离 fake Codex，不代表真实账户或原生 macOS 验收。

### 2026-11-12：长历史草稿读取成本（US-Q20）

- CPU/分配 profile 显示，草稿读取曾在 wire 截取前反复解码完整省略 ID 集合；40,000 项用例中 `readPreview` 累计分配约 33.31 MiB。
- 读取层现保留最多 100 个省略 ID 样本，并读取独立持久计数；原生投递仍保留全部选入 ID，手动上下文覆盖说明使用完整计数，选择分页仍恢复全部 40,000 项。
- 精确 race、atomic coverage 和跨包 `coverpkg` 定向回归通过，本机总用时 14.677 → 12.477 秒；POST 的 profile CPU 为 3.10 → 2.84 秒，草稿读取分配降至采样阈值以下。
- 两个独立编译的服务、隔离 HOME/XDG/状态目录和绝对路径 fake 命令进行真实 HTTP 对照：POST 153 → 124 ms，PATCH 100 → 4.24 ms，GET 30.4 → 0.82 ms。
- 两次 HTTP 对照均恢复 400 页、40,000 个不重复省略 ID，最大选择页 11,449 字节；分页总计 168 → 172 ms。样本均为 100 项，完整省略计数均为 40,000。
- 以上为本机单次对照，不代表 CI 或生产时延保证；生产 10 秒请求 deadline 未改变。受影响 Go 包的固定版本 lint 为 0 issues。
- 首次全仓 coverage/race 仅失败于 `agenttrust/TestPrepareWritesThroughSymlinkedConfig` 的文件模式夹具；API、Conversation、handoff、hybrid、thread_read 均通过。
  - 原因：继承 `umask 0077`，初始模式实际为 `0600`。
  - 随后组合核心 `853f55bd` 显式建立夹具模式，`make check` 与全仓相同 race/atomic coverage 检查通过。
- CI 的完整 instrumentation 仍使 40,000 项快照构建超过 REST 期限；恢复回归不再把 instrumentation 墙钟时间当成功能契约。
  - service 先构建大夹具，真实 HTTP GET/PATCH、selection 与 MCP 继续核对完整数据和响应边界。
  - 数据规模、断言和生产 10 秒期限不变；大数据 HTTP POST 的生产路径由上面的独立二进制测量验证。

## 核对范围与限制

本层没有可见 UI；浏览器入口和桌面/手机交互将在对应 UI 层核对。
测试使用隔离临时状态与 fake provider，未连接真实 provider 账号、未发出计费模型请求。
版本矩阵是上游协议核实；未宣称在原生 macOS 或真实移动设备上运行本层测试。
集成门禁和后续增强的实际结果在完成后追加，不把未运行项目记为通过。
