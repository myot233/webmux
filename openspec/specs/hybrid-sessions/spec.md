# hybrid-sessions Specification

## Purpose

混合会话让同一段 agent 对话在 ACP 聊天视图与原生 TUI 之间按需切换：webmux 会话 ID 不变，服务端保证同一时刻只有一端打开该 agent 会话，并在中断、失败、崩溃与空闲时给出确定的行为。

## Requirements

### Requirement: 混合会话的创建与起始端

系统 SHALL 只在使用勾选了混合模式的启动配置创建会话时创建混合会话。不带配置的自定义命令和未勾选的配置 SHALL 创建普通会话，行为不变。

`POST /api/sessions` 在 `configId` 指向混合配置时 SHALL 接受可选字段 `startSide`，取值为 `tui` 或 `acp`，缺省为 `tui`。对普通配置或自定义命令提供 `startSide` SHALL 返回 400。

- 从终端开始：系统 SHALL 在 PTY 中执行配置保存的 argv，会话处于 `tui` 端。
- 从聊天开始：档案声明了第三方连接器时，系统 SHALL 先检查连接器已安装，未安装时 SHALL 返回 409 `connector_missing`
  并附连接器信息，SHALL NOT 创建会话或写入混合会话记录。检查通过后，系统 SHALL 按档案启动 ACP 进程，
  依次调用 `initialize` 和 `session/new {cwd}`，记录返回的 agent 会话 ID，会话处于 `acp` 端。

两种起始方式 SHALL 都写入混合会话记录。

Web 端新建会话弹层 SHALL 在选中混合配置时显示“从终端开始 / 从聊天开始”选择，默认从终端开始，并按浏览器记住上一次的选择。

#### Scenario: 从聊天开始

- **WHEN** 用户选择勾选了混合模式的 omp 配置，选择“从聊天开始”并提交
- **THEN** 会话出现在列表中，`hybrid.side = "acp"`，页面显示聊天视图；PTY 中没有运行任何进程

#### Scenario: 从聊天开始但连接器未安装

- **WHEN** 用户选择 Codex 混合配置、从聊天开始并提交，服务上没有 `codex-acp`
- **THEN** 服务端返回 409 `connector_missing`，会话列表不变，没有启动任何进程

#### Scenario: 对普通配置指定起始端

- **WHEN** 客户端以未勾选混合模式的配置 ID 和 `startSide: "acp"` 创建会话
- **THEN** 服务端返回 400，不创建会话

### Requirement: 单写者与所在端

混合会话 SHALL 在任意时刻处于且只处于以下之一：`tui`、`acp`、`switching`。webmux 会话 ID SHALL 在混合会话的整个生命周期中保持不变。

系统 SHALL 保证同一 agent 会话 ID 在任意时刻至多被一个由 webmux 启动的进程打开。只调用 `session/list` 的临时探测进程不算打开。进入目标端之前，系统 SHALL 已确认原端的进程组已经全部退出。这是硬性要求：omp 在 TUI 仍持有会话租约时，另一个进程写入同一会话，会把它分叉成一个新 ID 的会话。

会话信息 SHALL 包含 `hybrid` 对象，至少含 `side`、`phase`、`state`、`agent`、`agentTitle`、`agentIcon` 与 `agentSessionId`（尚未得知时为 `null`）。普通会话 SHALL NOT 含该对象。

- `agentTitle` SHALL 是 agent 档案声明的显示名，非空；
- `agentIcon` SHALL 是 agent 档案声明的图标键，取值属于启动配置的图标目录；
- 两者在会话生命周期内 SHALL 保持不变，并在会话列表快照、列表事件与单个会话信息中一致。

#### Scenario: 切换期间不存在两个进程

- **WHEN** 会话从 `tui` 切到 `acp`，同时用进程表持续观察
- **THEN** 在 TUI 进程退出之前，不出现由 webmux 为该会话启动的 ACP 进程

#### Scenario: 普通会话的负载不变

