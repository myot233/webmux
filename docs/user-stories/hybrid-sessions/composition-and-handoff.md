# 编写、历史与交接

- **US-Q15 聊天附件**：作为聊天端使用者，我希望图片和文件随消息以 agent 原生内容发送，以便 agent 直接看到素材而不必自己读取路径。
  验收要点：
  - 聊天和快速开始共用的待发送附件列表在新增时淡入，删除后其余标签平滑就位；换行或清空时输入卡片随列表高度变化，系统减少动态效果时直接更新，动画不延迟移除或发送；
  - 发送后用户气泡显示附件（图片附件显示缩略小图与文件名，无法读取时回退为图片图标；其他文件显示文件图标与文件名）；
  - `state.promptCapabilities` 取自 ACP `initialize`，声明 `image` 时 PNG/JPEG/GIF/WebP 以 base64 `image` 块发送且字节一致，声明 `embeddedContext` 时其他附件以文本或 blob `resource` 块发送，否则按服务器路径降级并在时间线提示，图片能力不足时输入框提前提示；
  - Claude 连接器 0.99.1 声明图片与嵌入资源并复用同一管线，但不把该实测能力推广为未来 `latest` 或所有 MIME 类型的保证；
  - 队列条目跨设备带附件元数据，编辑文本保留附件，切到终端时附件路径并入退回的草稿，重启不持久化附件；
  - 只接受上传目录内、非符号链接的普通文件（临时目录本身可以是符号链接，如 macOS `/var`），单文件 10 MiB、单条最多 8 个且合计 20 MiB，越界或超限在读取前拒绝。
- **US-Q16 第三方连接器安装提示**：作为使用者，我希望在 agent 的聊天端需要另外安装的连接器缺失时得到明确的安装指引，以便知道该装什么而不是只看到“启动失败”。
  验收要点：
  - 档案声明的连接器（Codex 的 `codex-acp`、Claude Code 的 `claude-agent-acp`）在每次启动 ACP 进程前（资格探测、从聊天开始、切到聊天、恢复到聊天、重连）按启动配置的写法检查（直接执行查启动配置环境的 PATH，“加载 shell 启动文件”经同一 Bash/Zsh 包装执行 `command -v`）；
  - 未安装时不启动 ACP 进程，返回 409 `connector_missing` 并附 `{executable, title, installCommand, docsUrl}`，从聊天开始不创建会话与记录；
  - 终端端照常使用；
  - 会话页禁用“聊天”并显示对应“未安装 Codex ACP 适配器（codex-acp）”或“未安装 Claude Code ACP 适配器（claude-agent-acp）”与可关闭的安装提示条，安装后 30 秒内自动重新检测；
  - 新建会话弹层在“从聊天开始”旁预先说明，失败时保留输入并提供“改为从终端开始”；
  - 聊天视图在重连失败时显示安装提示与“切到终端”；
  - 设置页在 Codex 或 Claude 配置的勾选框旁显示可复制的安装命令与说明链接；
  - Claude 明确要求 Node.js 22+，安装命令 `npm install -g @agentclientprotocol/claude-agent-acp` 跟随 `latest`，实测 Claude Code 2.1.316/连接器 0.99.1 只作版本提示而非最低版本或未来兼容承诺；
  - webmux 不自动安装连接器或 Node，也不用 `npx` 临时下载。
- **US-Q17 识别聊天端 agent**：作为同时开着多个聊天会话的使用者，我希望在首页和聊天页都能一眼看出每个会话由哪个 agent 承接，以便不用切到终端或凭标题猜测。
  验收要点：
  - 混合会话的会话信息 `agent` 对象带档案名 `name`、档案声明的显示名 `title` 与品牌图标键 `icon`（属于启动配置图标目录），列表快照、列表事件与单个会话信息一致；普通会话 `agent` 为 `null`；前端不内置 agent 名称；
  - 首页混合会话卡片（聊天端、切换中与终端端，含“恢复到终端”后）以 agent 单色品牌小图标替换通用聊天/终端图标，元信息行形如“OpenCode · 聊天 · 2 个设备已连接”或“OpenCode · 终端 · 后台会话”；
  - 会话页显示聊天视图时，头部在标题后、状态点前显示低强调的图标与名称；窄屏（小于 640px）只显示图标，可访问名称与悬停提示均为“Agent：<名称>”，空间不足时先截断标题；
  - 标识只用于展示，不可聚焦或点击；切到终端视图后隐藏；浏览器不认识的图标键只显示名称，不用其他图标代替。
