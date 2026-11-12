# p2p-direct-transport Specification

## Purpose

在保持现有 HTTP/WebSocket 连接首屏即开且无需额外部署中继基础设施的前提下，于后台机会性建立 WebRTC DataChannel 直连并在实测延迟更优时无缝接管实时流与 `/api/*` 请求，在打洞失败、中继候选或断连切网时无感回落。

## ADDED Requirements

### Requirement: 默认机会性升级与配置覆盖

系统 SHALL 默认启用机会性 P2P 直连升级，默认开启本地路由器端口映射（UPnP IGD / NAT-PMP / PCP）。
默认公共 STUN 列表 SHALL 包含 `stun:stun.miwifi.com:3478`、`stun:stun.chat.bilibili.com:3478`、`stun:stun.cloudflare.com:3478` 与 `stun:stun.l.google.com:19302`。

用户 SHALL 能够通过 `config.toml` 的 `[p2p]` 段或网页设置覆盖启用状态、端口映射开关与 STUN 列表：

- 网页覆盖持久化于受控状态文件，执行重置后恢复 `config.toml` 配置；
- 状态文件损坏时 SHALL 按 `fail_closed` 停用 P2P 升级，并保留原文件供诊断。

#### Scenario: 默认配置启动

- **WHEN** 用户未在 `config.toml` 或网页设置中修改 `[p2p]` 配置并启动服务
- **THEN** `GET /api/p2p` 返回启用状态为真、端口映射开启、来源为 `config`，且包含预置的四台公共 STUN 服务器

#### Scenario: 网页修改 STUN 与端口映射并重启保留

- **WHEN** 用户在网页设置中自定义 STUN 列表或关闭端口映射并保存，随后重启服务
- **THEN** 服务采用网页保存的配置且来源标记为 `web`；执行重置后恢复 `config.toml` 的值

#### Scenario: 状态文件损坏时安全关闭

- **WHEN** P2P 设置状态文件包含损坏的 JSON
- **THEN** 服务停用 P2P 直连升级、报告存储不可用，拒绝覆盖损坏文件直至用户显式重置或修复

### Requirement: 本地与低延迟路径自动跳过

前端路径管理器 SHALL 始终先通过当前 HTTP/WebSocket 基线路径完成首屏连接与交互，不得阻塞首屏等待 P2P 协商。

满足以下任一条件时，前端 SHALL 自动跳过后台自动升级（用户手动触发 A/B 测速除外）：

- 页面访问源为回环地址（`localhost`、`127.0.0.1`、`::1`）；
- 页面访问源为 RFC1918 或 ULA 局域网私网地址；
- 页面访问源为 Tailscale 地址（`100.64.0.0/10` 或 `.ts.net`）；
- 基线应用层实测 RTT 已低于 15 ms。

#### Scenario: 回环或 Tailscale 访问自动跳过

- **WHEN** 浏览器通过 `http://127.0.0.1:7681` 或 `https://host.tailnet.ts.net` 打开应用
- **THEN** 首屏直接使用基线 WebSocket/HTTP，后台不发起自动 WebRTC 信令协商，状态标注为本地/低延迟跳过

#### Scenario: 公网高延迟隧道访问触发后台协商

- **WHEN** 浏览器通过公网隧道域名打开应用且实测基线 RTT 高于 15 ms
- **THEN** 首屏先经隧道立即可用，后台异步发起 WebRTC 信令协商

### Requirement: 复用现有鉴权连接的信令与协议握手

WebRTC 信令（Offer、Answer、ICE Candidate 与会话关闭）SHALL 复用现有 HTTP/WebSocket 鉴权、Host 校验与 Origin 校验边界，任何信令接口不得绕过 Token 校验。服务端 SHALL 限制并发协商会话数、信令请求速率与未建连超时回收，且日志 MUST NOT 记录完整 SDP 原文或任何认证凭据。

控制 DataChannel 建立后，双方 SHALL 先交换包含协议版本号、应用版本与特性集合的握手帧；协议主版本不兼容时 SHALL 优雅关闭 P2P 会话并保持基线连接运行。

#### Scenario: 未授权信令被拒绝

- **WHEN** 未携带有效 Token 或 Origin 不合法的客户端请求 `/api/p2p/signal`
- **THEN** 服务端返回 401 或 403，不分配任何 WebRTC 会话资源

#### Scenario: 协议版本不兼容回退

- **WHEN** 控制通道握手帧报告不兼容的协议主版本
- **THEN** 客户端与服务端关闭该 P2P 连接，所有终端流与 API 请求继续走原连接且不中断

### Requirement: 双栈 ICE 与本地网关端口映射

服务端 WebRTC 引擎 SHALL 同时启用 IPv6 与 IPv4 候选收集。

当端口映射开关处于开启状态时，服务端 SHALL 执行以下行为：

- 在后台尝试通过本地网关（UPnP IGD 或 NAT-PMP/PCP）映射外部 UDP 端口并定期续期，将获得的映射用于候选收集；
- 在服务退出或关闭开关时主动释放映射；
- 网关不支持或映射失败时静默降级为纯 STUN 打洞，不得阻塞服务启动或会话运行。

