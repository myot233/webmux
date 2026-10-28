## Context

动机与范围见 proposal.md，产品决策 D1–D32 见 issue #181，行为合同见本变更的 `specs/`，外部调研与备选方案的出处见 `research.md`。下面只列出决定架构的现状。

- **会话 = 一次 PTY 运行**。`session.Session` 同时持有身份（`Info`、列表订阅、关注）和一次运行（`cmd`、PTY、环形缓冲、`Generation`、`wait()` 中的退出发布）。进程退出即会话结束：发布退出帧、以 4410 关闭终端连接。`Manager.Create` 随机生成 8 位 ID，不接受外部 ID。
- **一切消费者都看 `Manager`**：`/ws/sessions`、`webmux ls`、代码工作区（`codeview.New(m)`）、Web Push（`push.watch` 只订阅 `Manager` 并读取 `Info.Attention`）都以 `Manager` 为唯一会话来源。
- **进程与清理**：`startPTY`、`startChild` 把根进程登记到 `managedLeaders`；Linux 的 `reapAdopted` 会 `Wait4` 任何未登记的本进程僵尸子进程。因此任何新子进程如果不登记，退出状态都会被抢走，`cmd.Wait` 得到 `ECHILD`。清理按会话 ID 环境标记和进程树执行。目前没有读取任意进程 cwd 的代码。
- **信号解析**：`OSCScanner` 对 Warp 事件只保留 `event` 名；OSC 7501 带 `id` 的上报整体丢弃，`app` 从不保留（`osc.go:245-321`）。
- **线上契约**：`/ws/s/{id}` 是带类型字节的二进制帧，`/ws/sessions`、`/ws/settings` 是文本 JSON。R8 要求 Controller 编解码的每个 JSON 值都是 `contract` 具名类型；R9 禁止 Service 在未登记的函数里编码 JSON；R10 要求 `contract` 只依赖标准库。
- **启动配置**：`launch-configs.json` 为 v2。自 #147 起，状态文件严格拒绝未知键与错误版本，并且“只接受当前版本、不双读”。新增可选字段的先例并不一致：`preview`（#139）把版本从 v1 升到 v2；`icon`（#137）没有升版本，但它早于严格读取。新安装预置 OpenCode、omp 的 bashrc 包装 `bash -ic 'exec -- "$@"' webmux <agent>`。
- **CLI 退出码**：0 分离、1 错误、2 用法、3 服务未运行，其余透传会话退出码。目前没有“拒绝附着”的路径。tmux 的客户端被别处分离或会话被结束时都退出 0（本地实测）。
- **前端约定**：`ws.ts` 只把 4404、4410 视为终止，其余关闭码一律退避重连。草稿规则会在列表快照中找不到会话时删除其本机草稿。
- **agent 源码事实**（omp `6f3fbc9`、OpenCode v2.2.0，详见 `research.md`）：
  - omp 的 TUI 持有会话的 OS 租约时，另一进程写入同一会话会把它分叉成新 ID。
  - OpenCode 的 TUI 默认连接后台服务，`opencode acp` 启动私有服务，两者共享同一个 SQLite；`--standalone` 让 TUI 自带服务，并随 TUI 退出。
  - 两者都声明了不回放历史的 `session/resume`，都能在 SIGHUP 时刷盘退出。
  - OpenCode 的 TUI 位置参数是目录，已没有 `--model`。M0 实测 2.1.4 与 2.2.0 的 `--help` 完全相同，ACP 行为一致（`baseline.md`）。
- **ACP 生态**（2026-10-28）：没有官方 Go SDK；`coder/acp-go-sdk` 停在 v0.13.5，缺少 `usage_update`、`notice`、`compaction_update`、`elicitation`，并且会把无法解析的行原样写入日志。规范以 `schema-v1.X.Y` 标签单独发布 `schema.json`。

## Goals / Non-Goals

**Goals:**

- 普通 PTY 会话的行为、线上负载和快照零变化；混合会话的所有新增状态都可以按会话关闭。
- 单写者可证明：任何时刻，同一 agent 会话 ID 至多被一个 webmux 托管的进程打开。
- ACP 协议细节只出现在 `internal/acp` 与 `internal/hybrid` 的转换层；前端、`contract` 与推送面对的是 webmux 自己的规范化事件。将来接入 ACP v2、Claude/Codex 适配器或 omp RPC 模式时，替换的是转换层与档案，而不是契约与前端。
- 每个里程碑都可独立合入并通过 `make test`；勾选框在最后一个前端里程碑才对用户可见。

**Non-Goals:**

