# HTTP 与 WebSocket：混合会话

混合会话让同一段 agent 对话在原生 TUI（终端端 `tui`）与 ACP 聊天视图（聊天端 `acp`）之间切换，webmux 会话 ID 始终不变。线上类型见 [`internal/contract/hybrid.go`](../../internal/contract/hybrid.go) 与 [`chat.go`](../../internal/contract/chat.go)，前端镜像见 [`web/src/chatSchemas.ts`](../../web/src/chatSchemas.ts)。认证、Origin 与错误格式同 [API 与协议](../protocol.md)。

## 会话信息中的 `hybrid`

混合会话的 `SessionInfo` 带 `hybrid` 对象，普通会话省略该字段：

| 字段             | 说明                                                                                                                               |
| ---------------- | ---------------------------------------------------------------------------------------------------------------------------------- |
| `side`           | `tui`、`acp` 或 `switching`                                                                                                        |
| `phase`          | 切换中的步骤：`interrupting`、`awaiting_choice`、`cancelling`、`stopping`、`starting`、`loading`、`rolling_back`；不在切换中时省略 |
| `dormant`        | 聊天端空闲休眠，agent 进程已结束                                                                                                   |
| `state`          | `active`、`closed` 或 `interrupted`；首页运行中分组只显示 `active`                                                                 |
| `agent`          | 档案名：`opencode` 或 `omp`                                                                                                        |
| `agentSessionId` | agent 自己的会话 ID；尚未得知时为 `null`                                                                                           |

会话在聊天端时，`status`、`attention`（来源 `agent`）与 `preview`（最新 agent 消息前三行）来自聊天端的回合状态；`cmd` 为启动配置的命令。

## 创建、切换与恢复

| 方法   | 路径                        | 说明                                                                                                                                                                                                                          |
| ------ | --------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| POST   | `/api/sessions`             | 以勾选了混合模式的启动配置 `configId` 创建；可选 `startSide: "tui" \| "acp"`，缺省 `tui`。对普通配置或自定义命令提供 `startSide` 返回 400；记录存储不可用 503；记录达 200 条 409 `limit`                                      |
| POST   | `/api/sessions/{id}/switch` | `{clientRequestId, to}` 切到另一端，完成后 200 `{side, draft?, dropped, unsupported}`；中断超时进入 `awaiting_choice` 后，另一请求以 `{clientRequestId, choice: "wait" \| "force" \| "cancel"}` 作答                          |
| POST   | `/api/sessions/{id}/resume` | `{clientRequestId, side?, agentSessionId?, confirmWithoutEnv?}` 把已关闭或已中断的会话恢复到指定端（缺省为记录中的端），沿用原 ID，200 返回 `SessionInfo`；会话仍活动时 409，切换中或同一记录正在恢复、移除时 409 `switching` |
| DELETE | `/api/sessions/{id}`        | 关闭混合会话：先取消聊天端回合或按中断流程处理终端端回合，再结束进程，记录改为 `closed`；切换中 409 `switching`                                                                                                               |
| GET    | `/api/hybrid-sessions`      | 全部记录，按更新时间倒序；`?state=recoverable` 只返回 `closed` 与 `interrupted`，其他取值 400                                                                                                                                 |
| GET    | `/api/hybrid-sessions/{id}` | 单条记录；不存在 404                                                                                                                                                                                                          |
| DELETE | `/api/hybrid-sessions/{id}` | 204；移除记录及仍留在列表中的已退出外壳，不删除 agent 的会话库；会话仍活动时 409，切换中 409 `switching`                                                                                                                      |

切换结果中：`draft` 是切到终端时退回的排队消息（按顺序以空行连接），只返回给发起切换的请求，其他设备收到提示事件；`dropped` 为恢复命令未保留的启动参数；`unsupported` 为聊天端不支持的参数。切换失败时会话回到原端并返回 502 与原因；回滚也失败时会话标记为 `interrupted`。

记录（`HybridRecord`）包含 ID、名称、`configId`、`configMissing`、agent、cwd、命令快照、所在端、当前与最近 8 个历史 agent 会话 ID、状态、中断前的活动 `activity`（`idle`、`turn` 或 `switching`，后两者表示上一轮可能未完成）、中断时丢失的排队条数与时间戳，不含环境变量值、消息、队列正文或 token。

恢复时命令取自记录中的快照，环境变量按 `configId` 重新读取。启动配置已删除且未确认时返回 409 `config_missing`；以 `confirmWithoutEnv: true` 重试则不带环境变量恢复。