- **WHEN** 客户端读取普通会话的会话信息
- **THEN** 负载中没有 `hybrid` 字段，其余字段与本变更前一致

#### Scenario: 会话信息带出 agent 显示名与图标

- **WHEN** 用户以 OpenCode 混合配置创建会话，客户端随后读取会话列表快照
- **THEN** 该会话的 `hybrid` 对象含 `agent: "opencode"`、`agentTitle: "OpenCode"` 与一个属于启动配置图标目录的 `agentIcon`，单个会话信息中的取值与之相同

### Requirement: 从终端切到聊天

收到切到 `acp` 的请求且会话在 `tui` 端时，系统 SHALL 先按 `agent-session-identity` 的资格判定确认可以切换（含连接器检查），不满足时返回 409 并附原因，不改变会话。确认可以切换后，系统 SHALL 进入 `switching`，按以下顺序执行：

1. 根据终端信号判断 TUI 是否在执行回合；在执行时，向 PTY 写入档案定义的中断键，并等待信号变为空闲；
2. 结束 TUI 进程，并等待其进程组清理完成；
3. 按档案启动 ACP 进程，调用 `initialize`，再用资格判定得到的 agent 会话 ID 与 cwd 调用 `session/load`；
4. 用记录中的配置项取值补齐 agent 未返回或回到默认值的配置（按档案判断）；记录中尚无某项取值时，使用按档案从 argv 推导的值；
5. 进入 `acp` 端。

切换期间，所有设备的输入 SHALL 被禁用，已连接的终端客户端 SHALL 以关闭码 4409 断开并收到“已切换到聊天视图”的原因。

#### Scenario: 空闲时切换

- **WHEN** TUI 空闲，用户点击“聊天”
- **THEN** 会话在一次切换内进入 `acp` 端，聊天视图回放完整历史，模型与推理强度与切换前一致

#### Scenario: 回合进行中切换

- **WHEN** TUI 正在执行回合，用户确认切到聊天
- **THEN** 系统先发送中断键并等到空闲，再退出 TUI；被中断的回合以中断结束，会话之后可以继续对话

#### Scenario: Codex 首次切到聊天

- **WHEN** argv 为 `["codex","-m","gpt-5.6-luna","-s","read-only"]` 的 Codex 混合会话在 TUI 中发出首条消息后切到聊天
- **THEN** 聊天端的 `model` 为 `gpt-5.6-luna`、`mode` 为 `read-only`，并写入混合会话记录；之后每次加载都重新应用记录中的取值

### Requirement: 中断等待超时

发送中断键后 10 秒内 TUI 仍未回到空闲时，系统 SHALL 暂停切换，在切换状态中提供“继续等待 / 强制切换 / 取消”三个选项，直到用户选择。

- 选择“继续等待”：SHALL 再等待 10 秒。
- 选择“强制切换”：SHALL 跳过等待，直接结束 TUI 继续切换。确认文案 SHALL 说明后果：被强制结束的回合可能不完整，omp 已输出的部分可能丢失。由于 OpenCode 的 TUI 以独立服务运行（见 `hybrid-agent-profiles`），强制结束后它的回合 SHALL NOT 在后台继续。
- 选择“取消”：SHALL 保持在 `tui` 端。

#### Scenario: 用户取消

- **WHEN** 中断后 10 秒仍未空闲，用户选择“取消”
- **THEN** 会话回到 `tui` 端，TUI 进程仍在运行，终端客户端可以重新连接

### Requirement: 从聊天切到终端

收到切到 `tui` 的请求且会话在 `acp` 端时，系统 SHALL 进入 `switching`，按以下顺序执行：

1. 有回合在运行时发送 `session/cancel`，并等待回合结束；挂起的权限请求 SHALL 答复为 cancelled；
2. 把队列中未发出的消息合并后退回草稿（见 `chat-prompt-queue`）；
3. agent 支持时调用 `session/close`，然后结束 ACP 进程，并等待其进程组清理完成；
4. 在 PTY 中启动档案生成的恢复命令，开始新的运行代际；
5. 进入 `tui` 端。

