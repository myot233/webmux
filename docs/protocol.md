# API 与协议

所有 API 以 `/api` 为前缀，WebSocket 以 `/ws` 为前缀，其余路径返回嵌入的前端（SPA 回退）。数据契约：

- 前端的完整数据契约定义在 [`web/src/schemas.ts`](../web/src/schemas.ts)。
- 后端对应的线上类型都在 [`internal/contract`](../internal/contract)（REST 按资源分文件，终端帧与 `/ws/sessions` 事件在 `terminal.go`），由 `internal/wire` 与 Service 类型互相转换。
- 两边的字段保持一致，`internal/api/testdata/wire` 的快照测试固定了每个端点和帧的 JSON 形状。

## 认证与 Origin

- 启用 token 时，HTTP 请求使用 `Authorization: Bearer <token>`；`/api/` 与 `/ws/` 不接受 Cookie 认证。
- WebSocket 握手不能设置请求头，因此：
  - 前端改为同时提供子协议 `webmux` 与 `webmux.token.<base64url(UTF-8 token)>`（无填充），服务端只选择 `webmux`、不回显 token。
  - 不接受 query `?token=`，token 不进入请求 URL。
  - 不使用 Cookie：Cookie 不按端口隔离，且浏览器会自动携带，服务端不接受 `webmux_token` Cookie。

- token 以定长摘要做常量时间比较。
- 带 `Origin` 的请求必须与 `Host` 同源，或在 `allowed_origins` 中。Origin 必须是完整的 HTTP(S) origin，不能含路径、query 或 fragment；`allowed_origins` 条目另须全小写，不带末尾斜杠与默认端口（`:80`、`:443`），否则启动失败。
- 静态页面公开；所有 API 与 WebSocket 都受认证和 Origin 检查保护。认证失败返回 401，Origin 拒绝返回 403。
- 端口转发网关 `/forward/{port}/` 是唯一使用 Cookie 的路径：`webmux_forward` 是独立于主 Token 的转发凭据，只能访问网关，见 [端口转发](protocol/http-forward.md)。
- 错误响应为 JSON `{"error": "..."}`。

## 文档地图

| 主题                                                             | 内容                                                                   |
| ---------------------------------------------------------------- | ---------------------------------------------------------------------- |
| [HTTP API：会话、启动配置与其他](protocol/http-sessions.md)      | 会话信息、按状态列出、输入与中断、结束与删除记录、冲突、上传与启动配置 |
| [HTTP API：创建会话](protocol/http-session-create.md)            | 位置、命令、视图、首条提示、新 worktree、分步失败、创建前的上传与选项  |
| [HTTP 与 WebSocket：聊天视图与混合会话](protocol/http-hybrid.md) | 视图与 agent 字段、切换、恢复、已归档会话、聊天通道与聊天命令          |
| [HTTP API：仓库与代码工作区](protocol/http-workspace.md)         | 仓库与 worktree、只读代码、Diff、图片与下载                            |
| [HTTP API：端口转发](protocol/http-forward.md)                   | 转发管理接口、`/forward/{port}/` 网关、转发凭据与兼容改写              |
| [HTTP API：公网隧道设置](protocol/http-tunnel.md)                | `/api/tunnel` 系列、持久化与错误语义                                   |
| [HTTP API：Tailscale 状态与设置](protocol/http-tailscale.md)     | `/api/tailscale` 检测、运行状态与网页设置                              |
| [HTTP API：语音识别设置](protocol/http-voice.md)                 | `/api/voice/settings` 识别命令与环境变量的网页设置                     |
| [HTTP API：注意力推送](protocol/http-push.md)                    | Web Push 订阅、来源资格与推送负载                                      |
| [HTTP API：交接预览](protocol/http-handoff.md)                   | 冻结来源、动态预算、手工编辑与 revision                                |
| [HTTP API：持久对话历史](protocol/http-conversation.md)          | Activity 分页、UTF-8 偏移、完整性与导出边界                            |
| [服务端偏好](protocol/settings.md)                               | `/api/settings`、`/ws/settings`、修订号与设备边界                      |
| [WebSocket 接口](protocol/websocket.md)                          | 会话列表事件、终端帧、回放、提交、控制端与关闭码                       |
| [本机 API](protocol/local-api.md)                                | Unix 套接字认证与 `/local/v1/*` 控制面                                 |
| [控制面 JSON 契约](protocol/control-json.md)                     | `version`、`status`、`stop`、`config` 与 `update check` 的 JSON 输出   |

修改线上字段时，同时更新 [`web/src/schemas.ts`](../web/src/schemas.ts) 与 [`internal/contract`](../internal/contract)；每个子页只描述自己的主题，索引保持短小。