- 不实现 ACP 服务端、远程 ACP 传输、多客户端 attach、客户端 `fs/*`/`terminal/*`（issue “不做”列表）。
- 不在服务端保存消息正文、队列或 token；不跨重启保留队列。
- 不为普通 PTY 会话增加任何跨重启能力。
- 不引入第三方 Go ACP SDK（理由见决策 3）。

## Decisions

### 1. 分层与包划分

| 包                      | 层      | 职责                                                                                          | 依赖                                                                |
| ----------------------- | ------- | --------------------------------------------------------------------------------------------- | ------------------------------------------------------------------- |
| `internal/acp`          | 共享    | JSON-RPC 2.0 over stdio 连接；ACP v1 子集的类型、方法与能力协商；不启动进程，不含 webmux 语义 | 标准库                                                              |
| `internal/agentprofile` | 共享    | 内置档案（纯数据）与纯函数：命令写法识别、恢复 argv、ACP argv、信号到身份的映射、版本比较     | 标准库                                                              |
| `internal/hybrid`       | Service | 混合会话注册表、每会话 actor、切换状态机、队列、事件日志、ACP 进程编排、记录存储、资格判定    | `session`、`launch`、`settings`、`acp`、`agentprofile`、`statefile` |
| `internal/session`      | Service | 增加外壳/运行分离、伴随进程、身份信号、根进程 cwd                                             | 不导入 `hybrid`                                                     |

- `acp` 放在共享层而不是 Service：它像 `contract` 一样只是协议，JSON 编解码在这里不触发 R9 的 Service 编码限制，也不需要在 `encoders` 中登记。`hybrid` 只操作类型化结构，不直接编码 JSON；持久化经 `statefile`。
- `agentprofile` 独立成包，是为了让 `launch`（校验 `hybrid` 开关）与 `hybrid`（编排）都能使用而不形成 `launch ↔ hybrid` 的循环。
- 依赖方向：`workspace → hybrid → session`。`session` 通过它自己声明的接口回调 `hybrid`（决策 2），不导入它。新增包全部登记到 `layers_test.go`。可以再加一条架构规则“只有 `hybrid` 导入 `acp`”，以防协议类型泄漏到其他 Service。

**备选方案：**

- **(a) 把 ACP 作为 `session` 的第二种后端**：`session` 会同时包含 PTY、JSON-RPC 和档案，核心包膨胀，还需要登记 R9 编码器。不采用。
- **(b) `hybrid` 自建一份会话注册表，由 Controller 合并列表**：推送、代码工作区、`webmux ls` 都需要第二个数据源，Controller 会承担合并逻辑。不采用。

### 2. 会话外壳与运行分离

把 `Session` 拆成两个概念：

- **外壳（shell）**：ID、`Info`、列表发布、终端订阅者集合与控制端、关注覆盖层；
- **运行（run）**：一次 PTY 进程、环形缓冲、运行代际、读写与退出流程，即现有实现。

普通会话恰好有一个运行，运行结束即外壳结束，行为与现在逐字节一致。

混合会话的外壳由 `hybrid` 以驱动（driver）身份创建：

- `Manager.CreateShell(input, driver)`：登记外壳，可选地立即启动第一个运行。`input.ID` 可选，用于恢复时沿用记录中的 ID。
- `Manager.SetIDReserver(r)` 在 `cmd/` 装配时注入，随机生成 ID 时跳过记录中保留的 ID。
- `Shell.StartRun(argv)` / `Shell.EndRun(ctx)`：在同一 ID 下启动新运行，或结束当前运行。运行结束时，如果驱动声明“这是切换”，就不发布退出帧，而是以 **4409（会话已移到另一端）** 关闭全部终端连接并清空订阅者。新运行使用新代际，客户端按现有续接协议收到 `reset`。
- `Shell.SetOverlay(fn)`：由驱动写入 `Info.Hybrid`，以及聊天端的 `Status`、`Attention`、`Preview`。仍经由 `updates()` 中的 `sameDisplay` 比较后发布到列表，推送因此无需改动。
- `driver.Close()`：`Manager.Close(id)` 遇到驱动外壳时交给驱动，按“关闭”语义处理（先按切换流程中断，再记录 `closed`）。这样 API、CLI 与 `Shutdown` 的现有调用点都不用分支。

**非活动的外壳**：

- TUI 自行退出（D23）时，外壳保留在 `Manager` 中，状态为已退出，`hybrid.state = interrupted`，这样会话页还能回放最后的终端输出并提供恢复按钮。
- 首页运行中列表过滤掉 `hybrid.state != active` 的会话，它们只出现在“可恢复”分组。
- 恢复或移除记录时，先移除这个已退出的外壳。
- 用户关闭时直接移除外壳。

