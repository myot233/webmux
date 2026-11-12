# Tasks

## 1. 配置、持久化存储与契约基础

- [x] 1.1 在 `internal/config` 新增 `[p2p]` 配置段（默认启用、默认开启端口映射、预置公共 STUN、来源与校验），运行 `go test -race ./internal/config` 验证通过
- [x] 1.2 新增 `internal/p2psettings` 并登记 `statefile.P2PSettings`、分层清单、持久化清单与 `docs/persistence.md`，运行 `make persistence-check` 与 `go test -race ./internal/p2psettings ./internal/statefile` 验证通过
- [x] 1.3 在 `internal/contract`、`internal/wire` 与 `web/src/contracts/schemas.ts` 定义 P2P 设置、诊断、信令与测速契约，运行 `pnpm --dir web exec vitest run --config vitest.config.ts test/wire-contract.test.ts` 验证通过

## 2. 后端 WebRTC P2P 引擎、端口映射与 API 接入

- [x] 2.1 引入 `pion/webrtc/v4` 并实现 `internal/p2p` 流通道封装（32 KiB 分片重组、状态码关闭帧、背压、`SetReadLimit` 与 `CloseRead`），运行 `go test -race ./internal/p2p` 验证大快照、业务关闭码与慢客户端背压通过
- [x] 2.2 实现 `internal/p2p` 双栈 ICE、单端口 UDP 复用、默认开启的本地网关端口映射（NAT-PMP/PCP + UPnP IGD）、控制通道握手/ping、`/api/*` RPC 承载及 `nop2p` 裁剪构建，运行 `go test -race ./internal/p2p` 与 `go test -tags nop2p ./internal/p2p ./cmd/webmux` 验证通过
- [x] 2.3 在 `internal/api` 与 `cmd/webmux` 接入 `/api/p2p*` 路由、流与 RPC 桥接、配置诊断视图及会话内进程 403 拦截，更新契约快照并运行 `go test -race ./internal/api ./cmd/webmux ./internal/archtest` 验证通过

## 3. 前端路径管理器、无感切换与全通道升级

- [x] 3.1 实现 `web/src/p2p-*.ts` 路径判定与 DataChannel 传输（本地/低 RTT 跳过、信令、版本握手与 ping、直连候选校验拒绝 `relay`、流分片与 `/api/*` + 上传承载），运行 `pnpm --dir web exec vitest run --config vitest.config.ts test/p2p.test.ts` 验证通过
- [x] 3.2 扩展 `web/src/platform/ws.ts`、`terminal-socket.svelte.ts` 与路径管理器，实现滞回切换、终端流先建后拆 `append` 无缺口升级与断连/切网自动回落重探，运行 `pnpm --dir web exec vitest run --config vitest.config.ts test/ws.test.ts test/transport.test.ts test/p2p-handoff.test.ts` 验证通过

## 4. 设置页网络面板、状态栏指标与用户故事

- [x] 4.1 实现 `P2PSettings.svelte` 并接入 `NetworkSettings.svelte`、`SettingsNavigation.svelte` 与 `SidebarFooter.svelte`（含开关、STUN、路径/候选/IP 族/RTT/耗时/切换次数指标与一键 A/B 测速），运行 `pnpm --dir web check:arch` 与浏览器组件测试验证通过
- [x] 4.2 更新受影响的用户故事与索引（`docs/user-stories/`、`docs/user-stories.md`、`docs/user-stories/coverage.md` 及验证日志），运行 `make docs-check` 验证通过

## 5. 端到端集成验证与交付检查

- [x] 5.1 新增真实 WebRTC P2P 升级、`/api/*` 承载、活跃输出无损切换与断连回落的集成/Playwright E2E 测试，记录 `CGO_ENABLED=0` 与 `-tags nop2p` 二进制体积差异，运行 `make check` 与 `make test` 验证全量门禁通过
