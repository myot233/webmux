# 真实 agent 基线与验收（2026-10-28）

本文件记录 tasks 1.1–1.4 的实测结果（档案、`hybrid-agent-profiles` 与 D25 规则据此定稿），以及 8.3 的真实 agent 验收结果。

## 方法

- **版本**：omp 18.13.1；OpenCode 2.1.4（本机安装）与 2.2.0（GitHub 发行版 `opencode-linux-x64.tar.gz`）。
- **隔离**：临时 HOME `/tmp/webmux-m0/home`，用 SQLite 备份接口复制两个 agent 的凭据库（omp `agent.db`、`models.db`、`config.yml`、`models.yml`；OpenCode `opencode.db`、`cli.json`），工作目录为临时目录 `proj` 及指向它的符号链接 `proj-link`。没有连接任何已有的开发者会话或 webmux 实例。环境移除 `WEBMUX_*` 与 `XDG_*`。OpenCode 2.2.0 使用另一份 HOME 副本，避免它迁移 2.1.4 的数据库。
- **ACP**：用 Python 客户端经 stdio 收发 JSON-RPC。`initialize` 声明 `fs` 与 `terminal` 均不支持，权限请求自动选择 `allow_once`。
- **TUI**：沿用 `add-program-status-signals` 基线的做法，用 `pty.fork()` 在 120×40 伪终端中启动 agent，应答 DA1 与 OSC 7501 探测。环境与 webmux 会话一致：`TERM=xterm-256color`、`TERM_PROGRAM=webmux`、`WARP_CLI_AGENT_PROTOCOL_VERSION=1`、`WEBMUX_SESSION_ID`，并移除宿主终端变量。退出方式为对进程组发送 SIGHUP。
- **模型**：omp 使用默认模型 `anthropic/claude-opus-5-5` 与 `--model anthropic/claude-haiku-4-5`；OpenCode 使用 `opencode-go/deepseek-v4.1-flash`。回合都是一句话或一次 `sleep` 工具调用。

## 1.1 ACP 能力与会话方法

### `initialize`

| 项目                  | omp 18.13.1                                       | OpenCode 2.1.4 / 2.2.0                                                          |
| --------------------- | ------------------------------------------------- | ------------------------------------------------------------------------------- |
| `protocolVersion`     | 1                                                 | 1                                                                               |
| `agentInfo`           | `{name: "omp", title: "omp", version: "18.13.1"}` | `{name: "OpenCode", version: "2.1.4"}`（2.2.0 同形）                            |
| `loadSession`         | true                                              | true                                                                            |
| `sessionCapabilities` | `list`、`fork`、`resume`、`close`                 | `additionalDirectories`、`close`、`delete`、`fork`、`list`、`resume`            |
| `promptCapabilities`  | `image`、`embeddedContext`                        | `image`、`embeddedContext`                                                      |
| `_meta`               | 结果顶层 `_meta.steering.supported = true`        | `agentCapabilities._meta["opencode/child-session-updates"] = true`，无 steering |
| `authMethods`         | `agent`（使用本地凭据）等 2 项                    | `opencode-login` 等 2 项                                                        |
| 耗时                  | 约 0.4 秒                                         | 约 0.25 秒                                                                      |

两个 agent 的 stdout 都只有 JSON 行，stderr 为空。

### 会话方法

| 方法                 | omp                                                                                                                                                                                         | OpenCode                                                                                                                         |
| -------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------- |
| `session/new`        | 约 0.15 秒；返回 `configOptions`（`mode`、`model`、`thinking`）与 `modes`；随后通知 `available_commands_update`、`session_info_update`                                                      | 约 1.6 秒（启动私有服务）；返回 `configOptions`（`model`、`mode`，2.1.4 另有 `effort`）；随后通知 `available_commands_update`    |
| `session/prompt`     | `stopReason: end_turn`，带 `usage`；回合中出现 `agent_message_chunk`、`agent_thought_chunk`、`tool_call`、`tool_call_update`、`usage_update`、`session_info_update`、`config_option_update` | 同样以 `end_turn` 结束；回合中出现 `agent_message_chunk`、`agent_thought_chunk`、`tool_call`、`tool_call_update`、`usage_update` |
| `session/list {cwd}` | 返回 `sessionId`、`cwd`、`updatedAt`、`_meta.messageCount`、`_meta.size`                                                                                                                    | 返回 `sessionId`、`cwd`、`title`、`updatedAt`                                                                                    |
| `session/load`       | 回放 `user_message_chunk`、`agent_message_chunk`（及思考、工具调用），再发 `available_commands_update`、`session_info_update`                                                               | 回放 `user_message_chunk`、`agent_message_chunk`（及工具调用），再发 `available_commands_update`                                 |
| `session/resume`     | 不回放，只发 `available_commands_update`、`session_info_update`                                                                                                                             | 不回放，只发 `available_commands_update`                                                                                         |
| `session/close`      | 返回 `{}`，约 2 毫秒                                                                                                                                                                        | 返回 `{}`，约 3 毫秒                                                                                                             |