**备选方案：每次切换都销毁并重建 `Session`，复用同一个 ID。** 旧会话的退出流程会向所有终端连接发出 4410，`webmux attach` 会以为会话已经结束，列表还会闪出“已退出”。不采用。

### 3. 自行实现最小 ACP 客户端，类型对照固定版本的 schema

`internal/acp` 包含以下部分：

- **成帧**：`bufio.Reader` 按行读取，硬上限 32 MiB。超限或非 JSON 行视为协议错误并断开，只计数，不记录内容。
- **请求关联**：原子递增 ID，`map[id]chan`；进程退出时让所有挂起调用失败。
- **单写协程**；入站通知在单协程中按序分发；入站请求（权限、表单）在各自的协程中等待答复。
- **取消**：`session/prompt` 的 ctx 取消时发送 `session/cancel`，其他请求发送 `$/cancel_request`。
- **类型**：只覆盖用到的方法与更新种类；未知的 `sessionUpdate` 保留为原始 JSON 交给上层忽略。

测试数据中固定 `schema-v1.27.0` 的 `schema.json` 与 `meta.json`，用一个守卫测试核对“我们使用的方法名与字段名都存在于该 schema 中”。升级 schema 时更新文件并重跑测试。

**备选方案：**

- `coder/acp-go-sdk`：已停更五个月，缺少需要的更新种类，并且会记录原始行，违反“不记录终端内容”的约定。
- `caelis-labs/acp-go-sdk`：维护活跃、Apache-2.0，但规范每周更新，引入生成代码的面积和变更频率远大于约 15 个方法的需求。

两者都不采用。如果以后需要 v2 或大面积能力，可以重新评估只用它的传输层。

### 4. 规范化事件：转换层，而不是直接转发 ACP

`hybrid` 把 ACP 更新转换为 webmux 自己的事件。事件按 v2 的方向设计：以 `messageId` / `toolCallId` 做 upsert。

| 规范化事件                                                                                           | 来源（ACP v1）                                                                                     |
| ---------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------- |
| `message.append {messageId, role: user\|agent\|thought, text}`                                       | `*_message_chunk`、`agent_thought_chunk`；缺少 `messageId` 时由服务端为连续片段分配                |
| `tool.upsert {toolCallId, kind?, title?, status?, content?, locations?}`                             | `tool_call`、`tool_call_update`（字段补丁语义）                                                    |
| `plan.set`、`commands.set`、`config.set`、`usage.set`、`info.set`                                    | `plan`、`available_commands_update`、`config_option_update`、`usage_update`、`session_info_update` |
| `compaction.upsert`、`notice`                                                                        | `compaction_update` / omp 文本形式（档案标注）、`notice`                                           |
| `turn.start {turnId, promptId}`、`turn.end {stopReason}`                                             | `session/prompt` 的发送与响应                                                                      |
| `request.open {requestId, kind, toolCallId?, options \| form}`、`request.close {requestId, outcome}` | `session/request_permission`、`elicitation/create`                                                 |
| `marker {kind: loaded \| switched \| truncated \| reconnected}`                                      | 服务端自身                                                                                         |

- 理由：前端与 `contract` 只依赖这一套模型；ACP v2、私有扩展或非 ACP 后端只需要换转换层。服务端本来就需要解析权限、`stopReason`、配置项与用量，才能完成关注、队列、D25 与休眠判定。
- 单个字段上限 256 KiB（例如工具输出），超出部分截断并加标记，避免一个事件撑满日志。
- 内容块只渲染 `text`、`diff` 与 `resource_link`；其他种类显示“暂不支持的内容（类型）”占位。

**备选方案：服务端把 ACP JSON 原样转发给浏览器（与 PTY 字节透传对称）。** 契约会变成“ACP 的任意版本”，R8 快照和 Zod 校验失去意义，v2 迁移需要同时改服务端和前端。不采用。

### 5. 线上契约：一条 JSON 流 + REST 命令

- **`GET /ws/s/{id}/chat`（文本 JSON）**是混合会话的控制与聊天通道，与所在端无关，始终可以连接：
  - 连接后先收到 `snapshot`：所在端、阶段/进度、资格与原因、当前 agent 会话 ID、回合状态、待答复请求、配置项、命令、计划、用量、队列、steering 能力、日志代际与起止序号；
  - 然后收到 `events {generation, from, items[]}`；
  - 之后是增量 `state` 与 `events`。
  - 客户端以 `?generation=&seq=` 续接：仍在日志内时只补缺失部分，否则重置。语义与终端流的 offset 续接对称。
  - 慢客户端以 4408 断开，与现有规则一致。
