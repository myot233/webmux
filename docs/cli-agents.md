# CLI agent 接入

新安装首次创建 `launch-configs.json` 时，WebMux 写入以下十二款 CLI 的启动配置，以及 Shell 和 omp，共 14 项。
这些是可编辑、可删除的普通配置，不会自动安装或认证 CLI。

旧版安装的 `launch-configs.json` 仍可能保留 Gemini CLI（`gemini --yolo`），但自 2026-06-18 起已停止为免费个人账号及 Google AI Pro/Ultra 处理请求，个人 Google 登录不可用。
新安装默认预置已替换为 Antigravity CLI（`agy --dangerously-skip-permissions`，混合模式）。
详情见[个人账号退役公告](https://developers.google.com/gemini-code-assist/docs/deprecations/code-assist-individuals)、
[企业订阅退役安排](https://docs.cloud.google.com/gemini/docs/codeassist/sunset)与[认证说明](https://geminicli.com/docs/get-started/authentication/)。
WebMux 不承诺账号套餐或额度可用；已有安装的 `geminicli` 图标与配置仍兼容读取。

| CLI                          | 交互式命令  | 初始参数                                               | 官方说明                                                                                    |
| ---------------------------- | ----------- | ------------------------------------------------------ | ------------------------------------------------------------------------------------------- |
| Claude Code                  | `claude`    | `--dangerously-skip-permissions`，混合模式             | [使用说明](https://code.claude.com/docs/en/overview)                                        |
| Codex CLI                    | `codex`     | `--dangerously-bypass-approvals-and-sandbox`，混合模式 | [使用说明](https://developers.openai.com/codex/cli/)                                        |
| Antigravity CLI              | `agy`       | `--dangerously-skip-permissions`，混合模式             | [使用说明](https://antigravity.google/docs)                                                 |
| Cursor CLI                   | `agent`     | 无                                                     | [使用说明](https://cursor.com/docs/cli/overview)                                            |
| GitHub Copilot CLI           | `copilot`   | 无                                                     | [使用说明](https://docs.github.com/en/copilot/how-tos/copilot-cli/use-copilot-cli/overview) |
| OpenCode（配置名 Open Code） | `opencode`  | `--auto`，混合模式                                     | [使用说明](https://opencode.ai/docs/)                                                       |
| Qwen Code                    | `qwen`      | 无                                                     | [官方仓库](https://github.com/QwenLM/qwen-code)                                             |
| Kimi Code CLI                | `kimi`      | `--auto`，混合模式                                     | [官方仓库](https://github.com/MoonshotAI/kimi-code)                                         |
| CodeBuddy Code               | `codebuddy` | 无                                                     | [快速开始](https://www.codebuddy.ai/docs/cli/quickstart)                                    |
| Qoder（配置名 Qoder）        | `qoder`     | `--dangerously-skip-permissions`，混合模式             | [官方仓库](https://github.com/qoder-ai/qodercli)                                            |
| QoderCN（配置名 QoderCN）    | `qodercn`   | `--dangerously-skip-permissions`，混合模式             | [官方网站](https://qoder.cn)                                                                |
| Devin CLI                    | `devin`     | `--permission-mode dangerous`，混合模式                | [官方文档](https://docs.devin.ai/cli)                                                       |

命令按 2026-10-31 的官方资料核对；Devin 按 2026-11-13 实装的 `devin 3000.12.57` 核对。Cursor 使用 `agent`，不是编辑器启动命令 `cursor`；旧 `cursor-agent` 命令也能自动匹配品牌图标。
Kimi 对应当前 Kimi Code CLI，旧 Kimi CLI 用户应先按官方说明迁移。

## 启动与参数

1. 在运行 WebMux 的机器上按官方说明安装 CLI，并以服务用户的身份完成认证。
2. 在“新建会话”的“运行”列表中选择启动配置，在“位置”中指定目录或工作树，再创建会话。
3. 在设置页编辑完整命令、参数、环境变量及图标。参数因 CLI 而异，请查看对应程序的 `--help`。

预置 agent 命令使用“加载 shell 启动文件”包装，Bash 为 `/usr/bin/env bash -ic 'exec -- "$@"' webmux <程序> <参数>`，Zsh 为 `zsh -ic 'exec -- "$@"' webmux <程序> <参数>`。
这使 CLI 能读取 `~/.bashrc` 或 `~/.zshrc` 中的 PATH 和环境。新安装按服务端 `$SHELL` 的 basename 选择 Bash 或 Zsh；未设置时使用 Bash，fish、nu 等其他 shell 不包装。也可编辑配置为可执行文件的绝对路径，取消该包装。
命令经 argv 传递，空参数、空格和字面量 `$HOME` 等字符不被当作脚本求值。

Claude Code、Codex、Antigravity CLI、OpenCode、Qoder、QoderCN、Kimi、Devin 和 omp 的原有初始参数会自动批准工具调用；Codex 参数还关闭其沙箱。
需要逐次授权时在设置页移除这些参数。Cursor、Copilot、Qwen 和 CodeBuddy 配置保留 CLI 默认授权行为。

CLI 使用服务用户的权限、账号、环境和配置。WebMux 不代管模型密钥，不为这些命令增加进程沙箱。
缺少程序时该次会话启动会报告错误；其他启动配置和已有会话仍可使用。

## 文件夹信任预写

Claude Code、Codex、GitHub Copilot CLI、Antigravity CLI、Qoder、QoderCN、Kimi 和 Devin 首次在某个目录启动时会询问“是否信任此文件夹”，`--dangerously-skip-permissions` 或 `--auto` 一类参数不会跳过这个问题；无人看着的会话会一直停在对话框上。
`agent_pretrust`（默认开启）让 WebMux 在新建会话、命令被识别为这八款 CLI 时，先预处理启动目录的信任记录或环境，前六款的做法与 [Orca](https://www.onorca.dev/docs/agents/supported) 一致：

| CLI             | 写入位置（环境变量可改）                            | 写入内容                                           |
| --------------- | --------------------------------------------------- | -------------------------------------------------- |
| Claude Code     | `~/.claude.json`（`CLAUDE_CONFIG_DIR`）             | `projects["<目录>"].hasTrustDialogAccepted = true` |
| Codex           | `~/.codex/config.toml`（`CODEX_HOME`）              | `[projects."<目录>"]` 下 `trust_level = "trusted"` |
| GitHub Copilot  | `~/.copilot/config.json`（`COPILOT_HOME`）          | 在 `trustedFolders` 数组追加目录                   |
| Antigravity CLI | `~/.gemini/antigravity-cli/settings.json`           | 在 `trustedWorkspaces` 数组追加目录                |
| Qoder           | `~/.qoder/settings.json`（`QODER_CONFIG_DIR`）      | 在 `permissions.trustDirectories` 数组追加目录     |
| QoderCN         | `~/.qoder-cn/settings.json`（`QODERCN_CONFIG_DIR`） | 在 `permissions.trustDirectories` 数组追加目录     |
| Kimi Code CLI   | 不写配置；只补充当前会话环境                        | `KIMI_CODE_TRUST_WORKSPACE=1`                      |
| Devin CLI       | `$XDG_DATA_HOME/devin/cli/trusted_workspaces.json`  | 在 `trusted_paths` 数组追加目录                    |

- 环境变量先取会话自己的环境变量，再取服务进程的。目录先解析符号链接。Antigravity CLI 没有改变该位置的环境变量，只用主目录；Qoder 与 QoderCN 只跟随环境变量和默认目录，不跟随命令行参数 `--config-dir`。
- 只处理启动目录。主目录、它的上级目录和文件系统根永不写入或注入，因为这些 CLI 的信任范围包含其下所有子目录。
- 已有文件保留其他内容与权限，用临时文件加改名替换；已经记录过则不写。Codex 追加一个新表，保留你的注释；你显式设为 `untrusted` 的目录不会被改写。
- Claude Code、Copilot、Antigravity CLI、Qoder 与 QoderCN 的配置文件不存在时不创建，让 CLI 完成首次设置；Codex 只在 `~/.codex` 目录已存在时才创建 `config.toml`；Devin 会创建缺失的目录与文件，内容为仅含 `trusted_paths` 的最小 JSON。无法解析（例如带注释的 JSON）时原样保留。
- Kimi 使用官方的进程级目录信任，等价于对当前工作区选择“Trust this folder”，不写持久记录，也不设置全局 `require_trust=false`。恢复 TUI 与 ACP 继承同一会话环境；会话或服务环境已显式设置该变量（包括 falsy 值）时不覆盖。
- 任何失败只写一条日志，不阻止会话启动，CLI 照常显示自己的对话框。设置 `agent_pretrust = false` 时不写文件，也不注入 Kimi trust 环境。
- 只对新建会话生效，不给其他已运行会话补写。混合会话的聊天端经 ACP 启动，不依赖这个对话框；Qoder 与 QoderCN 的 ACP 同样在已信任目录中才允许切换到 bypass / auto 等特权授权模式。
- 其他 CLI：Qwen Code 的文件夹信任默认关闭，OpenCode 与 omp 没有这个对话框；Cursor CLI 与 CodeBuddy 的交互式信任记录格式尚未核实，暂未预写。

## 已有安装

升级不会改写已经存在的 `launch-configs.json`，包括用户删空后的文件。
若列表中缺少某款 CLI，在“设置 → 启动配置 → 新建启动配置”填写名称和上表的交互式命令，按需勾选“加载 shell 启动文件”并选择 Bash 或 Zsh，保存后即可跨设备选用。

图标选择”自动匹配”即可识别十二款 CLI 的直接命令或标准 Bash/Zsh 包装，也可显式选择对应品牌图标。
图标在本地打包，运行时不请求公共图标服务。修改或删除配置不会改变已经启动的会话。

旧版 TOML `[[presets]]` 和 `shell` 不再提供启动入口；需要的命令应在设置页重新保存。

## 终端、混合会话与提醒

十二款 CLI 都使用持久 PTY 终端：浏览器断线不会结束进程，可重新连接并回放缓冲；服务重启会结束会话。
十二款中 Claude Code、Codex CLI、Antigravity CLI、OpenCode、Qoder、QoderCN、Kimi 与 Devin 提供 WebMux 的混合会话支持，可在终端与聊天视图之间切换；原有 omp 也继续支持混合会话。
Claude Code 与 Codex CLI 的聊天端需要另外安装第三方 ACP 连接器，见 [混合会话支持的 agent 与连接器](hybrid-sessions/agents.md)。
Qoder 与 QoderCN 使用内置 `--acp` 标志启动 ACP，Kimi 与 Devin 使用内置 `acp` 子命令，均无需额外连接器。
其他四款使用终端视图，不能仅凭其提供 ACP 接口就视为已适配 WebMux 的切换和恢复行为。

运行、空闲与等待状态使用通用终端判断；具体的回答、授权与完成提醒取决于 CLI 发出的结构化事件、OSC 通知或 BEL。
现有 Claude Code、Codex 和 omp 提醒配置见 [使用指南](usage/attention.md)。
