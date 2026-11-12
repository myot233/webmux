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
  - 旧回合迟到的 cancelled 压缩按实际提交位置进入交接候选，保留原 Run 和取消状态；不会额外生成用户请求或携带推理；
  - 缺口使策略回到 full 时，已确认导入到同一目标的 Activity 仍不重复导入；目标持有历史成本，来源删除不清空占用，已有附件也计入保守用量；
  - 创建、读取与编辑的预览元数据列表各最多 100 项，并给出完整 selected/omitted 计数和截断标记；全部选择记录可通过认证的 selection 接口按 Activity position 恢复，不静默丢弃省略项；
  - `history_capture=false` 时，selection 分页与其他历史入口同样返回 503，不暴露已保存的来源路径、位置或选择明细；运行身份与未决派发保护仍保留；
  - 预算区分 provider 用量与保守估算，计入 UTF-8、JSON 转义、目标占用、附件和工作预留；小窗口与超大请求返回 over_budget，不截短用户原文；
  - PATCH 使用 revision，冲突返回 409；编辑跨服务重启保留，新活动或缺口只标 stale，不覆盖编辑；手工编辑只允许后续 inline；
  - 预览不依赖活跃 actor，不启动 agent 或创建 worktree；认证 messages API 可在同一 Conversation 切换 agent，或 fork 到新 Conversation 的 cwd、已有或新 worktree，浏览器从会话菜单提供“更换 Agent…”与“交接到新聊天…”入口，后者复用位置选择与工作树表单；
  - pending receipt 先于资源创建，重试与重启沿用 worktree 和会话身份；明确消费后标 inline，拒绝标 failed，结果不明标 unknown 且不自动重发；
  - 生成持久预览前，暂存附件复制到已验证来源 runtime 的私有 uploads；原暂存文件不被夺走，其他表单仍可使用。草稿引用跨服务重启保留，沿用来源 runtime 的保留、清扫与忘记生命周期，不保证抵御操作系统清理临时目录；
  - 同名但内容不同的暂存附件、普通来源上传与旧草稿副本可并存，不覆盖或拒绝合法新预览；同一暂存文件的相同内容幂等复用不可变副本；
  - 不同暂存文件即使字节相同仍保留独立引用与原文件名，可跨重启重新预览并完整投递；路径授权与 MIME、大小、类型来自实际文件，不能合并为保存后无法再验证的重复引用；
  - 投递附件复制到目标会话自己的私有上传目录，已完成副本不受来源记录删除或暂存区重启清理影响；回放恢复目标附件元数据，不能跨会话读取原路径；
  - 附件上传期间，所有重新预览入口均禁用；上传完成后重新预览保留新增附件，不被旧请求快照覆盖；
  - 任务留空时可仅上传图片或文件，预览后「发送交接」可用；无文字且无附件不能发送。上传和预览不产生 provider 请求，只有明确发送才投递一次，目标保留空任务与实际附件字节；
  - 忘记 fork 来源后，目标独立持有用户新请求、回放摘要与附件引用；重启后原生 load 仍只显示用户新请求，不暴露历史交接包，不重建已删除的来源记录；
  - 升级旧 v10 或重开已有 v11 时，即使目标已更换原生线程、来源已忘记，仍从保留目标事实修复历史派发 owner；未知调用保留旧 native 身份门禁，不放行替换 writer、不自动发送或重建来源历史；
  - 原生 load 只有精确 native ID 与 payload hash 匹配才确认 unknown；用户新请求独立显示，不把历史 marker 用作标题；明确失败可显式重试，旧命令结果不被覆盖；
  - 明确失败的交接允许把目标切到终端完成登录等恢复；恢复操作不自动重发或放行普通聊天请求，failed owner 在归档恢复和重启后仍保留，pending/unknown 重试也不能借终端绕过；
  - 切换在旧 agent 退场前因忙碌或队列任务被拒绝时，旧 agent 保持当前身份，归档恢复或重启后仍可继续；failed 恢复保护只归实际目标，pending/unknown 仍阻止来源绕过；
  - 同一 Conversation 可用原预览重试失败 attempt 自己产生的未消费历史，不改写已审阅正文、位置或 revision；其他 Run、Activity、gap 或修订变化仍冲突，终端恢复改变来源历史时必须重新预览并审阅；
  - A→B→A 保留同一 Conversation、cwd/worktree 和 A 的原生身份，只交接 A 未见的 Activity；迟到的旧 Run 结果按位置计入，已消费的 A 自身历史不重复导入；
  - A 较早拒绝或未消费的请求在后续请求成功后仍可进入返回 A 的交接草稿，不被较晚的确认位置跳过；成功投递交接后继续推进覆盖位置，不永久重复导入该请求；
  - 缺少原生恢复能力或上下文耗尽时生成 full 的 fresh 草稿，待用户审阅后单独投递；有未完成、后台工作或外部 writer 时拒绝切换，不自动强制关闭；
  - 删除旧 provider 的启动配置不阻止切换到有效配置；运行中的 provider 仍按原启动命令、agent 档案与环境检查占用并等待退出，归档 provider 缺少配置时用服务环境检查外部 writer，配置存储故障仍拒绝切换；
  - 切换拥有持久的单写者 owner，重复请求、其他设备和重启不产生第二个 writer；运行与归档记录提供稳定的 Conversation/ProviderThread 链接，nativeContinue 只表示真正的原生 resume。
  - pending/unknown 只允许同一原生身份的聊天端回放恢复，不允许改用终端端绕过；投递完成后，旧 provider 仍需走交接才能重新激活，普通恢复不能与当前 provider 并行写入。
  - 未编辑的草稿可在已核实的 Codex 版本中按原始角色注入历史，用户新请求单独发送；其他版本和 agent 使用 inline，手工编辑固定 inline；
  - 原生接受记录独立于用户 prompt，后续取消或失败不抹掉注入事实；原生结果不明时禁止回退或重注入，只有完整、有序且原生身份一致的 load 才能对账，导入项不再计为新的工作。
  - 原生注入确认丢失后忘记 fork 来源，目标仍可在重启后用完整有序回放确认注入及项目身份；重复 load 不产生回放缺口、不新增工作、不重新注入，也不重建来源投递或发送已删除来源的用户请求。

  - 发送与保存关闭先锁定编辑并等待保存；延迟旧轮询不回滚新 revision，来源暂时不可读或目标被删除不丢本设备未发送内容；
  - 来源仍在而 fork 目标恰在草稿发现期间被删除时，返回来源及空预览、空投递，不将目标预览消失误报为来源不存在，也不据此清除本设备未发送内容；
  - 交接弹窗挂载在时间线外；历史与新任务分别编辑和自动保存，关闭、刷新、跨页面与服务重启可恢复，不从 ChatModel 或快速开始记忆拼接历史；
  - 自动保存请求尚未返回时继续编辑任务或历史，后续变化仍会在停止输入后保存到服务端；新设备可读到最新文字，另一字段不被改写，保存不自动投递；
  - 三种新聊天位置明确区分来源 provenance 与目标位置；新工作树在首次发送时创建，失败保留草稿；
  - 并发 revision 冲突保留本地编辑并提供两种明确选择；过期、超预算、准备中、已投递、结果不明、失败都有可见状态，unknown 不允许重复发送；
  - 已有预览的未保存编辑保留原始 revision 基准；服务端升级 revision 后，重复刷新、关闭和重开仍显示冲突，不自动保存或覆盖另一设备内容。明确读取服务端版本或重新预览后清除旧基准，之后的新冲突使用新的 revision；
  - 本地 revision 冲突只暂停远端预览的自动采纳，不停止投递状态查询；pending/unknown 后续完成时原弹窗解除投递锁，仍保留本地正文和恢复基准，允许明确选择重新预览，不自动 PATCH、重发或放弃编辑；
  - 明确发送直接返回已完成的 inline/injected 时，与轮询完成一致清空下一次交接的任务、附件、目标和模型；原始已投递预览、Activity 与附件副本不变。等待投递结果期间产生的更新未发送编辑不被清空，pending/unknown 仍禁止重复投递；
  - 首次预览前已填写的本地任务、目标、模型、附件与新聊天位置不会被另一设备的服务端预览覆盖；关闭、刷新后仍保留原 choice 备份与明确冲突选择，不伪造历史 revision。仅来源默认位置不算未发送编辑；
  - 此类冲突先只读展示另一设备历史；明确「读取服务端版本」才替换本地选项，「保留编辑重新预览」保留本地内容并重新选择历史，不冒用另一设备的手工正文；重新预览不投递，仍需单独确认发送。
  - 历史恢复：目标 ProviderThread 的 new/load/resume 配置只读 `thread_read`；可读取 selector 省略的冻结来源 Activity，默认每页 20 项、每项 4000 字节。长项按字节 offset 继续，中文/emoji 保持完整；来源删除或工具不可用时显示能力缺失，交接预算与 REST/UI 仍可使用。
  - 浏览器覆盖统计使用完整选择/省略数量；省略历史按冻结位置分页读取选择元数据，可继续到预览内嵌列表之外。预览 revision 变化时重置读取状态，不混合新旧草稿的省略项。
  - 历史恢复先读有界 scope 与选择元数据，再读正文；约 40000 个省略 ID 不会让小页或单项读取在历史分页前触发 4 MiB 上限，来源与冻结位置授权保持不变。
  - 刷新历史工具 scope 时取消，排队中的准入不丢失写者所有权；等待旧 provider 退出期间取消仍完成 provider 与 MCP helper 清理，清除打开中状态，不重开 provider 或发送用户请求，正常关闭保留中断记录。

