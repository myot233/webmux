# shell-chat-control Specification

## Purpose
让脚本和 agent 不打开浏览器也能驱动混合会话的聊天端：发送提示并取回回复、处理授权与提问、切换端、管理队列与运行时设置，以及恢复或清理历史会话。

## Requirements

### Requirement: 聊天命令的共同约定

`webmux chat <子命令>` SHALL 只作用于混合会话（创建会话的 `chat new` 见 `shell-session-launch`），并遵守 `shell-session-control` 的输出与退出码约定。

- 目标不是混合会话时 SHALL 以 1 结束并说明原因。
- 需要聊天端的子命令在会话处于终端端时 SHALL 以 1 结束，并提示 `webmux chat switch SESSION --to chat`。
- 每个写操作 SHALL 生成新的 `clientRequestId`，同一次调用内的重试 SHALL 复用它，使服务端去重生效。

#### Scenario: 普通终端会话

- **WHEN** 用户对一个 bash 会话执行 `webmux chat send <ID> hi`
- **THEN** 退出码 1，标准错误说明该会话不是混合会话

### Requirement: 发送提示

`webmux chat send SESSION [TEXT|-] [--attach PATH]... [--mode MODE] [--wait] [--timeout DUR] [--json]`
SHALL 调用 `POST /api/sessions/{id}/chat/prompts`。

- `-` 表示从标准输入读取提示。提示为空时 SHALL 以 2 结束。
- `--attach` 的文件 SHALL 先经会话上传接口上传，再作为附件引用。任一上传失败时 SHALL 不发送提示并以 1 结束。
- 不带 `--wait` 时，SHALL 在服务端接受后以 0 退出，并输出 `ChatPromptResult`（排队时 `queued` 为 true）。
- 发送结果 SHALL 给出可跟踪的标识：立即发送时为 `turnId` 与 `promptId`（与 `turn.start` 一致），排队时为 `itemId`，
  之后发送它的回合 SHALL 在 `turn.start` 中带同一个 `queueItemId`。
- 带 `--wait` 时，SHALL 在发送前连接 `/ws/s/{id}/chat`，跟踪这条提示对应的回合直到结束（`--mode steer` 跟踪正在进行的回合）：
  - 默认输出该回合 agent 消息的最终文本，`--json` 时输出该回合的事件数组；
  - `stopReason` 为 `end_turn` 时以 0 退出，其他原因以 1 退出，并在标准错误报告原因；
  - 回合中出现权限或问题请求时，SHALL 在标准错误输出请求 ID 与摘要并以 1 退出；
    带 `--auto-wait-requests` 时则继续等待，直到请求在其他客户端被回答；
  - 排队的提示在发送前被删除时 SHALL 以 1 退出；
  - 超时以 124 退出，SHALL NOT 取消回合。

#### Scenario: 让另一个 agent 完成任务并取回结果

- **WHEN** 编排脚本执行 `webmux chat send <ID> "修复 lint 并汇报改动" --wait --timeout 20m`
- **THEN** 回合正常结束后，标准输出是 agent 的最终回复，退出码 0

#### Scenario: 排队

- **WHEN** 回合进行中再执行 `webmux chat send <ID> "下一步"`
- **THEN** 提示进入服务端队列，输出的结果中 `queued` 为 true，所有设备看到同一队列

#### Scenario: 遇到权限请求

- **WHEN** `--wait` 期间 agent 请求执行命令的权限
- **THEN** CLI 输出请求 ID 并以 1 退出，回合保持等待

### Requirement: 观察聊天

- `webmux chat watch SESSION [--json] [--history]` SHALL 连接聊天 WebSocket，直到被中断：
  - `--json` 时 SHALL 把每个服务端帧作为一行 JSON 输出；
  - 默认 SHALL 跳过连接时回放的历史事件，`--history` 时包含它们；
  - 默认 SHALL 以文本形式输出用户与 agent 消息、工具调用标题与状态，以及回合开始和结束。
- `webmux chat status SESSION [--json]` SHALL 输出一次快照后退出，内容包括所在端、当前回合状态、队列长度和待回答请求列表（ID、类型、摘要、选项）。

#### Scenario: 列出待回答请求

- **WHEN** agent 正在等待授权，用户执行 `webmux chat status <ID> --json`
- **THEN** 输出中包含该请求的 ID 与可选的 `optionId`

### Requirement: 控制回合与请求

