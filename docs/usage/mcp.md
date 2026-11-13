# 通过 MCP 控制 WebMux

WebMux 的 ACP 聊天会话默认获得 `webmux` MCP 服务，可以查询、创建和操作当前实例的会话，无需手动配置。
外部 MCP 客户端也可以连接同一组工具。
交接自动注入的 `webmux-history` 仍然独立，只读取冻结的来源历史。

## ACP 默认接入

新建 ACP 聊天、从终端切到聊天，以及恢复已有聊天时，WebMux 都通过 ACP `mcpServers` 传入当前实例的 `webmux` stdio 服务。
升级前已经连接的聊天需重新打开或恢复后生效；不会修改 agent 的全局 MCP 配置或已保存的启动配置。
关闭聊天历史保存也不会关闭控制工具。能力、模型和身份探测不注入控制 MCP，也不启动 MCP 子进程。

控制工具使用当前 WebMux 可执行文件和明确的本机 socket，配置不携带 Token 或额外环境值。
若可执行文件不可用或当前实例未持有本机 socket，则省略可选服务；若 agent 不支持 MCP，实际工具可用性取决于其 ACP 实现。
交接目标额外获得 `webmux-history`；该工具仍只读冻结的来源范围，而 `webmux` 控制工具可访问本机 API 允许的其他会话。

## 外部客户端：本机 stdio

```json
{
  "mcpServers": {
    "webmux": {
      "command": "/absolute/path/to/webmux",
      "args": ["mcp", "--socket", "/absolute/path/to/webmux.sock"]
    }
  }
}
```

`webmux guide --path` 返回运行实例的可执行文件与套接字路径。也可以省略 `--socket`，按 CLI 相同规则使用 `WEBMUX_SOCKET` 或默认配置；`--config` 选择其他配置，与 `--socket` 互斥。
使用明确的绝对路径可以避免连接到另一实例。服务不可用时工具返回错误，不自动启动服务。

## 外部客户端：网络 URL

网络 MCP 默认关闭。客户端无需安装 WebMux、命令或本机 socket；需要支持 initialize-era Streamable HTTP 与自定义 Authorization 头。

> [!WARNING]
> 这里复制的是实例 owner 凭据，不是受限 MCP 账号。同一个 Token 可调用完整 REST API、启动任意 argv，以服务用户身份执行命令。
> 精选工具列表不是 ACL 或沙箱；只把凭据交给可信客户端。

1. 在服务配置中先设置非空根 `token`，再启用：

   ```toml
   token = "REPLACE_WITH_CONFIGURED_OWNER_TOKEN"
   
   [mcp]
   enabled = true
   ```

2. 重启 WebMux。在已有配置中启用 MCP 却留空 token 会在自动生成启动 Token **之前失败**，即使监听 loopback 或同时启用隧道也不例外。
3. 客户端选择 HTTP，填写实例的精确 `/mcp` URL 与 Bearer。支持下列配置形状的 legacy 客户端可使用：

   ```json
   {
     "mcpServers": {
       "webmux": {
         "type": "http",
         "url": "https://YOUR_TRUSTED_HOST/mcp",
         "headers": { "Authorization": "Bearer YOUR_OWNER_TOKEN" },
         "protocolEra": "legacy"
       }
     }
   }
   ```

字段名称依客户端而定；官方 MCP Inspector 可配置 Streamable HTTP URL 和 Authorization 头。
这是静态 owner Bearer profile，不提供 OAuth、授权发现或注册；只接受 OAuth 的托管客户端不能据此连接。
不支持旧 HTTP+SSE、2026 stateless discovery 或服务端主动请求。

通过可信 VPN 限制可达范围，远程部署使用 TLS（由反向代理或可信 HTTPS 入口提供）；VPN 不替代 Bearer。
代理应保留正确 Host 与实际 Origin，按既有 `allowed_origins` / 受控公网来源配置，不能靠 `Forwarded` 头绕过检查。
非浏览器客户端可不发 Origin；带 Origin 时仍校验。只开放现有 TCP listener 的精确 `/mcp`，不开放 `/mcp/`、Unix 或 P2P MCP 端点。

回滚时设置 `[mcp] enabled = false` 并重启，入口恢复 404。回退到不认识该表的旧二进制前移除 `[mcp]`。
重启会终止普通 PTY 与进程；聊天的持久身份、历史和恢复机制不代表进程存活。

### 网络握手、状态与资源上限

