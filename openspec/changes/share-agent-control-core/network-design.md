# Design

## Context

动机见 [网络提案](network-proposal.md)，协议行为见 [mcp-network-transport](specs/mcp-network-transport/spec.md)。本文件是拟议设计，不是已运行网络 MCP 的证据。

已观察的基线：

- `mcpstdio/server.go` 已使用 2025-11-25 initialize，兼容旧版本，但逐行串行处理；`mcpcontrol` 只调用 localclient；ACP 已注入 stdio 控制服务。
- `localclient` 的 REST 使用固定 `http://webmux`，WS 使用 `ws://webmux`，dialer 忽略 authority；没有真实 TCP base URL/Bearer 选项，不能直接复用该虚拟 Host 做网络自调用。
- `api.middleware` 对所有请求校验 Host，但认证/Origin 只覆盖 `/api/` 和 `/ws/`；空 Token 被视为免认证，因此新增 `/mcp` 必须显式纳入保护。
- `api.NewBridge` 的普通 HTTP 和 P2P RPC 共享 route mux；把 MCP 注册到 `routes()` 会错误暴露给 P2P。`NewLocal` 也复用普通 routes，不能在那里注册网络端点。
- `cmd/webmux/main.go` 已取得真实 TCP listener 地址，再组装 api.Deps；http.Server 已有十秒请求读取期限、六十秒 idle timeout，没有统一短 WriteTimeout。
- `ensureToken` 会为非 loopback/隧道曝光的空 Token 生成进程秘密；网络 MCP 的空 Token 拒绝必须先于该分支，不改变关闭 MCP 时既有逻辑。
- 现有 access-link、server-local-api、effective-configuration 和 controller-service-layering 分别定义浏览器片段、Unix peer 身份、脱敏来源和 Controller DTO 规则；它们不包含网络 MCP。此变更新建独立 capability，不重写这些既有边界。

依赖 [共享核心](design.md) 的不可变 Registry、每连接 Session 与锁外 Dispatch，以及 [共享指南](../share-agent-operation-guide/design.md) 的 RenderContext、core/context 和 guide_read。

## Goals / Non-Goals

**Goals:**

- 同一控制 registry 在 stdio 与 HTTP 上执行；身份、framing、连接生存期各自明确，不让任一入口假扮另一个入口。
- owner Bearer 经过既有 TCP REST/WS 权威执行；在服务进程内自调用也不绕过认证、Host 或服务端业务去重。
- 在等待存在时仍可发现工具、并发操作和显式取消；资源和生命周期可精确测试，断线不推导取消。

**Non-Goals:**

- 不移除 stdio、不改 ACP 默认注入，不实现完整 CLI/MCP parity；不声称已有被证明的 MCP-only 产品需求。
- 不实现 OAuth/OIDC、注册或 protected-resource metadata，不支持旧 HTTP+SSE、server/discover、SSE 恢复、MCP tasks 或 server-to-client requests。
- 不新增 answer、view、upload、任意 REST 转发、文件读取/写入能力；不把服务器路径变成客户端文件系统承诺。
- 不将 webmux-history/thread_read 迁移到 HTTP，不扩展冻结历史的来源权限，不引入多用户/按会话 ACL。

## Decisions

### 1. Configuration is an explicit security opt-in

只新增 `[mcp] enabled = false` / `Config.MCP.Enabled`，不加独立 listen、Token、URL 或 Web 设置覆盖，也不新增 CLI flag。沿用既有 `serve --config` 选择配置。

```toml
listen = "127.0.0.1:7681"
token = "<由部署者设置的 owner 凭据>"

[mcp]
enabled = true
```

`config` 的 default、TOML 解析、字段来源、有效配置 allowlist 和生成示例必须同时覆盖该 bool。配置读取后、`ensureStartupToken/ensureToken` 前检查启用且 Token 为空即失败；`serve` 的直接嵌入入口也检查，不能靠绕过 loadConfig 启动无认证 handler。
首次正常生成配置已经持久化的随机 owner Token 属于已有配置生成，不是新 MCP secret provisioning。已有配置空 Token 即使同时设置 tunnel，也不得用曝光分支补值后继续。

保留部署层原有 Token 长度/公开隧道要求，MCP 不另造第二密码策略。非 loopback、HTTPS 代理、可信 VPN 均沿用现有 listen 与网络保护；生产公网须 TLS，不把 VPN 当作应用认证替代。空配置原本允许的 loopback REST 不受影响，只有明确启用网络 MCP 才触发新前提。

