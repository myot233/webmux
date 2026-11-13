# shell-session-control Specification

## Purpose

让脚本和 agent 用 `webmux` 命令控制已有会话：发送输入、等待状态、读取输出与画面、订阅事件、结束与改名、发出提醒，以及访问没有专用命令的接口。

## Requirements

### Requirement: 共同的输出与退出码约定

本能力的所有命令 SHALL 经本机 API 访问服务，并遵守 `terminal-session-cli` 的连接约定。

- 数据 SHALL 只写标准输出，诊断 SHALL 只写标准错误。
- 提供 `--json` 的命令 SHALL 输出服务端契约类型的 JSON；流式命令 SHALL 输出 NDJSON，每行一个完整对象。
- 退出码：0 成功；1 服务端拒绝或操作失败；2 用法错误；3 服务未运行；124 等待超时（与 GNU `timeout` 一致）。退出码 124 SHALL 作为 `contract.ExitTimeout` 加入契约。
- 接受会话 ID 的参数 SHALL 接受 `self`。
- 这些命令 SHALL NOT 记录或回显 Token，服务端 SHALL NOT 记录 `send` 的内容或 `logs` 的输出。

#### Scenario: 管道组合

- **WHEN** 脚本执行 `webmux get self --json | jq -r .session.cwd`
- **THEN** 标准输出只有会话 JSON，`jq` 输出当前会话的启动目录

#### Scenario: 用法错误

- **WHEN** 用户执行 `webmux kill` 而不给会话 ID
- **THEN** 标准错误输出用法，退出码 2，不发送任何请求

### Requirement: 通用 API 命令

`webmux api [-X METHOD] PATH [-d DATA|@FILE|-]` SHALL 经本机 API 发送一次 HTTP 请求，并把响应体原样写到标准输出。

- `PATH` SHALL 以 `/api/` 或 `/local/v1/` 开头，可以带查询串；其他路径（包括 `/ws/`）SHALL 以退出码 2 拒绝，
  `/ws/` 的提示 SHALL 指向 `events`、`logs` 与 `chat watch`。
- 方法默认 GET；给了 `-d` 时默认 POST。`-d` 的值可以是字面 JSON、`@文件` 或 `-`（标准输入），
  SHALL 在发送前校验为 JSON，不是 JSON 时以退出码 2 结束。
- 2xx 响应 SHALL 以 0 退出；其他状态 SHALL 把服务端错误信息写到标准错误并以 1 退出，响应体仍写到标准输出。
- 服务端对会话内调用者的现有限制 SHALL 照常生效。

#### Scenario: 删除启动配置

- **WHEN** 用户执行 `webmux api -X DELETE /api/launch-configs/abc`
- **THEN** CLI 发送 DELETE，服务端返回 204 后退出码为 0

#### Scenario: 会话内读取 Token

- **WHEN** 会话内的 agent 执行 `webmux api /local/v1/token`
- **THEN** 服务端返回 403，标准错误给出拒绝原因，退出码 1

#### Scenario: WebSocket 路径

- **WHEN** 用户执行 `webmux api /ws/sessions`
- **THEN** 退出码 2，提示改用 `webmux events`

### Requirement: 会话生命周期命令

系统 SHALL 提供以下命令（读取元数据的 `webmux get` 见 `session-metadata`）：

- `webmux kill SESSION`：结束会话进程（`DELETE /api/sessions/{id}`），会话以已结束状态保留。
- `webmux rm SESSION`：移除会话（`DELETE /api/sessions/{id}?remove=1`），运行中的会话先被结束。
- `webmux rename SESSION NAME`：重命名会话（`PATCH /api/sessions/{id}`），混合会话同时更新记录名称。

会话不存在时 SHALL 以退出码 1 结束；混合会话切换中拒绝结束时 SHALL 输出服务端原因并以 1 结束。

#### Scenario: 清理自己创建的会话

- **WHEN** 脚本先用 `webmux run --detach -- make test` 得到 ID，结束后执行 `webmux rm <ID>`
- **THEN** 会话从列表消失，所有浏览器随即收到移除事件

### Requirement: 向会话发送输入

`webmux send SESSION [TEXT|-] [--enter] [--key NAME]... [--wait STATE] [--timeout DUR]` SHALL 向终端会话写入输入。

- 文字 SHALL 作为一次提交帧发出：正文为文字，`--enter` 时提交键为回车，否则为空。`-` 表示从标准输入读取全部文字。
- CLI SHALL 等待该提交的回执：`written` 以 0 退出；`partial`、`failed`、`missing` 以 1 退出并报告状态。
- `--key` 可重复，取值为 `enter`、`esc`、`tab`、`ctrl-c`、`ctrl-d`、`up`、`down`、`left`、`right`、`backspace`，
  SHALL 在文字之后按顺序作为原始输入发出。未知名称 SHALL 以退出码 2 拒绝。
