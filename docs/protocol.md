# API 与协议

所有 API 以 `/api` 为前缀，WebSocket 以 `/ws` 为前缀，其余路径返回嵌入的前端（SPA 回退）。前端的完整数据契约定义在 [`web/src/schemas.ts`](../web/src/schemas.ts)，与后端 JSON 字段保持一致。

## 认证与 Origin

- 启用 token 时，HTTP 请求使用 `Authorization: Bearer <token>` 或 Cookie `webmux_token`；WebSocket 还可使用 query `?token=`（HTTP API 不接受 query token）。
- token 以定长摘要做常量时间比较。
- 带 `Origin` 的请求必须与 `Host` 同源，或在 `allowed_origins` 中。Origin 必须是完整的 HTTP(S) origin，不能含路径、query 或 fragment。
- 静态页面公开；所有 API 与 WebSocket 都受认证和 Origin 检查保护。认证失败返回 401，Origin 拒绝返回 403。
- 错误响应为 JSON `{"error": "..."}`。

## HTTP API

### 会话

| 方法   | 路径                          | 说明                                             |
| ------ | ----------------------------- | ------------------------------------------------ |
| GET    | `/api/sessions`               | 会话列表 `SessionInfo[]`，按 `lastActiveAt` 倒序 |
| POST   | `/api/sessions`               | 创建会话，201 返回 `SessionInfo`                 |
| GET    | `/api/sessions/{id}`          | 单个会话；不存在 404                             |
| DELETE | `/api/sessions/{id}`          | 关闭会话（SIGHUP → SIGKILL），204                |
| DELETE | `/api/sessions/{id}?remove=1` | 移除已退出会话，204；仍在运行 409                |
| GET    | `/api/sessions/{id}/buffer`   | 下载完整原始缓冲（`application/octet-stream`）   |
| POST   | `/api/sessions/{id}/upload`   | 上传文件，见下文                                 |

创建请求有两种互斥形式：

```jsonc
// 预设
{ "cwd": "/home/u/proj", "preset": "claude", "args": ["--resume"], "name": "可选" }
// 自定义命令
{ "cwd": "/home/u/proj", "preset": "custom", "cmd": ["bash", "-lc", "make dev"], "name": "可选" }
```

自定义命令必须包含非空可执行程序，且不能同时传非空 `args`；预设形式不能传 `cmd`。可附加 `workspace: {repositoryId, worktreeId}`：省略 `cwd` 时使用工作树根目录，提供时必须位于该工作树内。

**上传**：`multipart/form-data`，恰好一个文件字段 `file`。成功返回 201 `{"path": "<服务器绝对路径>"}`。单文件上限 100 MiB（超限 413），读期限 5 分钟（超时 408）；无效请求 400，会话不存在 404，会话已退出 409。

```ts
interface SessionInfo {
  id: string;
  title: string;
  cwd: string;
  preset: string; // 预设名或 "custom"
  cmd: string[];
  status: "running" | "waiting" | "idle" | "exited";
  exitCode: number | null;
  preview: string; // 屏幕底部最多 100 条非空行，以 \n 分隔
  createdAt: string; // RFC 3339
  lastActiveAt: string;
  cols: number;
  rows: number;
  clients: number; // 当前附着的连接数
  workspace: {
    repositoryId: string;
    worktreeId: string;
    worktreePath: string;
  } | null;
  // 程序主动报告的“需要介入”，存在时 status 为 "waiting"；不含任何终端正文。
  attention: {
    kind: "question" | "permission" | "done" | "error" | "notify" | "bell";
    source: "agent" | "terminal"; // 结构化事件 / 普通通知或响铃
    since: string; // RFC 3339，种类变化时更新
  } | null;
}
```

### 其他

| 方法 | 路径                   | 说明                                                                                              |
| ---- | ---------------------- | ------------------------------------------------------------------------------------------------- |
| GET  | `/api/health`          | `{ok: true, sessions: n}`                                                                         |
| GET  | `/api/presets`         | `Preset[]`：`{name, label, cmd, allowArgs}`                                                       |
| GET  | `/api/config`          | `{defaultCwd, workspace: {gitAvailable, registryAvailable, gitError, registryError}}`，不含 token |
| GET  | `/api/fs/dirs?prefix=` | 匹配前缀的全部子目录（含符号链接目录），普通目录优先，按名称排序                                  |
| POST | `/api/fs/mkdir`        | `{path}`，204                                                                                     |

### 仓库与 worktree