### 2. Compose once, expose only on TCP

```text
cmd/webmux composition root
  |-- config + bound listener + server context
  |-- localclient (real own TCP origin, configured owner credential)
  |       ^
  |       |
  |-- mcpcontrol (remote guide context) --> sessionobserve
  |       |
  |-- mcpprotocol immutable control registry
  |       |
  |-- mcphttp (session table, POST/DELETE framing)
  |       |
  +-- api.Deps.MCP http.Handler
          |
          +--> TCP HTTPHandler exact /mcp branch, common security
          X--> local / P2P / static mux

mcpstdio --> mcpprotocol --> same mcpcontrol handlers --> Unix localclient
threadread --> separate registry --> frozen Reader (stdio only)
```

`cmd` 创建并拥有出站 client、remote control registry、mcphttp 生命周期，将 handler 注入 `api.Deps.MCP`。`api.NewBridge` 的 TCP HTTPHandler 在 static/shared mux 前精确分流 `/mcp`；`/mcp/` 和关闭时 `/mcp` 明确 404，避免 Svelte fallback。P2P 的
ServeRPC/ServeStream 和 NewLocal 不挂载该 handler，局部传入非空 Deps 也不能改变此规则。

API 将 `/mcp` 纳入 Host、Bearer、Origin 检查，再由受信 callback/context 注入 `Principal{kind:remote_owner, instance, epoch}`。mcphttp 无有效 principal 即拒绝，防止未来误装到未保护路由。API 不导入 mcphttp：受信包装/调用接口由 cmd 注入，只需共享 contract/context
类型和 http.Handler；mcphttp 不导入 api、Service 或 cmd。

`mcphttp` 归为 Controller，`mcpprotocol` 为共享基础设施，`mcpcontrol`/`sessionobserve`/`localclient` 保持客户端层；在架构清单补全分类。HTTP lifecycle/取消/error DTO 与共享 MCP envelope 位于 `internal/contract`；工具 payload 也按既有 Controller wire
规则使用声明过的 contract 类型，不为任意 map 或反向导入添加架构豁免。

### 3. Remote principal is not process identity

唯一认证主体是同一实例 owner；MCP 会话 ID 只是 bearer 认证后的路由句柄。clientInfo 仅校验后丢弃，不持久保存；`X-WebMux-*`、环境变量、Forwarded、clientInfo 名称和模型自述都不能构造 self。

网络工具 RenderContext 固定 `Transport:mcp-http`、`Locality:remote`、`SelfSessionID:nil`，工具摘要从当前 registry 生成；guide_read 和 capabilities 与 stdio 共用定义，但远程说明不提供“本机可执行命令即可用”的配方。新会话无隐式 parent，
不能被服务进程所在会话树污染；调用者仍可用现有业务 schema 中允许的显式目标，不增加声明身份的参数。

本地 stdio 不经过此身份包装，继续使用 Unix 对端进程归属及既有 local guide context。不因为“服务端内部调用”而给远程代码本地 UID/PID 身份。

### 4. Own TCP client without token passthrough

扩展 `localclient` 构造选项以支持固定 real base URL 与受信请求凭据提供者；既有 Unix New(dialer) 默认语义保持。所有 REST 帮助方法（含 bounded/raw）和 WS 握手均从同一 origin 派生，REST 发 Bearer，WS 以 Authorization 发 Bearer，不使用 URL/Cookie 或额外 token 子协议。

cmd 使用已绑定 listener 经 `localOrigin` 得到自调用 origin；wildcard 使用同地址族 loopback 和实际端口，明确绑定其他地址则使用实际可拨地址。URL/Host 必须一致，不能保留 `http://webmux` 再只换拨号器。禁用环境 HTTP proxy、拒绝 redirects、固定该实例 endpoint；
调用者的 Host、Forwarded、Origin、Authorization、URL 参数和 clientInfo 不参与该选择。close client 释放 idle TCP 连接。

出站 Token 来自启动时已验证的 owner 配置，不复制入站 header；认证完成后才允许使用。这里不是跨服务 token passthrough：目标是同一实例的同一 owner 权威，REST 仍独立校验凭据。这个等价关系是硬前提；将来若增加 scoped/OAuth identity，不能给它们继续套 owner client，
必须先设计授权保真委托/交换机制，否则拒绝该 profile。该扩展不在本变更范围。