其他观察：

- **权限请求**：omp 运行 shell 工具前发 `session/request_permission`，选项种类为 `allow_once`、`allow_always`、`reject_once`、`reject_always`。OpenCode 在默认权限配置下没有发出权限请求。
- **`auth_required`**：OpenCode 的模型所属提供方没有凭据时，`session/prompt` 返回错误 `-32000 Authentication required: provider authentication required`。
- **提供方拒绝**：OpenCode 默认模型 `opencode-go/iceman-contributor-free` 因工作区隐私设置被上游拒绝，`session/prompt` 返回 `-32603 Internal error: Upstream request failed: …`。这类错误应作为回合错误显示，不属于 `auth_required`。
- **会话写盘时机**：ACP `session/new` 创建的会话立即出现在 `session/list` 中；TUI 中的会话要到首条消息之后才出现（omp 与 OpenCode 都如此）。

与 design 决策 9 一致：两个 agent 都声明 `loadSession`、`list`、`resume`、`close`；omp 的 steering 在 `initialize` 结果顶层的 `_meta` 中声明。

## 1.2 参数（以发行版 `--help` 与实测为准）

`opencode --help` 与 `opencode acp --help` 在 2.1.4 与 2.2.0 中逐字相同：TUI 参数为 `--standalone`、`--server <url>`、`--auto`、`-c/--continue`、`-s/--session <id>`、`--prompt <text>`、位置参数 `[<directory>]`，全局参数为 `-h/--help`、`-v/--version`、`--wizard`、`--completions <shell>`、`--log-level <level>`、`--print-logs`；`opencode acp` 只接受 `--login`。也就是说，2.1.4 已经没有 `--model`，两个版本之间没有参数差异。

`omp acp --help` 只有用法一行，但 `omp acp` 解析完整 argv，并静默忽略未知参数。实测：

| 传给 `omp acp` 的参数                                     | 结果                                                                                         |
| --------------------------------------------------------- | -------------------------------------------------------------------------------------------- |
| `--model anthropic/claude-haiku-4-5`（放在 `acp` 前或后） | `session/new` 的 `model` 为该值                                                              |
| `--thinking low`                                          | `thinking` 为 `low`                                                                          |
| `--profile m0p`                                           | 会话写入该配置档；不带 `--profile` 的进程 `session/list` 查不到，`session/load` 返回内部错误 |
| `--approval-mode yolo`、`--auto-approve`                  | 握手正常                                                                                     |
| `--yolo`、`--definitely-bogus`                            | 握手正常（omp 18.13.1 没有 `--yolo`，按未知参数忽略）                                        |

逐项结论（保留 = 写入恢复 argv；透传 = 同时传给 ACP；丢弃 = 恢复时不保留并在界面列出；不可勾选 = 配置不能开启混合模式）：