随后服务端启动一个只调用 `initialize` 与 `session/list` 的临时 ACP 进程，在记录的 cwd 中核对 agent 会话：已不存在时返回 409 `session_missing`，`candidates` 为同一目录下的会话（`{agentSessionId, title, updatedAt}`，按更新时间倒序，最多 20 个）；以 `agentSessionId` 重试即改用所选会话，原 ID 进入历史。该会话正被其他会话打开时返回 409。记录中没有 agent 会话 ID 时不核对，所选端开始新对话。

核对通过后，恢复到终端端在 PTY 中执行档案生成的恢复命令；恢复到聊天端启动 ACP 进程并 `session/load`，再按档案补回记录中的配置项取值。成功后记录改为 `active`；中断时丢失过排队消息的，聊天状态的 `queueLost` 为丢失条数。恢复失败时记录保持原样。

## 启动配置

`GET/POST/PUT /api/launch-configs[/{id}]` 的配置与摘要带 `hybrid`（布尔）与只读 `hybridSupport: {supported, agent?, reason?}`；请求体可带 `hybrid`，带 `hybridSupport` 视为未知字段返回 400。`hybrid: true` 而命令不受支持时返回 400 并给出原因。

`POST /api/launch-configs/hybrid-support {cmd}` 只读返回同样的 `hybridSupport`，供设置页去抖查询。`reason` 取值：`unknown_agent`、`unsupported_form`、`session_store`、`directory_arg`、`non_interactive`。`launch-configs.json` 保持 v2，`hybrid` 为 `false` 时不写入。

## 聊天通道 `GET /ws/s/{id}/chat`

文本 JSON 帧，任一端都可连接；查询参数 `generation` 与 `seq`（客户端最后已应用的事件序号）用于续接。帧 `type`：

- `snapshot`：首帧。`state` 为完整聊天状态，`generation`、`from`（日志中最早的序号）、`next`；`reset: true` 表示所给位置已不在日志中，客户端须丢弃时间线；`truncated: true` 表示更早的事件已按字节上限丢弃。
- `events`：`{generation, from, items}`，snapshot 之后立即发送一帧补发（可为空），之后为实时增量。
- `state`：完整替换聊天状态（所在端、阶段、资格与原因、回合、待答复请求、配置项、命令、计划、用量、队列、是否可插话、`authRequired`、`promptCapabilities`、`permissionMode`、`idChange`、`queueLost` 等）。`promptCapabilities` 为 `{image, embeddedContext}`，来自 ACP `initialize`，连接断开或休眠时复位为 `false`。`permissionMode` 为 `ask` 或 `auto`，见下文。

关闭码：`4404` 会话不存在或不是混合会话；`4408` 客户端过慢或写超时，可重连；`1013` 日志被重置，重连后从新代际开始；`4410` 会话已结束。

事件为规范化模型，不是 ACP 原文：`message.append`（按 `messageId` 追加，`role` 为 user/agent/thought）、`tool.upsert`（按 `toolCallId` 的字段补丁）、`plan.set`、`commands.set`、`config.set`、`usage.set`、`info.set`、`compaction.upsert`、`notice`、`turn.start`、`turn.end`（`stopReason` 含 `cancelled`、`error`、`auth_required`）、`request.open`、`request.close`、`marker`（`loaded`、`switched`、`truncated`、`reconnected`、`resumed`）。

单个文本字段超过 256 KiB 时截断并标 `truncated`；不支持的内容块以 `type: "unsupported"` 占位。日志按编码字节估算，上限为 `scrollback_bytes`；每次 `session/load` 开始新代际。

## 聊天命令

所有改变状态的请求都带 `clientRequestId`（`[A-Za-z0-9_-]{8,64}`）。服务端按会话保留最近 256 条、30 分钟，重复请求返回首次结果，不再执行。

