## MODIFIED Requirements

### Requirement: 非安全上下文提示

页面不在安全上下文时，卡片 SHALL 说明推送需要 HTTPS，并给出操作步骤：用 Cloudflare Named Tunnel、Tailscale Serve/Funnel 或其他已授权 HTTPS 入口打开 WebMux，再在该地址开启推送。

#### Scenario: 通过 VPN 地址访问

- **WHEN** 手机通过 `http://100.64.0.2:7681` 打开设置页
- **THEN** 卡片说明需要 HTTPS，并列出改用 Tailscale Serve/Funnel 或 Named Tunnel 等 HTTPS 入口打开的步骤

### Requirement: 不支持来源提示

`GET /api/push` 报告 `unsupported_origin` 时，卡片 SHALL 说明推送只支持 HTTPS 入口与本机回环地址，并提示改用 Tailscale Serve/Funnel、Cloudflare Named Tunnel 等 HTTPS 入口打开。

#### Scenario: 非回环 HTTP 来源

- **WHEN** 页面经 `http://100.64.0.2:7681` 打开，服务端报告 `unsupported_origin`
- **THEN** 卡片显示该说明与改用 HTTPS 入口的提示，不提供开启开关

#### Scenario: 其他 HTTPS 反向代理

- **WHEN** 页面经已授权的其他 HTTPS 反向代理打开
- **THEN** 服务端不报告 `unsupported_origin`，卡片不显示该说明，照常提供开启开关

## ADDED Requirements

### Requirement: Quick Tunnel 提示

`GET /api/push` 报告 `quick_tunnel` 时，卡片 SHALL 说明地址每次启动都会变化，并提示改用 Cloudflare Named Tunnel、Tailscale Serve/Funnel 或其他已授权 HTTPS 入口。

#### Scenario: Quick Tunnel 来源

- **WHEN** 页面经 Quick Tunnel 地址打开，服务端报告 `quick_tunnel`
- **THEN** 卡片说明地址轮换会导致旧订阅失效，并列出改用 Named Tunnel 或 Tailscale Serve/Funnel 的步骤，不提供开启开关
