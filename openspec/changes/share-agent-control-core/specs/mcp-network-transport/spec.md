# Spec Delta

## Purpose

为外部 MCP 客户端提供默认关闭、始终认证的实例 URL 控制入口，复用已有会话操作与观察语义。明确 owner 凭据、远程身份、HTTP 生命周期、资源上限和取消恢复边界，避免将本机身份或冻结历史权限错误扩展到网络。

## ADDED Requirements

### Requirement: Opt-in TCP endpoint

系统 SHALL 以 TOML `[mcp] enabled` 控制既有 TCP 监听上的精确 `/mcp`，默认 false，无新端口。关闭时该路径 SHALL 返回 404，而不是静态页面。Unix、本机 bridge、P2P RPC/stream SHALL NOT 提供此入口；`/mcp/` SHALL 返回 404，不重定向。

#### Scenario: Default and non-TCP isolation

- **WHEN** 默认实例收到 `/mcp` 请求，或已启用实例通过 Unix/P2P 请求该路径
- **THEN** 返回 404，既有 stdio MCP 与 `/api/*`、`/ws/*` 不变

### Requirement: Nonempty startup credential

启用网络 MCP 时，系统 SHALL 在任何网络曝光自动补 Token 之前要求已解析配置的共享 owner Token 非空；为空 SHALL 使启动失败，且不为本功能生成、保存或向会话进程索取凭据。正常首次配置生成逻辑不变；已得到持久 Token 的首次配置可启用。该项 SHALL 有 default/toml 来源和
configured/effective 布尔诊断，不泄露 Token。

#### Scenario: Explicit empty token

- **WHEN** 已有配置启用 mcp 且 Token 为空，即使开启隧道或非 loopback 监听
- **THEN** 启动失败且不开始监听，错误要求配置非空 owner Token，不借既有曝光自动生成绕过该前提

### Requirement: Bearer owner authority

每个已启用 `/mcp` 请求 SHALL 使用 `Authorization: Bearer` 认证，包括初始化、GET、DELETE 和通知。缺失或错误凭据 SHALL 返回 401 与 `WWW-Authenticate: Bearer realm="webmux"`；查询参数、Cookie 和 MCP 会话 ID SHALL NOT 认证。成功身份 SHALL 是实例 owner，权限等同
REST，包括任意 argv 启动；工具清单 SHALL NOT 被描述为凭据 ACL 或沙箱。

#### Scenario: Session id is not authorization

- **WHEN** 客户端只携带有效 MCP-Session-Id、`?token=` 或 Cookie 而无 Bearer
- **THEN** 返回 401，不执行 handler，不透露该 MCP 会话是否存在

### Requirement: Host and Origin boundary

系统 SHALL 在 `/mcp` 沿用既有 TCP Host 和 Origin 判定：loopback 只允许既有合法本机或已配置/登记 Host；非 loopback 保持合法 authority 规则。Origin 缺省可供非浏览器客户端；存在时必须是同源、精确 allowed_origins 或已登记可信入口。非法 Host/Origin SHALL 返回 403；
不以任意 Forwarded 头新增信任，不任意禁用既有可信代理来源。

#### Scenario: Trusted reverse proxy

- **WHEN** 代理请求的 Host/Origin 满足现有 allowed_origins 或受控公开入口规则且 Bearer 有效
- **THEN** MCP 请求可继续，来自未获准浏览器 Origin 的相同请求返回 403

### Requirement: Explicit remote identity

网络调用 SHALL 标记为 remote owner；self 与隐式 parent SHALL 未知，不从服务进程 PID、环境、clientInfo、请求 header 或 MCP 会话 ID 推导。远程启动 SHALL NOT 因代理进程恰位于会话树内而自动附加 parentId。本地 stdio 的 socket peer PID 身份 SHALL 保留；工具参数中显式合法业务目标 ID 不是调用者身份。

#### Scenario: Forged local identity

