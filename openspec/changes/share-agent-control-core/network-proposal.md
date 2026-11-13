# Proposal

## Why

[#577](https://github.com/mzdluo123/webmux/issues/577) 需要让不安装 WebMux 二进制、不持有本机 socket 的外部 MCP 客户端，通过实例 URL 控制既有会话。
现有控制 MCP 已经通过 ACP 注入 stdio；本变更只增加受保护的网络传输，不以未经验证的 MCP-only 需求替代 CLI，也不承诺完整 CLI/MCP 能力对等。

## What Changes

- 在既有 TCP HTTP 监听增加单一 `/mcp`，以 `[mcp] enabled = false` 默认关闭；不新增监听端口、Unix/P2P 入口或旧 HTTP+SSE 端点。
- 启用时必须已解析出非空共享 owner Token；空 Token 直接使启动失败，不为 MCP 自动生成或获取秘密。网络 MCP 始终只接受 Bearer header，沿用 Host/Origin 校验和已有可信反向代理来源。
- 明确该 Token 代表实例 owner，权限等同既有 REST，包括以服务用户身份启动任意 argv；工具集较窄不等于凭据权限较窄，也不是沙箱或按会话授权。
- 在共享 `mcpprotocol` 上增加 `mcphttp`，复用 `mcpcontrol` handlers、schema、结果策略与 `sessionobserve`；用真实 authority、认证后的本实例 TCP REST/WS 调用，不以服务端 PID 经 Unix socket 代理远程身份。
- 固定 MCP `2025-11-25` initialize 协议，支持已核实的 HTTP `2025-03-26` / `2025-06-18` 协商；JSON POST 回应、通知 202、GET 405，无 SSE 或响应重放缓存。
- 使用有界、内存态 MCP 会话跟踪初始化、并发与取消；区分断线、显式取消、观察超时和已提交写入，保持业务 `clientRequestId` 去重窗口不变。
- 使用前置 guide 变更的 core/context 与 `guide_read`，明确远程 self/parent 未知、文件路径属于服务端；不新增 answer/view/upload 等工具，不把 `thread_read` 或冻结交接历史开放到网络。
- 本提交只提供设计与未执行任务；HTTP Bearer profile 不实现标准 OAuth 授权发现/授权服务器，不能宣称支持仅接受 OAuth 的托管客户端。

## Capabilities

### New Capabilities

- `mcp-network-transport`：可选网络 MCP 的 TCP 接入、owner 认证与身份、HTTP 协议、有界会话、并发取消和重连边界。

### Modified Capabilities

无。`server-local-api` 的 Unix 身份和 `/api/*`、`/ws/*` 等价范围保持不变；`access-link` 的浏览器片段认证不适用于 MCP。
共享工具语义由前置 `mcp-session-control` 负责，本变更只增加传输和远程身份约束。

## Impact

- 顺序依赖：[共享控制核心](proposal.md) → [共享操作指南](../share-agent-operation-guide/proposal.md) → 本网络扩展；每一层实现必须形成可用的完整切换，不交付 handler 桩或半迁移 PR。
- 预计影响 `internal/config`、`internal/localclient`、`internal/api`、`internal/mcpcontrol`、共享 `mcpprotocol`、新增 `mcphttp`、`cmd/webmux` 装配与架构清单；不引入新的常驻服务或第三方服务端 SDK。
- 实现时更新 MCP 专题说明、协议和安全部署边界，以及 US-A02、US-A03、US-K09、US-S20–S23；新增网络接入故事时同步故事索引、覆盖映射和总数。本设计阶段不把故事标记为已实现。
- 外部兼容性验收指定支持自定义 Authorization header 的官方 MCP Inspector，使用 URL、Bearer 与 initialize-era 配置，不依赖本机 WebMux 命令或 bridge；不宣称任意 MCP 托管平台均兼容。
