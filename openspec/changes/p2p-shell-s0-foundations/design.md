# Design

## Context

动机与范围见 [proposal.md](proposal.md)。结合对当前代码库的逐处核查，S0 需要在以下既有约束内完成底座抽离：

- **前端网络调用面**：整个 `web/src/` 面向主机的网络入口集中在四处：
  - `web/src/platform/api.ts` 的 `request()`（`fetch("/api" + path)`）与 `uploadForm()`（`new XMLHttpRequest()` 负责上传进度与 5 分钟超时）；
  - `web/src/platform/ws.ts` 的 `openSocket()`（语音识别单次流）与 `connect()`（`/ws/sessions`、`/ws/s/{id}`、`/ws/s/{id}/chat`、`/ws/settings` 四类持久重连流）；
  - 此外 `web/src/domains/workspace/forwards.svelte.ts` 的 `forwardLink()` 使用 `location.origin` 拼装分享地址；而 `web/src/domains/code/code-pdf.ts` 的 `fetch(url)` 读取 Vite 本地静态打包的字体/WASM 资产，属于应用静态资源而非主机 API。
- **前端架构守卫约束**：`pnpm --dir web check:arch` 强制执行分层方向、`knip` 可达性与 `dependency-cruiser` 的 `no-circular` 规则。当前 `ws.ts` 已导入 `api.ts` 的 `store`/`stored`，因此新增的传输与存储底座必须位于两者下层，严禁反向导入形成环路。
- **浏览器存储现状**：
  - 除通过 `stored()`/`store()` 访问外，存在三处直接硬编码 `"webmux.*"` 的 `localStorage` 调用：
    - `web/src/domains/settings/settings.svelte.ts` 清除 `webmux.settingsCache`；
    - `web/src/hooks/sessions/quick-start.ts` 读写删除 `webmux.quick-start-draft`；
    - `web/src/domains/code/diff-review.svelte.ts` 在 `storage` 事件中比对 `"webmux." + key`。
  - `web/src/domains/terminal/draft-storage.ts` 独立管理 `webmux.draft.*`、`webmux.scratch.*` 与 `webmux.draftPersistence`，其 `keep(sessionIds)` 会在收到会话快照时扫描前缀并删除不在快照中的全部草稿。
- **后端架构与并发约束**：
  - `internal/archtest` 规定 `internal/api`、`internal/hub`、`internal/wire` 属控制器层，服务层严禁导入 WebSocket 库（R1）；
  - `internal/hub` 当前在 `hub.go`、`chat.go`、`settings.go`、`voice.go` 中直接持有 `*websocket.Conn`，实际依赖的方法面仅有 6 个：`Read`、`Write`、`Close`、`CloseNow`、`SetReadLimit`、`CloseRead`。

## Goals / Non-Goals

**Goals:**

- 在不改变任何外部协议、存储兼容性与并发语义的前提下，让前端网络层、前端存储层与后端 `internal/hub` 具备可单测替换的清晰边界。
- 将重连退避、`1 MiB` 发送缓冲预算、`resize`/`resync` 控制帧合并、前台 `resume` 探活、`401` 事件派发与 `APIError` 归一化保留在共享上层逻辑中，避免未来 WebRTC DataChannel 传输重复实现。
- 确保多主机共享同一浏览器 origin 时，任一主机的会话列表快照核对（`keep`）或本机清理绝不波及其他主机的草稿、暂存与凭据。

**Non-Goals:**

- 不在 S0 实现 WebRTC `PeerConnection`、ICE 协商、SCTP DataChannel 分片重装或平滑热切换（属后续 P1–P3）。
- 不在 S0 实现多主机 URL 路由（如 `/h/<hostId>/...`）、跨域 CORS/Origin 扩展、Service Worker 缓存壳或中继 Web Push（属后续 H1–H3）。
- 不新增第三方 Go 或 npm 依赖，不改动任何 REST/WebSocket JSON 契约或 `internal/api/testdata/wire` 快照。