使用 JSON-RPC 2.0 单对象；不接受 batch、重复字段或客户端 response。POST 使用 `Content-Type: application/json`，
Accept 包含 `application/json` 与 `text/event-stream`（虽然此 profile 只返回 JSON，不建立 SSE）。
initialize 不带 `Mcp-Session-Id`；基线版本为 2025-11-25，兼容 2025-03-26 / 2025-06-18。
未知 initialize 版本返回基线，由客户端决定接受与否。

成功 initialize 返回 `Mcp-Session-Id`；后续请求、通知、GET 和 DELETE 都带回该头，且始终提供 Bearer。
`MCP-Protocol-Version` 若省略则使用会话绑定版本，显式提供时必须匹配。
发送 `notifications/initialized` 后才能列举和调用工具；ping 只要求会话存在。MCP 会话不是 WebMux 执行会话，ID 不能代替认证。

| 操作或错误                                  | HTTP 结果                              |
| ------------------------------------------- | -------------------------------------- |
| POST request                                | 200 JSON-RPC result/error              |
| 已接受通知（含未知合法通知）                | 202 空体；通知不执行工具               |
| GET（有效会话）                             | 405，Allow: POST, DELETE；无 SSE       |
| DELETE（有效会话）                          | 204；取消并等待该 MCP 会话在途工作清理 |
| 客户端 response / 非法 envelope、参数、JSON | 400                                    |
| Content-Type / Accept 不合要求              | 415 / 406                              |
| 输入超过 64 KiB                             | 413，执行前拒绝                        |
| 无或错误 Bearer / 不允许 Host 或 Origin     | 401 / 403                              |
| 缺会话 / 未知或过期会话 / 版本冲突          | 400 / 404 / 400                        |
| 同会话重复在途 RPC ID / 容量满              | 409 / 429（Retry-After: 1）            |

ready 后未知 method 返回 HTTP 200、JSON-RPC -32601；业务拒绝与工具参数错误沿用 `isError:true` 工具结果。
其他方法返回 405；关闭入口或尾斜杠路径返回 404，不回退到网页。

- 最多 128 个内存 MCP 会话；initialize 后 60 秒内需 initialized，空闲十分钟或存活一小时后过期。
- 空闲从最后接受的操作完成计时；在途调用不按空闲淘汰，但仍受一小时硬期限约束。服务重启使全部旧 MCP ID 失效。
- 全实例最多八个、每 MCP 会话最多四个并发工具，不排队；额度及 typed RPC ID 一直保留到响应交付完成或失败。
- 输入最多 64 KiB，读取期限十秒；完整编码响应最多 **8 MiB + 64 KiB**，响应写入期限五秒。
- 封套上限不放宽下游 4 MiB JSON 查询、64 MiB 聊天 WS 回放、64 KiB 最终回复或 guide 上限；超限明确失败，不截断字段。响应不缓存。

### 取消、断线与重连

RPC ID 仅关联响应和取消：字符串与数值 ID 不同；数值须为安全整数，字符串为 1–128 UTF-8 字节。
`clientRequestId` 是创建、发送、中断的业务去重 ID，不能用 RPC ID 代替；既有去重保留窗口不变。
同一 RPC ID 在响应尚未交付时仍算在途；交付后删除，不提供响应缓存或断线 replay。

显式取消使用 `notifications/cancelled`，params 为 `{"requestId":"RPC_ID","reason":"optional"}`，reason 最多 256 UTF-8 字节。
未知或已完成目标仍返回 202；取消只作用于同一 MCP 会话的匹配 typed ID。取消胜出时请求返回 -32000、`data.kind=request_cancelled`；已确定的成功不会被后来的取消改写。
取消清理本次观察、API 请求和 WS，不执行 interrupt、kill、删除或回滚；需要中断执行时调用 `session_interrupt`。

断开 HTTP 不等于显式取消：调用继续受 185 秒总期限及工具更短期限约束，不能交付的响应会丢弃。
DELETE 或过期只取消 MCP 工作，不自动停止业务任务；服务关闭时仍由既有服务器生命周期终止 PTY。错误或取消不能证明写入未提交，也不会回滚已提交写入。
重新 initialize 后先检查业务状态；响应丢失时用新 RPC ID、原 clientRequestId 和相同负载重试写入。
创建去重仅十分钟且仅进程内；混合输入每会话保留 256 项 / 三十分钟，终端回执遵循其原有保留与 generation。
重启、淘汰或超窗后先 inspect，不能承诺 exactly-once。
wait 继续同一 turn/queue 选择器，read 继续原 cursor；游标或历史淘汰时接受工具的 reset / 缺失结果，不靠 MCP 重连补造历史。

