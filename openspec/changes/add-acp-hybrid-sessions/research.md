# 调研记录

本文件记录 design.md 所依据的外部事实与对标，日期为 2026-10-28。真实 agent 行为的实测见 issue #181 的“研究记录”；实施时的基线写入本目录下的 `baseline.md`（里程碑 0）。

## ACP 协议（v1 稳定子集）

- 方法与 schema 以 `schema-v1.27.0` 为准，地址为 `https://github.com/agentclientprotocol/agent-client-protocol/releases/download/schema-v1.27.0/schema.json`，方法名映射见仓库 `schema/v1/meta.json`。
- **稳定的 agent 方法**：`initialize`、`authenticate`、`logout`，以及 `session/new|load|list|delete|resume|close|prompt|cancel|set_mode|set_config_option`。
  - `session/fork` 与 `providers/*` 仍属不稳定。
- **会话能力的声明**：
  - `list`、`resume`、`close`、`delete` 在 `agentCapabilities.sessionCapabilities.*` 下声明，值为 `{}` 表示支持。
  - `session/load` 仍由独立的 `loadSession` 布尔值声明。
- **`session/list {cwd?, cursor?}`**：返回 `sessions[{sessionId, cwd, title?, updatedAt?}]`，`cursor` 为不透明值。来源：https://agentclientprotocol.com/protocol/v1/session-list
- **`session/resume`**：不回放历史。
- **`session/load`**：先以 `session/update` 回放全部历史，然后才响应。
- **`session/close`**：隐含一次取消，然后释放资源。来源：https://agentclientprotocol.com/protocol/v1/session-setup
- **稳定的更新种类**：
  - `user_message_chunk`、`agent_message_chunk`、`agent_thought_chunk`（可带 `messageId`）
  - `tool_call`、`tool_call_update`
  - `plan`、`available_commands_update`、`current_mode_update`、`config_option_update`（整体替换）
  - `session_info_update`、`usage_update {used, size, cost?}`
  - `notice {severity, title, description?}`
  - `compaction_update`（补丁语义）、`compaction_summary_chunk`
- **需要客户端先声明能力的更新**：`notice` 需要客户端声明 `clientCapabilities.session.notices`，`compaction_update` 需要声明 `session.compaction`。因此 webmux 必须声明这两项，否则 OpenCode 不会发送压缩事件。
- **`elicitation/create`**：分为 `form`（携带 `requestedSchema`）与 `url` 两种模式。客户端以 `clientCapabilities.elicitation.{form,url}` 声明支持；第一期只声明 `form`。
- **错误码**：
  - `-32000`：需要认证（`auth_required`）
  - `-32002`：资源不存在
  - `-32800`：请求已取消
- **取消回合**：`session/cancel` 之后，客户端必须把挂起的权限请求答复为 `cancelled`，并继续接受迟到的更新。来源：https://agentclientprotocol.com/protocol/v1/prompt-turn
- **不支持文件与终端能力**：省略能力即视为不支持，`fs` 与 `terminal` 默认均为 false。来源：https://agentclientprotocol.com/protocol/v1/initialization
- **扩展**：
  - 自定义方法以 `_` 开头，未知通知应忽略。
  - 扩展能力通过能力对象上的 `_meta` 声明。
  - 来源：https://agentclientprotocol.com/protocol/v1/extensibility
- **传输**：stdout 只能输出协议消息，每行一条；日志走 stderr。来源：https://agentclientprotocol.com/protocol/v1/transports

### v2 草案的方向（design 决策 4 以此为目标形状）

- 工具调用统一为以 `toolCallId` 为键的 upsert，消息统一为以 `messageId` 为键的 upsert。
- 删除 `session/load`，改用 `session/resume {replayFrom}`。
- `session/prompt` 在消息插入后立即返回，回合结束由 `state_update` 表达。
- modes 被 config options 取代，客户端 `fs`/`terminal` 被删除。

来源：https://agentclientprotocol.com/rfds/v2/overview

### 仍是草案、本期不依赖的能力

- 远程传输（Streamable HTTP / WebSocket）：https://agentclientprotocol.com/rfds/streamable-http-websocket-transport
- 代理链：https://agentclientprotocol.com/rfds/proxy-chains
- Session Status 与插话/排队讨论：https://agentclientprotocol.com/rfds/updates

## Go 实现选型