- **WHEN** 远程客户端在 clientInfo 或自定义 header 声称属于会话 S，并启动会话
- **THEN** 远程指南 self 为 null，新会话没有由该声明推导的 parent，stdio 的真实本机调用仍可识别其所属会话

### Requirement: Authoritative authenticated API path

网络工具 SHALL 经认证的本实例 TCP REST/WS 执行业务，以真实 TCP authority 和 owner 凭据通过同一权限检查，不走免 Token Unix API、不直接调用服务。出站凭据 SHALL 由受信配置提供，而不是转发入站 Authorization；仅在已认证主体确为同一 owner 时允许这种委托，不得将未来 scoped/OAuth 主体静默升级为 owner。

#### Scenario: Inbound identity cannot choose backend

- **WHEN** 客户端提供伪造 Host、Forwarded、Authorization 变体或目标 URL 元数据
- **THEN** 不能改变出站 API 地址、凭据或身份；同实例 REST 的拒绝仍成为工具错误

### Requirement: Pinned HTTP protocol profile

网络入口 SHALL 使用 initialize-era MCP，基线 `2025-11-25`，可协商 `2025-03-26`、`2025-06-18`；不接受 HTTP `2024-11-05` 旧 SSE profile。未知 initialize 版本 SHALL 回复基线供客户端决定兼容性。`server/discover` SHALL 返回 -32601，不实现旧 `/sse`、OAuth discovery、
授权/注册端点或 server-to-client requests。

#### Scenario: Legacy negotiation is explicit

- **WHEN** 客户端分别初始化受支持旧 HTTP 版本和未知版本
- **THEN** 前者返回其版本，后者返回 2025-11-25；不会宣称实现现代 discover 或旧 HTTP+SSE

### Requirement: Single-message POST framing

每个 POST SHALL 接受一个 UTF-8 JSON-RPC 2.0 对象，不接受 batch 或重复 JSON 字段；Content-Type 为 application/json，Accept 包含 application/json 与 text/event-stream。请求 ID SHALL 是 1–128 UTF-8 字节字符串或安全整数，null/复合/不安全数值无效。正文上限 64 KiB，
超限 413；类型不支持 415，Accept 不符 406。语法错误用 -32700，无效 envelope 用 -32600，参数错误用 -32602。

#### Scenario: Malformed input has no side effect

- **WHEN** 客户端发送 batch、重复字段、畸形 JSON、无效 ID 或超过正文上限的 launch
- **THEN** 在业务执行前失败；可解析的协议错误用 HTTP 400 JSON-RPC error，无法确定请求 ID 时 id 为 null

### Requirement: JSON responses and accepted notifications

有效 request SHALL 返回 HTTP 200 application/json 的单个 JSON-RPC response；工具业务失败使用 `isError:true`，未知方法 -32601。已接受的 notification SHALL 返回空体 202，含 initialized/cancelled；本 profile 不发起 server requests，入站 response SHALL 返回 400。
有效 GET SHALL 返回 405 与 Allow: POST, DELETE；其他方法同样 405。不提供 SSE、进度推送或 HTTP 响应恢复。

#### Scenario: JSON-only transport

- **WHEN** 客户端 POST tools/list、POST initialized 通知，再 GET `/mcp`
- **THEN** 分别得到 JSON response、空体 202、405；不返回 HTML、SSE 或 redirect

### Requirement: Lifecycle input schemas

initialize params SHALL 包含非空 protocolVersion、object capabilities、含非空 name/version 的 object clientInfo；
其他规范扩展字段可忽略，但不得作为身份，完整正文仍受 64 KiB 限制。
initialized params SHALL 缺省或为对象；tools/list 的非空 cursor SHALL 被拒绝，因为本 registry 一次完整返回。
工具 arguments SHALL 沿用已有严格参数检查与 isError:true 工具错误，不接收任意 URL、API 路径或权限声明。

#### Scenario: Client metadata is bounded but not authoritative

- **WHEN** initialize 缺 capabilities/clientInfo，或工具收到不在 schema 中的字段
- **THEN** 前者返回 -32602，后者沿用 isError:true 工具错误；合法 clientInfo 的名称、版本和扩展字段不会改变 owner 身份或网络工具集

