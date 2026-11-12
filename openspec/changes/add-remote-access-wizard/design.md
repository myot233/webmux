# Design

## Context

- **现有配置面。** 访问方式已经有两套独立的管理页：
  - `TunnelManager`（`/settings/tunnel`）：Quick 或 Named 隧道，配置、启停、状态与“复制带 Token 链接”。
  - `TailscaleSettingsPage`（`/settings/tailscale`）：Serve 或 Funnel，端口、provider、WSL 指引与“复制带 Token 链接”。
  - 设置页通过 `AccessSummaryCard` 各显示一张摘要卡片，数据由 `useMountedRead` 读取。
- **现有接口足够，向导不需要后端改动。**
  - 已有接口：`GET /api/tunnel`、`PUT /api/tunnel/settings`、`PUT /api/tunnel/enabled`、`GET /api/tailscale`、`PUT /api/tailscale/settings`。
  - 它们已经返回向导所需的全部信息：`canEnable`、`enableBlockedReason`、`storageAvailable`、`tokenConfigured`、`state`、`publicUrl`。
- **Token 的存放和约束。**
  - 登录 Token 在浏览器里保存在 `localStorage`，前端通过 `stored("token")` 读取；
  - `access-link` 规定 `#token=` 片段不会发给服务器，认证后立刻从地址栏移除；
  - `tailscale-web-settings` 规定带 Token 链接“Token 不得进入页面可见文本或新的持久化位置”；
  - 修改登录 Token 只能通过配置文件或配置命令，运行中不能更换（`runtime-tunnel-control`）。
- **没有二维码能力。** 前端依赖里没有二维码库。`access-link` 里“未启用隧道时 MUST NOT 打印二维码”是针对终端输出，与网页无关。
- **体积受约束。** 二进制嵌入前端产物（`docs/binary-size.md`），新增依赖必须按需加载并量化增量。

## Goals / Non-Goals

**Goals:**

- 让没有网络背景的用户从“打开设置页”到“手机打开 WebMux”只需顺着一条路径点完，不需要先读文档、不需要判断选哪一个。
- 完全复用现有接口与校验，向导只是编排。后端行为与现有规格保持一致。
- 二维码和带 Token 链接不扩大 Token 的暴露面。

**Non-Goals:**

- 不做 CLI 向导，不做托管中继，不新增服务端接口。
- 不在网页里生成或更换登录 Token。
- 不替代详细页：高级选项（Funnel、provider、端口细节）仍在原页面。
- 不做自动检测“用户是否有域名”“是否装了 Cloudflare 账号”之类的外部事实，只根据 WebMux 能读到的状态推荐。

## Decisions

### 1. 向导是编排层，复用现有组件

| 方案                     | 说明                                                           | 结论                                       |
| ------------------------ | -------------------------------------------------------------- | ------------------------------------------ |
| 新写一套表单             | 向导自带 Quick、Named、Tailscale 表单                          | 否：校验和文案会与详细页漂移               |
| 复用现有配置组件（选定） | 把 `TunnelConfiguration`、`TailscaleConfiguration` 嵌入第 2 步 | 是：校验、错误、风险文案只有一份           |
| 在向导里 iframe 详细页   | 不改组件，直接嵌入                                             | 否：无法拿到保存结果，也无法控制焦点和步骤 |

落地方式：

- 先检查这两个配置组件是否已经能在页面外使用；不能则拆出“表单 + 保存结果回调”的纯展示部分，详细页与向导共用。
- 向导的步骤状态放在一个 hook（`useRemoteAccessWizard`）里，只做三件事：聚合 `config`、`tunnel`、`tailscale` 三个状态源，计算推荐，驱动步骤。
- 步骤状态不持久化，刷新回到第 1 步，避免出现“保存了一半的向导”和服务端真实状态不一致。

### 2. 推荐规则只依赖可观察状态

推荐规则（见规格）按优先级：已有 `ready` 入口、Tailscale 运行且 HTTPS 就绪、已保存 Named 凭据、否则 Quick。

- 不推荐 Funnel：它把 shell 公开到互联网，应该是用户有意识的选择，放在详细页。
- Quick Tunnel 总是兜底，但带“临时使用”提示。理由：它零配置，能让用户马上看到效果；但地址会变、不支持 Web Push，必须在第 4 步再次说明。
- 推荐只是排序与徽标，不禁用其他选项。读取失败时降级为“未知”，不阻塞向导。

