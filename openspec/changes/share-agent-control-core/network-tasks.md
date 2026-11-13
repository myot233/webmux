# Tasks

本表记录网络入口的实现与验证进度，依赖已完成的共享核心与共享指南切换；
不重复提取 observer/registry，也不创建不可用 handler 骨架。完成定义见 [network-design](network-design.md) 和
[网络传输规格](specs/mcp-network-transport/spec.md)。各组同时交付其测试与相关说明，最后一组只做跨层集成。

## 1. Configuration and startup security

- [x] 1.1 在 `internal/config` 增加 `[mcp] enabled` 默认 false、严格 TOML 解析和 default/toml 来源；
      同步生成配置示例与 configured/effective 的脱敏 bool allowlist，以配置解析/来源/敏感数据测试证明无 Token 泄露。
- [x] 1.2 在 `cmd/webmux/server_config.go` 的曝光自动补 Token 前拒绝 enabled 且空 Token，并在 direct serve 入口重复保证前提；
      测试 loopback、非 loopback、隧道、已持久化首次配置及显式空值，断言失败前未监听、未新增 MCP secret。
- [x] 1.3 在既有 `docs/usage/mcp.md` 记录启用配置、启动失败、重启/回滚和 owner 凭据权限；
      更新 US-K09 相关配置验收点，以示例配置测试与文档引用检查证明默认关闭和非空前提一致。

## 2. Authenticated own-instance TCP client and identity

- [x] 2.1 为 `internal/localclient` 增加真实 base URL 与受信凭据选项，迁移所有 REST 辅助方法及 WS 握手使用同一真实 authority；
      保留 Unix 默认构造语义，以 TCP REST/WS 捕获测试确认实际 Host、Bearer、地址族/动态端口和无 Cookie/query token。
- [x] 2.2 固定自调用 endpoint、禁用环境 proxy、拒绝 redirects，凭据仅来自已验证配置而非入站 header；
      测试伪造 Host/Forwarded/URL 元数据、重定向和代理环境，断言无法改目标或泄漏凭据，API 拒绝仍被保留。
- [x] 2.3 在 `internal/contract` 声明受信 remote-owner principal/context 边界，由认证后的 TCP 装配注入；
      mcpcontrol 使用前置 guide RenderContext 的 mcp-http/remote/self=null，测试 clientInfo/header/会话 ID 伪造均不能成为身份。
- [x] 2.4 测试服务进程本身处于 WebMux 会话树时远程 launch 仍无隐式 parent，而 stdio Unix peer 仍保留真实 caller/parent 归属；
      不增加 parent 声明参数或 PID 发现，未知指南 self 为 null；不从环境构造远程身份，断言请求路径未触及 Unix socket。
- [x] 2.5 更新既有 MCP 专题的 owner/REST 等权、任意 argv、高权限凭据、路径不等于客户端文件可达说明，涉及 US-A02、US-S20–S21；
      文档检查须拒绝“工具清单是 ACL”“MCP 是沙箱”“获得绝对路径即可上传”等错误承诺。

## 3. HTTP adapter and bounded transport lifecycle

- [x] 3.1 新建 `internal/mcphttp` Controller，包装下层 mcpprotocol Registry/Session/Dispatch 而不复制 tools switch；
      HTTP lifecycle、取消、错误和响应 DTO 放入 contract，并将可见 tool payload 映射到声明类型，以架构/契约测试证明不需豁免。
- [x] 3.2 实现单对象 POST、严格 duplicate-key/envelope/ID 校验、64 KiB body、Content-Type/Accept 检查；
      用边界表测试 batch、null/不安全数值 ID、畸形 JSON、非法参数、413/415/406、错误 ID=null 与业务零副作用。
- [x] 3.3 实现固定 2025-11-25 及 2025-03-26/06-18 的 HTTP 允许集、未知版本回基线和 initialize/initialized/ping 顺序；
      测试 clientInfo/capabilities schema、版本 header 缺省/不匹配、server/discover -32601，以及 stdio 既有协商不变。
- [x] 3.4 实现随机 MCP-Session-Id、owner/epoch/版本绑定、128 上限、未 ready 六十秒、闲置十分钟、硬期限一小时；
      注入时钟测试所有状态迁移、会话 header 重复/畸形、缺失400、过期404、容量429与无持久写入。
- [x] 3.5 实现全实例八个、每会话四个工具执行额度和 typed inflight RPC ID，禁止锁内 I/O 与无界等待队列；
      并发测试 wait 不中断其他 list/control、满额429/Retry-After、在途 ID409，以及 ping/cancel/DELETE 不占工具槽。
- [x] 3.6 从服务 context 建立独立调用期限，断线不触发显式取消；实现每会话 requestId 取消、终结竞争、DELETE 和关闭清理；
      race 用例覆盖同 ID 跨会话隔离、重复/迟到取消、完成先赢、取消先赢、已提交写不回滚、observer/WS/额度及时释放。