- **`/ws/s/{id}`** 只在 `tui` 端可连接。会话在聊天端或切换中时，以 4409 关闭并附上原因；运行被切走时，已有连接同样以 4409 关闭。4409 沿用“4xxx 对应 HTTP 状态”的现有约定（4404、4408、4410）。
  - `ws.ts` 新增终止状态 `moved`，收到 4409 时不重连，由混合会话页切换视图。
  - `webmux attach`：被切走视为被动分离，退出 0，并在标准错误说明原因；在聊天端拒绝附着时退出 1。
- **REST 命令**：与 `buffer`、`upload`、`code` 一样挂在会话下，因为混合会话的 ID 在整个生命周期（包括可恢复阶段）都不变：
  - `/api/sessions/{id}/chat/` 下的 `prompts`、`queue/{qid}`、`queue/{qid}/send-now`、`cancel`、`requests/{rid}`、`config/{configId}`；
  - `POST /api/sessions/{id}/switch {to, force?}`、`POST /api/sessions/{id}/resume {side, confirmWithoutEnv?}`；
  - 记录不是活动会话，单独使用 `GET /api/hybrid-sessions?state=recoverable`、`GET /api/hybrid-sessions/{id}`（会话页在会话不存在时的回退）与 `DELETE /api/hybrid-sessions/{id}`；
  - 设置页的勾选框使用只读的 `POST /api/launch-configs/hybrid-support {cmd}`，由服务端按档案判定，浏览器不复制规则。
  - 每个改动状态的请求都带 `clientRequestId`，服务端保留最近 256 条、30 分钟用于去重，与提交帧一致。
  - 结果同时以事件广播给所有设备，因此多设备一致不依赖请求方。
  - 错误映射：`hybrid.ErrSwitching`、`ErrWrongSide` → 409；`ErrNotEligible` → 409 并附原因码；`ErrAnswered` → 409；`ErrUnavailable` → 503；`ErrNotFound` → 404。
- **`SessionInfo.hybrid`**（可选对象）：`side`、`phase`、`dormant`、`state`（active/closed/interrupted）、`agent`、`agentSessionId`。普通会话省略该字段，现有快照不变。
- 所有新类型放在 `internal/contract`，转换放在 `internal/wire`，新增快照场景 `hybrid`。前端 schema 可以放在独立模块中，由 `schemas.ts` 重新导出，保持“一处契约入口”。

**备选方案：**

- **把聊天帧放进 `/ws/s/{id}` 的类型字节中**：终端协议需要判别两种负载，`attach` 和终端 hook 都要处理与自己无关的帧。不采用。
- **命令也走 WebSocket**：需要自建请求/应答与重试，与 `api.ts` 的错误、认证和 Zod 校验重复。不采用。

### 6. 每会话 actor 与单写者

每个混合会话由一个 goroutine（actor）拥有全部可变状态：所在端、阶段、ACP 连接、队列、待答复请求、日志游标。命令、ACP 通知、进程退出、计时器都作为消息投递给它。

- 切换中的阻塞步骤（等待空闲、等待退出、`session/load`）都以“子任务 + 完成消息”的方式运行，actor 本身不阻塞，所以切换期间仍能回答查询、拒绝新命令（409）。
- 订阅者的发布沿用有界队列，不在锁内做网络或进程操作（AGENTS 约束）。
- 状态机：

```text
tui ──switch──▶ switching(tui→acp: interrupt → exit → spawn → load → apply) ──ok──▶ acp
acp ──switch──▶ switching(acp→tui: cancel → close → stop → start-run)       ──ok──▶ tui
switching ──fail──▶ 原端（回滚，D22）
acp ──idle≥N──▶ acp+dormant ──open/prompt──▶ acp（resume 或 load）
tui ──TUI 自行退出──▶ interrupted（D23）
acp ──进程意外退出──▶ 重连一次（D24）──fail──▶ interrupted
any ──close──▶ closed（记录保留）
```

- **单写者证明**：只有 actor 能启动或结束运行与 ACP 进程。进入下一端之前必须先观察到上一端的进程退出事件（PTY 运行的 `cleanupDone`、ACP 进程的 `Wait`），因此任何时刻至多一个进程打开该 ID。这是 omp 的硬性要求：TUI 未释放租约时，ACP 的写入会把会话分叉成新 ID。资格探测用的临时 ACP 进程只调用 `session/list`，不调用 `load`/`resume`，不算写者。
- 多设备同时点击切换时，后到的请求在 actor 中得到 409 `switching`。

### 7. 伴随进程（ACP 子进程）