- **US-Q19 聊天历史**：作为同时进行多个聊天的使用者，我希望聊天模式的侧栏用稳定的顺序列出全部聊天，以便不回首页就切换聊天或恢复已结束的聊天，且行不会因为输出而跳动。
  验收要点：
  - 运行中的混合会话在上方平铺，顺序稳定：
    - 按创建时间从新到旧，创建时间相同按会话 ID 排序，与工作台树一致；
    - 输出、状态变化与改名都不改变行的位置，新建的聊天出现在最上面；
  - 服务端保存的已归档会话（`GET /api/sessions?state=archived`）收在下方的“已归档”折叠分组中：
    - 默认折叠并显示数量，当前打开或正在恢复已归档记录时自动展开；
    - 折叠时不在页面中挂载归档行；展开后先挂载最近 20 条，其余经“显示更多（还有 N 条）”按 20 条一批追加，收起后重新从首批开始，历史很长时也不会一次渲染全部行；
    - 按最近更新从新到旧排列，更新时间相同按 ID 排序；该时间只在归档、中断或编辑记录时变化；
  - 纯终端会话不列出，同一会话只出现一次，随会话事件实时更新；
  - 每行显示 agent 图标、标题和状态标记（彩色圆点：工作中、等待、空闲，空心圆点：已结束；已释放空间的已归档记录改为空心三角形，侧栏行不另显“已释放空间”文字徽章，原路径与提交在悬停提示中，文字在可访问名与悬停提示中），运行中与已归档的行标记在行尾右对齐，当前会话标 `aria-current`；
  - 点运行中的行在当前标签页进入该会话；点已归档记录按归档前最后使用的聊天或终端模式直接恢复并进入，不受聊天/工作台侧栏模式影响，也不改变侧栏模式；
    - 需要选择其他 agent 会话或确认缺失配置时转到该记录的恢复页，重试仍沿用原模式；
    - 已释放空间的记录先确认重建 worktree（US-J13），无法恢复时在侧栏说明原因、不新建会话；
  - “新聊天”回到快速开始页；手机上从抽屉中选择后抽屉关闭。

- **US-Q20 预览可携带的交接上下文**：作为切换 agent 的使用者，我希望检查服务端选择的历史与预算并编辑草稿，以便保留重要约束和近期结果。
  验收要点：
  - 认证 API 固定来源位置、目标配置、用户请求和附件引用；最近 user、最近 agent、首条 user 优先，其余从近到远尝试，去重后恢复原顺序；
  - 历史整项保留或省略，响应列出省略 ID 与原因、覆盖 Run 范围、来源完整性和历史读取引用；
  - 预算区分 provider 用量与保守估算，计入 UTF-8、JSON 转义、目标占用、附件和工作预留；小窗口与超大请求返回 over_budget，不截短用户原文；
  - PATCH 使用 revision，冲突返回 409；编辑跨服务重启保留，新活动或缺口只标 stale，不覆盖编辑；手工编辑只允许后续 inline；
  - 预览不依赖活跃 actor，不启动 agent 或创建 worktree；认证 messages API 可将 fork 预览投递到新 Conversation 的 cwd、已有或新 worktree，浏览器入口后续接入；
  - pending receipt 先于资源创建，重试与重启沿用 worktree 和会话身份；明确消费后标 inline，拒绝标 failed，结果不明标 unknown 且不自动重发；
  - 原生 load 只有精确 native ID 与 payload hash 匹配才确认 unknown；用户新请求独立显示，不把历史 marker 用作标题；明确失败可显式重试，旧命令结果不被覆盖。

