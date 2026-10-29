## 1. 来源判定

- [x] 1.1 在 `internal/api/push.go` 放宽 `pushOrigin`：HTTPS 来源经 `normalizeAuthority` 后可用；回环保留 `http`/`https`；非回环 `http` 与本机套接字仍为 `unsupported_origin`
- [x] 1.2 将 Quick Tunnel 判定移到通用 HTTPS 分支之前，条件为当前 `cloudflare` authority 匹配且不等于有效 Named 主机名
- [x] 1.3 无 `Origin` 的 GET 在 Named 与 `PublicOrigins` 之外，按主机名匹配 `allowed_origins` 中的 HTTPS 条目，并以该条目含端口的规范 authority 作为锚点
- [x] 1.4 更新 `unsupported_origin` 服务端文案，说明 HTTPS 入口（含 Tailscale Serve/Funnel）与本机回环
- [x] 1.5 更新 `quick_tunnel` 服务端文案，保留地址轮换说明并补充 Tailscale Serve/Funnel 等替代 HTTPS 入口

## 2. 测试

- [x] 2.1 扩充 `TestPushOrigin`：Tailscale `:8443` 保留端口、`allowed_origins` HTTPS、大小写规范化、`:443` 省略、Quick Tunnel 与 Named 并存、`http` VPN、路径与凭据拒绝、本机套接字
- [x] 2.2 在 `TestPushAPI` 增加 Tailscale 来源的可用状态、注册成功与删除，确认订阅来源经过同一判定
- [x] 2.3 新增 `push-status-tailscale` wire 快照并登记归一化规则，重录 `internal/api/testdata/wire`

## 3. 前端

- [x] 3.1 更新 `web/src/components/PushSettings.svelte` 的非安全上下文、不支持来源与 Quick Tunnel 提示，点名 Tailscale Serve/Funnel
- [x] 3.2 同步 `web/test/browser/PushSettings.test.ts` 的文案断言

## 4. 文档与故事

- [x] 4.1 更新 `docs/protocol/http-push.md`、`docs/cloudflare-tunnel.md`、`docs/tailscale.md`、`docs/deployment.md`
- [x] 4.2 更新 US-C12 与 US-N08，并在 `docs/user-stories.md` 记录本次局部核对

## 5. 验证

- [x] 5.1 运行 `go test ./internal/api`、定向 Vitest 浏览器用例、`svelte-check`、Oxlint、格式检查与文档守卫