`session` 新增 `Shell.StartSidecar(argv) (*Sidecar, error)`：

- 使用与 PTY 运行相同的环境构造（`sessionEnvWithVersion`，因此带有同一个 `WEBMUX_SESSION_ID`，与 D15 的环境一致）；
- 新进程组；
- 经 `startChild` 登记到 `managedLeaders`；
- stdin/stdout 管道；stderr 排空到 64 KiB 的有界环。stderr 不写日志，只在启动失败时作为错误详情返回给用户。
- 结束时依次关闭 stdin、等待宽限时间，再用现有的 `cleanupProcesses` 发送 SIGHUP/SIGKILL。

这样 Linux subreaper 不会抢走退出状态，进程树检查与关机清理覆盖 ACP 进程，不出现第二套清理逻辑。

**bashrc 包装的 stdout 污染（D18 遗留风险）**：ACP 进程以 `Stdout=stderr 管道`、`ExtraFiles=[stdout 管道]` 启动，argv 为 `bash -ic 'exec -- "$@" 1>&3 3>&-' webmux <agent> acp …`。`.bashrc` 期间的输出进入 stderr，只有 agent 本身的 stdout 进入协议通道。直接执行的写法不需要这层处理。M0 已实测：两个 agent 握手成功，`.bashrc` 导出的变量对 agent 的工具可见；不加 fd 3 时 `.bashrc` 的输出混入协议通道（`baseline.md`）。

### 8. 身份信号与资格判定

- **采集（`session`，与 agent 无关）**：扫描器在保持现有关注与状态行为的前提下，额外记录有界、校验过字符集的标识字段：
  - Warp 事件的 `agent`、`plugin_version`、`session_id`、`cwd`；
  - 7501 根记录的 `app`；
  - 7501 子记录的首段 `id`。

  这些字段只保存在内存中，经 `Manager.Identity(id)` 提供给 `hybrid`，不进入普通会话的 `Info`、日志或推送。根进程的 `pid`、可执行文件名与 cwd 来自进程表（新增 Linux `/proc/<pid>/cwd`、macOS `PROC_PIDVNODEPATHINFO`），一并提供。

- **解释（`agentprofile`）**：档案把原始字段映射为 `{agent, version, agentSessionId, cwd}`。例如：omp 取 Warp 的 `session_id` 与 `cwd`；OpenCode 取 `app=opencode`、子记录 ID（只接受 `ses_` 前缀）与根进程 cwd。
  - 按 7501 规范，子记录 `id` 本意是任务子树 ID，OpenCode 是借用。因此只有 OpenCode 档案解释它，`session` 层只做有界采集。如果 OpenCode 以后改变用法，退路是 D11 原方案：按 `updatedAt` 列出候选，由用户确认。
  - 这需要在 `program-status` 中加一条例外（本变更的 `specs/program-status` 增量），前提是先归档 `add-program-status-signals`（tasks 0.1）。
- **判定（`hybrid`）**：D13 的五个条件里，①②③⑤ 由内存信息即时计算；④ 在 ID 或 cwd 变化后去抖 1 秒，再用临时 ACP 进程执行一次 `session/list {cwd}`，并在切换前再执行一次。全局最多两个并发探测。结果与原因码进入 `snapshot`/`state`。
- **占用检测（D26）**：遍历 `Manager` 中所有会话的身份，普通会话也算在内。

### 9. 档案（第一期）

