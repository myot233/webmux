# Design

## Context

S0 已在 `main` 落地两项基础重构（见 `openspec/changes/p2p-shell-s0-foundations/`）：

1. 后端 `internal/hub` 引入 `MessageConn` 接口，将 `/ws/sessions`、`/ws/s/{id}`、`/ws/s/{id}/chat`、`/ws/settings` 与 `/ws/voice` 的会话桥接逻辑从 `*websocket.Conn` 解耦；
2. 前端引入 `web/src/platform/transport.ts` 的 `ClientTransport`（`fetch`、`upload`、`openStream`）与 `setClientTransport()`。

在此基础上实现 P2P 线（背景见 `proposal.md - Why`），还需遵守仓库的严格架构与安全约束：

- `internal/archtest` 分层规则（R0–R11）：控制器不碰文件系统与底层状态存储，服务层不依赖 `net/http` 服务端符号或控制器，所有 JSON 契约集中在 `internal/contract` 并经 `internal/wire` 转换，单生产文件严格 `< 500` 行；
- 持久化治理（`TestPersistenceInventory`）：新设置存储必须通过 `internal/statefile` + `internal/settingsstore` 统一管理，登记描述符与清单并具备 `fail_closed` 行为；
- 零 CGO 构建（`CGO_ENABLED=0`）并支持 `-tags nop2p` 可选编译裁剪。

## Goals / Non-Goals

**Goals:**

- 首屏保持“当前页面连接 1 RTT 直出”，WebRTC 打洞、端口映射与 RTT 探测完全在后台异步完成，绝不阻塞首屏。
- 在同一 `RTCPeerConnection` 上同时承载控制通道（握手与 RTT ping/echo）、全部 `/ws/*` 实时流通道以及 `/api/*` 请求/响应与文件上传。
- 实现双栈 ICE（IPv6 + IPv4）与默认开启的纯 Go 本地网关端口映射（UPnP IGD / NAT-PMP / PCP），无网关时静默退化为纯 STUN。
- 仅在确认直连候选对（`host`/`srflx`/`prflx`）且实测应用层 RTT 满足滞回优势时执行先建后拆（make-before-break）切换，保证终端字节零丢失、不清屏、控制权不闪烁；断连或切网立即回落。
- 提供 `-tags nop2p` 构建标签裁剪，使受限环境可零 WebRTC 依赖编译。

**Non-Goals:**

- 本变更不引入自建信令服务器或公网 TURN 中继服务器（对称 NAT 打洞失败直接保持原隧道连接）。
- 不实现跨域名稳定 PWA 壳与端口转发专用监听（属于 Issue #399 壳线及后续阶段）。

## Decisions

### 1. 分层与包边界：`internal/p2p` + `internal/p2psettings` + 组装根注入

- **服务层职责划分**：
  - `internal/p2psettings`（`service` 层）：基于 `settingsstore.Store[config.P2PConfig]` 管理 `p2p-settings.json`（`statefile.P2PSettings`，`fail_closed`），负责配置归一化、STUN URL 校验（仅允许 `stun:`/`stuns:` 方案、主机端口合法、上限 8 台）与重置。
  - `internal/p2p`（`service` 层）：管理 WebRTC `PeerConnection`、端口映射、流通道分片封装与 RPC 通道协议。为满足 `archtest` R1/R6（服务层不依赖控制器或 `net/http` 服务端符号），`internal/p2p` 定义纯传输无关的 `StreamHandler` 与 `RPCHandler` 回调接口，由唯一组装根 `cmd/webmux` 注入。
- **`/api/*` 复用控制器路由**：
  - 在 `internal/api` 提供内部执行入口，将 DataChannel RPC 请求转交给现有路由处理器（复核当前 Bearer Token 并屏蔽 `/local/*`），使所有 `/api/*` JSON 端点、二进制读取与 multipart 文件上传 100% 复用现有校验、状态码映射与契约快照，避免维护第二套业务路由。
- **备选方案**：在 DataChannel 上重做独立业务 RPC 路由表。否决原因：会导致几十个 `/api/*` 端点逻辑分叉且容易遗漏鉴权或错误状态映射。

### 2. 信令传输：复用现有已认证连接（`/api/p2p/signal` 与 `/api/p2p/ping`）

- **方案**：
  - 信令通过现有已认证连接上的 `POST /api/p2p/signal`（创建 Offer/Answer 会话、提交增量 ICE 候选、主动关闭会话）完成，天然穿透 HTTP 代理与 Cloudflare Tunnel，并直接复用 `internal/api` 的 Bearer Token、Host 与 Origin 校验以及 `local.fromSession(r)` 会话内进程 403 拦截。
  - 提供轻量 `GET /api/p2p/ping` 用于基线路径应用层 RTT 采样，与 P2P 控制通道上的 `ping`/`pong` 保持完全对称的测量口径。
- **备选方案**：单独维护长生命周期 `/ws/p2p` 信令连接。否决原因：打洞完成后信令通道长期闲置占用连接槽位，带 `peerId` 的短请求信令更加轻量。

### 3. DataChannel 帧协议：`MessageConn` 分片、状态码关闭帧与背压

浏览器与 Pion 的 SCTP DataChannel 单消息存在最大尺寸限制，且原生 `RTCDataChannel.close()` 不携带 WebSocket 自定义状态码（如 `4404`/`4408`/`4409`/`4410`）。每个流式 DataChannel（标签 `stream:<path>`）采用单字节帧头封装：

