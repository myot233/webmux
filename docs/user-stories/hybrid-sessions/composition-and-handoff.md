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
- **US-Q18 快速开始聊天**：作为想直接把任务交给 agent 的使用者，我希望在一个输入框写下任务、选好仓库和 agent 后按一次发送，就在新 worktree 或所选已有工作树里以聊天方式开工，以便省去建 worktree、选配置和等待聊天就绪这几步。
  验收要点：
  - 桌面快速开始页 `/new` 是聊天模式的主区域：底部输入框，上方一行是仓库与工作树方式（新建 worktree 的基准分支，或已有工作树），下方一行是 agent、该 agent 的配置项（模型配置项同样支持文本搜索）与权限模式。下拉按钮显示当前选项，空间不足时可横向滚动到两端；
  - 工作树方式弹出面板顶部提供“新建 worktree | 已有工作树”分段切换：
    - 选择“新建 worktree”时显示加号图标与基准分支名（`基准分支：<名称>`），下方以可搜索列表按名称选择本地分支或远程跟踪分支（如 `origin/*`），指向同一提交的分支互不影响；
    - 选择“已有工作树”时显示分支图标与工作树名称（`工作树：<名称>`，默认为主工作区），下方以可搜索列表展示所选仓库的工作树及路径（不可用的禁用并显示原因），发送时不新建 worktree，直接在所选工作树（`workspace`）中启动；
  - 窄屏（小于 640px）或高度小于 480px 的触屏设备，在快速开始页 `/new` 与聊天视图中共用紧凑输入状态：
    - 输入框未聚焦且无草稿文字、待发送附件、上传进度或发送中状态时，输入卡片收起为单行输入框与发送/停止按钮。下方不常驻选项行，仓库与分支位置行、初始化指令勾选栏及卡片内包括设置齿轮在内的辅助按钮隐藏；
    - 展开输入框后才显示会话选项入口；agent 与启动方式位于卡片内，权限与 agent 配置统一收进“会话选项”底部面板，显示当前值，读取期间保留加载提示与禁用状态；
    - 单行输入框与完整卡片之间平滑改变高度，发送按钮随之移动且文字不缩放；动画可以打断，窗口尺寸变化后不残留固定高度；
    - 会话选项面板使用与聊天视图相同的入口展开动画，系统开启“减少动态效果”时直接显示；
    - 点击或聚焦输入框本身（或打开其弹出菜单）时展开完整输入卡片，失焦且为空时自动收起；卡片容器不充当包含其他控件的额外按钮，键盘可直接聚焦文本框；进入 `/new` 在窄屏或触屏设备上不自动抢占焦点：
      - 单行紧凑卡片上下内边距对称，隐藏的选项行不在卡片下方留下多余间距；
      - 快速开始页 `/new` 跟踪可视视口高度（`visualViewport.height`）并锁定页面滚动，软键盘弹起时展开后的输入卡片整体保持在键盘上方可见；
  - 进入任一聊天视图时，输入框上方以同样的位置只读显示该会话的仓库与 worktree 分支（分离时为短提交号，无仓库的会话显示目录名），创建后不可更改；每次进入都重新读取仓库状态，读取完成前显示上次已知的值；
  - agent 旁的“启动方式”可选聊天或终端，本设备记住上次的选择；选终端时在 agent 的终端里启动（有附件时移入新会话上传目录并附上 `附件路径：`），不显示权限和配置项：
    - 已迁移命令行初始提示的 agent 通过首次启动参数直接传入消息（不写入 `SessionInfo.Cmd` 或混合会话记录）；
    - 未迁移的 agent 与不支持该消息形式的边界情况仍待输出安静后输入消息并回车；
  - 仓库列出全部已登记仓库（不可用的禁用并说明原因），agent 只列出勾选了混合模式的启动配置；缺少可用仓库或 agent 时禁用发送并提供“添加仓库”或“新建启动配置”入口；
  - 本设备记住上次成功时的仓库、agent 以及工作树方式（新建 worktree 或所选的已有工作树），仓库或 agent 失效时回退到第一个可用项，已有工作树失效时回退到所选仓库的主工作区（或第一个可用工作树）；基准分支默认为仓库主工作区当前的分支；
  - 未发送的文字、待发送附件及当次调整的仓库、工作树方式、基准分支、已有工作树、agent、权限模式、配置项与初始化指令勾选状态保存在本设备，切换页面后保留，发送成功后清除；
  - 打开页面和每次切换 agent 都经 `POST /api/launch-configs/{id}/options {repositoryId}` 让 agent 在仓库中临时开一个会话，重新读取配置项：
    - 读取期间显示“正在读取配置…”，agent、权限、配置与发送均禁用，完成后恢复可用；
    - 读取失败时沿用该启动配置记住的清单（`chatOptions`），不提供重试按钮；
    - agent 不接受的值不阻断发送，只在聊天中留下说明；
  - 权限模式默认“全部接受”；
  - 与聊天输入框相同，可用“上传文件”按钮、粘贴或拖放把文件加为附件（显示为可移除的标签，带上传进度，数量与大小限制相同）：
    - 会话还不存在时经 `POST /api/uploads` 上传到服务端的暂存文件夹，发送后移入新会话的上传目录，随首条消息发送；
    - 首条消息之前的步骤失败时附件保留以便重发；首条消息发送失败时附件已归新会话，输入框不再保留；
  - 输入框底部、发送按钮左侧有与聊天视图相同的“快捷指令”菜单（US-G02）：选择后正文原样插入光标处，不自动发送；正文里的 `{{dir}}`、`{{webmux}}` 不在浏览器展开；
  - 发送只发出一次 `POST /api/sessions`（新建 worktree 时带 `newWorktree`，选择已有工作树时带 `workspace`；另有 `configId`、`view: "chat"`、`prompt`、`expandPrompt: true`、`attachments`、`agentConfig`、`permissionMode` 与 `clientRequestId`），快速开始没有独立接口：
    - `expandPrompt` 只由快速开始页设置：服务端在会话创建后替换首条消息里的 `{{dir}}`（会话临时文件夹路径）与 `{{webmux}}`（WebMux 工具说明）：
      - 先 `{{dir}}`、说明最后插入，单遍，不展开其他名称；
      - 临时文件夹或说明不可用时预检即以 503 拒绝，不创建会话或 worktree；
      - `webmux run --prompt` 与其他调用者不设置它，消息原文不变；终端视图首条消息同样适用；
      - 终端视图的首条消息同样是会话名的来源（US-B07）：取去掉 `{{dir}}`、`{{webmux}}` 引用后的用户输入，不含初始化指令；
    - 选择「新建 worktree」时，服务端依次创建 worktree（分支名由快速开始页按与手动新建相同的 `wt/<形容词>-<名词>` 规则生成，路径为默认父目录下的分支名末段；分支冲突时重新生成重试）、以聊天视图启动混合会话、应用配置与权限模式，并把首条消息作为第一轮提示发出，浏览器断开不影响；
    - 选择「已有工作树」时跳过创建步骤，直接在所选工作树（`workspace`）中启动聊天视图会话，后续配置、权限与首条消息的处理相同；
    - 同一请求 ID 在执行中及完成后 10 分钟内重复提交只返回同一结果；
  - 仓库设置了初始化指令时，输入框附近显示“使用初始化指令”勾选框：
    - 从聊天开始时默认勾选，从终端开始时默认不勾选；手动选择在切换页面时保留，发送成功后或切换仓库时恢复默认；
    - 勾选时首条消息为“展开后的指令、空行、用户输入”，指令里写了 `{{user}}` 时用户输入放在那里而不是接在后面；可预览展开后的内容（`{{dir}}` 与 `{{webmux}}` 在会话创建后才替换为临时文件夹路径与 WebMux 工具说明）；
    - 引用了不存在的快捷指令时服务端拒绝发送并列出缺少的名称，不创建会话或 worktree；
  - 成功后当前标签页进入新会话的聊天视图；失败时停在原页面、保留文字，按失败体的 `step`（`precheck`、`worktree`、`session`、`config`、`prompt`）说明失败步骤与原因，已创建的 worktree 或会话保留并提供入口；
  - 生成的是标准混合会话：切换、恢复、关闭、归档与删除 worktree 都按本节其他故事进行。
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
  - 预览不依赖活跃 actor，不启动 agent 或创建 worktree；本层尚无浏览器交接入口或投递动作。

