## MODIFIED Requirements

<!-- 前置条件：本增量修改在途变更 add-program-status-signals 新建的 program-status 能力，须在该变更归档后才能归档（见 tasks 0.1）。 -->

### Requirement: 识别 OSC 7501 根记录

系统 SHALL 在实时 PTY 输出上增量识别 `ESC ] 7501 ; <body> (BEL | ESC \)`。识别 SHALL 支持在任意字节处被拆分的读取块，SHALL NOT 修改、删除或延迟发送给订阅者和写入回放缓冲区的原始字节。

`<body>` 按以下规则解析：

- 以 `:` 分隔为若干 `key=value` 对，键与值两侧的空白 SHALL 被去除。
- 键须匹配 `[a-z]+` 且不超过 16 字节。不合法的对 SHALL 被跳过，其余对照常处理。
- 未知键 SHALL 被忽略；重复键以最后一次出现为准。
- `state` 为必需键，取值 ∈ `idle`、`working`、`done`、`blocked`、`error`、`clear`。缺少 `state` 或取值不在此集合内时，SHALL 忽略整条上报。
- `kind` 取值 ∈ `permission`、`question`、`auth`；其他取值视为缺省。`kind` 只在 `state=blocked` 时有意义；`state` 不是 `blocked` 时，`kind` SHALL 视为缺省，因此只差 `kind` 的非阻塞上报被视为相同记录。
- 含合法 `id` 的上报属于子记录，SHALL NOT 改变根记录；`id` 不合法（任一段不匹配 `[A-Za-z0-9_.+-]{1,32}`、总长超过 128 字节或超过 8 级）时 SHALL 忽略整条上报。
- 整条序列超过 4096 字节时 SHALL 整体忽略，且不影响后续序列的识别。

除 `state` 与 `kind` 外，系统 SHALL NOT 解析、存储、写入日志或通过任何 API 下发上报中的其他值，`msg`、`title`、`progress` 都属于此类。唯一的例外是身份标识：系统 SHALL 可以按 `agent-session-identity` 的规则，在服务端内存中保留根记录的 `app`，以及合法子记录 `id` 的第一段，用于混合会话的身份跟踪与占用检测。这两个值 SHALL NOT 影响根记录、关注与状态，SHALL NOT 写入日志，也 SHALL NOT 出现在普通会话的任何 API 负载中。

不含 `id` 且 `state` 合法的上报 SHALL 整体替换会话的根记录；`state=clear` SHALL 删除会话的全部记录。

#### Scenario: 根记录阻塞

- **WHEN** agent 输出 `ESC ] 7501 ; state=blocked:kind=permission:app=codex ESC \`
- **THEN** 会话的根记录为 `blocked` + `permission`，订阅者收到的字节与 PTY 输出逐字节一致

#### Scenario: 拆分读取

- **WHEN** 一条 `state=done` 的根记录上报被逐字节拆分在多次 PTY 读取中到达
- **THEN** 系统识别到与一次性到达完全相同的上报

#### Scenario: 未知状态整体忽略

- **WHEN** agent 输出 `ESC ] 7501 ; state=thinking:kind=question ESC \`
- **THEN** 根记录保持不变

#### Scenario: 子记录不改变根记录

- **WHEN** 根记录为 `working`，agent 输出 `ESC ] 7501 ; id=task/1:state=blocked:kind=question ESC \`
- **THEN** 根记录仍为 `working`

#### Scenario: 不泄露正文

- **WHEN** agent 上报 `state=error:msg=<base64 的错误原文>:title=<base64 的标题>`
- **THEN** 任何 REST、WebSocket 负载、推送负载和服务端日志中都不出现解码后的原文或标题，也不出现其 base64 形式

#### Scenario: OpenCode 的会话 ID 只用于身份跟踪

- **WHEN** 混合会话中的 OpenCode 输出 `ESC ] 7501 ; state=working:id=ses_abc ESC \`，此时根记录为 `idle`
- **THEN** 根记录仍为 `idle`，混合会话的 `hybrid.agentSessionId` 变为 `ses_abc`；同样的输出出现在普通会话中时，任何负载都不出现 `ses_abc`
