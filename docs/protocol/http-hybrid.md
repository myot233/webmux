# HTTP 与 WebSocket：聊天视图与混合会话

用开启了聊天视图（界面称“混合模式”）的启动配置创建的会话有两个视图：原生 TUI（终端视图 `terminal`）与 ACP 聊天视图（`chat`）。同一段 agent 对话在两个视图之间切换，webmux 会话 ID 始终不变。

- 线上类型：[`internal/contract/views.go`](../../internal/contract/views.go)、[`sessions.go`](../../internal/contract/sessions.go) 与 [`chat.go`](../../internal/contract/chat.go)。
- 前端镜像：[`web/src/schemas.ts`](../../web/src/schemas.ts) 与 [`web/src/chat-schemas.ts`](../../web/src/chat-schemas.ts)。
- 认证、Origin 与错误格式：同 [API 与协议](../protocol.md)；创建见 [创建会话](http-session-create.md)，输入、中断、列表与删除记录等通用操作见 [会话](http-sessions.md#会话)。

## 会话信息中的视图与 agent

带聊天视图的会话与普通会话使用同一个 `SessionInfo`（完整定义见 [会话](http-sessions.md)），不含 `hybrid` 对象：

| 字段         | 说明                                                                                                                                                   |
| ------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `state`      | `active`、`closed` 或 `interrupted`；运行中列表与 `/ws/sessions` 只含 `active`                                                                         |
| `views`      | `["terminal","chat"]`                                                                                                                                  |
| `view`       | 最近一次落定的视图，`terminal` 或 `chat`；切换进行中仍为来源视图                                                                                       |
| `transition` | 切换进行中为 `{to, phase}`，`phase` 取 `interrupting`、`awaiting_choice`、`cancelling`、`stopping`、`starting`、`loading`、`rolling_back`；否则 `null` |
| `agent`      | `{name, title, icon, sessionId, work?}`，见下文                                                                                                        |
| `archive`    | 只在 `closed` 或 `interrupted` 时出现，见下文“已归档会话”                                                                                              |

`agent` 的字段：

- `name`：档案名，`opencode`、`omp`、`codex`、`claudecode`、`agy`、`qoder`、`qodercn` 或 `kimi`；Kimi 的恢复、配置与输入差异见 [agent 接入说明](../hybrid-sessions/agents.md#kimi-code-cli)。
- `title`：档案的显示名，如 `OpenCode`、`Claude Code`；会话卡片与聊天页头部用它标识 agent。
- `icon`：档案的品牌图标键，取值属于启动配置的图标目录（如 omp 为 `pi`）；浏览器不认识时只显示 `title`。
- `sessionId`：agent 自己的会话 ID；尚未得知时为 `null`。
- `work`：聊天视图回合进行中，一行可读的当前工作（工具调用标题，或无标题时按种类与首个位置、进行中的计划步骤）；回合结束时省略。

会话在聊天视图时，`status`、`attention`（来源 `agent`）与 `preview`（最新 agent 消息前三行）来自聊天视图的回合状态；`work` 只在回合进行中（含等待授权）存在；`cmd` 为启动配置的命令。

## 视图切换与恢复

- POST `/api/sessions/{id}/view`：`{clientRequestId, to, agentSessionId?}` 切到另一视图，完成后 200 `{view, draft?, dropped, unsupported}`。
  - `agentSessionId` 只用于切到 `chat`，从 409 `session_ambiguous` 的候选中选定会话。
  - 中断超时进入 `awaiting_choice` 后，另一请求以 `{clientRequestId, choice: "wait" | "force" | "cancel"}` 作答。
  - 普通会话 409 `no_chat_view`。
- POST `/api/sessions/{id}/resume`：`{clientRequestId, view?, agentSessionId?, confirmWithoutEnv?}` 把已关闭或已中断的会话恢复到指定视图（缺省为归档时的视图），沿用原 ID，200 返回 `state` 为 `active` 的 `SessionInfo`。
  - 会话仍为 `active`（含所有普通会话）时 409 `not_archived`；切换中或同一会话正在恢复、删除记录时 409 `switching`。
  - 请求体中的 `side` 作为未知字段返回 400。
- DELETE `/api/sessions/{id}`：关闭会话：先取消聊天视图回合或按中断流程处理终端视图回合，再结束进程，`state` 改为 `closed`，会话离开运行中列表；切换中 409 `switching`。
- DELETE `/api/sessions/{id}?forget=1`：删除归档记录与临时文件夹，不删除 agent 的会话库；对运行中的会话先关闭再删除；切换中 409 `switching`。界面只对已归档会话提供“删除记录”。

切换结果中：`draft` 是切到终端时退回的排队消息（按顺序以空行连接），只返回给发起切换的请求，其他设备收到提示事件；`dropped` 为恢复命令未保留的启动参数；`unsupported` 为聊天视图不支持的参数。切换失败时会话回到原视图并返回 502 与原因；回滚也失败时会话标记为 `interrupted`。

## 已归档会话

关闭或中断的会话只存在于混合会话记录（`hybrid-sessions.json`）中，经会话接口读取：

- `GET /api/sessions?state=archived`（或 `closed`、`interrupted`、`all`）列出它们，按 `archive.updatedAt` 倒序；`GET /api/sessions/{id}` 读取单个。
- `/ws/sessions` 只描述运行中会话：会话被关闭或中断时发送 `removed` 事件，客户端按需重新读取已归档列表。
- 工作树删除预览的 `archived` 列出该工作树中的已归档会话。

已归档会话的运行时字段取零值（见 [会话](http-sessions.md)），`archive` 给出记录独有的字段，不含环境变量值、消息、队列正文或 token：

- `configMissing`：启动配置是否已删除；
- `previousAgentSessionIds`：最近 8 个历史 agent 会话 ID（当前 ID 在 `agent.sessionId`）；
- `activity`：中断前的活动 `idle`、`turn` 或 `switching`，后两者表示上一轮可能未完成；
- `queuedCount`：中断时丢失的排队条数；
- `interruptedAt` 与 `updatedAt`：时间戳，`lastActiveAt` 等于 `updatedAt`。

已归档会话的 `title` 跟随运行中会话实际展示的标题，`name` 为创建名称；首页“已归档”区优先显示 `title`，旧记录为空时回退到创建名称。`workspace` 是会话最近一次的仓库关联，在关联变化、归档、中断与服务关机时保存；会话没有关联或记录早于该字段时为 `null`，首页按 `cwd` 归入包含它的已注册工作树。

### 恢复与身份核对

恢复时命令取自记录中的快照，环境变量按 `configId` 重新读取。启动配置已删除且未确认时返回 409 `config_missing`；以 `confirmWithoutEnv: true` 重试则不带环境变量恢复。

随后服务端启动一个只调用 `initialize` 与 `session/list` 的临时 ACP 进程，在记录的 cwd 中核对 agent 会话：

- agent 会话已不存在：返回 409 `session_missing`，`candidates` 为同一目录下的会话（`{agentSessionId, title, updatedAt}`，按更新时间倒序，最多 20 个）。以 `agentSessionId` 重试即改用所选会话，原 ID 进入历史。
- 该会话正被其他会话打开：返回 409。
- 档案的连接器未安装：恢复到聊天视图返回 409 `connector_missing`；恢复到终端视图跳过核对照常执行。
- 记录中没有 agent 会话 ID：不核对，所选视图开始新对话。

聊天视图打开会话时 Codex 报告会话正被其他 Codex 客户端使用（`thread_active_writer`），恢复返回 409 `occupied_external`，切到聊天则回到终端视图并以同样原因失败。

Claude Code 使用 `<CLAUDE_CONFIG_DIR>/sessions/<pid>.json` 注册记录取得 TUI 当前会话 ID；未设置配置目录时读取 `~/.claude`，不解析 transcript JSONL。原生进程使用根 PID，允许的 `node` 启动器使用其唯一直接 Claude 子进程 PID。

采用记录前验证记录内外 PID、UUID、规范化 cwd、进程启动标识与 PID domain。Linux 的 `procStart` 是 `/proc/<pid>/stat` 第 22 字段的十进制 ticks，`pidDomain` 为 `linux:<machine-id>:<pid-namespace>`。

注册记录按既有身份观察节奏重读，以跟踪 `/new` 与 `/resume`。记录不可用或验证失败时退回同一 cwd、本次 TUI 启动后更新的 `session/list` 候选：单候选采用，多候选返回 409 `session_ambiguous`，零候选保持不可切换。

即使注册记录已有 UUID，首条消息前 `session/list` 尚无可加载会话时仍返回 `no_session_id` 对应的资格原因，不尝试 `session/load`。

同一配置目录的有效注册记录还用于尽力检测外部 Claude 写者，排除当前进程、已退出进程与启动标识不匹配的记录；命中返回 409 `occupied_external`。来源不可读时仍保留 WebMux 内部占用检测，但不保证发现所有外部客户端。

进程事实与注册记录只用于服务端资格判定。读取频率、单条大小与扫描条目有界；注册路径、PID、启动标识、记录正文和凭据不进入 REST/WebSocket 负载、混合记录或日志，普通会话不读取 Claude 注册记录。

核对通过后，恢复到终端视图在 PTY 中执行档案生成的恢复命令（Claude Code 为 `--resume <id>`）；恢复到聊天视图启动 ACP 进程并 `session/load` 或支持的 `session/resume`，再按档案补回记录中的配置项取值。

Claude Code 在两种冷恢复后都重应用 `model` 与 `effort`。`mode` 采用连接器从 transcript 恢复的实际值，不用旧 argv 覆盖；运行条件导致模式回退时以下发的实际配置与通知为准。

成功后会话的 `state` 改为 `active`；中断时丢失过排队消息的，聊天状态的 `queueLost` 为丢失条数。恢复失败时记录保持原样。

## 启动配置

`GET/POST/PUT /api/launch-configs[/{id}]` 的配置与摘要带 `chat`（布尔，界面称“混合模式”）与只读 `chatSupport: {supported, agent?, reason?, connector?, supportedAgents}`。

- 请求体可带 `chat`；带 `chatSupport`、`hybridSupport` 或旧字段 `hybrid` 都视为未知字段返回 400。
- `chat: true` 而命令不受支持时返回 400 并给出原因；保存时不检查连接器是否已安装。

混合模式接受的启动文件包装为 `["bash","-ic","exec -- \"$@\"","webmux",…]` 或 `["zsh","-ic","exec -- \"$@\"","webmux",…]`；恢复与 ACP 命令沿用同一 shell。

`POST /api/launch-configs/chat-support {cmd}` 只读返回同样的 `chatSupport`，供设置页去抖查询（旧路径 `/api/launch-configs/hybrid-support` 已删除）：

- `reason` 取值：`unknown_agent`、`unsupported_form`、`session_store`、`directory_arg`、`non_interactive`、`session_arg`。
- `supportedAgents`：注册表中全部受支持 agent 的显示名（总是数组），用于原因与说明文案，前端不写死 agent 列表。
- `connector` 只在档案需要第三方 ACP 连接器时出现，来自档案，不表示已安装；Codex 为 `{executable: "codex-acp", title, installCommand, docsUrl}`，Claude Code 为同一形状且 `executable: "claude-agent-acp"`，安装命令为 `npm install -g @agentclientprotocol/claude-agent-acp`。

`launch-configs.json` 保持 v2，持久化键仍为 `hybrid`（由存储层映射为线上的 `chat`），为 `false` 时不写入；回退到旧版本时文件照常可读。

Claude Code 的 ACP 进程无参数执行 `claude-agent-acp`，`initialize` 固定请求 ACP v1。未显式配置时，服务端注入 `CLAUDE_CODE_EXECUTABLE` 为 TUI 实际 `claude` 的绝对路径；两端复用同一服务用户、`HOME`、`CLAUDE_CONFIG_DIR` 与已有认证，不另传或记录凭据。

连接器要求 Node.js 22+；安装命令跟随 `latest`，实测版本 0.99.1 与 Claude Code 2.1.316 只用于版本提示，不构成未来版本兼容保证。聊天命令隐藏 `login` 与 `logout`，认证失败仍走 `auth_required`。

## 聊天通道 `GET /ws/s/{id}/chat`

文本 JSON 帧，会话在任一视图时都可连接（普通会话在升级前返回 409 `no_chat_view`）；查询参数 `generation` 与 `seq`（客户端最后已应用的事件序号）用于续接。帧 `type`：

- `snapshot`：首帧。`state` 为完整聊天状态，`generation`、`from`（日志中最早的序号）、`next`；`reset: true` 表示所给位置已不在日志中，客户端须丢弃时间线；`truncated: true` 表示更早的事件已按字节上限丢弃。
- `events`：`{generation, from, items}`，snapshot 之后立即发送一帧补发（可为空），之后为实时增量。
- `state`：完整替换聊天状态（资格与原因、回合、待答复请求、配置项、命令、计划、用量、队列、是否可插话、`authRequired`、`promptCapabilities`、`permissionMode`、`forceSwitchNote`、`idChange`、`queueLost` 等）。其中几个字段如下：
  - 所在视图、切换阶段与 agent 身份不在聊天状态中（不再有 `side`、`phase`、`agent`、`agentSessionId`），只由会话信息的 `view`、`transition` 与 `agent` 提供。
  - `turn`：当前或上一回合；`startedAt` 在回合开始时记录，`endedAt` 在回合结束时记录，二者为 UTC RFC 3339 时间，聊天视图据此显示已用时间与结束后的总用时（刷新、重连和其他设备不重新计时）。回合以 `end_turn` 结束而仍有 `in_progress` 工具调用时带 `unfinished: true`，下一回合开始时清除。
  - `forceSwitchNote`：TUI 回合未停下时强制切换会损失什么，由该 agent 的适配器提供。Claude Code 明确 `session/cancel` 后后台 Bash 或子任务可能仍存活；强制切换终止连接器并等待受管后代退出，不保证任意自行 daemonize 的进程。
  - `promptCapabilities`：`{image, embeddedContext}`，来自 ACP `initialize`，连接断开时复位为 `false`。
  - `permissionMode`：`ask` 或 `auto`，见下文。
  - `eligibility`：`{available, reason?, connector?}`；`reason` 为 `connector_missing` 时 `connector` 给出未安装的连接器。
  - `outdated`：低于已验证版本的程序 `[{name, version, verified}]`，Codex 会分别列出 Codex CLI 与 codex-acp，Claude Code 会分别列出 `claude` 与 claude-agent-acp。
  - `connectorMissing`：聊天视图因连接器未安装而无法启动（重连、回滚）时给出连接器，安装后成功连接即清除。

关闭码：`4404` 会话不存在；`4408` 客户端过慢或写超时，可重连；`1013` 日志被重置，重连后从新代际开始；`4410` 会话已结束。

事件为规范化模型，不是 ACP 原文：

- 内容：`message.append`（按 `messageId` 追加，`role` 为 user/agent/thought）、`tool.upsert`（按 `toolCallId` 的字段补丁）。
- 会话元数据：`plan.set`、`commands.set`、`config.set`、`usage.set`、`info.set`（`title` 同时写回会话标题，创建名称仍优先）、`compaction.upsert`。
- 回合：`turn.start`（`{turnId, promptId, queueItemId?}`，发送排队提示的回合带该提示的 `queueItemId`）、`turn.end`（`stopReason` 含 `cancelled`、`error`、`auth_required`）。
- 请求：`request.open`、`request.close`。
- 提示与标记：`notice`、`marker`（`loaded`、`switched`、`truncated`、`reconnected`；`switched` 标记的 `view` 为切换后的视图，旧字段 `side` 已删除）。

单个文本字段超过 256 KiB 时截断并标 `truncated`；不支持的内容块以 `type: "unsupported"` 占位。日志按编码字节估算，上限为 `scrollback_bytes`；每次 `session/load` 开始新代际。

日志超出上限时，先缩减较旧事件中 tool 内容：文本与 diff 每块至多保留 2 KiB 并标 `truncated`，标题、状态、位置与序号不变；日志尾部约四分之一上限的事件不缩减，仍超出时才丢弃最早的事件。

已结束的同一条消息（相同 `messageId`，角色为 agent 或 thought，无附件）的连续 `message.append` 在日志中合并为一条：

- 合并事件的序号取最后一个块的序号，因此重放得到的序号可能不连续；客户端应以“序号不小于已应用序号才应用”去重，不要假定序号连续。
- 仍在流式中的消息和实时帧仍逐块发送。
- 续接点落在被合并的块之中时，只补发尚未应用的文字，序号为该组最后一个序号，不触发 `reset`。

## 聊天命令

所有改变状态的请求都带 `clientRequestId`（`[A-Za-z0-9_-]{8,64}`）。服务端按会话保留最近 256 条、30 分钟，重复请求返回首次结果，不再执行。

| 方法   | 路径                                           | 说明                                                                                                              |
| ------ | ---------------------------------------------- | ----------------------------------------------------------------------------------------------------------------- |
| PATCH  | `/api/sessions/{id}/chat/queue/{qid}`          | `{clientRequestId, text?, move?: "up"}` 编辑或上移，204                                                           |
| DELETE | `/api/sessions/{id}/chat/queue/{qid}`          | `{clientRequestId}` 删除，204                                                                                     |
| POST   | `/api/sessions/{id}/chat/queue/{qid}/send-now` | 取消当前回合后立即发送该条，204                                                                                   |
| POST   | `/api/sessions/{id}/chat/queue/resume`         | 暂停后继续，从队首发送，204                                                                                       |
| POST   | `/api/sessions/{id}/chat/queue/clear`          | 清空队列，204                                                                                                     |
| POST   | `/api/sessions/{id}/chat/requests/{requestId}` | 权限：`{outcome: "selected", optionId}` 或 `cancelled`；表单：`accept` 带 `content`、`decline`、`cancel`；204     |
| PUT    | `/api/sessions/{id}/chat/config/{configId}`    | 选择类 `{value}`、布尔类 `{enabled}`，经 `session/set_config_option` 生效并记录，204                              |
| PUT    | `/api/sessions/{id}/chat/permission-mode`      | `{clientRequestId, mode}`，`mode` 为 `ask` 或 `auto`（其他值 400）；两个视图均可设置，切换中 409 `switching`，204 |

普通会话调用这些接口返回 409 `no_chat_view`。发送提示与停止回合不在 `chat/` 下：

- 发送提示：`POST /api/sessions/{id}/input`（取代已删除的 `chat/prompts`），见下文。
- 停止当前回合：`POST /api/sessions/{id}/interrupt`（取代已删除的 `chat/cancel`），没有进行中的回合时不做任何事，204。

## 聊天视图的输入

会话在聊天视图时，`POST /api/sessions/{id}/input` 的 `{clientRequestId, text, attachments?, mode?}` 是一条提示：`mode` 为 `auto`（缺省，空闲且未暂停时立即发送，否则排队）或 `steer`（插话）；`attachments` 为上传接口返回的 `{path}` 数组；`submit` 被忽略，带 `keys` 返回 409 `wrong_view`。
会话在终端视图时，带 `attachments` 或 `mode` 的输入返回 409 `wrong_view`，不写入终端。

发送结果 `{view: "chat", queued, itemId?, turnId?, promptId?}`：

- 排队时给出 `itemId`，之后发送它的回合在 `turn.start` 中带同一个 `queueItemId`；
- 立即发送时给出与 `turn.start` 相同的 `turnId` 与 `promptId`；
- 插话两者都不给出。

`text` 不超过 64 KiB；有附件时可以只发附件。ACP 自动重连期间提交的自动发送消息进入队列等待连接恢复。回合以 `cancelled`、错误、`refusal` 或 `auth_required` 结束时，非空队列暂停。

插话只在档案声明、agent 在 `initialize` 中声明支持且有回合运行时可用；omp、Codex 与 Claude Code 的档案声明该能力。

Claude 的 `session/cancel` 只结束当前 ACP 回合，不表示后台任务或整个进程树已清理。

附件只能是本会话 `POST /api/sessions/{id}/upload` 存入临时文件夹 `uploads/` 的文件：

- 名称取上传时的文件名；MIME 与大小按文件内容推断，忽略客户端声明。
- 允许的图片类型为 PNG、JPEG、GIF、WebP，SVG 不作为图片块发送。
- 其他会话的上传、`uploads/` 之外的路径、符号链接与非普通文件返回 400 且不读取内容。

单个附件不超过 10 MiB，单条消息合计不超过 20 MiB，编码后（base64 膨胀）不超过 30 MiB，超过任一大小上限返回 413；最多 8 个附件，超过返回 400。

agent 声明 `promptCapabilities.image` 时图片以 `image` 块（base64 与原文件一致）发送；声明 `embeddedContext` 时其他附件以 `resource` 块发送（文本用 `text`，二进制用 base64 `blob`）；都不支持时图片与文件按服务器路径附加到 prompt 文本，并以 `notice` 提示降级。

队列条目在 `state.queue.items[].attachments` 中带同样元数据，编辑文本保留附件，重连与恢复后重放时按当时能力重新构造。附件不写入混合会话记录，上传文件仍按临时目录策略清理。

权限模式由 webmux 自己保存，不是 agent 的配置项：

- `ask`：把每个 `session/request_permission` 作为 `request.open` 交给用户。
- `auto`（默认）：由服务端直接选择第一个 `allow_once` 选项，没有则选第一个 `allow_always`，不产生 `request.open`、提醒或 `notice`。没有允许选项的请求仍交给用户；表单（`elicitation/create`）从不自动答复。

从 `ask` 改为 `auto` 时，已挂起的权限请求立即按同样规则批准并以 `request.close` 关闭。模式写入混合会话记录的 `permissionMode`（仅 `ask` 时写入，没有该字段的记录按 `auto` 处理），切换、重连与恢复后保持。

## 冲突与错误

409 响应体是所有会话命令共用的 `SessionConflict`（`{error, reason, eligibility?, candidates?, connector?, submission?}`，取代已删除的 `HybridConflict`），通用原因见 [会话](http-sessions.md#冲突响应)。与聊天视图相关的 `reason`：

- 当前状态不允许：`switching`（切换进行中）、`wrong_view`（取代旧的 `wrong_side`：选项不属于当前视图）、`no_chat_view`（会话没有聊天视图）。
- 不具备混合资格：`not_eligible`，`eligibility` 为 `unknown_agent`、`not_root`、`no_session_id`、`not_listed`、`occupied` 或 `checking`。
- 档案的第三方 ACP 连接器未安装：`connector_missing`，`connector` 为 `{executable, title, installCommand, docsUrl}`；以聊天视图创建、切到聊天、恢复到聊天、选项探测都可能返回，此时没有启动任何 ACP 进程。
- 无法确定终端正在使用的 Codex 或 Claude Code 会话：`session_ambiguous`，`candidates` 为本次 TUI 启动后更新过的会话，带 `agentSessionId` 重新切换。
- 会话被 webmux 之外的 Codex 客户端或经有效注册记录识别的 Claude Code 进程占用：`occupied_external`。
- 对象已变化：`answered`（请求已被其他设备答复）、`sent`（队列条目已发送或已删除）。
- 其余：`steering_unavailable`、`session_missing`、`config_missing`、`limit`。

Codex 与 Claude Code 的插话带 `_meta.steering.idleBehavior = "promptRequired"`：回合在注入前结束时，消息排到队首作为下一回合发送；agent 回答 `failed` 时请求以 502 失败。

agent 调用失败返回 502；`hybrid-sessions.json` 不可用返回 503 并附文件路径，普通会话不受影响。

终端 WebSocket `/ws/s/{id}` 在会话不在终端视图或切换中时以 `4409`（`session moved: <视图>`，视图为 `chat` 或 `terminal`）关闭，见 [WebSocket 接口](websocket.md)。