## Decisions

### 1. 前端模块无环拓扑：`storage.ts` → `transport.ts` → `{ api.ts, ws.ts }`

**方案**：在 `web/src/`（domain 层）新增 `web/src/platform/storage.ts` 与 `web/src/platform/transport.ts`：

- `storage.ts` 不依赖任何项目内网络或 UI 模块，仅依赖 `./schemas`（`TextSettingSchema`），负责当前活动 `hostId` 上下文、主机级与设备级读写辅助函数、完整存储键解析及按主机清理。
- `transport.ts` 仅依赖 `./storage` 与 `./base64url`，负责主机基准地址（默认同源）与可注入 `ClientTransport` 注册表，并提供默认的浏览器 HTTP + `XMLHttpRequest` + `WebSocket` 实现。
- `api.ts` 从 `./storage` 重新导出 `stored`、`store`（以及新增的显式作用域辅助函数），并通过 `./transport` 发起请求与上传；`ws.ts` 改为直接从 `./storage` 与 `./transport` 导入，彻底解除对 `./api` 的耦合。

**备选方案与取舍**：

- _把传输抽象直接放进 `api.ts` 或 `ws.ts`_：会让两者互相引用或迫使全仓数十处组件改动 `stored` 导入路径，违反最小侵入原则并触碰 `no-circular` 守卫。独立下沉为两个零环底层模块既保持现有调用方 `import { stored, store } from "./api"` 完全兼容，又通过 `knip` 入口可达检查。

### 2. 在底层流与请求原语处切分 `ClientTransport`，而非替换 `api()` / `connect()`

**方案**：在 `web/src/platform/transport.ts` 定义贴合承载原语的传输接口：

```ts
export interface TransportSocket {
  readonly readyState: number;
  readonly bufferedAmount: number;
  send(data: string | Uint8Array): void;
  close(code?: number, reason?: string): void;
  onopen: (() => void) | null;
  onmessage: ((event: { data: string | ArrayBuffer }) => void) | null;
  onclose: ((event: { code: number; reason: string }) => void) | null;
  onerror: (() => void) | null;
}

export interface UploadRequest {
  path: string;
  form: FormData;
  token: string;
  timeoutMs: number;
  signal: AbortSignal;
  onProgress: (percent: number) => void;
}

export interface UploadResult {
  status: number;
  statusText: string;
  body: unknown;
}

export interface ClientTransport {
  readonly origin: string;
  fetch(path: string, init: RequestInit): Promise<Response>;
  upload(request: UploadRequest): Promise<UploadResult>;
  openStream(
    path: string,
    options: { token: string; compress: boolean },
  ): TransportSocket;
}
```

- 默认传输根据可选主机基准 URL（默认取 `location.origin`）将 `fetch` 与 `upload` 映射到 `<origin>/api<path>`，将 `openStream` 映射到 `ws(s)://<host><path>[?compress=1]` 与 `protocols(token)`。设置 `binaryType = "arraybuffer"` 后的原生 `WebSocket` 天然满足 `TransportSocket`，热路径零额外包装开销。
- `api.ts` 继续统一负责请求体/响应体的 Zod 校验、`401` 触发 `webmux:auth`、`APIError` 构造与 `apiBlob` 流式读取；`ws.ts` 继续统一负责指数退避重连、`maxBufferedAmount`（`1 MiB`）、`resize`/`resync` 合并、`visibilitychange`/`pageshow` 的 `resume` 探活及 `4404`/`4409`/`4410` 状态转换。
- `forwards.svelte.ts` 的 `forwardLink()` 改为使用当前传输的 `origin` 构造分享 URL，而 `code-pdf.ts` 继续使用全局 `fetch` 读取前端自身静态打包资源。

**备选方案与取舍**：

