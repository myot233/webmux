## Why

本变更对应 #154。Web Push 的 `pushOrigin` 仍把来源限制为 Cloudflare Named Tunnel 与回环地址。Tailscale Serve/Funnel 的 HTTPS 入口已经通过 `api.PublicOrigins` 登记，Host/Origin 中间件也允许其写请求，但推送判定没有读取这个来源，因此 `https://<node>.ts.net:8443` 页面必然收到 `unsupported_origin`。手工 `tailscale serve`、Caddy 等已加入 `allowed_origins` 的 HTTPS 反代同样被拒绝。用户已经有 Tailscale HTTPS 入口，除地址会变化的 Quick Tunnel 外都应支持推送。

## What Changes

- 放宽推送来源判定：请求到达 `pushOrigin` 前已经通过 Host/Origin 中间件，因此任何 `https://` 来源都可作为订阅锚点；回环继续同时接受 `http` 与 `https`。非回环 `http` 与本机套接字仍为 `unsupported_origin`。
- Quick Tunnel 保持排除：当前 `cloudflare` 来源匹配请求来源且不是有效 Named 主机名时返回 `quick_tunnel`。该判断 MUST 位于通用 HTTPS 分支之前，并且即使同时配置了 Named 主机名也不能放行当前 Quick Tunnel 地址。
- 锚点规范化：非回环 HTTPS 来源按 authority 规范化，省略 `:443`，保留 `8443`、`10000` 等非默认端口，IPv6 字面量保留方括号，作为通知 `navigate` 的前缀；回环来源保留原 scheme 与 authority。
- 无 `Origin` 的 `GET /api/push` 继续推断 scheme：除 Named 主机名与已登记公网来源外，Host 命中 `allowed_origins` 的 HTTPS 条目时也按 `https` 判定，避免反向代理下卡片误报 `unsupported_origin`。
- 更新服务端错误文案、设置页提示、协议文档、Tailscale/Cloudflare 文档与相关用户故事，说明 HTTPS 入口（含 Tailscale Serve/Funnel）可用，Quick Tunnel 仍不可用。
- 不改变信任边界：托管 Tailscale 无需额外配置；手工 `tailscale serve` 或其他 TLS 反代仍需把 HTTPS origin 加入 `allowed_origins`，否则写请求在中间件即返回 403。订阅注册仍要求 token。

## Capabilities

### New Capabilities

- 无。

### Modified Capabilities

- `attention-push`：推送来源资格从 Named Tunnel 与回环放宽为通过 Host/Origin 校验的 HTTPS 入口与回环；登记订阅记录规范化后的 HTTPS 锚点。
- `push-notification-client`：非安全上下文与不支持来源的提示改为说明 HTTPS 入口（含 Tailscale Serve/Funnel），Quick Tunnel 提示保留地址轮换说明并列出替代入口。

## Impact

- 后端：`internal/api/push.go` 的来源判定与文案、`allowed_origins` 的 HTTPS scheme 推断；无新增路由、状态文件或配置项。
- 前端：`web/src/components/PushSettings.svelte` 的提示文案；浏览器用例断言同步。
- 测试：`internal/api/push_test.go` 覆盖 Tailscale 端口、`allowed_origins` HTTPS、Quick Tunnel 与 Named 并存、规范化与拒绝路径；新增 `push-status-tailscale` 线上快照。
- 文档：`docs/protocol/http-push.md`、`docs/cloudflare-tunnel.md`、`docs/tailscale.md`、`docs/deployment.md`、`docs/user-stories/access-and-sessions.md`、`docs/user-stories/clients-network-tray.md`。
- 不涉及：推送加密、队列、VAPID、Service Worker、通知点击逻辑、PTY 与会话生命周期。
