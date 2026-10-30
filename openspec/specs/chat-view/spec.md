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

- **用户消息**显示为气泡，**agent 消息**显示为正文。Markdown 渲染 SHALL 沿用代码预览的清洗规则，不加载外部资源；代码块 SHALL 可复制。
- **思考**默认折叠，显示为“已思考”。
- **工具调用**显示为一行摘要（种类图标、标题、状态），可以展开查看内容。连续的只读调用（读取、搜索）SHALL 合并为“已浏览 N 项”。
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

### Requirement: 权限请求与表单

agent 发起权限请求时，聊天视图 SHALL 在输入框位置显示编号选项，选项文字与种类取自请求；请求关联工具调用时，SHALL 同时标注在对应的工具调用上。

agent 发起 `form` 模式的表单请求时，聊天视图 SHALL 渲染 string、enum 与 boolean 字段，提供提交、拒绝与取消。其他字段类型 SHALL 提示“该表单需要在终端中完成”并只提供拒绝与取消。

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
- 以上保存方式使模式在切换两端、休眠唤醒、重连与恢复后保持不变。

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

agent 提供的配置项 SHALL 显示在输入框下方：

- 选择类渲染为下拉，布尔类渲染为开关；
- 变更经 `PUT /api/sessions/{id}/chat/config/{configId}` 提交；
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
| 回合以 `end_turn` 结束                        | `done`       |
| 回合以 `refusal`、错误或 `auth_required` 结束 | `error`      |

用户发送消息或答复请求时，系统 SHALL 清除关注。会话状态 SHALL 相应地取 `running`（回合进行中）、`waiting`（存在关注）或 `idle`。

#### Scenario: 回合结束后的关注

- **WHEN** 聊天端的回合以 `end_turn` 结束
- **THEN** 会话关注为 `done`、状态为 `waiting`；用户再次发送消息后关注被清除，状态变为 `running`

### Requirement: 聊天端的推送与卡片预览

现有 Web Push SHALL 按这些关注照常推送，SHALL NOT 新增推送通道或负载字段。会话卡片在聊天端 SHALL 用最新一条 agent 消息的前三行作为预览。

#### Scenario: 等待授权的推送

- **WHEN** 聊天端出现权限请求，且已启用推送
- **THEN** 用户收到“等待授权”的推送，点开后进入该会话的聊天视图