- _在高层替换整个 `api()` 与 `connect()`_：会导致后续 DataChannel 传输必须重写一套重连、流控合并、前台探活与 Zod 错误处理逻辑，极易产生双轨行为漂移。
- 在 `TransportSocket` / `fetch` / `upload` 原语层切分，既能直接适配未来的 `RTCDataChannel`（具备同名 `readyState`、`bufferedAmount` 与消息回调），又能让所有协议状态机只维护一份实现。

### 3. 本机存储三级作用域与默认同源原键名保留策略

**方案**：按数据归属将全部浏览器存储键划分为三级（详见 [specs/host-scoped-storage/spec.md](specs/host-scoped-storage/spec.md)）：

- **主机级（Host-scoped）**：
  - 鉴权与设备注册：`token`、`push.subscriptionId`、`settingsCache`、`sessionPins`、`dismissedUpdate`；
  - 工作区与启动记忆：`homeRepository`、`homeWorktree`、`workbenchTree`、`quickStart`、`quick-start-draft`、`lastLaunch`、`customCommand`、`hybridStartSide`、`hybridStartSide:<configId>`、`recentDirs`；
  - 会话视图与草稿：`layout.<sessionId>`、`layout.<sessionId>.active`、`diffReview`、`draft.<sessionId>`、`scratch.<sessionId>`、`draftPersistence`。
- **设备级（Device-scoped）**：`shellMode`、`sidebarCollapsed`、`sidebarWidth`、`homeSessionView`、`quickStartSide`、`codePanelShown`、`codePanelSize`、`sizeLocked`、`wsCompression`、`toastNotifications`、`fontSize`、`codeFontSize`、`keyboardSizeHold`、`touchGamepad`、`voiceInput`。
- **标签页级（Tab-scoped，`sessionStorage`）**：`webmux.clientId`、`webmux.clientLive`（每标签页随机实例标识，保持不变）。

**键名映射与安全规则**：

- 默认同源模式（`hostId === null`）：主机级与设备级键统一映射为 `webmux.<key>`，对现有线上数据与全部现有单测/E2E 测试 100% 原地兼容。
- 显式多主机模式（`hostId` 非空）：校验 `hostId` 匹配 `^[A-Za-z0-9_-]{1,64}$`（禁止空白与点号，杜绝伪造其他键路径）；设备级键仍读写 `webmux.<key>`，主机级键映射为 `webmux.host.<hostId>.<key>`（保留前缀 `webmux.host.` 与现有任何顶层键名均不冲突）。
- 提供 `hostStorageKey(key)` 返回当前主机命名空间下的完整键名，供 `diff-review.svelte.ts` 的 `window` `storage` 事件精确匹配；将 `settings.svelte.ts` 与 `quick-start.ts` 的裸 `localStorage` 调用统一收敛到 `store()`/`stored()`。
- `createDraftStorage(storage, now, options?: { hostId?: string | null })` 根据 `hostId` 计算 `draftPrefix`、`scratchPrefix` 与 `settingKey`，`browserDraftStorage()` 按当前活动 `hostId` 缓存实例，确保会话快照 `keep()` 与设置页“清除本机草稿”严格限定在当前主机前缀内。

**备选方案与取舍**：

- _强制将默认模式也迁移到 `webmux.host.default.<key>`_：需要一次性迁移脚本，且一旦用户回退二进制版本就会丢失令牌与未发送草稿，违背 S0“对现有用户零影响”的硬约束。保留 `null` 默认模式直写 `webmux.<key>` 无需迁移且回退安全。

### 4. 主机端在 `internal/hub` 定义 `MessageConn` 与 WebSocket 适配器

**方案**：在 `internal/hub` 内定义传输无关连接接口与帧/状态类型，而不新建额外的顶层包：

```go
type MessageType int

const (
	MessageText MessageType = iota + 1
	MessageBinary
)

type StatusCode uint16

// Mirror RFC 6455 and WebMux 44xx application close codes without exposing coder/websocket in hub signatures.
const (
	StatusNormalClosure StatusCode = 1000
	StatusInternalError StatusCode = 1011
	StatusTryAgainLater StatusCode = 1013
)

type MessageConn interface {
	Read(ctx context.Context) (MessageType, []byte, error)
	Write(ctx context.Context, typ MessageType, p []byte) error
	Close(code StatusCode, reason string) error
	CloseNow() error
	SetReadLimit(n int64)
	CloseRead(ctx context.Context) context.Context
}
```

