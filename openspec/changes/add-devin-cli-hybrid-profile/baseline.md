# Devin CLI（`devin`）实测基线（2026-11-13）

本文件记录 `devin 3000.12.57` 在本机的 ACP JSON-RPC stdio 与 PTY TUI 实测结果，
供实现 `internal/agentprofile`、`internal/hybrid`、`internal/agenttrust` 与
`internal/acp/testdata/fakeagent` 时直接对照，无需在开发或测试环境中安装真实 `devin`。

## 1. 测试方法与环境

- **实测版本**：`devin 3000.12.57 (96542e930752)`，`devin --version` 输出；
  ACP `agentInfo` 为 `{"name":"affogato","title":"Devin Agent","version":"0.0.0-dev"}`，
  即 ACP 侧不报告 CLI 版本，版本核对以 `devin --version` 为准。
- **数据目录**：凭据在 `~/.local/share/devin/credentials.toml`（`devin auth login` 写入），
  会话库为 SQLite（`session store: Sqlite (default)`，配置项 `session_store`），
  单会话 transcript 在 `~/.local/share/devin/cli/transcripts/<sessionId>.json`，
  工作区信任记录在 `~/.local/share/devin/cli/trusted_workspaces.json`，
  全局配置在 `~/.config/devin/config.json`（`--config <path>` 可换）。混合模式不得让
  TUI 与 `devin acp` 使用不同的配置或数据目录，否则两端无法共享凭据与会话库。
- **架构事实**：`devin` TUI（REPL）本身是 ACP 客户端，会话内由 `devin acp`
  子进程提供服务，因此 TUI 与 ACP 天然共享同一 SQLite 会话库与凭据。
- **测试清理**：实测会话（`balanced-crepe`、`luminous-acorn` 等，目录
  `/tmp`、`/tmp/devinprobe`）均为测试创建，可用 `devin rm` 或 `session/delete` 清理。

## 2. ACP 协议实测（`devin acp`）

### 2.1 `initialize`

`devin acp` 对 `initialize {"protocolVersion":1}` 返回：

```json
{
  "protocolVersion": 1,
  "agentCapabilities": {
    "loadSession": true,
    "promptCapabilities": { "image": true, "audio": false, "embeddedContext": true },
    "mcpCapabilities": { "http": true, "sse": true },
    "sessionCapabilities": {
      "list": {},
      "delete": {},
      "additionalDirectories": {},
      "close": { "_meta": { "cognition.ai/force": true } }
    },
    "auth": {},
    "_meta": {
      "cognition.ai/sessionDefaultConfigOptions": true,
      "cognition.ai/multiRootWorkspace": true,
      "cognition.ai/sessionListContentSearch": true,
      "cognition.ai/sessionListOrderBy": ["updated_at", "created_at"],
      "cognition.ai/sessionListSortDirection": true,
      "cognition.ai/sessionRename": true,
      "cognition.ai/persistOnCreate": true,
      "cognition.ai/sessionShare": true,
      "cognition.ai/midTurnPrompts": true
    }
  },
  "authMethods": [
    { "id": "devin-browser", "name": "Log in with browser", "description": "Sign in via your browser" }
  ],
  "agentInfo": { "name": "affogato", "title": "Devin Agent", "version": "0.0.0-dev" },
  "_meta": { "mcpConfigPath": "/home/rainchan/.config/devin/mcp_config.json" }
}
```

关键观察：

- `loadSession` 为 `true`，`sessionCapabilities` 声明 `list`、`delete`、
  `close`，满足 WebMux 混合会话的 `session/list` 探测与关闭要求。
- 未声明 `resume`：实测 `session/resume` 返回 `-32601 Method not found`，
  档案 `ResumesSessions() == false`，崩溃重连与切回聊天一律使用 `session/load`。
- `image` 与 `embeddedContext` 均为 `true`；`audio` 为 `false`。
- `_meta` 中大量 `cognition.ai/*` 扩展（mentionTypes、terminalLifecycle、
  documentLifecycle、sessionShare、midTurnPrompts 等）本次全部不实现，按未知扩展忽略。
- 握手后即推送 `_cognition.ai/mcp/serversChanged` 通知与
  `session/update`（`available_commands_update`）：可用命令含 `login`、`logout`、
  `status`、`billing`、`workspace`、`ask`、`plan`、`code`、`smart`、`bypass`、
  `compact`、`context` 等，多数带 `cognition.ai/icon`/`category` 元数据。
- `session/update` 通知的 `sessionId` 为词组 slug（如 `luminous-acorn`）。

### 2.2 `session/new`、`session/list` 与 `configOptions`