| 方法       | 路径                                                            | 说明                                                                                       |
| ---------- | --------------------------------------------------------------- | ------------------------------------------------------------------------------------------ |
| GET / POST | `/api/repositories`                                             | 列出仓库快照；POST `{path}` 登记仓库（新增 201，已存在 200）                               |
| DELETE     | `/api/repositories/{id}`                                        | 仅移除登记，204                                                                            |
| GET        | `/api/repositories/{id}/worktrees`                              | 刷新仓库快照（含 `checkedAt` 与 `worktrees`）                                              |
| GET        | `/api/repositories/{id}/branches`                               | `{branches, bases, defaultBase, defaultParent}`，不 fetch                                  |
| POST       | `/api/repositories/{id}/worktrees`                              | `{path, mode: "new", branch, base}` 或 `{path, mode: "existing", branch}`，201             |
| POST       | `/api/repositories/{id}/worktrees/{worktreeId}/removal-preview` | 删除预览：`{confirmationId, expiresAt, worktree, sessions, blockers, forceAllowed}`        |
| POST       | `/api/repositories/{id}/worktrees/{worktreeId}/remove`          | `{confirmationId, mode: "safe"}`；强制删除另需 `confirmPath` 与 `closeSessions: true`，204 |

错误码：400 输入错误，404 不存在，409 冲突或确认过期，503 Git / 清单不可用，504 超时，500 存储或非预期错误。JSON 结构见 `internal/workspace/types.go`。

### 只读代码工作区

位于 `/api/sessions/{id}/code`，响应带 `Cache-Control: no-store`。

| 路由                                               | 说明                                                                         |
| -------------------------------------------------- | ---------------------------------------------------------------------------- |
| `GET /context`                                     | 根目录与能力：`contextId`、`rootPath`、`browseAvailable`、`diffAvailable` 等 |
| `GET /tree?contextId&nodeId&includeIgnored&cursor` | 分页目录列表                                                                 |
| `GET /files?contextId&q&includeIgnored`            | 有界路径搜索                                                                 |
| `GET /file?contextId&nodeId&source`                | 文件快照，`source` 为 worktree / index / head                                |
| `GET /changes?contextId&cursor&q&nodeId&group`     | 分页 Git 修改列表                                                            |
| `GET /diff?contextId&changeId`                     | 冻结的新旧双侧原文                                                           |
| `GET /snapshots/{snapshotId}/file?side=old\|new`   | 读取已捕获的比较侧全文                                                       |
| `POST /check`                                      | 批量变化检测，返回 unchanged / updated / missing / expired / unknown         |

错误码：400 非法路径，404 不存在，409 版本冲突，410 快照失效，413 超出限制，504 取消或超时。

## WebSocket `/ws/sessions`

会话列表，文本帧 JSON。连接后先收到 `snapshot`，之后为增量事件：

```jsonc
{"type": "snapshot", "sessions": [SessionInfo, ...]}
{"type": "created", "session": SessionInfo}
{"type": "updated", "session": SessionInfo}
{"type": "removed", "id": "abc123"}
```

`updated` 每会话最多 500 ms 一次；仅预览变化最多 2 秒一次。客户端无需发送消息。

## WebSocket `/ws/s/{id}`

终端流，**二进制帧**，首字节为类型。

服务端 → 客户端：

| 首字节 | 含义 | 载荷                                  |
| ------ | ---- | ------------------------------------- |
| `0x00` | 输出 | PTY 原始字节                          |
| `0x01` | 快照 | 完整缓冲；客户端应先 `reset()` 再写入 |
| `0x02` | 状态 | JSON `SessionInfo`                    |
| `0x03` | 退出 | JSON `{"exitCode": n}`，随后关闭连接  |

客户端 → 服务端：

| 首字节 | 含义     | 载荷                           |
| ------ | -------- | ------------------------------ |
| `0x00` | 输入     | 写入 PTY 的 UTF-8 字节         |
| `0x01` | resize   | JSON `{"cols": n, "rows": n}`  |
| `0x02` | 重新同步 | 空；服务端回复一帧 `0x01` 快照 |

使用二进制帧而非 JSON，是为了避免 base64 开销以及在多字节字符中间截断时的 UTF-8 问题。

### 关闭码

| 关闭码 | 含义                                       |
| ------ | ------------------------------------------ |
| `4404` | 会话不存在                                 |
| `4408` | 客户端过慢或写超时；客户端应立即重连并回放 |
| `4410` | 会话已退出（先发送快照与退出帧）           |

认证失败在握手阶段直接返回 HTTP 401，不升级连接。
