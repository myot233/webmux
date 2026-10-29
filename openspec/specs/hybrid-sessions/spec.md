# hybrid-sessions Specification

## Purpose
混合会话让同一段 agent 对话在 ACP 聊天视图与原生 TUI 之间按需切换：webmux 会话 ID 不变，服务端保证同一时刻只有一端打开该 agent 会话，并在中断、失败、崩溃与空闲时给出确定的行为。

## Requirements

### Requirement: 混合会话的创建与起始端

系统 SHALL 只在使用勾选了混合模式的启动配置创建会话时创建混合会话。不带配置的自定义命令和未勾选的配置 SHALL 创建普通会话，行为不变。

`POST /api/sessions` 在 `configId` 指向混合配置时 SHALL 接受可选字段 `startSide`，取值为 `tui` 或 `acp`，缺省为 `tui`。对普通配置或自定义命令提供 `startSide` SHALL 返回 400。

- 从终端开始：系统 SHALL 在 PTY 中执行配置保存的 argv，会话处于 `tui` 端。
- 从聊天开始：系统 SHALL 按档案启动 ACP 进程，依次调用 `initialize` 和 `session/new {cwd}`，记录返回的 agent 会话 ID，会话处于 `acp` 端。

两种起始方式 SHALL 都写入混合会话记录。

Web 端新建会话弹层 SHALL 在选中混合配置时显示“从终端开始 / 从聊天开始”选择，默认从终端开始，并按浏览器记住上一次的选择。

#### Scenario: 从聊天开始

- **WHEN** 用户选择勾选了混合模式的 omp 配置，选择“从聊天开始”并提交
- **THEN** 会话出现在列表中，`hybrid.side = "acp"`，页面显示聊天视图；PTY 中没有运行任何进程

#### Scenario: 对普通配置指定起始端

- **WHEN** 客户端以未勾选混合模式的配置 ID 和 `startSide: "acp"` 创建会话
- **THEN** 服务端返回 400，不创建会话

### Requirement: 单写者与所在端

混合会话 SHALL 在任意时刻处于且只处于以下之一：`tui`、`acp`、`switching`。处于 `acp` 时 SHALL 还可以带有 `dormant` 标记。webmux 会话 ID SHALL 在混合会话的整个生命周期中保持不变。

系统 SHALL 保证同一 agent 会话 ID 在任意时刻至多被一个由 webmux 启动的进程打开。只调用 `session/list` 的临时探测进程不算打开。进入目标端之前，系统 SHALL 已确认原端的进程组已经全部退出。这是硬性要求：omp 在 TUI 仍持有会话租约时，另一个进程写入同一会话，会把它分叉成一个新 ID 的会话。

会话信息 SHALL 包含 `hybrid` 对象，至少含 `side`、`phase`、`dormant`、`state`、`agent` 与 `agentSessionId`（尚未得知时为 `null`）。普通会话 SHALL NOT 含该对象。

#### Scenario: 切换期间不存在两个进程

- **WHEN** 会话从 `tui` 切到 `acp`，同时用进程表持续观察
- **THEN** 在 TUI 进程退出之前，不出现由 webmux 为该会话启动的 ACP 进程

#### Scenario: 普通会话的负载不变

- **WHEN** 客户端读取普通会话的会话信息
- **THEN** 负载中没有 `hybrid` 字段，其余字段与本变更前一致

### Requirement: 从终端切到聊天

收到切到 `acp` 的请求且会话在 `tui` 端时，系统 SHALL 先按 `agent-session-identity` 的资格判定确认可以切换，不满足时返回 409 并附原因，不改变会话。确认可以切换后，系统 SHALL 进入 `switching`，按以下顺序执行：