- `session/new {"cwd": "/tmp", "mcpServers": []}` 返回 slug 格式 `sessionId`
  （`balanced-crepe`），以及 `modes` 与 `configOptions`：

```json
{
  "sessionId": "balanced-crepe",
  "modes": {
    "currentModeId": "accept-edits",
    "availableModes": [
      { "id": "accept-edits", "name": "Code" },
      { "id": "smart", "name": "Smart" },
      { "id": "ask", "name": "Ask" },
      { "id": "plan", "name": "Plan" },
      { "id": "bypass", "name": "Bypass Permissions" }
    ]
  },
  "configOptions": [
    { "id": "mode", "type": "select", "currentValue": "accept-edits", "options": ["accept-edits", "smart", "ask", "plan", "bypass"] },
    { "id": "model", "type": "select", "currentValue": "swe-2-high", "options": ["adaptive", "swe-2-high", "swe-2-flash-medium", "claude-fable-5-2-medium", "claude-opus-5-5-medium", "gpt-6-astra-medium", "gpt-6-sol-medium", "gpt-6-luna-medium", "gemini-4-carbon-medium", "..."] }
  ]
}
```

- **零消息会话不出现在 `session/list`**：仅有 `session/new` 而无首条消息的
  会话不会列出（TUI 新建会话同样在首条消息后可见），与 WebMux “请先在 TUI 中
  发送一条消息”的候选规则兼容。
- **`session/list` 条目**（cwd 过滤生效）：

```json
{
  "sessionId": "luminous-acorn",
  "cwd": "/tmp/devinprobe",
  "title": "<首条消息文本>",
  "updatedAt": "2026-11-13T08:33:41+00:00",
  "_meta": {
    "cognition.ai/createdAt": "2026-11-13T08:33:38.000Z",
    "cognition.ai/hasUserMessages": true,
    "cognition.ai/planFilePath": "/home/rainchan/.devin/plans/plan-8022def3fed46f2f.md",
    "cognition.ai/isLocked": true,
    "cognition.ai/lockHolderPid": 49779
  }
}
```

### 2.3 `session/load`、回放与 `session/close`

- `session/load` 回放顺序：`user_message_chunk` → `agent_thought_chunk` →
  `agent_message_chunk` → `session_info_update`（title）→ `available_commands_update`
  → `usage_update`，另有 `_cognition.ai/turn_stats` 私有通知；随后返回含
  `modes`/`configOptions`/`_meta` 的结果。TUI 与 ACP 创建的会话互相可见、可加载。
- `session/load` 对不存在的会话返回 `-32016 Session not found`
  （`errorKind: session_not_found`）。
- `session/close` 返回 `{}`；带 `"_meta":{"cognition.ai/force":true}` 的
  `session/close` 可强制释放另一进程持有的会话锁（见 2.5）。
- 待实施期补测：跨进程 `session/load` 后 `mode`/`model` 是否重置（决定
  `ReapplyOnLoad`）、`session/request_permission` 的选项形态、`session/cancel`
  的 `stopReason`、prompt 回合中是否回显 `user_message_chunk`。

### 2.4 会话进程锁（`session_locked`）

- Devin 会话按进程加写锁：`session/list` `_meta.isLocked`/`lockHolderPid`
  指示持锁进程。在 `devin --resume <id>` TUI 持锁时，`session/load` 返回：

```json
{
  "code": -32015,
  "message": "Session 'luminous-acorn' is already open in another process (PID 51463). Close the other instance before opening it here.",
  "data": { "cognition.ai/errorKind": "session_locked", "cognition.ai/retryable": true, "cognition.ai/lockHolderPid": 51463 }
}
```

- 实测 `session/load` 携带 `"_meta":{"cognition.ai/force":true}` **不能**抢锁；
  携带同样 `_meta` 的 `session/close` **可以**释放 TUI 持有的锁，随后
  `session/load` 成功。被强关的 TUI 发出 `state=error:app=devin` 且进程存活
  （会话已失）。WebMux 切换流程先 `EndRun` 结束 TUI 再加载，正常路径无锁冲突；
  锁只出现在崩溃残留或外部 devin 进程占用时。

## 3. 命令行参数实测（`devin --help` / `devin acp --help`）

- **透传 ACP**：`--model <v>`（`devin acp` 同样接受）。
- **仅 ACP**：`--agent-type`（summarizer/review/codemap，非默认 agent）、
  `--refusal-fallback`、`--cloud`（转云端会话库）——预置不使用；`--cloud`
  使两端会话库不同，拒绝混合。
- **仅 TUI（保留不透传）**：`--permission-mode <auto|accept-edits|smart|dangerous>`、
  `--sandbox`、`--verbosity <0-3>`、`--respect-workspace-trust [true|false]`、
  `--export [<path>]`。
