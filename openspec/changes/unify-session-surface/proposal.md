## Why

后端已经把每个会话统一为同一个会话外壳：混合会话只是挂了一个驱动，状态与提醒也已经由聊天端覆盖到会话信息上。
但对外的 HTTP/WebSocket 契约仍然按“普通会话 / 混合会话”和“终端端 / 聊天端”分成两套：

- 同一个意图有两套接口：创建是 `POST /api/sessions` 与 `POST /api/quick-starts`，列表是 `/api/sessions` 与 `/api/hybrid-sessions`，
  中断是终端按键与 `chat/cancel`。服务端没有“向会话输入”这个操作：`webmux send` 先读取会话、判断所在端后自己拨终端 WebSocket，
  聊天端只能走 `chat/prompts`，调用方必须先查清会话种类和所在端。
- 同一个会话有三种形状：`SessionInfo.hybrid`、`HybridRecord`、`ChatState` 都重复了所在端、切换阶段、agent 与 agent 会话 ID。
- 线上取值使用实现层的名字（`tui`/`acp`、`hybrid`、`HybridConflict`、`QuickStart*`），CLI 与界面却说“终端/聊天”。

每新增一个 agent 能力，都要在两套接口和三种形状里各改一遍。shell 命令的收敛建立在本契约之上，见同一 PR 中的 `unify-session-cli`。

## What Changes

会话是唯一的名词；终端与聊天是会话的**视图**（能力），不是会话种类。操作作用于会话，由服务端按当前视图分派。

- **BREAKING** `SessionInfo` 去掉 `hybrid` 对象，改为所有会话共有的字段：
  `state`（`active`/`closed`/`interrupted`）、`views`（`["terminal"]` 或 `["terminal","chat"]`）、`view`（`terminal`/`chat`）、
  `transition`（切换进行中时为 `{to, phase}`，否则为 `null`）、`agent`（有聊天视图时的 agent 身份与当前工作），
  以及只在 `closed`/`interrupted` 时出现的 `archive`（原 `HybridRecord` 独有的字段）。
- **BREAKING** 线上取值 `tui`/`acp` 统一改为 `terminal`/`chat`；`side`/`startSide` 改为 `view`；`switching` 不再是所在端，由 `transition` 表达。
- **BREAKING** `GET /api/sessions` 增加 `state` 筛选（缺省 `active`，行为不变），取代 `GET /api/hybrid-sessions[/{id}]`；
  `GET /api/sessions/{id}` 也返回已归档的会话。删除记录改为 `DELETE /api/sessions/{id}?forget=1`，取代 `DELETE /api/hybrid-sessions/{id}`。
- **BREAKING** `POST /api/sessions` 吸收快速开始：新增 `newWorktree`、`attachments`、`agentConfig`、`permissionMode`、`clientRequestId`，
  返回 `{session, worktree, configWarnings}`，多步失败按步骤报告。删除 `POST /api/quick-starts`；
  `POST /api/quick-starts/options` 改为 `POST /api/launch-configs/{id}/options`，`POST /api/quick-starts/uploads` 改为 `POST /api/uploads`。
- **BREAKING** 新增 `POST /api/sessions/{id}/input`（按当前视图写入终端或作为聊天提示）与 `POST /api/sessions/{id}/interrupt`
  （终端视图发送档案中断键或 Ctrl-C，聊天视图取消回合），取代 `POST .../chat/prompts` 与 `POST .../chat/cancel`。
- **BREAKING** `POST /api/sessions/{id}/switch` 改为 `POST /api/sessions/{id}/view`，`resume` 的 `side` 改为 `view`。
- **BREAKING** `HybridConflict` 改为所有会话命令共用的 `SessionConflict`，原因码 `wrong_side` 改为 `wrong_view`，
  新增 `no_chat_view`、`not_active`、`not_archived`、`input_failed`。
- **BREAKING** `ChatState` 去掉与会话信息重复的 `side`、`phase`、`agent`、`agentSessionId`；`ChatMarker.side` 改为 `view`。
- **BREAKING** 启动配置的 `hybrid`、`hybridSupport` 改为 `chat`、`chatSupport`，`POST /api/launch-configs/hybrid-support` 改为
  `POST /api/launch-configs/chat-support`；`launch-configs.json` 中的持久化键保持 `hybrid` 不变。
- **BREAKING** 一次性切换，不保留旧接口、旧字段或别名。
- Web 端改用新接口与新字段，**可见行为不变**。

## Capabilities

### New Capabilities

- `session-surface`：统一的会话契约与 HTTP 接口：会话信息的形状与命名、按状态列出、一次完成的创建、按视图分派的输入与中断、
  视图切换、恢复、停止/移除/删除记录，以及统一的冲突响应。

### Modified Capabilities

- `hybrid-sessions`：创建、所在端与切换的接口与字段改为视图模型。
- `hybrid-session-recovery`：恢复请求的 `view`、记录列表与删除记录改用会话接口。
- `quick-start-chat`：快速开始改为调用统一创建接口。
- `chat-prompt-queue`：发送提示改为统一输入接口。
- `chat-view`：会话卡片与聊天页头部引用的字段改名。
- `launch-configs`：`hybrid`/`hybridSupport` 改名，新建会话提交的字段改为 `view`。
- `hybrid-agent-profiles`：`hybridSupport` 改名为 `chatSupport`。
- `new-session-dialog`：创建请求契约增加 `newWorktree`。
- `agent-session-identity`、`program-status`、`acp-connector-availability`、`workbench-navigation`：引用的字段与取值改名。
- `session-metadata`：会话详情中的混合会话记录改为会话信息；父会话的创建入口只剩 `POST /api/sessions`；`chat new --label` 改为 `run --label`。
- `session-artifacts`：删除记录的接口改为 `DELETE /api/sessions/{id}?forget=1`。

## Impact

- 契约与转换：`internal/contract`（`sessions.go`、`hybrid.go`、`chat.go`、`quickstart.go`、`launch.go`、`workspace.go`）、
  `internal/wire`、`web/src/contracts/schemas.ts` 与 `chat-schemas.ts`，以及 `internal/api/testdata/wire` 快照。
- 控制器：`internal/api`（`router.go`、`chat.go`、`quickstart.go`、`launch.go`、`errors.go`）、`internal/hub` 的移动通知与聊天帧。
- 服务：`internal/workspace`（快速开始编排并入创建）、`internal/hybrid`（输入、中断、记录到会话信息的转换）、`internal/session`（服务端提交输入）、
  `internal/launch`（线上字段改名，存储键不变）。
- Web 端：`api.ts`、`chat-api.ts`、`quick-start.ts`、`recoverable.ts`、`archived.ts`、`hooks/hybrid.svelte.ts` 及读取 `hybrid` 字段的组件。
- CLI：所有调用旧接口的命令随 `unify-session-cli` 在同一 PR 中迁移；两个变更必须一起发布。
- 文档：`docs/protocol` 中的会话与混合会话协议、`docs/hybrid-sessions.md`，以及 `docs/user-stories/hybrid-sessions.md` 与
  `docs/user-stories/shell-control.md`（US-S03）中引用旧接口的条目。
- 兼容：旧客户端调用已删除的接口会得到 404；Windows 控制台客户端只用会话列表，不受接口删除影响，但会读取到新的字段形状。