| 字段                   | OpenCode                                                                                                                               | omp                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            |
| ---------------------- | -------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 可执行文件             | `opencode`                                                                                                                             | `omp`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| TUI argv（首次与恢复） | 档案注入 `--standalone`，TUI 自带服务，结束 TUI 即停止回合（M0 实测：运行中的工具被结束并记为 `failed`）                               | 原样                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           |
| ACP argv               | `opencode acp`（只接受 `--login`；白名单参数不透传，`--auto` 在界面提示不支持）                                                        | `omp acp` + 全部白名单参数（`omp acp` 解析完整 argv、静默忽略未知参数；M0 实测 `--model`、`--thinking`、`--profile`、`--approval-mode`、`--auto-approve` 生效）                                                                                                                                                                                                                                                                                                                                                                                                                                                                                |
| 恢复 argv              | `opencode --standalone [白名单] -s {id}`                                                                                               | `omp [白名单] --resume {id}`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| 白名单                 | 带值：`--log-level`；开关：`--standalone`、`--auto`、`--print-logs`                                                                    | 带值：`--model`、`--smol`、`--slow`、`--plan`、`--prewalk-into`、`--provider`、`--api-key`、`--system-prompt`、`--system-prompt-template`、`--append-system-prompt`、`--profile`、`--config`、`--add-dir`、`--models`、`--tools`、`--thinking`、`--service-tier`、`--hook`、`-e`/`--extension`、`--skills`、`--max-time`、`--warn-cost`、`--max-cost`、`--approval-mode`、`--plugin-dir`；开关：`--prewalk`、`--no-prewalk`、`--allow-home`、`--no-tools`、`--no-lsp`、`--no-pty`、`--hide-thinking`、`--advisor`、`--external-thinking`、`--no-extensions`、`--no-skills`、`--no-rules`、`--no-context-files`、`--no-title`、`--auto-approve` |
| 丢弃的参数             | `-c`/`--continue`、`-s`/`--session`、`--prompt`，以及白名单之外的参数                                                                  | 位置参数（提示词）、`-c`/`--continue`、`-r`/`--resume`（值可省略）、`--restore`、`--goal`、`--plan-yolo`、`--plan-yolo-into`、`--from-claude`、`--from-codex`、`--wait-async`、`--print-thoughts`、`--no-ui`，以及白名单之外的参数                                                                                                                                                                                                                                                                                                                                                                                                             |
| 不可勾选的参数         | 位置参数（工作目录）、`--server`（另一个服务的会话库）；`-h`/`--help`、`-v`/`--version`、`--completions`、`--wizard`（不启动交互界面） | `--session-dir`、`--no-session`、`--cwd`；`-p`/`--print`、`--mode`、`--export`、`--alias`、`-h`/`--help`、`-v`/`--version`、首个位置参数为子命令（不启动交互界面）                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| 中断键                 | 两次独立的 `Esc`，间隔 150 毫秒（实测 0.2 秒回到空闲；一次写入的 `\x1b\x1b` 被当作 Alt+Esc，不会中断，8.3 验收时发现）                 | `Esc`（实测 0.09 秒回到空闲）                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| 退出方式               | 回到空闲后对进程组发送 SIGHUP（复用 `Close`），5 秒后 SIGKILL；实测 0.1–0.15 秒退出，退出码 0                                          | 同左；实测 0.05 秒退出（因信号结束）                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           |
| 加载后需补齐的配置项   | 无（`model`、`mode`、`effort` 都随会话恢复）                                                                                           | `mode`（加载后回到 `default`；`model`、`thinking` 随会话恢复）                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 |
| 身份信号               | 7501 子记录 `id=ses_…`，首条消息创建会话后才出现，回合结束以 `state=clear:id=ses_…` 收尾；cwd 取根进程 cwd                             | Warp `session_start` 的 `session_id`、`cwd`，TUI 启动即发出；首条消息之前会话尚未写盘，`session/list` 查不到                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| steering               | 无                                                                                                                                     | `_session/steering`，且要求 `initialize` 结果顶层 `_meta.steering.supported`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| 压缩结果               | `compaction_update`                                                                                                                    | 回合文本（标注为压缩）                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| 已实测版本             | 2.1.4（ACP 与 TUI）；2.2.0（ACP 与 `--help`，与 2.1.4 相同）                                                                           | 18.13.1                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        |

参数解析按档案声明的元数（`--flag value` 与 `--flag=value` 两种写法）进行；未知参数一律丢弃，并在界面上列出。白名单以 M0 的 `--help` 输出与实测为依据（`baseline.md`），写入档案测试。

**cwd 一律解析符号链接**：两个 TUI 都记录解析后的路径（即使 `PWD` 是符号链接路径），omp 的 `session/list` 两种路径都能匹配，OpenCode 则按字符串精确匹配。因此 `hybrid` 传给 ACP 的 cwd 一律先 `filepath.EvalSymlinks`。

### 10. 事件日志、续接与休眠

- 日志按事件编码后的字节数计量，上限取 `scrollback_bytes`（D28）。超出时丢弃最旧的事件，`snapshot.from` 前移，前端显示“更早的历史未缓存”。
- **代际**：每次 `session/load` 产生新代际，日志清空后由回放重建，客户端收到 `reset`。如果档案声明并且 agent 声明了 `sessionCapabilities.resume`，休眠唤醒（D27）和崩溃重连（D24）优先使用 `session/resume`（不回放），代际与日志保持不变，只追加 `marker`；否则用 `load` 并换代。
- **休眠**：没有运行中的回合、没有待答复请求、队列为空，且最近 N 分钟没有命令时，actor 执行 `session/close` 并结束进程，状态为 `acp + dormant`。打开 `/ws/s/{id}/chat` 或发送命令时唤醒。
- **N 的来源**：`settings.json` 中的 `chatIdleSleepMinutes`，默认 30，0 表示关闭，网页可改、多设备同步、立即生效（D27）。`hybrid` 是第一个订阅 `settings` 变更的服务端服务，经 `cmd/` 注入；设置不可用时使用默认值 30。不放在 `config.toml`，因为那里的修改需要重启，且网页不可编辑。

