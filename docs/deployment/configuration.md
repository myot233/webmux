# 配置

本页说明怎么生成和修改 `~/.config/webmux/config.toml`、命令行怎么覆盖配置、各配置项的作用，以及状态目录里保存什么。

最短路径：直接运行 `webmux`，首次启动会自动生成带随机 Token 的配置文件；改完配置后重启服务生效。

## 常改的配置项

| 键                   | 默认值                   | 作用                                                 |
| -------------------- | ------------------------ | ---------------------------------------------------- |
| `listen`             | `127.0.0.1:7681`         | 监听地址；局域网或 VPN 直连改为 `0.0.0.0:7681`       |
| `token`              | 首次启动自动生成         | 访问用的共享 Token；规则见 [访问与安全](#访问与安全) |
| `allowed_origins`    | `[]`                     | 经反向代理或自定义域名访问时，额外允许的完整 Origin  |
| `scrollback_bytes`   | `2097152`                | 每会话保留的原始终端字节数                           |
| `idle_after_seconds` | `30`                     | 多少秒没有等待特征就判定为空闲                       |
| `default_cwd`        | `~`                      | 新建会话的默认目录                                   |
| `state_dir`          | `$XDG_STATE_HOME/webmux` | 服务端状态目录，见 [状态目录](#状态目录)             |

- 配置只在启动时读取；修改后需重启服务。
- `webmux status` 的 `drift` 会列出已修改但尚未通过重启生效的 `listen`、`token`。

## 生成与加载配置

```sh
webmux config init                 # 手动预生成 0600 的 ~/.config/webmux/config.toml，含随机 Token
# 或：mkdir -p ~/.config/webmux && cp deploy/config.example.toml ~/.config/webmux/config.toml && chmod 600 ~/.config/webmux/config.toml
```

示例见 [`deploy/config.example.toml`](../../deploy/config.example.toml)。

```sh
webmux --config ~/.config/webmux/config.toml   # 等同于 webmux serve --config ...
curl -H 'Authorization: Bearer YOUR_TOKEN' http://127.0.0.1:7681/api/health
```

### 默认路径与自动生成

- 默认路径为 `~/.config/webmux/config.toml`。
- 未显式指定 `--config` 且该文件不存在时，首次 `webmux` / `webmux serve` / `webmux start` 会自动生成它。
  - 生成时使用与 `config init` 相同的模板和随机 token，目录 0700、文件 0600，然后正常启动。
  - 启动输出会说明是新建还是使用现有配置，并提示用 `webmux status --json --reveal-token` 查看已持久化的 token。
  - 并发首次启动通过原子发布只创建一个文件，各进程读取同一 token。
- 默认配置已存在时直接读取，不修改、不重写。
- 生成失败（主目录只读、权限不足、磁盘满等）会报错退出，不会静默回退到内存默认值。
- 仅 `config show`、`config validate`、`status`、`stop` 等客户端命令只读取配置，不会触发生成。

### 优先级与覆盖

- 通过 `--config` 显式指定的文件只读：文件存在时只读取该文件，不存在时启动失败且绝不创建。
- `--config` 与 `--log-file` 支持 `~/`。
- 命令行 `--listen`、`--token` 优先于配置文件；显式传入空 token 也会覆盖配置。
- 命令行覆盖只影响本次进程，不写入新生成的默认配置。

## 命令行

- 无子命令或使用 `--listen` 等服务参数时启动服务，与 `serve` 相同；未知子命令退出码 2。
- 客户端子命令都接受 `--config`，按与服务相同的规则找到状态目录、运行时记录与本机套接字。
- `webmux start` 在不改变前台默认语义的前提下后台启动，可用 `webmux stop` 停止。

| 命令                                                      | 说明                                                                                                                               |
| --------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------- |
| `webmux serve [--log-file PATH]`                          | 启动服务；`--log-file` 把标准输出与标准错误写入文件（0600），旧文件保留为 `.1`，启动方关闭管道后服务照常运行                       |
| `webmux start [--log-file PATH]`                          | 后台启动并按 `serve` 规则加载配置；默认日志为 `<state_dir>/webmux.log`；等待就绪后输出 PID、本机链接与日志路径；已运行时可重复执行 |
| `webmux status [--json] [--reveal-token]`                 | 优先经本机 API，退回运行时记录 + TCP 健康检查；退出码 0 健康、3 未运行、1 其他；只有 `--json --reveal-token` 输出 Token            |
| `webmux stop [--timeout 15s] [--force] [--json]`          | 优先经本机 API 停止，卡死时退回 SIGTERM，超时 SIGKILL；systemd / launchd 实例需 `--force`；没有记录时不向任何进程发信号            |
| `webmux bridge`                                           | 把本机套接字桥接到标准输入输出，供 Windows 客户端经 `wsl.exe --exec webmux bridge` 使用                                            |
| `webmux config path\|show\|validate\|check\|init\|listen` | 见下表                                                                                                                             |
| `webmux version [--json]`                                 | 输出版本号；`--json` 另含控制契约版本                                                                                              |
| `webmux help`                                             | 显示全部命令                                                                                                                       |

| `config` 子命令 | 作用                                                               |
| --------------- | ------------------------------------------------------------------ |
| `path`          | 定位配置文件                                                       |
| `show`          | 查看生效配置（不含 Token）                                         |
| `validate`      | 校验配置，错误含 TOML 行列号                                       |
| `check`         | `config validate` 的别名                                           |
| `init`          | 以模板和随机 Token 初始化；已存在需 `--force`，原文件保留为 `.bak` |
| `listen`        | 原子更新配置文件中的 `listen` 地址                                 |

JSON 输出契约见 [控制面 JSON 契约](../protocol/control-json.md#控制面-json-契约)。

## 配置项

### 访问与安全

| 键                | 默认值           | 作用                                                                                              |
| ----------------- | ---------------- | ------------------------------------------------------------------------------------------------- |
| `listen`          | `127.0.0.1:7681` | HTTP / WebSocket 监听地址；默认仅监听本机回环                                                     |
| `token`           | 空               | 共享 token；首次启动自动生成并持久化                                                              |
| `allowed_origins` | `[]`             | 额外允许的完整 Origin，例如 `https://myhost.example`                                              |
| `tunnel`          | 未启用           | Cloudflare Tunnel，见 [公网访问（Cloudflare Tunnel）](remote-access.md#公网访问cloudflare-tunnel) |
| `push`            | 启用             | 注意力 Web Push，见 [手机推送（Web Push）](pwa-and-push.md#手机推送web-push)                      |

- `listen` 用于局域网或 VPN 直连时可设为 `0.0.0.0:7681`。
- 已持久化的 `token` 可用 `webmux status --json --reveal-token` 查看。
- 已有配置监听 loopback 且未启用隧道时，`token` 为空表示不认证。
- 其他监听地址或启用隧道且 `token` 为空时，每次启动临时生成 token，并打印带 `#token=` 的链接。
- 隧道模式下显式设置的 token 至少 43 字符。
- `allowed_origins` 的每一项须全小写，不能带路径、末尾斜杠或默认端口（http 的 `:80`、https 的 `:443`），否则启动失败。

> [!WARNING]
> 把 `listen` 改为非回环地址会把浏览器 Shell 暴露到网络；务必保留 token。

### 会话与目录

| 键                 | 默认值                   | 作用                                   |
| ------------------ | ------------------------ | -------------------------------------- |
| `scrollback_bytes` | `2097152`                | 每会话保留的原始终端字节数（不是行数） |
| `default_cwd`      | `~`                      | 新建会话的默认目录，支持 `~` 展开      |
| `state_dir`        | `$XDG_STATE_HOME/webmux` | 仓库清单、启动配置等服务端状态目录     |

- 未设置 `$XDG_STATE_HOME` 时，`state_dir` 为 `~/.local/state/webmux`。

### 注意力识别

| 键                   | 默认值                | 作用                                                                              |
| -------------------- | --------------------- | --------------------------------------------------------------------------------- |
| `idle_after_seconds` | `30`                  | 无等待特征时判定为空闲的秒数                                                      |
| `prompt_patterns`    | 常见提示符 / 确认模式 | 判定“可能在等你”的 Go 正则列表                                                    |
| `agent_protocol`     | `true`                | 在会话环境中设置 `WARP_CLI_AGENT_PROTOCOL_VERSION=1`，让 agent 发出结构化提醒事件 |
| `bell_attention`     | `true`                | 把终端响铃（输入 1 秒后）视为需要介入                                             |
| `program_status`     | `true`                | 识别 `OSC 7501` 程序状态报告，并由服务端应答 `OSC 7501;?` 探测                    |

### Agent 启动

| 键               | 默认值 | 作用                                                                      |
| ---------------- | ------ | ------------------------------------------------------------------------- |
| `agent_pretrust` | `true` | 启动 agent 前把启动目录写入它自己的“信任此文件夹”记录，避免会话卡在对话框 |

- 适用于 Claude Code、Codex、Copilot CLI、Antigravity CLI、Qoder、QoderCN 的持久信任记录，以及 Kimi 的进程级目录信任；见 [文件夹信任预写](../cli-agents.md#文件夹信任预写)。

### 语音识别（`[voice]`）

配置外部语音识别命令后，浏览器的聊天输入框旁与终端快捷栏会显示麦克风按钮，把采集到的语音识别为文字并插入当前输入位置（不自动回车）。

设置页的“语音输入”区域可以直接编辑识别设置，保存后立即生效，不需要重启：

- 可编辑识别命令与参数（WebMux 不提供识别服务，命令由使用者自己编写）和环境变量；
- 这些网页设置保存在状态目录的 `voice-settings.json` 并覆盖下面的 `[voice]`，“恢复为配置文件设置”会删除覆盖；
- 区域内还显示识别程序是否已配置，提供本浏览器的启用开关（默认开启，只影响当前浏览器），并链接到下面的识别协议；
- 接口见 [语音识别设置](../protocol/http-voice.md)。

```toml
[voice]
cmd = ["node", "/path/to/your-asr-provider.js"]
env = ["DASHSCOPE_API_KEY=sk-xxx"]
```

| 键          | 默认值 | 作用                                                                             |
| ----------- | ------ | -------------------------------------------------------------------------------- |
| `voice.cmd` | `[]`   | 识别程序的完整参数数组（argv），直接执行、不经 shell 展开；为空时禁用语音识别    |
| `voice.env` | `[]`   | 附加到子进程环境的 `KEY=VALUE` 列表；只保存在服务端，从不通过 `/api/config` 返回 |

#### `STDIN / STDOUT` 外部识别协议

使用者可用任意语言或脚本对接自己的云端/本地 ASR 服务，只要可执行文件满足以下标准流协议：

1. **进程生命周期**：每次在浏览器点击麦克风开始录音时，服务端通过 `exec.CommandContext` 启动一个独立的 `voice.cmd` 子进程；连接断开或识别结束时终止并清理该子进程。
2. **`STDIN`（音频输入流）**：
   - 格式固定为 **`16000 Hz`、单声道（`1 channel`）、16-bit 有符号小端序 PCM（`s16le`）裸字节流**，无 WAV/RIFF 头。
   - 浏览器录音期间按约 `20ms`（`640 bytes`）一帧持续写入子进程 `STDIN`。
   - 使用者再次点击按钮结束录音时，服务端关闭子进程 `STDIN`（发送 `EOF`），通知子进程刷出最终识别结果。
3. **`STDOUT`（识别事件流，UTF-8 按行输出）**：
   - **NDJSON 模式（推荐）**：每行输出一个 JSON 对象：
     - `{"type":"ready"}`：连接就绪，前端切入录音态并显示实时音量（若子进程启动后 `500ms` 内未报错也未输出 `ready`，服务端自动视为就绪以兼容纯文本脚本）。
     - `{"type":"partial","text":"..."}`：流式中间识别结果。
     - `{"type":"final","text":"..."}`：最终识别文本；服务端转发给浏览器后结束本次会话。
     - `{"type":"error","error":"..."}`：识别错误；浏览器弹出提示并恢复空闲态。
   - **纯文本回退模式**：若某行不是以 `{` 开头的有效 JSON，服务端将其收集并在进程以退出码 `0` 结束时合并为一条 `{"type":"final","text":"..."}` 事件返回。
4. **`STDERR`（错误诊断）**：子进程非零退出且未在 `STDOUT` 输出 `error` 事件时，服务端提取 `STDERR` 最后一行非空文本作为错误原因返回前端。

## 启动配置

所有启动命令都由使用者自定义：在设置页保存“名称 + 完整命令 + 环境变量”，新建会话时选用，或直接填写不保存的临时命令。

### 默认预设

- 新安装首次启动（`launch-configs.json` 不存在）时自动写入共 13 条启动配置：
  - Shell：`$SHELL -l -i`，未设置 `$SHELL` 时为 `/bin/sh`。
  - 十一款 CLI agent 和 omp；完整列表见 [CLI agent 接入](../cli-agents.md)。
- agent 预设按服务端 `$SHELL` 的 basename 选择启动文件包装：
  - Bash：`/usr/bin/env bash -ic 'exec -- "$@"' webmux <程序>`，读取 `~/.bashrc`。
  - Zsh：`zsh -ic 'exec -- "$@"' webmux <程序>`，读取 `~/.zshrc`。
  - 未设置 `$SHELL` 时使用 Bash；fish、nu 等其他 shell 直接执行。
- 它们是普通配置，可编辑或删除。
- 不再读取 `[[presets]]` 或 `shell`；旧配置请改为在设置页保存。
- 新安装的 omp 启动配置额外预置 `PI_FORCE_IMAGE_PROTOCOL=kitty`，以在浏览器终端启用 Kitty 图片传输；其他 agent 配置不增加该环境变量。
- 已经存在的 `launch-configs.json` 不自动修改；升级后可手动编辑自己的 omp 配置。

### 命令拆分与启动文件

- 完整命令按引号与反斜杠规则拆分为 argv，不经 shell 求值。
- 需要加载启动文件的 Open Code 配置可勾选“加载 shell 启动文件”并选择 Bash 或 Zsh，无需手写包装；编辑器保存对应的完整 argv。
- 末尾的程序名是 shell `-c` 的 `$0` 占位符，后接参数经 `"$@"` 原样传给 agent。
- WebMux 不改写 `~/.bashrc` 或 `~/.zshrc`；其中的输出、环境覆盖或提前退出同样会影响启动。

### 混合模式连接器

新安装的 Claude Code、Codex CLI、Antigravity CLI、Open Code、omp、Qoder、QoderCN 与 Kimi 配置默认勾选混合模式。

Codex 与 Claude 的聊天端分别由第三方连接器提供，webmux 不会自动安装 Node 或连接器：

```sh
npm install -g @agentclientprotocol/codex-acp
npm install -g @agentclientprotocol/claude-agent-acp  # 需要 Node.js 22+
which codex-acp claude-agent-acp  # 必须能在服务的 PATH 或“加载 shell 启动文件”配置中找到
```

- 没有连接器时对应会话照常在终端运行，页面提示安装命令；安装后 30 秒内自动重新检测。
- 每次启动聊天端前都会检查连接器；“加载 shell 启动文件”的配置会多启动一次对应的 `/usr/bin/env bash -ic` 或 `zsh -ic`。
- 资格核对还会冷启动连接器。
- 两条安装命令都跟随 npm `latest`。
- Claude 的实测基线是 Claude Code 2.1.316 与 claude-agent-acp 0.99.1，不保证未来版本兼容；旧版只提示，新版需重新验证。
- 本次未实测 npm 启动器或 macOS。
- Claude 聊天端通过 `CLAUDE_CODE_EXECUTABLE` 使用 TUI 同一个 `claude`；配置显式设置时保留配置值。
- Claude 聊天端复用服务用户、`HOME`、`CLAUDE_CONFIG_DIR` 与现有认证。
- 不要把服务账号无权读取的 Linux 凭据文件或 macOS Keychain 当作可共享凭据。
- 现有订阅登录、API key、OAuth token 或第三方 provider 由 Claude 自己管理；webmux 不保存凭据，也不在聊天端登录或注销。

> [!WARNING]
> 技术复用不扩展订阅许可，部署者须自行确认实际使用方式符合订阅与服务条款。

## 状态目录

完整文件、锁、所有者和故障行为见 [配置与持久化清单](../persistence.md)，由 `make persistence-check` 约束；下文仅说明部署边界。

| 文件（位于 `state_dir`）                          | 内容             |
| ------------------------------------------------- | ---------------- |
| `repositories.json`                               | 仓库清单         |
| `launch-configs.json`、`launch-configs.json.lock` | 启动配置及其锁   |
| `hybrid-sessions.json`（及其锁）                  | 混合会话记录     |
| `run/server.json`、`run/server.json.lock`         | 运行时记录及其锁 |
| `run/webmux.sock`                                 | 本机套接字       |

- 状态目录不保存会话、终端内容或 token。
- 目录权限 0700、文件 0600，通过原子替换写入。

### 多实例与故障

- 同一状态目录只能由一个实例使用；多个实例请配置不同的 `state_dir`。
- 第二个实例照常提供终端服务，但不写运行时记录、不创建本机套接字，并在日志中警告。
- 清单损坏、文件锁冲突或存储不可用时，仓库管理功能被禁用，但终端服务照常运行，损坏的清单不会被覆盖。

### 启动配置文件

- 文件损坏、`launch-configs.json.lock` 被另一实例持有或目录不可读写时，只有保存的启动配置功能返回 503。
  - 显式临时命令与已运行会话照常可用。
  - 损坏的文件不会被覆盖或改名，修复或删除后重启服务即可。
- 启动配置的环境变量以明文保存在该 0600 文件中。
- 首次启动时写入默认预设并创建 `launch-configs.json`。
- 文件已存在（包括删空后的空列表）时不再补写，也不会自动导入旧 TOML 的启动预设。
- `launch-configs.json` 当前为版本 `2`，每条配置可带可选的 `preview: {lines, skip}`。
  - 本次破坏性升级只接受版本 `2`；版本 `1` 文件保留原样且启动配置功能返回 503，不会自动转换。
  - 升级前备份旧配置；需保留内容时先用支持 v1 的旧版本保存为 v2，或备份后移走旧文件并在新版重新创建配置。
  - 回滚前停止新版并恢复对应版本的独立备份。

### 混合会话记录

- `hybrid-sessions.json` 只保存 webmux 会话与 agent 会话的对应关系、命令快照与状态，最多 200 条；记录已满时新建混合会话会先淘汰最近未更新的已关闭或已中断记录，运行中的记录不会被淘汰。
- 它不含环境变量值、消息、排队内容或 token。
- 文件损坏、锁冲突或不可读写时只禁用混合会话的创建与恢复，普通会话不受影响，原文件不被覆盖。
- 启动配置的混合勾选在 `launch-configs.json` 中以 `hybrid` 键保存（HTTP API 中的字段名为 `chat`），只在勾选时写入，文件仍为版本 `2`。
- 回退到不认识该字段的旧版本前，先取消所有混合勾选，否则旧版本报告启动配置文件损坏。
- 详见 [混合会话](../hybrid-sessions.md)。