| 选项                       | 现状                                                                                                                                    | 结论                  |
| -------------------------- | --------------------------------------------------------------------------------------------------------------------------------------- | --------------------- |
| `coder/acp-go-sdk`         | v0.13.5（2026-06-02）后停更；固定在旧 schema，缺少 `usage_update`、`notice`、`compaction_update`、`elicitation`；无法解析的行原样写日志 | 不采用                |
| `caelis-labs/acp-go-sdk`   | v1.5.1（2026-10-16），由 `schema-v1.26.0` 生成；带 stdio 传输与进程组关闭；Apache-2.0                                                   | 备选，用于以后评估 v2 |
| `ironpark/acp-go` 等社区库 | 维护活跃，但接口和生成方式各不相同                                                                                                      | 不采用                |
| 自行实现最小客户端         | 约 15 个方法；只依赖标准库；用固定版本的 schema 守卫测试防止字段漂移                                                                    | **采用**              |

社区库列表：https://agentclientprotocol.com/libraries/community#go 。目前没有官方 Go SDK，官方 SDK 为 Rust、TypeScript、Python、Java、Kotlin。

## 对标的客户端架构

- **Zed** `acp_thread`：
  - 线程是一个条目列表（用户消息、助手消息、工具调用、表单、压缩），工具调用按 `toolCallId` 反向查找；
  - 待批准的权限按 ID 保存在有序映射中；同一时刻只有一个运行中的回合；
  - 取消时把挂起的权限答复为 cancelled，并把待定条目标为取消；
  - 内部先把 v1 转换为 v2 形状。webmux 的规范化事件沿用这一做法。
  - 来源：https://github.com/zed-industries/zed/blob/main/crates/acp_thread/src/acp_thread.rs
- **Emacs agent-shell**：
  - 忙碌时提交的消息进入队列，回合结束后发送；
  - 取消时询问是否继续发送队列；插话是单独的命令。
  - 与 D6、D20 的交互一致。来源：https://github.com/xenodium/agent-shell
- **marimo `use-acp`、acp-ui**：
  - 浏览器经 stdio-to-WebSocket 桥接 agent；
  - 只保存一个待处理的权限请求；
  - acp-ui 的 `--persist` 让 agent 跨断线存活，客户端重连后用 `session/load` 恢复。
  - webmux 把这层桥接与状态放在服务端，以便多设备共享并在无人连接时继续运行。
  - 来源：https://github.com/marimo-team/use-acp 、https://github.com/formulahendry/acp-ui
- **tlbx**：.NET 实现，把持久 ACP 会话与终端会话放在浏览器 WebSocket 之后，是形态最接近的产品。来源：https://github.com/tlbx-ai/tlbx
- **OpenCode ACP 实现**：
  - 源码位于 `packages/cli/src/acp`（2.2.0）；
  - 声明 `loadSession`、`list`、`delete`、`resume`、`close`、`fork`，并发送 `usage_update`、`compaction_update`、`config_option_update`、`available_commands_update`、`session_info_update`。
  - 来源：https://github.com/anomalyco/opencode/tree/dev/packages/cli/src/acp
- **omp 的 `acp` 能力声明**已在源码层面核对，见下一节；发行版行为仍需在 M0 中抓取 `initialize` 响应确认。

## agent 源码核对（2026-10-28）

源码版本：omp 为 `can1357/oh-my-pi@6f3fbc9`（2026-10-27），OpenCode 为 `anomalyco/opencode@86ce19d`（v2.2.0，CLI 重写到 `packages/cli`）。只读阅读源码，未运行程序；发行版行为以 M0 为准。

### omp

- **`initialize`**（`packages/coding-agent/src/modes/acp/acp-agent.ts` L810–861）：
  - `loadSession: true`；
  - `sessionCapabilities: {list, fork, resume, close}`，没有 `delete`；
  - 顶层 `_meta: {steering: {supported: true}}`；
  - `agentInfo.name = "omp"`。
- **实现的方法**：
  - 配置项 ID 为 `mode`、`model`、`thinking`；
  - `_session/steering` 在回合空闲时不会开启新回合，`_meta.steering.queue: "followUp"` 会把消息放进 omp 自带的跟进队列；
  - 另有 `_omp/*` 私有扩展。
- **`session/resume`**：不回放历史，只有 `loadSession` 回放。
- **`session/list {cwd}` 的副作用**：
  - 不是纯读：可能执行旧目录迁移（重命名）、`mkdir`，以及在主文件缺失时恢复 `.bak`；
  - 不申请会话租约（源码：“inspection never does”）；
  - 每次启动进程还会写 presence 文件，并安排一次插件市场自动更新。
  - 结论：短时探测可以与 TUI 并存。
