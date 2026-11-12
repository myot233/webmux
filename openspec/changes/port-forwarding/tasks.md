# Tasks

## 1. 转发服务

- [x] 1.1 新建服务包 `internal/forward`，并在 `internal/archtest/layers_test.go` 中登记为服务层。用 `go test -race ./internal/forward ./internal/archtest` 验证。
  - 登记表：添加、修改、移除、列表、查询，以及订阅变化通道。
  - 端口校验：范围 1–65535，排除 WebMux 自身端口。
  - 哨兵错误：`ErrInvalid`、`ErrConflict`、`ErrNotFound`。
  - 每个转发持有一个 `context`，移除转发时取消。
- [x] 1.2 实现转发凭据和兑换码。测试覆盖以下行为：
  - 凭据为 32 字节随机数，服务端只保存 SHA-256，12 小时过期；
  - 兑换码 60 秒内有效，只能使用一次；
  - 并发兑换同一个兑换码时，只有一次成功；
  - 校验时用常数时间比较；
  - 过期条目会被清理。
- [x] 1.3 实现上游状态探测：每 3 秒拨号一次（超时 300 ms，IPv4 失败时再试 `[::1]`），状态有变化才推送，并且不持有锁。用本地临时监听器测试 `listening` 和 `unreachable` 之间的切换，测试中注入时钟，不依赖 sleep。

## 2. 网关与接口

- [x] 2.1 新增 contract 类型 `Forward`、`ForwardOptions`、`ForwardRequest`、`ForwardTicket`、`ListeningPort`，在 wire 中做转换，并同步更新 `web/src/contracts/schemas.ts`。用 `go test ./internal/api -run TestWireSnapshots -update` 重新录制快照。
- [x] 2.2 实现 `/api/forwards` 系列接口和 `POST /api/forwards/tickets`，在 `internal/api/errors.go` 中映射错误。用 `go test -race ./internal/api` 验证，覆盖：
  - 400（非法端口或自身端口）、409（重复添加）、404（转发不存在）；
  - 只带转发 Cookie 时访问 `/api/` 和 `/ws/` 返回 401。
- [x] 2.3 实现 `/forward/_auth` 兑换端点。测试覆盖：
  - 同源 Origin 才能兑换；
  - `next` 白名单（`//`、协议和反斜杠都返回 400）；
  - 兑换码复用返回 401；
  - Cookie 属性正确，HTTPS 和公网来源时带 `Secure`；
  - 303 跳转。
- [x] 2.4 实现 `/forward/{port}/` 网关（`httputil.ReverseProxy`，`FlushInterval: -1`），只注册在 TCP 监听器上。用 `httptest` 起一个上游测试：
  - 404、308、未认证时导航请求返回 303、子资源请求返回 401、跨站 WebSocket 返回 403、502；
  - `keep` 和 `strip` 两种前缀模式；
  - 上游收不到转发 Cookie；
  - SSE 能逐条收到；
  - WebSocket 能双向转发，移除转发后连接关闭；
  - 删除 `Service-Worker-Allowed` 头。
- [x] 2.5 实现兼容改写，并为每个选项开和关两种状态各写一个表驱动测试：Host、Origin/Referer、`X-Forwarded-*`、`Location`、`Access-Control-Allow-Origin`、`Set-Cookie` 的 Path 和 Domain。
- [x] 2.6 补充安全测试（`internal/api/forward_test.go`）：转发 Cookie 不能认证 `/api/` 和 `/ws/`；网关遵守 Host 检查。在 `cmd/webmux` 中构造 `forward.Service` 并注入，然后用 `go build ./cmd/webmux` 验证。

## 3. 端口检测

- [x] 3.1 Linux：解析 `/proc/net/tcp{,6}`，并把会话进程树中的 fd 映射到 inode。用固定内容的解析测试，以及在会话中启动真实监听进程的 `go test -race ./internal/session` 验证，覆盖：
  - 回环地址和通配地址；
  - 只监听外部地址时不统计；
  - IPv4 和 IPv6 去重；
  - 排除 WebMux 自身端口；
  - 停止监听后 3 秒内移除。
- [ ] 3.2 macOS：用 `SYS_PROC_INFO` 实现扫描，不用 cgo（`internal/session/listen_darwin.go`、`listen_xnu.go`）。
  - 已验证：固定字节的结构体解析测试 `TestParseSocketFDInfo`（`listen_xnu_test.go`，在 Linux 上也能运行）；`GOOS=darwin go vet ./...`（CI 已执行）。
  - 其他平台：`listen_other.go` 返回空结果。服务端只支持 Linux 与 macOS，`internal/session` 在其他 `GOOS` 下本就不能编译，因此不以 `GOOS=windows go vet` 作为验证。
  - **待验收**：在 macOS 真机上运行真实监听测试 `TestSessionListeningPorts`（`listen_unix_test.go`）；仓库 CI 与当前环境均没有 macOS。
