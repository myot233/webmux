# 网络压缩与 P2P 直连用户故事

本页延续手机、平板与触摸操作组（H 组）的 US-H10–US-H12，描述慢速与高延迟中转网络上的实时连接压缩、页面加载压缩及机会性 WebRTC DataChannel P2P 直连升级。
完整索引见[终端、输入与移动端](terminal.md)。

- **US-H10 压缩实时连接**：作为在移动网络等慢速链路上使用终端的使用者，我希望实时连接压缩在手机上默认开启，并可按设备调整，以便减少回放与输出流量而不拖慢输入。
  验收要点：
  - 设置页“网络”分区提供开关，只保存在当前浏览器（`webmux.wsCompression`）；未选择过时，主指针为粗指针（`(pointer: coarse)`，手机与平板）的设备默认开启，其余设备默认关闭，显式选择优先于默认值；
  - 开启后列表、终端、聊天与设置 WebSocket 均以 `compress=1` 重连，服务端协商 `permessage-deflate`（上下文复用），未带参数时不协商，即使浏览器总是提议该扩展；
  - 切换立即重连已打开的连接并按 offset 续传；
  - 小于 128 字节的消息不压缩；
  - 未开启压缩的终端连接带 `zreplay=1`，回放的快照帧由服务端以 DEFLATE 压缩后封装为 `0x07` 帧，避免在高延迟链路上等待原文；已启用 `permessage-deflate` 的连接不再重复压缩；
  - 回放不受实时输出约 60 帧/秒的合并节奏限制，尽快发出，回放后的首个实时输出也不延迟；
  - 认证与 Origin 校验不变。
- **US-H11 压缩加载应用**：作为在移动网络上打开 WebMux 的使用者，我希望页面资源以最小体积传输且服务端不增加延迟，以便更快加载应用。
  验收要点：
  - 构建时为 1 KiB 及以上的 HTML、JS、CSS、SVG、manifest、文本与 WebAssembly 生成 brotli（质量 11）和 gzip（级别 9）版本，内嵌二进制只保存这两个版本；
  - 按 `Accept-Encoding` 质量值选择 br、gzip 或原文，同等质量优先 br，响应带 `Vary: Accept-Encoding`；
  - 服务时不实时压缩，仅在客户端拒绝两种编码时一次性解出原文；
  - 压缩版本不能按 `.br`/`.gz` 路径直接访问；
  - WOFF2 与 PNG 本身已压缩，按原样提供。
- **US-H12 机会性 P2P 直连升级与双路径测速**：作为经高延迟中转入口连接 WebMux 的使用者，我希望先走现有连接立即打开会话，后台自动探测并无感升级到 WebRTC DataChannel 直连，失败或断连时自动退回基线连接而不中断终端，以便显著降低交互延迟。
  验收要点：
  - 默认配置与网页持久化覆盖：
    - `[p2p]` 默认开启（`enabled = true`、`port_mapping = true`）；
    - 默认 STUN 列表包含 `stun:stun.miwifi.com:3478`、`stun:stun.chat.bilibili.com:3478`、`stun:stun.cloudflare.com:3478` 与 `stun:stun.l.google.com:19302`；
    - 设置页“网络”分区可在线开关直连升级、路由器自动端口映射（UPnP IGD / NAT-PMP / PCP）并编辑 `stun:`/`stuns:` 列表（拒绝 `turn:`/`turns:`）；
    - 网页覆盖保存在 `<state_dir>/p2p-settings.json`（目录 0700、文件 0600、独占锁）并立即生效，支持“恢复配置文件默认”；
  - 浏览器访问主机为本机回环、RFC1918/CGNAT/链路本地内网、`.ts.net` 或连续 3 次基线探测 RTT 中位数 `< 15 ms` 时自动跳过协商（`?p2p=force` 仅当前标签页强制探测）；
  - 信令全部复用现有 Bearer Token + Host + Origin 校验下的 HTTP 接口（`/api/p2p`、`/api/p2p/settings`、`/api/p2p/ping`、`/api/p2p/sessions`），不依赖外部信令服务器；日志不输出终端内容、Token 或完整 SDP 正文；
  - 服务端使用单端口 ICE UDP 复用同时收集 IPv4 与 IPv6 候选，并在开启端口映射时自动续期、关闭或退出时清理外部映射；若选定候选对任一端为 `relay`（TURN），双方立即关闭该 PeerConnection 并保持基线路径；
  - 后台协商总窗口为 30 秒，候选按会话串行批量交换，并约每秒拉取服务器新增候选；本地候选收集完成后仍能收到晚到的远端候选，成功、失败或取消后停止信令轮询；
  - 控制通道完成版本握手后测量 P2P RTT，仅当连续 3 次满足“比基线低至少 `15 ms` 且不高于基线 `80%`”滞回阈值（`?p2p=force` 豁免 RTT 比较）才切入直连，同一网络纪元内切换上限 3 次且冷却至少 30 秒；
  - 直连生效后承载全部实时流（`/ws/s/{id}`、`/ws/sessions`、`/ws/s/{id}/chat`、`/ws/settings`、`/ws/voice`）以及 `/api/*` 请求与分块文件上传，单条应用消息按 32 KiB 分片重组并保留业务关闭码（如 `4404`、`4409`）；
  - 终端流采用先建后拆（make-before-break）：新 DataChannel 以 `resume=offset&mode=append` 就绪前旧 WebSocket 不断开，按重叠字节裁剪实现零丢字节、零重复渲染且不触发清屏重放；
  - P2P 断开、心跳超时或浏览器报告切网（`online` / Network Information `change`）时立即退回 WebSocket 基线，取消旧协商并合并 1 秒内的网络事件，以新的 PeerConnection 重收集候选；旧请求的迟到结果不得覆盖新路径，关闭开关或停止客户端立即取消活动协商；
  - 回到前台时验证活动路径；未使用直连时重新评估，包括此前直连成功但收益不足的情形；浏览器不提供网络变化事件时依靠连接状态与心跳恢复；
  - 侧栏底部“主机状态详情”与设置页“网络”分区展示当前传输路径（`P2P 直连` / `WebSocket 基线`）、候选对类型（`host`/`srflx`/`prflx`）、IP 族、端口映射状态、基线与直连 RTT、升级耗时与切换次数，并提供一键“双路径回显测速”对比两条路径的中位/最小 RTT；
  - 打开设置页与端口映射状态轮询只更新诊断，不重建正常直连，也不取消进行中的 ICE 协商或双路径测速；保存或恢复 P2P 配置时才重新评估连接，服务端报告关闭或不支持时仍立即回退并取消协商；
  - 设置页诊断区分“未能建立直连”和“直连成功，延迟收益不足”，后者保留候选类型、IP 族与实测 RTT，不计作穿透失败；
  - 支持 `-tags nop2p` 编译裁剪 WebRTC 依赖，裁剪构建下 `/api/p2p` 返回 `supported: false`、信令返回 `501`，其余 HTTP/WebSocket 功能照常运行。