- **租约**：TUI 持有会话的 OS 租约时，另一个进程写入同一会话，会把它移到一个新 ID 的“兄弟会话”（`session-manager.ts` L1107–1140）。因此必须等 TUI 完全退出后，ACP 才能 `load` 并发送 prompt。
- **`already loaded for X, not Y`** 是同一 ACP 进程内部的检查（同一 ID 以不同 cwd 加载），与 TUI 无关。
- **退出**：
  - SIGINT、SIGTERM、SIGHUP 都会触发清理：保存草稿，关闭 SessionManager 并刷盘，释放租约；
  - 也支持 `/exit`、`/quit`。
- **会话目录**：`<agentDir>/sessions/<编码后的 cwd>/`，`agentDir` 受以下变量影响：
  - `PI_CODING_AGENT_DIR`；
  - `PI_CONFIG_DIR`；
  - `OMP_PROFILE`（旧名 `PI_PROFILE`）；
  - XDG 目录。
- **`--session-dir`** 在 ACP 中只作用于新建会话，`list`/`load`/`resume` 始终用默认目录。
- **参数**：
  - `--resume`/`-r`/`--session`（值可选）、`--restore`、`--continue`/`-c`；
  - `--model`、`--thinking`、`--profile`、`--config`（可重复）；
  - `--approval-mode`（取值 `always-ask`、`always-ask-cached`、`write`、`yolo`）、`--auto-approve`/`--yolo`；
  - `--session-dir`、`--no-session`；
  - `omp acp` 解析完整的启动 argv。

### OpenCode v2.2.0

- **`initialize`**（`packages/cli/src/acp/service.ts` L106–131）：
  - `protocolVersion: 1`、`loadSession: true`；
  - `sessionCapabilities: {additionalDirectories, close, delete, fork, list, resume}`；
  - `_meta: {"opencode/child-session-updates": true}`；
  - 没有插话（steering）能力。
- **`session/resume`**：不回放历史，`loadSession` 回放。
- **服务进程**：
  - `opencode acp` 总是启动私有服务（`opencode serve --stdio`，生命周期绑定 stdin）；
  - TUI 默认连接后台服务；
  - 两个服务共享同一个 SQLite 数据库（WAL 模式，`busy_timeout = 5000`），路径可用 `OPENCODE_DB` 覆盖。
- **回合冲突**：新回合开始时，会把处于“运行中”的工具调用和压缩标为已中断。如果后台服务仍在执行同一会话的回合，ACP 端的新回合会中止它的工具。这是 design 决定注入 `--standalone` 的依据。
- **退出**：
  - SIGTERM、SIGINT 有 10 秒的清理期限，有 TTY 时 SIGHUP 按 SIGTERM 处理；
  - `/exit` 及其别名 `quit`、`q`；
  - 结束 TUI 不会停止后台服务中的回合；使用 `--standalone` 时，私有服务随 stdin 关闭而退出。
- **参数**：
  - TUI 接受：`[directory]`、`-c/--continue`、`-s/--session`（“继续，不存在则创建”）、`--prompt`、`--auto`（隐藏别名 `--yolo`、`--dangerously-skip-permissions`）、`--standalone`、`--server`、`--print-logs`；
  - TUI 已没有 `--model`/`--agent`，这两个参数只在 `run`/`mini` 子命令上；
  - `opencode acp` 只接受 `--login`。
- **未核对**：`session.list` 的 SQL 是否严格只读；结束 standalone TUI 是否中止进行中的回合（M0 tasks 1.4）。

## 子进程管理要点

- 不要在读完 `StdoutPipe` 之前调用 `Wait`。
- `Cmd.Cancel` 与 `WaitDelay` 可以限制孙进程持有管道导致的挂起（`go doc os/exec.Cmd`）。
- `bufio.Scanner` 默认 64 KiB 行长上限，会返回 `ErrTooLong`；ACP 的工具输出可能远超此值，应使用带硬上限的按行读取，超限断开，而不是截断。
- stderr 必须持续排空，否则管道写满后子进程会阻塞。
- webmux 在 Linux 上开启 subreaper；未登记的子进程僵尸会被 `reapAdopted` 回收，所以 ACP 子进程必须登记为受管首进程（见 design 决策 7）。