依据：

- [输入卡片变形动画](../../../web/src/hooks/shared/composer-morph.svelte.ts)、[抽屉入口动画](../../../web/src/lib/components/ui/drawer/drawer-content.svelte)。

- [混合会话服务](../../../internal/hybrid/)、[连接器检查](../../../internal/hybrid/connector.go)、[agent 适配器核心](../../../internal/agentprofile/)、[ACP 客户端](../../../internal/acp/)
- [agent 注册表](../../../internal/agentprofile/agents/agents.go)、[Codex 适配器](../../../internal/agentprofile/codex/codex.go)、[适配器契约测试](../../../internal/agentprofile/agenttest/agenttest.go)
- [Claude Code 适配器](../../../internal/agentprofile/claudecode/claudecode.go)、[Claude 注册记录](../../../internal/agentprofile/claudecode/registry.go)、[Antigravity CLI 适配器](../../../internal/agentprofile/agy/agy.go)
- [会话外壳与伴随进程](../../../internal/session/lifecycle.go)、[进程事实](../../../internal/session/process_facts.go)、[聊天接口](../../../internal/api/chat.go)、[聊天通道](../../../internal/hub/chat.go)
- [启动配置](../../../internal/launch/types.go)、[聊天视图](../../../web/src/components/chat/)、[代码链接解析](../../../web/src/domains/code/markdown-link.ts)。
- [代码工作区](../../../web/src/components/code/CodeWorkspace.svelte)、[会话页](../../../web/src/pages/Terminal.svelte)。
- [已归档记录](../../../web/src/components/sessions/ArchivedSessions.svelte)、[归位规则](../../../web/src/domains/sessions/archived.ts)、[安装提示](../../../web/src/components/sessions/ConnectorInstallHint.svelte)、[原生附着](../../../internal/attach/attach.go)
- [使用说明](../../hybrid-sessions.md)、[协议](../../protocol/http-hybrid.md)、[创建会话协议](../../protocol/http-session-create.md)、[端到端测试](../../../web/test/e2e/hybrid.spec.ts)。
- [会话卡片](../../../web/src/components/sessions/SessionRow.svelte)、[会话页头部](../../../web/src/components/terminal/TerminalHeader.svelte)、[品牌图标](../../../web/src/components/settings/LaunchConfigIcon.svelte)、[图标一致性测试](../../../internal/launch/presets_test.go)
- [快速开始页](../../../web/src/pages/QuickStart.svelte)、[快速开始状态](../../../web/src/hooks/sessions/quick-start.ts)。
- [创建编排](../../../internal/workspace/create_session.go)、[会话接口](../../../internal/api/sessions.go)、[按视图输入](../../../internal/hybrid/input.go)、[配置项清单](../../../internal/hybrid/options.go)
- [聊天历史](../../../web/src/components/chat/ChatHistory.svelte)、[快速开始浏览器用例](../../../web/test/browser/QuickStart.test.ts)、[聊天历史浏览器用例](../../../web/test/browser/ChatHistory.test.ts)、[快速开始端到端用例](../../../web/test/e2e/quick-start.spec.ts)。

- [手机会话选项面板](../../../web/src/components/chat/ChatSessionOptions.svelte)、[模型可搜索列表](../../../web/src/components/chat/ChatChoiceCommandList.svelte)、[共用输入卡片](../../../web/src/components/chat/ComposerCard.svelte)；
- [聊天浏览器用例](../../../web/test/browser/ChatView.test.ts)、[布局验收](../../../web/test/storybook/acp-chat.spec.ts)。

- [对话事务存储](../../../internal/conversation/)、[私有数据库生命周期](../../../internal/statefile/database.go)。

- [持久历史接口](../../protocol/http-conversation.md)、[历史路由](../../../internal/api/conversation.go)、[portable projection](../../../internal/conversation/projection.go)。

- [交接预览](../../protocol/http-handoff.md)、[预算与选择](../../../internal/handoff/)、[预览接口](../../../internal/api/handoff.go)。
