# agent-attention Specification

## Purpose

让 webmux 依据 CLI agent 主动发出的终端侧信道信号（BEL、OSC 9/777 通知与 Warp cli-agent 结构化事件），准确、稳定地标记“需要用户介入”的会话，不被持续刷新的终端动画掩盖，并在不冒充其他终端的前提下让 omp 等 agent 发出结构化事件。

## Requirements

### Requirement: 侧信道信号识别不改变终端字节流

系统 SHALL 在 PTY 输出上增量识别以下信号，识别 SHALL 支持在任意字节处被拆分的读取块：

- 裸 BEL：不属于 OSC 序列结束符的 `0x07`。
- 普通通知：`ESC ] 9 ; <text> (BEL | ESC \)`，其中 `<text>` 以“数字后接分号”开头的（如 `9;4;3` 进度、ConEmu 风格子命令）SHALL NOT 视为通知。
- 普通通知：`ESC ] 777 ; notify ; <title> ; <body> (BEL | ESC \)`，`<title>` 不等于 `warp://cli-agent`。
- 结构化事件：`ESC ] 777 ; notify ; warp://cli-agent ; <json> (BEL | ESC \)`。

识别 SHALL NOT 修改、删除或延迟发送给订阅者和写入回放缓冲区的原始字节，浏览器终端渲染行为保持不变。单个 OSC 负载超过 4096 字节时 SHALL 整体忽略且不得产生任何信号；OSC 0/2 标题既有的 512 字节上限及其行为 SHALL 保持不变。

#### Scenario: OSC 结束符不是响铃

- **WHEN** 程序输出 `ESC ] 0 ; title BEL`
- **THEN** 会话标题按既有规则更新，且不产生 BEL 信号

#### Scenario: 拆分读取的结构化事件

- **WHEN** 一个 `warp://cli-agent` 事件被逐字节拆分在多次 PTY 读取中到达
- **THEN** 系统识别到与一次性到达完全相同的事件，订阅者收到的字节与 PTY 输出逐字节一致

#### Scenario: 进度序列不是通知

- **WHEN** 程序输出 `ESC ] 9 ; 4 ; 3 BEL`
- **THEN** 不产生通知信号，也不产生 BEL 信号

#### Scenario: 超长负载

- **WHEN** 程序输出负载为 5000 字节的 `OSC 777` 序列
- **THEN** 该序列不产生任何信号，后续序列的识别不受影响

### Requirement: 锁存式关注状态

会话信息 SHALL 包含可空字段 `attention`，非空时为 `{kind, source, since}`：

- `kind` ∈ `question`、`permission`、`done`、`error`、`notify`、`bell`。
- `source` ∈ `agent`（OSC 7501 根记录或 Warp 结构化事件）、`terminal`（普通通知或 BEL）。
- `since` 为 RFC 3339 时间。

信号来源 SHALL 按以下层级排序，层级只在服务端内部使用，SHALL NOT 出现在任何 API 负载中：

| 层级 | 来源                                       |
| ---- | ------------------------------------------ |
| L1   | OSC 7501 根记录（见 `program-status`）     |
| L2   | Warp `warp://cli-agent` 结构化事件         |
| L3   | OSC 9 / OSC 777 普通通知                   |
| L4   | 裸 BEL                                     |
| L5   | 启发式（提示符正则、进程树等待、输出窗口） |

信号与关注种类的映射 SHALL 为：

| 信号                                                  | kind         | source     | 层级 |
| ----------------------------------------------------- | ------------ | ---------- | ---- |
| 根记录 `blocked`，`kind` 为 `question`、`auth` 或缺省 | `question`   | `agent`    | L1   |
| 根记录 `blocked` + `kind=permission`                  | `permission` | `agent`    | L1   |
| 根记录 `done`                                         | `done`       | `agent`    | L1   |
| 根记录 `error`                                        | `error`      | `agent`    | L1   |
| 结构化 `question_asked`                               | `question`   | `agent`    | L2   |
| 结构化 `permission_request`                           | `permission` | `agent`    | L2   |
| 结构化 `stop`、`idle_prompt`                          | `done`       | `agent`    | L2   |
| 结构化 `stop_failure`                                 | `error`      | `agent`    | L2   |
| OSC 9 / OSC 777 普通通知                              | `notify`     | `terminal` | L3   |
| 裸 BEL                                                | `bell`       | `terminal` | L4   |