- [x] 3.7 实现 JSON response、通知202、GET405、DELETE204、无 SSE/response replay 和 no-store；
      编码前执行 8 MiB+64 KiB 封套上限及五秒写期限，测试超限仅受控错误、无半个 JSON、无 Token/正文/reason 日志。
- [x] 3.8 在既有 `docs/usage/mcp.md` 与对应协议说明加入 HTTP status、session/version/TTL、并发、取消/断线、
      RPC ID 与 clientRequestId 区别及同 selector/cursor 续读规则；对照本组行为表测试核对文档，不扩大服务端去重窗口。

## 4. TCP composition and protected exposure

- [x] 4.1 `cmd/webmux` 在 listener 已绑定后构造真实 TCP client、remote control registry 和 mcphttp，将 handler 注入 api.Deps；
      统一服务 shutdown/Close 顺序，以启动/关闭集成测试证明无双监听、无 client/observer 泄漏，也没有 stub handler。
- [x] 4.2 `api.NewBridge` 仅在 TCP HTTPHandler 的精确 `/mcp` 分支挂载处理器，并纳入既有 Host、Bearer、Origin；
      测试所有方法均认证，关闭/尾斜杠404，Unix/P2P RPC/stream404，且 static fallback 不返回 HTML 或 redirect。
- [x] 4.3 复用现有 trusted proxy/allowed_origins/public registry，缺省 Origin 允许、非法 Origin403，不因 Forwarded 扩权；
      补充 loopback 重绑定、非 loopback 合法 Host、可信 HTTPS 代理成功与未授权代理失败矩阵，不收窄原可信来源。
- [x] 4.4 补充 Bearer 缺失/错误/重复 header、query/Cookie/仅 session ID、session 换持有人等认证负例；
      断言401 challenge、认证先于 session 查找，以及未来非 owner 主体不能获得 owner client 的显式拒绝边界。
- [x] 4.5 在架构清单登记 mcphttp Controller 和新增 DTO 使用，沿用核心已建立的 mcpprotocol/sessionobserve 分类；
      运行架构测试证明 mcphttp 不导入 api/Service，mcpcontrol 仅 HTTP/WS，DTO 与已有 wire 规则相容。
- [x] 4.6 更新 `SECURITY.md` 与既有部署/MCP 专题的 TCP、VPN/TLS、Host/Origin 和 owner 风险边界，对应 US-A02、US-A03、US-K09；
      检查文档不承诺 OAuth discovery、OAuth-only hosted clients 或缩权 MCP ACL，不新增与本功能无关的文档变动。

## 5. External-client acceptance and integrated delivery

- [x] 5.1 在隔离实例与可控 fake-agent 上使用官方 MCP Inspector 的真实安装版本，按 URL+Authorization+legacy 配置连接；
      完成 initialize、tools/list、capabilities、guide_read、launch、send、activity/terminal read、指定 turn/queue wait 和 interrupt，
      记录客户端/协议版本及输出证据；客户端不运行 WebMux 二进制、不连接 socket、不使用 Unix bridge。
- [x] 5.2 跨层验证等待并发、显式取消与断线差异、等待超时仅观察、写入已提交后丢响应；
      新 RPC ID+原业务 ID/负载在原去重窗口内不重复创建/发送，超窗/淘汰/重启不承诺 exactly-once，并以同 selector 续等。
- [x] 5.3 重启隔离服务验证旧 MCP session404、重新初始化后对仍存在的 WebMux 会话使用原 cursor/turn selector；
      丢失/淘汰历史仍给原工具 reset/缺失结果，不把传输重建冒充历史恢复，不宣称普通 PTY 跨重启持久。
- [x] 5.4 对照 HTTP 与 stdio 控制 registry 的工具名/schema/结果，只有 remote guide context 不同；
      验证网络无 thread_read/answer/view/upload，独立 webmux-history stdio 仍只读冻结来源并拒绝跨 conversation。
- [x] 5.5 实现验收通过后，在 `docs/user-stories/shell-control.md` 新增 US-S25“通过实例 URL 连接 MCP”，
      更新 US-S20–S23 的传输/观察边界，并同步 `docs/user-stories.md` 的 S 范围、总数与 coverage 映射；
      核对故事 ID 唯一、来源链接真实；如实现前其他已合入变更占用该 ID，顺延使用下一空闲 ID 并同步所有引用，不纳入未实现的 OAuth/文件能力。
- [x] 5.6 主执行者在所有改动完整落地后统一运行相关包 Go race、archtest、契约/协议测试、docguard 和适用完整 gate；
      记录真实 Inspector smoke、错误/取消/身份负例及资源上限结果，不把仅 httptest 或规划检查报告为外部兼容通过。
