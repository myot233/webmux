## Context

- `internal/api/push.go` 的 `pushOrigin` 目前只接受当前有效 Named 主机名与 `127.0.0.1`、`::1`、`localhost`，其余来源返回 `unsupported_origin`；Quick Tunnel 单独返回 `quick_tunnel`。
- `internal/api/router.go` 的中间件已经对 `/api/*` 执行认证、Host 白名单与 Origin 校验。`internal/api/public.go` 以来源为键保存 HTTPS origin，Tailscale 控制器在 Serve/Funnel 就绪后登记 `tailscale`，Cloudflare 控制器登记 `cloudflare`。
- `allowed_origins` 是手工部署 Tailscale、Caddy 等 HTTPS 入口的授权方式；写请求若 Origin 不在其中会被中间件返回 403。
- Host 白名单按主机名匹配，忽略了 `allowed_origins` 中 HTTPS 条目的端口，因此无 `Origin` 的同源 GET 仍可能需要从配置推断浏览器看到的 scheme。

## Goals / Non-Goals

**Goals:**

- 让托管 Tailscale Serve/Funnel 与其他已授权的 HTTPS 入口可以开启推送。
- 保持 Quick Tunnel 排除，因为其地址每次启动变化，订阅锚点会失效。
- 让通知 `navigate` 使用规范化、端口感知的 HTTPS authority。
- 保持回环的 `http` 与 `https` 两种安全上下文。

**Non-Goals:**

- 不为非回环 `http` 开启推送，也不把本机 Unix socket 视为浏览器来源。
- 不改变认证、Host/Origin 中间件或 `allowed_origins` 的授权语义。
- 不改动推送加密、VAPID、队列、Service Worker 或通知点击逻辑。
- 不自动为手工部署的 Tailscale/TLS 反代写入 `allowed_origins`。

## Decisions

### 1. 信任边界由中间件负责，`pushOrigin` 只区分稳定来源

`pushOrigin` 在中间件之后运行：带 Origin 的请求已通过 `originAllowed`，无 Origin 的同源 GET 已通过 Host 白名单。因此通用分支可以直接接受 `https://` 来源，不再重复维护一份白名单。这样托管 Tailscale 无需额外配置，`allowed_origins` 中的 HTTPS 条目也自然生效；未授权的来源在中间件阶段已经 403，不会到达推送判定。

### 2. Quick Tunnel 必须先于通用 HTTPS 分支排除

`PublicOrigins` 对 Named 与 Quick 都使用 `cloudflare` 来源。判定顺序为：回环 → Quick Tunnel → Named → 通用 HTTPS。Quick 条件为 `cloudflare` 条目的 authority 等于请求 authority，且该 authority 不是当前有效 Named 主机名。这样 Named 运行时不会被误判，Named 已配置但 Quick 正在运行时仍会拒绝 Quick 地址。

### 3. 锚点按 authority 规范化

HTTPS 来源统一经 `normalizeAuthority` 处理：小写主机、省略 `:443`、保留非默认端口。回环来源保留原有 `scheme://host` 形式，避免把 `http://localhost:443` 错误地改成无端口。无 `Origin` 的 GET 在现有 Named/公网来源之外，追加“Host 主机名命中 `allowed_origins` 的 HTTPS 条目”即视为 `https`，并以该条目的规范 authority（含配置端口）作为锚点，避免反向代理省略端口时丢失。

### 4. 文案与文档说明可用入口，不承诺 Quick Tunnel

服务端 `unsupported_origin` 文案、设置页非安全上下文与不支持来源提示、协议与部署文档统一改为“HTTPS 入口或本机回环地址”，并点名 Tailscale Serve/Funnel。`quick_tunnel` 文案保留“地址每次启动都会变化”的说明，并列出 Named Tunnel 与 Tailscale Serve/Funnel 等替代入口。