- 文字与 `--key` 都没有时 SHALL 以退出码 2 结束。
- 发送 SHALL 夺取控制。连接标签 SHALL 为 `script`，从会话内调用时 SHALL 为 `script · <来源会话 ID>`。
- 会话处于聊天端或切换中时 SHALL 以 1 结束，并提示使用 `webmux chat send`。
- `--wait STATE` SHALL 在发送前订阅会话列表，只接受发送之后的状态转变，语义同 `wait --changed`。

#### Scenario: 让 shell 执行命令并等待

- **WHEN** 脚本执行 `webmux send <ID> 'make test' --enter --wait idle --timeout 10m`
- **THEN** 命令写入 PTY，CLI 在会话从 running 回到 idle 后以 0 退出，超时则以 124 退出

#### Scenario: 中断 agent

- **WHEN** 用户执行 `webmux send <ID> --key esc`
- **THEN** 会话收到一个 ESC 字节，浏览器控制指示器显示 `script`

#### Scenario: 混合会话在聊天端

- **WHEN** 目标会话处于混合会话的聊天端
- **THEN** 不写入任何字节，退出码 1，提示使用 `webmux chat send`

### Requirement: 等待会话状态

`webmux wait SESSION... --until STATE[,STATE...] [--changed] [--stable DUR] [--any] [--timeout DUR] [--json]`
SHALL 订阅 `/ws/sessions`，在会话满足条件时退出。

- `STATE` 取 `running`、`waiting`、`idle`、`exited`、`attention`。`attention` 表示会话有锁存的提醒。多个取值为“任一满足”。
- 不带 `--changed` 时，订阅时的快照已满足就立即返回；带 `--changed` 时，SHALL 忽略快照状态，只接受之后的事件。
- `--stable DUR` SHALL 要求条件持续满足 DUR，期间条件被打破则重新计时。
- 多个会话默认全部满足才返回，`--any` 时任一满足即返回。
- 满足时以 0 退出，`--json` 时输出满足条件的 `SessionInfo`（多个会话时为数组）。超时以 124 退出。
  会话被移除以 1 退出。会话已结束而条件不含 `exited` 时以 1 退出。
- 连接断开时 SHALL 重新连接并以新快照继续判断，SHALL NOT 因断线误报满足。

#### Scenario: 等 agent 需要介入

- **WHEN** 脚本执行 `webmux wait <ID> --until attention,idle --changed --timeout 30m`
- **THEN** agent 请求授权或本轮结束时 CLI 以 0 退出

#### Scenario: 已经结束

- **WHEN** 会话已经以退出码 2 结束，用户执行 `webmux wait <ID> --until exited --json`
- **THEN** 立即以 0 退出，输出的 JSON 中 `exitCode` 为 2

### Requirement: 读取会话输出

`webmux logs SESSION [--follow] [--strip] [--tail N]` SHALL 输出会话的终端输出。

- 默认 SHALL 输出 `GET /api/sessions/{id}/buffer` 的原始字节。
- `--strip` SHALL 在客户端去除 CSI、OSC、DCS 与其他 ESC 序列，把 CRLF 规范为 LF，并按单独的 CR 丢弃同一行被覆盖的内容。帮助 SHALL 说明它对全屏 TUI 不可靠。
- `--tail N` SHALL 只输出最后 N 行，按换行计数，与 `--strip` 同用时按去除后的文本计数；它与 `--follow` 互斥。
- `--follow` SHALL 以观察者身份连接终端 WebSocket：先输出回放，再持续输出新输出，SHALL NOT 夺取控制或改变尺寸。
  会话结束时 SHALL 在标准错误报告退出码并以 0 退出。
- 输出 SHALL 不受标准输出是否为终端的影响。

#### Scenario: 读取最近输出

- **WHEN** 脚本执行 `webmux logs <ID> --strip --tail 50`
- **THEN** 标准输出是最后 50 行不含控制序列的文本

#### Scenario: 跟随不夺权

- **WHEN** 浏览器正在控制会话，用户执行 `webmux logs <ID> --follow`
- **THEN** 浏览器仍是控制者，CLI 持续输出新输出

### Requirement: 订阅会话事件

`webmux events [--session ID]...` SHALL 连接 `/ws/sessions`，把每条事件原样作为一行 JSON 输出，直到被中断。

- 给了 `--session` 时，SHALL 只输出与这些会话有关的 `created`、`updated`、`removed` 事件，快照只保留这些会话。
- 断线时 SHALL 重连，并输出一条新的快照事件。

#### Scenario: 交给 jq 过滤

- **WHEN** 用户执行 `webmux events | jq -c 'select(.type=="updated") | .session.status'`
- **THEN** 每次会话状态更新输出一行状态

### Requirement: 输出临时文件夹路径

`webmux dir SESSION` SHALL 调用确保接口（`POST /api/sessions/{id}/artifacts`），并输出会话临时文件夹的绝对路径，不带引号。临时文件夹不可用时 SHALL 以 1 结束。

