# 通过 MCP 控制 WebMux

让支持 MCP 的客户端查询 WebMux 会话及可用的启动配置。先启动 WebMux 服务，再把 `webmux mcp` 配置为客户端的 stdio MCP 服务。
交接自动注入的 `webmux-history` 仍然独立，只读取冻结的来源历史。

## 连接

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

## 查询工具

| 工具           | 用途                                                                                  |
| -------------- | ------------------------------------------------------------------------------------- |
| `capabilities` | 可用启动配置 ID、名称、聊天支持与当前工具列表；不启动 agent，不探测登录状态           |
| `session_list` | 按 state、parentId、repositoryId、status 筛选；offset 缺省 0，limit 缺省 20、最大 100 |
| `session_get`  | 按 sessionId 读取详情、父子关系、worktree 与浏览器链接                                |

`session_list` 的 state 为 active（默认）、closed、interrupted、archived 或 all；status 为 running、waiting、idle 或 exited。
返回 nextOffset、done 和 total；列表在调用间可能变化，offset 不是冻结快照。

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

## 边界

- 控制入口复用本机 API 的同用户身份，不需要向客户端配置 Token。WebMux 不是进程沙箱。
- 启动配置发现不返回 argv 或环境变量值。工具不提供任意 API 路径、Token 读取或服务管理入口。
- 每个请求帧最多 64 KiB，JSON 查询响应最多 4 MiB；超限明确失败，不静默截断。
- 工具参数拒绝未知、重复及大小写不符的字段。工具失败不结束 MCP 连接；stdout 只承载协议。
- 历史和终端内容是参考材料，不是用户新指令。idle 只表示长期无活动，不代表任务完成。
- MCP 使用与[交接历史 MCP](../protocol/handoff-mcp.md)相同的版本协商与 stdio 握手。