- `0x01`（文本尾片）与 `0x02`（文本续片）：承载 `MessageText`；
- `0x03`（二进制尾片）与 `0x04`（二进制续片）：承载 `MessageBinary`，单分片上限 `32 KiB`，接收端按序重组并严格执行 `SetReadLimit(n)`；
- `0x05`（应用层 Close 帧）：格式为 `[0x05][uint16 code][utf8 reason]`，送达对端后原样触发 `TransportSocket.onclose({ code, reason })`，随后关闭底层 DataChannel。
- **慢客户端背压**：服务端设置 `SetBufferedAmountLowThreshold(256 KiB)` 与高水位 `1 MiB`；超过高水位时等待缓冲回落或写入超时，超时后与 `wsConn` 一致触发 `4408 StatusSlowClient` 驱逐，绝不阻塞 PTY 读取。

### 4. 控制通道与 `/api/*` DataChannel RPC 协议

- **控制通道（label: `webmux:control`）**：
  1. 打开后双方交换 `hello` JSON 帧（含 `proto: 1`、`appVersion`、`features` 与服务端观测到的候选类型/端口映射状态）；`proto !== 1` 时立即关闭 P2P 会话。
  2. 承载 `{type: "ping", seq, clientTs}` / `{type: "pong", seq, clientTs}`，用于日常平滑 RTT 监控与设置页一键双路径回显 A/B 测速。
- **API 请求通道（每请求独立有序可靠 DataChannel `rpc`）**：
  - 每个 `/api/*` 请求（含 `upload`）打开独立 SCTP 流，天然避免大文件传输阻塞并发小请求，且 `AbortSignal` 取消时直接关闭该流即可终止：
    - 请求由 JSON 元数据首帧 `{method, path, headers, bodyLength}` + `32 KiB` 二进制请求体分片 + 结束帧组成；上传时结合 `bufferedAmount` 驱动 `onProgress` 回调。
    - 响应由 JSON 元数据首帧 `{status, statusText, headers}` + `32 KiB` 响应体分片 + 结束帧组成。

### 5. 纯 Go 端口映射（默认开启）与 `-tags nop2p` 构建裁剪

- **单端口 UDP 复用与默认端口映射**：
  - 服务端使用 `webrtc.NewICEUDPMux` 将所有 `PeerConnection` 复用到单个本地 UDP 端口，使网关只需维护一条端口映射。
  - 使用纯标准库 `net` 实现轻量 NAT-PMP/PCP（RFC 6886/6887）与 UPnP IGD（SSDP + SOAP `AddPortMapping`）映射器，后台自动探测默认网关、定期续租并在关闭时释放；网关不可用或 2 秒超时未响应时静默退化为纯 STUN，不引入重型第三方依赖。
- **`//go:build !nop2p` 与 `//go:build nop2p`**：
  - 将 Pion 相关实现隔离在 `!nop2p` 文件中，并提供 `nop2p` 存根实现（`Supported() == false`），满足裁剪构建需求。

### 6. 前端先建后拆（Make-Before-Break）与滞回切换状态机

- **路径判定与滞回切换**：
  1. 首屏直接使用基线 HTTP/WS `ClientTransport` 并记录首屏就绪时间。
  2. 访问源命中回环、私网或 Tailscale 时标记 `skipped_local`；基线中位 RTT `< 15 ms` 时标记 `skipped_low_rtt`，均不自动协商。
  3. 后台建立 `RTCPeerConnection` 后通过 `getStats()` 检查选中 `candidate-pair`：含 `relay` 候选时拒绝切换；直连候选（`host`/`srflx`/`prflx`）且连续 3 次采样满足 `p2pRttMs <= baselineRttMs - 10` 且 `p2pRttMs <= baselineRttMs * 0.85` 时触发切换。
- **无损 Make-Before-Break 流切换**：
  - 在 `web/src/platform/ws.ts` 中新增平滑切换钩子，路径升级时先保持旧 WebSocket 接收，同时用最新 `url()` 打开新流：
    - 终端流在 `url()` 中携带同一 `client` ID、当前 `generation`、`offset` 与 `zreplay=1`，且在 DataChannel 下省略 `compress=1`；
    - 新流 `onopen` 就绪后原子替换活跃引用并静默关闭旧连接；若新流建连失败则直接丢弃，旧连接零抖动。
  - 当 P2P 断开、心跳超时或网络切换时，立即将 `ClientTransport` 切回基线 HTTP/WS 并按同一 offset 续接，后台按指数退避重新探测。

## Risks / Trade-offs

- **[Risk] 对称 NAT（如蜂窝网叠加企业防火墙）无法打洞** → 后台 ICE 超时后自动回收 `PeerConnection`，前端保持原隧道连接无感运行，设置页如实展示打洞状态。
- **[Risk] 终端大快照（最高 2 MiB）超过 SCTP 单消息上限或撑满缓冲** → 流适配层强制按 `32 KiB` 分片发送并在接收端按序重组，配合 `BufferedAmount` 1 MiB 水位与 `SetReadLimit` 双向限流。
- **[Risk] 切换瞬间新旧流短暂并存产生边界重叠** → 依托服务端同一把锁内的 `0x04` 位置帧与前端基于绝对 `offset` 的推进校验，确保字节精确衔接、不丢字不重字。
- **[Risk] 引入 `pion/webrtc/v4` 增大编译产物体积** → 仅引入 DataChannel 所需子包，提供 `-tags nop2p` 编译裁剪，并实测记录 `CGO_ENABLED=0` 二进制体积增量。

## Migration Plan

1. 升级完全向后兼容：未配置 `[p2p]` 的现有 `config.toml` 自动采用默认策略（远程高 RTT 机会性升级、本地/Tailscale 自动跳过），无需手工迁移。
2. 回滚策略：用户可在网页设置页随时关闭“机会性 P2P 直连升级”或在 `config.toml` 设置 `[p2p] enabled = false`，所有流量立即回到原有 HTTP/WebSocket 路径。