### 5. Reuse existing Host/Origin trust rather than invent one

启用路径所有方法都经安全检查；先 Host、再认证、再 Origin、再会话查询/方法分派。对缺失 Origin 的非浏览器客户端不伪造 Origin；出现时沿用 api.originAllowed 的同源、精确 allowed_origins 和动态 public origins 判定。拒绝任意 Forwarded 头自动授权，但不禁止已经获准的 HTTPS 代理或 Tailscale 来源。

HTTP Bearer 只取单一 Authorization header，重复或歧义凭据拒绝；会话凭据不能代替 auth。401 返回普通 Bearer challenge，不伪造 OAuth resource_metadata 或授权发现。该 Token 已能访问完整 REST 且能启动任意 argv，文档必须在复制凭据前明确高权限，不能因 tools/list 不含服务管理工具便称受限授权。

### 6. Pin initialize-era Streamable HTTP

规范依据：[2025-11-25 transports](https://modelcontextprotocol.io/specification/2025-11-25/basic/transports)、[lifecycle](https://modelcontextprotocol.io/specification/2025-11-25/basic/lifecycle)、
[cancellation](https://modelcontextprotocol.io/specification/2025-11-25/basic/utilities/cancellation)。不追随当前 latest 的协议代际。既有 stdio 协商历史版本保持；HTTP 允许集为 2025-03-26、2025-06-18、2025-11-25，前两者本来已有 Streamable HTTP 且现有控制握手已支持。

| 输入                                           | HTTP / 协议结果                                               |
| ---------------------------------------------- | ------------------------------------------------------------- |
| POST request                                   | 200 application/json；JSON-RPC result/error                   |
| POST 已接受通知                                | 202 空体；未知合法通知可忽略，不调用工具                      |
| POST response                                  | 400；本 profile 没有待回复 server requests                    |
| GET（认证/会话有效）                           | 405，Allow: POST, DELETE                                      |
| DELETE（认证/会话有效）                        | 204 空体，销毁 MCP 会话                                       |
| 其他方法                                       | 405，不回退页面                                               |
| 非 JSON Content-Type / Accept 缺 required 类型 | 415 / 406                                                     |
| body > 64 KiB                                  | 413，执行前失败                                               |
| parse / envelope / params                      | HTTP 400，-32700 / -32600 / -32602                            |
| ready 后未知 method                            | HTTP 200，-32601                                              |
| 工具参数非法 / 业务拒绝                        | 沿用共享 handler 的 isError:true 错误文本；不改变全局错误约定 |
| 无/坏凭据、坏来源                              | 401 / 403；不查询 MCP 会话表                                  |
| 缺会话 / 未知会话 / 错版本                     | 400 / 404 / 400                                               |
| 重复在途 RPC ID / 执行或会话容量满             | 409 / 429，后者 Retry-After: 1                                |

framing 严格单对象、拒绝 duplicate keys、batch、非 2.0；ID 是区分类型的 string 或安全整数。安全整数范围为 `[-9007199254740991,9007199254740991]`，数值规范化作为 inflight key；字符串保持原值、UTF-8 1–128 字节。null/对象/小数无效。请求非法时仅在 ID
可安全采用时回显，否则 null。`_meta` 不构成身份或能力；不支持的 task/progress 不承诺执行扩展。

mcpprotocol 继续拥有 registry、初始化状态和 method dispatch；mcphttp 校验 transport header/body、调用同一 Dispatch、将输出写入 HTTP。不能复制一套 tools/list/tool-call switch。GET 405 是规范允许的无 SSE 行为，不实现旧双端点或假 SSE。

### 7. Session schema and bounded lifecycle

MCP 会话不等于 WebMux 会话；状态仅内存保留：

```text
Entry {
  id: base64url(random32), principal: remote_owner + instance epoch,
  protocolVersion, protocolSession,
  createdAt, lastActivity, initializedDeadline,
  inflight: map[typedRpcID] { cancel, deadline }
}
new initialize -> awaiting_initialized -> ready -> terminated
                        |                  |
                   60s deadline       DELETE / idle / 1h / shutdown
```

初始化不带 MCP-Session-Id；成功才分配 43 字符随机 ID、返回 header。后续 header 必须单值且格式正确，版本 header 若缺省使用该会话绑定版本以兼容旧客户端，显式值必须匹配。未知 initialize 版本返回基线供客户端自行拒绝，不把未协商现代功能列为支持。只有 initialized 后
tools/list/call 可用；ping 只需存在会话。未知合法 notification 忽略并 202，已知通知 params 非法则 400。

最多 128 Entry；awaiting 60 秒、idle 十分钟、hard 一小时。闲置计时从最后已接受操作完成开始；有 inflight 不闲置淘汰，但 hard 到期仍撤销。一个实例级定时扫描已足够，不为每个会话起常驻 goroutine。每个 Entry 最多 4 个 inflight tool、全实例 8 个，不建等待队列；
表锁只用于查找/状态/额度、从不包住 I/O。通知/ping/DELETE 不占工具槽，仍受 HTTP 读期限和 64 KiB 输入限制。

只保存版本、owner/epoch、时刻和 inflight cancel map；完成后删 request ID，不做结果缓存，不保存完整 clientInfo、reason、正文/历史。会话饱和即 429，不淘汰其他活动用户来迎合新 initialize。过期/DELETE 后清理 observer 与 WS；已知客户端重试旧 ID 得 404，重新
initialize。重启即 epoch 改变，不读写持久 MCP 会话文件。

### 8. Independent call contexts and cancellation races

每个调用从服务 context 派生独立 context，不从 r.Context 的断线信号推导用户取消；继续使用 registry 的 185 秒总期限和 handler 更短的读/wait 限制。同步 HTTP handler 可以在客户端掉线后继续有界工作，不增加无界后台任务；响应无法写回便丢弃，finally 关闭订阅、注销
inflight、释放额度。HTTP 读取保持十秒，MCP 响应写入设置至多五秒 deadline，不能给全站设置会杀死正常 60 秒 wait 的短 WriteTimeout。

通知 `notifications/cancelled` params 为 `{requestId: string|safe-integer, reason?: string}`，拒绝未知字段、重复字段、reason 超过 256 UTF-8 字节。查找键包含 Entry + typed request ID；未知/已结束目标仍 202，不透露别的会话。initialize 不可取消。取消 winner
只执行该 context cancel，释放观察、API 请求和 WS；不得调用 interrupt、kill、rm 或补偿回滚。存活被取消 request 返回 `error:{code:-32000,message:"Request cancelled",data:{kind:"request_cancelled"}}`。

取消与正常完成以同一 inflight 终结短锁决定唯一 response；cancel 在结果已确定后不覆盖真实成功，重复取消幂等。DELETE、hard expiry、shutdown 取消所有 entry 工作但不停止持久任务；本次 observation 已超时则继续映射 waitTimedOut，不伪造任务 stopReason。写入可能在
cancel/断线前已经提交，错误不能证明“未执行”。无隐式重试。

### 9. Preserve operation identity and bounded output

RPC ID 用于 HTTP response/cancel 相关性，clientRequestId 用于业务去重；两者不能互换。响应丢失后新 RPC ID + 原 clientRequestId/负载再次调用由服务端处理，MCP 不扩窗：创建十分钟进程内；混合每会话 256 项/三十分钟；终端 receipt 有自己的保留/generation。
重启/淘汰/超窗先 inspect，不能承诺 exactly-once。wait 超时后保持原 turn/queue selector，read 继续原 cursor；若原游标/历史已淘汰则呈现原工具 reset/缺失结果，不靠新 MCP 初始化补造历史。

HTTP 最终编码上限 8 MiB + 64 KiB，是额外封套上限，不放宽下游的 4 MiB 查询、64 MiB WS replay、64 KiB reply 和 guide 64 KiB 上限。
structuredContent 与 JSON text 双重编码可能额外转义，不能保证所有接近下游上限的结果均能装入封套；超限明确失败，不截断工具字段。
使用有界 encoder buffer，在写 header/正文前完成大小检查，超限返回有界 -32603，不先复制无界 JSON 后裁剪。
八个并发调用的回放帧载荷上限合计约 512 MiB，解码及响应另占内存；实现压力用例需记录真实峰值，不把载荷估计当进程内存上限。

所有响应 no-store；日志只记录受控错误类别/状态，不记录 header、session ID、正文、reason、完整 clientInfo 或模型可见结果。HTTP failure 不带 Token/路径/内部堆栈，不回显被拒绝的凭据。

### 10. Bearer compatibility is deliberately narrower than OAuth

[官方 MCP Inspector 配置文档](https://github.com/modelcontextprotocol/inspector/blob/main/docs/mcp-server-configuration.md) 明确支持 Streamable HTTP URL、`--header` / headers 和 legacy initialize-era。兼容验收固定其实际安装版本，设置 `type:http`、
`url:<实例>/mcp`、Authorization Bearer、`protocolEra:legacy`；使用 Inspector UI 保持同一连接完成多次调用，CLI 可做独立发现检查。仅需 URL 与凭据，客户端机器无 WebMux binary/socket/bridge。

这里的“URL-only”是无需 WebMux 本机安装，不是无需认证。标准 HTTP MCP 授权 profile 包含 OAuth 等约束，本变更明确只交付静态 owner Bearer 部署 profile，不能描述为完整实现标准 OAuth authorization。OAuth-only hosted clients 未被验收也不宣称可用；若产品需要它们，
必须单独决策授权服务器/发现、audience、scopes、委托及 Token 生命周期，不能靠把 owner Token 填进未知 OAuth 字段规避。

## Risks / Trade-offs

- [owner 凭据泄漏意味着完整实例权限和任意 argv] → 默认关闭、明确警示、既有 Bearer/Host/Origin、可信 VPN/TLS；不把工具表当 ACL。
- [自调用回环被误用为本地 UID 身份] → 固定真实 TCP endpoint、配置凭据和 explicit remote principal；身份负例同时覆盖服务启动于会话内的场景。
- [等待拖住取消或其他客户端] → 独立 context、锁外 Dispatch、有界执行额度且通知不占槽；取消和完成竞争必须 race 测试。
- [JSON-only 不能服务器推送，断线无 response replay] → 协议明确 GET405；客户端 inspect/业务 ID 重试/同 selector 续等，不新增可靠消息系统。
- [兼容性被误读为任意 hosted client 可用] → 只报告实际 Inspector 验收版本与 Bearer profile；OAuth 扩展须独立产品决策。

## Migration Plan

1. 依次合入共享核心和共享指南，再整层实现网络配置、TCP client、身份、HTTP adapter、装配和安全测试；不能先挂未实现 handler。
2. 默认关闭部署无行为变化；部署者明确配置 owner Token 后启用并重启。新增无持久状态，不迁移会话历史或去重格式。
3. 回滚时关闭 `[mcp].enabled` 并重启即可撤销网络入口；回退二进制前移除该新配置表以兼容严格旧解析。重启对持久会话的影响沿用已有恢复机制，不宣称所有 PTY 进程跨重启均存活。
4. 实现后才更新专题 docs、安全边界、故事与覆盖索引，不在设计 PR 宣称已发布。

## Verification Plan

- 单元矩阵：header/body/schema/status、所有握手状态、版本缺省/冲突、session TTL/满额、重复 typed ID、取消 winner/finish winner、跨会话相同 ID、disconnect 与 hard expiry；时钟可注入，不用真实长 sleep。
- API 装配矩阵：默认关闭、TCP启用、Unix/P2P/尾斜杠/static404，空 Token 在自动补值前失败，direct serve 防绕过，Host/Origin/代理 allowlist 和 Cookie/query/会话 ID 认证负例。
- 真实隔离实例与 fake-agent：官方 Inspector URL+header 发现、guide、launch/send/read/wait/interrupt，读/等/取消同时并发；记录实际客户端和协议版本，不用内部 httptest 冒充外部兼容。
- 写入响应故意丢失后相同业务 ID 不重复；换 RPC ID 可重试，同 RPC ID 在途409；跨重启旧 MCP ID404、重新初始化续读/续等，去重超窗先 inspect。
- identity 负例：伪造 clientInfo/session header、服务运行在已有 WebMux 会话内、远程无隐式 parent/self；本地 stdio peer 仍正确；REST/WS 都携真实 authority/Bearer、不经 Unix、Token 不随 redirects/环境 proxy 泄漏。
- tool registry 快照按名称/schema比较 HTTP 与 stdio 控制面，仅 remote context 不同；thread_read 不存在，冻结 stdio仍拒绝跨来源，未新增 answer/view/upload。
- 实现后由主执行者运行定向 Go race、架构/契约与 docguard，然后按变更范围运行完整 gate；本规划阶段没有运行实现验证。