### 11. 记录存储

- 记录存在 `hybrid-sessions.json`（v1，带锁，0600），所有者 `internal/hybrid`，故障策略为 `unavailable`：只禁用混合会话的创建与恢复，普通会话不受影响。
- 每条记录包含：ID、名称、`configId`、档案、cwd、argv 快照、所在端、`agentSessionId`、最近 8 个历史 ID、配置项取值（D25）、状态（`active`/`closed`/`interrupted`）、最后状态、时间戳，以及根进程 `pid` 与启动标识（复用 `runtime` 的进程身份）。
- 不保存环境变量值、消息、队列或 token。上限 200 条，超出时拒绝新建并提示移除旧记录。
- 启动时，`active` 记录先检查残留进程：PID 与启动标识都匹配时结束其进程组。然后标记为 `interrupted`。
- 状态迁移、ID 变化、配置变化时写盘，同一会话 1 秒去抖。

### 12. 前端结构

- `web/src/chat-model.ts`：纯函数 reducer，规范化事件 → 时间线条目。在 Node 中做单元测试，包括乱序补丁、截断与重置。
- `hooks/chat-socket.svelte.ts`：复用 `ws.ts` 的 `connect` 与重连约定，处理代际/序号续接。
- `hooks/hybrid.svelte.ts`：组合所在端、切换动作、资格提示、草稿退回。
- `components/chat/*`：由 PR #186 的 `web/stories/acp/*` 提升而来，原型 stories 改为引用正式组件。
- `pages/Terminal.svelte`：`info.hybrid` 存在时，按所在端在同一窗格内渲染终端或聊天；终端端仍使用现有 `useTerminal`。
- 首页：在现有分组下方增加“可恢复”分组（`GET /api/hybrid-sessions?state=recoverable`），会话列表事件触及混合会话时重新获取。
- 聊天输入框复用会话草稿持久化（US-F）；D20 退回的文本只合并进发起切换的设备的草稿，其他设备只收到提示，避免多份副本被重复发送。
- 草稿清理规则修订（`specs/terminal-draft-persistence`）：可恢复的混合会话视为存在，草稿保留到记录被移除或 7 天过期；4409 不视为会话不存在。

### 13. 里程碑（每个都是独立 PR）

| 里程碑 | 内容                                                                                                                                       | 用户可见         |
| ------ | ------------------------------------------------------------------------------------------------------------------------------------------ | ---------------- |
| M0     | 真实 agent 基线：omp/OpenCode ACP 能力、白名单、退出方式、OpenCode `--standalone` 的回合停止、bashrc fd 技巧、cwd 符号链接、D25 默认值判定 | 否               |
| M1     | `internal/acp` + 假 agent + schema 守卫                                                                                                    | 否               |
| M2     | `session` 外壳/运行分离、伴随进程、身份与 cwd、4409                                                                                        | 否（回归零差异） |
| M3     | `agentprofile`、`launch.hybrid` 字段与校验、记录存储登记                                                                                   | 否（API 字段）   |
| M4     | `hybrid`：从聊天端开始、事件流、REST 命令、队列、权限/表单、配置、关注                                                                     | 否（API）        |
| M5     | 双向切换、资格、回滚、休眠、重连、中断/恢复、CLI                                                                                           | 否（API/CLI）    |
| M6     | 前端聊天视图、切换 UI、可恢复分组、勾选框、起始端、设置                                                                                    | 是               |
| M7     | 文档、用户故事、E2E 截图、真实 agent 验收                                                                                                  | —                |

### 14. 评审确认的取舍（2026-10-28）

以下取舍在 PR #187 评审中由维护者按推荐确认；事实依据见 Context 与 `research.md`。

