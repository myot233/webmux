# Portable handoff 核对记录

本页记录 2026-11-11 起交接增强层的实际验证。故事 ID 保持不变。
身份、Activity、历史接口、预览、inline 和同一对话切换的先前依据见[基础层记录](verification-log-2026-11-06.md)。

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

## 核对范围与限制

本层没有可见 UI；浏览器入口和桌面/手机交互将在对应 UI 层核对。
测试使用隔离临时状态与 fake provider，未连接真实 provider 账号、未发出计费模型请求。
版本矩阵是上游协议核实；未宣称在原生 macOS 或真实移动设备上运行本层测试。
集成门禁和后续增强的实际结果在完成后追加，不把未运行项目记为通过。