### Requirement: Session initialization and binding

成功 initialize SHALL 返回随机 32 字节 base64url 无填充的 MCP-Session-Id。会话 SHALL 绑定 owner principal、进程 epoch、协商版本，状态为 awaiting_initialized 或 ready；只有 initialized 后可 tools/list/call，ping 可在会话存在时使用。初始化不得带会话 ID；
后续请求缺失 ID 返回 400，未知/终止/过期 ID 在认证后返回 404；重复 initialize 不复用旧会话。

#### Scenario: Initialization ordering

- **WHEN** 客户端初始化后尚未发送 initialized 即调用工具
- **THEN** 得到 -32600，未执行工具；发送合法通知后同一会话可正常调用

### Requirement: Session protocol version consistency

后续请求 SHALL 发送 MCP-Protocol-Version，值须与会话协商版本相同；为旧版兼容，缺失时 SHALL 使用该会话已绑定版本，不全局猜测。非法、未知或与绑定不符的版本 SHALL 返回 HTTP 400，不修改会话状态。会话 ID header SHALL 单值且符合 43 字符 base64url，重复/畸形返回 400。

#### Scenario: Version mismatch

- **WHEN** 2025-06-18 会话收到标记 2025-11-25 的请求
- **THEN** 返回 400；随后使用正确版本或省略版本 header 的合法旧客户端请求仍能继续

### Requirement: Bounded ephemeral session state

实例 SHALL 最多保留 128 个 MCP 会话；初始化后 60 秒未 ready、闲置 10 分钟或创建满 1 小时 SHALL 终止并释放状态。活动调用期间不按闲置淘汰，硬期限仍生效。仅保存版本、owner/epoch、时间和在途取消信息，不保存完整 clientInfo、正文或历史。满额 SHALL 返回 429 与 Retry-After: 1，不挤掉活动会话；状态不持久化。

#### Scenario: Restart invalidates transport only

- **WHEN** 实例重启后持旧 MCP 会话 ID 请求
- **THEN** 认证后得到 404；重新初始化后可用原 WebMux 会话 ID、仍有效 cursor/turn selector 继续观察，不伪造持久会话已删除

### Requirement: Independent bounded concurrent calls

实例 SHALL 最多并行 8 个工具调用，每 MCP 会话最多 4 个；调用不在全局或生命周期锁内执行。超额 SHALL 在执行前返回 429 与 Retry-After: 1，不排入无界队列。通知、ping、DELETE SHALL 不占工具执行槽，确保取消能进入。每会话在途 request ID 须唯一，相同 ID 冲突返回 HTTP 409，不作为业务重试缓存。

#### Scenario: Wait cannot block control

- **WHEN** 同一会话中一个 session_wait 在等待，另一客户端请求 session_list 或 interrupt，且未达到上限
- **THEN** 后者能独立完成；达到上限时新工具调用立即 429，取消通知仍能进入

### Requirement: Explicit cancellation and disconnect semantics

cancelled 通知 params SHALL 含合法 requestId 和可选不超过 256 字节 reason，只作用于同一 MCP 会话的在途非 initialize 请求；未知/已完成 ID 或跨会话不匹配 SHALL 无操作并返回 202。显式取消 SHALL 停止该调用的观察/网络等待、释放槽位，不发送 session_interrupt、
不删除持久会话、不保证撤销已提交写入。存活连接的被取消调用 SHALL 返回 -32000 且 data.kind 为 request_cancelled。

#### Scenario: Cancellation is not interruption

- **WHEN** 会话 A 取消其 session_wait，同时会话 B 用相同 request ID 等待另一任务
- **THEN** A 的观察停止，B 不受影响，两项持久任务仍可继续；只有显式 session_interrupt 才请求中断任务

#### Scenario: Disconnect is not explicit cancel

- **WHEN** POST 连接在工具执行中断开
- **THEN** 不合成 cancelled 或 interrupt；调用仍受原期限和并发额度限制，完成后丢弃无法交付的响应并释放资源