1. 根据终端信号判断 TUI 是否在执行回合；在执行时，向 PTY 写入档案定义的中断键，并等待信号变为空闲；
2. 结束 TUI 进程，并等待其进程组清理完成；
3. 按档案启动 ACP 进程，调用 `initialize`，再用资格判定得到的 agent 会话 ID 与 cwd 调用 `session/load`；
4. 用记录中的配置项取值补齐 agent 未返回或回到默认值的配置（按档案判断）；
5. 进入 `acp` 端。

切换期间，所有设备的输入 SHALL 被禁用，已连接的终端客户端 SHALL 以关闭码 4409 断开并收到“已切换到聊天视图”的原因。

#### Scenario: 空闲时切换

- **WHEN** TUI 空闲，用户点击“聊天”
- **THEN** 会话在一次切换内进入 `acp` 端，聊天视图回放完整历史，模型与推理强度与切换前一致

#### Scenario: 回合进行中切换

- **WHEN** TUI 正在执行回合，用户确认切到聊天
- **THEN** 系统先发送中断键并等到空闲，再退出 TUI；被中断的回合以中断结束，会话之后可以继续对话

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

会话处于 `dormant` 时 SHALL 直接从第 4 步开始。回合正在运行时，Web 端 SHALL 先请求用户确认“当前回合会被中断，已输出的内容会保留”。

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
- 会话在 `acp` 端时，ACP 进程意外退出 SHALL 触发一次自动重连：重新启动 ACP 并按档案执行 `session/resume` 或 `session/load`。当前回合 SHALL 以中断结束。重连失败时，会话 SHALL 标记为 `interrupted`。

#### Scenario: ACP 崩溃

- **WHEN** 聊天端回合进行中，ACP 进程被 SIGKILL
- **THEN** 当前回合显示为已中断，系统自动重连一次；重连成功后可以继续发送消息

### Requirement: 聊天端空闲休眠

会话在 `acp` 端时，如果没有运行中的回合、没有待答复请求、队列为空，并且最近 N 分钟没有任何命令，系统 SHALL 结束 ACP 进程并标记为 `dormant`。N 来自服务端设置 `chatIdleSleepMinutes`，默认 30，为 0 时不休眠。

- 休眠期间，会话 SHALL 仍是活动会话，不进入可恢复列表。
- 打开会话的聊天通道或提交任意命令时，系统 SHALL 自动唤醒，已缓存的事件 SHALL 保持可读。
- agent 声明了 `session/resume` 时，唤醒与崩溃重连 SHALL 使用它（不回放历史，事件日志保持连续）；否则 SHALL 使用 `session/load`，并以新的日志代际重新回放。

#### Scenario: 休眠后唤醒

- **WHEN** 会话空闲超过设定时间后，用户在手机上打开该会话并发送消息
- **THEN** ACP 进程被重新启动，消息在唤醒完成后发送，此前的对话仍然可见

### Requirement: 关闭混合会话

`DELETE /api/sessions/{id}` 作用于混合会话时，系统 SHALL：

- 在聊天端有回合运行时先取消，在终端端回合运行时先按中断流程处理；
- 再结束当前端的进程，把记录状态设为 `closed`，并从会话列表中移除该会话；
- SHALL NOT 删除记录或 agent 的会话库。

服务关机时，系统 SHALL 先结束所有混合会话的 ACP 进程，再执行现有的会话关闭流程。

#### Scenario: 关闭后可恢复

- **WHEN** 用户关闭一个处于聊天端的混合会话
- **THEN** 该会话从运行中列表消失，出现在“可恢复”分组中，标注“已关闭”

### Requirement: 认证失败的引导

聊天端的请求返回 `auth_required` 时，系统 SHALL 以错误结束当前回合，并在聊天视图中显示提示与“切到终端登录”按钮。聊天视图 SHALL NOT 尝试执行登录流程。

#### Scenario: 需要登录

- **WHEN** `session/prompt` 返回需要认证的错误
- **THEN** 回合以错误结束，页面显示“切到终端登录”按钮，点击后按切到终端的流程执行
