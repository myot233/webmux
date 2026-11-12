# Tasks

## 1. 主机端 `MessageConn` 抽象与 WebSocket 适配

- [x] 1.1 在 `internal/hub` 定义 `MessageConn`、`MessageType` 与 `StatusCode`，并实现 `NewWebSocketConn` 适配器（`wsconn.go`）；运行 `go build ./internal/hub` 验证核心流文件不再直接引用 `github.com/coder/websocket`
- [x] 1.2 将 `Hub.Attach`、`Hub.List`、`Hub.Chat`、`Hub.Settings`、`Hub.Voice` 切换为接收 `MessageConn`，更新 `internal/api/router.go` 注入 `hub.NewWebSocketConn`；运行 `go test -race ./internal/api ./internal/archtest` 验证路由与分层守卫通过
- [x] 1.3 在 `internal/hub` 新增内存 Fake `MessageConn` 单测，覆盖 `SetReadLimit`、`CloseRead` 半关闭取消、`10 s` 写超时驱逐（`4408`）及 `4404`/`4409`/`4410`/`1000`/`1011`/`1013` 关闭码；运行 `go test -race ./internal/hub` 与 `BenchmarkAttachOutput` 验证通过

## 2. 前端本机存储主机命名空间（`hostId`）隔离

- [x] 2.1 新建 `web/src/platform/storage.ts` 实现 `hostId` 上下文与字符集校验、主机级/设备级读写（默认 `webmux.<key>`，多主机 `webmux.host.<hostId>.<key>`）、`hostStorageKey()` 及 `clearHostStorage(hostId)`，并在 `web/src/platform/api.ts` 重新导出兼容入口
- [x] 2.2 新增 `web/test/storage.test.ts`，验证默认同源零迁移兼容、多 `hostId` 隔离、非法 `hostId` 拒绝、隐私模式降级与按主机清理，运行 `pnpm --dir web exec vitest run --config vitest.config.ts test/storage.test.ts` 通过
- [x] 2.3 收敛 `web/src/domains/settings/settings.svelte.ts`、`web/src/hooks/sessions/quick-start.ts` 与 `web/src/domains/code/diff-review.svelte.ts` 中硬编码 `"webmux.*"` 的 `localStorage` 访问及跨标签页 `storage` 事件比对，并补充多 `hostId` 隔离单测验证通过
- [x] 2.4 扩展 `web/src/domains/terminal/draft-storage.ts` 支持按 `hostId` 隔离草稿、暂存、`draftPersistence`、快照核对 `keep(sessionIds)` 与 `clear()`；更新并运行 `pnpm --dir web exec vitest run --config vitest.config.ts test/draft-storage.test.ts` 验证通过

## 3. 前端传输层与主机基准地址抽象

- [x] 3.1 新建 `web/src/platform/transport.ts`，定义 `ClientTransport`、`TransportSocket`、`UploadRequest`/`UploadResult` 与主机基准地址配置，实现默认同源 `fetch` + `XMLHttpRequest` + `WebSocket` 传输；运行 `pnpm --dir web check:arch` 验证无环且无死代码
- [x] 3.2 重构 `web/src/platform/api.ts`、`web/src/platform/ws.ts` 与 `web/src/domains/workspace/forwards.svelte.ts` 接入 `web/src/platform/transport.ts`。
  - `web/src/domains/code/code-pdf.ts` 的本地静态资源读取保持不变。
  - 维持 `401` 派发 `webmux:auth`、`APIError`、上传进度/取消、发送预算、控制帧合并、`resume` 探活与 `4404`/`4409`/`4410` 状态机不变。
- [x] 3.3 新增自定义传输与跨源主机基准地址的 Vitest 单测，并运行现有 `api`/`ws` 相关测试套件验证全部通过

## 4. 全链路集成校验与质量门禁

- [x] 4.1 运行前端构建与静态门禁 `make check`（含 `pnpm --dir web check:arch`、Oxlint、Oxfmt、`go vet`、`golangci-lint`、`internal/docguard`、`internal/persistguard`、`internal/archtest`），确认零警告零违规
- [x] 4.2 运行全量测试门禁 `make test`（Go `-race` 全包测试、Vitest 单元/浏览器测试、Storybook、Playwright E2E 与性能测试），验证默认同源单机部署的所有既有端到端行为无回归
