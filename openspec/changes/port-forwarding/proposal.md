# Proposal

## Why

在 WebMux 里开发 Web 应用时，开发服务器（例如 Vite 的 `127.0.0.1:5173`）只监听服务器本机。要在自己的浏览器里预览，现在只有两种办法：另开 SSH 隧道，比较麻烦；或者把开发服务器暴露到局域网或公网，不安全。

VS Code Remote 和 Codespaces 的“端口转发”解决的就是这个问题：服务器上一出现监听端口，就提示用户转发，点一下就能在本地浏览器打开。

WebMux 已经有一个经过认证、可以通过 Tailscale 或 Cloudflare Tunnel 访问的 HTTP 入口，再加一个受认证保护的反向代理路由就能实现同样的效果，不需要新开端口。

## What Changes

- 新增转发网关路由 `/forward/{port}/`，运行在 WebMux 现有监听地址上，把 HTTP（包括流式响应和 WebSocket 升级）反向代理到服务器本机回环地址的对应端口。不新开任何监听端口。只有已添加的转发能访问，目标只能是本机回环地址，并且不能指向 WebMux 自己的端口。
- 网关使用专用 Cookie `webmux_forward` 认证：
  - Cookie 里放的是服务端内存中随机生成的转发凭据，不是主 Token；
  - 属性为 `HttpOnly`、`SameSite=Strict` 和 `Path=/forward/`，只能访问转发，不能访问 `/api/` 和 `/ws/`，服务重启后失效；
  - 获取时，前端先用主 Token 换一个一次性兑换码，再提交表单兑换 Cookie，兑换码不出现在 URL 中；
  - 直接打开转发地址但没有有效 Cookie 时，网关自动跳转到认证页，完成认证后回到原地址。
- 每个转发都可以设置开发服务器兼容选项，默认全部开启，用来保证 Vite 这类开发服务器开箱即用：
  - 把上游 `Host` 和 `Origin` 改写为 `localhost:{port}`；
  - 补充 `X-Forwarded-*` 头；
  - 把响应中的 `Location`、`Set-Cookie` 和 `Access-Control-Allow-Origin` 改写回网关地址；
  - 选择保留还是去除 `/forward/{port}` 前缀。
- 转发配置只保存在服务端内存中，不写入磁盘。转发只能手动移除，否则一直保留到服务重启，与创建它的会话无关。多个浏览器之间通过 `/ws/sessions` 实时同步。
- 会话信息新增“监听端口”：服务端定期扫描每个会话进程树中处于监听状态的 TCP socket，支持 Linux 和 macOS。终端页检测到当前会话出现尚未转发的新端口时，在终端上方显示提示，可以转发并打开，也可以忽略。
- 首页顶栏新增“端口转发”按钮，带活跃转发数量，点击打开转发对话框，手机上显示为抽屉。对话框里可以新增、打开、复制链接、修改兼容选项和移除转发，也可以直接转发各会话检测到的端口。
- 新增 Storybook 原型 `Workspace/Port forwarding`，覆盖首页转发对话框和终端端口提示的主要状态。

## Capabilities

### New Capabilities

- `port-forwarding`：转发网关、转发凭据 Cookie 与兑换流程、开发服务器兼容改写、转发的内存登记与管理接口、首页转发对话框。
- `session-port-detection`：检测会话进程树中的监听端口，并通过会话信息下发；终端页提示新端口，支持转发或忽略。

### Modified Capabilities

- `access-link`：明确 Cookie 的边界：主 Token 仍然不能写入 Cookie；唯一允许的 Cookie 是 `webmux_forward` 转发凭据，它不能认证 `/api/` 和 `/ws/`。

## Impact

- 后端：
  - 新增服务包 `internal/forward`，负责转发登记、转发凭据和兑换码，以及端口校验。需要在 `internal/archtest/layers_test.go` 中登记分层。
  - `internal/api` 新增 `/forward/` 网关处理器（基于 `httputil.ReverseProxy`）、`/api/forwards` 管理接口和兑换端点。
  - `internal/session` 新增监听 socket 扫描，Linux 读取 `/proc/net/tcp{,6}` 和 `/proc/<pid>/fd`，macOS 通过 `proc_info` 系统调用实现，不依赖 cgo。其他平台返回空结果。
  - `internal/hub` 在 `/ws/sessions` 中新增 `forwards` 事件。
  - `cmd/webmux` 负责构造转发服务并注入。
- 接口：`internal/contract`、`internal/wire` 和 `web/src/contracts/schemas.ts` 新增 `Forward`、`ForwardRequest`、`ForwardTicket` 和会话的 `listeningPorts` 字段，并重新录制 `internal/api/testdata/wire`。
- 前端：
  - 新增 `ForwardDialog`、`ForwardButton`、`/forward-auth` 认证页和终端端口提示；
  - `WorkspaceHeader` 增加入口；
  - `TerminalNotices` 增加端口提示条。
- 安全：转发页面与 WebMux 同源，页面里的脚本能读取 `localStorage` 中的主 Token。这是选择路径前缀方案时已经接受的风险，详见 design.md。
- 文档：新增用户故事分组 R（端口转发），并更新故事索引和覆盖表。