种类优先级 SHALL 为 `question` = `permission` > `error` > `done` > `notify` > `bell`。合并规则如下：

1. **根记录存在时，L1 是唯一权威。**
   - 根记录的 `(state, kind)` 每变化为上表中的一项，关注 SHALL 被替换为对应种类，不受种类优先级限制。种类发生变化时，`since` 更新为当前时间；种类相同时，`since` 保持不变。
   - 根记录内容未变化的重复上报 SHALL NOT 改变关注；若此前关注已被清除，也 SHALL NOT 重新置位。
   - L2–L4 信号 SHALL NOT 改变关注。
2. **没有根记录时，按种类优先级合并 L2–L4 信号。** 新信号优先级低于当前关注时，SHALL NOT 改变当前关注；优先级不低于当前关注时 SHALL 替换，种类发生变化时 `since` 更新为当前时间。低层信号 SHALL NOT 替换由更高层置位的关注。
3. **启发式（L5）只参与 `status` 合成**，SHALL NOT 置位关注，也 SHALL NOT 触发推送。

关注置位后，任何后续 PTY 输出（包括 spinner、计时器和状态栏重绘）SHALL NOT 清除或降级关注；只有 L1 根记录的迁移与下文的清除条件可以改变它。

#### Scenario: 动画不掩盖等待回答

- **WHEN** omp 发出 `question_asked` 事件后，每 100 ms 持续重绘 spinner
- **THEN** 会话的 `attention.kind` 保持 `question`，`status` 保持 `waiting`，直到满足清除条件

#### Scenario: 弱信号不降级

- **WHEN** 会话没有根记录，已有由 `question_asked` 置位的 `question` 关注，随后收到一个裸 BEL
- **THEN** 关注仍为 `question`，`since` 不变

#### Scenario: 强信号升级

- **WHEN** 会话没有根记录，已有 `kind = done` 的关注，随后收到 `permission_request`
- **THEN** 关注变为 `permission`，`since` 更新为收到该事件的时间

#### Scenario: 根记录迁移可降级

- **WHEN** 根记录从 `blocked` + `permission` 依次变为 `working`、`done`
- **THEN** 关注依次为 `permission`、无、`done`，最后的 `since` 为收到 `done` 的时间

#### Scenario: 双协议 agent 不重复提醒

- **WHEN** omp 在同一回合结束时既发出 7501 `state=done`，又发出 Warp `stop` 事件，随后又响铃一次
- **THEN** 关注只置位一次，为 `done`，`since` 为 7501 上报的时间；推送只发送一次

#### Scenario: 根记录存在时忽略低层清除

- **WHEN** 根记录为 `done`，关注为 `done`，随后收到 Warp `prompt_submit`
- **THEN** 关注仍为 `done`

#### Scenario: 重复上报不复活已清除的提醒

- **WHEN** 根记录为 `done`，用户输入清除了关注，agent 随后再次上报相同的 `state=done`
- **THEN** 关注保持为空

#### Scenario: 根记录存在时普通通知不置位

- **WHEN** Codex 的根记录为 `idle`，关注为空，随后输出 `OSC 9` 回合完成通知
- **THEN** 关注保持为空

### Requirement: 关注的清除条件

系统 SHALL 在且仅在以下任一情况清除关注：

1. 一次成功写入 PTY 的用户输入中，至少含有一个非“终端应答”的字节。“终端应答”指仅由以下内容组成的输入：
   - OSC 序列、DCS 序列；
   - 终止字节为 `c`、`R`、`n`、`I`、`O`、`t` 或以 `$y` 结尾的 CSI 序列（设备属性、光标位置、状态报告、焦点事件、窗口尺寸报告、DECRQM 模式报告）。
