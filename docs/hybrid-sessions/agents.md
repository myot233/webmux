# 混合会话支持的 agent 与连接器

本页说明哪些 agent 能在聊天视图与终端之间切换、启动命令要满足什么条件，以及 Codex CLI 与 Claude Code 聊天端连接器的安装方法。

日常的开启、切换与恢复操作见 [混合会话](../hybrid-sessions.md)。

## 支持的 agent 与已验证版本

| Agent                    | 聊天端                                                                  | 已验证版本                                   |
| ------------------------ | ----------------------------------------------------------------------- | -------------------------------------------- |
| OpenCode                 | 无需另装                                                                | 2.1.4（2.2.0 参数相同）                      |
| omp                      | 无需另装                                                                | 18.13.1                                      |
| Codex CLI                | 需另装第三方适配器 [codex-acp](#codex-cli-与-codex-acp)                 | Codex CLI 0.165.0，codex-acp 2.3.0           |
| Claude Code              | 需另装第三方连接器 [claude-agent-acp](#claude-code-与-claude-agent-acp) | Claude Code 2.1.316，claude-agent-acp 0.99.1 |
| Antigravity CLI（`agy`） | 无需另装                                                                | 1.4.12                                       |
| Qoder 与 QoderCN         | 内置 `--acp` 标志启动 ACP，无需额外连接器                               | —                                            |
| Kimi Code CLI            | 内置 `acp` 子命令启动 ACP，无需额外连接器                               | 2.5.2                                        |

- 更低版本只在聊天视图显示提示，不阻止切换。
- 更高版本不自动视为已验证。
- 连接器未安装时，“聊天 | 终端”控件的聊天端禁用并显示原因，终端端照常使用。

## 启动命令兼容要求

能开启混合模式的命令需满足以下全部条件：

- 直接执行 `opencode …` / `omp …` / `codex …` / `claude …` / `agy …` / `qodercli …` / `kimi …`，或用“加载 shell 启动文件”保存的 Bash/Zsh 包装。
  - 其他 shell 包装与 `env` 前缀不受支持。
- 不能在命令里指定工作目录，请在新建会话时选择目录。受影响的参数：
  - OpenCode 的位置参数；
  - omp 的 `--cwd`；
  - Codex 的 `-C`/`--cd` 与 `--worktree`；
  - Claude Code 的 `-w`/`--worktree` 与 `--tmux`；
  - Kimi 的 `--environment-cwd`。
- 不能使用让两端看到不同会话库的参数：
  - omp 的 `--session-dir`、`--no-session`；
  - OpenCode 的 `--server`；
  - Codex 的 `--remote`；
  - Claude Code 的 `--no-session-persistence`、`--environment`；
  - Antigravity CLI 的 `--project`、`--new-project`；
  - Kimi 的 `--environment`。
- 不能使用不启动交互界面的参数（`-p`/`--print`、`--version`、非交互子命令等）。
- 不能使用打开已有会话的参数（`codex resume`/`fork`、Claude Code 的 `--continue`/`--resume`/`--session-id` 等）。
- Claude Code 的 `CLAUDE_CONFIG_DIR` 与 Kimi 的 `KIMI_CODE_HOME` 可在启动配置环境中设置，两端会使用同一个配置目录。
- Kimi 的 `--continue`/`--session` 会作为旧会话选择参数丢弃，不会进入新的恢复或 ACP 命令；`--agent`/`--agent-file` 也不会与恢复参数一起保留。

## 识别 agent 会话

切到聊天前，webmux 需要得到 agent 会话 ID，并确认 agent 的会话列表能查到它。

- OpenCode、omp、Codex、Claude Code、Antigravity CLI、Qoder、QoderCN 与 Kimi 都要在发出首条消息后，`session/list` 才包含可加载的会话。
- 各 agent 的会话 ID 来源：
  - Codex：它打开的写锁文件（`$CODEX_HOME/thread-writer-locks/`）；
  - Claude Code：进程注册记录，见 [会话识别与占用检测](#会话识别与占用检测)；
  - Antigravity CLI：注入的会话级 `--settings` `title` 钩子经 `OSC 7501` 子记录上报；
  - Kimi：不把 OSC 7501 子记录当作会话 ID，使用同一 cwd 下的 `session/list` 候选；根记录 `app=kimi-code` 或原生 `kimi` 根进程用于识别 agent。
- 读不到时按本次 TUI 启动后更新过的会话判定，有多个候选时让你选择。
- 由 webmux 切换或恢复打开的 TUI 直接沿用它恢复的会话 ID，不必先发消息就能切回。
- 为了核对，webmux 会短暂启动一次 agent 只列出会话，只在会话 ID 变化后与切换前进行。这次启动的副作用：
  - omp 每次启动会写 presence 文件并可能检查更新；
  - OpenCode 会拉起私有服务；
  - Codex 的 codex-acp 冷启动约 2.5 秒。

## Kimi Code CLI

- 聊天端直接运行同一服务用户的 `kimi acp`，复用其登录状态和 `KIMI_CODE_HOME`，不自动登录或注销；命令菜单隐藏 `/login` 与 `/logout`。
- 启动参数不会透传给 ACP 子命令。`-m`/`--model` 映射模型，`--auto`、`--yolo`、`--plan` 映射同名模式；`--skills-dir` 与 `--add-dir` 会列为聊天端不生效。
- 冷 `session/load` 与 `session/resume` 后，模型与思考等级由 Kimi 保留，WebMux 只重应用记录的模式。底层模式已生效时，无论冷恢复还是手动选择，`Already in <mode> mode` 都按幂等成功处理并显示目标模式，其他配置错误照常提示。崩溃重连使用无历史回放的 `session/resume`，插话使用通用队列而不是私有 steering。
- 浏览器草稿和 API 向混合或普通 Kimi TUI 提交文本与 Enter 时，都在同一回执内分隔 200ms，避免连续写入被 Kimi 当作粘贴内容；延迟期间输入锁继续阻止其他输入交错，空正文只发 Enter 时不等待，其他 agent 的提交方式不变。
- `agent_pretrust` 开启时为首次启动注入 `KIMI_CODE_TRUST_WORKSPACE=1`，恢复 TUI 与 ACP 继承同一环境；它只信任当前工作区，不写持久配置，详见 [文件夹信任预写](../cli-agents.md#文件夹信任预写)。

## Codex CLI 与 codex-acp

Codex CLI 没有自己的 ACP 模式，聊天端由第三方适配器 [codex-acp](https://github.com/agentclientprotocol/codex-acp) 提供，需要在服务器上另外安装（需要 Node）：

```sh
npm install -g @agentclientprotocol/codex-acp
```

### 安装检测

- webmux 不会自动安装或临时下载它。
- 每次启动聊天端之前（从聊天开始、切到聊天、恢复到聊天与重连）都按启动配置的写法检查它能否找到：
  - 直接执行时查启动配置环境的 `PATH`；
  - “加载 shell 启动文件”时经同一 Bash/Zsh 包装执行 `command -v`。
- 没有安装时不启动任何聊天端进程，终端端照常使用。
  - 页面显示“未安装 Codex ACP 适配器（codex-acp）”、可复制的安装命令与说明链接。
  - 安装后 30 秒内自动重新检测，无需刷新。

### 与终端端的对应

- 聊天端通过 `CODEX_PATH` 使用终端端同一个 `codex`，并设置 `NO_BROWSER=1`；启动配置已设置同名变量时以配置为准。
- 首次切到聊天时，按启动参数设置模式、模型与推理强度，之后每次加载都重新应用：
  - `--dangerously-bypass-approvals-and-sandbox` → 完全访问，`--approve-for-me` → 自动审查，`-s read-only` / `-s workspace-write` → 同名模式；
  - `-m` → 模型，`-c model_reasoning_effort=…` → 推理强度；
  - `--search`、`--profile`、`--enable` 等参数在聊天端不生效，页面会列出。
- 聊天视图的命令菜单不提供 `/logout`（它会注销服务用户的全局 Codex 登录）。

## Claude Code 与 claude-agent-acp

Claude Code 没有原生 ACP 模式，聊天端由第三方连接器 [claude-agent-acp](https://github.com/agentclientprotocol/claude-agent-acp) 提供。服务器需要 Node.js 22 或更高版本，并另外安装连接器：

```sh
npm install -g @agentclientprotocol/claude-agent-acp
```

### 安装检测与版本

- webmux 不会自动安装 Node 或连接器，也不会用 `npx` 临时下载。
- 每次启动聊天端之前都按启动配置的 PATH 与 Bash 包装检查 `claude-agent-acp`。
- 未安装时不启动 ACP 进程，终端端照常使用，页面显示可复制的安装命令；安装后 30 秒内自动重新检测。
- 安装命令跟随 npm `latest`，不是固定版本。
- 本次实测基线是 Claude Code 2.1.316 与连接器 0.99.1，不代表最低版本或对未来 `latest` 的兼容承诺；旧版只提示，新版仍需重新验证。
- WebMux 与连接器使用 ACP v1，不启用实验性的 v2。
- 行为基线来自 Linux 原生 Claude Code 与当前 API key/第三方 provider 的真实探测。
- npm 启动器、macOS 与订阅登录未在该基线中实测，不应视为已完成这些平台或账号矩阵的验收。

### 登录与凭据

- 聊天端通过 `CLAUDE_CODE_EXECUTABLE` 使用终端端同一个 `claude`；启动配置已设置该变量时以配置为准。
- 两端还复用同一服务用户、`HOME`、`CLAUDE_CONFIG_DIR` 与环境优先级。
- 现有订阅登录、API key、OAuth token 与第三方 provider 可继续由 Claude Code 自己使用。
- webmux 不新增或保存凭据，不在聊天端登录或注销，命令菜单隐藏 `/login` 与 `/logout`。认证失败时切到终端登录。

> [!WARNING]
> 技术上复用现有登录不等于授予其他分发形态的订阅使用权。
> 部署者应确认服务账号对 Linux 凭据文件或 macOS Keychain 的访问权限，并自行确认实际使用方式符合 Claude 订阅与服务条款；webmux 不作法律或许可判断。

### 模型、推理强度与模式

- 首次进入聊天端时，从 `--model`、`--effort` 与权限参数设置模型、推理强度和模式。
  - `--permission-mode manual` 对应 ACP 的 `default`；
  - `--dangerously-skip-permissions` 优先映射为 `bypassPermissions`；
  - 其他保留但不能映射的参数会列为“聊天端不生效”。
- 冷 `session/load` 与 `session/resume` 后都重新应用记录的模型和推理强度。
- 模式以连接器从会话恢复的实际值为准，不用旧启动参数覆盖；连接器回退 `auto` 或 `bypassPermissions` 时，界面显示实际模式与通知。
- 0.99.1 声明图片、嵌入资源和插话能力，沿用通用附件与队列管线。
- 空闲时插话返回 `promptRequired`，消息回到队列而不会启动 webmux 未跟踪的回合。

### 会话识别与占用检测

- Claude Code 的主身份来源是 `<CLAUDE_CONFIG_DIR>/sessions/<pid>.json`（未设置时为 `~/.claude`），不是 transcript JSONL。
- 该注册格式未文档化，因此采用前会核对实际 agent 的 PID、UUID、规范化 cwd、进程启动标识与 PID domain：
  - 原生 Linux 直接核对根进程；
  - npm `node` 启动器核对直接 Claude 子进程。
- 原生安装器的磁盘可执行文件可能以版本号命名（例如 `2.1.316`），进程匹配覆盖这种名称，但启动命令仍按 `claude` 识别。
- 注册记录随身份观察重读，以跟踪 TUI 内的 `/new` 与 `/resume`。首条消息前即使已有 UUID，也仍受 `session/list` 门槛限制。
- 注册记录缺失、不可读或验证失败时，改用同一 cwd 下、本次 TUI 启动后更新的 `session/list` 候选；没有候选时不能切换，多个候选时由你选择。
- webmux 也会扫描同配置目录的有效注册记录，尽力发现其他仍在运行的 Claude 进程是否持有同一会话。
  - 当前进程、已退出进程与启动标识不匹配的陈旧记录不算占用。
- 注册记录只由声明该能力的混合会话按既有观察节奏读取，普通 Shell 中手动运行 Claude 不会触发扫描。
- 读取有频率、64 KiB 单条与 256 条扫描上限。
- 注册路径、PID、进程启动标识、记录正文与凭据不进入 REST/WebSocket 负载、混合记录或日志，客户端只看到解释后的 agent 名、会话 ID 与通用错误。
