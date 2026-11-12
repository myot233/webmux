# Proposal

## Why

[RFC #399](https://github.com/mzdluo123/webmux/issues/399) 规划了两条互补演进路线：**机会性 WebRTC DataChannel P2P 直连升级**（改善中继/隧道场景下的终端输入回显延迟与吞吐）与**稳定域名壳**（解决更换临时隧道域名导致 PWA 安装、Web Push 订阅与本机状态失效的问题）。

当前代码库在三处隐含了“单一路径 = 同源 HTTP/WebSocket + 单一主机独占浏览器 origin”的假设：

- 前端网络入口（`web/src/platform/api.ts`、`web/src/platform/ws.ts`）直接绑定同源 `/api` 与 `location.host`；
- 浏览器本机存储直接读写扁平的 `webmux.*` `localStorage` 键；
- 主机端实时流控制器（`internal/hub`）直接耦合 `*websocket.Conn`。

先在 **S0 阶段**完成这三项纯内部底座抽离，才能让后续 P2P 直连（P1–P3）与稳定域名多主机壳（H1–H3）并行演进，同时对现有同源单机部署保持零配置、零协议变更与零存储迁移影响。

## What Changes

- **抽离前端传输层与主机基准地址抽象**（`web/src/`）：
  - 将 `web/src/platform/api.ts` 中的 `fetch` 请求与 `XMLHttpRequest` 上传收敛到可注入的客户端传输接口与主机基准地址配置之下；
  - 将 `web/src/platform/ws.ts` 中的两处 `new WebSocket`（一次性流 `openSocket` 与持久重连流 `connect`）统一到同一流式传输抽象；
  - 默认实现保持当前同源 `/api` 与 `ws(s)://${location.host}/ws/...` 行为不变，且不在 `api.ts` 与 `ws.ts` 之间引入循环依赖。
- **为浏览器本机存储引入按主机（`hostId`）隔离的命名空间机制**（`web/src/`）：
  - 梳理并分级全部 `localStorage` / `sessionStorage` 键，将凭据、推送订阅标识、服务端设置缓存、草稿/暂存及工作区记忆归入主机级命名空间，将纯界面与输入习惯保留为设备级偏好；
  - 默认同源单主机模式继续读写原 `webmux.*` 键名（升级与回退零数据丢失），显式指定 `hostId` 时隔离在 `webmux.host.<hostId>.*` 下；
  - 消除绕过存储辅助函数直写 `"webmux.*"` 的硬编码调用点。
- **在主机端 `internal/hub` 抽离传输无关的 `MessageConn` 接口**（`internal/hub`、`internal/api`）：
  - 将 `Attach`、`List`、`Chat`、`Settings`、`Voice` 五类流式处理器从 `github.com/coder/websocket` 的 `*websocket.Conn` 解耦；
  - 明确定义读写帧类型、应用层关闭码（`4404`/`4408`/`4409`/`4410`/`1000`/`1011`/`1013`）、单帧读上限、写超时背压与半关闭读取语义；
  - 提供包装 `*websocket.Conn` 的适配器供 `internal/api` 使用，保持线上 WebSocket 帧格式与并发契约完全不变。

## Capabilities

### New Capabilities

- `client-transport`: 前端主机传输层与基准地址抽象，覆盖鉴权 HTTP 请求、带进度与取消的表单上传、一次性流式消息连接及带重连/背压/前台探活的会话流连接，默认同源 HTTP/WebSocket 实现对现有调用方透明。
- `host-scoped-storage`: 浏览器本机存储的主机命名空间机制，区分主机级状态、设备级界面偏好与标签页级客户端身份，在默认同源模式下保持现有 `webmux.*` 键名兼容，在多主机模式下提供严格按 `hostId` 的读写、跨标签页事件过滤与整机清理能力。
- `hub-message-conn`: 主机端 `internal/hub` 的传输无关消息连接抽象（`MessageConn`）与 WebSocket 适配器，在保持单写者、输出节流合并、回放压缩、慢客户端驱逐与退出顺序不变的前提下解耦具体传输实现。

### Modified Capabilities

- `terminal-draft-persistence`: 在保持默认同源模式使用 `webmux.draft.<sessionId>` / `webmux.scratch.<sessionId>` 的基础上，支持在激活 `hostId` 命名空间时按主机隔离草稿、暂存、会话快照核对（`keep`）与本机清除范围，防止同源多主机互相覆盖或误删草稿。

## Impact

- **后端代码**：
  - `internal/hub`（`hub.go`、`chat.go`、`settings.go`、`voice.go`，新增 `MessageConn` 定义与 WebSocket 适配器及对应测试）；
  - `internal/api/router.go`（WebSocket 升级后适配为 `hub.MessageConn` 传入 `hub`）；
  - 不新增外部依赖，不改变 `internal/archtest` 分层规则（R0–R11）及 `internal/api/testdata/wire` 快照。
- **前端代码**：
  - 新增底层无环存储与传输模块（位于 `api.ts` 与 `ws.ts` 之下以遵守 `pnpm --dir web check:arch` 的 `no-circular` 与分层约束）；
  - 重构基础传输 `web/src/platform/api.ts`、`web/src/platform/ws.ts` 及其使用方 `web/src/domains/terminal/draft-storage.ts`、`web/src/domains/workspace/forwards.svelte.ts`。
  - 收敛 `web/src/domains/settings/settings.svelte.ts`、`web/src/hooks/sessions/quick-start.ts`、`web/src/domains/code/diff-review.svelte.ts` 中硬编码 `"webmux.*"` 的 `localStorage` 访问；
  - 静态打包资源加载（如 `web/src/domains/code/code-pdf.ts` 的字体/WASM `fetch`）保持应用资源本地读取，不经过主机 API 传输层。
- **协议、配置与兼容性**：零 REST/WebSocket 线协议变更，零 TOML 配置或 CLI 参数变更，现有单机同源用户的登录态、草稿、暂存、推送订阅与界面偏好无需迁移即可原样保留。