2. 根记录迁移为 `working`、`idle` 或 `clear`。
3. 根记录被 `OSC 133;A` 或 RIS 丢弃，且当前关注由 L1 置位。
4. 会话没有根记录时，收到结构化事件 `prompt_submit`、`permission_replied` 或 `session_start`。
5. 会话退出。

以下情况 SHALL NOT 清除关注：

- 调整终端尺寸、浏览器附着或断开、终端输出。
- 服务端写入的 7501 探测应答。
- 结构化事件 `tool_complete` 和未知事件。
- 根记录存在时收到的结构化清除事件。

#### Scenario: 用户回答后清除

- **WHEN** 会话有 `question` 关注，用户在该会话键入字符并成功写入 PTY
- **THEN** 关注被清除，会话状态按状态合成规则重新确定

#### Scenario: agent 报告恢复工作

- **WHEN** 根记录为 `blocked` + `permission`，用户在 agent 自己的界面里批准后，agent 上报 `state=working`
- **THEN** 关注被清除

#### Scenario: 终端自动应答不清除

- **WHEN** 会话有关注，浏览器终端仅回送设备属性应答 `ESC [ ? 6 2 ; 2 2 c` 或焦点事件 `ESC [ I`
- **THEN** 关注保持不变

#### Scenario: 打开会话页回放的终端应答不清除

- **WHEN** 会话有 `question` 关注，回放缓冲区中含有程序启动时发出的 DECRQM、设备属性和光标位置查询；用户打开或重新打开该会话的终端页
- **THEN** 浏览器终端对回放查询发出的自动应答（如 `ESC [ ? 2026 ; 2 $ y`）写入 PTY 后，关注保持不变

#### Scenario: 断开重连不清除

- **WHEN** 会话有 `permission` 关注，所有浏览器断开后又重新附着
- **THEN** 重新附着收到的会话状态仍包含该关注

#### Scenario: agent 崩溃回到提示符

- **WHEN** 根记录为 `blocked`，关注为 `question`，agent 崩溃后 shell 输出 `ESC ] 133 ; A BEL`
- **THEN** 根记录与关注都被清除

#### Scenario: 退出清除

- **WHEN** 有关注的会话进程退出
- **THEN** `attention` 为 null，`status` 为 `exited`

### Requirement: 关注与活动状态合成

会话 `status` SHALL 按以下顺序确定，命中即止：

1. **已退出**：`exited`。
2. **存在根记录**：由根记录决定，完全不使用启发式。
   - `blocked` → `waiting`。
   - `done`、`error` → 存在关注时为 `waiting`，否则为 `idle`。
   - `idle` → `idle`。
   - `working` → `running`；若超过 `idle_after_seconds` 没有任何 PTY 输出，则为 `idle`。
3. **存在关注**：`waiting`。
4. **会话收到过结构化 agent 信号**：会话进程生命周期内收到过任何 L1 根记录上报或 L2 结构化事件。此时只按输出新旧判定：
   - 最近 2 秒内有输出，为 `running`；
   - 超过 `idle_after_seconds` 无输出，为 `idle`；
   - 否则为 `running`。
   - 提示符正则与进程树等待判定 SHALL NOT 使用，因为这类 agent 自己会报告等待。
5. **其他情况**：沿用既有探测规则（最近输出窗口、提示符模式、进程树睡眠与空闲阈值），既有规则的判定条件 SHALL 保持不变。

普通通知（L3）与 BEL（L4）SHALL NOT 使会话进入第 4 条规则。`status` 的取值集合 SHALL 保持 `running`、`waiting`、`idle`、`exited` 不变。关注、关注种类或由根记录决定的状态发生变化时，SHALL 触发会话列表 `updated` 事件和终端连接上的会话状态帧。

#### Scenario: 有输出时仍为等待

- **WHEN** 会话存在关注且最近 2 秒内有 PTY 输出
- **THEN** `status` 为 `waiting`

#### Scenario: 7501 工作中不受静默影响

- **WHEN** 根记录为 `working`，agent 在等待模型响应期间已有 10 秒没有任何输出，`idle_after_seconds` 为默认值
- **THEN** `status` 为 `running`

