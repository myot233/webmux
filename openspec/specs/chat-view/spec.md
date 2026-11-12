# chat-view Specification

## Purpose

聊天视图把 ACP agent 的结构化输出呈现为适合手机和桌面阅读与操作的对话界面：消息、思考、工具调用、计划、压缩、用量、权限与表单。它通过一个可续接的事件流，让多台设备看到同一份状态。

## Requirements

### Requirement: 聊天事件流

系统 SHALL 为每个混合会话提供已认证的 WebSocket 端点 `/ws/s/{id}/chat`，使用文本 JSON 帧，并继承现有的认证、Host 与 Origin 校验。该端点 SHALL 在会话处于任一端时都可以连接。

连接建立后，服务端 SHALL 先发送 `snapshot`，再发送 `events`，之后持续推送增量。

- `snapshot` 包含：所在端、切换阶段与进度、资格与原因、agent 与 agent 会话 ID、回合状态、待答复请求、配置项、命令列表、计划、用量、队列、是否支持插话、事件日志的代际与起止序号。
- `events` 是带单调递增序号的规范化事件，种类包括：消息追加、工具调用 upsert、计划、命令、配置、用量、会话信息、压缩、提示、回合开始/结束、请求打开/关闭、标记。

慢客户端 SHALL 以 4408 断开后重连，不阻塞 agent 的读取。会话不存在时 SHALL 以 4404 关闭。

#### Scenario: 会话不存在

- **WHEN** 客户端连接不存在的会话 ID 对应的 `/ws/s/{id}/chat`
- **THEN** 服务端以 4404 关闭连接

### Requirement: 聊天事件续接与历史上限

客户端 SHALL 可以带上代际与最后序号重连：

- 代际相同且序号仍在日志内时，服务端 SHALL 只补发其后的事件；
- 否则 SHALL 发送重置并从日志起点重放。

日志按编码后的字节计量，上限取 `scrollback_bytes`。超出时最早的事件被丢弃，视图 SHALL 在顶部显示“更早的历史未缓存，可切到终端查看”。

#### Scenario: 断线续接

- **WHEN** 手机在 agent 输出过程中锁屏 30 秒后回到页面
- **THEN** 页面只收到断线期间的事件，已有内容不闪烁、不重复

#### Scenario: 历史超出上限

- **WHEN** 日志超过上限后，用户打开会话
- **THEN** 视图顶部显示更早历史未缓存的提示，其余对话正常显示

### Requirement: 时间线内容的展示

聊天视图 SHALL 按事件构建时间线：

- **用户消息**显示为气泡，**agent 消息**显示为正文。
  - Markdown 渲染 SHALL 使用与代码预览相同的标签与属性清洗，不执行脚本、不加载任何图片或外部资源。
  - 图片 SHALL 显示为文字，标题 SHALL NOT 生成锚点 `id`，相对图片与文档内 `#锚点` SHALL NOT 作用于聊天消息。
  - 链接按种类分别处理：
    - HTTP(S) 与 `mailto:` 链接 SHALL 在新标签页打开并带 `noopener noreferrer`；
    - 指向代码位置的链接（本机 `file://` URI、绝对路径或相对路径，含可选 `#L行`、`#L起-L止`、`:行`、`:起-止` 或 `#标题` 片段）SHALL 移除原始 `href`，点击或按 Enter 时在当前会话的代码浏览器中打开对应文件或目录，并定位到目标行范围或 Markdown 标题；
      - 相对路径 SHALL 先按会话当前目录解析，找不到时再按仓库根目录解析；两处都有同名文件时打开当前目录下的文件；
      - 文件正在显示对比时，`#标题` 链接 SHALL 切回文件并在 Markdown 预览中定位标题；
    - 其余协议与纯 `#片段` 链接 SHALL 只保留纯文本。
  - 代码块 SHALL 可复制。