| 方法   | 路径                                           | 说明                                                                                                                                                                                         |
| ------ | ---------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| POST   | `/api/sessions/{id}/chat/prompts`              | `{clientRequestId, text, mode, attachments?}`，`mode` 为 `auto`（空闲且未暂停时立即发送，否则排队）或 `steer`（插话）；`attachments` 为上传接口返回的 `{path}` 数组；200 `{queued, itemId?}` |
| PATCH  | `/api/sessions/{id}/chat/queue/{qid}`          | `{clientRequestId, text?, move?: "up"}` 编辑或上移，204                                                                                                                                      |
| DELETE | `/api/sessions/{id}/chat/queue/{qid}`          | `{clientRequestId}` 删除，204                                                                                                                                                                |
| POST   | `/api/sessions/{id}/chat/queue/{qid}/send-now` | 取消当前回合后立即发送该条，204                                                                                                                                                              |
| POST   | `/api/sessions/{id}/chat/queue/resume`         | 暂停后继续，从队首发送，204                                                                                                                                                                  |
| POST   | `/api/sessions/{id}/chat/queue/clear`          | 清空队列，204                                                                                                                                                                                |
| POST   | `/api/sessions/{id}/chat/cancel`               | 停止当前回合，204                                                                                                                                                                            |
| POST   | `/api/sessions/{id}/chat/requests/{requestId}` | 权限：`{outcome: "selected", optionId}` 或 `cancelled`；表单：`accept` 带 `content`、`decline`、`cancel`；204                                                                                |
| PUT    | `/api/sessions/{id}/chat/config/{configId}`    | 选择类 `{value}`、布尔类 `{enabled}`，经 `session/set_config_option` 生效并记录，204                                                                                                         |
| PUT    | `/api/sessions/{id}/chat/permission-mode`      | `{clientRequestId, mode}`，`mode` 为 `ask` 或 `auto`（其他值 400）；两端均可设置，切换中返回 409 `switching`，204                                                                            |

`text` 不超过 64 KiB；有附件时可以只发附件。会话处于休眠时，命令先唤醒 agent 再执行。回合以 `cancelled`、错误、`refusal` 或 `auth_required` 结束时，非空队列暂停。插话只在档案声明、agent 在 `initialize` 中声明支持且有回合运行时可用。

附件只能是 `POST /api/sessions/{id}/upload` 落在私有临时上传目录中的文件，名称取上传时的文件名，MIME 与大小按文件内容推断并忽略客户端声明；允许的图片类型为 PNG、JPEG、GIF、WebP，SVG 不作为图片块发送。越出上传目录的路径、符号链接与非普通文件返回 400 且不读取内容；系统临时目录本身可以是符号链接（如 macOS 的 `/var`）。

单个附件不超过 10 MiB，单条消息合计不超过 20 MiB，编码后（base64 膨胀）不超过 30 MiB，超过任一大小上限返回 413；最多 8 个附件，超过返回 400。

agent 声明 `promptCapabilities.image` 时图片以 `image` 块（base64 与原文件一致）发送；声明 `embeddedContext` 时其他附件以 `resource` 块发送（文本用 `text`，二进制用 base64 `blob`）；都不支持时图片与文件按服务器路径附加到 prompt 文本，并以 `notice` 提示降级。

队列条目在 `state.queue.items[].attachments` 中带同样元数据，编辑文本保留附件，重连与恢复后重放时按当时能力重新构造。附件不写入混合会话记录，上传文件仍按临时目录策略清理。

权限模式由 webmux 自己保存，不是 agent 的配置项：`ask` 把每个 `session/request_permission` 作为 `request.open` 交给用户；`auto`（默认）由服务端直接选择第一个 `allow_once` 选项，没有则选第一个 `allow_always`，不产生 `request.open`、提醒或 `notice`。没有允许选项的请求仍交给用户；表单（`elicitation/create`）从不自动答复。

从 `ask` 改为 `auto` 时，已挂起的权限请求立即按同样规则批准并以 `request.close` 关闭。模式写入混合会话记录的 `permissionMode`（仅 `ask` 时写入，没有该字段的记录按 `auto` 处理），切换、休眠唤醒、重连与恢复后保持。

## 冲突与错误

409 响应体为 `{error, reason, eligibility?, candidates?}`，`reason` 取值：`switching`（切换进行中）、`wrong_side`（不在聊天端）、`not_eligible`（`eligibility` 为 `unknown_agent`、`not_root`、`no_session_id`、`not_listed`、`occupied` 或 `checking`）、`answered`（请求已被其他设备答复）、`sent`（队列条目已发送或已删除）、`steering_unavailable`、`session_missing`、`config_missing`、`limit`。agent 调用失败返回 502；`hybrid-sessions.json` 不可用返回 503 并附文件路径，普通会话不受影响。

终端 WebSocket `/ws/s/{id}` 在会话不在终端端、休眠或切换中时以 `4409`（`session moved: <所在端>`）关闭，见 [WebSocket 接口](websocket.md)。