回合正在运行时，Web 端 SHALL 先请求用户确认“当前回合会被中断，已输出的内容会保留”。

#### Scenario: 往返后历史完整

- **WHEN** 会话从聊天开始并对话，切到终端继续对话，再切回聊天
- **THEN** 每一次切换后，对端都能看到此前全部对话并继续

### Requirement: 切换失败自动回滚

切换的任一步骤失败时，系统 SHALL 回到原端：原端为 `tui` 时按档案的恢复命令重新启动 TUI，原端为 `acp` 时重新启动 ACP 并 `session/load`。系统 SHALL 以可读原因通知所有设备。

回滚本身也失败时，会话 SHALL 标记为 `interrupted`，并进入可恢复列表。

#### Scenario: 加载失败

- **WHEN** 切到聊天时 `session/load` 返回错误
- **THEN** 会话回到 `tui` 端并运行恢复后的 TUI，页面显示失败原因

### Requirement: 并发切换请求

切换进行中再次收到切换、发送、关闭或恢复请求时，系统 SHALL 返回 409 `switching`，不排队执行，并 SHALL 继续向所有设备发布当前切换进度。关闭请求 SHALL 等到切换完成或回滚后才能再次提交。

#### Scenario: 两台设备同时切换

- **WHEN** 两台设备几乎同时请求切到聊天
- **THEN** 只有一次切换执行，另一台收到 409，两台都看到同一个进度并最终显示同一端

### Requirement: 进程意外退出

- 会话在 `tui` 端时，TUI 进程自行退出（包括 `/exit` 与崩溃）SHALL 使会话标记为 `interrupted`，SHALL NOT 自动切换。会话页 SHALL 保留最后的终端输出，并提供“恢复到聊天 / 恢复到终端”。
- 会话在 `acp` 端时，ACP 进程意外退出 SHALL 触发一次自动重连：
  - 重新启动 ACP，agent 声明了 `session/resume` 且档案允许时 SHALL 使用 `session/resume`（不回放历史，事件日志保持连续），否则 SHALL 使用 `session/load` 并以新的日志代际重新回放；
  - 当前回合 SHALL 以中断结束；重连失败时，会话 SHALL 标记为 `interrupted`。

#### Scenario: ACP 崩溃

- **WHEN** 聊天端回合进行中，ACP 进程被 SIGKILL
- **THEN** 当前回合显示为已中断，系统自动重连一次；重连成功后可以继续发送消息

### Requirement: 聊天端进程保持运行

会话处于 `acp` 端且状态为 `active` 时，系统 SHALL NOT 因空闲时长结束 ACP 进程，使 agent 内部的后台命令、子任务或定时器在聊天端空闲期间保持运行；系统 SHALL 仅在切换到 `tui`、显式归档（关闭）会话或服务停止时结束 ACP 进程。

#### Scenario: 聊天端空闲时保持存活

- **WHEN** 聊天端会话没有运行中的回合且长时间未收到用户命令
- **THEN** ACP 进程保持运行，会话始终处于活动状态，只有用户显式归档会话、切换到终端端或停止服务时才结束进程

### Requirement: 关闭混合会话

`DELETE /api/sessions/{id}` 作用于混合会话时，系统 SHALL：

- 在聊天端有回合运行时先取消，在终端端回合运行时先按中断流程处理；
- 再结束当前端的进程，把记录状态设为 `closed`，并从会话列表中移除该会话；
- SHALL NOT 删除记录或 agent 的会话库。

UI SHALL 把该操作称为“停止会话并归档”，确认文案 SHALL 说明 agent 进程将停止、对话保留在 agent 中、可在所属工作树的“已归档”中恢复到聊天或终端；确认按钮 SHALL 为“停止并归档”，且 SHALL NOT 使用破坏性样式。归档完成后的页面去向遵循 `workspace-overview` 的“归档后返回首页”。