#### Scenario: 7501 空闲立即生效

- **WHEN** 根记录变为 `idle`，最近 1 秒内仍有输出
- **THEN** `status` 为 `idle`

#### Scenario: 已查看的完成

- **WHEN** 根记录为 `done`，用户输入清除了关注
- **THEN** `status` 为 `idle`

#### Scenario: 结构化 agent 不触发启发式等待

- **WHEN** 只发出 Warp 事件的 agent 会话在 `prompt_submit` 后静默 3 秒，其 TUI 进程正在读终端，末行恰好匹配提示符正则
- **THEN** `status` 为 `running`，不显示“可能在等你”

#### Scenario: 无信号程序沿用旧规则

- **WHEN** 普通 shell 会话从未发出任何结构化信号
- **THEN** 其 `status` 与本变更前的探测结果一致，`attention` 为 null

#### Scenario: 普通通知不关闭启发式

- **WHEN** 只发出 OSC 9 与 BEL 的程序会话中没有关注，末行匹配提示符正则且静默超过 2 秒
- **THEN** `status` 为 `waiting`

### Requirement: 结构化事件解析的安全边界

系统 SHALL 只在负载为合法 JSON 对象、`v` 为不小于 1 的整数、`event` 为字符串时处理结构化事件；否则 SHALL 忽略该序列，且 SHALL NOT 将其降级为普通通知。

系统 SHALL 只使用 `event` 字段决定关注。此外，系统 SHALL 可以按 `agent-session-identity` 的规则，在服务端内存中提取有界、校验过的标识字段 `agent`、`plugin_version`、`session_id` 与 `cwd`，用于混合会话的身份跟踪与占用检测。

这些标识字段的使用受限：

- SHALL NOT 影响关注与状态；
- SHALL NOT 写入日志；
- SHALL NOT 出现在普通会话的任何 API 负载中。

`query`、`response`、`summary`、`tool_input` 等内容，以及普通通知的标题与正文，SHALL NOT 被存储、写入日志或通过任何 API 下发。

#### Scenario: 非法 JSON

- **WHEN** 程序输出 `OSC 777;notify;warp://cli-agent;{not json` 并以 BEL 结束
- **THEN** 不产生任何关注，也不把结束符 BEL 计为裸 BEL

#### Scenario: 不泄露正文

- **WHEN** omp 发出包含用户提问原文的 `stop` 事件
- **THEN** 会话信息中只出现 `attention.kind = done` 与 `source = agent`，任何 REST、WebSocket 负载和服务端日志中都不出现该原文

#### Scenario: 标识字段只供混合会话使用

- **WHEN** 普通会话中的 omp 发出带 `session_id` 与 `cwd` 的 `session_start` 事件
- **THEN** 该会话的关注按既有规则被清除，会话信息中不出现 `session_id` 或 `cwd`

### Requirement: BEL 信号可配置且避开输入回声

配置项 `bell_attention`（布尔，默认 `true`）为 `false` 时，裸 BEL SHALL NOT 产生关注。为 `true` 时，在最近一次清除类用户输入后 1 秒内到达的裸 BEL SHALL NOT 产生关注，以避免 shell 补全等对按键的即时响铃。

#### Scenario: Tab 补全响铃

- **WHEN** 用户在 shell 中按 Tab，shell 在 200 ms 内响铃
- **THEN** 不产生关注

#### Scenario: 空闲后的响铃

- **WHEN** 用户最后一次输入超过 1 秒后 omp 发出 BEL
- **THEN** 产生 `kind = bell`、`source = terminal` 的关注

### Requirement: PTY 环境规范化与协议变量注入

新会话的子进程环境 SHALL 移除继承自 webmux 进程的宿主终端与多路复用器身份变量，至少包括：