### 3. 就绪等待用轮询，且可取消

向导第 3 步用固定间隔读取 `GET /api/tunnel` 或 `GET /api/tailscale`，直到 `ready` 或 `failed`。

- 现有设置页已经依赖这些接口返回 `starting`，所以不需要新增推送通道。
- 通过页面卸载时的 `AbortController` 取消轮询。
- 间隔使用常量并在测试中注入计时器，不依赖真实 sleep。
- `/ws/sessions` 目前没有携带隧道状态事件，不借用它，避免为一个设置页向导扩大协议。

### 4. 二维码：纯前端、按需加载

- 在浏览器里生成，不调用外部服务，避免把 Token 发给第三方。
- 采用体积小、无依赖、MIT 许可的库（候选：`qrcode-generator`），通过动态 `import()` 加载，只在用户点击“显示带登录的链接”时下载，不进首屏包。
- 输出为 SVG（可缩放、深浅色主题友好、可设文字替代），用 `currentColor` 与背景色保证对比度；二维码本身始终使用高对比深色模块 + 浅色底，不跟随主题反色，避免某些扫码器无法识别。
- 纠错等级使用 M：链接长度约 100–140 字符，使二维码尺寸适中，手机扫描更稳定。

### 5. Token 展示边界

带 Token 的链接是“拿到就能控制 shell”的凭据，所以：

- 默认不渲染：只有点击“显示带登录的链接”后才从 `stored("token")` 读取并拼接。
- 生命周期受限：60 秒后自动清空，离开第 4 步或页面卸载也清空；同时释放二维码 SVG。
- 零新增存储：不写入任何 `localStorage`、`sessionStorage`、Cookie、URL 或遥测；不进入 `aria-label`、`title`、`alt`。
- 复制逻辑集中：把现有两处“复制带 Token 链接”抽成共享函数，向导和两个详细页共用，避免第三份实现。
- 手机优先复制与分享：在触屏设备上用户通常就是那台手机，二维码没有意义，默认不渲染。检测使用 `matchMedia("(pointer: coarse)")`，同时允许用户手动展开。

### 6. 风险提示保留

现有页面对公网入口的风险提示是刻意设计的（见 `TunnelManager` 与 Tailscale 页的警告）。向导不弱化它：开启公网入口前必须显式确认。Tailscale Serve 的“仅 tailnet”说明保留。

### 7. 路由与入口

- `web/src/hooks/shared/route.ts` 新增 `{ name: "settings-remote-access" }`，路径 `/settings/remote-access`，沿用现有路由写法。
- 设置页在 `TunnelSettings` 之前放入口卡片；两张 `AccessSummaryCard` 增加次要链接“用向导配置”。
- 不改变 `/settings/tunnel`、`/settings/tailscale` 的任何行为，已有书签与文档链接保持有效。

## Risks / Trade-offs

- **向导与详细页漂移。** 如果复制了表单，两边会不同步。→ 复用组件，不复制表单；E2E 同时覆盖两个入口。
- **二维码扩大 Token 暴露面。** 屏幕被旁人看到或截图。→ 默认隐藏、点击才显示、60 秒收起、手机不默认显示；风险文案提示“二维码等同于登录凭据”。
- **Quick Tunnel 被当作长期方案。** 用户配完发现收不到推送。→ 推荐处标注临时，第 4 步再次说明，并提供“改用固定入口”。
- **推荐规则可能与用户实际情况不符。** 例如装了 Tailscale 却想用公网。→ 推荐不是限制，三种方式都可选，且有直达详细页的链接。
- **轮询耗电与重复请求。** → 只在第 3 步轮询，页面卸载即停；失败不自动重试。
- **新增依赖的体积与审计。** → 动态加载、量化 `embed.zip` 增量，通过 `knip` 与依赖审计；如增量过大，改为仓库内置的最小实现，再评估。

## Open Questions

- 二维码库最终选哪个？候选 `qrcode-generator`，需要在实现时对比 `embed.zip` 增量与许可，再确认。
- 向导的入口是否要在“首次启动且从未配置过任何入口”时主动提示（例如会话列表页的一次性横幅）？这会扩大范围，默认不做，先观察使用情况。