## 取舍

- 实时连接压缩在手机与平板上默认开启：这类设备常走移动网络，带宽是瓶颈；桌面默认关闭，因为本机与局域网访问时压缩只增加 CPU 与每连接约 1.3 MB 服务端内存。默认值只在用户未选择时生效，选择一次后固定。
- 以主指针是否为粗指针判断设备，而不是 UA 或屏幕宽度：窄窗口的桌面浏览器与带触摸屏的笔记本（主指针为鼠标）不受影响。
- 开关按设备保存而非跨设备同步：手机走移动网络、桌面走局域网时可分别选择。
- 静态资源压缩没有开关：压缩在构建期完成，服务时只返回已有字节，不增加首个请求的延迟。
- 同时保留 brotli 与 gzip：Chrome 在非本机的纯 HTTP 地址上不提议 `br`，经 VPN 的 HTTP 访问仍需 gzip。
- P2P 升级坚持“基线先行、零中继成本”：首屏与鉴权始终走现有 HTTP/WebSocket，绝不因 UDP 被拦截或对称 NAT 打洞失败阻塞会话打开；显式拒绝 `relay` 候选，不部署或要求任何 TURN 服务器。
- 本机、局域网、Tailscale 与 `< 15 ms` 低延迟链路默认跳过 P2P：这些场景下直接 TCP/WebSocket 已经足够快，跳过可避免无意义的 ICE 探测与端口映射开销。

## 代码依据

- [实时连接压缩开关](../../web/src/components/settings/NetworkSettings.svelte)
- [P2P 直连面板与双路径测速](../../web/src/components/settings/P2PSettings.svelte)、[P2P 设置 Hook](../../web/src/hooks/settings/p2p-settings.svelte.ts)
- [前端路径管理与握手](../../web/src/platform/p2p.svelte.ts)、[Peer 会话](../../web/src/platform/p2p-session.ts)、[DataChannel 传输与 RPC](../../web/src/platform/p2p-transport.ts)、[流分片协议](../../web/src/platform/p2p-framing.ts)
- [连接与无感切换](../../web/src/platform/ws.ts)、[终端重叠裁剪](../../web/src/domains/terminal/terminal-protocol.ts)、[终端连接切换](../../web/src/hooks/terminal/terminal-socket.svelte.ts)
- [WebRTC 引擎](../../internal/p2p/service_webrtc.go)、[流连接封装](../../internal/p2p/stream_conn.go)、[路由器端口映射](../../internal/p2p/portmap.go)、[`nop2p` 裁剪构建](../../internal/p2p/service_nop2p.go)
- [P2P 持久设置](../../internal/p2psettings/store.go)、[P2P 路由与桥接](../../internal/api/p2p.go)、[P2P 控制器](../../cmd/webmux/p2pctl.go)
- [握手协商](../../internal/api/router.go)、[协商用例](../../internal/api/compression_test.go)、[P2P API 用例](../../internal/api/p2p_test.go)
- [构建期预压缩](../../web/scripts/embed-assets.ts)、[静态资源协商](../../internal/api/static.go)、[静态资源用例](../../internal/api/static_test.go)
- [协议说明](../protocol/websocket.md#压缩)
- [移动网络部署与验收](../deployment/remote-access.md#移动网络上的-p2p-直连)

## 验证范围

Go 用例覆盖握手协商、编码选择、DataChannel 分片背压、端口映射生命周期、真实 Pion 双端打洞、`/api/*` RPC 转发与 `nop2p` 构建；前端与隔离实例 Chromium 用例覆盖开关、先建后拆无缺口续接、真实 WebRTC 直连升级、双路径测速与断连回落。
未做跨运营商对称 NAT 与移动蜂窝真机测速。