依据：

- [输入卡片变形动画](../../../web/src/hooks/composer-morph.svelte.ts)、[抽屉入口动画](../../../web/src/lib/components/ui/drawer/drawer-content.svelte)。

- [混合会话服务](../../../internal/hybrid/)、[连接器检查](../../../internal/hybrid/connector.go)、[agent 适配器核心](../../../internal/agentprofile/)、[ACP 客户端](../../../internal/acp/)
- [agent 注册表](../../../internal/agentprofile/agents/agents.go)、[Codex 适配器](../../../internal/agentprofile/codex/codex.go)、[适配器契约测试](../../../internal/agentprofile/agenttest/agenttest.go)
- [Claude Code 适配器](../../../internal/agentprofile/claudecode/claudecode.go)、[Claude 注册记录](../../../internal/agentprofile/claudecode/registry.go)、[Antigravity CLI 适配器](../../../internal/agentprofile/agy/agy.go)
- [会话外壳与伴随进程](../../../internal/session/lifecycle.go)、[进程事实](../../../internal/session/process_facts.go)、[聊天接口](../../../internal/api/chat.go)、[聊天通道](../../../internal/hub/chat.go)
- [启动配置](../../../internal/launch/types.go)、[聊天视图](../../../web/src/components/chat/)。
- [代码链接解析](../../../web/src/markdown-link.ts)、[代码工作区](../../../web/src/components/CodeWorkspace.svelte)、[会话页](../../../web/src/pages/Terminal.svelte)
- [已归档记录](../../../web/src/components/ArchivedSessions.svelte)、[归位规则](../../../web/src/archived.ts)、[安装提示](../../../web/src/components/ConnectorInstallHint.svelte)、[原生附着](../../../internal/attach/attach.go)
- [使用说明](../../hybrid-sessions.md)、[协议](../../protocol/http-hybrid.md)、[创建会话协议](../../protocol/http-session-create.md)、[端到端测试](../../../web/test/e2e/hybrid.spec.ts)。
- [会话卡片](../../../web/src/components/SessionRow.svelte)、[会话页头部](../../../web/src/components/TerminalHeader.svelte)、[品牌图标](../../../web/src/components/LaunchConfigIcon.svelte)、[图标一致性测试](../../../internal/launch/presets_test.go)
- [快速开始页](../../../web/src/pages/QuickStart.svelte)、[快速开始状态](../../../web/src/quick-start.ts)。
- [创建编排](../../../internal/workspace/create_session.go)、[会话接口](../../../internal/api/sessions.go)、[按视图输入](../../../internal/hybrid/input.go)、[配置项清单](../../../internal/hybrid/options.go)
- [聊天历史](../../../web/src/components/ChatHistory.svelte)、[快速开始浏览器用例](../../../web/test/browser/QuickStart.test.ts)、[聊天历史浏览器用例](../../../web/test/browser/ChatHistory.test.ts)、[快速开始端到端用例](../../../web/test/e2e/quick-start.spec.ts)。

- [手机会话选项面板](../../../web/src/components/chat/ChatSessionOptions.svelte)、[模型可搜索列表](../../../web/src/components/chat/ChatChoiceCommandList.svelte)、[共用输入卡片](../../../web/src/components/chat/ComposerCard.svelte)；
- [聊天浏览器用例](../../../web/test/browser/ChatView.test.ts)、[布局验收](../../../web/test/storybook/acp-chat.spec.ts)。

- [对话事务存储](../../../internal/conversation/)、[私有数据库生命周期](../../../internal/statefile/database.go)。

- [持久历史接口](../../protocol/http-conversation.md)、[历史路由](../../../internal/api/conversation.go)、[portable projection](../../../internal/conversation/projection.go)。

- [交接预览](../../protocol/http-handoff.md)、[预算与选择](../../../internal/handoff/)、[预览接口](../../../internal/api/handoff.go)。