- **思考与工具调用**默认折叠：
  - agent 回复、计划、提示或用户输入之间连续的思考与工具调用 SHALL 合并为一行，时间线顺序 SHALL NOT 改变；只有思考时该行显示“已思考”。
  - 展开后 SHALL 按顺序列出每条思考与每次调用，思考默认折叠。
  - 连续的工具调用 SHALL 合并为一行，折叠行显示最新一条调用（种类图标、标题、状态）和调用数；等待授权的调用优先显示。
  - 进行中的调用 SHALL 显示动画，并遵循 `prefers-reduced-motion`。
  - 点击展开后列出每次调用，每次调用可再展开查看内容。
  - 调用带 `locations` 或 `resource_link` 内容块时，点击位置条目 SHALL 在代码浏览器中打开目标文件与行号。
- **diff 内容**显示增删行数，并可以打开现有代码面板查看。
- 不支持的内容种类 SHALL 显示占位与种类名，SHALL NOT 丢弃整条事件。
- **计划**以“计划 x/y”显示在输入框上方，可以展开。
- **压缩**显示为时间线分隔条，可以展开摘要。
- **提示**按严重程度着色。
- **回合结束**：`cancelled` 显示“已中断”，`refusal` 与错误使用错误样式。
- **用量**以上下文占用环形图与费用显示。

#### Scenario: 工具调用状态更新

- **WHEN** agent 先报告一个 `pending` 的编辑调用，随后以同一 `toolCallId` 报告 `completed` 与 diff
- **THEN** 时间线中只有一行该调用，状态从进行中变为完成，并显示增删行数

#### Scenario: 消息中的代码链接在代码浏览器打开而图片与危险协议保持惰性

- **WHEN** agent 消息含 `![截图](docs/a.png)`、`[源码](src/main.ts#L10-L20)`、`[绝对路径](file:///repo/README.md)`、`[危险](javascript:alert(1))` 与 `## 标题`
- **THEN** 图片显示为“图片：截图（docs/a.png）”文字且不发起请求，标题元素没有 `id`，`javascript:` 链接只保留纯文本，点击 `源码` 或 `绝对路径` 在当前会话的代码浏览器（或已弹出的独立代码窗口）中打开对应文件并选中第 10–20 行

### Requirement: 权限请求与表单

agent 发起权限请求时，聊天视图 SHALL 在输入框位置显示编号选项，选项文字与种类取自请求；请求关联工具调用时，SHALL 同时标注在对应的工具调用上。

agent 发起 `form` 模式的表单请求时，聊天视图 SHALL 渲染 string、enum、array（多选）与 boolean 字段，提供提交、拒绝与取消。其他字段类型 SHALL 提示“该表单需要在终端中完成”并只提供拒绝与取消。

答复 SHALL 经 `POST /api/sessions/{id}/chat/requests/{requestId}` 提交：

- 第一个有效答复生效，后续答复 SHALL 返回 409 `answered`；
- 所有设备 SHALL 立即看到该请求已关闭及其结果。

#### Scenario: 两台设备同时批准

- **WHEN** 桌面与手机同时显示同一个权限请求，手机先选择“允许一次”
- **THEN** agent 收到“允许一次”；桌面上的请求随即关闭，若桌面也提交了答复则收到 409

### Requirement: 权限模式

每个混合会话 SHALL 有一个由 webmux 保存的权限模式：`auto`（自动批准，默认）或 `ask`（每次询问）。它不是 agent 的配置项。

- 新建会话 SHALL 处于 `auto`。
- 当前模式 SHALL 出现在聊天状态的 `permissionMode` 中，并在所有设备同步。
- `ask` SHALL 写入混合会话记录的 `permissionMode`；没有该字段的记录 SHALL 按 `auto` 处理。
- 以上保存方式使模式在切换两端、重连与恢复后保持不变。

