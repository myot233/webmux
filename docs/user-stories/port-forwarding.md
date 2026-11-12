# 端口转发预览

本页是 R 组：在服务器上的会话里开发 Web 应用时，把开发服务器经 WebMux 现有地址转发到浏览器预览。使用说明见 [端口转发预览](../usage/port-forwarding.md)，协议见 [端口转发 API](../protocol/http-forward.md)。

## R. 端口转发预览

- **US-R01 转发网关与凭据**：作为在服务器上开发 Web 应用的使用者，我希望经 WebMux 已有的地址打开服务器本机端口上的开发服务器，以便不新开端口、不另建隧道就能在自己的浏览器里预览。
  验收要点：
  - 网关路由为 `/forward/{port}/`，只代理已添加的转发，上游固定为服务器本机回环地址；未添加或非法端口（含前导零）返回 404，`/forward/{port}` 308 到带斜杠的地址并保留查询参数；
  - 不新开任何监听端口，本机、局域网、Tailscale 与 Cloudflare Tunnel 入口都可用；
  - 网关只认 `webmux_forward` Cookie：值为服务端内存中的 256 位随机凭据，独立于主 token，`HttpOnly`、`SameSite=Strict`、`Path=/forward/`、12 小时过期，经 HTTPS 入口访问时带 `Secure`，服务重启后全部失效；
  - 该 Cookie 不能认证 `/api/` 与 `/ws/`，只带它的请求返回 401；主 token 为空时网关与 API 一样不要求认证；
  - 没有有效凭据时，浏览器顶层导航 303 到 `/forward-auth`，页面用主 token 换取 60 秒一次性兑换码，以表单 POST 兑换 Cookie 后回到原地址；兑换码不进入 URL，跨站提交 403，站外 `next` 400，重复使用 401；其他请求直接 401；
  - 由其他网站发起（或浏览器未报告来源，如经纯 HTTP 访问局域网地址）的认证页显示目标地址，用户点击“打开”后才兑换；认证页不能被嵌入框架；未认证请求不能探测哪些端口已转发；
  - 请求与响应流式转发，SSE 逐条到达，WebSocket 透明转发；跨站 WebSocket 升级 403；上游不可连接时返回含端口号的 502 纯文本；
  - 上游收不到 `webmux_forward`，响应中的 `Service-Worker-Allowed` 被删除；网关执行 Host 检查，不记录请求体、响应体或 Cookie；
  - 移除转发后新请求 404，该端口进行中的连接（含 WebSocket）被关闭；转发只在内存中，与会话无关，服务重启后清空。
- **US-R02 兼容改写与路径前缀**：作为使用 Vite 等开发服务器的使用者，我希望转发默认就能通过开发服务器的主机与来源检查，以便设置 base 后页面和热更新开箱可用。
  验收要点：
  - 每个转发有四个兼容选项，新建时全部开启：
    - 改写 Host 为 `localhost:{port}`；
    - 把与网关同主机的 Origin、Referer 改为 `http://localhost:{port}`；
    - 添加 `X-Forwarded-For/Host/Proto/Prefix`；
    - 把指向 `localhost:{port}` 或 `127.0.0.1:{port}` 的 `Location`、`Access-Control-Allow-Origin` 改回网关，并去掉 `Set-Cookie` 的 `Domain`、给 `Path` 加转发前缀；
  - 选项关闭时对应头原样转发；浏览器传入的 `X-Forwarded-*` 不会到达上游；
  - 路径前缀模式默认“去除前缀”（只用相对路径的服务无需改配置），可改为“保留前缀”（适合 `vite --base /forward/5173/`、Next.js `basePath` 等使用绝对路径的应用）；
  - 不改写 HTML、CSS 或 JS 响应体；修改选项只影响之后的新请求。
- **US-R03 首页转发管理**：作为使用者，我希望在首页查看和管理所有转发，以便手动添加端口、打开或复制链接、调整选项和移除转发。
  验收要点：
  - 侧栏底部在主机名与设置之间显示“端口转发”图标按钮，带提示；有转发时显示数量，可访问名称为“端口转发（N 个）”；
  - 对话框（窄屏为底部抽屉）包含添加表单、转发列表与“检测到的端口”；非法端口、WebMux 自身端口和重复端口在表单内联显示错误且不关闭对话框；
  - 每个转发显示端口、名称或来源会话、上游状态（可访问 / 无法连接，5 秒内更新）和路径，提供打开、复制完整链接、设置（名称、路径前缀、四个兼容选项）与移除（无需确认）；
  - “检测到的端口”列出各会话正在监听但未转发的端口（受 `ignoreHighPorts` 过滤控制，默认忽略 30000 及以上端口），可一键转发并记录来源会话；不支持检测的平台显示“当前平台不支持自动检测”；
  - 转发列表经 `/ws/sessions` 的快照与 `forwards` 事件在各设备间实时同步。