| agent    | 参数                                                                                                                                                                                                                                                                                                                                                                                           | 结论                                 | 依据                                                         |
| -------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------ | ------------------------------------------------------------ |
| OpenCode | `--standalone`                                                                                                                                                                                                                                                                                                                                                                                 | 保留（档案注入）                     | 见 1.4                                                       |
| OpenCode | `--auto`                                                                                                                                                                                                                                                                                                                                                                                       | 保留；不透传，界面提示聊天视图不支持 | `opencode acp` 只接受 `--login`                              |
| OpenCode | `--log-level <v>`、`--print-logs`                                                                                                                                                                                                                                                                                                                                                              | 保留                                 | 只影响日志                                                   |
| OpenCode | `-c/--continue`、`-s/--session <id>`、`--prompt <text>`                                                                                                                                                                                                                                                                                                                                        | 丢弃                                 | 会话类与提示词                                               |
| OpenCode | 位置参数                                                                                                                                                                                                                                                                                                                                                                                       | 不可勾选                             | 位置参数是工作目录                                           |
| OpenCode | `--server <url>`                                                                                                                                                                                                                                                                                                                                                                               | 不可勾选                             | TUI 连接另一个服务及其会话库，且与注入的 `--standalone` 冲突 |
| OpenCode | `-h`、`-v`、`--completions`、`--wizard`                                                                                                                                                                                                                                                                                                                                                        | 不可勾选                             | 不启动交互界面                                               |
| omp      | `--model`、`--smol`、`--slow`、`--plan`、`--prewalk-into`、`--provider`、`--api-key`、`--system-prompt`、`--system-prompt-template`、`--append-system-prompt`、`--profile`、`--config`、`--add-dir`、`--models`、`--tools`、`--thinking`、`--service-tier`、`--hook`、`-e/--extension`、`--skills`、`--max-time`、`--warn-cost`、`--max-cost`、`--approval-mode`、`--plugin-dir`（各带一个值） | 保留并透传                           | `omp acp` 解析完整 argv                                      |
| omp      | `--prewalk`、`--no-prewalk`、`--allow-home`、`--no-tools`、`--no-lsp`、`--no-pty`、`--hide-thinking`、`--advisor`、`--external-thinking`、`--no-extensions`、`--no-skills`、`--no-rules`、`--no-context-files`、`--no-title`、`--auto-approve`（开关）                                                                                                                                         | 保留并透传                           | 同上                                                         |
| omp      | 位置参数（提示词）、`-c/--continue`、`-r/--resume [v]`、`--restore <v>`、`--goal <v>`、`--plan-yolo`、`--plan-yolo-into <v>`、`--from-claude`、`--from-codex`、`--wait-async`、`--print-thoughts`、`--no-ui`                                                                                                                                                                                   | 丢弃                                 | 会话类、提示词、只在启动时或非交互模式下有意义               |
| omp      | `--session-dir`、`--no-session`                                                                                                                                                                                                                                                                                                                                                                | 不可勾选                             | 两端会话库不同或不保存                                       |
| omp      | `--cwd`                                                                                                                                                                                                                                                                                                                                                                                        | 不可勾选                             | 在命令中指定目录                                             |
| omp      | `-p/--print`、`--mode`、`--export`、`--alias`、`-h/--help`、`-v/--version`，首个位置参数为子命令                                                                                                                                                                                                                                                                                               | 不可勾选                             | 不启动交互界面                                               |

`--profile` 原计划可能列为不可勾选。实测它同时透传给 ACP 后，两端使用同一个配置档的会话库，因此改为保留并透传。

## 1.3 bashrc 包装的 fd 重定向

`.bashrc` 内容为 `echo "bashrc noise on stdout"` 与 `export M0_FROM_BASHRC=visible`。

```sh
bash -ic 'exec -- "$@" 1>&3 3>&-' webmux omp acp       # fd 3 为协议管道，fd 1/2 为 stderr 管道
bash -ic 'exec -- "$@" 1>&3 3>&-' webmux opencode acp
bash -ic 'exec -- "$@"' webmux omp acp                 # 对照：不加重定向
```

| 情况              | 结果                                                                                                                                                            |
| ----------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| omp，带 fd 3      | 握手与回合成功；协议通道 0 行非 JSON；stderr 依次为 bash 的两行作业控制提示和 `bashrc noise on stdout`；让 agent 运行 `printenv M0_FROM_BASHRC`，回复 `visible` |
| OpenCode，带 fd 3 | 同上，回复 `visible`                                                                                                                                            |
| omp，不加重定向   | `bashrc noise on stdout` 出现在协议通道的第一行（非 JSON），按规格会被视为协议错误                                                                              |

结论：design 决策 7 的写法成立。`bash -i` 在没有终端时向 stderr 输出两行作业控制提示，只在启动失败时作为错误详情显示。

## 1.4 退出、中断、`--standalone`、符号链接与 D25

### 身份信号（webmux 环境下的 TUI）

- **omp**：启动 0.5 秒内上报根记录 `state=idle:app=omp`，以及 Warp `session_start` 事件（含 `session_id`、`cwd`、`plugin_version`）。之后的 `prompt_submit`、`tool_complete`、`stop` 事件都带同一个 `session_id`，`prompt_submit` 与 `stop` 还含正文（`query`、`response`），因此 webmux 只能保留标识字段。
- **OpenCode**：启动后先发探测，应答后上报 `state=idle:app=opencode`。首条消息之后才出现子记录 `state=working:id=ses_…:app=opencode:title=<base64>`，回合结束时上报 `state=clear:id=ses_…`，随后根记录回到 `idle`。