#### Scenario: 交付文件

- **WHEN** agent 执行 `cp report.html "$(webmux dir self)/"`
- **THEN** 报告出现在 WebMux 代码栏的“临时文件夹”根中

### Requirement: 发出提醒

`webmux notify [--session SESSION] [--kind KIND] [MESSAGE]` SHALL 经 `POST /api/sessions/{id}/attention` 让会话产生提醒；
`webmux notify --clear [--session SESSION]` SHALL 经 `DELETE /api/sessions/{id}/attention` 清除会话当前锁存的提醒。

- 会话默认为 `self`；不在会话内又没给 `--session` 时 SHALL 以退出码 2 结束。
- `KIND` 取 `notify`（默认）、`done`、`error`、`question`。
- 服务端 SHALL 以来源 `cli` 锁存该提醒，SHALL 覆盖当前锁存的提醒而不受来源分层限制，之后按原有规则清除。
- `MESSAGE` 去除首尾空白后 SHALL 不超过 200 个字符，超出时 SHALL 以 400 拒绝（CLI 在本地以 2 拒绝）。
  服务端 SHALL 只在内存中保存它，并只用于推送通知正文与提醒提示。
- 推送 SHALL 遵循现有推送规则。
- 清除 SHALL 与用户输入清除提醒的效果相同：不论提醒来源，都清除锁存值与 CLI 消息，并经 `/ws/sessions` 同步。会话没有提醒时 SHALL 返回 204。
  `--clear` 与 `--kind`、`MESSAGE` SHALL 互斥。
- 混合会话在聊天端时，CLI 提醒 SHALL 保留到 agent 报告新的状态（新的提醒或开始新一轮）。
- 推送正文 SHALL 与会话标题一样受 `include_title` 控制：为 `false` 时使用固定正文。
- `send` 写入的输入 SHALL 与用户输入一样清除提醒。

#### Scenario: 长任务完成

- **WHEN** 会话内执行 `make release; webmux notify --kind done "release 构建完成"`
- **THEN** 会话显示“本轮完成”提醒，已订阅推送的设备收到正文为“release 构建完成”的通知

#### Scenario: agent 已报告状态

- **WHEN** Claude Code 已经通过 `OSC 7501` 报告工作中，它的工具子进程执行 `webmux notify --kind question "需要确认迁移方案"`
- **THEN** 会话显示“需要回答”提醒，来源为 `cli`；之后 Claude Code 报告新的状态时按原规则清除

#### Scenario: 脚本处理完后清除提醒

- **WHEN** 编排脚本用 `chat answer` 处理完子会话的权限请求后，执行 `webmux notify --clear --session <ID>`
- **THEN** 子会话的提醒消失，所有设备上的徽章同步清除

### Requirement: 读取当前屏幕

`webmux screen SESSION [--cursor] [--json]` SHALL 输出会话终端当前画面的文本，供 agent 查看全屏 TUI 的状态。

- CLI SHALL 以观察者身份连接终端 WebSocket，取得回放快照与控制者的网格尺寸（`ControlState` 的 `cols`、`rows`），
  在客户端用终端模拟器按该尺寸重放，得到当前可见画面后断开连接。SHALL NOT 夺取控制或改变尺寸。服务端 SHALL NOT 为此做终端模拟。
- SHALL 输出纯文本：每行去掉行尾空白，末尾的空行省略。本变更不输出颜色与样式。
- `--cursor` SHALL 在标准错误输出光标的行与列（从 1 开始）。`--json` SHALL 输出 `{cols, rows, cursor: {row, col}, altScreen, stable, lines}`。
- 回放到达后，CLI SHALL 在 150 ms 无输出时即返回（`stable` 为 true；会话已退出同样视为稳定），否则最多再等 1 秒就按已收到的数据输出，
  此时 `stable` 为 false，并在标准错误提示画面仍在变化；退出码 SHALL 仍为 0。
- 回放被截断（`StreamPosition.truncated`）时，SHALL 在标准错误提示画面可能不完整。这种情况通常不影响全屏 TUI：它们会重绘整个画面。
- 会话处于聊天端时 SHALL 以 1 结束，并提示使用 `webmux chat status`。
- 终端模拟复用服务端预览已在使用的 `xterm-go/vt`，在 CLI 进程内运行；服务端 SHALL NOT 为此新增任何模拟。

#### Scenario: 看到 TUI 的权限提示

- **WHEN** Claude Code 在终端端停在一个权限选择框，编排者执行 `webmux screen <ID>`
- **THEN** 标准输出是与浏览器所见一致的画面文本，包含选择框的选项，浏览器仍是控制者

#### Scenario: 结构化读取

- **WHEN** 脚本执行 `webmux screen <ID> --json | jq -r '.lines[-1]'`
- **THEN** 输出画面最后一行的文本