- **US-R04 会话端口提示**：作为在终端或聊天视图中启动开发服务器的使用者，我希望 WebMux 发现新监听的端口并询问是否转发，以便不离开当前会话就能打开预览。
  验收要点：
  - 会话信息的 `listeningPorts` 列出会话进程树（含终端主进程与混合会话聊天端 ACP 进程）中监听在回环或通配地址上的 TCP 端口及进程名：
    - 按端口升序、IPv4/IPv6 去重、排除 WebMux 自身端口；
    - 开始或停止监听后 3 秒内经 `updated` 事件更新；
    - 支持 Linux 与 macOS（不依赖 cgo 或外部命令），其他平台恒为空；
  - 会话页（终端视图与聊天视图）为每个未转发、未忽略且未被高端口过滤排除的监听端口显示“检测到端口 {port}（{process}）正在监听”，提供“转发并打开”“转发”“忽略”；最多显示两条，其余合并为“另有 N 个端口”，可打开转发对话框；
  - 默认忽略 30000 及以上端口的会话页提示与转发对话框“检测到的端口”列表（`ignoreHighPorts` 为 `null` 或 `true`），仍可手动输入端口添加转发；设置页“网络”分区提供“忽略 30000 及以上端口的转发提示”开关，保存在服务端并在所有设备间同步；
  - “转发并打开”在点击时打开新标签页，不被浏览器拦截；转发失败时提示保留并显示服务端错误；
  - 转发后提示变为“端口 {port} 已转发到 /forward/{port}/”，提供打开、复制链接、知道了；
  - 端口在任何设备上被转发或停止监听后提示消失；“忽略”只在当前标签页内存中生效，离开会话页再回来仍然有效，端口停止后再次监听会重新提示；
  - 混合会话在聊天视图时通过 `/ws/sessions` 同步监听端口与转发列表，与终端视图同样显示端口提示；
  - 窄屏上按钮换到文字下方，触控高度至少 44px。

## 取舍

- 选择路径前缀而不是子域名：路径前缀在 Tailscale MagicDNS 与 Cloudflare quick tunnel 下都能用，代价是使用绝对路径的应用必须设置 base，且转发页面与 WebMux 同源，页面脚本能读取浏览器中保存的主 token。只转发自己信任的开发服务器。
- 用专用 Cookie 而不是主 token：新标签页里的页面、脚本和 HMR 请求无法携带 Bearer 头；Cookie 按主机共享给同主机其他端口，所以它只能访问网关、不能访问 shell。
- 转发与忽略状态都不持久化：服务重启后开发服务器通常也已退出，重新添加的成本很低。

## 代码依据

- [转发登记、探测与凭据](../../internal/forward/forward.go)、[凭据与兑换码](../../internal/forward/credentials.go)、[服务用例](../../internal/forward/forward_test.go)
- [网关与转发接口](../../internal/api/forward.go)、[网关用例](../../internal/api/forward_test.go)、[路由注册](../../internal/api/router.go)
- [会话列表事件](../../internal/hub/hub.go)、[事件用例](../../internal/hub/list_test.go)、[线上类型](../../internal/contract/forward.go)
- [监听端口检测](../../internal/session/listening_ports.go)、[Linux 扫描](../../internal/procpeek/listen_linux.go)、[macOS 扫描](../../internal/procpeek/listen_darwin.go)、[真实监听用例](../../internal/session/listening_ports_unix_test.go)
- [前端契约](../../web/src/contracts/schemas.ts)、[转发状态与忽略集合](../../web/src/domains/workspace/forwards.svelte.ts)、[转发状态用例](../../web/test/browser/forwards.test.ts)
- [顶栏按钮](../../web/src/components/workspace/ForwardButton.svelte)、[转发对话框](../../web/src/components/workspace/ForwardDialog.svelte)、[对话框用例](../../web/test/browser/ForwardDialog.test.ts)
- [终端端口提示](../../web/src/components/terminal/TerminalPortPrompts.svelte)、[提示用例](../../web/test/browser/TerminalNotices.test.ts)、[混合会话用例](../../web/test/browser/TerminalHybrid.test.ts)
- [转发认证页](../../web/src/pages/ForwardAuth.svelte)、[认证页用例](../../web/test/browser/ForwardAuth.test.ts)、[端到端用例](../../web/test/e2e/port-forwarding.spec.ts)
