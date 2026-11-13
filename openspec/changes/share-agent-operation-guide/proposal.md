# Proposal

## Why

#579 指出的六类操作说明会让 agent 把重试当新请求、把回合结束当任务完成，或错误推断授权与文件能力。现有 shell 手册和 MCP instructions 分开维护，需要一个有明确调用上下文的共享操作指南，而不是复制整本 shell 手册。

## What Changes

- 在现有 `internal/agentguide` 提供公开的上下文输入与指南渲染核心，统一初始化说明、CLI 手册与 MCP instructions 的安全语义；CLI 命令目录继续从命令表生成。
- 修正逻辑操作 ID、去重窗口、四类操作证据及独立业务验收、真实授权 optionId、版本核对、配置与模型发现、交付目录生命周期六类说明。
- 给 control MCP 增加只读 `guide_read`，支持索引与单主题读取；按实际注册工具和本地/远程上下文筛选内容，不承诺完整 CLI/MCP 对等。
- CLI 现有等待结果对象仅增加顶层 unfinished boolean，true 时文本 stderr 提示后台工作未完成，保留 events 数组与全部现有退出映射。
- 仅规划实现与 US-S05/S06/S08/S09/S11/S14/S20-S23 的回归更新，本变更不修改用户故事或产品实现。

## Capabilities

### New Capabilities

无。指南本身已有稳定能力边界；control 生命周期使用下层变更的 `mcp-session-control`，不重复声明。

### Modified Capabilities

- `agent-shell-guide`：共享操作语义、上下文隔离与按工具能力裁剪的 MCP 指南读取。
- `shell-chat-control`：显式输出真实 unfinished 标记，保留现有结果对象、事件和退出码，不混同业务验收。

## Impact

依赖下层 [share-agent-control-core](../share-agent-control-core/proposal.md) 的 observer 与 protocol 边界，不修改其文件。影响 agentguide、CLI guide/等待适配器、MCP control instructions/工具、对应单元与真实 CLI/fake-agent 测试。HTTP MCP
可在后续上层变更使用同一接口，但本变更不实现网络传输、认证或远程文件能力。保留已由 ACP 注入的 stdio；#578 的定位不构成已经证实的 MCP-only 需求。freeze-history 的 MCP registry 与操作指南保持独立。