- [x] 3.3 把 `listeningPorts` 接入 `Info`、`sameDisplay` 和 manager 每秒的快照周期（每 2 秒扫描一次，集合不变时跳过）。用 `cmd/internal/assemble` 填写 `ExcludePorts`。重新录制 wire 快照，并验证端口变化会触发 `updated` 事件。

## 4. 实时同步

- [x] 4.1 `internal/hub` 在 `/ws/sessions` 快照中加入 `forwards` 字段，转发变化时推送 `forwards` 事件；变化信号合并发送，订阅不会积压，因此不会因转发事件触发慢客户端断开。用 `go test -race ./internal/hub` 覆盖快照、事件和取消订阅。
- [x] 4.2 前端在 `forwards.svelte.ts` 中基于 `ws.ts` 的 `connect` 接收 `forwards` 快照和事件（`ws.ts` 会被无法加载 rune 模块的 Node 单测引用）；在 `api.ts` 中新增各写操作。补充 Vitest 测试，覆盖 Zod 校验失败时不更新状态，以及忽略集合的清理规则。

## 5. 前端界面

- [x] 5.1 新增 `/forward-auth` 路由和页面：
  - 已登录时自动兑换并提交隐藏表单；
  - 未登录时复用现有的登录表单；
  - 失败时显示错误和“返回 WebMux”按钮。

  用 Vitest 组件测试和 `test/route.test.ts` 验证。

- [x] 5.2 新增 `ForwardButton` 和 `ForwardDialog`：
  - 按钮接入 `WorkspaceHeader`，放在新建会话和设置之间；
  - 实现添加表单、转发列表、设置面板、检测到的端口和空状态，布局按 Storybook 原型 `Workspace/Port forwarding`；
  - 修改组件时同步更新原型 story，删除原型专用的数据桩。

  组件测试覆盖：内联错误、添加、移除、修改选项、复制链接和计数的可访问名称。

- [x] 5.3 在 `TerminalNotices` 中加入端口提示：
  - 实现“转发并打开”“转发”“忽略”三个操作，以及转发后的提示；
  - 最多显示 2 条，其余合并为“另有 N 个端口”，点击后打开对话框；
  - “转发并打开”在点击时同步打开空白标签页，以免被浏览器拦截。

  合并前先阅读 `float-terminal-notices` 变更。组件测试覆盖：已转发的端口不提示、忽略后再次监听会重新提示、转发失败时保留提示并显示错误。

- [x] 5.4 运行 `pnpm --dir web check`、`lint`、`check:arch` 和 `exec oxfmt --check src`，全部通过。在 1280×720 和 390×844 视口、浅色和深色主题下检查对话框、抽屉和提示条。

## 6. 端到端与文档

- [x] 6.1 新增 Playwright 测试 `test/e2e/port-forwarding.spec.ts`。在隔离的测试会话中启动一个本地 HTTP 和 WebSocket 测试服务器，验证以下流程：
  1. 终端出现端口提示；
  2. 点击“转发并打开”，新标签页经过认证跳转后显示页面；
  3. WebSocket 回显正常；
  4. 移除转发后访问返回 404。
- [x] 6.2 在 `docs/user-stories/` 中新增分组 R（端口转发预览）及其故事：
  - US-R01：转发网关与凭据；
  - US-R02：兼容改写与路径前缀；
  - US-R03：首页转发管理；
  - US-R04：终端端口提示。

  更新 `docs/user-stories.md` 的索引和计数，以及 `coverage.md`；在 `access-and-sessions.md` 中说明 US-A 认证边界的变化。用 `make docs-check` 验证。

- [x] 6.3 运行 `make check` 和 `make test`，全部通过。用真实的 Vite 6 项目，在本机和 Tailscale 主机名下各验证一次页面和 HMR，并保留截图和验证记录。
  - 2026-11-06 验收记录（隔离实例：临时 HOME 与 token，监听 `0.0.0.0:7795`；Vite 6.4.5，`--base /forward/5173/`）：
    - 3 秒内检测到 `5173/node` 并出现端口提示；“转发并打开”后改为“保留前缀”。
    - `http://127.0.0.1:7795/forward/5173/` 与 `http://rc-pc.java-acrux.ts.net:7795/forward/5173/` 均显示页面。
    - 修改 `src/style.css` 后两处页面均经 HMR 更新且未整页刷新（Vite 日志 `hmr update /src/style.css`）。
    - 直接在新标签打开转发地址时出现跨站确认页，确认后进入。
  - 2026-11-06 `make check` 通过；`make test` 各阶段通过，唯一失败的浏览器用例 `CodeDocument` 横向滚动条与转发无关，单独运行与 CI 均通过。

## Workflow follow-up

- 实现合并后归档本变更，并把 `port-forwarding`、`session-port-detection` 同步到主规格。
