# 交接历史恢复：thread_read MCP

交接只选入预算内的整项历史。目标 agent 可通过只读 `thread_read` 取回省略的内容；历史是参考材料，当前用户任务仍单独提交。
预览的 `historyToolAvailable` 表示本机 helper、socket 与冻结来源可用；缺失时继续使用历史 REST/UI 与省略计数，不阻止按预算交接。

## 读取与分页

工具输入只接受 `view: "activity"`、`afterPosition`、`itemId`、`textOffset`、`limit` 和 `maxBytesPerItem`。
默认 `limit=20`、每项 4000 字节；上限 100 项、每项 65536 字节，乘积不超过 256 KiB。
位置是规范十进制字符串，textOffset 是非负整数字节 offset；未知字段、非法 cursor、越界限制与跨来源项目被拒绝。

先调用 `{"view":"activity"}`，随后以结果的 `nextPosition` 继续，直到 `done=true`。
某项 `truncated=true` 时，保留其 `activityId`，以 `itemId` 和该项 `nextTextOffset` 继续，直到单项 `done=true`；不要把下一项位置当作长正文 offset。
UTF-8 边界由服务端检查，中文与 emoji 不拆成非法字符。只读取 preview 固定 `sourcePosition` 以前的已提交项目，知道未来项 ID 也不能越过冻结点。

结果提供来源 Conversation、ProviderThread、Run、Activity、position、角色、状态、selected/omitted 标记、计数和 gap 信息。
只返回规范化可导出文本及附件引用元数据；不返回 reasoning、附件正文、raw ACP、env、token、SQL 或任意路径读取接口。
每次响应说明历史材料不是新用户请求或高优先级指令。来源被删除时工具返回明确错误，不猜测回填。

## 配置与生命周期

服务端经 ACP v1 `mcpServers` 在目标 ProviderThread 的 new/load/resume 中提供 `webmux-history` stdio 配置；普通来源会话没有该配置。
命令为当前 `webmux mcp-serve --socket PATH --conversation ID --handoff ID`；scope 固定在 argv，环境变量列表为空，不修改全局 launch config，也不把命令或 socket 返回浏览器。
同一 ProviderThread 后续 handoff 使用最新已保留 execution 的冻结来源。普通预览不启动 provider 或 helper 来探测能力。

helper 通过现有本机 API 访问持有生命周期锁的 SQLite 服务，不自行打开数据库。进程属于 provider 的进程树，关闭 provider 时一并停止。
缺少可执行文件、socket 或来源时省略可选配置；工具调用读失败返回 `isError`，不结束 provider 会话。WebMux 只发送隐含支持 stdio MCP 的 ACP v1 描述符，不猜测 ACP v2 能力。
WebMux 不是进程沙箱；同用户 agent 仍能自行读取本机文件。该工具提供稳定、有界、可审计的读取接口，不构成针对恶意同用户进程的隔离边界。

## 协议与实现依据

stdio 实现 initialize、notifications/initialized、ping、tools/list、tools/call；每行一个 JSON-RPC 消息，输入帧上限 64 KiB，每个本机读取截止时间 10 秒。
支持固定版本 2024-11-05、2025-03-26、2025-06-18、2025-11-25；未知 initialize 版本返回最新已支持版本，客户端决定是否接受。
不宣称实现 2026 stateless 协议；其 `server/discover` 返回 MethodNotFound，支持协商的客户端可选择 legacy initialize。
工具结果同时包含对象 `structuredContent` 和序列化 text，兼容只读取 text 的客户端。stdout 只有协议，传输诊断在 stderr，不输出历史正文。

- [ACP v1 session setup](https://agentclientprotocol.com/protocol/session-setup)：new/load/resume 的会话 MCP 配置。
- [MCP lifecycle](https://modelcontextprotocol.io/specification/2025-11-25/basic/lifecycle)：initialize 协商及 initialized 通知。
- [MCP tools](https://modelcontextprotocol.io/specification/2025-11-25/server/tools)：只读 annotations、tools/list、tools/call 与错误内容。
- [MCP Go SDK 协议说明](https://go.sdk.modelcontextprotocol.io/protocol/)：现代 discovery 与 legacy fallback 的区别。