### 空闲时 SIGHUP

| agent                   | 退出耗时     | 退出状态                     | 之后 `session/load`            |
| ----------------------- | ------------ | ---------------------------- | ------------------------------ |
| omp                     | 0.05 秒      | 因信号结束（shell 视为 129） | 回放完整的用户消息、思考与回复 |
| OpenCode `--standalone` | 0.10–0.15 秒 | 0                            | 回放完整的用户消息与回复       |

### 回合中

让 agent 运行 `sleep 20`–`sleep 30`，在工具运行时操作：

| 情况                                  | 结果                                                                                                                                                 |
| ------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------- |
| omp，按 `Esc`                         | 0.09 秒内上报 `stop` 与 `idle`；SIGHUP 后 0.05 秒退出；`load` 中没有运行中的工具调用（omp 已把 `sleep` 放到后台，等待它的 `wait` 工具记为 `failed`） |
| omp，直接 SIGHUP                      | 0.05 秒退出；`load` 中该工具调用记为 `failed`                                                                                                        |
| OpenCode `--standalone`，按 `Esc Esc` | 0.2 秒内上报 `state=clear`；SIGHUP 后 0.1 秒退出；`load` 中工具调用记为 `failed`                                                                     |
| OpenCode `--standalone`，直接 SIGHUP  | 0.1 秒退出；`sleep` 进程随之结束；25 秒后命令本应写入的标记文件不存在；`load` 中工具调用记为 `failed`，没有新的回复                                  |

结论：

- design 决策 9 的“退出方式”（回到空闲后对进程组 SIGHUP，5 秒后 SIGKILL）对两个 agent 都成立，实测远低于 5 秒。
- OpenCode 以 `--standalone` 运行时，结束 TUI 即停止回合，历史完整，没有残留运行中的工具调用。决策 14 第 4 项的前提成立，不需要退回原方案。

### 符号链接 cwd

| 情况                                                                    | omp                                  | OpenCode                                                                    |
| ----------------------------------------------------------------------- | ------------------------------------ | --------------------------------------------------------------------------- |
| ACP 用链接路径 `session/new`，再分别用链接路径与真实路径 `session/list` | 两种路径都能列出                     | 只有链接路径能列出（按字符串匹配）                                          |
| 在链接目录（`PWD` 为链接路径）中启动 TUI                                | Warp `cwd` 与根进程 cwd 都是真实路径 | 根进程 cwd 为真实路径；首条消息后，会话只出现在真实路径的 `session/list` 中 |

结论：TUI 记录的是解析后的路径。webmux 传给 ACP 的 cwd 必须先解析符号链接，否则 OpenCode 查不到 TUI 创建的会话。已写入决策 9 与 `hybrid-agent-profiles`。

### D25：加载后的配置项

在一个进程中用 `session/set_config_option` 修改配置，关闭后在新进程中 `session/load`：

| 配置项                | omp                        | OpenCode 2.1.4 | OpenCode 2.2.0 |
| --------------------- | -------------------------- | -------------- | -------------- |
| `model`               | 保留（无论之后是否有回合） | 保留           | 保留           |
| `thinking` / `effort` | 保留                       | 保留           | 无此配置项     |
| `mode`                | 回到 `default`             | 保留（`plan`） | —              |

判定方式：档案声明“加载后不保留”的配置项列表，omp 为 `["mode"]`，OpenCode 为空。切换或恢复后，只对这些配置项在返回值与记录值不同时重新 `set_config_option`。不采用“返回值等于默认值即补齐”的通用规则，因为默认值本身也可能是用户的选择。

## 对设计与规格的修订

- design 决策 9：按 1.2 定稿白名单、丢弃与不可勾选列表；omp ACP 透传全部白名单参数；补充中断键与退出耗时、加载后需补齐的配置项、身份信号与 cwd 规则。删除不存在的 omp `--yolo`。
- `hybrid-agent-profiles`：“不可勾选的参数”增加 OpenCode `--server`、omp `--cwd` 与“不启动交互界面”一类；`--profile` 明确为透传；“ACP 启动命令”改为透传 omp 白名单；“两端环境一致”增加 cwd 解析规则与场景。
- 其余规格与本基线一致，无需修改：资格条件 4 的“首条消息之前”场景与会话写盘时机相符；`session/resume` 不回放历史，与决策 10 相符。