- `webmux chat cancel SESSION` SHALL 调用取消接口，结束当前回合。没有进行中的回合时 SHALL 以 0 退出。
- `webmux chat answer SESSION REQUEST_ID (--option ID | --content JSON | --decline | --cancel)` SHALL 调用回答接口：
  授权请求用 `--option` 选择选项；问题表单用 `--content` 提交字段值对象、`--decline` 拒绝；`--cancel` 对两者都以取消作答。
  选项与请求类型不符时 SHALL 以 2 结束；请求已被回答或不存在时 SHALL 以 1 结束。
  （`--json` 在所有命令中都是输出开关，因此结构化内容用 `--content`。）
- `webmux chat switch SESSION --to chat|terminal [--force]` SHALL 调用切换接口，并等待切换结束。
  需要用户选择时（终端 TUI 在中断超时内没有空闲），带 `--force` 时 SHALL 选择强制切换；
  不带 `--force` SHALL 选择取消切换，并以 1 结束、提示可用 `--force` 重试，不让切换停在等待选择的状态。
  服务端返回会话歧义时 SHALL 列出候选会话，并接受 `--agent-session ID` 重试。

#### Scenario: 代为批准一次

- **WHEN** 用户执行 `webmux chat answer <ID> <请求 ID> --option allow_once`
- **THEN** agent 继续执行，所有设备上的请求卡片同时关闭

#### Scenario: 切到聊天后发送

- **WHEN** 会话在终端端，脚本先执行 `webmux chat switch <ID> --to chat`，再执行 `webmux chat send <ID> 继续`
- **THEN** 切换完成后提示被发送，`webmux attach` 中的客户端按 US-Q13 被动分离

### Requirement: 混合会话历史

- `webmux chat ls [--state active|closed|interrupted]... [--repo REPO] [--json]` SHALL 调用 `GET /api/hybrid-sessions`，
  列出混合会话记录：ID、名称、agent、状态、所在端、目录、agent 会话 ID。默认列出全部状态；`--archived` SHALL 等同于 `--state closed --state interrupted`。
- `webmux chat resume RECORD [--side chat|terminal] [--agent-session ID] [--without-env] [--detach]` SHALL 调用 `POST /api/sessions/{id}/resume`：
  - 成功后行为与 `run` 相同：有终端且以终端端恢复时附着，否则输出会话 ID；
  - 启动配置已删除、需要确认缺失的环境变量时，不带 `--without-env` SHALL 以 1 结束并说明原因，带上则以 `confirmWithoutEnv` 重试；
  - 服务端报告会话歧义时，SHALL 列出候选并接受 `--agent-session` 重试。
- `webmux chat forget RECORD [--yes]` SHALL 调用 `DELETE /api/hybrid-sessions/{id}`，删除已结束或已中断的记录。
  标准输入是终端且没有 `--yes` 时，SHALL 要求确认；非交互又没有 `--yes` 时，SHALL 以 2 结束。活动中的记录 SHALL 以 1 拒绝。

#### Scenario: 恢复昨天中断的 agent

- **WHEN** 用户执行 `webmux chat ls --archived`，再执行 `webmux chat resume <记录 ID> --side chat --detach`
- **THEN** 同一 agent 会话以聊天端恢复，CLI 输出新的会话 ID

#### Scenario: 非交互删除需要确认

- **WHEN** 脚本以管道执行 `webmux chat forget <记录 ID>`
- **THEN** 不删除记录，退出码 2，提示需要 `--yes`

### Requirement: 运行时配置与队列

- `webmux chat set SESSION ID=VALUE...` SHALL 对每一项调用 `PUT .../chat/config/{configId}`：
  select 类型取 `VALUE`，boolean 类型接受 `true`/`false`。任一项失败时 SHALL 报告失败的项并以 1 结束，已成功的项不回滚。
- `webmux chat permission SESSION ask|auto` SHALL 调用 `PUT .../chat/permission-mode`。
- `webmux chat queue ls SESSION [--json]` SHALL 从聊天快照列出队列：序号、ID、文字摘要。
- `webmux chat queue rm SESSION QID`、`send-now SESSION QID`、`clear SESSION`、`resume SESSION` SHALL 分别调用队列的删除、立即发送、清空与恢复接口；
  `edit SESSION QID [TEXT|-] [--up]` SHALL 调用修改接口（`--up` 上移一位）。子命令名在会话 ID 之前，与其他命令一致。
- 这些写操作 SHALL 立即同步到所有设备。

#### Scenario: 切换模型

- **WHEN** 用户执行 `webmux chat set <ID> model=sonnet`
- **THEN** agent 的模型配置变为该值，Web 端聊天视图的配置菜单同步显示

#### Scenario: 清空排队的提示

- **WHEN** 队列中有三条提示，脚本执行 `webmux chat queue clear <ID>`
- **THEN** 队列清空，所有设备不再显示这些提示
