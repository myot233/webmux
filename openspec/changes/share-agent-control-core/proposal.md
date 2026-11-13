# Proposal

## Why

CLI 与本地 MCP 已共用 REST/WebSocket API，但分别解释回合、队列和进程退出，已出现排队输入被删除后 MCP 等到超时的行为漂移。
[#578](https://github.com/mzdluo123/webmux/issues/578) 要求明确入口定位；[#579](https://github.com/mzdluo123/webmux/issues/579) 与 [#577](https://github.com/mzdluo123/webmux/issues/577) 需要稳定的共享语义和传输边界。

## What Changes

- 保留 CLI、本地 stdio MCP；前者面向 shell/脚本，后者提供工具发现、参数 schema、结构化回执和游标，不宣称已有经过验证的 MCP-only 客户端需求。
- 提取 `sessionobserve`，供 CLI 与 MCP 共同解释指定回合、排队项、待回答请求、后台工作和进程退出；不把静默当成完成。
- 修复 MCP 在已看到排队项后未检测后续删除的问题；显式处理回放缺失，禁止改等别的回合。
- 从 `mcpstdio` 提取传输无关的 `mcpprotocol` 注册表、生命周期与分派，stdio 只保留逐行输入输出。
- 保留适配层的超时、回复大小、事件输出、退出码和错误展示；不把 CLI 改为 MCP 客户端。
- 交接历史 MCP 只复用协议机制，工具注册表和冻结来源权限仍独立。
- 在核心迁移与独立指南变更完成后，增加默认关闭的 Streamable HTTP `/mcp`；共享 owner Bearer 始终认证，不继承服务进程的本机身份。
- 网络扩展复用既有工具、明确远程 self/parent 未知、有界并发与取消；不实现 OAuth、文件传输或扩大冻结历史范围。
- 本 PR 仅新增设计、delta specs 与未执行的实现任务，不修改运行行为；后续实现必须整批迁移每层消费者并删除旧实现。

## Capabilities

### New Capabilities

- `mcp-session-control`：规定已有 MCP 控制工具的可观察边界、正确回合定位、队列移除、完整性和超时结果，以及控制与冻结历史的隔离。
- `mcp-network-transport`：规定默认关闭的认证 TCP 入口、owner 身份、HTTP 会话、并发取消、重连与外部客户端兼容验收。

### Modified Capabilities

无。现有 CLI 规格不因纯协议提取而重写；CLI 的 `unfinished` 输出增量由后续 `share-agent-operation-guide` 变更负责。

## Impact

- 目标代码：`internal/clicmd/turn_stream.go`、`streams.go`、`chat.go`、`internal/mcpcontrol/wait.go`、`internal/mcpstdio/server.go` 与 `internal/threadread/server.go`。
- 新包只依赖共享契约和客户端层；在 `internal/archtest/layers_test.go` 登记，不直接导入 Service 或 Controller。
- 服务端继续拥有请求去重、输入回执、权限与持久会话生命周期；不增加持久状态、第三方运行依赖或通用重试缓存。
- 本设计阶段没有已实现能力变化，不更新用户故事为已完成；实现修复时更新 US-S23，保持 US-S09、US-S14 与 US-S20–S22 的现有边界。
- [设计与迁移边界](design.md)、[MCP 控制规格](specs/mcp-session-control/spec.md)、[逐层实现任务](tasks.md)。
- 网络层的独立评审材料：[网络提案](network-proposal.md)、[传输与身份设计](network-design.md)、[网络规格](specs/mcp-network-transport/spec.md) 与 [详细任务](network-tasks.md)。
- 为遵守 changes 直接子目录上限，网络层扩展此 change，而非新增第三个顶层 change；它只依赖已落地的核心分组和指南，不让指南实现反向依赖网络。
- 网络实现涉及配置、认证 TCP client、HTTP adapter 与 cmd/API 装配；新增故事拟用 US-S25，并同步 US-A02/A03/K09/S20–S23，未实现前不更新故事目录或计数。