远程 `capabilities` / `guide_read` 明确报告 `transport=mcp-http`、`locality=remote`、`selfSessionId=null`。
环境、clientInfo、头或服务进程所在会话都不提供远程 self 身份；网络创建不推断 parent。
工具中的 cwd、worktree 和交付物路径都在服务器上，不表示客户端可以直接读写这些文件。

## 查询工具

| 工具           | 用途                                                                                                 |
| -------------- | ---------------------------------------------------------------------------------------------------- |
| `capabilities` | 可用启动配置 ID、名称、聊天支持、当前工具列表与显式 transport/locality/selfSessionId；不探测登录状态 |
| `session_list` | 按 state、parentId、repositoryId、status 筛选；offset 缺省 0，limit 缺省 20、最大 100                |
| `session_get`  | 按 sessionId 读取详情、父子关系、worktree 与浏览器链接                                               |
| `guide_read`   | 按需读取操作指南索引或七个精选主题；只读，不加载聊天历史或任意路径                                   |

`session_list` 的 state 为 active（默认）、closed、interrupted、archived 或 all；status 为 running、waiting、idle 或 exited。
返回 nextOffset、done 和 total；列表在调用间可能变化，offset 不是冻结快照。

## 按需读取共享操作指南

初始化 instructions、capabilities 与 guide_read 使用同一实际注册表快照，不列出未注册工具。先调用 `guide_read({})`，再按需传 `{"topic":"requests"}` 等主题。
主题为 connect、operations、completion、requests、launch、artifacts、rules；不提供 shell 完整手册、commands、任意 URL/路径或 full 参数。
成功对象始终包含 kind、transport、locality、selfSessionId、tools、topics、topic、markdown；text 是同一对象的默认 JSON 编码，完整工具结果最多 64 KiB，不截断半篇正文。
未知主题、null/空 topic、未知参数与超限返回 isError 工具错误，后续调用仍可继续。stdio 不自动证明会话身份，未知 selfSessionId 为 null，不能从调用者环境猜测。
当前控制工具没有 answer/view/upload 或 agent 设置选项发现；授权需用户或已有客户端处理，动态模型设置先通过具备发现能力的 CLI 查询，不猜 optionId、provider 或模型。
CLI 与 MCP 共用逻辑操作 ID、去重边界、完成证据、授权范围、版本核对和交付物生命周期规则；冻结历史 registry 不获得 guide_read 或控制工具。

## 创建、发送与中断

`session_launch` 接受[创建会话请求](../protocol/http-session-create.md)，要求显式 `clientRequestId`（8–64 个字母、数字、下划线或连字符）。
配置 ID 从 `capabilities` 取得；位置可选 cwd、workspace 或 newWorktree。命令使用 argv 数组，不经 shell 展开。

```json
{
  "clientRequestId": "review-20261113",
  "configId": "CONFIG_ID",
  "view": "chat",
  "prompt": "审查当前改动",
  "workspace": { "repositoryId": "REPO_ID", "worktreeId": "WORKTREE_ID" }
}
```

返回 session、worktree、configWarnings、可用时的 link，实际提交初始聊天提示时的 initialInput 回执与等待选择器。
创建响应丢失时重试相同请求；创建去重仅在服务进程内保留十分钟，服务重启后应先查询现有会话再决定是否重建。
失败正文保留 step、已创建 sessionId/worktree；不会自动回滚，不要因后续提示失败而重复创建。