- **会话选择（拒绝混合 `session_arg`）**：`-c`/`--continue`、`-r`/`--resume [<id>]`。
- **会话库/配置切换（拒绝混合 `session_store`）**：`--cloud`、`--config <path>`
  （配置文件含 `session_store` 等可改变存储的键）。
- **非交互（拒绝）**：`-p`/`--print [<prompt>]`、`-h`/`--help`、`-V`/`--version`；
  全部子命令：`acp`、`auth`、`cloud`、`desktop`、`doctor`、`forward`、`help`、
  `list`/`ls`、`mcp`、`migrate`、`models`、`plugins`、`rm`、`rules`、`sandbox`、
  `setup`、`skills`、`ssh`、`uninstall`、`update`、`version`。
- **丢弃**：`--prompt-file <file>`（初始提示词来源）、未识别参数。
- **位置参数**：`devin [PATH]...` 会打开 Devin Desktop 而非 TUI——按目录参数
  （`directory_arg`）拒绝混合；`--` 之后为初始提示词，可构造
  `InitialPrompt: ["--", prompt]`。

## 4. TUI 信号、锁与中断

- **OSC 7501**：TUI 不发 `7501;?` 探测，直接发根记录（ST 结尾）：
  `state=idle:app=devin`、`state=working:app=devin:msg=<b64>`（msg 解码为
  `Thinking`/`Typing`）、`state=done:app=devin:msg=<b64>`（`Devin finished`）、
  `state=error:app=devin:msg=<b64>`（`Devin encountered an error`）。未观察到
  `blocked` 与子记录 `id=`；会话 ID 不在终端信号中。
- **其他信号**：启动时发出 OSC 10/11 `?`、DA、kitty keyboard `\x1b[?u`、
  DSR `\x1b[6n` 查询并等待应答（探测脚本必须应答才继续）；运行中发 OSC 133
  `A;k=i`/`B`/`C` 提示符标记与 OSC 0/30 标题 `devin: <cwd>`。
- **工作区信任**：不受信目录启动时停于 `Trust <dir>?` 提示；接受后把目录追加到
  `~/.local/share/devin/cli/trusted_workspaces.json` 的 `trusted_paths` 数组，
  信任覆盖子目录；`--respect-workspace-trust false` 跳过检查。
- **中断键**：`Esc` 取消运行中的回合（`Ctrl+C` 在草稿为空时同样取消）；因此
  `InterruptKeys` 为单次 `\x1b`。
- **会话锁与 TUI**：`devin --resume <id>` 打开会话期间 `session/list` 报
  `isLocked: true`、`lockHolderPid` 为 TUI 内 ACP 子进程；TUI 退出后锁释放。

## 5. `.bashrc` 包装与 fd 3

`devin` 为原生二进制（`~/.local/bin/devin`），TUI 与 `devin acp` 均可经
`bash|zsh -ic 'exec -- "$@" …'` 启动；ACP 经 fd 3 重定向后日志走 stderr、协议
走 fd 3，与既有 `startupACPScript` 包装兼容（已补测：包装后 initialize 响应
经 fd 3 到达，日志仍走 stderr）。

## 6. 实施期补测（2026-11-13）

- **空会话不持久化**：`session/new` 成功但未发 prompt 的会话（如
  `mangrove-spectrum`）在跨进程 `session/load` 时返回 `-32016
  session_not_found`（`cognition.ai/errorKind: session_not_found`）。首个
  prompt 完成后会话才写入 SQLite。混合侧因此 SHALL NOT 在 TUI 尚未产生回合前
  假定会话可加载。
- **跨进程 `session/load` 自带配置态**：load 响应含 `modes` 与
  `configOptions`（`mode`/`model`/`thought_level`/`speed`），值为该会话持久化
  的状态（实测：会话经 `session/set_mode` 改为 `plan` 后，新进程 load 仍报
  `mode=plan`）。因此 `ReapplyOnLoad` 不需要，档案不声明。
- **活动回合不回显 `user_message_chunk`**：实测 `session/prompt` 回合仅产生
  `session_info_update`、`config_option_update`（×2）、`current_mode_update`
  （×2）、`agent_thought_chunk`、`agent_message_chunk`、`usage_update`；
  `user_message_chunk` 只出现在 `session/load` 回放中。无需 agy 式忽略规则。
- **`session/cancel` 的 `stopReason`**：实测为 `"cancelled"`（标准值）。
- **`session/set_mode`**：标准方法可用，返回 `{}`；模式随会话持久化。
- **`session/request_permission` 选项形态**：`accept-edits`/`smart` 模式下
  简单命令未触发权限请求，选项形态未捕获；WebMux 按标准 `options` 数组透传，
  留待实机验收确认。