## 8.3 真实 agent 验收

### 方法

- **构建**：当前分支的 `make web` 产物与 `CGO_ENABLED=0 go build ./cmd/webmux`，版本 `dev`。独立 `state_dir` 与配置 `/tmp/webmux-acc`，监听 `127.0.0.1:7791`，固定测试 token；HOME 为 M0 的隔离 HOME（凭据为复制件），环境移除 `WEBMUX_SESSION_ID` 与 `XDG_*`。没有连接已有的开发者服务或会话。
- **agent**：omp 18.13.2（M0 之后自动更新，高于档案记录的 18.13.1）与 OpenCode 2.1.4。使用首次启动预置的 bashrc 包装配置“omp”“Open Code”并勾选混合模式；omp 配置追加 `--model anthropic/claude-haiku-4-5`，OpenCode 在聊天中切到 `opencode-go/deepseek-v4.1-flash`。
- **驱动**：经 REST 与 WebSocket（聊天 `/ws/s/{id}/chat`、终端 `/ws/s/{id}`，令牌走子协议）操作，终端输入通过终端帧写入；`webmux run`/`attach`/`ls` 用同一二进制在 `script` 伪终端中执行。回合内容都是让 agent 记住或复述一个词，用复述结果判断两端是否同一段对话。

### 结果

| 验收点（#181）                                        | 结果             | 实测                                                                                                                                                                                                  |
| ----------------------------------------------------- | ---------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 场景 A：从聊天开始，切到 TUI 继续，再切回（omp）      | 通过             | 聊天中记住 BANANA；切到 TUI 25 毫秒，TUI 显示历史并复述；TUI 中再记 CHERRY；切回后聊天回放两轮，回答“BANANA, CHERRY”                                                                                  |
| 场景 A（OpenCode）                                    | 通过             | 聊天中记住 LEMON，TUI（`opencode --standalone`）显示历史并回答“LEMON GRAPE”，切回后回答“LEMON, GRAPE”；聊天选的模型在加载后保持                                                                       |
| 场景 B：从终端开始，切到聊天，再切回（omp、OpenCode） | 通过             | TUI 首条消息后“聊天”可用；切到聊天回放 TUI 的对话并能复述（KIWI、PLUM）；切回 TUI 后历史可见；已打开的终端 WebSocket 以 4409 关闭                                                                     |
| 回合进行中从聊天切到 TUI                              | 通过             | 工具调用 `sleep 20` 进行中切换：回合以 `cancelled` 结束，25 秒后命令应写的标记文件不存在                                                                                                              |
| 回合进行中从 TUI 切到聊天（omp）                      | 通过             | 见 1.4：Esc 0.09 秒回到空闲，加载后工具调用为 `failed`                                                                                                                                                |
| 回合进行中从 TUI 切到聊天（OpenCode）                 | 修复后通过       | 修复前：一次写入的 `\x1b\x1b` 被当作 Alt+Esc，中断无效，切换等到回合结束（23.6 秒），命令照常写出文件。修复后（两次 Esc 间隔 150 毫秒）切换 4.7 秒，文件未写出，之后可继续对话                        |
| 切到 TUI 看一眼后直接切回（OpenCode）                 | 修复后通过       | 修复前：恢复的 TUI 在输入前不上报会话 ID，切回返回 409 `no_session_id`。修复后沿用恢复的 ID，3.2 秒内“聊天”可用，无需输入                                                                             |
| 队列                                                  | 通过             | 连续发送三条：第一条直接发送，后两条 `queued: true`，按序回答 ONE、TWO、THREE                                                                                                                         |
| 插话（omp steering）                                  | 通过             | 工具调用进行中以 `steer` 发送，同一回合内回复包含插入的要求                                                                                                                                           |
| 权限请求与多设备                                      | 通过             | 两个聊天连接同时在线，第一个答复 204，迟到的答复 409 `answered`；另一连接看到请求关闭与回复                                                                                                           |
| 模型与配置往返一致                                    | 通过（见下）     | 聊天中设 sonnet、thinking off、mode plan，切到 TUI 再切回，三项保持；其间在 TUI 发出的一条消息在切回时被中断（回放中没有它的回复），因此“TUI 中完成一轮后”未单独验证                                  |
| `/new` 改变会话 ID                                    | 通过             | omp TUI 执行 `/new` 并对话一轮后出现 ID 变化提示，切到聊天加载的是新会话                                                                                                                              |
| 会话被占用                                            | 通过             | 普通会话中 `/resume` 同一 ID 后资格为 `occupied`，切换返回 409；关闭普通会话后恢复可用                                                                                                                |
| `/compact`                                            | 通过（OpenCode） | OpenCode：聊天中 `/compact` 产生 `compaction.upsert`（运行中→摘要），时间线显示“上下文已压缩”并可展开摘要。omp：命令送达，omp 回复“Nothing to compact (session too small)”，较大会话的 omp 压缩未验证 |
| 空闲休眠与唤醒                                        | 通过             | `chatIdleSleepMinutes: 1`，42 秒内进入休眠且无 `omp acp` 进程；休眠时发送的消息排队，唤醒后 5 秒内回答                                                                                                |
| 服务重启后恢复                                        | 通过             | 强制结束服务后四条记录均为“已中断”；恢复到聊天（omp）与恢复到终端（OpenCode）后历史完整                                                                                                               |
| 关闭与移除记录                                        | 通过             | `DELETE /api/sessions/{id}` 返回 204 时关闭仍在进行，此时记录仍为 `active`、移除返回 409；3 秒后记录为 `closed`（代码中先写记录再移除外壳，`removed` 事件晚于记录写入），移除返回 204，之后 404       |
| `webmux ls`                                           | 通过             | 状态列显示“waiting · 聊天”                                                                                                                                                                            |
| `webmux attach`                                       | 通过             | 聊天端拒绝附着，退出码 1；终端端附着后切到聊天，显示“已切换到聊天视图……”并以退出码 0 结束                                                                                                             |
| `webmux run --launch-config` / `webmux run -- omp`    | 通过             | 前者创建终端端混合会话；后者仍是普通会话（无 `hybrid`）                                                                                                                                               |
| bashrc 包装                                           | 通过             | 全部场景使用预置的 `bash -ic 'exec -- "$@"'` 配置                                                                                                                                                     |