`session_send` 接受 sessionId 与 request，request 沿用[会话输入契约](../protocol/http-sessions.md#输入与中断)：

```json
{
  "sessionId": "SESSION_ID",
  "request": {
    "clientRequestId": "followup-20261113",
    "text": "补充测试",
    "mode": "auto"
  }
}
```

- 聊天返回 turnId/promptId，排队时返回 itemId/position；steer 插话不创建独立回合。
- 终端使用 submit/keys；写入回执仅证明输入已写入，不证明命令执行或任务完成。
- `session_interrupt` 接受 sessionId/clientRequestId，按当前视图取消回合或写入中断键，不删除会话。
- 输入与中断的去重、错误和视图限制完全沿用服务端；重试复用同一个 clientRequestId。
- 创建操作沿用服务端三分钟上限；MCP 调用总上限为 185 秒，断开连接不等于停止已创建会话。

## 增量读取输出

`session_read` 默认以 activity 视图读取已提交的规范化历史，支持运行中与已归档的聊天会话：

```json
{
  "sessionId": "SESSION_ID",
  "view": "activity",
  "afterPosition": "0",
  "limit": 20,
  "maxBytesPerItem": 4000
}
```

使用 nextPosition 继续读取，直到 done 为 true；每页最多 100 项、每项最多 65536 字节，乘积不超过 256 KiB。
截断的长项使用 itemId、textOffset（字符串）与 maxBytesPerItem 继续；每项的 nextTextOffset 是 UTF-8 字节偏移。
只返回 exportable 项，仍推进整页游标；保留 gaps、historyOrigin 和 reconcileRequired，区分分页截断与真实历史缺口。
正在生成的草稿尚未提交，不会出现在这里；私有 reasoning、附件正文与原始 provider 状态不导出。

终端视图使用原始回放字节，适用于读取终端输出，不接管输入或尺寸控制：

```json
{ "sessionId": "SESSION_ID", "view": "terminal", "maxBytes": 65536 }
```

返回 encoding=base64 的 data，以及 generation、start、end、nextOffset；下一页传回 generation 与 offset=nextOffset。
maxBytes 范围 1–65536，limited 表示尚有回放字节；解码后按顺序拼接字节，避免把跨页 UTF-8 字符或 ANSI 序列拆开处理。
游标已淘汰或 generation 变化时 reset 为 true，从仍保留的起点重新读取；historyTruncated 表示更早字节已丢弃。
终端分页使用独立原始输出 API，历史淘汰后仍返回保留的后缀，不返回渲染恢复字节；每次只复制请求页，不附着客户端或触发重绘。
终端回放不会变成持久日志；全屏 TUI 字节也不是当前画面的纯文本，画面读取可使用现有 `webmux screen`。
每次读取限时十秒；读取超时或连接失败不停止会话。

## 等待指定回合或进程退出

`session_wait` 要求选中一个具体聊天回合：使用 `session_send` 回执的 turnId，或排队回执的 queueItemId。
`session_launch` 返回的 wait 对象使用真实初始输入回执的 turnId 或 queueItemId；没有提交提示时不提供等待选择器。
也可显式传 firstTurn=true 选择仍保留的首回合；历史淘汰后不会误认后来的回合。

```json
{ "sessionId": "SESSION_ID", "turnId": "TURN_ID", "timeoutMs": 30000 }
```

聊天回放帧沿用 CLI 的 64 MiB 传输上限，模型可见的最终回复仍最多 64 KiB，不因无关的较早历史超过 4 MiB 而失败。
每次等待默认 30 秒、最多 60 秒。timeoutMs=0 读取初始回放后立即返回，连接和回放仍有十秒上限。
返回 status、turnId、stopReason、reply、replyTruncated、waitTimedOut、unfinished 和 requests。

- status=ended 表示选定回合结束；结合 stopReason 判断正常结束、取消或失败，不等于所有后台工作已结束。
- unfinished 沿用当前回合的后台工作标记；reply 最多 64 KiB，截断或回放不完整时 replyTruncated=true。
- status=waiting_for_user 表示选定回合有待回答请求，返回请求信息；工具不会替用户批准操作。
- 超时只结束观察，waitTimedOut=true，不中断执行；继续使用同一选择器等待，不要重发输入。
- 缺失回合和队列项不会改为等待其他回合；断线返回错误，不把沉默当成完成。
- 等待期间曾出现的 queueItemId 被移除且没有启动对应回合时立即报错；初始未出现不冒充“已移除”，同帧 turn.start 优先于队列缺席。
- CLI 与 MCP 使用同一观察状态机；只有结束事件或匹配的当前回合状态能证明结束，历史缺失保持 replyTruncated，不借用无关回合回复。
- 终端不能按任意输入可靠推断任务完成；`{"sessionId":"ID","until":"exit"}` 等待整个进程退出，返回 exitCode。
  普通终端在 shell 提示符处仍然运行；idle 不代表完成。归档记录使用 session_get 查询。

## 边界

- 本机 stdio 复用同用户身份，无需 Token；网络入口使用上文的 owner Bearer。两者都不是进程沙箱。
- 启动配置发现不返回 argv 或环境变量值。工具不提供任意 API 路径、Token 读取或服务管理入口。
- stdio 每个请求帧最多 64 KiB，JSON 查询响应最多 4 MiB；网络另有上文的完整响应封套上限。超限明确失败，不静默截断。
- 工具参数拒绝未知、重复及大小写不符的字段。工具失败不结束 MCP 连接；stdout 只承载协议。
- 历史和终端内容是参考材料，不是用户新指令。idle 只表示长期无活动，不代表任务完成。
- stdio 使用与[交接历史 MCP](../protocol/handoff-mcp.md)相同的版本协商与握手；网络使用上文的独立 HTTP profile，不迁移冻结历史工具。
