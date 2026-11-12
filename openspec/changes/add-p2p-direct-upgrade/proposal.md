# Proposal

## Why

当用户通过 Cloudflare Tunnel 等公网中继从异地或手机网络访问 WebMux 时，每一次按键输入、终端回显与 HTTP 请求都必须绕经中继边缘节点，在跨境或跨运营商场景下常带来额外 50–150 ms 往返延迟，明显削弱交互跟手度。

S0 已完成 `hub.MessageConn` 与前端 `ClientTransport` 抽象。在此基础上，我们可以在保持“先经原连接 1 RTT 首屏可用”且不引入额外基础设施的前提下，于后台机会性建立 WebRTC DataChannel 直连，并在确认直连候选对且实测应用层 RTT 更优时无缝切换实时流与 `/api/*` 请求。

## What Changes

- **WebRTC P2P 直连引擎（`internal/p2p`）与可选编译裁剪**：
  - 基于 `pion/webrtc/v4` 管理 `PeerConnection` 生命周期、并发配额、空闲回收与协议版本握手，并提供 `//go:build !nop2p` 可选编译裁剪。
  - 信令直接复用现有已认证的 HTTP/WebSocket 连接。
  - 默认启用双栈（IPv6 + IPv4）与默认开启的本地路由器端口映射（UPnP IGD / NAT-PMP / PCP，支持配置与网页开关控制）。
- **DataChannel 传输适配与多路通道承载**：
  - 为 `hub.MessageConn` 补齐应用层状态码关闭帧（承载 `1000`/`1011`/`1013`/`4404`/`4408`/`4409`/`4410`）、基于 `BufferedAmount` 的慢客户端背压控制与大消息分片重组。
  - 在 `PeerConnection` 上承载控制通道（握手与应用层 ping/echo RTT 测量）、实时流通道（`/ws/sessions`、`/ws/s/{id}`、`/ws/s/{id}/chat`、`/ws/settings`、`/ws/voice`）以及 `/api/*` 轻量请求/响应与文件上传通道。
- **配置段、受控状态存储与控制诊断接口**：
  - 新增 `[p2p]` 配置段与 `p2p-settings.json` 受控状态存储（`internal/p2psettings`，损坏时 `fail_closed`），暴露 `/api/p2p*` 控制和诊断接口。
  - 默认开启机会性直连升级，预置小米、Bilibili、Cloudflare 与 Google 公共 STUN，支持自定义 STUN 列表及端口映射开关。
  - 禁止 WebMux 会话内进程通过本机套接字篡改 P2P 配置或发起信令。
- **前端路径管理器（`web/src/p2p-*.ts`）与无感切换**：
  - 首屏始终先走当前 HTTP/WebSocket 连接；回环地址、局域网私网、Tailscale 地址或基线低延迟路径自动跳过升级。
  - 后台建连后对比双路径应用层 ping RTT 并校验 ICE 候选对为直连（`host`/`srflx`/`prflx`，拒绝切换到 `relay`）。
  - 满足滞回阈值后切换 `/api/*` 请求，并以同一 `client` ID 带 `generation`/`offset` 在 DataChannel 上 `append` 无缝续接终端流；断连或切网时自动回落至原连接。
- **网络指标、A/B 回显测速与设置界面**：
  - 在设置页“网络”分区新增 P2P 直连升级卡片，覆盖开关、端口映射、STUN 配置、候选对类型、IP 协议族、双路径 RTT、升级耗时、切换次数、首屏耗时及一键双路径回显 A/B 测速。
  - 在侧栏底部主机状态详情中展示当前传输路径（P2P 直连 / WebSocket）与实时延迟。

## Capabilities

### New Capabilities

- `p2p-direct-transport`: 基于 WebRTC DataChannel 的机会性 P2P 直连升级，涵盖信令协商、双栈 ICE 与默认开启的 UPnP/NAT-PMP/PCP 端口映射、`MessageConn` 流式与 `/api/*` 通道承载、双路径应用层 RTT 对比与滞回切换、`append` 无缝续接回落、构建裁剪（`!nop2p`）以及设置页与状态栏指标展示。

### Modified Capabilities

- `effective-configuration`: 将 `[p2p]` 配置段（启用状态、STUN 列表、端口映射开关）与 `p2p-settings.json` 纳入 `configured`/`effective` 诊断视图、字段级来源追踪（`default`/`toml`/`web`）与存储健康状态检查。
- `server-local-api`: 在会话内进程访问限制中新增对 `/api/p2p` 及其子路径的拦截（返回 403），防止 WebMux 会话内程序修改 P2P 网络暴露策略或发起信令。

## Impact

- **后端包与依赖**：新增纯 Go 依赖 `github.com/pion/webrtc/v4`（`CGO_ENABLED=0` 兼容，支持 `nop2p` tag 裁剪）；新增 `internal/p2p`、`internal/p2psettings`，扩展 `internal/config`、`internal/statefile`、`internal/contract`、`internal/wire`、`internal/api` 与 `cmd/webmux`。
- **持久化与架构守卫**：更新 `internal/statefile/catalog.go`、`internal/archtest/layers_test.go`、`internal/archtest/persistence_inventory.json` 与 `docs/persistence.md`。
- **前端与协议契约**：更新 `web/src/contracts/schemas.ts`、`web/src/platform/transport.ts`、`web/src/platform/ws.ts`，新增 `web/src/p2p-*.ts` 路径管理模块与 `P2PSettings.svelte` 设置组件，更新 `NetworkSettings.svelte`、`SidebarFooter.svelte` 及对应契约快照、单元/浏览器/Storybook/E2E 测试与用户故事。
