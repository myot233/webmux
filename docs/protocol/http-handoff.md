# HTTP API：交接预览

预览在服务端选择已提交 Activity，保存为独立草稿；当前层提供 fork 到新 Conversation 的 inline 投递 API，尚未连接浏览器交接入口或同一 Conversation 的 agent 切换。
它不启动 agent、不创建 worktree，也不依赖浏览器缓存或活跃 actor。来源历史接口见[持久对话历史](http-conversation.md)。

## 创建与读取

```http
POST /api/conversations/{id}/handoff-previews
GET /api/handoffs/{handoffId}
```

创建体包含 `clientRequestId`、`configId`、`agentConfig`、`userText`、`fork`、`location` 与 `attachments`。
`targetProviderThreadId` 可选，只能属于来源 Conversation 和同一启动配置；不指定时预留 fresh ProviderThread。
`fork=true` 预留独立目标 Conversation；位置保存 cwd、repository/worktree 或新 worktree 的 base/branch 意图，执行留待投递阶段。
附件引用必须属于来源会话上传或服务端暂存，名称、MIME、大小和类型由实际文件重新核对；不接受任意文件路径。

`clientRequestId` 为 8–64 位字母、数字、横线或下划线；相同来源、相同 key 和相同请求返回原预览，不重复分配目标。
相同 key 配不同请求返回 409。首次创建后删除启动配置不影响相同请求的读取与重试。

响应保存 source point、Run 范围、目标配置快照、原始用户请求和附件引用；包含 selected/omitted IDs、逐项省略原因、rendered text、budget 与 revision。
历史引用标明来源与角色，并说明历史只作上下文，当前用户请求单独保存。

## 选择与预算

- 最近 user、最近 agent、首条 user 优先；其余从近到远尝试，按原顺序返回，重复 ID 只计算一次。
- 单项超预算时整体省略，继续尝试其他项；不切割中文、emoji 或长项正文。预算减少时总选择成本不增加，所选集合可能变化。
- `omittedItems` 的 reason 为 `budget`，手工编辑替代结构化历史后为 `manual_context`。
- `handoff_token_cap` 默认 16000，范围 1024–64000；一个 UTF-8 字节按一个 token 估算，包含 envelope 与 JSON 转义。
- 未知窗口使用 128000；可靠 provider 窗口/compaction threshold 优先。扣除目标已占用上下文、当前用户请求、图片 8192/其他附件 4096，以及 `max(16000, ceil(window/4))` 工作预留。
- 模型或选项改变时保留可比较的 native occupancy，失效窗口和 threshold 不再使用。预算 `source=provider|estimated`，不声称精确 token 数。
- 当前采用 inline 约束，历史还受 prompt 原文字节和附件 wire 上限限制；超预算返回 `over_budget`，不自动缩短用户请求或附件。

## 编辑与过期

```http
PATCH /api/handoffs/{handoffId}
Content-Type: application/json

{"revision":1,"text":"需要保留的上下文"}
```

成功编辑递增 revision；旧 revision 返回 409，不覆盖其他设备的编辑。
编辑保存为 `manual_context`，跨刷新与服务重启保留，后续只允许 inline；用户当前请求保持独立。
超预算编辑也保留原文并标记 over_budget，便于继续修改。

新 Activity、Run 或来源完整性变化使旧草稿 `stale=true`，不会重新选择或覆盖编辑；显式重新生成需要新的 clientRequestId。
目标模型、位置、用户请求或附件变化也应创建新预览，不能复用旧预算结论。
目标选项基线独立于预算比较；即使未知窗口仍估算为 128000，选项漂移也标 stale。请求中明确指定的覆盖值允许生效；禁用启动配置的聊天能力同样使草稿失效并保留编辑。
`nativeAvailable` 与 `historyToolAvailable` 当前均为 false；后续适配器与实际 MCP 工具就绪后才可启用。

## 边界与错误

认证与 Origin 检查沿用统一 API；响应 `Cache-Control: no-store`，操作限时 10 秒，正文不写日志。
400 表示无效输入，404 表示不存在，409 表示 key/revision/目标冲突，503 表示依赖存储或服务不可用。
删除来源记录会清理预览、编辑和 receipt；释放或重建 worktree 不删除草稿。私有存储边界见[持久化清单](../persistence.md)。

## 投递用户消息

```http
POST /api/conversations/{id}/messages
GET /api/handoffs/{handoffId}/delivery
```

请求使用新的 `clientRequestId`，包含 `handoffId/revision/text/configId/agentConfig/location/attachments/retry`。
文字、选项、位置和附件必须与预览快照一致，来源位置和预算仍有效；修改这些内容需要新预览。
POST 返回 202 和持久 attempt：`executionId/deliveryId/handoffId`、目标 Conversation/ProviderThread、预留的 `sessionId`、status、phase、step 与 errorClass。
服务端异步准备资源，GET 查询最新结果。sessionId 在创建前已经保留，只有创建完成后才可打开；失败保留已创建资源。

- 先事务提交 receipt 和 pending delivery，再创建 worktree/session。重启后沿用精确 branch/path 与 runtime ID；不通过现有 CreateSessionRequest.prompt 展开交接。
- 附件先以确定名称复制到目标 uploads，复制意图与 digest 先保存；来源被删除或暂存区清理不再影响已完成的目标副本。回放恢复目标附件元数据，provider 能力不足时按目标路径降级。
- 最终 inline 文本含来源 coverage、逐项角色/类型/Run/Activity/status，以及独立 `User message:`；界面时间线、会话标题与本地 Activity 只使用用户新请求。
- `pending` 表示准备或等待明确结果，`inline` 表示确认消费，`failed` 表示明确拒绝或准备失败，`unknown` 表示可能已发送且结果不明。
- 正常 prompt 响应或前台 agent 回复可确认消费；transport 断连不能简单按 turn 结束算成功。已确认消费不会被后续取消、错误或 auth 结果覆盖。
- 未进入 actor 调用的准备可在相同 key 重试后继续；actor 的持久 dispatch gate 保证最多调用一次。可能已经调用的 unknown 禁止自动重发。
- 原生 load 只有在同一 ProviderThread/native ID 回放出精确 payload hash（包含附件能力降级后的实际文字）时，才把 unknown 对账为 inline；marker 本身不构成证明，未匹配的用户文字不被剥离。
- 明确 failed 可使用新 clientRequestId 和 `retry=true` 显式重试，沿用资源；旧 key 始终返回自己的首次 attempt 结果。unknown 需要用户检查，不能用 retry 绕过。
- step 沿用 `precheck/worktree/session/config/prompt`。当前仅执行 `fork=true`；同一 Conversation 切换在后续层开放。