服务关机时，系统 SHALL 先结束所有混合会话的 ACP 进程，再执行现有的会话关闭流程。

#### Scenario: 关闭后可恢复

- **WHEN** 用户归档一个处于聊天端的混合会话
- **THEN** 该会话从运行中列表消失，出现在所属工作树的“已归档”区，标注“已关闭”

#### Scenario: 归档确认文案

- **WHEN** 用户在混合会话的会话页菜单中选择“停止会话并归档”
- **THEN** 确认框标题为「停止并归档“<会话标题>”？」，说明 agent 进程将停止、对话保留并可恢复，主按钮为非破坏性的“停止并归档”

### Requirement: 认证失败的引导

聊天端的请求返回 `auth_required` 时，系统 SHALL 以错误结束当前回合，并在聊天视图中显示提示与“切到终端登录”按钮。聊天视图 SHALL NOT 尝试执行登录流程。

#### Scenario: 需要登录

- **WHEN** `session/prompt` 返回需要认证的错误
- **THEN** 回合以错误结束，页面显示“切到终端登录”按钮，点击后按切到终端的流程执行

### Requirement: 首次在聊天端打开时播种配置

档案提供了 argv → 配置项的推导时，系统 SHALL 在混合会话第一次在聊天端打开 agent 会话（`session/new` 或第一次 `session/load`）之后，
把推导出的、记录中尚无取值的配置项用 `session/set_config_option` 设置，并写入混合会话记录。

- agent 返回的配置项中没有该 ID 或没有该取值时，系统 SHALL 跳过该项，并在聊天视图提示“以下参数在聊天端不生效”；
- 设置失败 SHALL NOT 使打开失败，系统 SHALL 在聊天视图显示失败的配置项；
- 用户之后在聊天视图修改的取值 SHALL 覆盖推导值。TUI 中通过 `/model` 等命令做出的修改 SHALL NOT 被延续。

#### Scenario: 从聊天开始的 Codex 会话

- **WHEN** 预置的 Codex CLI 混合配置从聊天开始
- **THEN** `session/new` 之后系统设置 `mode = agent-full-access`，聊天视图的模式显示为完全访问

#### Scenario: agent 不提供推导值

- **WHEN** argv 带 `-m no-such-model`，agent 的 `model` 配置项不含该取值
- **THEN** 会话照常打开，聊天视图提示 `-m no-such-model` 在聊天端不生效

### Requirement: 首条消息临时标题

从聊天开始、由 webmux 新建 agent 会话的混合会话，在用户未命名且尚未收到 agent 标题时，发送第一条消息 SHALL 立即以该消息文本作为聊天端临时标题。

- 临时标题去除控制字符，把连续空白折叠为一个空格并去除首尾空白，超过 80 个字符时截断为前 79 个字符加“…”。
- 文本为空（例如只有附件）时 SHALL NOT 设置临时标题。临时标题只设置一次。
- 加载或恢复已有 agent 会话的混合会话 SHALL NOT 使用临时标题。
- agent 上报标题后 SHALL 替换临时标题；agent 清除标题后 SHALL 回到目录名，不再恢复临时标题。

#### Scenario: 新聊天会话的第一条消息

- **WHEN** 用户新建从聊天开始的未命名混合会话，发送消息“帮我 修复\n登录页的重定向问题”
- **THEN** 会话列表与聊天页标题立即显示“帮我 修复 登录页的重定向问题”，不等待 agent 回复

#### Scenario: agent 标题替换临时标题

- **WHEN** 临时标题已生效，第一轮结束时 agent 上报标题“修复登录重定向”
- **THEN** 标题变为“修复登录重定向”，此后再发送消息不会改回临时标题

#### Scenario: 用户命名不被覆盖

- **WHEN** 用户以名称 `review` 新建从聊天开始的混合会话并发送第一条消息
- **THEN** 会话标题始终为 `review`

#### Scenario: 只有附件的第一条消息

