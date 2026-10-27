## MODIFIED Requirements

### Requirement: 推送来源判定

系统 SHALL 按页面来源判定推送资格；请求带 Origin 时以 Origin 为准，否则以 Host 推断。请求到达该判定前 MUST 已通过 Host 与 Origin 校验。符合资格的来源为：

- 任何通过校验的 `https://` 来源，包括托管 Named Tunnel、托管 Tailscale Serve/Funnel 与 `allowed_origins` 中的 HTTPS 条目；
- 主机为 `127.0.0.1`、`::1` 或 `localhost` 的 `http` 或 `https` 回环来源。

非回环 HTTPS 来源 SHALL 以规范化 authority 作为通知 `navigate` 的锚点：省略 `:443`，保留 `8443`、`10000` 等非默认端口，IPv6 字面量保留方括号；回环来源 SHALL 保留浏览器发送的 scheme 与 authority。当前 Quick Tunnel 公网来源 SHALL 判为 `quick_tunnel`，即使同时配置了 Named 主机名也不得放行。非回环 `http` 来源与本机套接字 SHALL 判为 `unsupported_origin`。

#### Scenario: Named Tunnel 来源

- **WHEN** 配置 `tunnel.token` 与 `tunnel.hostname = "term.example.com"`，手机从 `https://term.example.com` 请求 `GET /api/push`
- **THEN** 响应 `available` 为 `true`，锚点为 `https://term.example.com`

#### Scenario: 托管 Tailscale 来源

- **WHEN** Tailscale Serve 就绪于 `https://node.tailnet.ts.net:8443`，浏览器从该来源请求 `GET /api/push`
- **THEN** 响应 `available` 为 `true`，锚点保留 `:8443`

#### Scenario: allowed_origins 中的 HTTPS 反向代理

- **WHEN** 手工 `tailscale serve` 或其他 TLS 反向代理来源已加入 `allowed_origins`，浏览器从该来源请求 `GET /api/push`
- **THEN** 响应 `available` 为 `true`

#### Scenario: 无 Origin 的 HTTPS 入口

- **WHEN** 反向代理后的同源 `GET /api/push` 不带 Origin，Host 主机名命中 `allowed_origins` 的 HTTPS 条目；即使代理省略了端口
- **THEN** 推送状态仍按 `https` 判定为可用，锚点保留该条目配置的端口（如 `https://proxy.example:9443`）

#### Scenario: Quick Tunnel 来源

- **WHEN** Quick Tunnel 就绪于 `https://abc.trycloudflare.com`，即使同时配置了 Named 主机名，浏览器从该来源请求 `GET /api/push`
- **THEN** 响应 `available` 为 `false`，`reason` 为 `quick_tunnel`

#### Scenario: 局域网或 VPN 地址

- **WHEN** 浏览器从 `http://100.64.0.2:7681`（已在 `allowed_origins` 中）请求 `GET /api/push`
- **THEN** 响应 `available` 为 `false`，`reason` 为 `unsupported_origin`

#### Scenario: 桌面回环地址

- **WHEN** 本机浏览器从 `http://127.0.0.1:7681` 请求 `GET /api/push`
- **THEN** 响应 `available` 为 `true`

### Requirement: 登记订阅

`POST /api/push/subscriptions` SHALL 接受 `{"endpoint": "...", "keys": {"p256dh": "...", "auth": "..."}}`，返回 200 与 `{"id": "<订阅 ID>"}`。endpoint MUST 是不含凭据、长度不超过 2048 字节的 `https://` URL；`p256dh` MUST 解码为 P-256 曲线上的 65 字节未压缩点，`auth` MUST 解码为 16 字节，否则返回 400。同一 endpoint 再次登记 SHALL 更新密钥并返回原 ID。订阅 SHALL 记录登记时的页面来源；HTTPS 来源 MUST 规范化为小写 authority，省略 `:443`，保留其他端口，作为通知锚点。来源不符合资格时 SHALL 返回 409 及原因；已有 32 个订阅时登记新 endpoint SHALL 返回 409。

#### Scenario: Tailscale 来源登记

- **WHEN** 浏览器从 `https://node.tailnet.ts.net:8443` 登记订阅
- **THEN** 返回 200 与订阅 ID，存储的锚点为 `https://node.tailnet.ts.net:8443`

#### Scenario: 幂等登记

- **WHEN** 同一浏览器用相同 endpoint 连续登记两次
- **THEN** 两次返回相同 `id`，`GET /api/push` 的 `subscriptions` 为 1

#### Scenario: Quick Tunnel 来源被拒

- **WHEN** 浏览器从 Quick Tunnel 公网来源登记订阅
- **THEN** 返回 409，错误说明地址每次启动都会变化，并列出 Named Tunnel 或 Tailscale Serve/Funnel 等替代入口，订阅数不变

#### Scenario: 非法 endpoint

- **WHEN** 请求体 endpoint 为 `http://push.example/x`
- **THEN** 返回 400，不保存订阅

#### Scenario: 订阅已满

- **WHEN** 已有 32 个订阅，又登记一个新 endpoint
- **THEN** 返回 409，提示先在其他设备关闭推送

#### Scenario: Origin 不符

- **WHEN** 登记请求的 Origin 不在允许来源内
- **THEN** 返回 403，与其他写接口相同