- 输入框下方的配置项旁 SHALL 显示“权限”选择（“每次询问”/“自动批准”），变更经 `PUT /api/sessions/{id}/chat/permission-mode` 提交 `{clientRequestId, mode}`；`mode` 不是 `ask` 或 `auto` 时 SHALL 返回 400，切换进行中 SHALL 返回 409 `switching`，终端端 SHALL 允许设置；
- `auto` 模式下，agent 的权限请求 SHALL 由服务端直接选择第一个 `allow_once` 选项，没有时选择第一个 `allow_always`；此时 SHALL NOT 打开请求、产生关注与推送，也 SHALL NOT 在时间线追加批准提示；
- 请求没有允许类选项时 SHALL 照常交给用户；表单请求 SHALL NOT 被自动答复；
- 从 `ask` 改为 `auto` 时，已挂起的权限请求 SHALL 按同样规则立即批准并关闭。

#### Scenario: 新会话默认自动批准

- **WHEN** 用户新建一个混合会话并在聊天视图发送提示，agent 请求读取文件的权限
- **THEN** 权限选择显示“自动批准”，agent 收到“允许一次”，不出现等待授权的请求

#### Scenario: 每次询问在重启后保持

- **WHEN** 用户把权限改为“每次询问”，随后切换两端并在服务重启后恢复会话
- **THEN** 权限仍为“每次询问”，agent 的下一个权限请求交给用户答复

#### Scenario: 自动批准工具调用

- **WHEN** 用户在手机上把权限从“每次询问”改为“自动批准”，随后 agent 请求读取文件的权限
- **THEN** agent 收到“允许一次”，回合继续进行；所有设备的权限选择显示“自动批准”，时间线不出现批准提示，会话不进入等待授权状态

#### Scenario: 切换为自动批准时处理挂起请求

- **WHEN** 一个权限请求正在等待答复时，另一台设备把权限改为“自动批准”
- **THEN** 该请求以“允许一次”关闭，等待中的设备随即看到请求关闭；切到终端再切回后权限仍为“自动批准”

### Requirement: 配置项与斜杠命令

agent 提供的配置项 SHALL 在桌面输入框下方显示，在手机收进输入卡片内的“会话选项”入口：

- 选择类渲染为下拉并显示当前值，布尔类渲染为开关；
- 桌面选项行空间不足时 SHALL 支持横向滚动；手机 SHALL 不常驻额外选项行，收起状态 SHALL 隐藏设置入口，展开输入框后才显示卡片内的“会话选项”按钮，由该按钮打开底部面板；
- 小于 640px 宽或小于 480px 高的触屏视口 SHALL 使用手机布局，面板按行显示名称与当前值，内部滚动且关闭入口保持可见；
- 关闭按钮、遮罩和下滑 SHALL 能关闭面板，关闭后焦点 SHALL 回到入口；
- 单行输入框与展开卡片之间 SHALL 平滑改变高度，发送按钮同步移动且文字不缩放；动画中断、窗口尺寸变化或减少动态效果时 SHALL 恢复正确布局。
- 面板打开时 SHALL 从入口位置展开轮廓，文字与控件 SHALL 不缩放；系统要求减少动态效果时 SHALL 跳过展开动画，快速关闭重开 SHALL 不残留裁切；
- 变更经 `PUT /api/sessions/{id}/chat/config/{configId}` 提交；
- 手机面板 SHALL 在提交期间和失败后保留已确认值，并在面板内显示保存错误；
- 收到 `config_option_update` 时整体替换显示。

系统 SHALL 记录每次生效的取值，用于切换和恢复后补齐（D25）。

输入 `/` 时 SHALL 打开命令菜单。菜单列出 agent 提供的命令名称、描述与参数提示，并随输入过滤。

#### Scenario: 手机切换模型

- **WHEN** 用户在手机上把模型从 haiku 改为 sonnet
- **THEN** agent 收到 `session/set_config_option`，所有设备的下拉都显示 sonnet，切到终端再切回后仍是 sonnet

### Requirement: 切换入口与进度

