## MODIFIED Requirements

### Requirement: 会话生命周期命令

系统 SHALL 提供以下命令（读取元数据的 `webmux get` 见 `session-metadata`）：

- `webmux kill SESSION`：结束会话进程（`DELETE /api/sessions/{id}`），会话以已结束状态保留。
- `webmux rm SESSION [--forget] [--yes]`：移除会话（`DELETE /api/sessions/{id}?remove=1`），运行中的会话先被结束；带聊天视图的会话保留归档。
  - `--forget` SHALL 改为调用 `DELETE /api/sessions/{id}?forget=1`，同时删除归档记录与临时文件夹；对 `active` 的带聊天视图会话先关闭再删除。
    标准输入是终端且没有 `--yes` 时，SHALL 要求确认；非交互又没有 `--yes` 时，SHALL 以 2 结束并提示需要 `--yes`。
  - 不带 `--forget` 对已归档（`closed` 或 `interrupted`）的会话执行时，SHALL 以 1 结束，并提示使用 `--forget` 删除记录。
- `webmux rename SESSION NAME`：重命名会话（`PATCH /api/sessions/{id}`），带聊天视图的会话同时更新记录名称。

会话不存在时 SHALL 以退出码 1 结束；切换中拒绝结束时 SHALL 输出服务端原因并以 1 结束。

#### Scenario: 清理自己创建的会话

- **WHEN** 脚本先用 `webmux run --detach -- make test` 得到 ID，结束后执行 `webmux rm <ID>`
- **THEN** 会话从列表消失，所有浏览器随即收到移除事件

#### Scenario: 非交互删除记录需要确认

- **WHEN** 脚本以管道执行 `webmux rm <已归档会话 ID> --forget`
- **THEN** 不删除记录，退出码 2，提示需要 `--yes`

#### Scenario: 已归档会话需要 --forget

- **WHEN** 用户对一个已中断的会话执行 `webmux rm <ID>`
- **THEN** 退出码 1，标准错误提示改用 `webmux rm <ID> --forget`

### Requirement: 向会话发送输入

`webmux send SESSION [TEXT|-] [--enter] [--key NAME]... [--attach-file PATH]... [--mode steer] [--wait STATE | --reply] [--auto-wait-requests] [--timeout DUR] [--json]`
SHALL 调用 `POST /api/sessions/{id}/input`，由服务端按会话当前视图分派，CLI SHALL NOT 为此连接终端 WebSocket。

- `-` 表示从标准输入读取全部文字。文字、`--key` 与 `--attach-file` 都没有时 SHALL 以退出码 2 结束。
- `--key` 可重复，取值为 `enter`、`esc`、`tab`、`ctrl-c`、`ctrl-d`、`up`、`down`、`left`、`right`、`backspace`，未知名称 SHALL 以退出码 2 拒绝。
- 终端视图：
  - 文字 SHALL 作为一次带回执的提交写入：正文为文字，`--enter` 时 `submit` 为 true（追加提交键），否则不追加；`--key` SHALL 在文字之后按顺序写入。
  - 服务端回执为 `written` 时以 0 退出；`partial`、`failed`、`missing` 以 409 `input_failed` 答复，CLI SHALL 以 1 退出并报告状态。
  - 写入 SHALL 夺取控制：服务端以 `script` 为控制者标签，从会话内调用时为 `script · <来源会话 ID>`。
  - `--attach-file` 或 `--mode` SHALL 被服务端以 409 `wrong_view` 拒绝，CLI 以 1 退出并报告原因，不写入任何字节。
- 聊天视图：
  - 文字 SHALL 作为提示发送，语义同 Web 端发送；`--attach-file` 的文件 SHALL 先经会话上传接口上传，再作为附件引用，任一上传失败时 SHALL 不发送并以 1 结束。
  - `--mode steer` SHALL 映射为 `mode: "steer"`，缺省为 `auto`。`--enter` SHALL 没有额外作用（提示总是提交）。
  - `--key` SHALL 被服务端以 409 `wrong_view` 拒绝，CLI 以 1 退出并报告原因。
  - 不带 `--reply` 与 `--wait` 时，SHALL 在服务端接受后以 0 退出；`--json` 时输出输入结果（`view`、`queued`、`itemId`、`turnId`、`promptId`）。
    立即发送时结果为 `turnId` 与 `promptId`（与 `turn.start` 一致），排队时为 `itemId`，之后发送它的回合 SHALL 在 `turn.start` 中带同一个 `queueItemId`。
- 会话切换中（409 `switching`）或不是 `active`（409 `not_active`）时 SHALL 以 1 结束并报告原因。
- `--wait STATE` SHALL 对两种视图都适用：在发送前订阅会话列表，只接受发送之后的状态转变，语义同 `wait --changed`。
- `--reply` SHALL 只用于聊天视图，与 `--wait` 互斥（同时给出时以 2 结束）。会话在终端视图时 SHALL 不写入任何内容，以 1 结束并提示改用 `--wait STATE`。
  带 `--reply` 时，SHALL 在发送前连接 `/ws/s/{id}/chat`，跟踪本次输入开启的回合直到结束（`--mode steer` 跟踪正在进行的回合）：
  - 默认输出该回合 agent 消息的最终文本，`--json` 时输出该回合的事件数组；
  - `stopReason` 为 `end_turn` 时以 0 退出，其他原因以 1 退出，并在标准错误报告原因；
  - 回合中出现权限或问题请求时，SHALL 在标准错误输出请求 ID 与摘要并以 1 退出；
    带 `--auto-wait-requests` 时则继续等待，直到请求在其他客户端被回答；
  - 排队的提示在发送前被删除时 SHALL 以 1 退出；
  - 超时以 124 退出，SHALL NOT 取消回合。