依据：

- [输入卡片变形动画](../../../web/src/hooks/composer-morph.svelte.ts)、[抽屉入口动画](../../../web/src/lib/components/ui/drawer/drawer-content.svelte)。

- [混合会话服务](../../../internal/hybrid/)、[连接器检查](../../../internal/hybrid/connector.go)、[agent 适配器核心](../../../internal/agentprofile/)、[ACP 客户端](../../../internal/acp/)
- [agent 注册表](../../../internal/agentprofile/agents/agents.go)、[Codex 适配器](../../../internal/agentprofile/codex/codex.go)、[适配器契约测试](../../../internal/agentprofile/agenttest/agenttest.go)
- [Claude Code 适配器](../../../internal/agentprofile/claudecode/claudecode.go)、[Claude 注册记录](../../../internal/agentprofile/claudecode/registry.go)、[Antigravity CLI 适配器](../../../internal/agentprofile/agy/agy.go)
- [会话外壳与伴随进程](../../../internal/session/lifecycle.go)、[进程事实](../../../internal/session/process_facts.go)、[聊天接口](../../../internal/api/chat.go)、[聊天通道](../../../internal/hub/chat.go)
- [启动配置](../../../internal/launch/types.go)、[聊天视图](../../../web/src/components/chat/)、[代码链接解析](../../../web/src/markdown-link.ts)、[代码工作区](../../../web/src/components/CodeWorkspace.svelte)、[会话页](../../../web/src/pages/Terminal.svelte)
- [已归档记录](../../../web/src/components/ArchivedSessions.svelte)、[归位规则](../../../web/src/archived.ts)、[安装提示](../../../web/src/components/ConnectorInstallHint.svelte)、[原生附着](../../../internal/attach/attach.go)
- [使用说明](../../hybrid-sessions.md)、[协议](../../protocol/http-hybrid.md)、[创建会话协议](../../protocol/http-session-create.md)、[端到端测试](../../../web/test/e2e/hybrid.spec.ts)。
- [会话卡片](../../../web/src/components/SessionRow.svelte)、[会话页头部](../../../web/src/components/TerminalHeader.svelte)、[品牌图标](../../../web/src/components/LaunchConfigIcon.svelte)、[图标一致性测试](../../../internal/launch/presets_test.go)
- [快速开始页](../../../web/src/pages/QuickStart.svelte)、[快速开始状态](../../../web/src/quick-start.ts)、[创建编排](../../../internal/workspace/create_session.go)、[会话接口](../../../internal/api/sessions.go)、[按视图输入](../../../internal/hybrid/input.go)、[配置项清单](../../../internal/hybrid/options.go)
- [聊天历史](../../../web/src/components/ChatHistory.svelte)、[快速开始浏览器用例](../../../web/test/browser/QuickStart.test.ts)、[聊天历史浏览器用例](../../../web/test/browser/ChatHistory.test.ts)、[快速开始端到端用例](../../../web/test/e2e/quick-start.spec.ts)。

- [手机会话选项面板](../../../web/src/components/chat/ChatSessionOptions.svelte)、[模型可搜索列表](../../../web/src/components/chat/ChatChoiceCommandList.svelte)、[共用输入卡片](../../../web/src/components/chat/ComposerCard.svelte)；
- [聊天浏览器用例](../../../web/test/browser/ChatView.test.ts)、[布局验收](../../../web/test/storybook/acp-chat.spec.ts)。

- [对话事务存储](../../../internal/conversation/)、[私有数据库生命周期](../../../internal/statefile/database.go)。

- [持久历史接口](../../protocol/http-conversation.md)、[历史路由](../../../internal/api/conversation.go)、[portable projection](../../../internal/conversation/projection.go)。

- [交接预览](../../protocol/http-handoff.md)、[预算与选择](../../../internal/handoff/)、[预览接口](../../../internal/api/handoff.go)。
