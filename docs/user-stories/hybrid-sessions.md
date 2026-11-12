# 混合会话

## Q. 混合会话（聊天 ↔ 终端）

按使用目的分组，故事 ID 保持稳定，验收要点原文迁入子页。

- [模式与时间线](hybrid-sessions/mode-and-timeline.md)：US-Q01–US-Q07。
- [命令、生命周期与附件](hybrid-sessions/lifecycle-and-commands.md)：US-Q08–US-Q14。
- [编写、历史与交接](hybrid-sessions/composition-and-handoff.md)：US-Q15–US-Q17、US-Q19–US-Q20。

- [快速开始聊天](hybrid-sessions/quick-start.md)：US-Q18，工作树与首条消息编排。

依据：

- [输入卡片变形动画](../../web/src/hooks/shared/composer-morph.svelte.ts)、[抽屉入口动画](../../web/src/lib/components/ui/drawer/drawer-content.svelte)。

- [混合会话服务](../../internal/hybrid/)、[连接器检查](../../internal/hybrid/connector.go)、[agent 适配器核心](../../internal/agentprofile/)、[ACP 客户端](../../internal/acp/)
- [agent 注册表](../../internal/agentprofile/agents/agents.go)、[Codex 适配器](../../internal/agentprofile/codex/codex.go)、[适配器契约测试](../../internal/agentprofile/agenttest/agenttest.go)
- [Claude Code 适配器](../../internal/agentprofile/claudecode/claudecode.go)、[Claude 注册记录](../../internal/agentprofile/claudecode/registry.go)、[Antigravity CLI 适配器](../../internal/agentprofile/agy/agy.go)
- [会话外壳与伴随进程](../../internal/session/lifecycle.go)、[进程事实](../../internal/session/process_facts.go)、[聊天接口](../../internal/api/chat.go)、[聊天通道](../../internal/hub/chat.go)
- [启动配置](../../internal/launch/types.go)、[聊天视图](../../web/src/components/chat/)、[代码链接解析](../../web/src/domains/code/markdown-link.ts)、[代码工作区](../../web/src/components/code/CodeWorkspace.svelte)、[会话页](../../web/src/pages/Terminal.svelte)
- [已归档记录](../../web/src/components/sessions/ArchivedSessions.svelte)、[归位规则](../../web/src/domains/sessions/archived.ts)、[安装提示](../../web/src/components/sessions/ConnectorInstallHint.svelte)、[原生附着](../../internal/attach/attach.go)
- [使用说明](../hybrid-sessions.md)、[协议](../protocol/http-hybrid.md)、[创建会话协议](../protocol/http-session-create.md)、[端到端测试](../../web/test/e2e/hybrid.spec.ts)。
- [会话卡片](../../web/src/components/sessions/SessionRow.svelte)、[会话页头部](../../web/src/components/terminal/TerminalHeader.svelte)、[品牌图标](../../web/src/components/settings/LaunchConfigIcon.svelte)、[图标一致性测试](../../internal/launch/presets_test.go)
- 快速开始：[页面](../../web/src/pages/QuickStart.svelte)、[状态](../../web/src/hooks/sessions/quick-start.ts)、[创建编排](../../internal/workspace/create_session.go)；
  [会话接口](../../internal/api/sessions.go)、[按视图输入](../../internal/hybrid/input.go)、[配置项清单](../../internal/hybrid/options.go)。
- [聊天历史](../../web/src/components/chat/ChatHistory.svelte)、[快速开始浏览器用例](../../web/test/browser/QuickStart.test.ts)、[聊天历史浏览器用例](../../web/test/browser/ChatHistory.test.ts)、[快速开始端到端用例](../../web/test/e2e/quick-start.spec.ts)。

- [手机会话选项面板](../../web/src/components/chat/ChatSessionOptions.svelte)、[模型可搜索列表](../../web/src/components/chat/ChatChoiceCommandList.svelte)、[共用输入卡片](../../web/src/components/chat/ComposerCard.svelte)；
- [聊天浏览器用例](../../web/test/browser/ChatView.test.ts)、[布局验收](../../web/test/storybook/acp-chat.spec.ts)。

- [对话事务存储](../../internal/conversation/)、[私有数据库生命周期](../../internal/statefile/database.go)。

- [持久历史接口](../protocol/http-conversation.md)、[历史路由](../../internal/api/conversation.go)、[portable projection](../../internal/conversation/projection.go)。

- [交接预览](../protocol/http-handoff.md)、[预算与选择](../../internal/handoff/)、[预览接口](../../internal/api/handoff.go)。