#### Scenario: 让 shell 执行命令并等待

- **WHEN** 脚本执行 `webmux send <ID> 'make test' --enter --wait idle --timeout 10m`
- **THEN** 命令写入 PTY，CLI 在会话从 running 回到 idle 后以 0 退出，超时则以 124 退出

#### Scenario: 中断 agent

- **WHEN** 用户对终端视图的会话执行 `webmux send <ID> --key esc`
- **THEN** 会话收到一个 ESC 字节，浏览器控制指示器显示 `script`

#### Scenario: 让另一个 agent 完成任务并取回结果

- **WHEN** 编排脚本对聊天视图的会话执行 `webmux send <ID> "修复 lint 并汇报改动" --reply --timeout 20m`
- **THEN** 回合正常结束后，标准输出是 agent 的最终回复，退出码 0

#### Scenario: 排队

- **WHEN** 聊天视图的回合进行中再执行 `webmux send <ID> "下一步" --json`
- **THEN** 提示进入服务端队列，输出的结果中 `queued` 为 true，所有设备看到同一队列

#### Scenario: 遇到权限请求

- **WHEN** `--reply` 期间 agent 请求执行命令的权限
- **THEN** CLI 输出请求 ID 并以 1 退出，回合保持等待

#### Scenario: 混合会话在聊天端

- **WHEN** 目标会话处于聊天视图，用户执行 `webmux send <ID> --key esc`
- **THEN** 聊天中没有新提示，退出码 1，标准错误报告 `wrong_view`

#### Scenario: 终端视图上的 --reply

- **WHEN** 目标会话处于终端视图，用户执行 `webmux send <ID> hi --reply`
- **THEN** 不写入任何内容，退出码 1，提示改用 `--wait STATE`

## ADDED Requirements

### Requirement: 中断会话

`webmux interrupt SESSION` SHALL 调用 `POST /api/sessions/{id}/interrupt`，由服务端按当前视图分派：聊天视图取消进行中的回合，终端视图写入档案定义的中断键或 Ctrl-C。

- 聊天视图没有进行中的回合时 SHALL 以 0 退出。
- 会话切换中或不是 `active` 时 SHALL 以 1 结束并报告服务端原因。

#### Scenario: 取消聊天回合

- **WHEN** 聊天视图的会话有回合进行中，用户执行 `webmux interrupt <ID>`
- **THEN** 回合以取消结束，退出码 0

#### Scenario: 中断 shell 命令

- **WHEN** 普通 shell 会话正在运行 `sleep 100`，用户执行 `webmux interrupt <ID>`
- **THEN** 终端收到 Ctrl-C，`sleep` 结束，退出码 0

### Requirement: 切换会话视图

`webmux view SESSION terminal|chat [--force] [--agent-session ID]` SHALL 调用 `POST /api/sessions/{id}/view`（`to` 为给定视图），并等待切换结束。

- 需要用户选择时（终端 TUI 在中断超时内没有空闲），带 `--force` 时 SHALL 选择强制切换；
  不带 `--force` SHALL 选择取消切换，并以 1 结束、提示可用 `--force` 重试，不让切换停在等待选择的状态。
- 服务端返回会话歧义时 SHALL 列出候选会话，并接受 `--agent-session ID` 重试；`--agent-session` 只在切到 `chat` 时有效，否则以 2 结束。
- 会话没有聊天视图（409 `no_chat_view`）时 SHALL 以 1 结束并说明原因。
- 每次调用 SHALL 生成新的 `clientRequestId`，同一次调用内的重试 SHALL 复用它。

#### Scenario: 切到聊天后继续

- **WHEN** 会话在终端视图，脚本先执行 `webmux view <ID> chat`，再执行 `webmux send <ID> 继续`
- **THEN** 切换完成后提示被发送，`webmux attach` 中的客户端按 US-Q13 被动分离

#### Scenario: 普通会话

- **WHEN** 用户对一个 bash 会话执行 `webmux view <ID> chat`
- **THEN** 退出码 1，标准错误说明该会话没有聊天视图

### Requirement: 恢复已归档会话

`webmux resume SESSION [--view terminal|chat] [--agent-session ID] [--without-env] [--detach]` SHALL 调用 `POST /api/sessions/{id}/resume`，只对已归档（`closed` 或 `interrupted`）的会话有效。

- `--view` 缺省为会话归档时的视图。
- 成功后行为与 `run` 相同：以终端视图恢复且有终端时附着，否则输出会话 ID。
- 启动配置已删除、需要确认缺失的环境变量时，不带 `--without-env` SHALL 以 1 结束并说明原因，带上则以 `confirmWithoutEnv` 重试。
- 服务端报告会话歧义时，SHALL 列出候选并接受 `--agent-session` 重试。
- 会话仍为 `active`（409 `not_archived`）时 SHALL 以 1 结束并说明原因。

#### Scenario: 恢复昨天中断的 agent

- **WHEN** 用户执行 `webmux ls --archived`，再执行 `webmux resume <ID> --view chat --detach`
- **THEN** 同一 agent 会话以聊天视图恢复，会话以原 ID 回到运行中列表，CLI 输出会话 ID

#### Scenario: 运行中的会话

- **WHEN** 用户对一个运行中的 shell 会话执行 `webmux resume <ID>`
- **THEN** 退出码 1，标准错误报告 `not_archived`