混合会话页的头部 SHALL 显示“聊天 | 终端”分段控件，当前所在端为选中状态。在窄屏上，控件 SHALL 只显示图标并保留可访问名称。

- 聊天端 SHALL 隐藏仅对终端有意义的头部操作，保留全屏、代码面板与关闭会话。
- 切换不可用时，控件上的目标端 SHALL 禁用并显示原因。
- 切换期间，页面 SHALL 显示当前步骤（中断中、退出中、启动中、加载历史中），并禁用输入。
- 切换失败时，页面 SHALL 停留在原端，并显示原因。

#### Scenario: 切换进度

- **WHEN** 用户从终端切到聊天
- **THEN** 页面依次显示各步骤，完成后显示聊天视图，期间输入框不可用

### Requirement: 聊天端的关注映射

会话在聊天端时，系统 SHALL 用现有的关注种类表达回合状态，来源为 `agent`：

| 情况                                          | 关注         |
| --------------------------------------------- | ------------ |
| 有待答复的权限请求                            | `permission` |
| 有待答复的表单请求                            | `question`   |
| 回合以 `end_turn` 结束且无未完成后台工作      | `done`       |
| 回合以 `refusal`、错误或 `auth_required` 结束 | `error`      |

用户发送消息或答复请求时，系统 SHALL 清除关注。会话状态 SHALL 相应地取 `running`（回合进行中）、`waiting`（存在关注）或 `idle`。

#### Scenario: 回合结束后的关注

- **WHEN** 聊天端的回合以 `end_turn` 结束且无未完成后台工作
- **THEN** 会话关注为 `done`、状态为 `waiting`；用户再次发送消息后关注被清除，状态变为 `running`

### Requirement: 回合结束后的后台工作

回合以 `end_turn` 结束时，若仍有 `in_progress` 工具调用，或本轮工具输入明确包含 `run_in_background: true` / `IsDaemon: true`，系统 SHALL 设置 `turn.unfinished: true`，显示“本轮已结束，仍有任务未完成”，且 SHALL NOT 立即产生 `done` 关注。

系统 SHALL NOT 依赖 agent 在回合后主动发送后台完成通知，SHALL 观察 agent 进程树。连续两次静默轮询确认本回合新启动的进程全部退出后，若档案声明了后台状态检查 prompt，系统 SHALL 自动发送该 `session/prompt`；未声明的 agent SHALL 直接按提醒策略产生 `done`，SHALL NOT 额外发起模型回合。

系统 SHALL 通过档案声明的检查回合刷新工具状态与回复，并在其结束后才按提醒策略产生 `done`。新回合开始、切换、重连或会话关闭 SHALL 停止观察。非 Linux/macOS 无进程树支持时，两次轮询后按无新进程处理。

自动检查回合 SHALL 标记为后台刷新，旧工具在该回合补发的完成事件 SHALL NOT 再次触发后台观察或自动检查。系统 SHALL NOT 伪造工具退出码或读取 agent 的私有 transcript。

#### Scenario: agy 在检查回合补发后台完成状态

- **WHEN** agy 的后台进程退出且连续两次轮询没有发现新进程
- **THEN** 系统自动发送英文状态检查消息，agy 补发旧工具的 `completed` 更新与结果回复，随后清除未完成状态并产生 `done`，且不再自动发起第二次检查

#### Scenario: Kimi 的后台启动调用已经完成

- **WHEN** Kimi 的 Bash 工具输入包含 `run_in_background: true`，启动调用已标为 `completed`，回合以 `end_turn` 返回且后台进程仍在运行
- **THEN** 回合仍显示有任务未完成，不产生 `done` 关注；后台进程退出且连续两次静默轮询后，清除 `turn.unfinished` 并直接按提醒策略产生 `done`，不自动发起状态检查回合

### Requirement: 聊天端的推送与卡片预览