#### Scenario: 路由器不支持端口映射时降级

- **WHEN** 端口映射默认开启但所在网络无 UPnP 或 NAT-PMP/PCP 网关响应
- **THEN** 服务正常继续使用 STUN 收集候选，不使服务或信令报错中断

### Requirement: 直连候选校验与双路径 RTT 滞回切换

客户端与服务端 SHALL 在基线连接与 P2P 控制通道上使用对称的应用层 ping/echo 测量往返延迟。路径管理器 SHALL 校验选中的 ICE 候选对类型：仅当本地与远端候选均为直连类型（`host`、`srflx`、`prflx`，严禁 `relay`），且连续采样下 P2P RTT 相比基线 RTT 至少低 10 ms 且降幅不少于 15% 时，才将传输路径切换至 P2P。

#### Scenario: 选中 relay 候选时不切换

- **WHEN** WebRTC 连接建立但选中的候选对包含 `relay` 类型候选
- **THEN** 路径管理器拒绝切换到该 P2P 连接，继续使用原连接

#### Scenario: 直连候选且 RTT 显著更低时升级

- **WHEN** 选中 `srflx` 或 `host` 直连候选对，基线 RTT 为 110 ms，P2P 控制通道连续实测 RTT 为 25 ms
- **THEN** 路径管理器将活跃传输路径切换至 P2P，并记录升级耗时与切换次数

### Requirement: 全通道 DataChannel 适配与无损分片流控

升级至 P2P 后，DataChannel 传输层 SHALL 承载全部实时流通道（`/ws/sessions`、`/ws/s/{id}`、`/ws/s/{id}/chat`、`/ws/settings`、`/ws/voice`）以及 `/api/*` 请求/响应与文件上传。

流通道适配层 SHALL 满足三项传输保障：

- 大消息自动分片与按序重组，支持超过 64 KiB 的终端快照回放；
- 应用层关闭帧透传业务状态码（如 `1000`、`4404`、`4408`、`4409`、`4410`）与原因；
- 基于发送缓冲水位的慢客户端背压与超时断开保护。

#### Scenario: 大体积终端回放与业务关闭码透传

- **WHEN** 会话缓冲存在超过 128 KiB 的历史输出且经 DataChannel 附着，随后会话退出触发 `4410` 关闭
- **THEN** 客户端完整重组回放数据无截断，并准确收到 `4410` 状态码与关闭原因

#### Scenario: API 请求与文件上传经 DataChannel 完成

- **WHEN** 传输路径处于 P2P 直连状态，前端发起 `/api/*` JSON 请求、二进制拉取或文件上传
- **THEN** 请求经 P2P 通道完成并返回一致的状态码、响应头与响应体，进度回调正常工作

### Requirement: 终端无缺口切换与断连回落

传输路径在基线连接与 P2P 直连之间切换时，终端流 SHALL 采用先建后拆（make-before-break）策略：新通道以同一 `client` ID 携带当前 `generation` 与 `offset` 请求 `append` 增量附着，在新通道建连就绪前保持旧通道接收，就绪后原子切换输入写入端并关闭旧通道，全程不得清屏、丢字节或丢失控制端所有权。

当 P2P 连接断开、ICE 失败、网络切换或页面从后台恢复且 P2P 不可达时，路径管理器 SHALL 立即回落至基线连接并按同一 offset 机制续接，同时在后台按退避策略重新探测直连。

#### Scenario: 活跃输出期间无感升级与回落

- **WHEN** 终端会话持续输出文本期间完成 P2P 升级，随后主动断开 P2P 连接触发回落
- **THEN** 升级与回落过程中终端输出字节连续无缺口、不清屏，输入控制权保持在同一客户端

### Requirement: 可观测指标、A/B 回显测速与构建裁剪

服务端与前端 SHALL 暴露并在设置页网络分区与侧栏主机状态详情中展示核心指标，包括当前连接路径（P2P 直连 / 基线 WebSocket）、候选对类型、IP 协议族（IPv4/IPv6）、端口映射状态、基线与 P2P RTT、升级耗时、切换次数与首屏就绪耗时；设置页 SHALL 提供一键双路径回显 A/B 测速。

Go 构建 SHALL 支持 `-tags nop2p` 编译裁剪标签：启用该标签时不链接 WebRTC 依赖，`/api/p2p` 明确报告当前构建未包含 P2P 支持，其余功能正常运行。

#### Scenario: 查看设置页诊断与运行 A/B 测速

- **WHEN** 用户打开设置页网络分区并点击双路径回显测速
- **THEN** 界面展示当前路径、候选类型、IP 协议族、升级耗时、切换次数及基线与 P2P 的 RTT 对比结果

#### Scenario: 使用 nop2p 标签裁剪构建

- **WHEN** 以 `go build -tags nop2p` 编译服务端二进制并启动
- **THEN** 二进制不引入 WebRTC 运行时包，`GET /api/p2p` 标记构建未包含 P2P 支持，常规 HTTP/WebSocket 会话功能不受影响
