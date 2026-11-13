## ADDED Requirements

### Requirement: Devin 的 ACP 环境与认证

Devin 的 ACP 进程与资格探测 SHALL 复用会话环境、服务用户与 `HOME`，直接运行同一配置的 `devin acp`，复用 `~/.local/share/devin/credentials.toml` 的现有登录状态。系统 SHALL NOT 为聊天端要求独立凭据、自动执行登录或注销，也 SHALL NOT 在记录、负载或日志中保存凭据值。

聊天命令菜单 SHALL 隐藏 `login` 与 `logout`。Devin 返回认证错误或要求 `devin-browser` 登录时 SHALL 按通用 agent 错误呈现，用户须在 Devin TUI 或 `devin auth login` 完成登录。

#### Scenario: 复用现有登录

- **WHEN** 服务用户已通过 `devin auth login` 登录，未设置额外 ACP 凭据
- **THEN** 聊天端以该身份创建和恢复会话，不启动新的登录流程

#### Scenario: 认证命令不出现在菜单

- **WHEN** 用户在 Devin 混合会话的聊天视图中输入 `/`
- **THEN** 命令菜单中没有 `/login` 或 `/logout`

### Requirement: Devin 的目录级工作区信任

`agent_pretrust` 开启且启动命令识别为 `devin` 时，系统 SHALL 在首次启动前把会话目录追加到 `trusted_workspaces.json` 的 `trusted_paths`，使本次 TUI、恢复 TUI 与 ACP 进程不再停留在 `Trust <dir>?` 提示。该文件位于 `$XDG_DATA_HOME/devin/cli/`（未设置时为 `~/.local/share/devin/cli/`）。

文件缺失时 SHALL 连同父目录创建最小 JSON；已有目录 SHALL NOT 重复追加；已有文件的其他字段保持不变。

系统 SHALL 沿用现有文件夹信任边界：主目录、其祖先与文件系统根 SHALL NOT 被自动信任。`agent_pretrust=false` 时 SHALL NOT 写入任何信任记录；写入失败 SHALL 仅记日志而不阻断启动，Devin 退回自带的信任提示。

#### Scenario: 新 Devin 会话跳过启动信任页

- **WHEN** `agent_pretrust` 开启，用户在普通项目目录新建 Devin 会话
- **THEN** 该目录被追加到 `trusted_paths`，TUI 不停留在 `Trust <dir>?` 提示

#### Scenario: ACP 继承同一目录信任

- **WHEN** Devin 混合会话从 TUI 切到聊天或从聊天恢复到 TUI
- **THEN** 两端使用同一 `trusted_workspaces.json`，工作区信任与首次选择一致

#### Scenario: 拒绝过宽目录与关闭开关

- **WHEN** 用户试图在主目录、其祖先或文件系统根自动预信任 Devin，或 `agent_pretrust=false`
- **THEN** WebMux 不写入信任记录，启动流程按原规则继续

### Requirement: Devin 的聊天端配置推导

首次在聊天端打开 Devin 混合会话时，系统 SHALL 从白名单 argv 推导配置种子：

- `--model <model>` → `model=<model>`；
- `--permission-mode accept-edits` → `mode=accept-edits`；
- `--permission-mode smart` → `mode=smart`；
- `--permission-mode dangerous` → `mode=bypass`。

`--permission-mode auto` 无对应 `mode` 值，SHALL 与 `--sandbox`、`--verbosity`、`--respect-workspace-trust`、`--export` 一起列入“以下参数在聊天端不生效”，SHALL NOT 透传给 `devin acp`；只有 `--model` SHALL 按 ACP 透传规则传给 `devin acp`。

#### Scenario: 预置的权限模式

- **WHEN** 新安装的 Devin 预置以 `devin --permission-mode dangerous` 从聊天开始
- **THEN** 首次聊天端配置种子包含 `mode=bypass`，ACP 进程只执行 `devin acp`（无多余参数）

#### Scenario: 模型透传与种子一致

- **WHEN** 配置 argv 为 `["devin","--model","swe-2-high"]`
- **THEN** ACP 进程 argv 为 `["devin","acp","--model","swe-2-high"]`，配置种子包含 `model=swe-2-high`

#### Scenario: 无法映射的权限模式

- **WHEN** 配置 argv 为 `["devin","--permission-mode","auto","--sandbox"]`
- **THEN** 聊天视图列出 `--permission-mode auto` 与 `--sandbox` 在聊天端不生效，不推导 `mode` 种子

### Requirement: Devin 的会话锁与强制释放

Devin 会话按进程加写锁：`session/list` 条目的 `_meta` 可含 `isLocked` 与 `lockHolderPid`；持锁会话的 `session/load` 返回错误码 `-32015`、`errorKind` 为 `session_locked` 且 `retryable` 为真。系统 SHALL 按以下规则处理：

- 切换流程在 `session/load` 前结束 TUI，正常路径 SHALL NOT 预抢锁；
- `session/load` 返回 `session_locked` 且 `lockHolderPid` 对应进程已不存在时，系统 SHALL 先发送一次带 `_meta."cognition.ai/force": true` 的 `session/close` 释放残留锁，再重试 `session/load`；
- `lockHolderPid` 对应进程仍存活时，SHALL NOT 强制接管，按会话被外部占用的冲突错误呈现；
- `session/load` 携带 force `_meta` SHALL NOT 被用于抢锁（实测不生效）。

#### Scenario: 残留锁自动释放

- **WHEN** Devin 会话的 `session/load` 返回 `session_locked`，`lockHolderPid` 指向的进程已退出
- **THEN** 系统强制 `session/close` 后重试 `session/load`，切换继续

#### Scenario: 存活持锁者不接管

- **WHEN** Devin 会话的 `session/load` 返回 `session_locked`，`lockHolderPid` 指向的进程仍在运行
- **THEN** 系统按外部占用冲突呈现错误，不发送强制 `session/close`

### Requirement: Devin 的聊天端回合能力

Devin 档案 SHALL NOT 声明 `session/resume`：崩溃重连与切到聊天一律使用 `session/load` 回放并开启新的日志生成。系统 SHALL 复用握手中的图片与嵌入资源能力以及现有权限、表单、消息队列和取消管线；音频输入按 Devin 未声明支持处理。

Devin 未声明 WebMux 可识别的私有 steering 能力，系统 SHALL NOT 对 Devin 调用私有 steering 扩展，插话使用通用排队规则。`_cognition.ai/*` 通知与 `cognition.ai/*` 元数据 SHALL 按未知扩展忽略，SHALL NOT 进入聊天事件或持久记录。

#### Scenario: 崩溃后回放重连

- **WHEN** Devin ACP 进程异常退出后重连
- **THEN** WebMux 以 `session/load` 重连并回放历史，不调用 `session/resume`

#### Scenario: 取消进行中的回合

- **WHEN** Devin 回合正在输出，用户点击停止
- **THEN** WebMux 发送 `session/cancel`，界面按通用规则结束该回合

#### Scenario: 私有扩展被忽略

- **WHEN** `devin acp` 推送 `_cognition.ai/turn_stats` 或带 `cognition.ai/*` 元数据的更新
- **THEN** 系统不报错、不转发给聊天事件，按未知通知处理