- `TERM_PROGRAM`、`TERM_PROGRAM_VERSION`、`LC_TERMINAL`、`LC_TERMINAL_VERSION`；
- `TMUX`、`TMUX_PANE`、`STY`、`WT_SESSION`、`WT_PROFILE_ID`、`VTE_VERSION`；
- 所有 `WARP_`、`KITTY_`、`ITERM_`、`WEZTERM_`、`GHOSTTY_` 前缀变量，以及所有 `ZELLIJ` 前缀变量。

清理后 SHALL 设置 WebMux 的虚拟终端身份：

- `TERM_PROGRAM=webmux`，以及运行服务版本的 `TERM_PROGRAM_VERSION`；
- 保留 `TERM=xterm-256color` 与 `COLORTERM=truecolor` 的兼容默认值。

启动请求显式提供的同名变量（包括 `TERM`、`COLORTERM`、`TERM_PROGRAM`、`TERM_PROGRAM_VERSION`） SHALL 优先覆盖这些默认值。`WEBMUX_SESSION_ID` 仍由服务端设置且不可覆盖。

配置项 `agent_protocol`（布尔，默认 `true`）为 `true` 时，系统 SHALL 在子进程环境中设置 `WARP_CLI_AGENT_PROTOCOL_VERSION=1`；为 `false` 时 SHALL NOT 设置。系统 SHALL NOT 设置 `TERM_PROGRAM=WarpTerminal` 或 `WARP_CLIENT_VERSION`。无论该配置如何，结构化事件一旦出现在 PTY 输出中都 SHALL 被识别。

#### Scenario: 在 tmux 中启动 webmux

- **WHEN** 开发者在 tmux 窗格内启动 webmux 并新建会话
- **THEN** 会话内 `env` 不含 `TMUX` 与 `TMUX_PANE`，因此 omp 不会用 DCS 透传包裹事件

#### Scenario: 默认注入

- **WHEN** 使用默认配置新建 omp 会话
- **THEN** omp 进程环境中 `WARP_CLI_AGENT_PROTOCOL_VERSION=1`、`TERM_PROGRAM=webmux` 与服务版本 `TERM_PROGRAM_VERSION`，且不存在宿主的 `TERM_PROGRAM=WarpTerminal`、`WARP_CLIENT_VERSION` 或 Kitty 身份变量

#### Scenario: 关闭注入

- **WHEN** 配置 `agent_protocol = false` 后新建会话
- **THEN** 子进程环境中不存在任何 `WARP_` 前缀变量；omp 仍以 BEL 报告 ask 等待和完成，系统据此产生 `bell` 关注

### Requirement: 前端展示关注种类

前端 SHALL 用 Zod 校验 `attention` 字段（可空对象，`kind` 与 `source` 为上述枚举，`since` 为带偏移的时间），校验失败按既有会话数据校验失败处理。

会话列表行与终端页状态处 SHALL 在存在关注时按种类显示文案：

- `question`：“需要回答”
- `permission`：“等待授权”
- `done`：“本轮完成”
- `error`：“出错停止”
- `notify`：“有通知”
- `bell`：“响铃提醒”

不存在关注时 SHALL 沿用既有状态文案（`waiting` 仍为“可能在等你”）。状态圆点颜色与列表计数 SHALL 继续由 `status` 决定。

#### Scenario: 列表显示等待授权

- **WHEN** 某会话收到 `permission_request`
- **THEN** 会话列表中该行显示“等待授权”，并计入“等待”数量

#### Scenario: 启发式等待保持旧文案

- **WHEN** 会话无关注但既有规则判定为 `waiting`
- **THEN** 显示“可能在等你”

### Requirement: 聊天端的关注来源

混合会话在聊天端时，关注与状态 SHALL 由聊天端的回合状态决定（映射见 `chat-view` 的“聊天端的关注映射”），来源为 `agent`，并 SHALL 沿用既有的锁存与清除语义。

切换到终端端后，关注 SHALL 被清除，并重新由终端信号决定；切换到聊天端时，终端信号留下的关注与 OSC 7501 根记录 SHALL 被清除。

#### Scenario: 切走后不残留

- **WHEN** 聊天端有一个“等待授权”的关注，用户取消回合并切到终端
- **THEN** 会话的关注被清除，之后只由 TUI 的终端信号决定