- `Hub.Attach`、`Hub.List`、`Hub.Chat`、`Hub.Settings`、`Hub.Voice` 签名中的 `*websocket.Conn` 全部替换为 `MessageConn`；`hub.go`、`chat.go`、`settings.go`、`voice.go` 不再导入 `github.com/coder/websocket`。
- 在同包 `internal/hub/wsconn.go` 提供轻量适配器 `NewWebSocketConn(c *websocket.Conn) MessageConn`，将 `MessageType` 与 `StatusCode` 一一映射到 `coder/websocket`；`internal/api/router.go` 在 `accept()` 成功后调用 `hub.NewWebSocketConn(c)` 传入 `hub`。
- 该接口提前对齐后续 DataChannel 承载需求：有序可靠通道直接提供 `Read`/`Write` 与 `SetReadLimit`，通过应用层控制帧传递 `Close(code, reason)`（保留 `4404`/`4408`/`4409`/`4410`），通过底层流重置实现 `CloseNow()`，并在收到非预期入站数据或 EOF 时取消上下文以实现 `CloseRead(ctx)`。

**备选方案与取舍**：

- _新建 `internal/msgconn` 共享包_：需要在 `internal/archtest/layers_test.go` 新增包分层声明，且目前唯一消费方就是 `internal/hub` 与 `internal/api`（`api` 本就依赖 `hub`）。
- 直接放在控制器层的 `internal/hub` 内聚度最高，完全符合 `internal/archtest` R0–R11，且让 `hub` 单测可直接用内存假实现（Fake `MessageConn`）覆盖写超时、慢客户端关闭码与半关闭逻辑，无需每次启动真实 HTTP/WebSocket 服务器。

## Risks / Trade-offs

- **[Risk] 新增业务存储键时误归类作用域** → 在 `web/src/platform/storage.ts` 采用显式的设备级键白名单集合（`Set`），默认归入更安全的主机级隔离，并在单测中固定全量键名分类断言。
- **[Risk] 切换活动 `hostId` 后模块级内存单例残留旧主机状态** → `browserDraftStorage()` 按当前活动 `hostId` 索引实例，同时在 `storage.ts` 暴露主机上下文变更钩子，供后续多主机壳切换路由时重置内存缓存。
- **[Risk] WebSocket 关闭错误判定耦合具体库错误类型** → `hub` 现有逻辑仅通过 `ctx.Err()`、返回值与显式 `Close(code, reason)` 控制生命周期，不依赖 `websocket.CloseStatus(err)` 类型断言；适配器原样透传错误即可保持兼容。

## Migration Plan

1. 先落地主机端 `internal/hub` 的 `MessageConn` 接口、`NewWebSocketConn` 适配器与 `internal/api/router.go` 调用更新，补充内存 `MessageConn` 单测并确认现有 `internal/hub` 与 `internal/api` 全部测试（含 `BenchmarkAttachOutput`）通过。
2. 落地前端 `web/src/platform/storage.ts` 与 `web/src/domains/terminal/draft-storage.ts` 主机命名空间支持，收敛三处裸 `localStorage` 访问，补充默认同源兼容与多 `hostId` 隔离单测。
3. 落地前端 `web/src/platform/transport.ts`，重构 `web/src/platform/api.ts`、`web/src/platform/ws.ts`、`web/src/domains/workspace/forwards.svelte.ts` 接入传输层抽象，验证 `pnpm --dir web check:arch` 无循环依赖与无未引用导出。
4. 运行全量质量门禁（`make check` 与 `make test`）。因无持久化格式迁移与线协议变更，回退可直接切换回任一旧版本二进制而无副作用。
