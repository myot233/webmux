# CLI agent 接入

新安装首次创建 `launch-configs.json` 时，WebMux 写入以下九款 CLI 的启动配置，以及 Shell 和 omp，共 11 项。
这些是可编辑、可删除的普通配置，不会自动安装或认证 CLI。

Gemini CLI 默认入口按原名单保留，但自 2026-06-18 起已停止为免费个人账号及 Google AI Pro/Ultra 处理请求，个人 Google 登录不可用。
该入口面向付费 Gemini API Key 或仍在支持期内的 Gemini Code Assist Standard/Enterprise 认证用户。
详情见[个人账号退役公告](https://developers.google.com/gemini-code-assist/docs/deprecations/code-assist-individuals)、
[企业订阅退役安排](https://docs.cloud.google.com/gemini/docs/codeassist/sunset)与[认证说明](https://geminicli.com/docs/get-started/authentication/)。
WebMux 不承诺账号套餐或额度可用；个人账号用户可按公告迁移到 Antigravity CLI，并自行保存启动配置。

| CLI                          | 交互式命令  | 初始参数                                     | 官方说明                                                                                    |
| ---------------------------- | ----------- | -------------------------------------------- | ------------------------------------------------------------------------------------------- |
| Claude Code                  | `claude`    | `--dangerously-skip-permissions`             | [使用说明](https://code.claude.com/docs/en/overview)                                        |
| Codex CLI                    | `codex`     | `--dangerously-bypass-approvals-and-sandbox` | [使用说明](https://developers.openai.com/codex/cli/)                                        |
| Gemini CLI                   | `gemini`    | `--yolo`                                     | [快速开始](https://geminicli.com/docs/get-started/)                                         |
| Cursor CLI                   | `agent`     | 无                                           | [使用说明](https://cursor.com/docs/cli/overview)                                            |
| GitHub Copilot CLI           | `copilot`   | 无                                           | [使用说明](https://docs.github.com/en/copilot/how-tos/copilot-cli/use-copilot-cli/overview) |
| OpenCode（配置名 Open Code） | `opencode`  | `--auto`，混合模式                           | [使用说明](https://opencode.ai/docs/)                                                       |
| Qwen Code                    | `qwen`      | 无                                           | [官方仓库](https://github.com/QwenLM/qwen-code)                                             |
| Kimi Code CLI                | `kimi`      | 无                                           | [官方仓库](https://github.com/MoonshotAI/kimi-code)                                         |
| CodeBuddy Code               | `codebuddy` | 无                                           | [快速开始](https://www.codebuddy.ai/docs/cli/quickstart)                                    |

命令按 2026-10-31 的官方资料核对。Cursor 使用 `agent`，不是编辑器启动命令 `cursor`；旧 `cursor-agent` 命令也能自动匹配品牌图标。
Kimi 对应当前 Kimi Code CLI，旧 Kimi CLI 用户应先按官方说明迁移。

## 启动与参数

1. 在运行 WebMux 的机器上按官方说明安装 CLI，并以服务用户的身份完成认证。
2. 在“新建会话 → 预先输入”选择启动配置，指定目录或工作树，再创建会话。
3. 在设置页编辑完整命令、参数、环境变量及图标。参数因 CLI 而异，请查看对应程序的 `--help`。

预置 agent 命令使用“加载 ~/.bashrc”包装：`bash -ic 'exec -- "$@"' webmux <程序> <参数>`。
这使 CLI 能读取 `.bashrc` 中的 PATH 和环境；也可编辑配置为可执行文件的绝对路径，取消该包装。
命令经 argv 传递，空参数、空格和字面量 `$HOME` 等字符不被当作脚本求值。

Claude Code、Codex、Gemini、OpenCode 和 omp 的原有初始参数会自动批准工具调用；Codex 参数还关闭其沙箱。
需要逐次授权时在设置页移除这些参数。新增的 Cursor、Copilot、Qwen、Kimi 和 CodeBuddy 配置保留 CLI 默认授权行为。

CLI 使用服务用户的权限、账号、环境和配置。WebMux 不代管模型密钥，不为这些命令增加进程沙箱。
缺少程序时该次会话启动会报告错误；其他启动配置和已有会话仍可使用。

## 已有安装

升级不会改写已经存在的 `launch-configs.json`，包括用户删空后的文件。
若列表中缺少某款 CLI，在“设置 → 启动配置 → 新建启动配置”填写名称和上表的交互式命令，按需勾选“加载 ~/.bashrc”，保存后即可跨设备选用。

图标选择“自动匹配”即可识别九款 CLI 的直接命令或标准 Bash 包装，也可显式选择对应品牌图标。
图标在本地打包，运行时不请求公共图标服务。修改或删除配置不会改变已经启动的会话。

旧版 TOML `[[presets]]` 和 `shell` 不再提供启动入口；需要的命令应在设置页重新保存。

## 终端、混合会话与提醒

九款 CLI 都使用持久 PTY 终端：浏览器断线不会结束进程，可重新连接并回放缓冲；服务重启会结束会话。
目前九款中只有 OpenCode 提供 WebMux 的混合会话支持，可在终端与聊天视图之间切换；原有 omp 也继续支持混合会话。
其他八款使用终端视图，不能仅凭其提供 ACP 接口就视为已适配 WebMux 的切换和恢复行为。

运行、空闲与等待状态使用通用终端判断；具体的回答、授权与完成提醒取决于 CLI 发出的结构化事件、OSC 通知或 BEL。
现有 Claude Code、Codex 和 omp 提醒配置见 [使用指南](usage.md#需要你介入的提醒)。