未在真实 agent 上验收（由 `internal/hybrid` 测试与 E2E 覆盖）：10 秒未空闲后的“继续等待 / 强制 / 取消”选择、加载失败回滚、ACP 崩溃重连、`auth_required`。

### 发现与处理

- **OpenCode 中断键**（已修复）：档案改为按键列表 `InterruptKeys`，逐个写入并间隔 150 毫秒；假 agent 的 opencode 档案同步把单次写入的 Esc Esc 视为 Alt+Esc，`TestSwitchMidTurnInterrupts/opencode` 在修复前失败。
- **恢复的 TUI 无需输入即可切回**（已修复）：资格判定在终端未上报 ID 时使用本次运行所恢复的会话 ID；假 agent 的 opencode 档案恢复时不再立即上报 ID，新增 `TestSwitchBackWithoutTerminalInput`，修复前失败。
- **TUI 端的模型**（未修改，待决定）：恢复命令保留启动配置中的 `--model`，因此在聊天中改选的模型在 TUI 端不生效（本次 TUI 显示 Haiku 4.5，聊天为 sonnet）；切回聊天后仍是聊天选定的模型。规格只约束聊天端，是否在恢复命令中用当前配置值替换 `--model` 需要另行决定。
- **omp 版本**：验收期间 omp 自动更新到 18.13.2，高于档案的实测版本 18.13.1；档案只对低于实测版本的 agent 提示，因此不显示提示，各项行为与 18.13.1 一致。
- **思考与回复共用 messageId**（已修复）：截图发现 omp 回放历史时思考与回复使用同一个 ACP `messageId`，时间线把回复并进“已思考”。规范化事件为非回复角色的 ID 加上角色前缀，`TestTranslateContentAndMessages` 在修复前失败；修复后回放的回复全部显示。
- **切换中的连接状态**（未修改）：切换进行时会话页头部显示“重连中”，与切换进度同时出现，容易误解为连接问题。

### 截图

`docs/images/pr/acp-hybrid/real-*.png`，取自本次运行的真实服务与 agent：压缩、权限、排队各有桌面（1440×900）与手机（Pixel 7 尺寸，412×915，2.625 倍）的亮色与暗色版本；切换中为桌面亮色与手机暗色。Storybook 截图（`docs/images/pr/acp-hybrid/` 中不带 `real-` 前缀的文件）覆盖其余场景。