- **WHEN** 新聊天会话的第一条消息只有一张图片附件、没有文本
- **THEN** 标题保持目录名；之后发送的带文本消息也不再设置临时标题

#### Scenario: 从终端切到聊天后发送

- **WHEN** 从终端开始的未命名会话切到聊天端，用户随后发送消息“继续”
- **THEN** 不设置临时标题，标题保持目录名，直到 agent 上报标题

### Requirement: 过滤活动回合中 agent 回显的用户消息片段

系统在 `acp` 端发起 `session/prompt` 时，已经在 `turn.start` 事件之后立即向聊天时间线写入
包含原始文本与附件元数据的用户消息。因此：

- 在活动 `session/prompt` 回合进行期间（即从发出 `session/prompt` 到收到其最终响应或取消完成之间），
  若 agent 发送 `sessionUpdate: "user_message_chunk"` 通知（例如 `agy acp` 在实时回合开头回显用户消息），
  系统 SHALL 忽略该 `user_message_chunk` 通知，SHALL NOT 在时间线中生成第二条重复的用户消息。
- 在非活动回合阶段（即执行 `session/load` 回放历史会话期间），系统 SHALL 照常将 agent 发送的
  `user_message_chunk` 通知转换为时间线中的用户消息事件。

#### Scenario: agy 在实时回合中回显 user_message_chunk

- **WHEN** 用户在 `agy` 混合会话聊天端发送一条消息，`agy acp` 在处理 `session/prompt` 期间依次发送
  `user_message_chunk` 与 `agent_message_chunk`
- **THEN** 聊天时间线中该回合只包含一条用户消息与一条 agent 回复，不出现重复的用户消息气泡

#### Scenario: session/load 历史回放保留 user_message_chunk

- **WHEN** 会话从终端切到聊天并执行 `session/load`，`agy acp` 依次回放历史中的
  `user_message_chunk`、`tool_call` 与 `agent_message_chunk`
- **THEN** 聊天时间线完整还原历史中的用户消息、工具调用与 agent 回复

### Requirement: 终端视图创建混合会话时的首条提示传递

当客户端通过创建会话接口以终端视图启动混合会话并提供非空 `prompt`（含可选的仓库初始化指令展开文本）时，系统 SHALL 先分配会话 ID 并展开初始化指令中的 `{{dir}}` 与 `{{webmux}}` 占位符，再按 `hybrid-agent-profiles` 的规则检查能否将完整提示作为首次 TUI 启动的命令行参数传递：

- 能够通过命令行参数传递时，系统 SHALL 直接以包含该提示参数的 `argv` 启动首次 TUI 进程，SHALL NOT 再向 PTY 注入该提示词文本与回车；
- 不能通过命令行参数传递（档案尚未声明支持、启动配置已自带提示词参数，或提示词文本触发 CLI 参数解析边界限制）时，系统 SHALL 以常规 `TUIArgv` 启动 TUI 进程，并回退到 `SendWhenReady` 在终端就绪后写入提示文本与回车。

展开提示词失败时，系统 SHALL 清理刚分配的空壳会话并按创建会话的 `prompt` 步骤返回错误。

#### Scenario: 终端视图带初始化指令通过启动参数传递首条提示

- **WHEN** 客户端在设置了初始化指令 `读取 {{dir}} 并使用 {{webmux}}` 的仓库中，以 Claude Code 混合配置、终端视图和 `prompt: "修复测试"` 创建会话
- **THEN** 系统分配会话 ID 后将展开了真实目录与会话 ID 的完整提示词通过 `-- <完整提示>` 传给首次启动的 `claude` 进程，且不再向 PTY 写入该提示词

#### Scenario: 未迁移档案在终端视图回退到 PTY 输入

- **WHEN** 客户端以尚未声明首条提示参数的混合配置（如 Qoder）、终端视图和 `prompt: "修复测试"` 创建会话
- **THEN** 系统以常规命令启动 TUI 进程，并在终端就绪后将 `"修复测试"` 与回车写入 PTY