现有 Web Push SHALL 按这些关注照常推送，SHALL NOT 新增推送通道或负载字段。会话卡片在聊天端 SHALL 用最新一条 agent 消息的前三行作为预览。

#### Scenario: 等待授权的推送

- **WHEN** 聊天端出现权限请求，且已启用推送
- **THEN** 用户收到“等待授权”的推送，点开后进入该会话的聊天视图

### Requirement: 会话列表中的 agent 标识

首页会话卡片 SHALL 为任意所在端的混合会话（`hybrid.side` 为 `tui`、`acp` 或 `switching`，含“恢复到终端”后的会话）在“所在端”信息处显示 agent 标识：

- 标识由 `agentIcon` 对应的单色品牌小图标与 `agentTitle` 组成，紧接其后是现有的所在端文字，形如“OpenCode · 聊天”；
- 品牌图标 SHALL 替换原来的通用聊天或终端图标，尺寸与同一行的其他元信息图标相同，颜色跟随该行的弱化文字色；
- 图标 SHALL 是装饰性的（对辅助技术隐藏），agent 名称以可见文字提供；
- `agentIcon` 不在本地图标目录中时 SHALL 只显示名称，SHALL NOT 用终端等其他图标代替。

#### Scenario: 聊天端卡片

- **WHEN** 首页列出一个处于聊天端的 OpenCode 混合会话，有 2 个设备连接
- **THEN** 卡片元信息行显示 OpenCode 品牌小图标与“OpenCode · 聊天 · 2 个设备已连接”，不再显示通用聊天图标

#### Scenario: 切换中的卡片

- **WHEN** 首页列出一个正在切换的 OpenCode 混合会话
- **THEN** 卡片显示 OpenCode 品牌小图标与“OpenCode · 切换中”，切换过程中标识不消失

#### Scenario: 终端端卡片

- **WHEN** 用户把一个已关闭的 Claude Code 混合会话“恢复到终端”，随后回到首页
- **THEN** 卡片显示 Claude Code 品牌小图标与“Claude Code · 终端”，不再只显示通用终端图标

#### Scenario: 未知图标键

- **WHEN** 会话的 `agentIcon` 不在本地图标目录中
- **THEN** 卡片只显示 agent 名称与所在端文字，不显示任何替代图标

### Requirement: 聊天页头部的 agent 标识

会话页显示聊天视图时，头部 SHALL 在会话标题之后、状态点之前显示当前 agent 的标识，强调程度低于标题：

- 宽度不小于 640px 时，标识 SHALL 显示小图标与 `agentTitle`，文字使用弱化色与小号字；
- 宽度小于 640px 时，标识 SHALL 只显示图标；
- 任何宽度下，标识 SHALL 有可访问名称“Agent：<agentTitle>”，并以悬停提示显示同样的文字；
- 空间不足时 SHALL 先截断会话标题，agent 标识 SHALL 保持完整；
- 标识 SHALL 只用于展示，SHALL NOT 可聚焦或可点击；
- `agentIcon` 不在本地图标目录中时，所有宽度 SHALL 显示名称且不显示图标。

会话页显示终端视图时，头部 SHALL NOT 显示该标识。

#### Scenario: 桌面聊天页

- **WHEN** 用户在 1280px 宽的窗口中打开处于聊天端的 Claude Code 混合会话
- **THEN** 头部依次显示会话标题、Claude Code 小图标与“Claude Code”、状态点；悬停标识时提示“Agent：Claude Code”

#### Scenario: 手机聊天页

- **WHEN** 用户在 390px 宽的手机上打开同一会话的聊天视图
- **THEN** 头部在标题后只显示 Claude Code 图标，屏幕阅读器读出“Agent：Claude Code”，长标题被截断而图标完整可见

#### Scenario: 切到终端后隐藏

- **WHEN** 用户在聊天页点击“终端”并完成切换
- **THEN** 头部不再显示 agent 标识；切回聊天后标识重新出现
