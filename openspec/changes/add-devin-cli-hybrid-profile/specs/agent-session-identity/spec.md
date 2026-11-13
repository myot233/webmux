## ADDED Requirements

### Requirement: Devin 的终端身份识别

OSC 7501 根记录的 `app` 为 `devin` 时，系统 SHALL 将 agent 识别为 `devin`，cwd 取根进程 cwd。`devin` TUI 不发 `7501;?` 探测而直接上报根记录，其根记录不携带会话 ID；没有可靠 ID 来源时，系统 SHALL 使用 `session/list` 候选判定。

终端信号未识别出 agent、但 PTY 根进程可执行文件为 `devin` 时，系统 SHALL 按既有根进程回退规则识别当前 Devin 配置。Devin 的会话 ID 为词组 slug（如 `luminous-acorn`），SHALL 作为不透明字符串处理，SHALL NOT 假定为 UUID 或改写格式。

#### Scenario: OSC 7501 根记录

- **WHEN** Devin TUI 发出 `state=idle:app=devin`，根进程 cwd 为 `/work/project`
- **THEN** 会话的 agent 识别为 `devin`，资格探测使用 `/work/project`

#### Scenario: 无 OSC 7501 时按进程识别

- **WHEN** Devin 混合会话的原生根进程可执行文件为 `devin`，终端没有 OSC 7501 根记录
- **THEN** 系统识别该会话的 agent 为 `devin`，不提示“未识别到支持的 Agent”

### Requirement: Devin 会话 ID 的候选判定

Devin TUI 当前没有可靠会话 ID 时，系统 SHALL 用正确 cwd 调用 `devin acp` 的 `session/list`，取更新时间晚于本次 TUI 启动的会话作为候选：

1. 没有候选时，“聊天”按钮保持不可用，提示“请先在 TUI 中发送一条消息”；
2. 切换时恰有一个候选则采用它；
3. 有多个候选时，切换 SHALL 返回 409 `session_ambiguous` 并附候选列表，由用户选择后重试；
4. 当前 TUI 是 WebMux 以 `--resume <id>` 从聊天端恢复时，SHALL 优先使用本次恢复 ID，并仍核对该 ID 能被 `session/list` 找到。

候选探测 SHALL 只初始化与列出会话，SHALL NOT 加载或恢复会话。`session/list` 返回的 slug ID SHALL 原样用于后续 `session/load` 与 TUI `--resume`，SHALL NOT 改写。条目的 `_meta.isLocked`/`lockHolderPid` 只是瞬态持锁信息，SHALL NOT 用于过滤候选。

#### Scenario: 唯一候选自动选择

- **WHEN** Devin 混合会话切到聊天，`session/list` 在当前 cwd 下恰有一个本次 TUI 启动后更新的会话
- **THEN** 系统采用该会话 ID，切换成功

#### Scenario: 多个候选提示用户

- **WHEN** 同一 cwd 下本次 Devin TUI 启动后有多个会话被更新，用户点击“聊天”
- **THEN** 服务端返回 409 `session_ambiguous` 与候选列表，前端显示候选供用户选择

#### Scenario: 首条消息之前

- **WHEN** 从终端开始的 Devin 混合会话还没有发送过消息，`session/list` 中没有本次启动后更新的会话
- **THEN** “聊天”按钮不可用，提示“请先在 TUI 中发送一条消息”

#### Scenario: 从聊天恢复来的 TUI

- **WHEN** WebMux 以 `devin --resume calm-fjord` 恢复 TUI，尚未获得新的终端会话 ID
- **THEN** 系统以 `calm-fjord` 核对切换资格，不把其他近期会话当作当前会话