| #   | 问题                                   | 采用                                                                              | 未采用                                                                            |
| --- | -------------------------------------- | --------------------------------------------------------------------------------- | --------------------------------------------------------------------------------- |
| 1   | 本地终端被切走或被拒绝时的退出码       | 被切走 = 被动分离，退出 0；在聊天端拒绝附着，退出 1                               | 专用退出码 75（可能与 agent 退出码重合）；被切走退出 1（切换并非失败）            |
| 2   | `launch-configs.json` 是否升版本       | 保持 v2，只加可选字段，为 false 时不写入                                          | 升到 v3（按“不双读”规则所有用户都要迁移）；勾选状态另存（同一份配置分在两个文件） |
| 3   | TUI 自行退出后的外壳与草稿             | 外壳以“已退出 + interrupted”保留，可回放最后输出；可恢复会话的草稿保留            | 立即移除外壳（看不到崩溃输出）                                                    |
| 4   | OpenCode 的 TUI                        | 档案注入 `--standalone`（以 M0 实测为前提）                                       | 保持原命令（强制切换后回合可能在后台继续）                                        |
| 5   | 让两端看到不同目录或会话库的参数       | 不可勾选：OpenCode 位置目录，omp `--session-dir`/`--no-session`                   | 把位置目录当作 cwd（增加 cwd 来源分支）                                           |
| 6   | 接口路径                               | 挂在 `/api/sessions/{id}/…`、`/ws/s/{id}/chat`；记录单独用 `/api/hybrid-sessions` | 独立命名空间 `/api/hybrid`、`/ws/hybrid`                                          |
| 7   | 空闲休眠设置                           | `settings.json` 的 `chatIdleSleepMinutes`                                         | `config.toml`（需要重启，网页不可改）                                             |
| 8   | 与 `add-program-status-signals` 的顺序 | 先归档它，本变更再修改 `program-status`；只有 OpenCode 档案解释 `ses_` 子记录     | 由后归档的一方处理（两份规格一段时间内互相矛盾）                                  |
| 9   | D13 ④ 的探测频率                       | 维持 D13：ID 变化时与切换前各探测一次                                             | 只在切换前探测（按钮先可用、点了才报错）                                          |

## Risks / Trade-offs

- **外壳/运行拆分触及会话核心** → 先以纯重构合入，并保证普通会话的 wire 快照、`-race` 全套与回放/退出顺序测试不变；新增路径只对带驱动的外壳生效。
- **agent 版本更新导致信号或参数漂移** → 档案集中维护；D13 ④ 的实测核对兜底；版本低于实测值时只提示（D30）；E2E 使用假 agent，真实 agent 只在基线中验证。
- **在 webmux 之外打开同一会话**会破坏单写者 → 无法检测，写入用户文档（D26）。
- **临时探测进程的开销**（约 0.5 秒/次）→ 去抖、并发上限，只在 ID 变化与切换前执行。
- **OpenCode 的 TUI 默认是后台服务的客户端**，强制切换可能在后台留下仍在运行的回合 → 档案注入 `--standalone`，让结束 TUI 即停止回合。M0 必须实测确认：结束 standalone TUI 会停止回合，并且历史完整。不成立时退回“保持原命令 + D21 提示后台可能继续”，并同步修订 `hybrid-agent-profiles` 与 `hybrid-sessions` 中的相应条文。
- **注入 `--standalone` 的代价**：每个混合 TUI 启动一个私有服务，启动稍慢，看不到后台服务中其他客户端的实时事件。两者共享 SQLite，历史不受影响。
- **事件日志按字节截断**可能切掉工具调用的开头，后续补丁无处可依 → reducer 对未知 `toolCallId` 的补丁创建占位条目，标注“开头未缓存”。
- **被切走时退出 0，脚本无法区分“被切走”与“按了分离键”** → 两者语义相同（会话仍存活），标准错误有原因；需要区分时，脚本可以用 `webmux ls --json` 查询 `hybrid.side`。
- **`launch-configs.json` 加字段不升版本** → 回退到 #147 之后的旧版本时，只有真正勾选过的配置会让旧版本报告“文件损坏”，启动配置返回 503，文件不被改写。发布说明提示“回退前先取消所有混合勾选”。升到 v3 的话，按“不双读”规则所有用户都要迁移，代价更大。
- **资格探测会短暂启动 agent**：omp 每次启动会写 presence 文件、安排插件市场自动更新，还可能执行目录迁移；OpenCode 每次启动会拉起私有服务。→ 只在 ID 变化与切换前探测，并在用户文档中说明。

## Migration Plan

- 新字段和新文件都是增量：没有勾选混合模式的配置与会话不受影响，不需要迁移。
- 回滚：取消所有配置的混合勾选即可停用。降级到旧版本前，先关闭混合会话并取消所有混合勾选，否则旧版本的启动配置功能返回 503；旧版本会忽略 `hybrid-sessions.json`（未登记的文件）。
- 产品边界文字（“会话不跨服务重启”）在 M7 随文档一起更新，只放开混合会话。

## Open Questions

M0 已关闭此前的两项待定：档案白名单按实测定稿（决策 9）；omp `--restore` 不在本期使用（唤醒与重连使用 `session/resume`），归入丢弃参数。目前没有待定问题。