### Requirement: Session teardown and request deadlines

合法 DELETE SHALL 以空体 204 终止该 MCP 会话并取消它的在途调用，不终止 WebMux 会话；重复使用该 ID SHALL 404。硬期限终止和服务停机 SHALL 同样释放 observer、WS 与取消索引。工具总期限 SHALL 最多 185 秒；读取十秒、wait 默认三十秒/最多六十秒和初始回放边界 SHALL
保持原契约，不把传输超时当作任务失败或业务回滚。

#### Scenario: Timeout keeps selector

- **WHEN** session_wait 观察超时或 MCP 会话被删除
- **THEN** 不重发输入；前者返回 waitTimedOut，后者结束观察，重新初始化后可用相同 selector 等待仍在执行的任务

### Requirement: Business retry identity stays distinct

JSON-RPC ID SHALL 只关联 MCP response/cancel，不替代工具 clientRequestId。重试同一逻辑写操作 SHALL 使用相同业务 ID 与相同负载，允许新的 JSON-RPC ID。系统 SHALL 不扩大服务端去重：创建为进程内十分钟；混合输入/中断为每会话最近 256 项且三十分钟；终端按既有
submission receipt 保留规则，不提供跨重启 exactly-once。

#### Scenario: Lost write response

- **WHEN** launch 已提交但 HTTP response 丢失，客户端在去重窗口内以新 JSON-RPC ID、原 clientRequestId 和相同负载重试
- **THEN** 得到服务端原结果而不创建第二项；超窗/淘汰/重启后须先查询核对，不以 MCP 会话或 JSON-RPC ID 承诺去重

### Requirement: Shared guide and frozen tool scope

网络控制 SHALL 使用与 stdio 相同的控制工具注册表、schema 和输出规则，含前置 guide_read；上下文 SHALL 为 mcp-http/remote、selfSessionId:null。服务器路径 SHALL NOT 被描述为远程客户端可写，不承诺文件上传/下载或 answer/view 工具。webmux-history 与 thread_read
SHALL 保持仅独立 stdio 冻结来源范围，不加入网络工具表。

#### Scenario: Remote discovery does not grant frozen history

- **WHEN** 远程客户端读取 tools/list、capabilities 和 guide_read，再调用 thread_read
- **THEN** 清单一致且不含 thread_read，调用为未知工具错误，指南不出现本机 socket/可执行文件或隐式 self 的可用性承诺

### Requirement: Bounded outputs and secret hygiene

HTTP 工具完整编码响应 SHALL 不超过 8 MiB 加 64 KiB，超限返回受控 -32603，不发送半个 JSON。既有工具的更小限制 SHALL 保持，包含 guide 完整 64 KiB、读取页和回复上限及 WS 回放限制。响应 SHALL 设置 no-store；日志/错误 SHALL NOT 输出 Token、正文、回复、完整
clientInfo、取消 reason 或会话凭据。失败不终止其他 MCP 会话。

#### Scenario: Oversized response

- **WHEN** 合法查询结果编码后超过 HTTP 输出上限
- **THEN** 返回有界明确错误，连接和其他调用可继续，不静默截断工具字段或扩大下游限制

### Requirement: Explicit Bearer compatibility acceptance

发布说明 SHALL 将此功能称为静态 owner Bearer 的 Streamable HTTP profile，不宣称完整标准 OAuth 授权实现或 OAuth-only 托管客户端兼容。验收 SHALL 使用明确支持自定义 Authorization 的真实 MCP Inspector，以 URL、header 和 initialize-era 配置完成发现、
launch/send/read/wait/interrupt；不得使用 WebMux 二进制、Unix bridge 或本地 socket 替代该链路。

#### Scenario: Actual URL client

- **WHEN** 隔离环境中实际 MCP Inspector 连接受保护的 `/mcp`
- **THEN** 可完成全链路并记录客户端版本、协商版本、结果和限制；仅支持 OAuth 自动发现的托管客户端被明确列为未支持，而不是报告为通过
